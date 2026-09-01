package service

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mpackstation/internal/provider"
	"mpackstation/internal/store"
)

var (
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrProviderNotFound    = errors.New("provider resource not found")
	ErrInvalidSHA1         = errors.New("invalid sha1")
)

type ModSearchInput struct {
	Provider, Query, MCVersion, Loader, Cursor string
	Limit                                      int
}
type ModSearchResult struct {
	Items      []provider.Project `json:"items"`
	NextCursor string             `json:"next_cursor"`
	Total      int                `json:"total"`
}
type AddModInput struct {
	Provider, ProjectID, VersionID string
	Required                       bool
}
type LocalModInput struct {
	DisplayName string `json:"displayName"`
	FileName    string `json:"fileName"`
	SHA1        string `json:"sha1"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Required    bool   `json:"required"`
}
type UpdateModInput struct {
	VersionID *string `json:"versionId"`
	Status    *string `json:"status"`
	Required  *bool   `json:"required"`
}
type Mod struct {
	ID          string  `json:"id"`
	PackID      string  `json:"packId"`
	Source      string  `json:"source"`
	ProjectID   *string `json:"projectId"`
	VersionID   *string `json:"versionId"`
	DisplayName string  `json:"displayName"`
	FileName    string  `json:"fileName"`
	SHA1        *string `json:"sha1"`
	Status      string  `json:"status"`
	Required    bool    `json:"required"`
	// Mirror* name the pinned counterpart on the other platform (null when the
	// mod is single-platform here). Versions are pinned at add time on both
	// sides and never auto-updated.
	MirrorSource    *string `json:"mirrorSource"`
	MirrorProjectID *string `json:"mirrorProjectId"`
	// Origin: manual = 手动添加; compat-fix = 兼容知识库自动加装的补丁。
	Origin    string `json:"origin"`
	AddedAt   string `json:"addedAt"`
	UpdatedAt string `json:"updatedAt"`
}
type Lock struct {
	ID             string `json:"id"`
	PackID         string `json:"packId"`
	SchemaVersion  int    `json:"schemaVersion"`
	SnapshotJSON   string `json:"snapshot"`
	SnapshotSHA256 string `json:"sha256"`
	CreatedAt      string `json:"createdAt"`
}
type Conflict struct {
	ID                   string         `json:"id"`
	PackID               string         `json:"packId"`
	Fingerprint          string         `json:"fingerprint"`
	Kind                 string         `json:"kind"`
	Severity             string         `json:"severity"`
	Status               string         `json:"status"`
	Summary              string         `json:"summary"`
	DetailPath           string         `json:"detailPath,omitempty"`
	Detail               map[string]any `json:"detail,omitempty"`
	CreatedAt, UpdatedAt string
	ResolvedAt           *string `json:"resolvedAt,omitempty"`
}
type PackHealth struct {
	PackID          string `json:"packId"`
	Mods            int    `json:"mods"`
	Installed       int    `json:"installed"`
	PendingErrors   int    `json:"pendingErrors"`
	PendingWarnings int    `json:"pendingWarnings"`
	Healthy         bool   `json:"healthy"`
}

var p5Registries sync.Map // *API -> *provider.Registry; composition stays out of transport.

// SetProviderRegistry injects provider adapters at the composition root (most
// commonly fixture adapters in tests, production adapters later).
func (a *API) SetProviderRegistry(r *provider.Registry) {
	if a != nil {
		p5Registries.Store(a, r)
	}
}
func (a *API) p5Registry() *provider.Registry {
	if a == nil {
		return nil
	}
	if r, ok := p5Registries.Load(a); ok {
		return r.(*provider.Registry)
	}
	return nil
}

func (a *API) ModSearch(ctx context.Context, packID string, in ModSearchInput) (ModSearchResult, error) {
	if err := a.ready(); err != nil {
		return ModSearchResult{}, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return ModSearchResult{}, err
	}
	ad, err := a.p5Adapter(in.Provider)
	if err != nil {
		return ModSearchResult{}, err
	}
	r, err := ad.Search(ctx, provider.SearchRequest{Query: in.Query, MCVersion: in.MCVersion, Loader: in.Loader, Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return ModSearchResult{}, mapProviderError(err)
	}
	return ModSearchResult{Items: r.Items, NextCursor: r.NextCursor, Total: r.Total}, nil
}
func (a *API) p5Adapter(name string) (provider.Adapter, error) {
	ad, err := a.p5Registry().Get(name)
	if err != nil {
		return nil, ErrProviderUnavailable
	}
	return ad, nil
}

// ModVersions lists a project's versions on a provider so the UI can offer a
// real version choice before AddPackMod. Read-only; no download happens here.
func (a *API) ModVersions(ctx context.Context, packID, providerName, projectID string) ([]provider.Version, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, ErrInvalidArgument
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	ad, err := a.p5Adapter(providerName)
	if err != nil {
		return nil, err
	}
	return ad.Versions(ctx, projectID)
}
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, provider.ErrNotFound):
		return ErrProviderNotFound
	case errors.Is(err, provider.ErrUnavailable), errors.Is(err, provider.ErrRateLimited), errors.Is(err, provider.ErrUnauthorized):
		return ErrProviderUnavailable
	default:
		return err
	}
}

// ModSearchMirror is the other-platform half of a merged catalog hit. Present
// only when the same mod was found on both platforms (identity table first,
// normalized-name pairing as fallback).
type ModSearchMirror struct {
	Provider  string `json:"provider"`
	ProjectID string `json:"projectId"`
	Slug      string `json:"slug"`
	Downloads int64  `json:"downloads"`
}

// ModSearchAllItem is one catalog hit tagged with the platform it came from.
// When Mirror is set the two entries are the same mod; Downloads is then the
// sum of both platforms so dual-platform mods rank first.
type ModSearchAllItem struct {
	Provider string `json:"provider"`
	provider.Project
	Mirror *ModSearchMirror `json:"mirror,omitempty"`
}

// ModSearchAllResult merges every platform's hits and reports per-platform
// failures independently, so one missing key or outage never blocks the rest.
type ModSearchAllResult struct {
	Items      []ModSearchAllItem `json:"items"`
	Errors     map[string]string  `json:"errors"`
	Total      int                `json:"total"`
	NextCursor *string            `json:"next_cursor"`
}

// providerErrorCode maps provider failures to stable per-platform codes.
func providerErrorCode(err error) string {
	switch {
	case errors.Is(err, provider.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, provider.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, provider.ErrNotFound):
		return "not_found"
	default:
		return "unavailable"
	}
}

// ModSearchAll fans one fuzzy name query out to every known platform
// concurrently. Adapters are stateless, so no locking is needed: each
// goroutine writes only its own result slot. Rate-limit safety comes from
// exactly one request per platform per search, no retries, and a per-platform
// timeout.
func (a *API) ModSearchAll(ctx context.Context, packID string, in ModSearchInput) (ModSearchAllResult, error) {
	if err := a.ready(); err != nil {
		return ModSearchAllResult{}, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return ModSearchAllResult{}, err
	}
	known := []provider.Name{provider.CurseForge, provider.Modrinth}
	type outcome struct {
		name  provider.Name
		items []provider.Project
		err   error
	}
	slots := make([]outcome, len(known))
	var wg sync.WaitGroup
	for i, name := range known {
		slots[i].name = name
		ad, err := a.p5Registry().Get(string(name))
		if err != nil {
			slots[i].err = errNotConfigured
			continue
		}
		wg.Add(1)
		go func(i int, ad provider.Adapter) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			r, err := ad.Search(pctx, provider.SearchRequest{Query: in.Query, MCVersion: in.MCVersion, Loader: in.Loader, Cursor: in.Cursor, Limit: in.Limit})
			if err != nil {
				slots[i].err = err
				return
			}
			slots[i].items = r.Items
		}(i, ad)
	}
	wg.Wait()

	out := ModSearchAllResult{Items: []ModSearchAllItem{}}
	for _, s := range slots {
		if s.err != nil {
			if out.Errors == nil {
				out.Errors = map[string]string{}
			}
			if errors.Is(s.err, errNotConfigured) {
				out.Errors[string(s.name)] = "not_configured"
			} else {
				out.Errors[string(s.name)] = providerErrorCode(s.err)
			}
			continue
		}
		for _, p := range s.items {
			out.Items = append(out.Items, ModSearchAllItem{Provider: string(s.name), Project: p})
		}
	}
	// 跨平台合并: 身份表(本机已确认 + 内置知识库)优先, 名称规范化相同兜底;
	// 配对成功的合并成一张卡, 下载量取两边之和。
	identities, err := a.repo.ListModIdentities(ctx)
	if err != nil {
		identities = nil // 合并是增强, 身份表读取失败不阻塞搜索
	}
	identities = append(identities, baselineModIdentities()...)
	out.Items = pairSearchItems(out.Items, identities)
	// Deterministic merge: most-downloaded first, provider+name as tiebreak.
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].Downloads != out.Items[j].Downloads {
			return out.Items[i].Downloads > out.Items[j].Downloads
		}
		if out.Items[i].Provider != out.Items[j].Provider {
			return out.Items[i].Provider < out.Items[j].Provider
		}
		return out.Items[i].Name < out.Items[j].Name
	})
	out.Total = len(out.Items)
	return out, nil
}

// normalizeModName folds a mod name for cross-platform comparison: case,
// spaces and punctuation are ignored ("Just Enough Items (JEI)" matches
// "just enough items jei"). Conservative on purpose: only exact normalized
// equality pairs two entries — a missed pair shows two cards, a wrong pair
// would weld two different mods together.
func normalizeModName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// pairSearchItems merges cross-platform duplicates of the same mod. The
// higher-download entry stays primary; the other platform becomes its Mirror.
// Same-provider name collisions are never merged.
func pairSearchItems(items []ModSearchAllItem, identities []store.ModIdentityRecord) []ModSearchAllItem {
	byID := make(map[string]string, 2*len(identities))
	for _, id := range identities {
		k := "pair:" + id.MRProjectID + "|" + id.CFProjectID
		byID["modrinth:"+id.MRProjectID] = k
		byID["curseforge:"+id.CFProjectID] = k
	}
	out := make([]ModSearchAllItem, 0, len(items))
	pos := map[string]int{}
	providerAt := map[string]string{}
	for _, it := range items {
		k := "name:" + normalizeModName(it.Name)
		if pk, ok := byID[it.Provider+":"+it.ID]; ok {
			k = pk
		}
		idx, seen := pos[k]
		if !seen || providerAt[k] == it.Provider {
			if !seen {
				pos[k] = len(out)
				providerAt[k] = it.Provider
			}
			out = append(out, it)
			continue
		}
		if it.Downloads > out[idx].Downloads {
			mirror := &ModSearchMirror{Provider: out[idx].Provider, ProjectID: out[idx].ID, Slug: out[idx].Slug, Downloads: out[idx].Downloads}
			it.Downloads += out[idx].Downloads
			it.Mirror = mirror
			out[idx] = it
			providerAt[k] = it.Provider
		} else {
			out[idx].Mirror = &ModSearchMirror{Provider: it.Provider, ProjectID: it.ID, Slug: it.Slug, Downloads: it.Downloads}
			out[idx].Downloads += it.Downloads
		}
	}
	return out
}

var errNotConfigured = errors.New("provider not configured")

func (a *API) ListPackMods(ctx context.Context, packID string) ([]Mod, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	rows, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return nil, err
	}
	out := make([]Mod, 0, len(rows))
	for _, m := range rows {
		out = append(out, modDTO(m))
	}
	return out, nil
}
func modDTO(m store.PackModRecord) Mod {
	strPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	origin := m.Origin
	if origin == "" {
		origin = "manual"
	}
	return Mod{ID: m.ID, PackID: m.PackID, Source: m.Source, ProjectID: strPtr(m.ProjectID), VersionID: strPtr(m.VersionID), DisplayName: m.DisplayName, FileName: m.FileName, SHA1: strPtr(m.SHA1), Status: m.Status, Required: m.Required, MirrorSource: strPtr(m.MirrorSource), MirrorProjectID: strPtr(m.MirrorProjectID), Origin: origin, AddedAt: iso(m.AddedAt), UpdatedAt: iso(m.UpdatedAt)}
}

// otherProviderOf names the opposite catalog platform, or "" for local mods.
func otherProviderOf(source string) string {
	switch source {
	case "modrinth":
		return "curseforge"
	case "curseforge":
		return "modrinth"
	}
	return ""
}

// resolveMirror best-effort pins the counterpart project+version on the other
// platform at add time. Every failure path is silent by design (用户拍板:
// 镜像查不到照常添加, 标"仅单平台"), and once pinned the mirror never follows
// newer releases — rebuilding the pack reproduces exactly what was debugged.
func (a *API) resolveMirror(ctx context.Context, m *store.PackModRecord, pack store.PackRecord, primaryVersionNumber string) {
	otherName := otherProviderOf(m.Source)
	if otherName == "" || m.ProjectID == "" {
		return
	}
	ad, err := a.p5Adapter(otherName)
	if err != nil {
		return // 对方平台未配置/不可用: 仅单平台
	}
	// 1. 定位对方平台项目: 已知镜像 > 身份表 > 名称精确搜索
	otherProject := m.MirrorProjectID
	if otherProject == "" {
		if ids, err := a.repo.ListModIdentities(ctx); err == nil {
			ids = append(ids, baselineModIdentities()...)
			for _, id := range ids {
				if m.Source == "modrinth" && id.MRProjectID == m.ProjectID {
					otherProject = id.CFProjectID
				} else if m.Source == "curseforge" && id.CFProjectID == m.ProjectID {
					otherProject = id.MRProjectID
				}
				if otherProject != "" {
					break
				}
			}
		}
	}
	if otherProject == "" {
		r, err := ad.Search(ctx, provider.SearchRequest{Query: m.DisplayName, Limit: 10})
		if err != nil {
			return
		}
		for _, p := range r.Items {
			if normalizeModName(p.Name) == normalizeModName(m.DisplayName) {
				otherProject = p.ID
				break
			}
		}
	}
	if otherProject == "" {
		return
	}
	m.MirrorSource, m.MirrorProjectID = otherName, otherProject
	// 2. 立即钉版本: 兼容当前包(MC 版本+loader)优先, 同版本号优先, 否则最新兼容
	if vs, err := ad.Versions(ctx, otherProject); err == nil {
		if best := pickMirrorVersion(vs, pack.MCVersion, pack.Loader, primaryVersionNumber); best != "" {
			m.MirrorVersionID = best
		}
	}
	// 3. 项目级配对永久复用(即使版本没钉到)
	mr, cf := otherProject, m.ProjectID
	if m.Source == "modrinth" {
		mr, cf = m.ProjectID, otherProject
	}
	_ = a.repo.UpsertModIdentity(ctx, store.ModIdentityRecord{MRProjectID: mr, CFProjectID: cf, DisplayName: m.DisplayName, ConfirmedAt: time.Now().UnixMilli()})
}

// pickMirrorVersion chooses the counterpart file: same version number wins,
// otherwise the newest file compatible with the pack (versions arrive
// newest-first from the provider layer). "" means no compatible file exists.
func pickMirrorVersion(vs []provider.Version, mcVersion, loader, wantVersionNumber string) string {
	loader = strings.ToLower(loader)
	firstCompatible := ""
	for _, v := range vs {
		mcOK, loaderOK := false, loader == ""
		for _, g := range v.GameVersions {
			if g == mcVersion {
				mcOK = true
				break
			}
		}
		for _, l := range v.Loaders {
			if strings.ToLower(l) == loader {
				loaderOK = true
				break
			}
		}
		if !mcOK || !loaderOK {
			continue
		}
		if firstCompatible == "" {
			firstCompatible = v.ID
		}
		if wantVersionNumber != "" && v.VersionNumber == wantVersionNumber {
			return v.ID
		}
	}
	return firstCompatible
}

func (a *API) AddPackMod(ctx context.Context, packID string, in AddModInput, requestID string) (Mod, error) {
	return a.addPackMod(ctx, packID, in, requestID, "manual", 0)
}

// addPackMod is the add flow with origin tagging and auto-fix recursion depth.
// origin "manual" = 用户手动添加; "compat-fix" = 兼容知识库自动加装的补丁。
func (a *API) addPackMod(ctx context.Context, packID string, in AddModInput, requestID, origin string, depth int) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	if strings.TrimSpace(in.Provider) == "" || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.VersionID) == "" {
		return Mod{}, ErrInvalidArgument
	}
	packRec, err := a.repo.GetPack(ctx, packID)
	if err != nil {
		return Mod{}, err
	}
	ad, err := a.p5Adapter(in.Provider)
	if err != nil {
		return Mod{}, err
	}
	meta, err := ad.Metadata(ctx, in.ProjectID, in.VersionID)
	if err != nil {
		return Mod{}, mapProviderError(err)
	}
	dl, err := ad.Download(ctx, provider.DownloadRequest{ProjectID: in.ProjectID, VersionID: in.VersionID})
	if err != nil {
		return Mod{}, mapProviderError(err)
	}
	if dl.SHA1 == "" && len(dl.Content) > 0 {
		sum := sha1.Sum(dl.Content)
		dl.SHA1 = hex.EncodeToString(sum[:])
		s := sha256.Sum256(dl.Content)
		dl.SHA256 = hex.EncodeToString(s[:])
	}
	if !validHash(dl.SHA1, 40) {
		return Mod{}, ErrInvalidSHA1
	}
	now := time.Now().UnixMilli()
	id := newID("mod")
	m := store.PackModRecord{ID: id, PackID: packID, Source: string(ad.Name()), ProjectID: in.ProjectID, VersionID: in.VersionID, DisplayName: meta.Project.Name, FileName: dl.FileName, SHA1: strings.ToLower(dl.SHA1), Status: "installed", Required: in.Required, AddedAt: now, UpdatedAt: now, Origin: origin}
	// 添加时立即钉死另一平台的对应版本(查不到不阻塞: 照常添加, 仅单平台)。
	a.resolveMirror(ctx, &m, packRec, meta.Version.VersionNumber)
	activityText := "Added " + m.DisplayName
	if origin == "compat-fix" {
		activityText = "Auto-added compat fix " + m.DisplayName + "(兼容知识库命中, 自动加装)"
	}
	err = a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: m.SHA1, SHA256: dl.SHA256, FilePath: "jar://" + m.SHA1, SizeBytes: dl.Size, ModIDs: []string{id}, ParsedAt: now}); err != nil {
			return err
		}
		if err := tx.AddPackMod(ctx, m); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "add-mod", Text: activityText, At: now}, map[string]any{"mod_id": id}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", id, "mod.added", map[string]any{"mod_id": id}, now)
	})
	if err != nil {
		return Mod{}, err
	}
	// 兼容知识库: 新模组入场后检查已知冲突, 有官方解法且解法模组不在包里就
	// 自动加装(只加兼容补丁不带内容; 深度限制防链式失控)。失败不影响本次添加。
	if depth < 2 {
		a.autoFixCompat(ctx, packRec, requestID, depth)
	}
	return modDTO(m), nil
}

// autoFixCompat scans the pack against the embedded compat knowledge and
// installs fix mods for known issues. Best-effort: any failure is silent —
// unresolved known issues still surface as conflicts on the next resolve.
func (a *API) autoFixCompat(ctx context.Context, pack store.PackRecord, requestID string, depth int) {
	mods, err := a.repo.ListPackMods(ctx, pack.ID)
	if err != nil {
		return
	}
	for _, hit := range scanCompatKnowledge(pack.MCVersion, pack.Loader, mods, baselineCompatKnowledge()) {
		fix := hit.Issue.Fix
		if fix == nil || fix.Type != "install_mod" || fixAlreadyPresent(mods, fix) {
			continue
		}
		// 修复模组优先走 Modrinth(更快), 未配置再用 CurseForge
		providerName, projectID := "", ""
		if fix.Mod.MR != "" {
			if _, err := a.p5Adapter("modrinth"); err == nil {
				providerName, projectID = "modrinth", fix.Mod.MR
			}
		}
		if providerName == "" && fix.Mod.CF != "" {
			if _, err := a.p5Adapter("curseforge"); err == nil {
				providerName, projectID = "curseforge", fix.Mod.CF
			}
		}
		if providerName == "" {
			continue
		}
		ad, _ := a.p5Adapter(providerName)
		vs, err := ad.Versions(ctx, projectID)
		if err != nil {
			continue
		}
		versionID := pickMirrorVersion(vs, pack.MCVersion, pack.Loader, "")
		if versionID == "" {
			continue // 没有兼容当前包的版本, 留给冲突列表提示
		}
		_, _ = a.addPackMod(ctx, pack.ID, AddModInput{Provider: providerName, ProjectID: projectID, VersionID: versionID, Required: false}, requestID, "compat-fix", depth+1)
	}
}

// AddLocalPackMod registers a pre-indexed local JAR by content hash. The
// client never supplies a server filesystem path; blobstore owns that mapping.
func (a *API) AddLocalPackMod(ctx context.Context, packID string, in LocalModInput, requestID string) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return Mod{}, err
	}
	if !validHash(strings.ToLower(in.SHA1), 40) || strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.FileName) == "" || in.Size < 0 {
		return Mod{}, ErrInvalidArgument
	}
	now := time.Now().UnixMilli()
	id := newID("mod")
	m := store.PackModRecord{ID: id, PackID: packID, Source: "local", DisplayName: strings.TrimSpace(in.DisplayName), FileName: strings.TrimSpace(in.FileName), SHA1: strings.ToLower(in.SHA1), Status: "installed", Required: in.Required, AddedAt: now, UpdatedAt: now}
	err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: m.SHA1, SHA256: in.SHA256, FilePath: "jar://" + m.SHA1, SizeBytes: in.Size, ModIDs: []string{id}, ParsedAt: now}); err != nil {
			return err
		}
		if err := tx.AddPackMod(ctx, m); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "add-mod", Text: "Added local " + m.DisplayName, At: now}, map[string]any{"mod_id": id}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", id, "mod.added", map[string]any{"mod_id": id}, now)
	})
	if err != nil {
		return Mod{}, err
	}
	return modDTO(m), nil
}
func validHash(v string, n int) bool {
	if len(v) != n {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func (a *API) UpdatePackMod(ctx context.Context, packID, modID string, in UpdateModInput, requestID string) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	rows, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return Mod{}, err
	}
	var found store.PackModRecord
	ok := false
	for _, m := range rows {
		if m.ID == modID {
			found = m
			ok = true
			break
		}
	}
	if !ok {
		return Mod{}, store.ErrNotFound
	}
	if in.VersionID != nil {
		if strings.TrimSpace(*in.VersionID) == "" {
			return Mod{}, ErrInvalidArgument
		}
		if *in.VersionID != found.VersionID {
			ad, e := a.p5Adapter(found.Source)
			if e != nil {
				return Mod{}, e
			}
			meta, e := ad.Metadata(ctx, found.ProjectID, *in.VersionID)
			if e != nil {
				return Mod{}, mapProviderError(e)
			}
			dl, e := ad.Download(ctx, provider.DownloadRequest{ProjectID: found.ProjectID, VersionID: *in.VersionID})
			if e != nil {
				return Mod{}, mapProviderError(e)
			}
			if !validHash(dl.SHA1, 40) {
				return Mod{}, ErrInvalidSHA1
			}
			found.VersionID, found.SHA1, found.FileName, found.DisplayName, found.Status = *in.VersionID, strings.ToLower(dl.SHA1), dl.FileName, meta.Project.Name, "installed"
			// 主版本换了, 镜像版本跟着重钉(镜像项目沿用已配对的, 不重新找)。
			if packRec, e := a.repo.GetPack(ctx, packID); e == nil {
				a.resolveMirror(ctx, &found, packRec, meta.Version.VersionNumber)
			}
			now := time.Now().UnixMilli()
			found.UpdatedAt = now
			if err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
				if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: found.SHA1, SHA256: dl.SHA256, FilePath: "jar://" + found.SHA1, SizeBytes: dl.Size, ParsedAt: now}); err != nil {
					return err
				}
				return tx.UpdatePackMod(ctx, found)
			}); err != nil {
				return Mod{}, err
			}
			return modDTO(found), nil
		}
		found.VersionID = *in.VersionID
	}
	if in.Status != nil {
		switch *in.Status {
		case "pending", "installed", "disabled":
			found.Status = *in.Status
		default:
			return Mod{}, ErrInvalidArgument
		}
	}
	if in.Required != nil {
		found.Required = *in.Required
	}
	found.UpdatedAt = time.Now().UnixMilli()
	if err := a.repo.UpdatePackMod(ctx, found); err != nil {
		return Mod{}, err
	}
	return modDTO(found), nil
}
func (a *API) RemovePackMod(ctx context.Context, packID, modID, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	at := time.Now().UnixMilli()
	err := a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.RemovePackMod(ctx, packID, modID, at); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "remove-mod", Text: "Removed mod", At: at}, map[string]any{"mod_id": modID}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", modID, "mod.removed", map[string]any{"mod_id": modID}, at)
	})
	return err
}

func (a *API) ResolvePack(ctx context.Context, packID, requestID string) (Lock, error) {
	if err := a.ready(); err != nil {
		return Lock{}, err
	}
	mods, err := a.repo.ListPackMods(ctx, packID)
	if err != nil {
		return Lock{}, err
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })
	lockID := newID("lock")
	deps := []store.ModDependencyRecord{}
	confs := []store.ConflictRecord{}
	snap := struct {
		Schema       int                         `json:"schemaVersion"`
		PackID       string                      `json:"packId"`
		Mods         []Mod                       `json:"mods"`
		Dependencies []store.ModDependencyRecord `json:"dependencies"`
		Conflicts    []Conflict                  `json:"conflicts"`
	}{Schema: 1, PackID: packID, Mods: []Mod{}, Dependencies: nil, Conflicts: nil}
	for _, m := range mods {
		snap.Mods = append(snap.Mods, modDTO(m))
		ad, e := a.p5Adapter(m.Source)
		if e != nil {
			confs = append(confs, conflict(m, "provider_unavailable", "Provider unavailable", e.Error()))
			continue
		}
		meta, e := ad.Metadata(ctx, m.ProjectID, m.VersionID)
		if e != nil {
			confs = append(confs, conflict(m, "dependency", "Metadata unavailable", e.Error()))
			continue
		}
		for _, d := range meta.Dependencies {
			dID := fmt.Sprintf("dep-%s-%d", lockID, len(deps))
			drec := store.ModDependencyRecord{ID: dID, PackID: packID, FromPackModID: m.ID, ToProjectID: d.ProjectID, ToVersionID: d.VersionID, Type: normalizeDepType(d.Kind), Constraint: d.Constraint, Reason: d.Reason, CreatedAt: time.Now().UnixMilli()}
			deps = append(deps, drec)
			snap.Dependencies = append(snap.Dependencies, drec)
			if !hasProject(mods, d.ProjectID) {
				c := conflict(m, "dependency", "Missing dependency "+d.ProjectID, d.Reason)
				confs = append(confs, c)
				snap.Conflicts = append(snap.Conflicts, conflictDTO(c))
			}
		}
	}
	// 兼容知识库: 已知问题未被自动修复的(无解法或解法装不上)进冲突列表。
	if packRec, e := a.repo.GetPack(ctx, packID); e == nil {
		for _, hit := range scanCompatKnowledge(packRec.MCVersion, packRec.Loader, mods, baselineCompatKnowledge()) {
			if hit.Issue.Fix != nil && fixAlreadyPresent(mods, hit.Issue.Fix) {
				continue // 补丁已在包里, 视为已处理
			}
			sev := "warning"
			if hit.Issue.Severity == "fatal" {
				sev = "error"
			}
			summary := hit.Issue.Summary
			detail := map[string]any{"reason": hit.Issue.Source, "modA": hit.ModA.DisplayName, "modB": hit.ModB.DisplayName}
			if hit.Issue.Fix != nil {
				summary += "(解法: " + hit.Issue.Fix.Note + ")"
				detail["fixNote"] = hit.Issue.Fix.Note
			}
			now := time.Now().UnixMilli()
			c := store.ConflictRecord{ID: newID("conflict"), PackID: packID, Fingerprint: hit.ModA.ID + ":" + hit.ModB.ID + ":known_issue", Kind: "known_issue", Severity: sev, Summary: summary, Detail: detail, CreatedAt: now, UpdatedAt: now}
			confs = append(confs, c)
			snap.Conflicts = append(snap.Conflicts, conflictDTO(c))
		}
	}
	raw, _ := json.Marshal(snap)
	sum := sha256.Sum256(raw)
	lock := store.LockRecord{ID: lockID, PackID: packID, SchemaVersion: 1, SnapshotJSON: string(raw), SnapshotSHA256: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UnixMilli()}
	if err := a.repo.CreateLock(ctx, lock, deps, confs, requestID); err != nil {
		return Lock{}, err
	}
	_ = requestID
	return Lock{ID: lock.ID, PackID: packID, SchemaVersion: 1, SnapshotJSON: lock.SnapshotJSON, SnapshotSHA256: lock.SnapshotSHA256, CreatedAt: iso(lock.CreatedAt)}, nil
}
func normalizeDepType(v string) string {
	switch v {
	case "optional", "incompatible", "embedded":
		return v
	default:
		return "required"
	}
}
func hasProject(mods []store.PackModRecord, p string) bool {
	for _, m := range mods {
		if m.ProjectID == p && m.Status != "removed" {
			return true
		}
	}
	return false
}
func conflict(m store.PackModRecord, kind, summary, reason string) store.ConflictRecord {
	if kind != "dependency" && kind != "version" && kind != "loader" && kind != "duplicate" && kind != "crash" {
		kind = "dependency"
	}
	return store.ConflictRecord{ID: newID("conflict"), PackID: m.PackID, Fingerprint: m.ID + ":" + kind + ":" + summary, Kind: kind, Severity: "error", Summary: summary, Detail: map[string]any{"reason": reason}, CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli()}
}
func conflictDTO(c store.ConflictRecord) Conflict {
	return Conflict{ID: c.ID, PackID: c.PackID, Fingerprint: c.Fingerprint, Kind: c.Kind, Severity: c.Severity, Status: c.Status, Summary: c.Summary, DetailPath: c.DetailPath, Detail: c.Detail, CreatedAt: iso(c.CreatedAt), UpdatedAt: iso(c.UpdatedAt)}
}
func (a *API) ListLocks(ctx context.Context, packID string) ([]Lock, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	x, e := a.repo.ListLocks(ctx, packID)
	if e != nil {
		return nil, e
	}
	o := make([]Lock, 0, len(x))
	for _, v := range x {
		o = append(o, Lock{ID: v.ID, PackID: v.PackID, SchemaVersion: v.SchemaVersion, SnapshotJSON: v.SnapshotJSON, SnapshotSHA256: v.SnapshotSHA256, CreatedAt: iso(v.CreatedAt)})
	}
	return o, nil
}
func (a *API) ListConflicts(ctx context.Context, packID string) ([]Conflict, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return nil, err
	}
	x, e := a.repo.ListConflicts(ctx, packID)
	if e != nil {
		return nil, e
	}
	o := make([]Conflict, 0, len(x))
	for _, v := range x {
		o = append(o, conflictDTO(v))
	}
	return o, nil
}
func (a *API) ResolveConflict(ctx context.Context, packID, id, status, requestID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
		return err
	}
	at := time.Now().UnixMilli()
	return a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.ResolveConflict(ctx, packID, id, status, at); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "conflict", Action: status, Text: "Conflict " + status, At: at}, map[string]any{"conflict_id": id}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "conflict", id, "conflict."+status, map[string]any{"conflict_id": id}, at)
	})
}
func (a *API) PackHealth(ctx context.Context, packID string) (PackHealth, error) {
	e := a.ready()
	if e != nil {
		return PackHealth{}, e
	}
	if _, e := a.repo.GetPack(ctx, packID); e != nil {
		return PackHealth{}, e
	}
	p, w, m, i, e := a.repo.PackHealth(ctx, packID)
	if e != nil {
		return PackHealth{}, e
	}
	return PackHealth{PackID: packID, Mods: m, Installed: i, PendingErrors: p, PendingWarnings: w, Healthy: p == 0}, nil
}
