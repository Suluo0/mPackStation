-- Canonical mPackStation v7 database baseline.
-- All timestamps are unix milliseconds.  Large JSON values are validated here
-- for syntax; domain-specific schemas and size limits live in service code.

CREATE TABLE IF NOT EXISTS packs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  mc_version TEXT NOT NULL DEFAULT '',
  loader TEXT NOT NULL DEFAULT '' CHECK (loader IN ('','forge','neoforge','fabric','quilt')),
  loader_version TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  icon_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  archived_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_edited_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS pack_locks (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  snapshot_schema_version INTEGER NOT NULL CHECK (snapshot_schema_version > 0),
  snapshot_json TEXT NOT NULL CHECK (json_valid(snapshot_json)),
  snapshot_sha256 TEXT NOT NULL CHECK (length(snapshot_sha256) = 64),
  created_at INTEGER NOT NULL,
  UNIQUE (pack_id, id)
);

CREATE TABLE IF NOT EXISTS pack_versions (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  version TEXT NOT NULL CHECK (length(version) BETWEEN 1 AND 128),
  channel TEXT NOT NULL DEFAULT 'draft' CHECK (channel IN ('draft','release')),
  changelog TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','imported','build')),
  lock_id TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (pack_id, id),
  UNIQUE (pack_id, version),
  FOREIGN KEY (pack_id, lock_id) REFERENCES pack_locks(pack_id, id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS pack_current_version (
  pack_id TEXT PRIMARY KEY REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY (pack_id, pack_version_id) REFERENCES pack_versions(pack_id, id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS jar_index (
  sha1 TEXT PRIMARY KEY CHECK (length(sha1) = 40),
  sha256 TEXT CHECK (sha256 IS NULL OR length(sha256) = 64),
  file_path TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  mod_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(mod_ids)),
  loaders TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(loaders)),
  mc_versions TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(mc_versions)),
  raw_meta_path TEXT NOT NULL DEFAULT '',
  parsed_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS pack_mods (
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
);
CREATE INDEX IF NOT EXISTS idx_pack_mods_pack ON pack_mods(pack_id, status);
CREATE INDEX IF NOT EXISTS idx_pack_mods_sha1 ON pack_mods(sha1);
CREATE UNIQUE INDEX IF NOT EXISTS uq_pack_mods_remote_project
  ON pack_mods(pack_id, source, project_id)
  WHERE source IN ('curseforge','modrinth') AND project_id IS NOT NULL AND status <> 'removed';
CREATE UNIQUE INDEX IF NOT EXISTS uq_pack_mods_local_sha1
  ON pack_mods(pack_id, sha1)
  WHERE source = 'local' AND sha1 IS NOT NULL AND status <> 'removed';

CREATE TABLE IF NOT EXISTS conflicts (
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
);
CREATE INDEX IF NOT EXISTS idx_conflicts_pack ON conflicts(pack_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS uq_conflicts_fingerprint
  ON conflicts(pack_id, fingerprint) WHERE fingerprint <> '';

CREATE TABLE IF NOT EXISTS mod_dependencies (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  lock_id TEXT,
  from_pack_mod_id TEXT NOT NULL,
  to_project_id TEXT,
  to_version_id TEXT,
  type TEXT NOT NULL CHECK (type IN ('required','optional','incompatible','embedded')),
  constraint_text TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  FOREIGN KEY (pack_id, lock_id) REFERENCES pack_locks(pack_id, id) ON DELETE CASCADE,
  FOREIGN KEY (pack_id, from_pack_mod_id) REFERENCES pack_mods(pack_id, id) ON DELETE CASCADE,
  UNIQUE (pack_id, id)
);
CREATE INDEX IF NOT EXISTS idx_mod_dependencies_pack ON mod_dependencies(pack_id, lock_id);

CREATE TABLE IF NOT EXISTS pack_alerts (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('crash','update')),
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved','ignored')),
  source_ref TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (pack_id, kind, source_ref)
);
CREATE INDEX IF NOT EXISTS idx_pack_alerts_pack ON pack_alerts(pack_id, status);

CREATE TABLE IF NOT EXISTS pack_mod_updates (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_mod_id TEXT NOT NULL,
  candidate_version_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','ignored')),
  checked_at INTEGER NOT NULL,
  UNIQUE (pack_id, pack_mod_id),
  FOREIGN KEY (pack_id, pack_mod_id) REFERENCES pack_mods(pack_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS content_documents (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('recipe','structure','ore')),
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  active_revision_id TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (pack_id, kind, slug),
  UNIQUE (pack_id, id)
);

CREATE TABLE IF NOT EXISTS content_revisions (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL REFERENCES content_documents(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CHECK (revision > 0),
  state TEXT NOT NULL CHECK (state IN ('draft','applied','archived')),
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  source_revision_id TEXT,
  created_at INTEGER NOT NULL,
  UNIQUE (document_id, revision),
  UNIQUE (document_id, id),
  FOREIGN KEY (document_id, source_revision_id) REFERENCES content_revisions(document_id, id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_content_applied ON content_revisions(document_id) WHERE state = 'applied';

CREATE TABLE IF NOT EXISTS content_validation_runs (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES content_revisions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('passed','warning','failed')),
  issues TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(issues)),
  affected_mods TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(affected_mods)),
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_content_validation_revision ON content_validation_runs(revision_id, created_at DESC);

CREATE TABLE IF NOT EXISTS quest_books (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL UNIQUE REFERENCES packs(id) ON DELETE CASCADE,
  active_revision_id TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS quest_revisions (
  id TEXT PRIMARY KEY,
  quest_book_id TEXT NOT NULL REFERENCES quest_books(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CHECK (revision > 0),
  state TEXT NOT NULL CHECK (state IN ('draft','applied','archived')),
  created_at INTEGER NOT NULL,
  UNIQUE (quest_book_id, revision),
  UNIQUE (quest_book_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_quest_applied ON quest_revisions(quest_book_id) WHERE state = 'applied';

CREATE TABLE IF NOT EXISTS quest_chapters (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES quest_revisions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  cover_color TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL CHECK (position >= 0),
  UNIQUE (revision_id, id),
  UNIQUE (revision_id, position)
);

CREATE TABLE IF NOT EXISTS quest_nodes (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES quest_revisions(id) ON DELETE CASCADE,
  chapter_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  icon TEXT NOT NULL DEFAULT '',
  x REAL NOT NULL DEFAULT 0,
  y REAL NOT NULL DEFAULT 0,
  prerequisites TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(prerequisites)),
  rewards TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(rewards)),
  mod_refs TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(mod_refs)),
  position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  UNIQUE (revision_id, id),
  FOREIGN KEY (revision_id, chapter_id) REFERENCES quest_chapters(revision_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_quest_node_position ON quest_nodes(revision_id, chapter_id, position);

CREATE TABLE IF NOT EXISTS quest_edges (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES quest_revisions(id) ON DELETE CASCADE,
  from_node_id TEXT NOT NULL,
  to_node_id TEXT NOT NULL,
  CHECK (from_node_id <> to_node_id),
  UNIQUE (revision_id, from_node_id, to_node_id),
  FOREIGN KEY (revision_id, from_node_id) REFERENCES quest_nodes(revision_id, id) ON DELETE CASCADE,
  FOREIGN KEY (revision_id, to_node_id) REFERENCES quest_nodes(revision_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tasks (
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
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_pack ON tasks(pack_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_tasks_active_idempotency
  ON tasks(pack_id, kind, idempotency_key)
  WHERE idempotency_key IS NOT NULL AND status IN ('queued','leased','running','paused');

CREATE TABLE IF NOT EXISTS task_events (
  id TEXT PRIMARY KEY,
  task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  sequence INTEGER NOT NULL CHECK (sequence > 0),
  status TEXT NOT NULL CHECK (status IN ('queued','leased','running','paused','succeeded','failed','canceled')),
  message TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  created_at INTEGER NOT NULL,
  UNIQUE (task_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_task_events_task ON task_events(task_id, sequence);

CREATE TABLE IF NOT EXISTS task_idem_keys (
  idempotency_key TEXT PRIMARY KEY CHECK (length(idempotency_key) BETWEEN 1 AND 256),
  endpoint TEXT NOT NULL CHECK (length(endpoint) BETWEEN 1 AND 128),
  kind TEXT NOT NULL,
  payload_hash TEXT NOT NULL CHECK (length(payload_hash) = 64),
  task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS outbox_events (
  id TEXT PRIMARY KEY,
  pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at INTEGER NOT NULL,
  last_attempt_at INTEGER,
  last_error_code TEXT NOT NULL DEFAULT '',
  delivered_at INTEGER,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_events(delivered_at, next_attempt_at);

CREATE TABLE IF NOT EXISTS activities (
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
);
CREATE INDEX IF NOT EXISTS idx_activities_time ON activities(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activities_pack ON activities(pack_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
  principal_kind TEXT NOT NULL DEFAULT 'local',
  principal_id TEXT NOT NULL DEFAULT 'local',
  action TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  request_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_events(created_at DESC);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS secrets (
  key TEXT PRIMARY KEY,
  ciphertext TEXT NOT NULL,
  key_version INTEGER NOT NULL CHECK (key_version > 0),
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS onboarding_state (
  step TEXT PRIMARY KEY CHECK (step IN ('curseforgeKey','firstPack','firstMod')),
  acknowledged INTEGER NOT NULL DEFAULT 0 CHECK (acknowledged IN (0,1)),
  acknowledged_at INTEGER,
  updated_at INTEGER NOT NULL,
  CHECK ((acknowledged = 1 AND acknowledged_at IS NOT NULL) OR (acknowledged = 0 AND acknowledged_at IS NULL))
);
INSERT OR IGNORE INTO onboarding_state(step, acknowledged, acknowledged_at, updated_at)
VALUES ('curseforgeKey',0,NULL,unixepoch('now') * 1000), ('firstPack',0,NULL,unixepoch('now') * 1000), ('firstMod',0,NULL,unixepoch('now') * 1000);

CREATE TABLE IF NOT EXISTS remote_cache (
  cache_key TEXT PRIMARY KEY,
  provider TEXT NOT NULL DEFAULT '',
  payload_path TEXT NOT NULL,
  payload_sha256 TEXT CHECK (payload_sha256 IS NULL OR length(payload_sha256) = 64),
  size_bytes INTEGER NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  fetched_at INTEGER NOT NULL,
  ttl_seconds INTEGER NOT NULL DEFAULT 3600 CHECK (ttl_seconds >= 0)
);

CREATE TABLE IF NOT EXISTS blob_grace (
  sha1 TEXT PRIMARY KEY REFERENCES jar_index(sha1) ON DELETE CASCADE,
  first_unreferenced_at INTEGER NOT NULL,
  delete_after INTEGER NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS import_previews (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) = 64),
  input_hash TEXT NOT NULL CHECK (length(input_hash) = 64),
  source TEXT NOT NULL CHECK (source IN ('curseforge_url','modrinth_url','local_zip')),
  staged_path TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  consumed_at INTEGER,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_import_previews_expiry ON import_previews(expires_at, consumed_at);

CREATE TABLE IF NOT EXISTS pack_version_inputs (
  id TEXT PRIMARY KEY,
  pack_version_id TEXT NOT NULL REFERENCES pack_versions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('lock','content','quest','build_config')),
  source_id TEXT NOT NULL DEFAULT '',
  input_hash TEXT NOT NULL CHECK (length(input_hash) = 64),
  payload TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
  created_at INTEGER NOT NULL,
  UNIQUE (pack_version_id, kind, source_id)
);
CREATE INDEX IF NOT EXISTS idx_pack_version_inputs_version ON pack_version_inputs(pack_version_id);

CREATE TABLE IF NOT EXISTS delivery_checks (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT REFERENCES pack_versions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('dependency','conflict','missing_file','content','version','quest')),
  status TEXT NOT NULL CHECK (status IN ('passed','warning','blocked')),
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  input_fingerprint TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  checked_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_delivery_check_pack ON delivery_checks(pack_id, kind) WHERE pack_version_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_delivery_check_version ON delivery_checks(pack_id, pack_version_id, kind) WHERE pack_version_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT REFERENCES pack_versions(id) ON DELETE SET NULL,
  task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  path TEXT NOT NULL,
  file_name TEXT NOT NULL,
  sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
  size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
  source_fingerprint TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('building','ready','failed','deleted')),
  kind TEXT NOT NULL CHECK (kind IN ('zip','manifest','mrpack','log')),
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_artifacts_pack ON artifacts(pack_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_artifact_fingerprint ON artifacts(pack_version_id, kind, source_fingerprint)
  WHERE pack_version_id IS NOT NULL AND source_fingerprint <> '';

CREATE TABLE IF NOT EXISTS releases (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT REFERENCES pack_versions(id) ON DELETE SET NULL,
  provider TEXT NOT NULL CHECK (provider IN ('curseforge','modrinth','local')),
  status TEXT NOT NULL CHECK (status IN ('pending','publishing','succeeded','failed','canceled')),
  remote_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 256),
  remote_state TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(remote_state)),
  artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(provider, idempotency_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_release_remote ON releases(provider, remote_id) WHERE remote_id <> '';
CREATE INDEX IF NOT EXISTS idx_releases_pack ON releases(pack_id, created_at DESC);

CREATE TABLE IF NOT EXISTS allowed_export_dirs (
  name TEXT PRIMARY KEY,
  absolute_path TEXT NOT NULL UNIQUE,
  marker_verified_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
