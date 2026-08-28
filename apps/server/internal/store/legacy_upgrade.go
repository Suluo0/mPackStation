package store

import (
	"database/sql"
	"fmt"
)

// upgradeLegacyV1 converts the prototype's eight-table schema in place. The
// operation is deliberately conservative: rows are copied into constrained
// replacement tables, unknown enum values fail the migration, and no row is
// silently discarded. New databases skip this function entirely.
func upgradeLegacyV1(tx *sql.Tx) error {
	if err := addPackColumns(tx); err != nil {
		return err
	}
	if err := rebuildLegacyPackMods(tx); err != nil {
		return err
	}
	if err := rebuildLegacyTasks(tx); err != nil {
		return err
	}
	if err := rebuildLegacyConflicts(tx); err != nil {
		return err
	}
	if err := rebuildLegacyActivities(tx); err != nil {
		return err
	}
	if err := addLegacyColumns(tx, "jar_index", map[string]string{
		"sha256": "TEXT",
	}); err != nil {
		return err
	}
	if err := addLegacyColumns(tx, "settings", map[string]string{
		"updated_at": "INTEGER NOT NULL DEFAULT 0",
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE settings SET updated_at = CASE WHEN updated_at = 0 THEN unixepoch('now') * 1000 ELSE updated_at END`); err != nil {
		return fmt.Errorf("backfill settings timestamps: %w", err)
	}
	if err := addLegacyColumns(tx, "remote_cache", map[string]string{
		"provider":       "TEXT NOT NULL DEFAULT ''",
		"payload_sha256": "TEXT",
		"size_bytes":     "INTEGER NOT NULL DEFAULT 0",
	}); err != nil {
		return err
	}
	return nil
}

func addPackColumns(tx *sql.Tx) error {
	columns, err := tableColumns(tx, "packs")
	if err != nil {
		return err
	}
	if _, ok := columns["status"]; !ok {
		if _, err := tx.Exec(`ALTER TABLE packs ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`); err != nil {
			return fmt.Errorf("add packs.status: %w", err)
		}
	}
	if _, ok := columns["archived_at"]; !ok {
		if _, err := tx.Exec(`ALTER TABLE packs ADD COLUMN archived_at INTEGER`); err != nil {
			return fmt.Errorf("add packs.archived_at: %w", err)
		}
	}
	return nil
}

func addLegacyColumns(tx *sql.Tx, table string, additions map[string]string) error {
	columns, err := tableColumns(tx, table)
	if err != nil {
		return err
	}
	for name, definition := range additions {
		if columns[name] {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE "` + table + `" ADD COLUMN "` + name + `" ` + definition); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, name, err)
		}
	}
	return nil
}

func rebuildLegacyPackMods(tx *sql.Tx) error {
	if !tableExistsTx(tx, "pack_mods") {
		return nil
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_pack_mods_pack; DROP INDEX IF EXISTS idx_pack_mods_sha1;`); err != nil {
		return fmt.Errorf("drop legacy pack_mods indexes: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE pack_mods_v7 (
        id TEXT PRIMARY KEY,
        pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
        source TEXT NOT NULL CHECK (source IN ('curseforge','modrinth','local')),
        project_id TEXT,
        version_id TEXT,
        display_name TEXT NOT NULL,
        file_name TEXT NOT NULL DEFAULT '',
        sha1 TEXT REFERENCES jar_index(sha1) ON DELETE SET NULL,
        status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','installed','disabled','removed')),
        required INTEGER NOT NULL DEFAULT 1 CHECK (required IN (0,1)),
        added_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL,
        resolved_meta_path TEXT NOT NULL DEFAULT '',
        CHECK ((source = 'local' AND project_id IS NULL) OR (source IN ('curseforge','modrinth') AND project_id IS NOT NULL)),
        UNIQUE (pack_id, id)
    )`); err != nil {
		return fmt.Errorf("create pack_mods replacement: %w", err)
	}
	// The old schema used empty strings for nullable platform fields and sha1.
	// NULLIF is the one intentional normalisation in this upgrade.
	if _, err := tx.Exec(`INSERT INTO pack_mods_v7
        (id, pack_id, source, project_id, version_id, display_name, file_name, sha1,
         status, required, added_at, updated_at, resolved_meta_path)
        SELECT id, pack_id, source, NULLIF(project_id,''), NULLIF(version_id,''), display_name,
               file_name, NULLIF(sha1,''),
               CASE status WHEN 'pending' THEN 'pending' WHEN 'installed' THEN 'installed'
                           WHEN 'removed' THEN 'removed' ELSE 'pending' END,
               CASE required WHEN 0 THEN 0 ELSE 1 END, added_at, added_at, resolved_meta_path
        FROM pack_mods`); err != nil {
		return fmt.Errorf("copy pack_mods: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE pack_mods; ALTER TABLE pack_mods_v7 RENAME TO pack_mods`); err != nil {
		return fmt.Errorf("replace pack_mods: %w", err)
	}
	return nil
}

func rebuildLegacyTasks(tx *sql.Tx) error {
	if !tableExistsTx(tx, "tasks") {
		return nil
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_tasks_status`); err != nil {
		return fmt.Errorf("drop legacy tasks index: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE tasks_v7 (
        id TEXT PRIMARY KEY,
        pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
        kind TEXT NOT NULL CHECK (kind IN ('resolve','download','index','build','publish','import','cache_gc')),
        title TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','leased','running','paused','succeeded','failed','canceled')),
        progress REAL NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
        message TEXT NOT NULL DEFAULT '',
        payload TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
        payload_path TEXT NOT NULL DEFAULT '',
        error_code TEXT NOT NULL DEFAULT '',
        error_message TEXT NOT NULL DEFAULT '',
        attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
        max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 16),
        recover_count INTEGER NOT NULL DEFAULT 0 CHECK (recover_count >= 0),
        lease_owner TEXT,
        lease_epoch INTEGER,
        lease_expires_at INTEGER,
        idempotency_key TEXT,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL,
        started_at INTEGER,
        finished_at INTEGER,
        CHECK ((status IN ('leased','running') AND lease_owner IS NOT NULL AND lease_epoch IS NOT NULL AND lease_expires_at IS NOT NULL)
            OR (status NOT IN ('leased','running') AND lease_owner IS NULL AND lease_epoch IS NULL AND lease_expires_at IS NULL))
    )`); err != nil {
		return fmt.Errorf("create tasks replacement: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO tasks_v7
        (id, pack_id, kind, title, status, progress, message, payload, payload_path,
         error_code, error_message, attempt, max_attempts, recover_count, idempotency_key,
         created_at, updated_at, started_at, finished_at)
        SELECT id, pack_id,
               CASE kind WHEN 'pack' THEN 'build' WHEN 'index' THEN 'index'
                         WHEN 'sync' THEN 'resolve' WHEN 'cache' THEN 'cache_gc' ELSE kind END,
               title,
               CASE status WHEN 'success' THEN 'succeeded' WHEN 'canceled' THEN 'canceled'
                           WHEN 'running' THEN 'paused' WHEN 'queued' THEN 'queued'
                           WHEN 'failed' THEN 'failed' ELSE status END,
               CASE WHEN progress < 0 THEN 0 WHEN progress > 100 THEN 100 ELSE progress END,
               message, '{}', payload_path, '', error, 0, 3, 0, NULL,
               created_at, COALESCE(finished_at, created_at), started_at, finished_at
        FROM tasks`); err != nil {
		return fmt.Errorf("copy tasks: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE tasks; ALTER TABLE tasks_v7 RENAME TO tasks`); err != nil {
		return fmt.Errorf("replace tasks: %w", err)
	}
	return nil
}

func rebuildLegacyConflicts(tx *sql.Tx) error {
	if !tableExistsTx(tx, "conflicts") {
		return nil
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_conflicts_pack`); err != nil {
		return fmt.Errorf("drop legacy conflicts index: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE conflicts_v7 (
        id TEXT PRIMARY KEY,
        pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
        fingerprint TEXT NOT NULL DEFAULT '',
        kind TEXT NOT NULL CHECK (kind IN ('dependency','version','loader','duplicate','crash')),
        severity TEXT NOT NULL DEFAULT 'error' CHECK (severity IN ('error','warning')),
        status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','resolved','ignored')),
        summary TEXT NOT NULL,
        detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
        detail_path TEXT NOT NULL DEFAULT '',
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL,
        resolved_at INTEGER
    )`); err != nil {
		return fmt.Errorf("create conflicts replacement: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO conflicts_v7
        (id, pack_id, fingerprint, kind, severity, status, summary, detail, detail_path,
         created_at, updated_at, resolved_at)
        SELECT id, pack_id, '', kind,
               CASE severity WHEN 'warning' THEN 'warning' ELSE 'error' END,
               CASE status WHEN 'resolved' THEN 'resolved' WHEN 'ignored' THEN 'ignored' ELSE 'pending' END,
               summary, '{}', detail_path, created_at, COALESCE(resolved_at, created_at), resolved_at
        FROM conflicts`); err != nil {
		return fmt.Errorf("copy conflicts: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE conflicts; ALTER TABLE conflicts_v7 RENAME TO conflicts`); err != nil {
		return fmt.Errorf("replace conflicts: %w", err)
	}
	return nil
}

func rebuildLegacyActivities(tx *sql.Tx) error {
	if !tableExistsTx(tx, "activities") {
		return nil
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_activities_time`); err != nil {
		return fmt.Errorf("drop legacy activities index: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE activities_v7 (
        id TEXT PRIMARY KEY,
        pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
        task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
        origin_event_id TEXT UNIQUE,
        kind TEXT NOT NULL CHECK (kind IN ('pack','mod','conflict','task','build','content','quest','system')),
        action TEXT NOT NULL DEFAULT '',
        text TEXT NOT NULL,
        detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
        request_id TEXT NOT NULL DEFAULT '',
        created_at INTEGER NOT NULL
    )`); err != nil {
		return fmt.Errorf("create activities replacement: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO activities_v7
        (id, pack_id, kind, action, text, detail, request_id, created_at)
        SELECT id, pack_id,
               CASE kind WHEN 'edit' THEN 'content' WHEN 'conflict' THEN 'conflict'
                         WHEN 'pack' THEN 'pack' WHEN 'task' THEN 'task' ELSE 'system' END,
               '', text, '{}', '', created_at
        FROM activities`); err != nil {
		return fmt.Errorf("copy activities: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE activities; ALTER TABLE activities_v7 RENAME TO activities`); err != nil {
		return fmt.Errorf("replace activities: %w", err)
	}
	return nil
}

func tableExistsTx(tx *sql.Tx, table string) bool {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
