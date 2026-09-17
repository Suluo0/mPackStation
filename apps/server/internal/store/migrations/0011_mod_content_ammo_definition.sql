-- 0011: Extend mod_content.kind CHECK to include 'ammo_definition' for matter_cannon ammo property files.

CREATE TABLE mod_content_new (
  id            TEXT PRIMARY KEY,
  pack_id       TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  mod_id        TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
  modid         TEXT NOT NULL DEFAULT '',
  version       TEXT NOT NULL DEFAULT '',
  kind          TEXT NOT NULL CHECK (kind IN ('metadata','recipe','item_model','structure','worldgen','loot_table','advancement','tag','lang','texture','ammo_definition')),
  path          TEXT NOT NULL,
  key           TEXT NOT NULL DEFAULT '',
  payload       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
  is_dynamic    INTEGER NOT NULL DEFAULT 0 CHECK (is_dynamic IN (0,1)),
  parse_error   TEXT NOT NULL DEFAULT '',
  parsed_at     INTEGER NOT NULL,
  UNIQUE(mod_id, kind, path)
);

INSERT INTO mod_content_new (id, pack_id, mod_id, modid, version, kind, path, key, payload, is_dynamic, parse_error, parsed_at)
SELECT id, pack_id, mod_id, modid, version, kind, path, key, payload, is_dynamic, parse_error, parsed_at
FROM mod_content;

DROP TABLE mod_content;
ALTER TABLE mod_content_new RENAME TO mod_content;

CREATE INDEX idx_mod_content_pack ON mod_content(pack_id);
CREATE INDEX idx_mod_content_mod ON mod_content(mod_id);
CREATE INDEX idx_mod_content_kind ON mod_content(kind);

-- Migrate existing dirty data: ae2:matter_cannon files were incorrectly classified as 'recipe'.
UPDATE mod_content SET kind = 'ammo_definition'
WHERE kind = 'recipe' AND path LIKE '%/recipe/matter_cannon/%';
