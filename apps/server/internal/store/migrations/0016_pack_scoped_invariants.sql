-- 0016: close the pack-scoped identity and generation invariants introduced by
-- 0015.  The previous migration is immutable; all compatibility enforcement
-- is added with new columns, indexes, triggers and backfill evidence here.

ALTER TABLE mod_versions ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'
  CHECK(status IN ('pending','ready','failed'));
ALTER TABLE mod_versions ADD COLUMN diagnostics TEXT NOT NULL DEFAULT '{}'
  CHECK(json_valid(diagnostics));
ALTER TABLE pack_mod_selections ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'
  CHECK(status IN ('pending','ready','failed','superseded'));
ALTER TABLE pack_mod_selections ADD COLUMN diagnostics TEXT NOT NULL DEFAULT '{}'
  CHECK(json_valid(diagnostics));

CREATE INDEX IF NOT EXISTS idx_mod_versions_identity
  ON mod_versions(mod_id, declared_version, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pack_mod_selections_mod
  ON pack_mod_selections(pack_id, mod_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_parse_run_inputs_file
  ON parse_run_inputs(file_id);
CREATE INDEX IF NOT EXISTS idx_content_definitions_key_run
  ON content_definitions(pack_id, key_id, run_id);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_winner
  ON catalog_entries(pack_id, winner_definition_id);
CREATE INDEX IF NOT EXISTS idx_catalog_icons_file
  ON catalog_icons(file_id);

-- A current selection is a pointer inside the same pack and instance. SQLite
-- cannot add this composite FK to an already-created table, so triggers carry
-- the same invariant for both new writes and legacy callers.
CREATE TRIGGER IF NOT EXISTS pack_mod_current_selection_insert_guard
BEFORE INSERT ON pack_mods
WHEN NEW.current_selection_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM pack_mod_selections s
  WHERE s.pack_id=NEW.pack_id AND s.id=NEW.current_selection_id
    AND s.pack_mod_id=NEW.id AND s.mod_id=NEW.mod_id
)
BEGIN SELECT RAISE(ABORT, 'current selection does not belong to pack mod'); END;

CREATE TRIGGER IF NOT EXISTS pack_mod_current_selection_update_guard
BEFORE UPDATE OF current_selection_id,pack_id,id,mod_id ON pack_mods
WHEN NEW.current_selection_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM pack_mod_selections s
  WHERE s.pack_id=NEW.pack_id AND s.id=NEW.current_selection_id
    AND s.pack_mod_id=NEW.id AND s.mod_id=NEW.mod_id
)
BEGIN SELECT RAISE(ABORT, 'current selection does not belong to pack mod'); END;

CREATE TRIGGER IF NOT EXISTS pack_mod_identity_insert_guard
BEFORE INSERT ON pack_mods
WHEN NEW.mod_id IS NOT NULL AND NEW.mod_id<>'' AND NOT EXISTS (
  SELECT 1 FROM mods WHERE mod_id=NEW.mod_id
)
BEGIN SELECT RAISE(ABORT, 'pack mod identity is not registered'); END;

CREATE TRIGGER IF NOT EXISTS pack_mod_identity_update_guard
BEFORE UPDATE OF mod_id ON pack_mods
WHEN NEW.mod_id IS NOT NULL AND NEW.mod_id<>'' AND NOT EXISTS (
  SELECT 1 FROM mods WHERE mod_id=NEW.mod_id
)
BEGIN SELECT RAISE(ABORT, 'pack mod identity is not registered'); END;

CREATE TRIGGER IF NOT EXISTS minecraft_pack_mod_guard
BEFORE UPDATE OF mod_id,origin,status,required ON pack_mods
WHEN OLD.mod_id='minecraft' AND (
  NEW.mod_id<>'minecraft' OR NEW.origin<>'builtin' OR NEW.status='removed' OR NEW.required<>1
)
BEGIN SELECT RAISE(ABORT, 'minecraft is a required builtin pack member'); END;

CREATE TRIGGER IF NOT EXISTS minecraft_pack_mod_delete_guard
BEFORE DELETE ON pack_mods
WHEN OLD.mod_id='minecraft'
BEGIN SELECT RAISE(ABORT, 'minecraft is a required builtin pack member'); END;

CREATE TRIGGER IF NOT EXISTS pack_selection_identity_guard
BEFORE INSERT ON pack_mod_selections
WHEN NOT EXISTS (
  SELECT 1 FROM pack_mods pm
  WHERE pm.pack_id=NEW.pack_id AND pm.id=NEW.pack_mod_id AND pm.mod_id=NEW.mod_id
)
BEGIN SELECT RAISE(ABORT, 'selection identity is not the pack member identity'); END;

CREATE TRIGGER IF NOT EXISTS pack_selection_version_guard
BEFORE INSERT ON pack_mod_selections
WHEN NOT EXISTS (
  SELECT 1 FROM mod_versions v WHERE v.id=NEW.version_id AND v.mod_id=NEW.mod_id
)
BEGIN SELECT RAISE(ABORT, 'selection version identity mismatch'); END;

CREATE TRIGGER IF NOT EXISTS pack_selection_update_guard
BEFORE UPDATE OF pack_id,id,pack_mod_id,mod_id,version_id ON pack_mod_selections
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM pack_mods pm
    WHERE pm.pack_id=NEW.pack_id AND pm.id=NEW.pack_mod_id AND pm.mod_id=NEW.mod_id
  ) THEN RAISE(ABORT, 'selection identity is not the pack member identity') END;
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM mod_versions v WHERE v.id=NEW.version_id AND v.mod_id=NEW.mod_id
  ) THEN RAISE(ABORT, 'selection version identity mismatch') END;
END;

CREATE TRIGGER IF NOT EXISTS builtin_selection_acquisition_guard
BEFORE INSERT ON pack_mod_selections
WHEN NEW.mod_id='minecraft' AND NEW.acquisition<>'mojang'
BEGIN SELECT RAISE(ABORT, 'minecraft selections must use mojang acquisition'); END;

CREATE TRIGGER IF NOT EXISTS content_source_selection_guard
BEFORE INSERT ON pack_content_sources
WHEN NEW.kind='mod' AND (
  NEW.selection_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM pack_mod_selections s
    WHERE s.pack_id=NEW.pack_id AND s.id=NEW.selection_id
  )
)
BEGIN SELECT RAISE(ABORT, 'mod content source must reference a pack selection'); END;

CREATE TRIGGER IF NOT EXISTS parse_run_source_guard
BEFORE INSERT ON parse_runs
WHEN NEW.phase='content' AND (
  NEW.source_id IS NULL OR NOT EXISTS (
    SELECT 1 FROM pack_content_sources s
    WHERE s.pack_id=NEW.pack_id AND s.id=NEW.source_id
  )
)
BEGIN SELECT RAISE(ABORT, 'content parse run must reference a pack source'); END;

CREATE TRIGGER IF NOT EXISTS parse_input_source_guard
BEFORE INSERT ON parse_run_inputs
WHEN NEW.source_input_no IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM parse_runs r
  JOIN source_inputs s ON s.pack_id=r.pack_id AND s.source_id=r.source_id
    AND s.input_no=NEW.source_input_no
  WHERE r.pack_id=NEW.pack_id AND r.id=NEW.run_id
)
BEGIN SELECT RAISE(ABORT, 'parse input is not in the parse source snapshot'); END;

CREATE TRIGGER IF NOT EXISTS definition_run_source_guard
BEFORE INSERT ON content_definitions
WHEN NOT EXISTS (
  SELECT 1 FROM parse_runs r
  WHERE r.pack_id=NEW.pack_id AND r.id=NEW.run_id AND r.phase='content'
)
BEGIN SELECT RAISE(ABORT, 'content definition has no content parse run'); END;

CREATE TRIGGER IF NOT EXISTS catalog_current_generation_guard
BEFORE UPDATE OF current_generation_id ON packs
WHEN NEW.current_generation_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM catalog_generations g
  WHERE g.pack_id=NEW.id AND g.id=NEW.current_generation_id AND g.status='ready'
    AND g.config_revision=NEW.config_revision
)
BEGIN SELECT RAISE(ABORT, 'current generation is not ready for pack revision'); END;

CREATE TRIGGER IF NOT EXISTS catalog_winner_key_guard
BEFORE INSERT ON catalog_entries
WHEN NEW.winner_definition_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM content_definitions d
  WHERE d.pack_id=NEW.pack_id AND d.id=NEW.winner_definition_id
    AND d.key_id=NEW.key_id
)
BEGIN SELECT RAISE(ABORT, 'catalog winner does not match content key'); END;

CREATE TRIGGER IF NOT EXISTS catalog_selected_key_guard
BEFORE INSERT ON catalog_definition_decisions
WHEN NEW.decision='selected' AND NOT EXISTS (
  SELECT 1 FROM content_definitions d
  WHERE d.pack_id=NEW.pack_id AND d.id=NEW.definition_id AND d.key_id=NEW.key_id
)
BEGIN SELECT RAISE(ABORT, 'selected definition does not match content key'); END;

-- Backfill a pending Minecraft choice for existing packs. A pending choice has
-- no input manifest and no file object; unknown evidence is kept NULL.
INSERT OR IGNORE INTO mod_versions(id,mod_id,declared_version,input_manifest_sha256,created_at,status,diagnostics)
SELECT 'minecraft-pending-'||p.id, 'minecraft', p.mc_version,
       NULL, p.created_at, 'pending', '{"reason":"resources_not_verified"}'
FROM packs p;
INSERT OR IGNORE INTO pack_mod_selections(pack_id,id,pack_mod_id,mod_id,version_id,acquisition,requested_version,created_at,status,diagnostics)
SELECT p.id, 'minecraft-selection-'||p.id, pm.id, 'minecraft', 'minecraft-pending-'||p.id,
       'mojang', p.mc_version, p.created_at, 'pending', '{"reason":"resources_not_verified"}'
FROM packs p JOIN pack_mods pm ON pm.pack_id=p.id AND pm.mod_id='minecraft';
UPDATE pack_mods
SET current_selection_id='minecraft-selection-'||pack_id
WHERE mod_id='minecraft' AND current_selection_id IS NULL;
INSERT OR IGNORE INTO pack_content_sources(pack_id,id,kind,selection_id,external_revision,display_name,descriptor,created_at)
SELECT s.pack_id, 'minecraft-source-'||s.pack_id, 'mod', s.id, p.mc_version,
       'Minecraft', '{"builtin":true}', s.created_at
FROM pack_mod_selections s JOIN packs p ON p.id=s.pack_id
WHERE s.mod_id='minecraft';
