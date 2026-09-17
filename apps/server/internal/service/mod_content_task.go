package service

// mod_content_task.go wires the pure extractor into the task queue and HTTP
// API. It owns: task submission, task execution (download jar -> extract ->
// persist), and read-only query methods consumed by httpapi.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

var (
	// ErrModContentNotParsed means no parse run exists for this mod.
	ErrModContentNotParsed = errors.New("mod content not parsed")
	// ErrModContentAlreadyCurrent means the same jar sha1 was already parsed.
	ErrModContentAlreadyCurrent = errors.New("mod content already current")
	// ErrJarBytesUnavailable means a local mod has no persisted jar bytes and
	// cannot be re-downloaded.
	ErrJarBytesUnavailable = errors.New("jar bytes unavailable")
	// ErrInvalidContentKind means the kind filter is not in the whitelist.
	ErrInvalidContentKind = errors.New("invalid content kind")
)

var validContentKinds = map[string]bool{
	"metadata": true, "recipe": true, "item_model": true, "structure": true,
	"worldgen": true, "loot_table": true, "advancement": true, "tag": true, "lang": true, "texture": true, "ammo_definition": true, "item_icon": true,
}

// parseModContentPayload is the task payload.
type parseModContentPayload struct {
	PackID string `json:"packId"`
	ModID  string `json:"modId"`
}

// ModContentItem is the API DTO for one extracted content row.
type ModContentItem struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Path       string          `json:"path"`
	Key        string          `json:"key"`
	Payload    json.RawMessage `json:"payload"`
	IsDynamic  bool            `json:"isDynamic"`
	ParseError string          `json:"parseError,omitempty"`
}

// ModContentRun is the API DTO for a parse run summary.
type ModContentRun struct {
	Status       string `json:"status"`
	ParsedCount  int    `json:"parsedCount"`
	DynamicCount int    `json:"dynamicCount"`
	ErrorCount   int    `json:"errorCount"`
	TotalFiles   int    `json:"totalFiles"`
	ParsedAt     string `json:"parsedAt,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// SubmitParseModContent enqueues a parse task for one installed mod. Returns
// the task and a bool indicating whether it was newly submitted (false =
// idempotent skip because the same sha1 is already parsed).
func (a *API) SubmitParseModContent(ctx context.Context, packID, modID string) (*task.Task, bool, error) {
	if err := a.ready(); err != nil {
		return nil, false, err
	}
	if a.queue == nil {
		return nil, false, &DomainError{Status: 503, Code: "task_queue_unavailable", Message: "task queue not configured"}
	}
	mod, err := a.repo.GetPackModInPack(ctx, packID, modID)
	if err != nil {
		return nil, false, &DomainError{Status: 404, Code: "mod_not_found", Message: "mod not found"}
	}
	if mod.Status != "installed" && mod.ModID != "minecraft" {
		return nil, false, &DomainError{Status: 422, Code: "mod_not_installed", Message: "mod is not installed"}
	}
	// Idempotency: if the latest run succeeded for this exact sha1, skip.
	if mod.SHA1 != "" {
		if run, err := a.repo.LatestModContentRun(ctx, modID); err == nil && run.Status == "succeeded" && run.SHA1 == mod.SHA1 {
			return nil, false, ErrModContentAlreadyCurrent
		}
	}
	payload, _ := json.Marshal(parseModContentPayload{PackID: packID, ModID: modID})
	t, _, err := a.queue.Submit(ctx, task.SubmitRequest{
		Kind:        task.KindParseModContent,
		Title:       "Parse mod content: " + mod.DisplayName,
		Payload:     payload,
		MaxAttempts: 2,
		/* No IdempotencyKey: failed parses must be retryable. Idempotency is
		   enforced inside the handler (same sha1 already succeeded → short-circuit). */
	})
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}

// ParseResult summarises a completed parse run.
type ParseResult struct {
	TotalFiles, ParsedCount, DynamicCount, ErrorCount int
}

// HandleParseModContentTask is the queue worker for KindParseModContent.
func (a *API) HandleParseModContentTask(ctx context.Context, ex *task.Execution) error {
	var p parseModContentPayload
	if err := json.Unmarshal(ex.Task.Payload, &p); err != nil {
		return &task.TaskError{Code: "invalid_payload", Message: "task payload is not valid JSON"}
	}
	result, err := a.parseAndPersistModContent(ctx, p.PackID, p.ModID, ex.Task.ID, func(pct float64, msg string) {
		_ = ex.Progress(ctx, pct, msg)
	})
	if err != nil {
		return err
	}
	if a.queue != nil {
		if _, err := a.SubmitCatalogInit(ctx, p.PackID, "zh_cn"); err != nil {
			return &task.TaskError{Code: "catalog_enqueue_failed", Message: "mod parsed but catalog rebuild could not be queued", Retryable: true}
		}
	}
	ex.Succeed(ctx, fmt.Sprintf("extracted %d items (%d dynamic)", result.ParsedCount, result.DynamicCount))
	return nil
}

// ParseModContentSync executes a parse immediately without the queue.
// Used by tests and the acceptance layer; production uses the async POST.
func (a *API) ParseModContentSync(ctx context.Context, packID, modID string) (*ParseResult, error) {
	return a.parseAndPersistModContent(ctx, packID, modID, "sync", nil)
}

// parseAndPersistModContent is the core logic: validate mod, check idempotency,
// fetch jar, extract, persist, write evidence. Exposed for direct testing.
func (a *API) parseAndPersistModContent(ctx context.Context, packID, modID, taskID string, progress func(float64, string)) (*ParseResult, error) {
	mod, err := a.repo.GetPackModInPack(ctx, packID, modID)
	if err != nil {
		return nil, &task.TaskError{Code: "mod_not_found", Message: "mod not found"}
	}
	if mod.Status != "installed" && mod.ModID != "minecraft" {
		return nil, &task.TaskError{Code: "mod_not_installed", Message: "mod is not installed"}
	}
	now := a.now().UnixMilli()
	runID := newID("mcrun")
	recordScopedFailure := func(code, message string) {
		if revision, revisionErr := a.repo.GetPackConfigRevision(ctx, packID); revisionErr == nil {
			_ = a.repo.RecordScopedParseFailure(ctx, packID, runID, taskID, "mod-content-v2", code, message, revision, now)
		}
	}

	// Idempotency: same sha1 already succeeded.
	if mod.SHA1 != "" {
		if run, err := a.repo.GetModContentRunBySHA(ctx, mod.ID, mod.SHA1); err == nil && run.Status == "succeeded" {
			return &ParseResult{TotalFiles: run.TotalFiles, ParsedCount: run.ParsedCount, DynamicCount: run.DynamicCount, ErrorCount: run.ErrorCount}, nil
		}
	}

	if progress != nil {
		progress(10, "fetching verified content archive")
	}
	var archiveBytes, languageBytes []byte
	var declaredVersion, archiveLocation, descriptorPath string
	if mod.ModID == "minecraft" {
		pack, getErr := a.repo.GetPack(ctx, packID)
		if getErr != nil {
			return nil, &task.TaskError{Code: "pack_not_found", Message: "pack not found"}
		}
		declaredVersion = pack.MCVersion
		archiveLocation = "mojang:client:" + declaredVersion
		descriptorPath = "mojang:version_metadata"
		archiveBytes, err = a.loadMinecraftClientArchive(ctx, declaredVersion)
		if err == nil {
			languageBytes, _ = a.loadVanillaLanguage(ctx, declaredVersion, "zh_cn")
		}
	} else {
		archiveBytes, err = a.fetchModJarBytes(ctx, mod)
		archiveLocation = mod.Source + ":" + mod.ProjectID + ":" + mod.VersionID
	}
	if err != nil {
		a.finishModContentRun(ctx, runID, mod.PackID, mod.ID, mod.SHA1, "failed", 0, 0, 0, 0, "archive fetch failed", now)
		recordScopedFailure("archive_fetch_failed", err.Error())
		return nil, &task.TaskError{Code: "archive_fetch_failed", Message: "failed to fetch content archive", Retryable: true}
	}
	actualSHA1, actualSHA256, verifyErr := validateMeasuredDownload(mod.SHA1, "", archiveBytes)
	if verifyErr != nil {
		a.finishModContentRun(ctx, runID, mod.PackID, mod.ID, mod.SHA1, "failed", 0, 0, 0, 0, "archive checksum mismatch", now)
		recordScopedFailure("archive_hash_mismatch", verifyErr.Error())
		return nil, &task.TaskError{Code: "archive_hash_mismatch", Message: "content archive checksum mismatch"}
	}
	if progress != nil {
		progress(30, "extracting content")
	}
	extracted, err := ExtractModContent(bytes.NewReader(archiveBytes))
	if err != nil {
		a.finishModContentRun(ctx, runID, mod.PackID, mod.ID, mod.SHA1, "failed", 0, 0, 0, 0, "archive extraction failed", now)
		recordScopedFailure("extract_failed", err.Error())
		return nil, &task.TaskError{Code: "extract_failed", Message: "failed to extract mod content"}
	}
	if mod.ModID == "minecraft" {
		extracted.ModID, extracted.Version, extracted.Loader = "minecraft", declaredVersion, "builtin"
		metadata, _ := json.Marshal(map[string]string{"loader": "builtin", "modid": "minecraft", "version": declaredVersion, "source": descriptorPath})
		extracted.Metadata = ContentItem{Kind: "metadata", Path: descriptorPath, Key: "minecraft:metadata", Payload: metadata}
		if len(languageBytes) > 0 {
			extracted.Langs = append(extracted.Langs, ContentItem{Kind: "lang", Path: "assets/minecraft/lang/zh_cn.json", Key: "minecraft:lang/zh_cn", Payload: json.RawMessage(languageBytes)})
			extracted.Stats.TotalFiles++
			extracted.Stats.ParsedCount++
		}
	} else {
		declaredVersion = extracted.Version
		descriptorPath = extracted.Metadata.Path
	}
	canonicalModID := normalizeDeclaredModID(extracted.ModID)
	if canonicalModID == "" || (mod.ModID != "" && canonicalModID != mod.ModID) {
		recordScopedFailure("mod_identity_mismatch", "archive mod id does not match selected mod")
		return nil, &task.TaskError{Code: "mod_identity_mismatch", Message: "archive mod id does not match selected mod"}
	}
	mod.ModID, mod.SHA1, mod.FileName, mod.Status, mod.UpdatedAt = canonicalModID, actualSHA1, nonEmptyString(mod.FileName, canonicalModID+"-"+declaredVersion+".jar"), "installed", a.now().UnixMilli()
	selection := verifiedPackSelection(mod, declaredVersion, actualSHA256, archiveLocation, int64(len(archiveBytes)), mod.UpdatedAt)
	if canonicalModID == "minecraft" {
		selection.Acquisition, selection.Platform = "mojang", ""
		selection.RequestedVersion = declaredVersion
		selection.ExternalProjectID, selection.ExternalReleaseID = "", ""
	}
	if len(languageBytes) > 0 {
		langSHA1, langSHA256 := measuredArchive(languageBytes)
		selection.AdditionalFiles = append(selection.AdditionalFiles, store.PackScopedInput{File: store.PackScopedFile{ID: store.NewGlobalID("file", langSHA256), SHA256: langSHA256, SHA1: langSHA1, SizeBytes: int64(len(languageBytes)), MediaType: "application/json", Location: "mojang:asset:zh_cn:" + declaredVersion, Verified: true}, Role: "language", LogicalPath: "assets/minecraft/lang/zh_cn.json"})
		selection.ManifestSHA256 = manifestHash(actualSHA256, langSHA256)
		selection.VersionID = store.NewGlobalID("version", canonicalModID+"\x00"+declaredVersion+"\x00"+selection.ManifestSHA256)
		selection.SelectionID = store.NewScopedID("selection", packID, mod.ID+"\x00"+selection.VersionID)
		selection.SourceID = store.NewScopedID("source", packID, selection.SelectionID)
	}
	if err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: mod.SHA1, SHA256: actualSHA256, FilePath: archiveLocation, SizeBytes: int64(len(archiveBytes)), ModIDs: []string{canonicalModID}, Loaders: []string{extracted.Loader}, ParsedAt: mod.UpdatedAt}); err != nil {
			return err
		}
		if err := tx.UpdatePackMod(ctx, mod); err != nil {
			return err
		}
		return tx.EnsurePackSelection(ctx, selection)
	}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, &task.TaskError{Code: "duplicate_mod_identity", Message: "the same mod id already exists in this pack"}
		}
		return nil, &task.TaskError{Code: "selection_write_failed", Message: err.Error()}
	}

	// Re-check after resolving the archive's measured identity and hash.
	if run, err := a.repo.GetModContentRunBySHA(ctx, mod.ID, mod.SHA1); err == nil && run.Status == "succeeded" {
		return &ParseResult{TotalFiles: run.TotalFiles, ParsedCount: run.ParsedCount, DynamicCount: run.DynamicCount, ErrorCount: run.ErrorCount}, nil
	}

	if err := a.repo.UpsertModContentRun(ctx, store.ModContentRunRecord{
		ID: runID, PackID: mod.PackID, ModID: mod.ID, SHA1: mod.SHA1,
		Status: "running", StartedAt: now,
	}); err != nil {
		return nil, &task.TaskError{Code: "db_error", Message: "failed to record parse run"}
	}

	if progress != nil {
		progress(80, "persisting results")
	}
	records := a.toModContentRecords(extracted, mod)
	finishedAt := a.now().UnixMilli()
	configRevision, err := a.repo.GetPackConfigRevision(ctx, packID)
	if err != nil {
		return nil, &task.TaskError{Code: "db_error", Message: "failed to read pack revision"}
	}
	scoped := store.ScopedParse{
		PackID: packID, RunID: runID, SourceID: selection.SourceID, TaskID: taskID,
		ParserVersion: "mod-content-v2", ConfigRevision: configRevision,
		InputSHA256: selection.ManifestSHA256,
		File:        *selection.File, AdditionalFiles: selection.AdditionalFiles,
		LogicalPath: selection.LogicalPath, FileRole: selection.FileRole,
		DeclaredModID: canonicalModID, DeclaredVersion: declaredVersion, DescriptorPath: descriptorPath,
		CreatedAt: now, Definitions: scopedDefinitions(packID, runID, records),
	}
	if err := a.repo.PersistScopedParse(ctx, scoped); err != nil {
		a.finishModContentRun(ctx, runID, mod.PackID, mod.ID, mod.SHA1, "failed", 0, 0, 0, 0, "scoped database write failed", now)
		return nil, &task.TaskError{Code: "db_error", Message: "failed to persist versioned parse evidence: " + err.Error()}
	}
	// Atomically replace content rows and mark run as succeeded in one
	// transaction so the two can never diverge.
	if err := a.repo.ReplaceModContentAndFinishRun(ctx, mod.PackID, mod.ID, records, store.ModContentRunRecord{
		ID: runID, PackID: mod.PackID, ModID: mod.ID, SHA1: mod.SHA1,
		Status:     "succeeded",
		TotalFiles: extracted.Stats.TotalFiles, ParsedCount: extracted.Stats.ParsedCount,
		DynamicCount: extracted.Stats.DynamicCount, ErrorCount: extracted.Stats.ErrorCount,
		StartedAt: now, FinishedAt: finishedAt,
	}); err != nil {
		a.finishModContentRun(ctx, runID, mod.PackID, mod.ID, mod.SHA1, "failed", 0, 0, 0, 0, "database write failed", now)
		return nil, &task.TaskError{Code: "db_error", Message: "failed to persist extracted content"}
	}

	_ = store.AddModContentEvidence(ctx, a.repo, mod.PackID, "task:"+taskID, now,
		"mod_content.parsed", mod.ID, map[string]any{
			"mod_id": mod.ID, "sha1": mod.SHA1,
			"parsed_count":  extracted.Stats.ParsedCount,
			"dynamic_count": extracted.Stats.DynamicCount,
			"error_count":   extracted.Stats.ErrorCount,
		})

	return &ParseResult{
		TotalFiles:   extracted.Stats.TotalFiles,
		ParsedCount:  extracted.Stats.ParsedCount,
		DynamicCount: extracted.Stats.DynamicCount,
		ErrorCount:   extracted.Stats.ErrorCount,
	}, nil
}

func scopedDefinitions(packID, runID string, records []store.ModContentRecord) []store.ScopedDefinition {
	result := make([]store.ScopedDefinition, 0, len(records))
	for _, record := range records {
		resourceKey := record.Key
		if resourceKey == "" {
			resourceKey = record.Path
		}
		registry := record.Kind
		if record.Kind == "item_model" {
			registry = "item"
			if strings.Contains(record.Path, "/models/block/") || strings.Contains(record.Path, "/blockstates/") {
				registry = "block"
			}
		} else if record.Kind == "tag" {
			registry = "item"
			if strings.Contains(record.Path, "/tags/block") {
				registry = "block"
			}
		}
		parseStatus, runtimeStatus := "parsed", "static"
		if record.ParseError != "" {
			parseStatus = "invalid"
		} else if record.IsDynamic != 0 {
			parseStatus, runtimeStatus = "partial", "dynamic"
		}
		inputNo := 0
		if record.Path == "assets/minecraft/lang/zh_cn.json" {
			inputNo = 1
		}
		diagnostics, _ := json.Marshal(map[string]string{"parse_error": record.ParseError})
		keyID := store.NewScopedID("key", packID, record.Kind+"\x00"+registry+"\x00"+resourceKey)
		result = append(result, store.ScopedDefinition{
			ID: store.NewScopedID("definition", packID, runID+"\x00"+record.ID), KeyID: keyID,
			Kind: record.Kind, Registry: registry, ResourceKey: resourceKey, ArchivePath: record.Path,
			Payload: json.RawMessage(record.Payload), Diagnostics: diagnostics,
			ParseStatus: parseStatus, RuntimeStatus: runtimeStatus, InputNo: inputNo,
		})
	}
	return result
}

// fetchModJarBytes obtains jar bytes by re-downloading from the provider.
// Local mods without persisted bytes return ErrJarBytesUnavailable.
// (Blobstore-first caching is a future enhancement; currently mods are not
// persisted to blobstore at install time.)
func (a *API) fetchModJarBytes(ctx context.Context, mod store.PackModRecord) ([]byte, error) {
	if mod.Source == "modrinth" || mod.Source == "curseforge" {
		if mod.ProjectID == "" || mod.VersionID == "" {
			return nil, fmt.Errorf("%w: missing project/version id", ErrJarBytesUnavailable)
		}
		ad, err := a.p5Adapter(mod.Source)
		if err != nil {
			return nil, err
		}
		dl, err := ad.Download(ctx, provider.DownloadRequest{ProjectID: mod.ProjectID, VersionID: mod.VersionID})
		if err != nil {
			return nil, mapProviderError(err)
		}
		if len(dl.Content) == 0 {
			return nil, fmt.Errorf("%w: provider returned empty content", ErrJarBytesUnavailable)
		}
		return dl.Content, nil
	}
	return nil, fmt.Errorf("%w: local mod has no persisted jar bytes", ErrJarBytesUnavailable)
}

// ListModContent returns paginated extracted content for a mod.
func (a *API) ListModContent(ctx context.Context, packID, modID, kind string, limit int, cursor string) ([]ModContentItem, string, int, *ModContentRun, error) {
	if err := a.ready(); err != nil {
		return nil, "", 0, nil, err
	}
	if kind != "" && !validContentKinds[kind] {
		return nil, "", 0, nil, ErrInvalidContentKind
	}
	if _, err := a.repo.GetPackModInPack(ctx, packID, modID); err != nil {
		return nil, "", 0, nil, &DomainError{Status: 404, Code: "mod_not_found", Message: "mod not found in this pack"}
	}
	if limit <= 0 || limit > 5000 {
		limit = 100
	}
	run, err := a.repo.LatestModContentRun(ctx, modID)
	if err != nil {
		return nil, "", 0, nil, ErrModContentNotParsed
	}
	records, nextCursor, total, err := a.repo.ListModContent(ctx, packID, modID, kind, limit, cursor)
	if err != nil {
		return nil, "", 0, nil, err
	}
	items := make([]ModContentItem, 0, len(records))
	for _, r := range records {
		items = append(items, toModContentItem(r))
	}
	return items, nextCursor, total, toModContentRun(run), nil
}

// GetModContent returns one extracted content row by id.
func (a *API) GetModContent(ctx context.Context, packID, modID, contentID string) (ModContentItem, error) {
	if err := a.ready(); err != nil {
		return ModContentItem{}, err
	}
	r, err := a.repo.GetModContent(ctx, packID, modID, contentID)
	if err != nil {
		return ModContentItem{}, err
	}
	return toModContentItem(r), nil
}

// GetModContentRun returns the latest parse run for a mod.
func (a *API) GetModContentRun(ctx context.Context, packID, modID string) (*ModContentRun, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPackModInPack(ctx, packID, modID); err != nil {
		return nil, &DomainError{Status: 404, Code: "mod_not_found", Message: "mod not found in this pack"}
	}
	run, err := a.repo.LatestModContentRun(ctx, modID)
	if err != nil {
		return nil, ErrModContentNotParsed
	}
	return toModContentRun(run), nil
}

// --- internal helpers ---

func (a *API) finishModContentRun(ctx context.Context, runID, packID, modID, sha1, status string, totalFiles, parsedCount, dynamicCount, errorCount int, errorMessage string, startedAt int64) {
	finished := a.now().UnixMilli()
	_ = a.repo.UpsertModContentRun(ctx, store.ModContentRunRecord{
		ID: runID, PackID: packID, ModID: modID, SHA1: sha1, Status: status,
		TotalFiles: totalFiles, ParsedCount: parsedCount, DynamicCount: dynamicCount,
		ErrorCount: errorCount, ErrorMessage: errorMessage,
		StartedAt: startedAt, FinishedAt: finished,
	})
}

func (a *API) toModContentRecords(ext *ExtractedContent, mod store.PackModRecord) []store.ModContentRecord {
	now := a.now().UnixMilli()
	all := make([]ContentItem, 0, 64)
	if ext.Metadata.Kind != "" {
		all = append(all, ext.Metadata)
	}
	all = append(all, ext.Recipes...)
	all = append(all, ext.Items...)
	all = append(all, ext.Structures...)
	all = append(all, ext.Worldgen...)
	all = append(all, ext.LootTables...)
	all = append(all, ext.Advancements...)
	all = append(all, ext.Tags...)
	all = append(all, ext.Langs...)
	all = append(all, ext.Textures...)
	all = append(all, ext.ItemIcons...)

	out := make([]store.ModContentRecord, 0, len(all))
	for i, item := range all {
		payload := item.Payload
		if len(payload) == 0 {
			payload = json.RawMessage(`{}`)
		}
		out = append(out, store.ModContentRecord{
			ID:     fmt.Sprintf("mc-%s-%06d", mod.ID, i),
			PackID: mod.PackID, ModID: mod.ID,
			Modid: ext.ModID, Version: ext.Version,
			Kind: item.Kind, Path: item.Path, Key: item.Key,
			Payload: string(payload), IsDynamic: boolInt64(item.IsDynamic),
			ParseError: item.ParseError, ParsedAt: now,
		})
	}
	return out
}

func toModContentItem(r store.ModContentRecord) ModContentItem {
	return ModContentItem{
		ID: r.ID, Kind: r.Kind, Path: r.Path, Key: r.Key,
		Payload: json.RawMessage(r.Payload), IsDynamic: r.IsDynamic != 0,
		ParseError: r.ParseError,
	}
}

func toModContentRun(r store.ModContentRunRecord) *ModContentRun {
	run := &ModContentRun{
		Status: r.Status, ParsedCount: r.ParsedCount, DynamicCount: r.DynamicCount,
		ErrorCount: r.ErrorCount, TotalFiles: r.TotalFiles, ErrorMessage: r.ErrorMessage,
	}
	if r.FinishedAt > 0 {
		run.ParsedAt = time.UnixMilli(r.FinishedAt).UTC().Format(time.RFC3339)
	}
	return run
}

func boolInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
