-- 0008: mod content extraction tables + parse_mod_content task kind.
--
-- SQLite cannot alter a CHECK constraint in place, so the tasks table is
-- rebuilt with the extended kind list (same pattern as 0007).

CREATE TABLE tasks_v10 (
  id TEXT PRIMARY KEY,
  pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
  kind TEXT NOT NULL CHECK (kind IN ('resolve','download','index','build','publish','import','cache_gc','tool_install','launcher_install','launcher_launch','parse_mod_content')),
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
);

INSERT INTO tasks_v10
  (id, pack_id, kind, title, status, progress, message, payload, payload_path,
   error_code, error_message, attempt, max_attempts, recover_count,
   lease_owner, lease_epoch, lease_expires_at, idempotency_key,
   created_at, updated_at, started_at, finished_at)
SELECT id, pack_id, kind, title, status, progress, message, payload, payload_path,
       error_code, error_message, attempt, max_attempts, recover_count,
       lease_owner, lease_epoch, lease_expires_at, idempotency_key,
       created_at, updated_at, started_at, finished_at
FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_v10 RENAME TO tasks;

-- Per-content rows extracted from a mod jar. Read-only directory; user-edited
-- content lives in content_documents (separate table, separate lifecycle).
CREATE TABLE mod_content (
  id            TEXT PRIMARY KEY,
  pack_id       TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  mod_id        TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
  modid         TEXT NOT NULL DEFAULT '',
  version       TEXT NOT NULL DEFAULT '',
  kind          TEXT NOT NULL CHECK (kind IN ('metadata','recipe','item_model','structure','worldgen','loot_table','advancement','tag')),
  path          TEXT NOT NULL,
  key           TEXT NOT NULL DEFAULT '',
  payload       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
  is_dynamic    INTEGER NOT NULL DEFAULT 0 CHECK (is_dynamic IN (0,1)),
  parse_error   TEXT NOT NULL DEFAULT '',
  parsed_at     INTEGER NOT NULL,
  UNIQUE(mod_id, kind, path)
);
CREATE INDEX idx_mod_content_pack ON mod_content(pack_id);
CREATE INDEX idx_mod_content_mod ON mod_content(mod_id);
CREATE INDEX idx_mod_content_kind ON mod_content(kind);

-- One run summary per (mod, sha1). Succeeded runs make repeat triggers idempotent.
CREATE TABLE mod_content_runs (
  id            TEXT PRIMARY KEY,
  pack_id       TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  mod_id        TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
  sha1          TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','succeeded','failed')),
  total_files   INTEGER NOT NULL DEFAULT 0,
  parsed_count  INTEGER NOT NULL DEFAULT 0,
  dynamic_count INTEGER NOT NULL DEFAULT 0,
  error_count   INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  started_at    INTEGER NOT NULL,
  finished_at   INTEGER,
  UNIQUE(mod_id, sha1)
);
CREATE INDEX idx_mod_content_runs_mod ON mod_content_runs(mod_id);
