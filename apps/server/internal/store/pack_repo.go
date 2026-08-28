package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrNotFound indicates that a requested resource does not exist.
var ErrNotFound = errors.New("resource not found")

// ErrConflict indicates a state or uniqueness conflict.
var ErrConflict = errors.New("resource conflict")

// Repository is the only application-facing owner of business SQL.
type Repository struct {
	db dbExecutor
}

type dbExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// NewRepository builds a repository over a migrated database.
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// DatabaseDir returns the directory containing the main SQLite database. It is
// used only for environment probes; callers must not use it to bypass the
// repository or blobstore boundaries.
func (r *Repository) DatabaseDir(ctx context.Context) (string, error) {
	var path string
	if err := r.db.QueryRowContext(ctx, `SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		return "", fmt.Errorf("database path: %w", err)
	}
	if path == "" {
		return "", nil
	}
	return filepath.Dir(path), nil
}

// WithTx runs fn in a short database transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(*Repository) error) error {
	tx, err := r.db.(*sql.DB).BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(&Repository{db: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

type PackRecord struct {
	ID, Name, MCVersion, Loader, LoaderVersion, Description, IconPath string
	Status                                                            string
	CreatedAt, UpdatedAt, LastEditedAt                                int64
}

type PackVersionRecord struct {
	ID, PackID, Version, Channel, Changelog, Source string
	LockID                                          sql.NullString
	CreatedAt, UpdatedAt                            int64
}

type PackSummaryRecord struct {
	PackRecord
	Version                             string
	ModTotal, ModInstalled, ModSelected int
	ConflictsResolved, ConflictsPending int
	Recipes, Structures, Ores, Quests   int
	Crashes, Updatable                  int
}

type TaskRecord struct {
	ID, PackID, Kind, Title, Status string
	PackName                        string
	Progress                        float64
	ErrorCode, ErrorMessage         string
	CreatedAt                       int64
	StartedAt, FinishedAt           sql.NullInt64
}

type ActivityRecord struct {
	ID, PackID, TaskID, Kind, Action, Text string
	At                                     int64
}

type SystemRecord struct {
	CurseForgeKeyConfigured                bool
	ModrinthReachable, CurseForgeReachable bool
	CacheSizeBytes, StorageFreeBytes       int64
	StorageWritable                        bool
}

type OnboardingRecord struct {
	CurseForgeKey, FirstPack, FirstMod bool
}

func (r *Repository) CreatePack(ctx context.Context, p PackRecord, version PackVersionRecord) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO packs
		(id,name,mc_version,loader,loader_version,description,icon_path,status,created_at,updated_at,last_edited_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.Name, p.MCVersion, p.Loader, p.LoaderVersion, p.Description, p.IconPath,
		p.Status, p.CreatedAt, p.UpdatedAt, p.LastEditedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("%w: pack name already exists", ErrConflict)
		}
		return fmt.Errorf("insert pack: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO pack_versions
		(id,pack_id,version,channel,changelog,source,lock_id,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, version.ID, version.PackID, version.Version, version.Channel, version.Changelog,
		version.Source, nullStringArg(version.LockID), version.CreatedAt, version.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert pack version: %w", err)
	}
	if _, err = r.db.ExecContext(ctx, `INSERT INTO pack_current_version(pack_id,pack_version_id,updated_at) VALUES (?,?,?)`, p.ID, version.ID, p.UpdatedAt); err != nil {
		return fmt.Errorf("set current pack version: %w", err)
	}
	return nil
}

func nullStringArg(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}

func (r *Repository) GetPack(ctx context.Context, id string) (PackRecord, error) {
	var p PackRecord
	err := r.db.QueryRowContext(ctx, `SELECT id,name,mc_version,loader,loader_version,description,icon_path,status,
		created_at,updated_at,last_edited_at FROM packs WHERE id=?`, id).Scan(&p.ID, &p.Name, &p.MCVersion,
		&p.Loader, &p.LoaderVersion, &p.Description, &p.IconPath, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.LastEditedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PackRecord{}, ErrNotFound
	}
	if err != nil {
		return PackRecord{}, fmt.Errorf("get pack: %w", err)
	}
	return p, nil
}

func (r *Repository) ListPacks(ctx context.Context, includeArchived bool) ([]PackRecord, error) {
	query := `SELECT id,name,mc_version,loader,loader_version,description,icon_path,status,created_at,updated_at,last_edited_at FROM packs`
	if !includeArchived {
		query += ` WHERE status='active'`
	}
	query += ` ORDER BY last_edited_at DESC, id ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list packs: %w", err)
	}
	defer rows.Close()
	var result []PackRecord
	for rows.Next() {
		var p PackRecord
		if err := rows.Scan(&p.ID, &p.Name, &p.MCVersion, &p.Loader, &p.LoaderVersion, &p.Description, &p.IconPath, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.LastEditedAt); err != nil {
			return nil, fmt.Errorf("scan pack: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *Repository) DashboardPacks(ctx context.Context) ([]PackSummaryRecord, error) {
	const query = `SELECT p.id,p.name,p.mc_version,p.loader,p.loader_version,p.description,p.icon_path,p.status,p.created_at,p.updated_at,p.last_edited_at,
		COALESCE((SELECT pv.version FROM pack_current_version pcv JOIN pack_versions pv ON pv.id=pcv.pack_version_id WHERE pcv.pack_id=p.id), '0.1.0'),
		COALESCE((SELECT COUNT(*) FROM pack_mods pm WHERE pm.pack_id=p.id AND pm.status<>'removed'),0),
		COALESCE((SELECT COUNT(*) FROM pack_mods pm WHERE pm.pack_id=p.id AND pm.status='installed'),0),
		COALESCE((SELECT COUNT(*) FROM pack_mods pm WHERE pm.pack_id=p.id AND pm.status IN ('pending','installed','disabled')),0),
		COALESCE((SELECT COUNT(*) FROM conflicts c WHERE c.pack_id=p.id AND c.status='resolved'),0),
		COALESCE((SELECT COUNT(*) FROM conflicts c WHERE c.pack_id=p.id AND c.status='pending'),0),
		COALESCE((SELECT COUNT(*) FROM content_documents d WHERE d.pack_id=p.id AND d.kind='recipe'),0),
		COALESCE((SELECT COUNT(*) FROM content_documents d WHERE d.pack_id=p.id AND d.kind='structure'),0),
		COALESCE((SELECT COUNT(*) FROM content_documents d WHERE d.pack_id=p.id AND d.kind='ore'),0),
		COALESCE((SELECT COUNT(*) FROM quest_nodes qn JOIN quest_revisions qr ON qr.id=qn.revision_id JOIN quest_books qb ON qb.id=qr.quest_book_id WHERE qb.pack_id=p.id AND (qr.id=qb.active_revision_id OR (qb.active_revision_id IS NULL AND qr.state='applied'))),0),
		COALESCE((SELECT COUNT(*) FROM pack_alerts a WHERE a.pack_id=p.id AND a.kind='crash' AND a.status='open'),0),
		COALESCE((SELECT COUNT(*) FROM pack_mod_updates u WHERE u.pack_id=p.id AND u.status='pending'),0)
		FROM packs p WHERE p.status='active' ORDER BY p.last_edited_at DESC,p.id ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dashboard packs: %w", err)
	}
	defer rows.Close()
	var result []PackSummaryRecord
	for rows.Next() {
		var p PackSummaryRecord
		if err := rows.Scan(&p.ID, &p.Name, &p.MCVersion, &p.Loader, &p.LoaderVersion, &p.Description, &p.IconPath, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.LastEditedAt, &p.Version, &p.ModTotal, &p.ModInstalled, &p.ModSelected, &p.ConflictsResolved, &p.ConflictsPending, &p.Recipes, &p.Structures, &p.Ores, &p.Quests, &p.Crashes, &p.Updatable); err != nil {
			return nil, fmt.Errorf("scan dashboard pack: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *Repository) UpdatePack(ctx context.Context, p PackRecord) error {
	result, err := r.db.ExecContext(ctx, `UPDATE packs SET name=?,mc_version=?,loader=?,loader_version=?,description=?,icon_path=?,updated_at=?,last_edited_at=? WHERE id=? AND status='active'`, p.Name, p.MCVersion, p.Loader, p.LoaderVersion, p.Description, p.IconPath, p.UpdatedAt, p.LastEditedAt, p.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("%w: pack name already exists", ErrConflict)
		}
		return fmt.Errorf("update pack: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetPackStatus(ctx context.Context, id, status string, at int64) error {
	var result sql.Result
	var err error
	if status == "archived" {
		result, err = r.db.ExecContext(ctx, `UPDATE packs SET status='archived',archived_at=?,updated_at=?,last_edited_at=? WHERE id=?`, at, at, at, id)
	} else {
		result, err = r.db.ExecContext(ctx, `UPDATE packs SET status='active',archived_at=NULL,updated_at=?,last_edited_at=? WHERE id=?`, at, at, id)
	}
	if err != nil {
		return fmt.Errorf("set pack status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeletePack(ctx context.Context, id string) error {
	var active int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE pack_id=? AND status IN ('queued','leased','running','paused')`, id).Scan(&active); err != nil {
		return fmt.Errorf("check active tasks: %w", err)
	}
	if active > 0 {
		return fmt.Errorf("%w: pack has active tasks", ErrConflict)
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM packs WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete pack: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) AddActivity(ctx context.Context, a ActivityRecord, detail map[string]any, requestID string) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO activities(id,pack_id,task_id,kind,action,text,detail,request_id,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, a.ID, nullString(a.PackID), nullString(a.TaskID), a.Kind, a.Action, a.Text, string(b), requestID, a.At)
	return err
}

func (r *Repository) AddOutbox(ctx context.Context, id, packID, aggregateType, aggregateID, eventType string, payload any, at int64) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO outbox_events(id,pack_id,aggregate_type,aggregate_id,event_type,payload,next_attempt_at,created_at) VALUES (?,?,?,?,?,?,?,?)`, id, nullString(packID), aggregateType, aggregateID, eventType, string(b), at, at)
	return err
}

func (r *Repository) AddAudit(ctx context.Context, id, packID, action, requestID string, detail any, at int64) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO audit_events(id,pack_id,principal_kind,principal_id,action,detail,request_id,created_at) VALUES (?,?, 'local','local',?,?,?,?)`, id, nullString(packID), action, string(b), requestID, at)
	return err
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (r *Repository) TodayResolvedCount(ctx context.Context, startMs int64) (int, error) {
	var conflicts, validation int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conflicts WHERE status='resolved' AND resolved_at>=?`, startMs).Scan(&conflicts); err != nil {
		return 0, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_validation_runs WHERE status='passed' AND created_at>=?`, startMs).Scan(&validation); err != nil {
		return 0, err
	}
	return conflicts + validation, nil
}

func (r *Repository) ListTasks(ctx context.Context, limit int) ([]TaskRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT t.id,COALESCE(t.pack_id,''),t.kind,t.title,t.status,t.progress,t.error_code,t.error_message,t.created_at,t.started_at,t.finished_at,COALESCE(p.name,'') FROM tasks t LEFT JOIN packs p ON p.id=t.pack_id WHERE t.kind IN ('index','build','import','resolve') ORDER BY t.created_at DESC,t.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskRecord
	for rows.Next() {
		var t TaskRecord
		if err := rows.Scan(&t.ID, &t.PackID, &t.Kind, &t.Title, &t.Status, &t.Progress, &t.ErrorCode, &t.ErrorMessage, &t.CreatedAt, &t.StartedAt, &t.FinishedAt, &t.PackName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) ListActivities(ctx context.Context, limit int) ([]ActivityRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,COALESCE(pack_id,''),COALESCE(task_id,''),kind,action,text,created_at FROM activities ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActivityRecord
	for rows.Next() {
		var a ActivityRecord
		if err := rows.Scan(&a.ID, &a.PackID, &a.TaskID, &a.Kind, &a.Action, &a.Text, &a.At); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) System(ctx context.Context) (SystemRecord, error) {
	var s SystemRecord
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secrets WHERE key='curseforge_api_key' AND ciphertext<>''`).Scan(&n); err != nil {
		return s, err
	}
	s.CurseForgeKeyConfigured = n > 0
	var cf, mr string
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM settings WHERE key='provider.curseforge.reachable'),'unknown'),COALESCE((SELECT value FROM settings WHERE key='provider.modrinth.reachable'),'unknown')`).Scan(&cf, &mr); err != nil {
		return s, err
	}
	s.CurseForgeReachable = cf == "true"
	s.ModrinthReachable = mr == "true"
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes),0) FROM remote_cache`).Scan(&s.CacheSizeBytes); err != nil {
		return s, err
	}
	return s, nil
}

func (r *Repository) Onboarding(ctx context.Context) (OnboardingRecord, error) {
	var o OnboardingRecord
	var key int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secrets WHERE key='curseforge_api_key' AND ciphertext<>''`).Scan(&key); err != nil {
		return o, err
	}
	o.CurseForgeKey = key > 0
	var packs, mods int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM packs WHERE status='active'`).Scan(&packs); err != nil {
		return o, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pack_mods WHERE status<>'removed'`).Scan(&mods); err != nil {
		return o, err
	}
	o.FirstPack = packs > 0
	o.FirstMod = mods > 0
	var steps [3]int
	rows, err := r.db.QueryContext(ctx, `SELECT step,acknowledged FROM onboarding_state`)
	if err != nil {
		return o, err
	}
	defer rows.Close()
	for rows.Next() {
		var step string
		var ack int
		if err := rows.Scan(&step, &ack); err != nil {
			return o, err
		}
		switch step {
		case "curseforgeKey":
			steps[0] = ack
		case "firstPack":
			steps[1] = ack
		case "firstMod":
			steps[2] = ack
		}
	}
	o.CurseForgeKey = o.CurseForgeKey || steps[0] > 0
	o.FirstPack = o.FirstPack || steps[1] > 0
	o.FirstMod = o.FirstMod || steps[2] > 0
	return o, rows.Err()
}

func (r *Repository) AcknowledgeOnboarding(ctx context.Context, step string, at int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE onboarding_state SET acknowledged=1,acknowledged_at=?,updated_at=? WHERE step=?`, at, at, step)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ExportDir(ctx context.Context) (string, error) { return "", nil }
