package service

import (
	"path"
	"regexp"
	"strings"
)

// Datapack layout: data/<namespace>/<category>/...
// Recipe unlocks live under advancement/recipes/** and must NOT be recipes.
var (
	reDataCategory = regexp.MustCompile(`^data/[^/]+/([^/]+)/`)
	assetCategory  = regexp.MustCompile(`^assets/[^/]+/([^/]+)/`)
)

// datapackCategory returns the first path segment after the namespace for a
// data/ or assets/ entry (e.g. "recipe", "advancement", "models", "lang").
func datapackCategory(lowerPath string) string {
	if m := reDataCategory.FindStringSubmatch(lowerPath); m != nil {
		return m[1]
	}
	if m := assetCategory.FindStringSubmatch(lowerPath); m != nil {
		return m[1]
	}
	return ""
}

// isUnderCategory reports whether the datapack category equals any of cats.
// assets/models/item and assets/models/block need a secondary segment check.
func isUnderCategory(lowerPath string, cats ...string) bool {
	c := datapackCategory(lowerPath)
	for _, want := range cats {
		if c == want {
			return true
		}
	}
	return false
}

func isItemOrBlockModel(lowerPath string) bool {
	if !strings.HasPrefix(lowerPath, "assets/") {
		return false
	}
	return strings.Contains(lowerPath, "/models/item/") ||
		strings.Contains(lowerPath, "/models/block/") ||
		strings.Contains(lowerPath, "/blockstates/")
}

// kindFromPathClassify is the authoritative path→kind mapper used by both
// kindFromPath and classifyAndParse. Category is taken from the segment after
// the namespace so advancement/recipes/** stays advancement.
func kindFromPathClassify(lower string) (kind string, ext string, ok bool) {
	if lower == "fabric.mod.json" || lower == "meta-inf/neoforge.mods.toml" ||
		lower == "meta-inf/mods.toml" || lower == "quilt.mod.json" {
		return "metadata", path.Ext(lower), true
	}
	if !strings.HasPrefix(lower, "data/") && !strings.HasPrefix(lower, "assets/") {
		return "", "", false
	}
	ext = path.Ext(lower)
	if ext != ".json" && ext != ".nbt" && ext != ".png" {
		return "", "", false
	}

	// assets first (models/lang/textures live only under assets/)
	if strings.HasPrefix(lower, "assets/") {
		switch {
		case isItemOrBlockModel(lower):
			return "item_model", ext, true
		case isUnderCategory(lower, "lang"):
			return "lang", ext, true
		case isUnderCategory(lower, "textures") && ext == ".png":
			return "texture", ext, true
		default:
			return "", "", false
		}
	}

	// data/<ns>/<category>/...
	cat := datapackCategory(lower)
	switch cat {
	case "recipe", "recipes":
		return "recipe", ext, true
	case "advancement", "advancements":
		// Includes advancement/recipes/** — recipe unlock advancements.
		return "advancement", ext, true
	case "loot_table", "loot_tables":
		return "loot_table", ext, true
	case "structures":
		return "structure", ext, true
	case "worldgen":
		// worldgen/structure/** is structure; other worldgen/* stay worldgen.
		if strings.Contains(lower, "/worldgen/structure/") {
			return "structure", ext, true
		}
		return "worldgen", ext, true
	case "tags":
		return "tag", ext, true
	default:
		return "", "", false
	}
}
