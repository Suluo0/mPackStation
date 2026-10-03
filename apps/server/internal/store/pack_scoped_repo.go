package store

// pack_scoped_repo.go is the SQL boundary for the immutable mod identity,
// selection, input-file and parse-batch model. The older pack_mods and
// mod_content tables remain available as compatibility projections; new
// writes are mirrored into this model whenever the input declaration is
// sufficiently known.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type PackScopedFile struct {
	// SHA512 只用于 .mrpack manifest（规范必填），来自下载字节实测；没有就不写进 expected_hashes。
	ID, SHA256, SHA1, SHA512, MediaType, Location string
	SizeBytes                                     int64
	Verified                                      bool
}

type PackScopedInput struct {
	File        PackScopedFile
	Role        string
	LogicalPath string
}

type PackScopedSelection struct {
	PackID, PackModID, ModID, SelectionID, VersionID, SourceID     string
	DeclaredVersion, ManifestSHA256, Acquisition, RequestedVersion string
	Platform, ExternalProjectID, ProjectSlug, ProjectDisplayName   string
	ExternalReleaseID, ReleaseName, ReleaseFileKey, DownloadURL    string
	File                                                           *PackScopedFile
	AdditionalFiles                                                []PackScopedInput
	LogicalPath, FileRole                                          string
	CreatedAt                                                      int64
	VersionStatus                                                  string
}

type ScopedDefinition struct {
	ID, KeyID, Kind, Registry, ResourceKey, ArchivePath string
	Payload, Diagnostics                                json.RawMessage
	ParseStatus, RuntimeStatus                          string
	InputNo                                             int
}

type ScopedParse struct {
	PackID, RunID, SourceID, TaskID, ParserVersion string
	DeclaredModID, DeclaredVersion, DescriptorPath string
	ConfigRevision                                 int64
	InputSHA256                                    string
	File                                           PackScopedFile
	AdditionalFiles                                []PackScopedInput
	LogicalPath, FileRole                          string
	Definitions                                    []ScopedDefinition
	CreatedAt                                      int64
}

func (r *Repository) RecordScopedParseFailure(ctx context.Context, packID, runID, taskID, parserVersion, code, message string, configRevision, at int64) error {
	diagnostics, _ := json.Marshal(map[string]string{"code": code, "message": message})
	if _, err := r.db.ExecContext(ctx, `INSERT INTO parse_runs(pack_id,id,phase,task_id,config_revision,parser_version,execution_status,completeness,diagnostics,started_at,finished_at,created_at) VALUES(?,?,'metadata',?,?,?,'failed','unresolved',?,?,?,?)`, packID, runID, taskID, configRevision, parserVersion, string(diagnostics), at, at, at); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO parse_coverage(pack_id,run_id,kind,resource_status,completeness,diagnostics) VALUES(?,?,'archive','failed','unresolved',?)`, packID, runID, string(diagnostics))
	return err
}

// ListPackMembers is the complete pack-scoped membership list, including the
// required builtin Minecraft row. ListPackMods intentionally keeps its legacy
// user-mod semantics for existing API clients.
func (r *Repository) ListPackMembers(ctx context.Context, packID string) ([]PackModRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,source,COALESCE(project_id,''),COALESCE(version_id,''),display_name,file_name,COALESCE(sha1,''),status,required,added_at,updated_at,mirror_source,COALESCE(mirror_project_id,''),COALESCE(mirror_version_id,''),origin,COALESCE(mod_id,''),COALESCE(current_selection_id,''),COALESCE(category,'') FROM pack_mods WHERE pack_id=? AND status<>'removed' ORDER BY CASE WHEN mod_id='minecraft' THEN 0 ELSE 1 END, COALESCE(NULLIF(category,''),'未分类') COLLATE NOCASE, display_name COLLATE NOCASE,id`, packID)
	if err != nil {
		return nil, fmt.Errorf("list pack members: %w", err)
	}
	defer rows.Close()
	var out []PackModRecord
	for rows.Next() {
		m, err := scanPackMod(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanPackMod(s interface{ Scan(...any) error }) (PackModRecord, error) {
	var m PackModRecord
	var req int
	if err := s.Scan(&m.ID, &m.PackID, &m.Source, &m.ProjectID, &m.VersionID, &m.DisplayName, &m.FileName, &m.SHA1, &m.Status, &req, &m.AddedAt, &m.UpdatedAt, &m.MirrorSource, &m.MirrorProjectID, &m.MirrorVersionID, &m.Origin, &m.ModID, &m.CurrentSelectionID, &m.Category); err != nil {
		return m, err
	}
	m.Required = req != 0
	return m, nil
}

// EnsurePackSelection registers one exact version and its verified input file,
// then points the pack member at a new immutable selection. It is idempotent
// for the supplied IDs and never mutates an existing version's manifest.
func (r *Repository) EnsurePackSelection(ctx context.Context, s PackScopedSelection) error {
	if s.PackID == "" || s.PackModID == "" || s.ModID == "" || s.VersionID == "" || s.SelectionID == "" || s.SourceID == "" {
		return ErrInvalidArgument
	}
	if s.ManifestSHA256 == "" {
		return ErrInvalidArgument
	}
	status := s.VersionStatus
	if status == "" {
		status = "pending"
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO mods(mod_id,display_name,kind) VALUES(?,COALESCE((SELECT display_name FROM pack_mods WHERE pack_id=? AND id=?),?),CASE WHEN ?='minecraft' THEN 'builtin' ELSE 'normal' END) ON CONFLICT(mod_id) DO NOTHING`, s.ModID, s.PackID, s.PackModID, s.ModID, s.ModID); err != nil {
		return fmt.Errorf("register mod identity: %w", err)
	}
	if s.File != nil {
		if err := r.registerScopedFile(ctx, *s.File, s.CreatedAt); err != nil {
			return err
		}
	}
	for _, input := range s.AdditionalFiles {
		if err := r.registerScopedFile(ctx, input.File, s.CreatedAt); err != nil {
			return err
		}
	}
	if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO mod_versions(id,mod_id,declared_version,input_manifest_sha256,created_at,status) VALUES(?,?,?,?,?,?)`, s.VersionID, s.ModID, s.DeclaredVersion, s.ManifestSHA256, s.CreatedAt, status); err != nil {
		return fmt.Errorf("register mod version: %w", err)
	}
	if s.File != nil {
		if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO version_files(version_id,role,logical_path,file_id,required) VALUES(?,?,?,?,1)`, s.VersionID, nonEmpty(s.FileRole, "primary"), nonEmpty(s.LogicalPath, s.File.ID), s.File.ID); err != nil {
			return fmt.Errorf("register version file: %w", err)
		}
	}
	for _, input := range s.AdditionalFiles {
		if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO version_files(version_id,role,logical_path,file_id,required) VALUES(?,?,?,?,1)`, s.VersionID, input.Role, input.LogicalPath, input.File.ID); err != nil {
			return fmt.Errorf("register additional version file: %w", err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE pack_mods SET mod_id=? WHERE pack_id=? AND id=? AND (mod_id IS NULL OR mod_id='')`, s.ModID, s.PackID, s.PackModID); err != nil {
		return fmt.Errorf("bind pack mod identity: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO pack_mod_selections(pack_id,id,pack_mod_id,mod_id,version_id,acquisition,requested_version,created_at,status) VALUES(?,?,?,?,?,?,?,?,?)`, s.PackID, s.SelectionID, s.PackModID, s.ModID, s.VersionID, s.Acquisition, s.RequestedVersion, s.CreatedAt, status); err != nil {
		return fmt.Errorf("register pack selection: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE pack_mods SET current_selection_id=? WHERE pack_id=? AND id=?`, s.SelectionID, s.PackID, s.PackModID); err != nil {
		return fmt.Errorf("set current pack selection: %w", err)
	}
	if status == "ready" {
		if _, err := r.db.ExecContext(ctx, `UPDATE pack_mods SET status='installed',updated_at=? WHERE pack_id=? AND id=?`, s.CreatedAt, s.PackID, s.PackModID); err != nil {
			return fmt.Errorf("mark selected pack mod installed: %w", err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO pack_content_sources(pack_id,id,kind,selection_id,external_revision,display_name,descriptor,created_at) VALUES(?,?,?,?,?,?,?,?)`, s.PackID, s.SourceID, "mod", s.SelectionID, s.DeclaredVersion, "", `{}`, s.CreatedAt); err != nil {
		return fmt.Errorf("register content source: %w", err)
	}
	if s.Platform != "" && s.ExternalProjectID != "" {
		projectID := NewGlobalID("project", s.Platform+"\x00"+s.ExternalProjectID)
		if _, err := r.db.ExecContext(ctx, `INSERT INTO platform_projects(id,platform,external_project_id,slug,display_name) VALUES(?,?,?,?,?) ON CONFLICT(platform,external_project_id) DO UPDATE SET slug=excluded.slug,display_name=excluded.display_name`, projectID, s.Platform, s.ExternalProjectID, s.ProjectSlug, s.ProjectDisplayName); err != nil {
			return fmt.Errorf("register platform project: %w", err)
		}
		if _, err := r.db.ExecContext(ctx, `INSERT INTO mod_project_links(mod_id,project_id,status,evidence,confirmed_at) VALUES(?,?,'confirmed','{"source":"jar_descriptor"}',?) ON CONFLICT(mod_id,project_id) DO UPDATE SET status='confirmed',confirmed_at=excluded.confirmed_at`, s.ModID, projectID, s.CreatedAt); err != nil {
			return fmt.Errorf("link platform project: %w", err)
		}
		if s.ExternalReleaseID != "" {
			releaseID := NewGlobalID("release", projectID+"\x00"+s.ExternalReleaseID)
			if _, err := r.db.ExecContext(ctx, `INSERT INTO platform_releases(id,project_id,external_version_id,version_name) VALUES(?,?,?,?) ON CONFLICT(project_id,external_version_id) DO UPDATE SET version_name=excluded.version_name`, releaseID, projectID, s.ExternalReleaseID, s.ReleaseName); err != nil {
				return fmt.Errorf("register platform release: %w", err)
			}
			if s.File != nil {
				releaseFileID := NewGlobalID("release-file", releaseID+"\x00"+nonEmpty(s.ReleaseFileKey, s.File.ID))
				hashPayload := map[string]string{"sha1": s.File.SHA1, "sha256": s.File.SHA256}
				if s.File.SHA512 != "" {
					hashPayload["sha512"] = s.File.SHA512
				}
				hashes, _ := json.Marshal(hashPayload)
				if _, err := r.db.ExecContext(ctx, `INSERT INTO platform_release_files(id,release_id,file_key,file_name,download_url,expected_size,expected_hashes,file_id,verification_status) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(release_id,file_key) DO UPDATE SET file_id=excluded.file_id,verification_status=excluded.verification_status`, releaseFileID, releaseID, nonEmpty(s.ReleaseFileKey, s.File.ID), s.LogicalPath, s.DownloadURL, s.File.SizeBytes, string(hashes), s.File.ID, "verified"); err != nil {
					return fmt.Errorf("register platform release file: %w", err)
				}
				if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO version_release_files(version_id,release_file_id,status,evidence) VALUES(?,?,'verified','{"source":"download_hash"}')`, s.VersionID, releaseFileID); err != nil {
					return fmt.Errorf("link version release file: %w", err)
				}
				if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO selection_platform_pins(pack_id,selection_id,role,release_file_id) VALUES(?,?,'primary',?)`, s.PackID, s.SelectionID, releaseFileID); err != nil {
					return fmt.Errorf("pin platform release file: %w", err)
				}
			}
		}
	}
	return nil
}

func (r *Repository) registerScopedFile(ctx context.Context, f PackScopedFile, at int64) error {
	if f.ID == "" || len(f.SHA256) != 64 {
		return fmt.Errorf("invalid scoped file identity")
	}
	if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO file_objects(id,sha256,sha1,size_bytes,media_type,created_at) VALUES(?,?,?,?,?,?)`, f.ID, f.SHA256, nullString(f.SHA1), f.SizeBytes, f.MediaType, at); err != nil {
		return fmt.Errorf("register file object: %w", err)
	}
	if f.Location != "" {
		availability := "missing"
		if f.Verified {
			availability = "available"
		}
		if _, err := r.db.ExecContext(ctx, `INSERT INTO file_locations(file_id,location,availability,verified_at) VALUES(?,?,?,?) ON CONFLICT(file_id,location) DO UPDATE SET availability=excluded.availability,verified_at=excluded.verified_at`, f.ID, f.Location, availability, at); err != nil {
			return fmt.Errorf("register file location: %w", err)
		}
	}
	return nil
}

// PersistScopedParse records the immutable run, its frozen input and all
// source definitions. Child tables are filled by PersistScopedDefinition.
func (r *Repository) PersistScopedParse(ctx context.Context, p ScopedParse) error {
	if p.PackID == "" || p.RunID == "" || p.SourceID == "" || p.File.ID == "" {
		return ErrInvalidArgument
	}
	if err := r.registerScopedFile(ctx, p.File, p.CreatedAt); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO source_inputs(pack_id,source_id,input_no,file_id,role,logical_path,provenance) VALUES(?,?,?,?,?,?,?)`, p.PackID, p.SourceID, 0, p.File.ID, nonEmpty(p.FileRole, "primary"), nonEmpty(p.LogicalPath, p.File.ID), `{"verified":true}`); err != nil {
		return fmt.Errorf("freeze source input: %w", err)
	}
	for index, input := range p.AdditionalFiles {
		if err := r.registerScopedFile(ctx, input.File, p.CreatedAt); err != nil {
			return err
		}
		inputNo := index + 1
		if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO source_inputs(pack_id,source_id,input_no,file_id,role,logical_path,provenance) VALUES(?,?,?,?,?,?,?)`, p.PackID, p.SourceID, inputNo, input.File.ID, input.Role, input.LogicalPath, `{"verified":true}`); err != nil {
			return fmt.Errorf("freeze additional source input: %w", err)
		}
	}
	partialCount, dynamicCount, unsupportedCount, errorCount := 0, 0, 0, 0
	for _, definition := range p.Definitions {
		switch definition.ParseStatus {
		case "partial":
			partialCount++
		case "unsupported":
			unsupportedCount++
		case "invalid":
			errorCount++
		}
		if definition.RuntimeStatus == "dynamic" {
			dynamicCount++
		}
	}
	completeness := "complete"
	if partialCount+dynamicCount+unsupportedCount+errorCount > 0 {
		completeness = "partial"
	}
	parsedCount := len(p.Definitions) - unsupportedCount - errorCount
	if _, err := r.db.ExecContext(ctx, `INSERT INTO parse_runs(pack_id,id,source_id,phase,task_id,config_revision,environment_sha256,parser_version,input_sha256,execution_status,completeness,total_files,parsed_count,partial_count,dynamic_count,unsupported_count,error_count,diagnostics,started_at,finished_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,'succeeded',?,?,?,?,?,?,?, '{}',?,?,?)`, p.PackID, p.RunID, p.SourceID, "content", p.TaskID, p.ConfigRevision, "", p.ParserVersion, p.InputSHA256, completeness, len(p.Definitions), parsedCount, partialCount, dynamicCount, unsupportedCount, errorCount, p.CreatedAt, p.CreatedAt, p.CreatedAt); err != nil {
		return fmt.Errorf("create scoped parse run: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO parse_run_inputs(pack_id,run_id,input_no,source_input_no,logical_path,actual_sha256,file_id) VALUES(?,?,?,?,?,?,?)`, p.PackID, p.RunID, 0, 0, nonEmpty(p.LogicalPath, p.File.ID), p.File.SHA256, p.File.ID); err != nil {
		return fmt.Errorf("record parse input: %w", err)
	}
	for index, input := range p.AdditionalFiles {
		inputNo := index + 1
		if _, err := r.db.ExecContext(ctx, `INSERT INTO parse_run_inputs(pack_id,run_id,input_no,source_input_no,logical_path,actual_sha256,file_id) VALUES(?,?,?,?,?,?,?)`, p.PackID, p.RunID, inputNo, inputNo, input.LogicalPath, input.File.SHA256, input.File.ID); err != nil {
			return fmt.Errorf("record additional parse input: %w", err)
		}
	}
	if p.DeclaredModID != "" {
		descriptor := nonEmpty(p.DescriptorPath, "builtin:declaration")
		metadata, _ := json.Marshal(map[string]string{"source": descriptor})
		if _, err := r.db.ExecContext(ctx, `INSERT INTO file_mod_declarations(pack_id,run_id,input_no,descriptor_path,ordinal,declared_mod_id,declared_version,resolved_mod_id,metadata,status) VALUES(?,?,?,?,0,?,?,?,?,?)`, p.PackID, p.RunID, 0, descriptor, p.DeclaredModID, p.DeclaredVersion, p.DeclaredModID, string(metadata), "resolved"); err != nil {
			return fmt.Errorf("record mod declaration: %w", err)
		}
	}
	for _, d := range p.Definitions {
		if err := r.PersistScopedDefinition(ctx, p.PackID, p.RunID, d); err != nil {
			return err
		}
	}
	kinds := map[string][2]int{}
	for _, d := range p.Definitions {
		counts := kinds[d.Kind]
		counts[0]++
		if d.ParseStatus == "parsed" || d.ParseStatus == "partial" {
			counts[1]++
		}
		kinds[d.Kind] = counts
	}
	for kind, counts := range kinds {
		kindCompleteness := "complete"
		if counts[0] != counts[1] {
			kindCompleteness = "partial"
		}
		if _, err := r.db.ExecContext(ctx, `INSERT INTO parse_coverage(pack_id,run_id,kind,resource_status,completeness,scanned_count,parsed_count) VALUES(?,?,?,'ready',?,?,?)`, p.PackID, p.RunID, kind, kindCompleteness, counts[0], counts[1]); err != nil {
			return fmt.Errorf("record parse coverage: %w", err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO parse_coverage(pack_id,run_id,kind,resource_status,completeness,scanned_count,parsed_count,partial_count,dynamic_count,unsupported_count,invalid_count) VALUES(?,?,'archive','ready',?,?,?,?,?,?,?)`, p.PackID, p.RunID, completeness, len(p.Definitions), parsedCount, partialCount, dynamicCount, unsupportedCount, errorCount); err != nil {
		return fmt.Errorf("record archive coverage: %w", err)
	}
	return nil
}

func (r *Repository) PersistScopedDefinition(ctx context.Context, packID, runID string, d ScopedDefinition) error {
	if d.ID == "" || d.KeyID == "" || d.ResourceKey == "" {
		return ErrInvalidArgument
	}
	if d.ParseStatus == "" {
		d.ParseStatus = "parsed"
	}
	if d.RuntimeStatus == "" {
		d.RuntimeStatus = "static"
	}
	diag := d.Diagnostics
	if len(diag) == 0 {
		diag = json.RawMessage(`{}`)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO content_keys(pack_id,id,kind,registry,resource_key,qualifier) VALUES(?,?,?,?,?,?)`, packID, d.KeyID, d.Kind, d.Registry, d.ResourceKey, ""); err != nil {
		return fmt.Errorf("create content key: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO content_definitions(pack_id,id,key_id,run_id,input_no,archive_path,fragment_key,raw_text,parse_status,runtime_status,diagnostics) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, packID, d.ID, d.KeyID, runID, d.InputNo, d.ArchivePath, "", string(d.Payload), d.ParseStatus, d.RuntimeStatus, string(diag)); err != nil {
		return fmt.Errorf("create content definition: %w", err)
	}
	switch d.Kind {
	case "recipe":
		var root map[string]json.RawMessage
		_ = json.Unmarshal(d.Payload, &root)
		recipeType := "unknown"
		_ = json.Unmarshal(root["type"], &recipeType)
		_, err := r.db.ExecContext(ctx, `INSERT INTO recipe_definitions(pack_id,definition_id,recipe_type,layout) VALUES(?,?,?,?)`, packID, d.ID, recipeType, string(d.Payload))
		if err != nil {
			return fmt.Errorf("create recipe definition: %w", err)
		}
		if err := r.persistRecipeTerms(ctx, packID, d.ID, d.Payload); err != nil {
			return err
		}
	case "tag":
		var tagRoot struct {
			Replace bool              `json:"replace"`
			Values  []json.RawMessage `json:"values"`
			Remove  []json.RawMessage `json:"remove"`
		}
		_ = json.Unmarshal(d.Payload, &tagRoot)
		if _, err := r.db.ExecContext(ctx, `INSERT INTO tag_definitions(pack_id,definition_id,replace_existing) VALUES(?,?,?)`, packID, d.ID, boolInt(tagRoot.Replace)); err != nil {
			return fmt.Errorf("create tag definition: %w", err)
		}
		ordinal := 0
		for _, group := range []struct {
			operation string
			values    []json.RawMessage
		}{{"add", tagRoot.Values}, {"remove", tagRoot.Remove}} {
			for _, raw := range group.values {
				target, required := "", true
				if json.Unmarshal(raw, &target) != nil {
					var object struct {
						ID       string `json:"id"`
						Required *bool  `json:"required"`
					}
					if json.Unmarshal(raw, &object) != nil {
						continue
					}
					target = object.ID
					if object.Required != nil {
						required = *object.Required
					}
				}
				targetKind, keyKind := "member", "item_model"
				if len(target) > 0 && target[0] == '#' {
					targetKind, keyKind, target = "tag", "tag", target[1:]
				}
				if target == "" {
					continue
				}
				targetKeyID, err := r.ensureContentKey(ctx, packID, keyKind, d.Registry, target)
				if err != nil {
					return err
				}
				if _, err := r.db.ExecContext(ctx, `INSERT INTO tag_entries(pack_id,definition_id,ordinal,operation,target_kind,target_key_id,required,raw_entry) VALUES(?,?,?,?,?,?,?,?)`, packID, d.ID, ordinal, group.operation, targetKind, targetKeyID, boolInt(required), string(raw)); err != nil {
					return fmt.Errorf("create tag entry: %w", err)
				}
				ordinal++
			}
		}
	case "lang":
		if _, err := r.db.ExecContext(ctx, `INSERT INTO language_definitions(pack_id,definition_id,text) VALUES(?,?,?)`, packID, d.ID, string(d.Payload)); err != nil {
			return fmt.Errorf("create language definition: %w", err)
		}
		var values map[string]json.RawMessage
		if json.Unmarshal(d.Payload, &values) == nil {
			keys := make([]string, 0, len(values))
			for key := range values {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for ordinal, key := range keys {
				if _, err := r.db.ExecContext(ctx, `INSERT INTO content_text_bindings(pack_id,definition_id,role,ordinal,translation_key,evidence) VALUES(?,?,'translation',?,?,?)`, packID, d.ID, ordinal, key, `{"source":"language_file"}`); err != nil {
					return fmt.Errorf("create language binding: %w", err)
				}
			}
		}
	case "item_model", "texture", "block_model":
		form := d.Kind
		if _, err := r.db.ExecContext(ctx, `INSERT INTO asset_definitions(pack_id,definition_id,form,support_status,reason) VALUES(?,?,?,?,?)`, packID, d.ID, form, d.ParseStatus, ""); err != nil {
			return fmt.Errorf("create asset definition: %w", err)
		}
	}
	return nil
}

func (r *Repository) ensureContentKey(ctx context.Context, packID, kind, registry, resourceKey string) (string, error) {
	id := NewScopedID("key", packID, kind+"\x00"+registry+"\x00"+resourceKey)
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO content_keys(pack_id,id,kind,registry,resource_key,qualifier) VALUES(?,?,?,?,?,'')`, packID, id, kind, registry, resourceKey)
	return id, err
}

func (r *Repository) persistRecipeTerms(ctx context.Context, packID, definitionID string, payload json.RawMessage) error {
	var root map[string]json.RawMessage
	if json.Unmarshal(payload, &root) != nil {
		return nil
	}
	inputSlot := 0
	insert := func(role string, slot int, raw json.RawMessage) error {
		alternatives := []json.RawMessage{raw}
		var array []json.RawMessage
		if json.Unmarshal(raw, &array) == nil {
			alternatives = array
		}
		for alternative, value := range alternatives {
			id, count := "", 1
			if json.Unmarshal(value, &id) != nil {
				var object struct {
					Item, ID, Tag string
					Count         int
				}
				if json.Unmarshal(value, &object) != nil {
					continue
				}
				id, count = object.Item, object.Count
				if id == "" {
					id = object.ID
				}
				if object.Tag != "" {
					id = "#" + object.Tag
				}
			}
			if count <= 0 {
				count = 1
			}
			refKind, keyKind := "item", "item_model"
			if len(id) > 0 && id[0] == '#' {
				refKind, keyKind, id = "item_tag", "tag", id[1:]
			}
			if id == "" {
				continue
			}
			keyID, err := r.ensureContentKey(ctx, packID, keyKind, "item", id)
			if err != nil {
				return err
			}
			if _, err := r.db.ExecContext(ctx, `INSERT INTO recipe_terms(pack_id,definition_id,role,slot,alternative,ref_kind,target_key_id,count,raw_term) VALUES(?,?,?,?,?,?,?,?,?)`, packID, definitionID, role, slot, alternative, refKind, keyID, count, string(value)); err != nil {
				return fmt.Errorf("create recipe term: %w", err)
			}
		}
		return nil
	}
	var ingredients []json.RawMessage
	if json.Unmarshal(root["ingredients"], &ingredients) == nil {
		for _, ingredient := range ingredients {
			if err := insert("input", inputSlot, ingredient); err != nil {
				return err
			}
			inputSlot++
		}
	} else {
		var key map[string]json.RawMessage
		var pattern []string
		if json.Unmarshal(root["key"], &key) == nil && json.Unmarshal(root["pattern"], &pattern) == nil {
			for _, row := range pattern {
				for _, symbol := range row {
					if symbol == ' ' {
						continue
					}
					if raw := key[string(symbol)]; len(raw) > 0 {
						if err := insert("input", inputSlot, raw); err != nil {
							return err
						}
						inputSlot++
					}
				}
			}
		}
	}
	if raw := root["result"]; len(raw) > 0 {
		return insert("output", 0, raw)
	}
	return nil
}

// NewScopedID makes deterministic IDs for immutable evidence rows.
func NewScopedID(prefix, packID, value string) string {
	sum := sha256.Sum256([]byte(packID + "\x00" + value))
	return prefix + "-" + hex.EncodeToString(sum[:12])
}

// NewGlobalID makes deterministic IDs for globally shareable evidence rows.
func NewGlobalID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:12])
}

func nonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

var _ = sql.ErrNoRows
var _ = errors.Is
