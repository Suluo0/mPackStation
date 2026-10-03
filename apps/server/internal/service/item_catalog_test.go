package service

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

func writeCatalogFixture(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "cache", "minecraft-assets")
	if err := os.MkdirAll(filepath.Join(dir, "1.21.1", "lang"), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "1.21.1.jar"))
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	files := map[string]string{
		"assets/minecraft/models/item/redstone.json":    `{"parent":"minecraft:item/generated"}`,
		"assets/minecraft/models/item/cobblestone.json": `{"parent":"minecraft:block/cobblestone"}`,
		"assets/minecraft/models/item/iron_ingot.json":  `{"parent":"minecraft:item/generated"}`,
		"assets/minecraft/models/item/diamond.json":     `{"parent":"minecraft:item/generated"}`,
		// 0025 回归锁：动画帧模型没有语言键，不得成为物品条目
		"assets/minecraft/models/item/clock_01.json":      `{"parent":"minecraft:item/generated"}`,
		"assets/minecraft/models/item/clock_02.json":      `{"parent":"minecraft:item/generated"}`,
		"assets/minecraft/models/item/bow_pulling_0.json": `{"parent":"minecraft:item/generated"}`,
		"assets/minecraft/blockstates/cobblestone.json":   `{"variants":{"":{"model":"minecraft:block/cobblestone"}}}`,
		// 语言表比版本超前/遗留的键：只有名字、既无模型也无引用，不得成为条目
		"assets/minecraft/lang/en_us.json":                       `{"item.minecraft.redstone":"Redstone Dust","block.minecraft.cobblestone":"Cobblestone","item.minecraft.cobblestone":"Cobblestone","item.minecraft.iron_ingot":"Iron Ingot","item.minecraft.diamond":"Diamond","item.minecraft.black_bundle":"Black Bundle","item.modifiers.head":"When on Head:","item.op_block_warning.line1":"Warning"}`,
		"data/minecraft/tags/item/stone_crafting_materials.json": `{"values":["minecraft:cobblestone"]}`,
		"data/minecraft/tags/item/all_stone.json":                `{"values":["#minecraft:stone_crafting_materials"]}`,
		"data/minecraft/tags/block/mineable/pickaxe.json":        `{"values":["minecraft:cobblestone"]}`,
		"data/minecraft/recipe/iron_block.json":                  `{"type":"minecraft:crafting_shaped","key":{"#":{"item":"minecraft:iron_ingot"}},"pattern":["###","###","###"],"result":{"id":"minecraft:iron_block","count":1}}`,
	}
	for name, body := range files {
		entry, createErr := w.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr = entry.Write([]byte(body)); createErr != nil {
			t.Fatal(createErr)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "1.21.1", "lang", "zh_cn.json"), []byte(`{"item.minecraft.redstone":"红石粉","block.minecraft.cobblestone":"圆石","item.minecraft.cobblestone":"圆石","item.minecraft.iron_ingot":"铁锭","item.minecraft.diamond":"钻石","item.minecraft.black_bundle":"黑色收纳袋"}`), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIndexesVanillaItemsBlocksRecipesTagsAndNames(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := New(db)
	pack, err := api.CreatePack(context.Background(), CreatePackInput{Name: "Catalog", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1"}, "create")
	if err != nil {
		t.Fatal(err)
	}
	writeCatalogFixture(t, dir)
	catalog, err := api.RebuildItemCatalog(context.Background(), pack.ID, "zh_cn", "rebuild")
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]CatalogItemView{}
	for _, item := range catalog.Items {
		items[item.ID] = item
	}
	itemKnown := func(id string) bool { _, ok := items[id]; return ok }
	if items["minecraft:redstone"].DisplayName != "红石粉" || items["minecraft:iron_ingot"].DisplayName != "铁锭" || items["minecraft:diamond"].DisplayName != "钻石" {
		t.Fatalf("localized items missing: %#v", items)
	}
	if len(items["minecraft:cobblestone"].Tags) != 2 {
		t.Fatalf("nested reverse tags missing: %#v", items["minecraft:cobblestone"].Tags)
	}
	blocks := map[string]CatalogBlockView{}
	for _, block := range catalog.Blocks {
		blocks[block.ID] = block
	}
	if blocks["minecraft:cobblestone"].DisplayName != "圆石" || len(blocks["minecraft:cobblestone"].ItemIDs) != 1 {
		t.Fatalf("block relation missing: %#v", blocks["minecraft:cobblestone"])
	}
	if len(catalog.Recipes) != 1 || len(catalog.Recipes[0].Refs) != 10 {
		t.Fatalf("recipe references = %#v", catalog.Recipes)
	}
	// 0025：语言键是物品权威；模型残渣（clock_01 / bow_pulling_0）不得出现
	if itemKnown("minecraft:clock_01") || itemKnown("minecraft:bow_pulling_0") {
		t.Fatalf("model-only junk leaked into catalog: %#v", items)
	}
	// v4：只有名字、既无模型资产又无配方/标签引用的遗留语言键（别的版本的内容、
	// item.modifiers.* 这类描述键）不得成为条目
	if itemKnown("minecraft:black_bundle") || itemKnown("modifiers:head") || itemKnown("op_block_warning:line1") {
		t.Fatalf("name-only lang leftovers leaked into catalog: %#v", items)
	}
	if items["minecraft:redstone"].Evidence != "lang" {
		t.Fatalf("localized item evidence = %q, want lang", items["minecraft:redstone"].Evidence)
	}
	if items["minecraft:redstone"].ModelPath != "assets/minecraft/models/item/redstone.json" {
		t.Fatalf("model path not backfilled: %#v", items["minecraft:redstone"])
	}
	// iron_block 只在配方里被引用（reference 证据），没有语言键/模型——仍应存在
	if !itemKnown("minecraft:iron_block") {
		t.Fatalf("recipe-referenced item missing: %#v", items)
	}
	status, err := api.GetCatalogStatus(context.Background(), pack.ID)
	if err != nil || status.Stale || status.Status != "succeeded" {
		t.Fatalf("catalog status %#v: %v", status, err)
	}
}

func TestCreatePackQueuesCatalogInitialization(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	queue, err := task.NewQueue(db)
	if err != nil {
		t.Fatal(err)
	}
	api := New(db)
	api.SetTaskQueue(queue)
	pack, err := api.CreatePack(context.Background(), CreatePackInput{Name: "Queued", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1"}, "create")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := queue.List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Kind != task.KindCatalogInit || tasks[0].PackID == nil || *tasks[0].PackID != pack.ID {
		t.Fatalf("catalog init task not queued: %#v", tasks)
	}
}

func TestFullVanillaCatalogCoverage(t *testing.T) {
	source := os.Getenv("MPACKSTATION_TEST_VANILLA_JAR")
	if source == "" {
		t.Skip("set MPACKSTATION_TEST_VANILLA_JAR for the full archive check")
	}
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache", "minecraft-assets")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(cache, "1.21.1.jar"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := New(db)
	pack, err := api.CreatePack(context.Background(), CreatePackInput{Name: "Full vanilla", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1"}, "create")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := api.RebuildItemCatalog(context.Background(), pack.ID, "en_us", "full")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("coverage items=%d blocks=%d recipes=%d tags=%d", len(catalog.Items), len(catalog.Blocks), len(catalog.Recipes), len(catalog.Tags))
	if len(catalog.Items) < 1000 || len(catalog.Blocks) < 1000 || len(catalog.Recipes) < 1200 || len(catalog.Tags) < 300 {
		t.Fatalf("unexpected coverage items=%d blocks=%d recipes=%d tags=%d", len(catalog.Items), len(catalog.Blocks), len(catalog.Recipes), len(catalog.Tags))
	}
	want := map[string]string{"minecraft:redstone": "Redstone Dust", "minecraft:cobblestone": "Cobblestone", "minecraft:iron_ingot": "Iron Ingot", "minecraft:diamond": "Diamond"}
	for _, item := range catalog.Items {
		if name, ok := want[item.ID]; ok {
			if item.DisplayName != name {
				t.Errorf("%s name %q", item.ID, item.DisplayName)
			}
			delete(want, item.ID)
		}
	}
	if len(want) > 0 {
		t.Fatalf("sample items missing: %v", want)
	}
	for _, id := range []string{"minecraft:redstone", "minecraft:cobblestone", "minecraft:iron_ingot", "minecraft:diamond"} {
		icon, iconErr := api.GetCatalogIcon(context.Background(), pack.ID, id)
		if iconErr != nil || icon.Mime != "image/png" || len(icon.Data) == 0 {
			t.Fatalf("sample icon %s unavailable: %v", id, iconErr)
		}
	}
}
