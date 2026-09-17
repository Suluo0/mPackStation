-- 0015: immutable pack-scoped mod identity, exact inputs and generation model.
-- Existing rows are retained as legacy/unresolved evidence. No applied migration
-- is edited and no missing file is marked verified by this migration.

CREATE TABLE mods (
  mod_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN ('normal','builtin'))
);
INSERT OR IGNORE INTO mods(mod_id,display_name,kind) VALUES('minecraft','Minecraft','builtin');

ALTER TABLE pack_mods ADD COLUMN mod_id TEXT;
ALTER TABLE pack_mods ADD COLUMN current_selection_id TEXT;
CREATE UNIQUE INDEX uq_pack_mods_mod_identity ON pack_mods(pack_id,mod_id) WHERE mod_id IS NOT NULL;
CREATE INDEX idx_pack_mods_status_mod ON pack_mods(pack_id,status,mod_id);

CREATE TABLE mod_versions (
  id TEXT PRIMARY KEY, mod_id TEXT NOT NULL REFERENCES mods(mod_id) ON DELETE RESTRICT,
  declared_version TEXT NOT NULL, input_manifest_sha256 TEXT CHECK(input_manifest_sha256 IS NULL OR length(input_manifest_sha256)=64),
  created_at INTEGER NOT NULL, UNIQUE(mod_id,declared_version,input_manifest_sha256), UNIQUE(mod_id,id)
);
CREATE TABLE file_objects (
  id TEXT PRIMARY KEY, sha256 TEXT NOT NULL UNIQUE CHECK(length(sha256)=64), sha1 TEXT,
  size_bytes INTEGER NOT NULL CHECK(size_bytes>=0), media_type TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL,
  CHECK(sha1 IS NULL OR length(sha1)=40)
);
CREATE TABLE file_locations (
  file_id TEXT NOT NULL REFERENCES file_objects(id) ON DELETE CASCADE, location TEXT NOT NULL,
  availability TEXT NOT NULL CHECK(availability IN ('available','missing','corrupt')),
  verified_at INTEGER, PRIMARY KEY(file_id,location)
);
CREATE TABLE version_files (
  version_id TEXT NOT NULL REFERENCES mod_versions(id) ON DELETE RESTRICT, role TEXT NOT NULL,
  logical_path TEXT NOT NULL, file_id TEXT NOT NULL REFERENCES file_objects(id) ON DELETE RESTRICT,
  required INTEGER NOT NULL CHECK(required IN(0,1)), PRIMARY KEY(version_id,role,logical_path)
);
CREATE INDEX idx_version_files_file ON version_files(file_id);
CREATE TABLE version_compatibility (
  version_id TEXT NOT NULL REFERENCES mod_versions(id) ON DELETE CASCADE, ordinal INTEGER NOT NULL CHECK(ordinal>=0),
  mc_constraint TEXT NOT NULL DEFAULT '', loader_name TEXT NOT NULL DEFAULT '', loader_constraint TEXT NOT NULL DEFAULT '',
  conditions TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(conditions)), evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(evidence)),
  PRIMARY KEY(version_id,ordinal)
);
CREATE TABLE platform_projects (
  id TEXT PRIMARY KEY, platform TEXT NOT NULL CHECK(platform IN('modrinth','curseforge')),
  external_project_id TEXT NOT NULL, slug TEXT NOT NULL DEFAULT '', display_name TEXT NOT NULL DEFAULT '',
  UNIQUE(platform,external_project_id)
);
CREATE TABLE mod_project_links (
  mod_id TEXT NOT NULL REFERENCES mods(mod_id) ON DELETE RESTRICT, project_id TEXT NOT NULL REFERENCES platform_projects(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK(status IN('candidate','confirmed','rejected')), evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(evidence)), confirmed_at INTEGER,
  PRIMARY KEY(mod_id,project_id)
);
CREATE TABLE platform_releases (
  id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES platform_projects(id) ON DELETE RESTRICT,
  external_version_id TEXT NOT NULL, version_name TEXT NOT NULL DEFAULT '', published_at INTEGER,
  UNIQUE(project_id,external_version_id)
);
CREATE TABLE platform_release_files (
  id TEXT PRIMARY KEY, release_id TEXT NOT NULL REFERENCES platform_releases(id) ON DELETE RESTRICT,
  file_key TEXT NOT NULL, file_name TEXT NOT NULL DEFAULT '', download_url TEXT NOT NULL DEFAULT '', expected_size INTEGER,
  expected_hashes TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(expected_hashes)), file_id TEXT REFERENCES file_objects(id) ON DELETE SET NULL,
  verification_status TEXT NOT NULL CHECK(verification_status IN('pending','verified','mismatch')), UNIQUE(release_id,file_key)
);
CREATE TABLE version_release_files (
  version_id TEXT NOT NULL REFERENCES mod_versions(id) ON DELETE RESTRICT, release_file_id TEXT NOT NULL REFERENCES platform_release_files(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK(status IN('candidate','verified','rejected')), evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(evidence)),
  PRIMARY KEY(version_id,release_file_id)
);

CREATE UNIQUE INDEX uq_pack_mod_instance_identity ON pack_mods(pack_id,id,mod_id);
CREATE TABLE pack_mod_selections (
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE, id TEXT NOT NULL, pack_mod_id TEXT NOT NULL,
  mod_id TEXT NOT NULL, version_id TEXT NOT NULL, acquisition TEXT NOT NULL CHECK(acquisition IN('local','modrinth','curseforge','mojang')),
  requested_version TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL,
  PRIMARY KEY(pack_id,id), UNIQUE(pack_id,pack_mod_id,id),
  FOREIGN KEY(pack_id,pack_mod_id,mod_id) REFERENCES pack_mods(pack_id,id,mod_id) ON DELETE CASCADE,
  FOREIGN KEY(mod_id,version_id) REFERENCES mod_versions(mod_id,id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX uq_pack_current_selection ON pack_mods(pack_id,current_selection_id) WHERE current_selection_id IS NOT NULL;
CREATE TABLE selection_platform_pins (
  pack_id TEXT NOT NULL, selection_id TEXT NOT NULL, role TEXT NOT NULL CHECK(role IN('primary','mirror')),
  release_file_id TEXT NOT NULL REFERENCES platform_release_files(id) ON DELETE RESTRICT,
  PRIMARY KEY(pack_id,selection_id,role,release_file_id),
  FOREIGN KEY(pack_id,selection_id) REFERENCES pack_mod_selections(pack_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_selection_primary_pin ON selection_platform_pins(pack_id,selection_id) WHERE role='primary';
CREATE TABLE pack_content_sources (
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE, id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK(kind IN('mod','datapack','resourcepack','document','runtime')), selection_id TEXT,
  external_revision TEXT, display_name TEXT NOT NULL DEFAULT '', descriptor TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(descriptor)), created_at INTEGER NOT NULL,
  PRIMARY KEY(pack_id,id), UNIQUE(pack_id,selection_id),
  FOREIGN KEY(pack_id,selection_id) REFERENCES pack_mod_selections(pack_id,id) ON DELETE RESTRICT
);
CREATE TABLE source_inputs (
  pack_id TEXT NOT NULL, source_id TEXT NOT NULL, input_no INTEGER NOT NULL CHECK(input_no>=0), file_id TEXT NOT NULL REFERENCES file_objects(id) ON DELETE RESTRICT,
  role TEXT NOT NULL, logical_path TEXT NOT NULL, provenance TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(provenance)),
  PRIMARY KEY(pack_id,source_id,input_no), UNIQUE(pack_id,source_id,role,logical_path),
  FOREIGN KEY(pack_id,source_id) REFERENCES pack_content_sources(pack_id,id) ON DELETE CASCADE
);

ALTER TABLE packs ADD COLUMN config_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE packs ADD COLUMN current_generation_id TEXT;

CREATE TABLE parse_runs (
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE, id TEXT NOT NULL, source_id TEXT, request_id TEXT,
  phase TEXT NOT NULL CHECK(phase IN('metadata','content')), task_id TEXT, config_revision INTEGER NOT NULL,
  environment TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(environment)), environment_sha256 TEXT NOT NULL DEFAULT '', parser_version TEXT NOT NULL,
  input_sha256 TEXT NOT NULL DEFAULT '', execution_status TEXT NOT NULL CHECK(execution_status IN('queued','running','succeeded','failed','canceled')),
  completeness TEXT NOT NULL CHECK(completeness IN('complete','partial','unresolved')), total_files INTEGER NOT NULL DEFAULT 0,
  parsed_count INTEGER NOT NULL DEFAULT 0, partial_count INTEGER NOT NULL DEFAULT 0, dynamic_count INTEGER NOT NULL DEFAULT 0, unsupported_count INTEGER NOT NULL DEFAULT 0, error_count INTEGER NOT NULL DEFAULT 0,
  diagnostics TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(diagnostics)), started_at INTEGER, finished_at INTEGER, created_at INTEGER NOT NULL,
  PRIMARY KEY(pack_id,id), FOREIGN KEY(pack_id,source_id) REFERENCES pack_content_sources(pack_id,id) ON DELETE CASCADE
);
CREATE INDEX idx_parse_runs_source ON parse_runs(pack_id,source_id,created_at DESC);
CREATE UNIQUE INDEX uq_parse_runs_source_id ON parse_runs(pack_id,source_id,id);
CREATE TABLE parse_run_inputs (
  pack_id TEXT NOT NULL, run_id TEXT NOT NULL, input_no INTEGER NOT NULL, file_id TEXT NOT NULL REFERENCES file_objects(id) ON DELETE RESTRICT,
  source_input_no INTEGER, logical_path TEXT NOT NULL, actual_sha256 TEXT NOT NULL CHECK(length(actual_sha256)=64),
  PRIMARY KEY(pack_id,run_id,input_no), FOREIGN KEY(pack_id,run_id) REFERENCES parse_runs(pack_id,id) ON DELETE CASCADE
);
CREATE TABLE file_mod_declarations (
  pack_id TEXT NOT NULL, run_id TEXT NOT NULL, input_no INTEGER NOT NULL, descriptor_path TEXT NOT NULL, ordinal INTEGER NOT NULL,
  declared_mod_id TEXT NOT NULL, declared_version TEXT NOT NULL DEFAULT '', resolved_mod_id TEXT REFERENCES mods(mod_id) ON DELETE RESTRICT,
  metadata TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(metadata)), status TEXT NOT NULL CHECK(status IN('declared','resolved','unresolved','invalid')),
  PRIMARY KEY(pack_id,run_id,input_no,descriptor_path,ordinal), FOREIGN KEY(pack_id,run_id,input_no) REFERENCES parse_run_inputs(pack_id,run_id,input_no) ON DELETE CASCADE
);
CREATE TABLE parse_coverage (
  pack_id TEXT NOT NULL, run_id TEXT NOT NULL, kind TEXT NOT NULL, resource_status TEXT NOT NULL CHECK(resource_status IN('pending','ready','missing','failed')),
  completeness TEXT NOT NULL CHECK(completeness IN('complete','partial','unresolved')), scanned_count INTEGER NOT NULL DEFAULT 0, parsed_count INTEGER NOT NULL DEFAULT 0,
  partial_count INTEGER NOT NULL DEFAULT 0, dynamic_count INTEGER NOT NULL DEFAULT 0, unsupported_count INTEGER NOT NULL DEFAULT 0, invalid_count INTEGER NOT NULL DEFAULT 0,
  diagnostics TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(diagnostics)), PRIMARY KEY(pack_id,run_id,kind), FOREIGN KEY(pack_id,run_id) REFERENCES parse_runs(pack_id,id) ON DELETE CASCADE
);
CREATE TABLE content_keys (
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE, id TEXT NOT NULL, kind TEXT NOT NULL,
  registry TEXT NOT NULL DEFAULT '', resource_key TEXT NOT NULL, qualifier TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(pack_id,id), UNIQUE(pack_id,kind,registry,resource_key,qualifier)
);
CREATE TABLE content_definitions (
  pack_id TEXT NOT NULL, id TEXT NOT NULL, key_id TEXT NOT NULL, run_id TEXT NOT NULL, input_no INTEGER NOT NULL,
  archive_path TEXT NOT NULL, fragment_key TEXT NOT NULL DEFAULT '', raw_file_id TEXT, raw_text TEXT, normalized TEXT,
  parse_status TEXT NOT NULL CHECK(parse_status IN('parsed','partial','unsupported','invalid')), runtime_status TEXT NOT NULL CHECK(runtime_status IN('static','dynamic','unknown')),
  conditions TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(conditions)), diagnostics TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(diagnostics)),
  PRIMARY KEY(pack_id,id), UNIQUE(pack_id,id,key_id), UNIQUE(pack_id,run_id,input_no,archive_path,fragment_key,key_id),
  FOREIGN KEY(pack_id,key_id) REFERENCES content_keys(pack_id,id) ON DELETE CASCADE, FOREIGN KEY(pack_id,run_id,input_no) REFERENCES parse_run_inputs(pack_id,run_id,input_no) ON DELETE RESTRICT,
  FOREIGN KEY(raw_file_id) REFERENCES file_objects(id) ON DELETE RESTRICT, CHECK(raw_file_id IS NOT NULL OR raw_text IS NOT NULL)
);
CREATE UNIQUE INDEX uq_content_definitions_pack_id ON content_definitions(pack_id,id);

CREATE TABLE recipe_definitions(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,recipe_type TEXT NOT NULL,layout TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(layout)),PRIMARY KEY(pack_id,definition_id),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id) ON DELETE CASCADE);
CREATE TABLE recipe_terms(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,role TEXT NOT NULL CHECK(role IN('input','output')),slot INTEGER NOT NULL CHECK(slot>=0),alternative INTEGER NOT NULL CHECK(alternative>=0),ref_kind TEXT NOT NULL CHECK(ref_kind IN('item','item_tag','unknown')),target_key_id TEXT,count INTEGER,position TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(position)),predicate TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(predicate)),raw_term TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(raw_term)),PRIMARY KEY(pack_id,definition_id,role,slot,alternative),FOREIGN KEY(pack_id,definition_id) REFERENCES recipe_definitions(pack_id,definition_id) ON DELETE CASCADE,FOREIGN KEY(pack_id,target_key_id) REFERENCES content_keys(pack_id,id) ON DELETE RESTRICT,CHECK(count IS NULL OR count>0));
CREATE TABLE tag_definitions(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,replace_existing INTEGER NOT NULL DEFAULT 0 CHECK(replace_existing IN(0,1)),PRIMARY KEY(pack_id,definition_id),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id) ON DELETE CASCADE);
CREATE TABLE tag_entries(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,ordinal INTEGER NOT NULL,operation TEXT NOT NULL CHECK(operation IN('add','remove')),target_kind TEXT NOT NULL CHECK(target_kind IN('member','tag')),target_key_id TEXT NOT NULL,required INTEGER NOT NULL CHECK(required IN(0,1)),conditions TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(conditions)),raw_entry TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(raw_entry)),PRIMARY KEY(pack_id,definition_id,ordinal),FOREIGN KEY(pack_id,definition_id) REFERENCES tag_definitions(pack_id,definition_id) ON DELETE CASCADE,FOREIGN KEY(pack_id,target_key_id) REFERENCES content_keys(pack_id,id) ON DELETE RESTRICT);
CREATE TABLE language_definitions(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,text TEXT NOT NULL,PRIMARY KEY(pack_id,definition_id),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id) ON DELETE CASCADE);
CREATE TABLE content_text_bindings(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,role TEXT NOT NULL,ordinal INTEGER NOT NULL,translation_key TEXT NOT NULL,evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(evidence)),PRIMARY KEY(pack_id,definition_id,role,ordinal),FOREIGN KEY(pack_id,definition_id) REFERENCES language_definitions(pack_id,definition_id) ON DELETE CASCADE);
CREATE TABLE definition_links(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,ordinal INTEGER NOT NULL,role TEXT NOT NULL,from_key_id TEXT NOT NULL,to_key_id TEXT NOT NULL,detail TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(detail)),certainty TEXT NOT NULL,PRIMARY KEY(pack_id,definition_id,ordinal),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id) ON DELETE CASCADE,FOREIGN KEY(pack_id,from_key_id) REFERENCES content_keys(pack_id,id) ON DELETE RESTRICT,FOREIGN KEY(pack_id,to_key_id) REFERENCES content_keys(pack_id,id) ON DELETE RESTRICT);
CREATE TABLE asset_definitions(pack_id TEXT NOT NULL,definition_id TEXT NOT NULL,form TEXT NOT NULL,data_file_id TEXT,mime TEXT,width INTEGER,height INTEGER,support_status TEXT NOT NULL,reason TEXT NOT NULL DEFAULT '',PRIMARY KEY(pack_id,definition_id),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id) ON DELETE CASCADE,FOREIGN KEY(data_file_id) REFERENCES file_objects(id) ON DELETE RESTRICT);

CREATE TABLE catalog_generations(pack_id TEXT NOT NULL,id TEXT NOT NULL,config_revision INTEGER NOT NULL,environment TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(environment)),input_sha256 TEXT NOT NULL,resolver_version TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN('building','ready','failed','superseded')),completeness TEXT NOT NULL CHECK(completeness IN('complete','partial','unresolved')),started_at INTEGER NOT NULL,finished_at INTEGER,diagnostics TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(diagnostics)),PRIMARY KEY(pack_id,id),FOREIGN KEY(pack_id) REFERENCES packs(id) ON DELETE CASCADE);
CREATE TABLE catalog_generation_sources(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,source_id TEXT NOT NULL,run_id TEXT,inclusion_status TEXT NOT NULL,priority INTEGER,order_evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(order_evidence)),condition_result TEXT NOT NULL CHECK(condition_result IN('true','false','unknown')),PRIMARY KEY(pack_id,generation_id,source_id),FOREIGN KEY(pack_id,generation_id) REFERENCES catalog_generations(pack_id,id) ON DELETE CASCADE,FOREIGN KEY(pack_id,source_id) REFERENCES pack_content_sources(pack_id,id) ON DELETE CASCADE,FOREIGN KEY(pack_id,source_id,run_id) REFERENCES parse_runs(pack_id,source_id,id) ON DELETE RESTRICT);
CREATE TABLE catalog_entries(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,key_id TEXT NOT NULL,winner_definition_id TEXT,availability TEXT NOT NULL CHECK(availability IN('available','reference_only','missing','dynamic','unsupported','invalid','pending')),completeness TEXT NOT NULL,diagnostics TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(diagnostics)),PRIMARY KEY(pack_id,generation_id,key_id),FOREIGN KEY(pack_id,generation_id) REFERENCES catalog_generations(pack_id,id) ON DELETE CASCADE,FOREIGN KEY(pack_id,key_id) REFERENCES content_keys(pack_id,id) ON DELETE RESTRICT,FOREIGN KEY(pack_id,winner_definition_id) REFERENCES content_definitions(pack_id,id) ON DELETE RESTRICT);
CREATE TABLE catalog_definition_decisions(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,definition_id TEXT NOT NULL,key_id TEXT NOT NULL,decision TEXT NOT NULL CHECK(decision IN('selected','contributed','overridden','excluded','unresolved')),precedence INTEGER,condition_result TEXT NOT NULL CHECK(condition_result IN('true','false','unknown')),reason TEXT NOT NULL DEFAULT '',evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(evidence)),PRIMARY KEY(pack_id,generation_id,definition_id),FOREIGN KEY(pack_id,generation_id,key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id));
CREATE UNIQUE INDEX uq_catalog_selected ON catalog_definition_decisions(pack_id,generation_id,key_id) WHERE decision='selected';
CREATE TABLE catalog_tag_members(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,tag_key_id TEXT NOT NULL,member_key_id TEXT NOT NULL,certainty TEXT NOT NULL CHECK(certainty IN('confirmed','possible')),PRIMARY KEY(pack_id,generation_id,tag_key_id,member_key_id),FOREIGN KEY(pack_id,generation_id,tag_key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id),FOREIGN KEY(pack_id,generation_id,member_key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id));
CREATE INDEX idx_catalog_reverse_tags ON catalog_tag_members(pack_id,generation_id,member_key_id,tag_key_id);
CREATE TABLE catalog_tag_member_evidence(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,id TEXT NOT NULL,tag_key_id TEXT NOT NULL,member_key_id TEXT NOT NULL,definition_id TEXT NOT NULL,entry_ordinal INTEGER NOT NULL,nested_path TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(nested_path)),PRIMARY KEY(pack_id,generation_id,id),FOREIGN KEY(pack_id,generation_id,tag_key_id,member_key_id) REFERENCES catalog_tag_members(pack_id,generation_id,tag_key_id,member_key_id));
CREATE TABLE catalog_tag_issues(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,id TEXT NOT NULL,tag_key_id TEXT NOT NULL,definition_id TEXT,entry_ordinal INTEGER,target_key_id TEXT,code TEXT NOT NULL,detail TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(detail)),PRIMARY KEY(pack_id,generation_id,id),FOREIGN KEY(pack_id,generation_id,tag_key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id));
CREATE TABLE catalog_text_resolutions(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,key_id TEXT NOT NULL,role TEXT NOT NULL,requested_locale TEXT NOT NULL,text TEXT NOT NULL,resolved_locale TEXT,fallback_kind TEXT NOT NULL,language_definition_id TEXT,binding_definition_id TEXT,PRIMARY KEY(pack_id,generation_id,key_id,role,requested_locale),FOREIGN KEY(pack_id,generation_id,key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id));
CREATE TABLE catalog_item_block_links(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,item_key_id TEXT NOT NULL,block_key_id TEXT NOT NULL,definition_id TEXT NOT NULL,link_ordinal INTEGER NOT NULL,PRIMARY KEY(pack_id,generation_id,item_key_id,block_key_id,definition_id,link_ordinal),FOREIGN KEY(pack_id,generation_id,item_key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id),FOREIGN KEY(pack_id,generation_id,block_key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id));
CREATE TABLE catalog_icons(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,key_id TEXT NOT NULL,view_kind TEXT NOT NULL,status TEXT NOT NULL,file_id TEXT,width INTEGER,height INTEGER,mime TEXT,source_kind TEXT NOT NULL,renderer_version TEXT NOT NULL,input_sha256 TEXT,reason TEXT NOT NULL DEFAULT '',PRIMARY KEY(pack_id,generation_id,key_id,view_kind),FOREIGN KEY(pack_id,generation_id,key_id) REFERENCES catalog_entries(pack_id,generation_id,key_id),FOREIGN KEY(file_id) REFERENCES file_objects(id) ON DELETE RESTRICT);
CREATE TABLE catalog_icon_inputs(pack_id TEXT NOT NULL,generation_id TEXT NOT NULL,key_id TEXT NOT NULL,view_kind TEXT NOT NULL,definition_id TEXT NOT NULL,role TEXT NOT NULL,PRIMARY KEY(pack_id,generation_id,key_id,view_kind,definition_id,role),FOREIGN KEY(pack_id,generation_id,key_id,view_kind) REFERENCES catalog_icons(pack_id,generation_id,key_id,view_kind),FOREIGN KEY(pack_id,definition_id) REFERENCES content_definitions(pack_id,id));

-- Legacy evidence is explicitly unresolved: it is never selected as current truth.
INSERT OR IGNORE INTO mods(mod_id,display_name,kind) SELECT 'minecraft','Minecraft','builtin' WHERE EXISTS(SELECT 1 FROM packs);
INSERT OR IGNORE INTO pack_mods(id,pack_id,source,display_name,status,required,added_at,updated_at,mod_id,origin)
SELECT 'minecraft-'||p.id,p.id,'local','Minecraft','pending',1,p.created_at,p.updated_at,'minecraft','builtin' FROM packs p
WHERE NOT EXISTS(SELECT 1 FROM pack_mods pm WHERE pm.pack_id=p.id AND pm.mod_id='minecraft');
CREATE TRIGGER IF NOT EXISTS pack_mods_minecraft_guard_insert BEFORE INSERT ON pack_mods WHEN NEW.mod_id='minecraft' AND NEW.origin<>'builtin' BEGIN SELECT RAISE(ABORT,'minecraft must be builtin'); END;
CREATE TRIGGER IF NOT EXISTS pack_mods_minecraft_guard_update BEFORE UPDATE OF mod_id,origin ON pack_mods WHEN NEW.mod_id='minecraft' AND NEW.origin<>'builtin' BEGIN SELECT RAISE(ABORT,'minecraft must be builtin'); END;
