-- Derived catalogs are rebuilt atomically; raw mod_content remains authoritative.
CREATE TABLE pack_catalog_state (
 pack_id TEXT PRIMARY KEY REFERENCES packs(id) ON DELETE CASCADE,
 source_revision INTEGER NOT NULL DEFAULT 1 CHECK(source_revision>0),
 built_revision INTEGER NOT NULL DEFAULT 0 CHECK(built_revision>=0),
 build_status TEXT NOT NULL DEFAULT 'pending' CHECK(build_status IN ('pending','running','succeeded','failed')),
 built_at INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 warnings TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(warnings))
);
INSERT INTO pack_catalog_state(pack_id) SELECT id FROM packs;
CREATE TRIGGER catalog_new_pack AFTER INSERT ON packs BEGIN INSERT INTO pack_catalog_state(pack_id) VALUES(new.id); END;
CREATE TRIGGER catalog_pack_version AFTER UPDATE OF mc_version,loader,loader_version ON packs BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.id; END;
CREATE TABLE pack_catalog_items (
 pack_id TEXT NOT NULL REFERENCES pack_catalog_state(pack_id) ON DELETE CASCADE,
 item_id TEXT NOT NULL CHECK(instr(item_id,':')>1),
 evidence TEXT NOT NULL CHECK(evidence IN ('model','reference')),
 source TEXT NOT NULL,
 model_path TEXT NOT NULL DEFAULT '',
 icon_status TEXT NOT NULL DEFAULT 'pending' CHECK(icon_status IN ('pending','ready','missing','dynamic','failed')),
 PRIMARY KEY(pack_id,item_id)
);
CREATE TABLE pack_catalog_blocks (
 pack_id TEXT NOT NULL REFERENCES pack_catalog_state(pack_id) ON DELETE CASCADE,
 block_id TEXT NOT NULL CHECK(instr(block_id,':')>1),
 evidence TEXT NOT NULL CHECK(evidence IN ('blockstate','model','reference')),
 source TEXT NOT NULL,
 blockstate_path TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(pack_id,block_id)
);
CREATE TABLE pack_catalog_item_blocks (
 pack_id TEXT NOT NULL, item_id TEXT NOT NULL, block_id TEXT NOT NULL,
 PRIMARY KEY(pack_id,item_id,block_id),
 FOREIGN KEY(pack_id,item_id) REFERENCES pack_catalog_items(pack_id,item_id) ON DELETE CASCADE,
 FOREIGN KEY(pack_id,block_id) REFERENCES pack_catalog_blocks(pack_id,block_id) ON DELETE CASCADE
);
CREATE TABLE pack_catalog_item_names (
 pack_id TEXT NOT NULL, item_id TEXT NOT NULL, locale TEXT NOT NULL CHECK(length(locale)>0),
 name TEXT NOT NULL, translation_key TEXT NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY(pack_id,item_id,locale),
 FOREIGN KEY(pack_id,item_id) REFERENCES pack_catalog_items(pack_id,item_id) ON DELETE CASCADE
);
CREATE TABLE pack_catalog_item_icons (
 pack_id TEXT NOT NULL, item_id TEXT NOT NULL,
 mime TEXT NOT NULL CHECK(mime='image/png'), data BLOB NOT NULL CHECK(length(data)>0 AND length(data)<=1048576),
 width INTEGER NOT NULL CHECK(width>0 AND width<=256), height INTEGER NOT NULL CHECK(height>0 AND height<=256),
 source TEXT NOT NULL,
 PRIMARY KEY(pack_id,item_id),
 FOREIGN KEY(pack_id,item_id) REFERENCES pack_catalog_items(pack_id,item_id) ON DELETE CASCADE
);
CREATE TABLE pack_catalog_block_names (
 pack_id TEXT NOT NULL, block_id TEXT NOT NULL, locale TEXT NOT NULL CHECK(length(locale)>0),
 name TEXT NOT NULL, translation_key TEXT NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY(pack_id,block_id,locale),
 FOREIGN KEY(pack_id,block_id) REFERENCES pack_catalog_blocks(pack_id,block_id) ON DELETE CASCADE
);
CREATE TABLE pack_catalog_tags (
 pack_id TEXT NOT NULL REFERENCES pack_catalog_state(pack_id) ON DELETE CASCADE,
 registry TEXT NOT NULL CHECK(registry IN ('item','block')),
 tag_id TEXT NOT NULL CHECK(instr(tag_id,':')>1),
 status TEXT NOT NULL CHECK(status IN ('resolved','partial','invalid','missing')),
 diagnostics TEXT NOT NULL CHECK(json_valid(diagnostics)),
 PRIMARY KEY(pack_id,registry,tag_id)
);
CREATE TABLE pack_catalog_tag_names (
 pack_id TEXT NOT NULL, registry TEXT NOT NULL, tag_id TEXT NOT NULL, locale TEXT NOT NULL CHECK(length(locale)>0),
 name TEXT NOT NULL, translation_key TEXT NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY(pack_id,registry,tag_id,locale),
 FOREIGN KEY(pack_id,registry,tag_id) REFERENCES pack_catalog_tags(pack_id,registry,tag_id) ON DELETE CASCADE
);
-- Full original definitions preserve replace/remove/optional edges and provenance.
CREATE TABLE pack_catalog_tag_definitions (
 pack_id TEXT NOT NULL, registry TEXT NOT NULL, tag_id TEXT NOT NULL, ordinal INTEGER NOT NULL CHECK(ordinal>=0),
 source TEXT NOT NULL, path TEXT NOT NULL, payload TEXT NOT NULL CHECK(json_valid(payload)),
 replace_existing INTEGER NOT NULL DEFAULT 0 CHECK(replace_existing IN (0,1)),
 conditions TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(conditions)),
 PRIMARY KEY(pack_id,registry,tag_id,ordinal),
 FOREIGN KEY(pack_id,registry,tag_id) REFERENCES pack_catalog_tags(pack_id,registry,tag_id) ON DELETE CASCADE
);
CREATE TABLE pack_catalog_tag_entries (
 pack_id TEXT NOT NULL, registry TEXT NOT NULL, tag_id TEXT NOT NULL,
 definition_ordinal INTEGER NOT NULL, entry_ordinal INTEGER NOT NULL,
 target_kind TEXT NOT NULL CHECK(target_kind IN ('member','tag')),
 target_id TEXT NOT NULL, required INTEGER NOT NULL CHECK(required IN (0,1)),
 operation TEXT NOT NULL CHECK(operation IN ('add','remove')),
 PRIMARY KEY(pack_id,registry,tag_id,definition_ordinal,entry_ordinal),
 FOREIGN KEY(pack_id,registry,tag_id,definition_ordinal) REFERENCES pack_catalog_tag_definitions(pack_id,registry,tag_id,ordinal) ON DELETE CASCADE
);
CREATE TABLE pack_catalog_tag_members (
 pack_id TEXT NOT NULL, registry TEXT NOT NULL, tag_id TEXT NOT NULL, member_id TEXT NOT NULL,
 PRIMARY KEY(pack_id,registry,tag_id,member_id),
 FOREIGN KEY(pack_id,registry,tag_id) REFERENCES pack_catalog_tags(pack_id,registry,tag_id) ON DELETE CASCADE
);
CREATE INDEX catalog_reverse_tags ON pack_catalog_tag_members(pack_id,registry,member_id,tag_id);
CREATE INDEX catalog_name_search ON pack_catalog_item_names(pack_id,locale,name);
CREATE INDEX catalog_block_name_search ON pack_catalog_block_names(pack_id,locale,name);
CREATE TABLE pack_catalog_recipes (
 pack_id TEXT NOT NULL REFERENCES pack_catalog_state(pack_id) ON DELETE CASCADE,
 recipe_id TEXT NOT NULL CHECK(instr(recipe_id,':')>1), type TEXT NOT NULL,
 source TEXT NOT NULL, path TEXT NOT NULL, payload TEXT NOT NULL CHECK(json_valid(payload)),
 parse_status TEXT NOT NULL CHECK(parse_status IN ('parsed','partial','unsupported','invalid')),
 diagnostics TEXT NOT NULL CHECK(json_valid(diagnostics)),
 PRIMARY KEY(pack_id,recipe_id)
);
CREATE TABLE pack_catalog_recipe_refs (
 pack_id TEXT NOT NULL, recipe_id TEXT NOT NULL, role TEXT NOT NULL CHECK(role IN ('input','output')),
 slot INTEGER NOT NULL CHECK(slot>=0), alternative INTEGER NOT NULL CHECK(alternative>=0),
 ref_kind TEXT NOT NULL CHECK(ref_kind IN ('item','item_tag')), ref_id TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 1 CHECK(count>0),
 PRIMARY KEY(pack_id,recipe_id,role,slot,alternative),
 FOREIGN KEY(pack_id,recipe_id) REFERENCES pack_catalog_recipes(pack_id,recipe_id) ON DELETE CASCADE
);
CREATE TRIGGER catalog_pack_mods_INSERT AFTER INSERT ON pack_mods BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.pack_id; END;
CREATE TRIGGER catalog_pack_mods_UPDATE AFTER UPDATE ON pack_mods BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.pack_id; END;
CREATE TRIGGER catalog_pack_mods_DELETE AFTER DELETE ON pack_mods BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=old.pack_id; END;
CREATE TRIGGER catalog_mod_content_INSERT AFTER INSERT ON mod_content BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.pack_id; END;
CREATE TRIGGER catalog_mod_content_UPDATE AFTER UPDATE ON mod_content BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.pack_id; END;
CREATE TRIGGER catalog_mod_content_DELETE AFTER DELETE ON mod_content BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=old.pack_id; END;
CREATE TRIGGER catalog_mod_content_runs_INSERT AFTER INSERT ON mod_content_runs BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.pack_id; END;
CREATE TRIGGER catalog_mod_content_runs_UPDATE AFTER UPDATE ON mod_content_runs BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=new.pack_id; END;
CREATE TRIGGER catalog_mod_content_runs_DELETE AFTER DELETE ON mod_content_runs BEGIN UPDATE pack_catalog_state SET source_revision=source_revision+1,build_status='pending',last_error='' WHERE pack_id=old.pack_id; END;
