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

// 回归（0028）：纯展示字段不该让目录作废。
// 0013 的触发器是 `AFTER UPDATE ON pack_mods`（不带列名），所以改一次分类标签
// 就会把整个物品目录打成 pending，用户看到的是 409 catalog_stale + 要重建 13000 个文件。
// 这里把「哪些列改了算数」钉死：category 不算，status 算。
func TestCatalogRevisionIgnoresDisplayOnlyModFields(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "category.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO packs(id,name,mc_version,loader,status,created_at,updated_at,last_edited_at) VALUES('p','Pack','1.21.1','fabric','active',1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO pack_mods(id,pack_id,source,display_name,status,added_at,updated_at) VALUES('m','p','local','Mod','installed',1,1)`); err != nil {
		t.Fatal(err)
	}
	revision := func() int {
		t.Helper()
		var v int
		if e := db.QueryRow(`SELECT source_revision FROM pack_catalog_state WHERE pack_id='p'`).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	base := revision()

	// 分类：搬来搬去、改名、清空 —— 一次都不该动目录版本。
	for _, category := range []string{"优化", "科技", "主线", ""} {
		if _, err = db.Exec(`UPDATE pack_mods SET category=?, updated_at=updated_at+1 WHERE pack_id='p' AND id='m'`, category); err != nil {
			t.Fatal(err)
		}
		if got := revision(); got != base {
			t.Fatalf("category=%q 把目录版本从 %d 抬到 %d —— 分类是纯展示字段，不该让目录作废", category, base, got)
		}
	}
	// 同值空写也不该动（`UPDATE OF` 只看语句提到哪些列，守卫才能拦住这种）。
	if _, err = db.Exec(`UPDATE pack_mods SET status=status WHERE pack_id='p' AND id='m'`); err != nil {
		t.Fatal(err)
	}
	if got := revision(); got != base {
		t.Fatalf("空写把目录版本从 %d 抬到 %d", base, got)
	}
	// 启停是会改变目录成员的（重建时 Status != "installed" 会被跳过），必须仍然算数。
	if _, err = db.Exec(`UPDATE pack_mods SET status='disabled' WHERE pack_id='p' AND id='m'`); err != nil {
		t.Fatal(err)
	}
	if got := revision(); got != base+1 {
		t.Fatalf("停用模组后目录版本 = %d, want %d —— 停用会改变目录成员，必须让目录失效", got, base+1)
	}
	// 换版本同理（sha1 有 FK 指向 jar_index，这里用没有 FK 的 version_id，
	// 反正两者都是「换了一个 mod 文件」的信号，触发器同一档）。
	base = revision()
	if _, err = db.Exec(`UPDATE pack_mods SET version_id='v-2' WHERE pack_id='p' AND id='m'`); err != nil {
		t.Fatal(err)
	}
	if got := revision(); got != base+1 {
		t.Fatalf("换 version_id 后目录版本 = %d, want %d", got, base+1)
	}
}
