package store

import (
	"path/filepath"
	"testing"
)

func TestItemCatalogSchemaRelationsAndCascade(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO packs(id,name,mc_version,loader,status,created_at,updated_at,last_edited_at) VALUES('p','Pack','1.21.1','neoforge','active',1,1,1)`); err != nil {
		t.Fatal(err)
	}
	var source, built int
	var status string
	if err = db.QueryRow(`SELECT source_revision,built_revision,build_status FROM pack_catalog_state WHERE pack_id='p'`).Scan(&source, &built, &status); err != nil {
		t.Fatal(err)
	}
	if source != 1 || built != 0 || status != "pending" {
		t.Fatalf("unexpected initial state %d/%d/%s", source, built, status)
	}
	statements := []string{
		`INSERT INTO pack_catalog_items(pack_id,item_id,evidence,source,model_path,icon_status,icon_reason) VALUES('p','minecraft:iron_ingot','model','minecraft:1.21.1','assets/minecraft/models/item/iron_ingot.json','ready','')`,
		`INSERT INTO pack_catalog_item_names VALUES('p','minecraft:iron_ingot','zh_cn','铁锭','item.minecraft.iron_ingot','minecraft:1.21.1')`,
		`INSERT INTO pack_catalog_item_icons VALUES('p','minecraft:iron_ingot','image/png',x'89504E47',32,32,'generated')`,
		`INSERT INTO pack_catalog_blocks VALUES('p','minecraft:iron_block','blockstate','minecraft:1.21.1','assets/minecraft/blockstates/iron_block.json')`,
		`INSERT INTO pack_catalog_item_blocks VALUES('p','minecraft:iron_ingot','minecraft:iron_block')`,
		`INSERT INTO pack_catalog_tags VALUES('p','item','c:ingots/iron','resolved','[]')`,
		`INSERT INTO pack_catalog_tag_definitions VALUES('p','item','c:ingots/iron',0,'neoforge','data/c/tags/item/ingots/iron.json','{"values":["minecraft:iron_ingot"]}',0,'[]')`,
		`INSERT INTO pack_catalog_tag_entries VALUES('p','item','c:ingots/iron',0,0,'member','minecraft:iron_ingot',1,'add')`,
		`INSERT INTO pack_catalog_tag_members VALUES('p','item','c:ingots/iron','minecraft:iron_ingot')`,
		`INSERT INTO pack_catalog_recipes VALUES('p','minecraft:iron_block','minecraft:crafting_shaped','minecraft:1.21.1','data/minecraft/recipe/iron_block.json','{"type":"minecraft:crafting_shaped"}','parsed','[]')`,
		`INSERT INTO pack_catalog_recipe_refs VALUES('p','minecraft:iron_block','input',0,0,'item_tag','c:ingots/iron',1)`,
	}
	for _, statement := range statements {
		if _, err = db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	var tagID string
	if err = db.QueryRow(`SELECT tag_id FROM pack_catalog_tag_members WHERE pack_id='p' AND registry='item' AND member_id='minecraft:iron_ingot'`).Scan(&tagID); err != nil || tagID != "c:ingots/iron" {
		t.Fatalf("reverse tag lookup %q: %v", tagID, err)
	}
	if _, err = db.Exec(`INSERT INTO pack_catalog_tag_members VALUES('p','item','c:ingots/iron','minecraft:iron_ingot')`); err == nil {
		t.Fatal("duplicate tag member accepted")
	}
	if _, err = db.Exec(`DELETE FROM packs WHERE id='p'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM pack_catalog_items WHERE pack_id='p'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("catalog did not cascade: %d %v", count, err)
	}
}

func TestCatalogRevisionChangesWithSources(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "revision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO packs(id,name,mc_version,loader,status,created_at,updated_at,last_edited_at) VALUES('p','Pack','1.21.1','neoforge','active',1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO pack_mods(id,pack_id,source,display_name,status,added_at,updated_at) VALUES('m','p','local','Mod','installed',1,1)`); err != nil {
		t.Fatal(err)
	}
	var revision int
	if err = db.QueryRow(`SELECT source_revision FROM pack_catalog_state WHERE pack_id='p'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 2 {
		t.Fatalf("source revision %d, want 2", revision)
	}
}
