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
	NextCursor string             `json:"nextCursor,omitempty"`
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
	ID          string `json:"id"`
	PackID      string `json:"packId"`
	Source      string `json:"source"`
	ProjectID   string `json:"projectId,omitempty"`
	VersionID   string `json:"versionId,omitempty"`
	DisplayName string `json:"displayName"`
	FileName    string `json:"fileName"`
	SHA1        string `json:"sha1,omitempty"`
	Status      string `json:"status"`
	Required    bool   `json:"required"`
	AddedAt     string `json:"addedAt"`
	UpdatedAt   string `json:"updatedAt"`
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
	return Mod{ID: m.ID, PackID: m.PackID, Source: m.Source, ProjectID: m.ProjectID, VersionID: m.VersionID, DisplayName: m.DisplayName, FileName: m.FileName, SHA1: m.SHA1, Status: m.Status, Required: m.Required, AddedAt: iso(m.AddedAt), UpdatedAt: iso(m.UpdatedAt)}
}

func (a *API) AddPackMod(ctx context.Context, packID string, in AddModInput, requestID string) (Mod, error) {
	if err := a.ready(); err != nil {
		return Mod{}, err
	}
	if strings.TrimSpace(in.Provider) == "" || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.VersionID) == "" {
		return Mod{}, ErrInvalidArgument
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil {
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
	m := store.PackModRecord{ID: id, PackID: packID, Source: string(ad.Name()), ProjectID: in.ProjectID, VersionID: in.VersionID, DisplayName: meta.Project.Name, FileName: dl.FileName, SHA1: strings.ToLower(dl.SHA1), Status: "installed", Required: in.Required, AddedAt: now, UpdatedAt: now}
	err = a.repo.WithTx(ctx, func(tx *store.Repository) error {
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: m.SHA1, SHA256: dl.SHA256, FilePath: "jar://" + m.SHA1, SizeBytes: dl.Size, ParsedAt: now}); err != nil {
			return err
		}
		if err := tx.AddPackMod(ctx, m); err != nil {
			return err
		}
		if err := tx.AddActivity(ctx, store.ActivityRecord{ID: newID("activity"), PackID: packID, Kind: "mod", Action: "add-mod", Text: "Added " + m.DisplayName, At: now}, map[string]any{"mod_id": id}, requestID); err != nil {
			return err
		}
		return tx.AddOutbox(ctx, newID("outbox"), packID, "pack_mod", id, "mod.added", map[string]any{"mod_id": id}, now)
	})
	if err != nil {
		return Mod{}, err
	}
	return modDTO(m), nil
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
		if err := tx.UpsertJarIndex(ctx, store.JarIndexRecord{SHA1: m.SHA1, SHA256: in.SHA256, FilePath: "jar://" + m.SHA1, SizeBytes: in.Size, ParsedAt: now}); err != nil {
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
	raw, _ := json.Marshal(snap)
	sum := sha256.Sum256(raw)
	lock := store.LockRecord{ID: lockID, PackID: packID, SchemaVersion: 1, SnapshotJSON: string(raw), SnapshotSHA256: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UnixMilli()}
	if err := a.repo.CreateLock(ctx, lock, deps, confs); err != nil {
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
