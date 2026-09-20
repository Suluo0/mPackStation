package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// buildMinimalJar constructs a tiny fabric mod jar in memory with one
// standard recipe, one special dynamic recipe, one item model, and one
// worldgen structure.
func buildMinimalJar(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"testmod","name":"Test Mod","version":"1.0.0"}`)
	addZipFile(t, zw, "data/testmod/recipe/simple.json", `{"type":"minecraft:crafting_shapeless","ingredients":[{"item":"minecraft:dirt"}],"result":{"id":"testmod:magic_dirt","count":1}}`)
	addZipFile(t, zw, "data/testmod/recipe/special/wildcard.json", `{"type":"testmod:wildcard"}`)
	addZipFile(t, zw, "assets/testmod/models/item/magic_dirt.json", `{}`)
	addZipFile(t, zw, "data/testmod/worldgen/structure/test_ruin.json", `{"type":"minecraft:jigsaw"}`)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func addZipFile(t *testing.T, zw *zip.Writer, name, content string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestExtractModContent_MinimalJar(t *testing.T) {
	jar := buildMinimalJar(t)
	ext, err := ExtractModContent(bytes.NewReader(jar))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if ext.ModID != "testmod" {
		t.Errorf("ModID = %q, want testmod", ext.ModID)
	}
	if ext.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", ext.Version)
	}
	if ext.Loader != "fabric" {
		t.Errorf("Loader = %q, want fabric", ext.Loader)
	}
	if len(ext.Recipes) != 2 {
		t.Fatalf("Recipes len = %d, want 2", len(ext.Recipes))
	}
	// Standard recipe.
	std := ext.Recipes[0]
	if std.IsDynamic {
		t.Errorf("standard recipe IsDynamic = true, want false")
	}
	if std.ParseError != "" {
		t.Errorf("standard recipe ParseError = %q, want empty", std.ParseError)
	}
	var stdPayload map[string]any
	if err := json.Unmarshal(std.Payload, &stdPayload); err != nil {
		t.Fatalf("unmarshal standard recipe: %v", err)
	}
	if stdPayload["type"] != "minecraft:crafting_shapeless" {
		t.Errorf("standard recipe type = %v, want minecraft:crafting_shapeless", stdPayload["type"])
	}
	// Dynamic recipe.
	dyn := ext.Recipes[1]
	if !dyn.IsDynamic {
		t.Errorf("special recipe IsDynamic = false, want true")
	}
	var dynPayload map[string]any
	if err := json.Unmarshal(dyn.Payload, &dynPayload); err != nil {
		t.Fatalf("unmarshal dynamic recipe: %v", err)
	}
	if dynPayload["type"] != "testmod:wildcard" {
		t.Errorf("dynamic recipe type = %v, want testmod:wildcard", dynPayload["type"])
	}
	if len(ext.Items) != 1 {
		t.Errorf("Items len = %d, want 1", len(ext.Items))
	}
	if len(ext.Structures) != 1 {
		t.Errorf("Structures len = %d, want 1", len(ext.Structures))
	}
	if ext.Stats.ParsedCount != 5 {
		t.Errorf("Stats.ParsedCount = %d, want 5", ext.Stats.ParsedCount)
	}
	if ext.Stats.DynamicCount != 1 {
		t.Errorf("Stats.DynamicCount = %d, want 1", ext.Stats.DynamicCount)
	}
}

func TestExtractModContent_NeoforgeMetadata(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "META-INF/neoforge.mods.toml", "modLoader=\"javafml\"\nloaderVersion=\"[1,)\"\n[[mods]]\nmodId=\"neomod\"\nversion=\"2.0.0\"\ndisplayName=\"Neo Mod\"\n")
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if ext.ModID != "neomod" {
		t.Errorf("ModID = %q, want neomod", ext.ModID)
	}
	if ext.Version != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", ext.Version)
	}
	if ext.Loader != "neoforge" {
		t.Errorf("Loader = %q, want neoforge", ext.Loader)
	}
}

func TestExtractModContent_InvalidJsonSkipped(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"badmod","version":"1.0"}`)
	addZipFile(t, zw, "data/badmod/recipe/good.json", `{"type":"minecraft:crafting_shapeless","ingredients":[],"result":{"id":"badmod:x"}}`)
	addZipFile(t, zw, "data/badmod/recipe/bad.json", `{invalid json`)
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(ext.Recipes) != 2 {
		t.Fatalf("Recipes len = %d, want 2", len(ext.Recipes))
	}
	if ext.Recipes[0].ParseError != "" {
		t.Errorf("good recipe ParseError = %q, want empty", ext.Recipes[0].ParseError)
	}
	if ext.Recipes[1].ParseError == "" {
		t.Errorf("bad recipe ParseError = empty, want non-empty")
	}
	if ext.Stats.ErrorCount != 1 {
		t.Errorf("Stats.ErrorCount = %d, want 1", ext.Stats.ErrorCount)
	}
}

func TestExtractModContent_NotAZip(t *testing.T) {
	_, err := ExtractModContent(strings.NewReader("this is not a zip file"))
	if err == nil {
		t.Fatal("expected error for non-zip input, got nil")
	}
}

func TestExtractModContent_EmptyJar(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if ext.ModID != "" {
		t.Errorf("ModID = %q, want empty", ext.ModID)
	}
	if ext.Stats.ParsedCount != 0 {
		t.Errorf("Stats.ParsedCount = %d, want 0", ext.Stats.ParsedCount)
	}
}

func TestExtractModContent_PathTraversalSkipped(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"evilmod","version":"1.0"}`)
	addZipFile(t, zw, "../../etc/passwd", "evil content")
	addZipFile(t, zw, "data/evilmod/recipe/ok.json", `{"type":"minecraft:crafting_shapeless","ingredients":[],"result":{"id":"evilmod:ok"}}`)
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(ext.Recipes) != 1 {
		t.Errorf("Recipes len = %d, want 1 (path traversal should be skipped)", len(ext.Recipes))
	}
}

func TestExtractModContent_ForgeMetadata(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "META-INF/mods.toml", "modLoader=\"javafml\"\nloaderVersion=\"[1,)\"\n[[mods]]\nmodId=\"forgemod\"\nversion=\"3.0.0\"\ndisplayName=\"Forge Mod\"\n")
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if ext.ModID != "forgemod" {
		t.Errorf("ModID = %q, want forgemod", ext.ModID)
	}
	if ext.Version != "3.0.0" {
		t.Errorf("Version = %q, want 3.0.0", ext.Version)
	}
	if ext.Loader != "forge" {
		t.Errorf("Loader = %q, want forge", ext.Loader)
	}
}

func TestParseRecipe_StandardTypes(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"crafting_shaped", `{"type":"minecraft:crafting_shaped","pattern":["###","#X#","###"],"key":{"#":{"item":"minecraft:iron_ingot"},"X":{"item":"minecraft:redstone"}},"result":{"id":"minecraft:compass","count":1}}`},
		{"crafting_shapeless", `{"type":"minecraft:crafting_shapeless","ingredients":[{"item":"minecraft:dirt"}],"result":{"id":"testmod:magic_dirt","count":1}}`},
		{"smelting", `{"type":"minecraft:smelting","ingredient":{"item":"minecraft:iron_ore"},"result":"minecraft:iron_ingot","experience":0.7,"cookingtime":200}`},
		{"blasting", `{"type":"minecraft:blasting","ingredient":{"item":"minecraft:iron_ore"},"result":"minecraft:iron_ingot","experience":0.7,"cookingtime":100}`},
		{"stonecutting", `{"type":"minecraft:stonecutting","ingredient":{"item":"minecraft:stone"},"result":"minecraft:stone_stairs","count":1}`},
		{"smithing_transform", `{"type":"minecraft:smithing_transform","template":{"item":"minecraft:netherite_upgrade_smithing_template"},"base":{"item":"minecraft:diamond_pickaxe"},"addition":{"item":"minecraft:netherite_ingot"},"result":{"id":"minecraft:netherite_pickaxe","count":1}}`},
		{"custom_type", `{"type":"ae2:inscriber","ingredients":{"top":{"item":"ae2:logic_processor"},"middle":{"item":"minecraft:iron_ingot"}},"result":{"id":"ae2:calculation_processor","count":1}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			addZipFile(t, zw, "fabric.mod.json", `{"id":"testmod","version":"1.0"}`)
			addZipFile(t, zw, "data/testmod/recipe/"+c.name+".json", c.json)
			zw.Close()
			ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if len(ext.Recipes) != 1 {
				t.Fatalf("recipes len = %d, want 1", len(ext.Recipes))
			}
			r := ext.Recipes[0]
			if r.IsDynamic {
				t.Errorf("IsDynamic = true, want false for %s", c.name)
			}
			if r.ParseError != "" {
				t.Errorf("ParseError = %q, want empty", r.ParseError)
			}
			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if payload["type"] == nil {
				t.Error("payload missing type field")
			}
		})
	}
}

func TestParseRecipe_DynamicDetection(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		json      string
		wantDyn   bool
	}{
		{"special_no_content", "data/x/recipe/special/wildcard.json", `{"type":"x:wildcard"}`, true},
		{"normal_with_content", "data/x/recipe/normal.json", `{"type":"x:custom","ingredients":[],"result":{}}`, false},
		{"special_with_content", "data/x/recipe/special/full.json", `{"type":"x:full","ingredients":[{"item":"minecraft:dirt"}],"result":{"id":"x:y"}}`, false},
		{"special_only_type_and_extra", "data/x/recipe/special/extra.json", `{"type":"x:extra","conditions":[]}`, true},
		// Vanilla special recipes are often flat under recipe/, not recipe/special/.
		{"vanilla_book_cloning_flat", "data/minecraft/recipe/book_cloning.json", `{"type":"minecraft:crafting_special_bookcloning","category":"misc"}`, true},
		{"vanilla_decorated_pot_flat", "data/minecraft/recipe/decorated_pot.json", `{"type":"minecraft:crafting_decorated_pot"}`, true},
		{"type_only_custom_flat", "data/x/recipe/only_type.json", `{"type":"x:runtime_logic"}`, true},
		{"furnace_with_content", "data/x/recipe/smelting.json", `{"type":"minecraft:smelting","ingredient":{"item":"minecraft:iron_ore"},"result":"minecraft:iron_ingot"}`, false},
		{"shaped_with_content_flat", "data/x/recipe/planks.json", `{"type":"minecraft:crafting_shaped","pattern":["#"],"key":{"#":{"item":"minecraft:oak_log"}},"result":{"id":"minecraft:oak_planks","count":4}}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			addZipFile(t, zw, "fabric.mod.json", `{"id":"x","version":"1.0"}`)
			addZipFile(t, zw, c.path, c.json)
			zw.Close()
			ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if len(ext.Recipes) != 1 {
				t.Fatalf("recipes len = %d, want 1", len(ext.Recipes))
			}
			if ext.Recipes[0].IsDynamic != c.wantDyn {
				t.Errorf("IsDynamic = %v, want %v", ext.Recipes[0].IsDynamic, c.wantDyn)
			}
		})
	}
}

// TestParseRecipe_AmmoDefinitionNotDynamic: type-reclassified recipe files
// (ae2:matter_cannon → ammo_definition) must not be marked IsDynamic even
// when they lack ingredients/result.
func TestParseRecipe_AmmoDefinitionNotDynamic(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "data/ae2/recipe/matter_cannon.json",
		`{"type":"ae2:matter_cannon","ammo":{"item":"ae2:matter_ball"},"weight":64}`)
	zw.Close()
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip reader: %v", err)
	}
	item, ok := classifyAndParse(zr.File[0])
	if !ok {
		t.Fatal("classifyAndParse ok = false, want true")
	}
	if item.Kind != "ammo_definition" {
		t.Errorf("Kind = %q, want ammo_definition", item.Kind)
	}
	if item.IsDynamic {
		t.Error("IsDynamic = true, want false for ammo_definition")
	}
}

// TestParseRecipe_VanillaSpecialFlat ensures vanilla book_cloning style
// payloads (flat under data/<ns>/recipe/, not recipe/special/) are dynamic.
func TestParseRecipe_VanillaSpecialFlat(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"minecraft","version":"1.21.1"}`)
	addZipFile(t, zw, "data/minecraft/recipe/book_cloning.json",
		`{"type":"minecraft:crafting_special_bookcloning","category":"misc"}`)
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(ext.Recipes) != 1 {
		t.Fatalf("Recipes len = %d, want 1", len(ext.Recipes))
	}
	r := ext.Recipes[0]
	if !r.IsDynamic {
		t.Fatalf("book_cloning IsDynamic = false, want true")
	}
	if r.ParseError != "" {
		t.Errorf("ParseError = %q, want empty", r.ParseError)
	}
	if r.Key != "minecraft:book_cloning" {
		t.Errorf("Key = %q, want minecraft:book_cloning", r.Key)
	}
	var payload map[string]any
	if err := json.Unmarshal(r.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["type"] != "minecraft:crafting_special_bookcloning" {
		t.Errorf("payload type = %v, want minecraft:crafting_special_bookcloning", payload["type"])
	}
	if ext.Stats.DynamicCount != 1 {
		t.Errorf("Stats.DynamicCount = %d, want 1", ext.Stats.DynamicCount)
	}
}

func TestExtractModContent_ZipBombTooManyEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Create 100001 tiny entries to exceed the 100000 cap.
	for i := 0; i < modContentMaxEntries+1; i++ {
		w, err := zw.Create(fmt.Sprintf("data/testmod/recipe/r%d.json", i))
		if err != nil {
			t.Fatalf("create entry %d: %v", i, err)
		}
		w.Write([]byte(`{"type":"minecraft:crafting_shapeless","ingredients":[],"result":{"id":"testmod:x"}}`))
	}
	zw.Close()
	_, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Fatal("expected error for too many entries, got nil")
	}
	if !errors.Is(err, ErrJarTooManyEntries) {
		t.Errorf("error = %v, want ErrJarTooManyEntries", err)
	}
}

func TestExtractModContent_LargeEntrySkipped(t *testing.T) {
	// Construct a zip with one entry whose uncompressed size exceeds 50MB.
	// We can't easily create a 50MB+ compressed entry, so we verify the
	// UncompressedSize64 check by creating a stored (uncompressed) entry.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipFile(t, zw, "fabric.mod.json", `{"id":"bigmod","version":"1.0"}`)
	// Create a stored entry with >50MB of zeros.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "data/bigmod/recipe/big.json", Method: zip.Store})
	if err != nil {
		t.Fatalf("create header: %v", err)
	}
	chunk := make([]byte, 1024*1024) // 1MB
	for i := 0; i < 51; i++ {       // 51MB > 50MB cap
		w.Write(chunk)
	}
	zw.Close()
	ext, err := ExtractModContent(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	// The big entry should be recorded as a parse error, not silently dropped.
	if ext.Stats.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1 (big entry recorded as error)", ext.Stats.ErrorCount)
	}
	if ext.Stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2 (metadata + big entry)", ext.Stats.TotalFiles)
	}
	if len(ext.Recipes) != 1 {
		t.Fatalf("Recipes len = %d, want 1 (big entry recorded as error item)", len(ext.Recipes))
	}
	if ext.Recipes[0].ParseError == "" {
		t.Error("big recipe ParseError is empty, want non-empty")
	}
}

func TestDeriveKey(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"data/ae2/recipe/misc/facade.json", "ae2:misc/facade"},
		{"assets/ae2/models/item/magic_dirt.json", "ae2:magic_dirt"},
		{"data/ae2/worldgen/structure/meteorite.json", "ae2:meteorite"},
		{"data/ae2/loot_table/blocks/chest.json", "ae2:blocks/chest"},
	}
	for _, c := range cases {
		got := deriveKey(c.path)
		if got != c.want {
			t.Errorf("deriveKey(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
