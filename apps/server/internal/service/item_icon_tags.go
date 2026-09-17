package service

import (
	"encoding/json"
	"strings"
)

func itemTagID(p string) string {
	parts := strings.SplitN(p, "/", 3)
	if len(parts) != 3 || parts[0] != "data" || !strings.HasSuffix(p, ".json") {
		return ""
	}
	for _, prefix := range []string{"tags/item/", "tags/items/"} {
		if strings.HasPrefix(parts[2], prefix) {
			return parts[1] + ":" + strings.TrimSuffix(strings.TrimPrefix(parts[2], prefix), ".json")
		}
	}
	return ""
}
func (r *iconResources) addTag(path string, raw []byte) {
	id := itemTagID(path)
	if id == "" {
		return
	}
	var p struct {
		Values  []json.RawMessage
		Replace bool
	}
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	if p.Replace {
		r.tags[id] = nil
	}
	r.tags[id] = append(r.tags[id], p.Values...)
}

// These are representative members, not a replacement for runtime tag sets.
// Verified against NeoForge 1.21.1 src/generated/resources/data/c/tags/item/
// {ingots/iron,dusts/redstone,gems/diamond}.json. Other versions are not guessed.
func (r *iconResources) addCommonTagRepresentatives(version string) {
	if version != "1.21.1" {
		return
	}
	for tag, item := range map[string]string{"c:ingots/iron": "minecraft:iron_ingot", "c:dusts/redstone": "minecraft:redstone", "c:gems/diamond": "minecraft:diamond"} {
		raw, _ := json.Marshal(item)
		r.tags[tag] = append(r.tags[tag], raw)
	}
}
func (r *iconResources) tagIcons(icons []ContentItem) map[string]string {
	available := map[string]bool{}
	for _, icon := range icons {
		available[icon.Key] = true
	}
	var resolve func(string, map[string]bool) string
	resolve = func(id string, seen map[string]bool) string {
		if seen[id] || len(seen) >= 32 {
			return ""
		}
		seen[id] = true
		defer delete(seen, id)
		for _, raw := range r.tags[id] {
			var member string
			if json.Unmarshal(raw, &member) != nil {
				var p struct{ ID string }
				if json.Unmarshal(raw, &p) != nil {
					continue
				}
				member = p.ID
			}
			if strings.HasPrefix(member, "#") {
				if found := resolve(resourceID(strings.TrimPrefix(member, "#")), seen); found != "" {
					return found
				}
			} else if available[resourceID(member)] {
				return resourceID(member)
			}
		}
		return ""
	}
	result := map[string]string{}
	for tag := range r.tags {
		if id := resolve(tag, map[string]bool{}); id != "" {
			result["#"+tag] = id
		}
	}
	return result
}
