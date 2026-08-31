package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// PackVersionInputRecord is an immutable input captured for a build version.
// Payload is intentionally retained as JSON so a later rebuild can explain
// which source snapshot produced an artifact without reading the work tree.
type PackVersionInputRecord struct {
	ID, PackVersionID, Kind, SourceID, InputHash, Payload string
	CreatedAt                                             int64
}

func (r *Repository) GetLock(ctx context.Context, packID, lockID string) (LockRecord, error) {
	var x LockRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,snapshot_schema_version,snapshot_json,snapshot_sha256,created_at FROM pack_locks WHERE pack_id=? AND id=?`, packID, lockID).Scan(&x.ID, &x.PackID, &x.SchemaVersion, &x.SnapshotJSON, &x.SnapshotSHA256, &x.CreatedAt)
	if err == sql.ErrNoRows {
		return LockRecord{}, ErrNotFound
	}
	if err != nil {
		return LockRecord{}, fmt.Errorf("get lock: %w", err)
	}
	return x, nil
}

// DeliveryCheckRecord is the persisted result of one release-readiness gate.
type DeliveryCheckRecord struct {
	ID, PackID, PackVersionID, Kind, Status, Detail, InputFingerprint, RunID string
	CheckedAt                                                                int64
}

// ArtifactRecord describes a validated, registered build output.
type ArtifactRecord struct {
	ID, PackID, PackVersionID, TaskID, Path, FileName, SHA256, SourceFingerprint, Status, Kind string
	SizeBytes                                                                                  int64
	CreatedAt                                                                                  int64
}

// ExportDirRecord is a user-approved absolute directory for generated files.
type ExportDirRecord struct {
	Name, AbsolutePath string
	MarkerVerifiedAt   int64
	CreatedAt          int64
}

// ReleaseRecord is the durable state of one external or local publication.
type ReleaseRecord struct {
	ID, PackID, PackVersionID, Provider, Status, RemoteID, IdempotencyKey string
	RemoteState, ArtifactID, ErrorCode, ErrorMessage                      string
	CreatedAt, UpdatedAt                                                  int64
}

func (r *Repository) ListDeliveryChecks(ctx context.Context, packID, versionID string) ([]DeliveryCheckRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),kind,status,detail,input_fingerprint,run_id,checked_at FROM delivery_checks WHERE pack_id=? AND ((pack_version_id IS NULL AND ?='') OR pack_version_id=?) ORDER BY kind`, packID, versionID, versionID)
	if err != nil {
		return nil, fmt.Errorf("list delivery checks: %w", err)
	}
	defer rows.Close()
	var out []DeliveryCheckRecord
	for rows.Next() {
		var x DeliveryCheckRecord
		if err := rows.Scan(&x.ID, &x.PackID, &x.PackVersionID, &x.Kind, &x.Status, &x.Detail, &x.InputFingerprint, &x.RunID, &x.CheckedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) SaveDeliveryChecks(ctx context.Context, packID, versionID string, checks []DeliveryCheckRecord) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		if _, err := tx.GetPackVersion(ctx, packID, versionID); err != nil {
			return err
		}
		for _, c := range checks {
			if c.ID == "" || c.Kind == "" || c.Status == "" || !json.Valid([]byte(c.Detail)) {
				return fmt.Errorf("save delivery check: invalid check")
			}
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO delivery_checks(id,pack_id,pack_version_id,kind,status,detail,input_fingerprint,run_id,checked_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(pack_id,pack_version_id,kind) WHERE pack_version_id IS NOT NULL DO UPDATE SET status=excluded.status,detail=excluded.detail,input_fingerprint=excluded.input_fingerprint,run_id=excluded.run_id,checked_at=excluded.checked_at`, c.ID, packID, versionID, c.Kind, c.Status, c.Detail, c.InputFingerprint, c.RunID, c.CheckedAt); err != nil {
				return fmt.Errorf("save delivery check: %w", err)
			}
		}
		return nil
	})
}

func (r *Repository) HasBlockedDeliveryCheck(ctx context.Context, packID, versionID string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM delivery_checks WHERE pack_id=? AND pack_version_id=? AND status='blocked'`, packID, versionID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check blocked delivery: %w", err)
	}
	return n > 0, nil
}

func (r *Repository) ListArtifacts(ctx context.Context, packID, versionID string) ([]ArtifactRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),COALESCE(task_id,''),path,file_name,sha256,size_bytes,source_fingerprint,status,kind,created_at FROM artifacts WHERE pack_id=? AND (?='' OR pack_version_id=?) ORDER BY created_at DESC,id DESC`, packID, versionID, versionID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()
	var out []ArtifactRecord
	for rows.Next() {
		var a ArtifactRecord
		if err := rows.Scan(&a.ID, &a.PackID, &a.PackVersionID, &a.TaskID, &a.Path, &a.FileName, &a.SHA256, &a.SizeBytes, &a.SourceFingerprint, &a.Status, &a.Kind, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) ListReleases(ctx context.Context, packID, versionID string) ([]ReleaseRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),provider,status,remote_id,idempotency_key,remote_state,COALESCE(artifact_id,''),error_code,error_message,created_at,updated_at FROM releases WHERE pack_id=? AND (?='' OR pack_version_id=?) ORDER BY created_at DESC,id DESC`, packID, versionID, versionID)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer rows.Close()
	var out []ReleaseRecord
	for rows.Next() {
		var x ReleaseRecord
		if err := rows.Scan(&x.ID, &x.PackID, &x.PackVersionID, &x.Provider, &x.Status, &x.RemoteID, &x.IdempotencyKey, &x.RemoteState, &x.ArtifactID, &x.ErrorCode, &x.ErrorMessage, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetPackVersion returns a version only when it belongs to packID. Keeping
// the pair in every query prevents cross-pack resource confusion.
func (r *Repository) GetPackVersion(ctx context.Context, packID, versionID string) (PackVersionRecord, error) {
	var v PackVersionRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,version,channel,changelog,source,lock_id,created_at,updated_at FROM pack_versions WHERE pack_id=? AND id=?`, packID, versionID).Scan(
		&v.ID, &v.PackID, &v.Version, &v.Channel, &v.Changelog, &v.Source, &v.LockID, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return PackVersionRecord{}, ErrNotFound
	}
	if err != nil {
		return PackVersionRecord{}, fmt.Errorf("get pack version: %w", err)
	}
	return v, nil
}

func (r *Repository) ListPackVersions(ctx context.Context, packID string) ([]PackVersionRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,pack_id,version,channel,changelog,source,lock_id,created_at,updated_at FROM pack_versions WHERE pack_id=? ORDER BY created_at DESC,id DESC`, packID)
	if err != nil {
		return nil, fmt.Errorf("list pack versions: %w", err)
	}
	defer rows.Close()
	var out []PackVersionRecord
	for rows.Next() {
		var v PackVersionRecord
		if err := rows.Scan(&v.ID, &v.PackID, &v.Version, &v.Channel, &v.Changelog, &v.Source, &v.LockID, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) CreatePackVersion(ctx context.Context, v PackVersionRecord) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO pack_versions(id,pack_id,version,channel,changelog,source,lock_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, v.ID, v.PackID, v.Version, v.Channel, v.Changelog, v.Source, nullStringArg(v.LockID), v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create pack version: %w", err)
	}
	return nil
}

// RecordBuildInputs persists immutable provenance and the readiness checks in
// one short transaction. A version cannot silently change its build inputs.
func (r *Repository) RecordBuildInputs(ctx context.Context, packID, versionID string, inputs []PackVersionInputRecord, checks []DeliveryCheckRecord) error {
	return r.WithTx(ctx, func(tx *Repository) error {
		if _, err := tx.GetPackVersion(ctx, packID, versionID); err != nil {
			return err
		}
		for _, in := range inputs {
			if in.ID == "" || in.Kind == "" || in.InputHash == "" {
				return fmt.Errorf("record build input: invalid input")
			}
			var oldHash, oldPayload string
			err := tx.db.QueryRowContext(ctx, `SELECT input_hash,payload FROM pack_version_inputs WHERE pack_version_id=? AND kind=? AND source_id=?`, versionID, in.Kind, in.SourceID).Scan(&oldHash, &oldPayload)
			switch {
			case err == nil && (oldHash != in.InputHash || oldPayload != in.Payload):
				return fmt.Errorf("record build input: %w", ErrConflict)
			case err != nil && err != sql.ErrNoRows:
				return fmt.Errorf("read build input: %w", err)
			case err == sql.ErrNoRows:
				if _, err := tx.db.ExecContext(ctx, `INSERT INTO pack_version_inputs(id,pack_version_id,kind,source_id,input_hash,payload,created_at) VALUES(?,?,?,?,?,?,?)`, in.ID, versionID, in.Kind, in.SourceID, in.InputHash, in.Payload, in.CreatedAt); err != nil {
					return fmt.Errorf("insert build input: %w", err)
				}
			}
		}
		for _, check := range checks {
			if check.ID == "" || check.Kind == "" || check.Status == "" {
				return fmt.Errorf("record delivery check: invalid check")
			}
			if check.Detail == "" {
				check.Detail = `{}`
			}
			if !json.Valid([]byte(check.Detail)) {
				return fmt.Errorf("record delivery check: invalid detail")
			}
			if _, err := tx.db.ExecContext(ctx, `INSERT INTO delivery_checks(id,pack_id,pack_version_id,kind,status,detail,input_fingerprint,run_id,checked_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(pack_id,pack_version_id,kind) WHERE pack_version_id IS NOT NULL DO UPDATE SET status=excluded.status,detail=excluded.detail,input_fingerprint=excluded.input_fingerprint,run_id=excluded.run_id,checked_at=excluded.checked_at`, check.ID, packID, versionID, check.Kind, check.Status, check.Detail, check.InputFingerprint, check.RunID, check.CheckedAt); err != nil {
				return fmt.Errorf("insert delivery check: %w", err)
			}
		}
		return nil
	})
}

// GetArtifactByFingerprint finds a ready artifact generated from the exact
// source fingerprint. It is the repository side of build idempotency.
func (r *Repository) GetArtifactByFingerprint(ctx context.Context, packID, versionID, kind, fingerprint string) (ArtifactRecord, error) {
	var a ArtifactRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),COALESCE(task_id,''),path,file_name,sha256,size_bytes,source_fingerprint,status,kind,created_at FROM artifacts WHERE pack_id=? AND pack_version_id=? AND kind=? AND source_fingerprint=?`, packID, versionID, kind, fingerprint).Scan(
		&a.ID, &a.PackID, &a.PackVersionID, &a.TaskID, &a.Path, &a.FileName, &a.SHA256, &a.SizeBytes, &a.SourceFingerprint, &a.Status, &a.Kind, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return ArtifactRecord{}, ErrNotFound
	}
	if err != nil {
		return ArtifactRecord{}, fmt.Errorf("get artifact: %w", err)
	}
	return a, nil
}

// GetArtifact returns an artifact only by its stable ID. Callers must still
// verify its pack/version ownership before using it for a release.
func (r *Repository) GetArtifact(ctx context.Context, id string) (ArtifactRecord, error) {
	var a ArtifactRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),COALESCE(task_id,''),path,file_name,sha256,size_bytes,source_fingerprint,status,kind,created_at FROM artifacts WHERE id=?`, id).Scan(
		&a.ID, &a.PackID, &a.PackVersionID, &a.TaskID, &a.Path, &a.FileName, &a.SHA256, &a.SizeBytes, &a.SourceFingerprint, &a.Status, &a.Kind, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return ArtifactRecord{}, ErrNotFound
	}
	if err != nil {
		return ArtifactRecord{}, fmt.Errorf("get artifact: %w", err)
	}
	return a, nil
}

// RegisterArtifact records an output only after the caller has atomically
// renamed the fully validated temporary file. Repeated registration returns
// the existing row and never creates a second logical artifact.
func (r *Repository) RegisterArtifact(ctx context.Context, a ArtifactRecord) (ArtifactRecord, error) {
	if a.Status == "" {
		a.Status = "ready"
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO artifacts(id,pack_id,pack_version_id,task_id,path,file_name,sha256,size_bytes,source_fingerprint,status,kind,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(pack_version_id,kind,source_fingerprint) WHERE pack_version_id IS NOT NULL AND source_fingerprint <> '' DO NOTHING`, a.ID, a.PackID, nullIfEmpty(a.PackVersionID), nullIfEmpty(a.TaskID), a.Path, a.FileName, a.SHA256, a.SizeBytes, a.SourceFingerprint, a.Status, a.Kind, a.CreatedAt)
	if err != nil {
		return ArtifactRecord{}, fmt.Errorf("register artifact: %w", err)
	}
	return r.GetArtifactByFingerprint(ctx, a.PackID, a.PackVersionID, a.Kind, a.SourceFingerprint)
}

// RegisterExportDir approves a directory by name, preserving the first path
// binding. A path change requires an explicit user-facing operation later.
func (r *Repository) RegisterExportDir(ctx context.Context, dir ExportDirRecord) error {
	var old string
	err := r.db.QueryRowContext(ctx, `SELECT absolute_path FROM allowed_export_dirs WHERE name=?`, dir.Name).Scan(&old)
	if err == nil {
		if old != dir.AbsolutePath {
			return fmt.Errorf("register export directory: %w", ErrConflict)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read export directory: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO allowed_export_dirs(name,absolute_path,marker_verified_at,created_at) VALUES(?,?,?,?)`, dir.Name, dir.AbsolutePath, dir.MarkerVerifiedAt, dir.CreatedAt); err != nil {
		return fmt.Errorf("register export directory: %w", err)
	}
	return nil
}

// GetExportDir returns a named approved export directory.
func (r *Repository) GetExportDir(ctx context.Context, name string) (ExportDirRecord, error) {
	var d ExportDirRecord
	err := r.db.QueryRowContext(ctx, `SELECT name,absolute_path,marker_verified_at,created_at FROM allowed_export_dirs WHERE name=?`, name).Scan(&d.Name, &d.AbsolutePath, &d.MarkerVerifiedAt, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return ExportDirRecord{}, ErrNotFound
	}
	if err != nil {
		return ExportDirRecord{}, fmt.Errorf("get export directory: %w", err)
	}
	return d, nil
}

// CreateRelease creates or returns the durable idempotency record.
func (r *Repository) CreateRelease(ctx context.Context, rel ReleaseRecord) (ReleaseRecord, error) {
	if rel.RemoteState == "" {
		rel.RemoteState = `{}`
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO releases(id,pack_id,pack_version_id,provider,status,remote_id,idempotency_key,remote_state,artifact_id,error_code,error_message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider,idempotency_key) DO NOTHING`, rel.ID, rel.PackID, nullIfEmpty(rel.PackVersionID), rel.Provider, rel.Status, rel.RemoteID, rel.IdempotencyKey, rel.RemoteState, nullIfEmpty(rel.ArtifactID), rel.ErrorCode, rel.ErrorMessage, rel.CreatedAt, rel.UpdatedAt); err != nil {
		return ReleaseRecord{}, fmt.Errorf("create release: %w", err)
	}
	return r.GetReleaseByKey(ctx, rel.Provider, rel.IdempotencyKey)
}

// GetReleaseByKey returns the unique publication record for a provider key.
func (r *Repository) GetReleaseByKey(ctx context.Context, providerName, key string) (ReleaseRecord, error) {
	var rel ReleaseRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),provider,status,remote_id,idempotency_key,remote_state,COALESCE(artifact_id,''),error_code,error_message,created_at,updated_at FROM releases WHERE provider=? AND idempotency_key=?`, providerName, key).Scan(
		&rel.ID, &rel.PackID, &rel.PackVersionID, &rel.Provider, &rel.Status, &rel.RemoteID, &rel.IdempotencyKey, &rel.RemoteState, &rel.ArtifactID, &rel.ErrorCode, &rel.ErrorMessage, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return ReleaseRecord{}, ErrNotFound
	}
	if err != nil {
		return ReleaseRecord{}, fmt.Errorf("get release: %w", err)
	}
	return rel, nil
}

// SetReleasePublishing reserves a release for one explicit publish attempt.
func (r *Repository) SetReleasePublishing(ctx context.Context, id string, at int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE releases SET status='publishing',error_code='',error_message='',updated_at=? WHERE id=? AND status IN ('pending','failed')`, at, id)
	if err != nil {
		return fmt.Errorf("start release: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// FinishRelease stores remote state without ever retrying a non-idempotent
// provider call. Recovery is an explicit subsequent request.
func (r *Repository) FinishRelease(ctx context.Context, id, status, remoteID, remoteState, errorCode, errorMessage string, at int64) error {
	if remoteState == "" {
		remoteState = `{}`
	}
	if !json.Valid([]byte(remoteState)) {
		return fmt.Errorf("finish release: invalid remote state")
	}
	res, err := r.db.ExecContext(ctx, `UPDATE releases SET status=?,remote_id=?,remote_state=?,error_code=?,error_message=?,updated_at=? WHERE id=? AND status='publishing'`, status, remoteID, remoteState, errorCode, errorMessage, at, id)
	if err != nil {
		return fmt.Errorf("finish release: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// GetRelease returns a release by its stable ID.
func (r *Repository) GetRelease(ctx context.Context, id string) (ReleaseRecord, error) {
	var rel ReleaseRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,pack_id,COALESCE(pack_version_id,''),provider,status,remote_id,idempotency_key,remote_state,COALESCE(artifact_id,''),error_code,error_message,created_at,updated_at FROM releases WHERE id=?`, id).Scan(
		&rel.ID, &rel.PackID, &rel.PackVersionID, &rel.Provider, &rel.Status, &rel.RemoteID, &rel.IdempotencyKey, &rel.RemoteState, &rel.ArtifactID, &rel.ErrorCode, &rel.ErrorMessage, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return ReleaseRecord{}, ErrNotFound
	}
	if err != nil {
		return ReleaseRecord{}, fmt.Errorf("get release: %w", err)
	}
	return rel, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
