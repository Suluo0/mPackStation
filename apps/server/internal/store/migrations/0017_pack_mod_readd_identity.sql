-- Recreate pack_mod identity unique index so removed mods can be re-added.
-- The previous index blocked INSERT when a soft-deleted row still held (pack_id, mod_id).

DROP INDEX IF EXISTS uq_pack_mods_mod_identity;

CREATE UNIQUE INDEX uq_pack_mods_mod_identity
  ON pack_mods(pack_id, mod_id)
  WHERE mod_id IS NOT NULL AND status <> 'removed';
