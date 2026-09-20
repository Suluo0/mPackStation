package service

import "testing"

func TestKindFromPath_DoesNotClassifyAdvancementRecipesAsRecipe(t *testing.T) {
	cases := []struct {
		path string
		kind string
		ok   bool
	}{
		// Recipe unlock advancements are NOT crafting recipes.
		{"data/minecraft/advancement/recipes/brewing/blaze_powder.json", "advancement", true},
		{"data/ae2/advancements/recipes/misc/certus_quartz.json", "advancement", true},
		// Real recipes.
		{"data/minecraft/recipe/blaze_powder.json", "recipe", true},
		{"data/minecraft/recipes/crafting_table.json", "recipe", true},
		{"data/ae2/recipe/misc/fluix_crystal.json", "recipe", true},
		// Other datapack kinds.
		{"data/minecraft/loot_table/blocks/chest.json", "loot_table", true},
		{"data/minecraft/advancement/adventure/kill_a_mob.json", "advancement", true},
		{"data/minecraft/tags/items/dusts.json", "tag", true},
		{"data/minecraft/worldgen/configured_feature/ore_diamond.json", "worldgen", true},
		{"data/minecraft/worldgen/structure/village_plains.json", "structure", true},
		{"data/minecraft/structures/village/plains/house.nbt", "structure", true},
		// Assets.
		{"assets/minecraft/models/item/diamond.json", "item_model", true},
		{"assets/minecraft/lang/en_us.json", "lang", true},
		{"assets/minecraft/textures/item/diamond.png", "texture", true},
		// Out of scope.
		{"META-INF/MANIFEST.MF", "", false},
		{"assets/minecraft/sounds.json", "", false},
	}
	for _, c := range cases {
		got, ok := kindFromPath(c.path)
		if ok != c.ok || got != c.kind {
			t.Errorf("kindFromPath(%q) = (%q, %v), want (%q, %v)", c.path, got, ok, c.kind, c.ok)
		}
	}
}
