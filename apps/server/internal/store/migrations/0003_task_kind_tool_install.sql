-- 0003: allow the tool_install task kind.
--
-- SQLite cannot alter a CHECK constraint in place, so the tasks table is
-- rebuilt with the extended kind list. All columns (including lease fields)
-- are copied verbatim; the shape otherwise matches 0002_v7_core.sql exactly.

CREATE TABLE tasks_v8 (
  id TEXT PRIMARY KEY,
  pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
  kind TEXT NOT NULL CHECK (kind IN ('resolve','download','index','build','publish','import','cache_gc','tool_install')),
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

INSERT INTO tasks_v8
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
ALTER TABLE tasks_v8 RENAME TO tasks;
