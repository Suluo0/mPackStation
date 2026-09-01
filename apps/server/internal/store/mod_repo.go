package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// PackModRecord is the transport-neutral representation of a selected mod.
// One row is one logical mod: Source/ProjectID/VersionID are the primary
// (user-picked) platform pin; Mirror* are the pinned counterpart on the other
// platform, resolved once at add time and never auto-updated.
type PackModRecord struct {
	ID, PackID, Source, ProjectID, VersionID, DisplayName, FileName, SHA1, Status string
	MirrorSource, MirrorProjectID, MirrorVersionID                                 string
	Required                                             bool
	AddedAt, UpdatedAt                                   int64
	// Origin: manual = 用户手动添加; compat-fix = 兼容知识库自动加装的补丁。
	Origin string
}
type JarIndexRecord struct {
	SHA1, SHA256, FilePath, RawMetaPath string
	SizeBytes                           int64
	ModIDs, Loaders, MCVersions         []string
	ParsedAt                            int64
}
type ModDependencyRecord struct {
	ID, PackID, LockID, FromPackModID, ToProjectID, ToVersionID, Type, Constraint, Reason string
	CreatedAt                                                                             int64
}
type ConflictRecord struct {
	ID, PackID, Fingerprint, Kind, Severity, Status, Summary, DetailPath string
	Detail                                                               map[string]any
	CreatedAt, UpdatedAt                                                 int64
	ResolvedAt                                                           sql.NullInt64
}
type LockRecord struct {
	ID, PackID                   string
	SchemaVersion                int
	SnapshotJSON, SnapshotSHA256 string
	CreatedAt                    int64
}

func (r *Repository) ListPackMods(ctx context.Context, packID string) ([]PackModRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,source,COALESCE(project_id,''),COALESCE(version_id,''),display_name,file_name,COALESCE(sha1,''),status,required,added_at,updated_at,mirror_source,COALESCE(mirror_project_id,''),COALESCE(mirror_version_id,''),origin FROM pack_mods WHERE pack_id=? AND status<>'removed' ORDER BY display_name COLLATE NOCASE,id`, packID)
	if err != nil {
		return nil, fmt.Errorf("list pack mods: %w", err)
	}
	defer rows.Close()
	var out []PackModRecord
	for rows.Next() {
		var m PackModRecord
		var req int
		if err := rows.Scan(&m.ID, &m.PackID, &m.Source, &m.ProjectID, &m.VersionID, &m.DisplayName, &m.FileName, &m.SHA1, &m.Status, &req, &m.AddedAt, &m.UpdatedAt, &m.MirrorSource, &m.MirrorProjectID, &m.MirrorVersionID, &m.Origin); err != nil {
			return nil, err
		}
		m.Required = req != 0
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *Repository) AddPackMod(ctx context.Context, m PackModRecord) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO pack_mods(id,pack_id,source,project_id,version_id,display_name,file_name,sha1,status,required,added_at,updated_at,mirror_source,mirror_project_id,mirror_version_id,origin) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.ID, m.PackID, m.Source, nullString(m.ProjectID), nullString(m.VersionID), m.DisplayName, m.FileName, nullString(m.SHA1), m.Status, boolInt(m.Required), m.AddedAt, m.UpdatedAt, m.MirrorSource, nullString(m.MirrorProjectID), nullString(m.MirrorVersionID), m.Origin)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("%w: mod already selected", ErrConflict)
		}
		return fmt.Errorf("add pack mod: %w", err)
	}
	return nil
}
// ModIdentityRecord is one confirmed cross-platform pairing: the same mod as
// Modrinth project + CurseForge project. The user db holds pairings confirmed
// on this machine; a read-only baseline ships with the app (knowledge pack).
type ModIdentityRecord struct {
	MRProjectID, CFProjectID, DisplayName string
	ConfirmedAt                           int64
}

// UpsertModIdentity records or refreshes a confirmed pairing.
func (r *Repository) UpsertModIdentity(ctx context.Context, m ModIdentityRecord) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO mod_identity(mr_project_id,cf_project_id,display_name,confirmed_at) VALUES(?,?,?,?)
		ON CONFLICT(mr_project_id,cf_project_id) DO UPDATE SET display_name=excluded.display_name,confirmed_at=excluded.confirmed_at`, m.MRProjectID, m.CFProjectID, m.DisplayName, m.ConfirmedAt)
	return err
}

// ListModIdentities returns every pairing confirmed on this machine.
func (r *Repository) ListModIdentities(ctx context.Context) ([]ModIdentityRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT mr_project_id,cf_project_id,display_name,confirmed_at FROM mod_identity`)
	if err != nil {
		return nil, fmt.Errorf("list mod identities: %w", err)
	}
	defer rows.Close()
	var out []ModIdentityRecord
	for rows.Next() {
		var m ModIdentityRecord
		if err := rows.Scan(&m.MRProjectID, &m.CFProjectID, &m.DisplayName, &m.ConfirmedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) UpdatePackMod(ctx context.Context, m PackModRecord) error {
	res, err := r.db.ExecContext(ctx, `UPDATE pack_mods SET version_id=?,display_name=?,file_name=?,sha1=?,status=?,required=?,updated_at=?,mirror_source=?,mirror_project_id=?,mirror_version_id=? WHERE pack_id=? AND id=? AND status<>'removed'`, nullString(m.VersionID), m.DisplayName, m.FileName, nullString(m.SHA1), m.Status, boolInt(m.Required), m.UpdatedAt, m.MirrorSource, nullString(m.MirrorProjectID), nullString(m.MirrorVersionID), m.PackID, m.ID)
	if err != nil {
		return fmt.Errorf("update pack mod: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *Repository) RemovePackMod(ctx context.Context, packID, modID string, at int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE pack_mods SET status='removed',updated_at=? WHERE pack_id=? AND id=? AND status<>'removed'`, at, packID, modID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (r *Repository) UpsertJarIndex(ctx context.Context, j JarIndexRecord) error {
	var oldMods, oldLoaders, oldGames string
	if err := r.db.QueryRowContext(ctx, `SELECT mod_ids,loaders,mc_versions FROM jar_index WHERE sha1=?`, j.SHA1).Scan(&oldMods, &oldLoaders, &oldGames); err == nil {
		var merge func(string, []string) []string
		merge = func(raw string, add []string) []string {
			var all []string
			_ = json.Unmarshal([]byte(raw), &all)
			seen := map[string]bool{}
			out := make([]string, 0, len(all)+len(add))
			for _, v := range all {
				if v != "" && !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
			for _, v := range add {
				if v != "" && !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
			return out
		}
		j.ModIDs = merge(oldMods, j.ModIDs)
		j.Loaders = merge(oldLoaders, j.Loaders)
		j.MCVersions = merge(oldGames, j.MCVersions)
	}
	mods, _ := json.Marshal(j.ModIDs)
	loaders, _ := json.Marshal(j.Loaders)
	games, _ := json.Marshal(j.MCVersions)
	_, err := r.db.ExecContext(ctx, `INSERT INTO jar_index(sha1,sha256,file_path,size_bytes,mod_ids,loaders,mc_versions,raw_meta_path,parsed_at) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(sha1) DO UPDATE SET sha256=excluded.sha256,file_path=excluded.file_path,size_bytes=excluded.size_bytes,mod_ids=excluded.mod_ids,loaders=excluded.loaders,mc_versions=excluded.mc_versions,raw_meta_path=excluded.raw_meta_path,parsed_at=excluded.parsed_at`, j.SHA1, nullString(j.SHA256), j.FilePath, j.SizeBytes, string(mods), string(loaders), string(games), j.RawMetaPath, j.ParsedAt)
	return err
}

func (r *Repository) CreateLock(ctx context.Context, lock LockRecord, deps []ModDependencyRecord, conflicts []ConflictRecord, requestID string) error {
	if lock.SnapshotSHA256 == "" {
		sum := sha256.Sum256([]byte(lock.SnapshotJSON))
		lock.SnapshotSHA256 = hex.EncodeToString(sum[:])
	}
	return r.WithTx(ctx, func(tx *Repository) error {
		if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_locks(id,pack_id,snapshot_schema_version,snapshot_json,snapshot_sha256,created_at) VALUES (?,?,?,?,?,?)`, lock.ID, lock.PackID, lock.SchemaVersion, lock.SnapshotJSON, lock.SnapshotSHA256, lock.CreatedAt); err != nil {
			return fmt.Errorf("create lock: %w", err)
		}
		if err := tx.SetCurrentLock(ctx, lock.PackID, lock.ID); err != nil {
			return err
		}
		for _, d := range deps {
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO mod_dependencies(id,pack_id,lock_id,from_pack_mod_id,to_project_id,to_version_id,type,constraint_text,reason,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, d.ID, lock.PackID, lock.ID, d.FromPackModID, nullString(d.ToProjectID), nullString(d.ToVersionID), d.Type, d.Constraint, d.Reason, d.CreatedAt); err != nil {
				return fmt.Errorf("create dependency: %w", err)
			}
		}
		for _, c := range conflicts {
			detail, _ := json.Marshal(c.Detail)
			if c.Fingerprint == "" {
				c.Fingerprint = c.ID
			}
			_, err := tx.db.ExecContext(ctx, `INSERT INTO conflicts(id,pack_id,fingerprint,kind,severity,status,summary,detail,detail_path,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(pack_id,fingerprint) WHERE fingerprint <> '' DO UPDATE SET kind=excluded.kind,severity=excluded.severity,summary=excluded.summary,detail=excluded.detail,detail_path=excluded.detail_path,updated_at=excluded.updated_at,status=CASE WHEN conflicts.status='ignored' THEN 'ignored' ELSE 'pending' END,resolved_at=NULL`, c.ID, lock.PackID, c.Fingerprint, c.Kind, c.Severity, "pending", c.Summary, string(detail), c.DetailPath, c.CreatedAt, c.UpdatedAt)
			if err != nil {
				return fmt.Errorf("upsert conflict: %w", err)
			}
		}
		if err := tx.AddActivity(ctx, ActivityRecord{ID: lock.ID + "-activity", PackID: lock.PackID, Kind: "mod", Action: "resolve", Text: "Resolved pack dependencies", At: lock.CreatedAt}, map[string]any{"lock_id": lock.ID}, requestID); err != nil {
			return fmt.Errorf("record lock activity: %w", err)
		}
		if err := tx.AddOutbox(ctx, lock.ID+"-outbox", lock.PackID, "pack_lock", lock.ID, "pack.lock.created", map[string]any{"lock_id": lock.ID}, lock.CreatedAt); err != nil {
			return fmt.Errorf("record lock outbox: %w", err)
		}
		return nil
	})
}

// SetCurrentLock associates the immutable lock with the pack's current version.
func (r *Repository) SetCurrentLock(ctx context.Context, packID, lockID string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE pack_versions SET lock_id=? WHERE pack_id=? AND id=(SELECT pack_version_id FROM pack_current_version WHERE pack_id=?)`, lockID, packID, packID)
	if err != nil {
		return fmt.Errorf("set current lock: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *Repository) ListLocks(ctx context.Context, packID string) ([]LockRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,snapshot_schema_version,snapshot_json,snapshot_sha256,created_at FROM pack_locks WHERE pack_id=? ORDER BY created_at DESC,id DESC`, packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LockRecord
	for rows.Next() {
		var x LockRecord
		if err := rows.Scan(&x.ID, &x.PackID, &x.SchemaVersion, &x.SnapshotJSON, &x.SnapshotSHA256, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) ListConflicts(ctx context.Context, packID string) ([]ConflictRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,fingerprint,kind,severity,status,summary,detail,detail_path,created_at,updated_at,resolved_at FROM conflicts WHERE pack_id=? ORDER BY CASE severity WHEN 'error' THEN 0 ELSE 1 END,created_at DESC,id`, packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConflictRecord
	for rows.Next() {
		var x ConflictRecord
		var raw string
		if err := rows.Scan(&x.ID, &x.PackID, &x.Fingerprint, &x.Kind, &x.Severity, &x.Status, &x.Summary, &raw, &x.DetailPath, &x.CreatedAt, &x.UpdatedAt, &x.ResolvedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &x.Detail)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) ResolveConflict(ctx context.Context, packID, id, status string, at int64) error {
	if status != "resolved" && status != "ignored" {
		return ErrConflict
	}
	res, err := r.db.ExecContext(ctx, `UPDATE conflicts SET status=?,resolved_at=?,updated_at=? WHERE pack_id=? AND id=?`, status, at, at, packID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *Repository) PackHealth(ctx context.Context, packID string) (pending, warnings, mods, installed int, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conflicts WHERE pack_id=? AND status='pending' AND severity='error'`, packID).Scan(&pending)
	if err != nil {
		return
	}
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conflicts WHERE pack_id=? AND status='pending' AND severity='warning'`, packID).Scan(&warnings)
	if err != nil {
		return
	}
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pack_mods WHERE pack_id=? AND status<>'removed'`, packID).Scan(&mods)
	if err != nil {
		return
	}
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pack_mods WHERE pack_id=? AND status='installed'`, packID).Scan(&installed)
	return
}
