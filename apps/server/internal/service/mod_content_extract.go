package service

// mod_content_extract.go is the pure extraction layer: given a mod jar
// (zip) byte stream, return all data-driven game content. No SQL, no HTTP,
// no task semantics — those live in mod_content_task.go.

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const (
	modContentMaxJarBytes   = 100 * 1024 * 1024 // 100 MB
	modContentMaxEntries    = 100_000
	modContentMaxEntryBytes = 50 * 1024 * 1024  // 50 MB per entry
	modContentMaxTotalBytes = 500 * 1024 * 1024 // 500 MB total decompressed
)

var (
	// ErrJarTooLarge means the jar exceeds the safety cap.
	ErrJarTooLarge = errors.New("jar too large")
	// ErrJarTooManyEntries means the zip declares more entries than the cap.
	ErrJarTooManyEntries = errors.New("jar too many entries")
	// ErrJarInvalid means the bytes are not a valid zip archive.
	ErrJarInvalid = errors.New("jar invalid")
)

// ExtractedContent is the full result of parsing one mod jar.
type ExtractedContent struct {
	ModID        string
	Version      string
	Loader       string        // fabric / neoforge / forge / quilt / unknown
	Items        []ContentItem // kind=item_model
	Recipes      []ContentItem // kind=recipe
	Structures   []ContentItem // kind=structure
	Worldgen     []ContentItem // kind=worldgen
	LootTables   []ContentItem // kind=loot_table
	Advancements []ContentItem // kind=advancement
	Tags         []ContentItem // kind=tag
	Langs        []ContentItem // kind=lang
	Textures     []ContentItem // kind=texture (PNG icons)
	ItemIcons    []ContentItem // kind=item_icon (rendered inventory icons)
	Metadata     ContentItem   // kind=metadata
	Stats        ExtractStats
}

// ContentItem is one extracted row, ready to persist into mod_content.
type ContentItem struct {
	Kind       string
	Path       string
	Key        string
	Payload    json.RawMessage
	IsDynamic  bool
	ParseError string
}

// ExtractStats summarises the parse run.
type ExtractStats struct {
	TotalFiles   int
	ParsedCount  int
	DynamicCount int
	ErrorCount   int
}

// ExtractModContent parses a mod jar from r and returns all data-driven
// content. Single-entry failures are recorded on the item (ParseError) and
// do not abort the whole run.
func ExtractModContent(r io.Reader) (*ExtractedContent, error) {
	raw, err := io.ReadAll(io.LimitReader(r, modContentMaxJarBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read jar: %w", err)
	}
	if len(raw) > modContentMaxJarBytes {
		return nil, ErrJarTooLarge
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, ErrJarInvalid
	}
	if len(zr.File) > modContentMaxEntries {
		return nil, ErrJarTooManyEntries
	}

	out := &ExtractedContent{}
	var totalDecompressed int64

	for _, f := range zr.File {
		name := f.Name
		// Path traversal / absolute path / Windows drive — skip, do not read.
		if isUnsafeZipPath(name) {
			continue
		}
		if f.UncompressedSize64 > modContentMaxEntryBytes {
			// Record the oversized entry as a parse error rather than silently
			// dropping it, so users can see that content was skipped.
			if kind, ok := kindFromPath(name); ok {
				out.Stats.TotalFiles++
				out.Stats.ErrorCount++
				item := ContentItem{Kind: kind, Path: name, Key: deriveKey(name), ParseError: "entry exceeds 50MB limit"}
				item.Payload = json.RawMessage(`{}`)
				appendContentItem(out, item)
			}
			continue
		}
		if totalDecompressed+int64(f.UncompressedSize64) > modContentMaxTotalBytes {
			break
		}
		totalDecompressed += int64(f.UncompressedSize64)

		item, ok := classifyAndParse(f)
		if !ok {
			continue
		}
		out.Stats.TotalFiles++
		if item.ParseError != "" {
			out.Stats.ErrorCount++
		} else {
			out.Stats.ParsedCount++
		}
		if item.IsDynamic {
			out.Stats.DynamicCount++
		}

		appendContentItem(out, item)
	}

	// Post-process: generate item_icon (inventory-rendered icons) from item_models + textures.
	generateItemIcons(out)

	return out, nil
}

// isUnsafeZipPath rejects zip entry names that could escape the extraction
// root: path traversal (..), absolute paths (Unix or Windows), and Windows
// drive letters. Uses path.Clean so legitimate names like "my..mod" are not
// falsely rejected.
func isUnsafeZipPath(name string) bool {
	cleaned := path.Clean(name)
	if strings.HasPrefix(cleaned, "..") {
		return true
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return true
	}
	if len(name) >= 2 && name[1] == ':' {
		return true
	}
	return false
}

// kindFromPath returns the content kind for a zip entry path without reading
// the file. Returns ok=false for entries outside the data-driven scope.
func kindFromPath(name string) (string, bool) {
	lower := strings.ToLower(name)
	if lower == "fabric.mod.json" || lower == "meta-inf/neoforge.mods.toml" ||
		lower == "meta-inf/mods.toml" || lower == "quilt.mod.json" {
		return "metadata", true
	}
	if !strings.HasPrefix(lower, "data/") && !strings.HasPrefix(lower, "assets/") {
		return "", false
	}
	ext := path.Ext(lower)
	if ext != ".json" && ext != ".nbt" && ext != ".png" {
		return "", false
	}
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/recipe/") || strings.Contains(lower, "/recipes/")) {
		return "recipe", true
	}
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/models/item/") {
		return "item_model", true
	}
	if strings.HasPrefix(lower, "assets/") && (strings.Contains(lower, "/models/block/") || strings.Contains(lower, "/blockstates/")) {
		return "item_model", true
	}
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/lang/") {
		return "lang", true
	}
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/textures/") && ext == ".png" {
		return "texture", true
	}
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/worldgen/structure/") || strings.Contains(lower, "/structures/")) {
		return "structure", true
	}
	if strings.HasPrefix(lower, "data/") && strings.Contains(lower, "/worldgen/") {
		return "worldgen", true
	}
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/loot_table/") || strings.Contains(lower, "/loot_tables/")) {
		return "loot_table", true
	}
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/advancement/") || strings.Contains(lower, "/advancements/")) {
		return "advancement", true
	}
	if strings.HasPrefix(lower, "data/") && strings.Contains(lower, "/tags/") {
		return "tag", true
	}
	return "", false
}

// appendContentItem appends a ContentItem to the correct slice on
// ExtractedContent based on its kind.
func appendContentItem(out *ExtractedContent, item ContentItem) {
	switch item.Kind {
	case "metadata":
		out.Metadata = item
		applyMetadata(out, item)
	case "recipe":
		out.Recipes = append(out.Recipes, item)
	case "item_model":
		out.Items = append(out.Items, item)
	case "structure":
		out.Structures = append(out.Structures, item)
	case "worldgen":
		out.Worldgen = append(out.Worldgen, item)
	case "loot_table":
		out.LootTables = append(out.LootTables, item)
	case "advancement":
		out.Advancements = append(out.Advancements, item)
	case "tag":
		out.Tags = append(out.Tags, item)
	case "lang":
		out.Langs = append(out.Langs, item)
	case "texture":
		out.Textures = append(out.Textures, item)
	case "item_icon":
		out.ItemIcons = append(out.ItemIcons, item)
	}
}

// classifyAndParse maps a zip entry to a ContentItem. Returns ok=false for
// entries outside the data-driven scope (binary assets, META-INF, etc.).
func classifyAndParse(f *zip.File) (ContentItem, bool) {
	name := f.Name
	lower := strings.ToLower(name)

	// Metadata files.
	if lower == "fabric.mod.json" {
		return parseMetadataFile(f, "fabric"), true
	}
	if lower == "meta-inf/neoforge.mods.toml" {
		return parseMetadataFile(f, "neoforge"), true
	}
	if lower == "meta-inf/mods.toml" {
		return parseMetadataFile(f, "forge"), true
	}
	if lower == "quilt.mod.json" {
		return parseMetadataFile(f, "quilt"), true
	}

	// Must be under data/ or assets/ and end with .json (or .nbt for structures).
	if !strings.HasPrefix(lower, "data/") && !strings.HasPrefix(lower, "assets/") {
		return ContentItem{}, false
	}

	ext := path.Ext(lower)
	if ext != ".json" && ext != ".nbt" && ext != ".png" {
		return ContentItem{}, false
	}

	// Recipe.
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/recipe/") || strings.Contains(lower, "/recipes/")) {
		return parseRecipeFile(f), true
	}
	// Item model.
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/models/item/") {
		return simpleItem(f, "item_model"), true
	}
	// Block model (for icon lookup).
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/models/block/") {
		return simpleItem(f, "item_model"), true
	}
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/blockstates/") {
		return simpleItem(f, "item_model"), true
	}
	// Lang files (assets/<namespace>/lang/<locale>.json).
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/lang/") {
		return simpleItem(f, "lang"), true
	}
	// Textures (PNG icons for items/blocks).
	if strings.HasPrefix(lower, "assets/") && strings.Contains(lower, "/textures/") && ext == ".png" {
		return simpleBinaryItem(f, "texture", "image/png"), true
	}
	// Structure (worldgen/structure JSON or structures/ NBT template).
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/worldgen/structure/") || strings.Contains(lower, "/structures/")) {
		return simpleItem(f, "structure"), true
	}
	// Other worldgen (configured_feature, placed_feature, biome, structure_set, etc.).
	if strings.HasPrefix(lower, "data/") && strings.Contains(lower, "/worldgen/") {
		return simpleItem(f, "worldgen"), true
	}
	// Loot table.
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/loot_table/") || strings.Contains(lower, "/loot_tables/")) {
		return simpleItem(f, "loot_table"), true
	}
	// Advancement.
	if strings.HasPrefix(lower, "data/") && (strings.Contains(lower, "/advancement/") || strings.Contains(lower, "/advancements/")) {
		return simpleItem(f, "advancement"), true
	}
	// Tag.
	if strings.HasPrefix(lower, "data/") && strings.Contains(lower, "/tags/") {
		return simpleItem(f, "tag"), true
	}

	return ContentItem{}, false
}

// simpleItem reads a JSON entry and stores its raw payload. Used for all
// content kinds where we only need the id + original JSON (no field-level
// extraction).
func simpleItem(f *zip.File, kind string) ContentItem {
	item := ContentItem{Kind: kind, Path: f.Name, Key: deriveKey(f.Name)}
	data, err := readModContentZipEntry(f)
	if err != nil {
		item.ParseError = err.Error()
		item.Payload = json.RawMessage(`{}`)
		return item
	}
	if !json.Valid(data) {
		item.ParseError = "invalid json"
		item.Payload = json.RawMessage(`{}`)
		return item
	}
	item.Payload = json.RawMessage(data)
	return item
}

// simpleBinaryItem reads a binary entry (PNG texture etc.) and stores it as
// base64 in a JSON wrapper: {"mime":"image/png","data":"<base64>"}.
func simpleBinaryItem(f *zip.File, kind string, mime string) ContentItem {
	item := ContentItem{Kind: kind, Path: f.Name, Key: deriveKey(f.Name)}
	data, err := readModContentZipEntry(f)
	if err != nil {
		item.ParseError = err.Error()
		item.Payload = json.RawMessage(`{}`)
		return item
	}
	wrapped := struct {
		Mime string `json:"mime"`
		Data string `json:"data"`
		Size int    `json:"size"`
	}{
		Mime: mime,
		Data: base64.StdEncoding.EncodeToString(data),
		Size: len(data),
	}
	payload, err := json.Marshal(wrapped)
	if err != nil {
		item.ParseError = "marshal failed"
		item.Payload = json.RawMessage(`{}`)
		return item
	}
	item.Payload = payload
	return item
}
func parseRecipeFile(f *zip.File) ContentItem {
	item := ContentItem{Kind: "recipe", Path: f.Name, Key: deriveKey(f.Name)}
	data, err := readModContentZipEntry(f)
	if err != nil {
		item.ParseError = err.Error()
		item.Payload = json.RawMessage(`{}`)
		return item
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		item.ParseError = "invalid json: " + err.Error()
		item.Payload = json.RawMessage(`{}`)
		return item
	}

	// Classify by type: some files under /recipe/ are not crafting recipes.
	// ae2:matter_cannon = ammo property definitions, not recipes.
	if typ, ok := raw["type"].(string); ok {
		switch typ {
		case "ae2:matter_cannon":
			item.Kind = "ammo_definition"
		}
	}

	// Dynamic detection (criterion A): recipe under recipe/special/ with only
	// a type declaration (no ingredients / result / output).
	isSpecialPath := strings.Contains(strings.ToLower(f.Name), "/recipe/special/")
	_, hasIngredients := raw["ingredients"]
	_, hasIngredient := raw["ingredient"]
	_, hasResult := raw["result"]
	_, hasOutput := raw["output"]
	hasContent := hasIngredients || hasIngredient || hasResult || hasOutput
	if isSpecialPath && !hasContent {
		item.IsDynamic = true
		// Store only the type declaration as payload.
		typ, _ := raw["type"].(string)
		dynPayload, _ := json.Marshal(map[string]string{"type": typ})
		item.Payload = json.RawMessage(dynPayload)
		return item
	}

	// Standard or custom static recipe: keep full original JSON.
	item.Payload = json.RawMessage(data)
	return item
}

// parseMetadataFile reads a mod metadata descriptor (fabric.mod.json or
// neoforge/forge mods.toml) and returns a metadata ContentItem.
func parseMetadataFile(f *zip.File, loader string) ContentItem {
	item := ContentItem{Kind: "metadata", Path: f.Name, Key: loader + ":metadata"}
	data, err := readModContentZipEntry(f)
	if err != nil {
		item.ParseError = err.Error()
		item.Payload = json.RawMessage(`{}`)
		return item
	}

	var modid, version string
	if loader == "fabric" || loader == "quilt" {
		var meta map[string]any
		if err := json.Unmarshal(data, &meta); err != nil {
			item.ParseError = "invalid json: " + err.Error()
			item.Payload = json.RawMessage(`{}`)
			return item
		}
		modid, _ = meta["id"].(string)
		version, _ = meta["version"].(string)
		if version == "" {
			// Fabric sometimes nests version under a string or ${version}.
			version = "unknown"
		}
	} else {
		// neoforge.mods.toml / mods.toml: lightweight regex extraction, no
		// third-party toml dependency.
		modid = extractTomlString(data, `modId\s*=\s*"([^"]+)"`)
		version = extractTomlString(data, `version\s*=\s*"([^"]+)"`)
		if version == "" {
			version = extractTomlString(data, `version\s*=\s*'([^']+)'`)
		}
	}

	payload, _ := json.Marshal(map[string]string{
		"loader":  loader,
		"modid":   modid,
		"version": version,
	})
	item.Payload = json.RawMessage(payload)
	return item
}

// applyMetadata copies modid/version/loader from a metadata item onto the
// top-level ExtractedContent fields.
func applyMetadata(out *ExtractedContent, item ContentItem) {
	var meta map[string]string
	if err := json.Unmarshal(item.Payload, &meta); err != nil {
		return
	}
	if out.ModID == "" {
		out.ModID = meta["modid"]
	}
	if out.Version == "" {
		out.Version = meta["version"]
	}
	if out.Loader == "" {
		out.Loader = meta["loader"]
	}
}

// deriveKey converts a jar-relative path to a namespaced content id.
// data/ae2/recipe/misc/facade.json -> ae2:misc/facade
// assets/ae2/models/item/magic_dirt.json -> ae2:magic_dirt
func deriveKey(p string) string {
	parts := strings.SplitN(p, "/", 3)
	if len(parts) < 3 {
		return p
	}
	namespace := parts[1]
	rest := parts[2]
	// Strip the category prefix (recipe/, models/item/, worldgen/structure/, etc.).
	rest = stripCategoryPrefix(rest)
	// Strip extension.
	if ext := path.Ext(rest); ext != "" {
		rest = strings.TrimSuffix(rest, ext)
	}
	return namespace + ":" + rest
}

// stripCategoryPrefix removes the first path component after namespace if it
// is a known content category (recipe, models, worldgen, loot_table, etc.).
func stripCategoryPrefix(rest string) string {
	categories := []string{
		"recipe/", "recipes/",
		"models/item/", "models/block/",
		"lang/",
		"worldgen/structure/", "worldgen/configured_feature/", "worldgen/placed_feature/",
		"worldgen/biome/", "worldgen/structure_set/", "worldgen/noise_settings/",
		"worldgen/density_function/", "worldgen/noise/", "worldgen/configured_carver/",
		"worldgen/processor_list/", "worldgen/template_pool/", "worldgen/flat_level_generator_preset/",
		"worldgen/world_preset/", "worldgen/",
		"structures/",
		"loot_table/", "loot_tables/",
		"advancement/", "advancements/",
		"tags/items/", "tags/blocks/", "tags/fluids/", "tags/entity_types/", "tags/functions/",
		"tags/",
	}
	for _, c := range categories {
		if strings.HasPrefix(rest, c) {
			return strings.TrimPrefix(rest, c)
		}
	}
	return rest
}

// readZipEntry reads a zip file entry into bytes, with size cap.
func readModContentZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open entry: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, modContentMaxEntryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read entry: %w", err)
	}
	if len(data) > modContentMaxEntryBytes {
		return nil, errors.New("entry too large")
	}
	return data, nil
}

// tomlStringRe is compiled once at package init.
var tomlStringRe = regexp.MustCompile

// extractTomlString runs a named-capture regex against TOML bytes and returns
// the first capture group.
func extractTomlString(data []byte, pattern string) string {
	re := tomlStringRe(pattern)
	m := re.FindSubmatch(data)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// generateItemIcons resolves item model inheritance without confusing block IDs.
func generateItemIcons(out *ExtractedContent) {
	r := newIconResources()
	r.addContent(out)
	out.ItemIcons, _ = r.icons()
}
