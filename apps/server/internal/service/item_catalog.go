package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mpackstation/internal/store"
)

var catalogIDPattern = regexp.MustCompile(`^[a-z0-9_.-]+:[a-z0-9_./-]+$`)
var catalogLocalePattern = regexp.MustCompile(`^[a-z]{2,3}_[a-z0-9]{2,8}$`)

// catalogResolverVersion 标记目录是哪一版解析规则产出的。v3 把物品条目从
// 「models/item 下有这个文件」改成「语言键/被引用/方块的物品形态」；v4 再加一道收口：
// 光有语言键不算物品，必须同时有模型资产或被配方/标签引用。
const catalogResolverVersion = "catalog-v4"

func catalogLocale(value string) string {
	if value == "" {
		return "zh_cn"
	}
	return strings.ToLower(strings.ReplaceAll(value, "-", "_"))
}

func catalogPathID(name, top string, directories ...string) (string, bool) {
	name = filepath.ToSlash(name)
	parts := strings.Split(name, "/")
	if len(parts) < 4 || parts[0] != top || !strings.HasSuffix(name, ".json") {
		return "", false
	}
	for _, directory := range directories {
		prefix := top + "/" + parts[1] + "/" + directory + "/"
		if strings.HasPrefix(name, prefix) {
			id := parts[1] + ":" + strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
			return id, catalogIDPattern.MatchString(id)
		}
	}
	return "", false
}

// langKeyToID 把语言键的后半段（去掉 item./block. 前缀）转成资源 ID：
// "minecraft.apple" → "minecraft:apple"。命名空间不含点，所以路径里再出现点就不是
// 注册表条目，而是文案子键（block.minecraft.banner.base.creeper 这类），一律拒绝——
// 它们此前被当成物品/方块建了 890 行。
func langKeyToID(rest string) string {
	ns, path, ok := strings.Cut(rest, ".")
	if !ok || path == "" || strings.Contains(path, ".") {
		return ""
	}
	return resourceID(ns + ":" + path)
}

func catalogTagPath(name string) (registry, id string, ok bool) {
	if id, ok = catalogPathID(name, "data", "tags/item", "tags/items"); ok {
		return "item", id, true
	}
	if id, ok = catalogPathID(name, "data", "tags/block", "tags/blocks"); ok {
		return "block", id, true
	}
	return "", "", false
}

func isCatalogResourcePath(name string) bool {
	if _, ok := catalogPathID(name, "assets", "models/item", "models/block", "blockstates", "lang"); ok {
		return true
	}
	if _, _, ok := catalogTagPath(name); ok {
		return true
	}
	_, ok := catalogPathID(name, "data", "recipe", "recipes")
	return ok
}

type catalogResource struct {
	source, path string
	payload      json.RawMessage
}
type catalogLanguage struct{ value, source string }
type catalogBuilder struct {
	items   map[string]*store.CatalogItem
	blocks  map[string]*store.CatalogBlock
	tags    map[string]*store.CatalogTag
	recipes map[string]*store.CatalogRecipe
	langs   map[string]map[string]catalogLanguage
	// models 只是图标/材质的来源，不再凭空产生物品条目；收集起来在 finish 阶段
	// 给已确立资格的物品回填 model_path。
	models map[string]string
	// referenced 记录被配方或标签点名过的 ID，与 items 里的 evidence 字段分开维护，
	// 因为语言键先到时会把 evidence 锁在 lang 上，引用信息就丢了。
	referenced map[string]bool
}

func newCatalogBuilder() *catalogBuilder {
	return &catalogBuilder{items: map[string]*store.CatalogItem{}, blocks: map[string]*store.CatalogBlock{}, tags: map[string]*store.CatalogTag{}, recipes: map[string]*store.CatalogRecipe{}, langs: map[string]map[string]catalogLanguage{}, models: map[string]string{}, referenced: map[string]bool{}}
}

func (b *catalogBuilder) item(id, evidence, source, modelPath string) {
	id = resourceID(id)
	if !catalogIDPattern.MatchString(id) {
		return
	}
	if evidence == "reference" {
		b.referenced[id] = true
	}
	if old := b.items[id]; old != nil {
		if old.Evidence == "lang" {
			return // 语言键是权威，任何来源都不覆盖
		}
		if old.Evidence == "reference" && (evidence == "model" || evidence == "lang") {
			old.Evidence, old.Source = evidence, source
		}
		if evidence != "lang" && old.ModelPath == "" {
			old.ModelPath = modelPath
		}
		return
	}
	b.items[id] = &store.CatalogItem{ID: id, Evidence: evidence, Source: source, ModelPath: modelPath, IconStatus: "pending", Names: []store.CatalogName{}, Tags: []string{}}
}

func (b *catalogBuilder) block(id, evidence, source, blockstatePath string) {
	id = resourceID(id)
	if !catalogIDPattern.MatchString(id) {
		return
	}
	if old := b.blocks[id]; old != nil {
		if old.Evidence != "blockstate" && evidence == "blockstate" {
			old.Evidence, old.Source, old.BlockstatePath = evidence, source, blockstatePath
		}
		return
	}
	b.blocks[id] = &store.CatalogBlock{ID: id, Evidence: evidence, Source: source, BlockstatePath: blockstatePath, Names: []store.CatalogName{}, Tags: []string{}, ItemIDs: []string{}}
}

func tagMapKey(registry, id string) string { return registry + "\x00" + id }
func (b *catalogBuilder) tag(registry, id string) *store.CatalogTag {
	id = resourceID(id)
	if (registry != "item" && registry != "block") || !catalogIDPattern.MatchString(id) {
		return nil
	}
	key := tagMapKey(registry, id)
	if b.tags[key] == nil {
		b.tags[key] = &store.CatalogTag{Registry: registry, ID: id, Status: "missing", Diagnostics: []string{}, Names: []store.CatalogName{}, Definitions: []store.CatalogDefinition{}, Entries: []store.CatalogTagEntry{}, Members: []string{}}
	}
	return b.tags[key]
}

func (b *catalogBuilder) add(resource catalogResource) {
	if id, ok := catalogPathID(resource.path, "assets", "models/item"); ok {
		b.models[id] = resource.path
		return
	}
	if id, ok := catalogPathID(resource.path, "assets", "blockstates"); ok {
		b.block(id, "blockstate", resource.source, resource.path)
		return
	}
	if _, ok := catalogPathID(resource.path, "assets", "models/block"); ok {
		return
	}
	if id, ok := catalogPathID(resource.path, "assets", "lang"); ok {
		locale := catalogLocale(strings.TrimPrefix(strings.SplitN(id, ":", 2)[1], "lang/"))
		if !catalogLocalePattern.MatchString(locale) {
			return
		}
		var values map[string]string
		if json.Unmarshal(resource.payload, &values) != nil {
			return
		}
		if b.langs[locale] == nil {
			b.langs[locale] = map[string]catalogLanguage{}
		}
		for key, value := range values {
			if value != "" {
				b.langs[locale][key] = catalogLanguage{value: value, source: resource.source}
			}
			// 语言键只有 item. 这一支产物品行（物品的权威来源）。
			// block. 键在这里只当名字用：方块行由 blockstates/*.json 产生，
			// 方块的物品形态在 finish 里按「有 blockstate + 有物品模型 + 有名字键」判定。
			if rest, ok := strings.CutPrefix(key, "item."); ok {
				if id := langKeyToID(rest); id != "" {
					b.item(id, "lang", resource.source, "")
				}
			}
		}
		return
	}
	if registry, id, ok := catalogTagPath(resource.path); ok {
		if tag := b.tag(registry, id); tag != nil {
			tag.Status = "resolved"
			tag.Definitions = append(tag.Definitions, store.CatalogDefinition{Source: resource.source, Path: resource.path, Payload: resource.payload})
		}
		return
	}
	if id, ok := catalogPathID(resource.path, "data", "recipe", "recipes"); ok {
		b.addRecipe(id, resource)
		return
	}
}

type catalogEdge struct {
	id                    string
	tag, required, remove bool
	definition, position  int
}

func catalogEntry(raw json.RawMessage, remove bool) (catalogEdge, bool) {
	var id string
	required := true
	if json.Unmarshal(raw, &id) != nil {
		var object struct {
			ID       string `json:"id"`
			Required *bool  `json:"required"`
		}
		if json.Unmarshal(raw, &object) != nil {
			return catalogEdge{}, false
		}
		id = object.ID
		if object.Required != nil {
			required = *object.Required
		}
	}
	isTag := strings.HasPrefix(id, "#")
	id = resourceID(strings.TrimPrefix(id, "#"))
	return catalogEdge{id: id, tag: isTag, required: required, remove: remove}, catalogIDPattern.MatchString(id)
}

func catalogIngredient(raw json.RawMessage) []store.CatalogRecipeRef {
	var alternatives []json.RawMessage
	if json.Unmarshal(raw, &alternatives) != nil {
		alternatives = []json.RawMessage{raw}
	}
	result := []store.CatalogRecipeRef{}
	for index, alternative := range alternatives {
		var direct string
		if json.Unmarshal(alternative, &direct) == nil {
			kind := "item"
			if strings.HasPrefix(direct, "#") {
				kind = "item_tag"
				direct = strings.TrimPrefix(direct, "#")
			}
			direct = resourceID(direct)
			if catalogIDPattern.MatchString(direct) {
				result = append(result, store.CatalogRecipeRef{Kind: kind, ID: direct, Alternative: index, Count: 1})
			}
			continue
		}
		var object struct {
			Item, ID, Tag string
			Count         int
		}
		if json.Unmarshal(alternative, &object) != nil {
			continue
		}
		kind, id := "item", object.Item
		if id == "" {
			id = object.ID
		}
		if object.Tag != "" {
			kind, id = "item_tag", object.Tag
		}
		id = resourceID(strings.TrimPrefix(id, "#"))
		if object.Count <= 0 {
			object.Count = 1
		}
		if catalogIDPattern.MatchString(id) {
			result = append(result, store.CatalogRecipeRef{Kind: kind, ID: id, Alternative: index, Count: object.Count})
		}
	}
	return result
}

func (b *catalogBuilder) addRecipe(id string, resource catalogResource) {
	recipe := &store.CatalogRecipe{ID: id, Source: resource.source, Path: resource.path, Payload: resource.payload, Status: "parsed", Diagnostics: []string{}, Refs: []store.CatalogRecipeRef{}}
	var root map[string]json.RawMessage
	if json.Unmarshal(resource.payload, &root) != nil {
		recipe.Status = "invalid"
		recipe.Diagnostics = append(recipe.Diagnostics, "配方 JSON 格式错误")
		b.recipes[id] = recipe
		return
	}
	_ = json.Unmarshal(root["type"], &recipe.Type)
	add := func(role string, slot int, raw json.RawMessage) {
		for _, ref := range catalogIngredient(raw) {
			ref.Role, ref.Slot = role, slot
			recipe.Refs = append(recipe.Refs, ref)
			if ref.Kind == "item" {
				b.item(ref.ID, "reference", resource.source, "")
			} else {
				b.tag("item", ref.ID)
			}
		}
	}
	switch recipe.Type {
	case "minecraft:crafting_shaped":
		var key map[string]json.RawMessage
		var pattern []string
		_ = json.Unmarshal(root["key"], &key)
		_ = json.Unmarshal(root["pattern"], &pattern)
		for row, line := range pattern {
			for column, symbol := range line {
				if symbol != ' ' {
					add("input", row*3+column, key[string(symbol)])
				}
			}
		}
	case "minecraft:crafting_shapeless", "ae2:transform":
		var ingredients []json.RawMessage
		_ = json.Unmarshal(root["ingredients"], &ingredients)
		for slot, ingredient := range ingredients {
			add("input", slot, ingredient)
		}
	case "minecraft:smelting", "minecraft:blasting", "minecraft:smoking", "minecraft:campfire_cooking", "minecraft:stonecutting":
		add("input", 0, root["ingredient"])
	case "minecraft:smithing_transform", "minecraft:smithing_trim":
		for slot, key := range []string{"template", "base", "addition"} {
			add("input", slot, root[key])
		}
	case "minecraft:crafting_transmute":
		add("input", 0, root["input"])
		add("input", 1, root["material"])
	default:
		recipe.Status = "unsupported"
		recipe.Diagnostics = append(recipe.Diagnostics, "保留了原始定义，但尚未结构化此配方类型")
	}
	if raw, exists := root["result"]; exists {
		add("output", 0, raw)
	}
	b.recipes[id] = recipe
}

func catalogDiagnostic(tag *store.CatalogTag, message string) {
	for _, old := range tag.Diagnostics {
		if old == message {
			return
		}
	}
	tag.Diagnostics = append(tag.Diagnostics, message)
	if tag.Status == "resolved" {
		tag.Status = "partial"
	}
}

// blocksWithoutItemForm 是资产齐全（blockstate + item 模型 + 名字键都有）、游戏里却没有
// 物品形态的原版方块：注册表里没给它们登记 Item，/give 给不出、JEI 也不显示。
// assets 层看不出「有没有 Item」，只能按实测清单钉死。
var blocksWithoutItemForm = map[string]bool{"minecraft:air": true, "minecraft:structure_void": true, "minecraft:light": true}

// hasLangKey 报告任一语言文件里出现过这个键。
func (b *catalogBuilder) hasLangKey(key string) bool {
	for _, values := range b.langs {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func (b *catalogBuilder) names(keys ...string) []store.CatalogName {
	result := []store.CatalogName{}
	for locale, values := range b.langs {
		for _, key := range keys {
			if value, ok := values[key]; ok {
				result = append(result, store.CatalogName{Locale: locale, Name: value.value, Key: key, Source: value.source})
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Locale < result[j].Locale })
	return result
}

func (b *catalogBuilder) finish() store.Catalog {
	edges := map[string][]catalogEdge{}
	for key, tag := range b.tags {
		for definitionIndex := range tag.Definitions {
			definition := &tag.Definitions[definitionIndex]
			var raw struct {
				Values, Remove     []json.RawMessage
				Replace            bool
				Neo, Forge, Fabric json.RawMessage
			}
			var object map[string]json.RawMessage
			if json.Unmarshal(definition.Payload, &object) != nil {
				tag.Status = "invalid"
				catalogDiagnostic(tag, "标签定义格式错误")
				continue
			}
			_ = json.Unmarshal(object["values"], &raw.Values)
			_ = json.Unmarshal(object["remove"], &raw.Remove)
			_ = json.Unmarshal(object["replace"], &raw.Replace)
			conditions := []json.RawMessage{}
			for _, conditionKey := range []string{"neoforge:conditions", "forge:conditions", "fabric:load_conditions"} {
				if value := object[conditionKey]; len(value) > 0 {
					conditions = append(conditions, value)
				}
			}
			definition.Replace = raw.Replace
			definition.Conditions, _ = json.Marshal(conditions)
			if len(conditions) > 0 {
				catalogDiagnostic(tag, "包含运行时加载条件，当前定义未展开")
				continue
			}
			if len(tag.Definitions) > 1 && (raw.Replace || len(raw.Remove) > 0) {
				catalogDiagnostic(tag, "多来源覆盖或移除依赖实际加载顺序；当前按稳定来源顺序展开")
			}
			if raw.Replace {
				edges[key] = nil
			}
			position := 0
			for _, part := range []struct {
				values []json.RawMessage
				remove bool
			}{{raw.Values, false}, {raw.Remove, true}} {
				for _, value := range part.values {
					edge, ok := catalogEntry(value, part.remove)
					if !ok {
						tag.Status = "invalid"
						catalogDiagnostic(tag, "无效的成员标识")
						continue
					}
					edge.definition, edge.position = definitionIndex, position
					position++
					edges[key] = append(edges[key], edge)
					operation, targetKind := "add", "member"
					if edge.remove {
						operation = "remove"
					}
					if edge.tag {
						targetKind = "tag"
					}
					tag.Entries = append(tag.Entries, store.CatalogTagEntry{Definition: definitionIndex, Position: edge.position, TargetKind: targetKind, TargetID: edge.id, Required: edge.required, Operation: operation})
					if !edge.tag && !edge.remove {
						if tag.Registry == "item" {
							b.item(edge.id, "reference", definition.Source, "")
						} else {
							b.block(edge.id, "reference", definition.Source, "")
						}
					}
				}
			}
		}
	}
	for key, values := range edges {
		registry := b.tags[key].Registry
		for _, edge := range values {
			if edge.tag {
				b.tag(registry, edge.id)
			}
		}
	}
	state := map[string]int{}
	var visit func(string, int) map[string]bool
	visit = func(key string, depth int) map[string]bool {
		tag := b.tags[key]
		result := map[string]bool{}
		if state[key] == 1 || depth > 128 {
			tag.Status = "invalid"
			catalogDiagnostic(tag, "标签循环引用或嵌套过深")
			return result
		}
		if state[key] == 2 {
			for _, member := range tag.Members {
				result[member] = true
			}
			return result
		}
		state[key] = 1
		for _, edge := range edges[key] {
			members := map[string]bool{}
			if edge.tag {
				childKey := tagMapKey(tag.Registry, edge.id)
				child := b.tags[childKey]
				members = visit(childKey, depth+1)
				if child.Status != "resolved" && edge.required {
					catalogDiagnostic(tag, "引用未完整解析的标签 #"+edge.id)
					if child.Status == "invalid" {
						tag.Status = "invalid"
					}
				}
			} else if tag.Registry == "item" {
				if b.items[edge.id] != nil {
					members[edge.id] = true
				}
			} else if b.blocks[edge.id] != nil {
				members[edge.id] = true
			}
			for member := range members {
				if edge.remove {
					delete(result, member)
				} else {
					result[member] = true
				}
			}
		}
		if tag.Status == "invalid" {
			result = map[string]bool{}
		}
		for member := range result {
			tag.Members = append(tag.Members, member)
		}
		sort.Strings(tag.Members)
		state[key] = 2
		return result
	}
	tagKeys := make([]string, 0, len(b.tags))
	for key := range b.tags {
		tagKeys = append(tagKeys, key)
	}
	sort.Strings(tagKeys)
	for _, key := range tagKeys {
		visit(key, 0)
	}
	catalog := store.Catalog{Items: []store.CatalogItem{}, Blocks: []store.CatalogBlock{}, Tags: []store.CatalogTag{}, Recipes: []store.CatalogRecipe{}, Warnings: []string{}}
	for _, key := range tagKeys {
		tag := b.tags[key]
		dotted := strings.NewReplacer(":", ".", "/", ".").Replace(tag.ID)
		tag.Names = b.names("tag." + tag.Registry + "." + dotted)
		for _, member := range tag.Members {
			if tag.Registry == "item" && b.items[member] != nil {
				b.items[member].Tags = append(b.items[member].Tags, tag.ID)
			}
			if tag.Registry == "block" && b.blocks[member] != nil {
				b.blocks[member].Tags = append(b.blocks[member].Tags, tag.ID)
			}
		}
		catalog.Tags = append(catalog.Tags, *tag)
	}
	// 方块的物品形态：有 blockstate（方块行的唯一来源）、有 models/item 同名模型、
	// 有干净的 block 语言键给名字，三者同时成立才算「能被拿在手里的方块」。
	// 原版实测 air / structure_void / light 三条资产齐全却没有物品形态
	// （/give 给不出、JEI 不显示），按清单排除；water、lava、fire、cave_air、
	// void_air、moving_piston 这些同样无物品形态的方块没有 item 模型，天然落在规则外。
	for id, block := range b.blocks {
		if _, excluded := blocksWithoutItemForm[id]; excluded {
			continue
		}
		modelPath := b.models[id]
		dotted := strings.ReplaceAll(id, ":", ".")
		if modelPath == "" || (!b.hasLangKey("block."+dotted) && !b.hasLangKey("block."+strings.ReplaceAll(dotted, "/", "."))) {
			continue
		}
		b.item(id, "lang", block.Source, modelPath)
	}
	for id, item := range b.items {
		dotted := strings.ReplaceAll(id, ":", ".")
		item.Names = b.names("item."+dotted, "block."+dotted, "item."+strings.ReplaceAll(dotted, "/", "."), "block."+strings.ReplaceAll(dotted, "/", "."))
		if item.ModelPath == "" {
			// 模型只是图标的来源（0025）：物品由语言键/引用产生，路径在这里回填
			item.ModelPath = b.models[id]
		}
		if item.ModelPath == "" && !b.referenced[id] {
			// 条目资格到此收口：光有语言键不算物品。Mojang 的语言表比版本超前且从不删键
			// （1.21.1 的 zh_cn 里有 bundle/cushion/spear、pottery_shard 旧拼写、
			// modifiers.head、op_block_warning.line1 这类 163 个键），它们在 jar 里
			// 既没有模型资产也没被任何配方/标签引用，游戏里给不出、JEI 不显示。
			continue
		}
		if block := b.blocks[id]; block != nil {
			block.ItemIDs = append(block.ItemIDs, id)
		}
		catalog.Items = append(catalog.Items, *item)
	}
	for id, block := range b.blocks {
		dotted := strings.ReplaceAll(id, ":", ".")
		// 方块名优先取 block. 键；原版少数方块（glow_item_frame）只在 item. 键下有文案。
		block.Names = b.names("block."+dotted, "block."+strings.ReplaceAll(dotted, "/", "."), "item."+dotted, "item."+strings.ReplaceAll(dotted, "/", "."))
		catalog.Blocks = append(catalog.Blocks, *block)
	}
	for _, recipe := range b.recipes {
		catalog.Recipes = append(catalog.Recipes, *recipe)
	}
	sort.Slice(catalog.Items, func(i, j int) bool { return catalog.Items[i].ID < catalog.Items[j].ID })
	sort.Slice(catalog.Blocks, func(i, j int) bool { return catalog.Blocks[i].ID < catalog.Blocks[j].ID })
	sort.Slice(catalog.Recipes, func(i, j int) bool { return catalog.Recipes[i].ID < catalog.Recipes[j].ID })
	return catalog
}

type CatalogItemView struct {
	ID             string              `json:"id"`
	DisplayName    string              `json:"displayName"`
	ResolvedLocale string              `json:"resolvedLocale"`
	Evidence       string              `json:"evidence"`
	ModelPath      string              `json:"modelPath"`
	IconStatus     string              `json:"iconStatus"`
	IconReason     string              `json:"iconReason"`
	Names          []store.CatalogName `json:"names"`
	Tags           []string            `json:"tags"`
}
type CatalogBlockView struct {
	ID             string              `json:"id"`
	DisplayName    string              `json:"displayName"`
	ResolvedLocale string              `json:"resolvedLocale"`
	Evidence       string              `json:"evidence"`
	BlockstatePath string              `json:"blockstatePath"`
	Names          []store.CatalogName `json:"names"`
	Tags           []string            `json:"tags"`
	ItemIDs        []string            `json:"itemIds"`
}
type CatalogTagView struct {
	Registry       string   `json:"registry"`
	ID             string   `json:"id"`
	DisplayName    string   `json:"displayName"`
	ResolvedLocale string   `json:"resolvedLocale"`
	Status         string   `json:"status"`
	LabelSource    string   `json:"labelSource"`
	Diagnostics    []string `json:"diagnostics"`
	Members        []string `json:"members"`
}
type CatalogRecipeView struct {
	ID          string                   `json:"id"`
	Type        string                   `json:"type"`
	Status      string                   `json:"status"`
	Payload     json.RawMessage          `json:"payload"`
	Diagnostics []string                 `json:"diagnostics"`
	Refs        []store.CatalogRecipeRef `json:"refs"`
}
type ItemCatalog struct {
	Revision         int64               `json:"revision"`
	BuiltAt          int64               `json:"builtAt"`
	Locale           string              `json:"locale"`
	AvailableLocales []string            `json:"availableLocales"`
	Warnings         []string            `json:"warnings"`
	Items            []CatalogItemView   `json:"items"`
	Blocks           []CatalogBlockView  `json:"blocks"`
	Tags             []CatalogTagView    `json:"tags"`
	Recipes          []CatalogRecipeView `json:"recipes"`
}

func catalogName(names []store.CatalogName, locale, fallback string) (string, string) {
	for _, candidate := range []string{locale, "en_us"} {
		for _, name := range names {
			if name.Locale == candidate {
				return name.Name, candidate
			}
		}
	}
	return fallback, ""
}
func catalogView(c store.Catalog, locale string) ItemCatalog {
	view := ItemCatalog{Revision: c.Revision, BuiltAt: c.BuiltAt, Locale: locale, AvailableLocales: []string{}, Warnings: c.Warnings, Items: []CatalogItemView{}, Blocks: []CatalogBlockView{}, Tags: []CatalogTagView{}, Recipes: []CatalogRecipeView{}}
	locales, itemViews, blockViews := map[string]bool{}, map[string]CatalogItemView{}, map[string]CatalogBlockView{}
	for _, item := range c.Items {
		name, resolved := catalogName(item.Names, locale, item.ID)
		v := CatalogItemView{ID: item.ID, DisplayName: name, ResolvedLocale: resolved, Evidence: item.Evidence, ModelPath: item.ModelPath, IconStatus: item.IconStatus, IconReason: item.IconReason, Names: item.Names, Tags: item.Tags}
		view.Items = append(view.Items, v)
		itemViews[item.ID] = v
		for _, n := range item.Names {
			locales[n.Locale] = true
		}
	}
	for _, block := range c.Blocks {
		name, resolved := catalogName(block.Names, locale, block.ID)
		v := CatalogBlockView{ID: block.ID, DisplayName: name, ResolvedLocale: resolved, Evidence: block.Evidence, BlockstatePath: block.BlockstatePath, Names: block.Names, Tags: block.Tags, ItemIDs: block.ItemIDs}
		view.Blocks = append(view.Blocks, v)
		blockViews[block.ID] = v
		for _, n := range block.Names {
			locales[n.Locale] = true
		}
	}
	for _, tag := range c.Tags {
		name, resolved := catalogName(tag.Names, locale, "#"+tag.ID)
		source := "translation"
		if resolved == "" {
			source = "id"
			if len(tag.Members) == 1 {
				if tag.Registry == "item" {
					v := itemViews[tag.Members[0]]
					if v.ResolvedLocale != "" {
						name, resolved, source = v.DisplayName, v.ResolvedLocale, "single_member"
					}
				} else {
					v := blockViews[tag.Members[0]]
					if v.ResolvedLocale != "" {
						name, resolved, source = v.DisplayName, v.ResolvedLocale, "single_member"
					}
				}
			}
		}
		view.Tags = append(view.Tags, CatalogTagView{Registry: tag.Registry, ID: tag.ID, DisplayName: name, ResolvedLocale: resolved, Status: tag.Status, LabelSource: source, Diagnostics: tag.Diagnostics, Members: tag.Members})
	}
	for _, recipe := range c.Recipes {
		view.Recipes = append(view.Recipes, CatalogRecipeView{ID: recipe.ID, Type: recipe.Type, Status: recipe.Status, Payload: recipe.Payload, Diagnostics: recipe.Diagnostics, Refs: recipe.Refs})
	}
	for value := range locales {
		view.AvailableLocales = append(view.AvailableLocales, value)
	}
	sort.Strings(view.AvailableLocales)
	return view
}

// RebuildItemCatalog indexes vanilla resources and all current successfully parsed mods.
func (a *API) RebuildItemCatalog(ctx context.Context, packID, locale, requestID string) (ItemCatalog, error) {
	locale = catalogLocale(locale)
	if !catalogLocalePattern.MatchString(locale) {
		return ItemCatalog{}, ErrInvalidArgument
	}
	if err := a.ready(); err != nil {
		return ItemCatalog{}, err
	}
	minecraftID := "minecraft-" + packID
	if _, err := a.parseAndPersistModContent(ctx, packID, minecraftID, "catalog:"+requestID, nil); err != nil {
		return ItemCatalog{}, err
	}
	snapshot, err := a.repo.CatalogSourceSnapshot(ctx, packID)
	if err != nil {
		return ItemCatalog{}, err
	}
	if !assetVersionPattern.MatchString(snapshot.Pack.MCVersion) {
		return ItemCatalog{}, ErrInvalidArgument
	}
	generationRevision, err := a.repo.GetPackConfigRevision(ctx, packID)
	if err != nil {
		return ItemCatalog{}, err
	}
	builder := newCatalogBuilder()
	warnings := []string{"目录来自已验证的 Minecraft 与模组解析批次；运行时脚本注册需要运行时快照才能补齐。"}
	resources := newIconResources()
	active, seenSHA := map[string]bool{}, map[string]bool{}
	for index, mod := range snapshot.Mods {
		if mod.Status != "installed" {
			continue
		}
		if mod.CurrentSelectionID == "" {
			warnings = append(warnings, mod.DisplayName+" 尚无已验证的精确版本选择，未纳入目录。")
			continue
		}
		run := snapshot.Runs[index]
		if run.Status != "succeeded" || run.SHA1 != mod.SHA1 || mod.SHA1 == "" {
			warnings = append(warnings, mod.DisplayName+" 当前版本未成功解析，未纳入目录。")
			continue
		}
		if seenSHA[mod.SHA1] {
			continue
		}
		seenSHA[mod.SHA1] = true
		active[mod.ID] = true
	}
	sort.Slice(snapshot.Content, func(i, j int) bool {
		if snapshot.Content[i].ModID != snapshot.Content[j].ModID {
			return snapshot.Content[i].ModID < snapshot.Content[j].ModID
		}
		return snapshot.Content[i].Path < snapshot.Content[j].Path
	})
	modResources := &ExtractedContent{}
	for _, row := range snapshot.Content {
		if active[row.ModID] {
			builder.add(catalogResource{source: row.ModID, path: row.Path, payload: json.RawMessage(row.Payload)})
			content := ContentItem{Kind: row.Kind, Path: row.Path, Key: row.Key, Payload: []byte(row.Payload)}
			switch row.Kind {
			case "item_model":
				modResources.Items = append(modResources.Items, content)
			case "texture":
				modResources.Textures = append(modResources.Textures, content)
			case "tag":
				modResources.Tags = append(modResources.Tags, content)
			}
		}
	}
	resources.addContent(modResources)
	catalog := builder.finish()
	rendered, missingReasons := resources.icons()
	itemIndex := map[string]int{}
	for index, item := range catalog.Items {
		itemIndex[item.ID] = index
	}
	for _, icon := range rendered {
		var payload struct {
			Mime, Data, Source string
			Size               int
		}
		if json.Unmarshal(icon.Payload, &payload) != nil {
			continue
		}
		data, decodeErr := base64.StdEncoding.DecodeString(payload.Data)
		if decodeErr != nil {
			continue
		}
		index, exists := itemIndex[icon.Key]
		if !exists {
			// 模型文件不再等量于物品：给不存在的物品存图标会撞外键。
			continue
		}
		catalog.Icons = append(catalog.Icons, store.CatalogIcon{ItemID: icon.Key, Mime: payload.Mime, Data: data, Width: 32, Height: 32, Source: payload.Source})
		catalog.Items[index].IconStatus = "ready"
	}
	for id, reason := range missingReasons {
		if index, ok := itemIndex[id]; ok {
			catalog.Items[index].IconStatus = "missing"
			catalog.Items[index].IconReason = reason
		}
	}
	catalog.Revision = snapshot.Revision
	catalog.BuiltAt = a.now().UnixMilli()
	catalog.Warnings = warnings
	if err = a.repo.ReplaceCatalog(ctx, packID, catalog, requestID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ItemCatalog{}, &DomainError{Status: 409, Code: "catalog_stale", Message: "pack changed during catalog rebuild; retry"}
		}
		return ItemCatalog{}, err
	}
	if err = a.repo.PublishCatalogGeneration(ctx, packID, generationRevision, catalog, catalogResolverVersion, requestID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ItemCatalog{}, &DomainError{Status: 409, Code: "catalog_stale", Message: "pack changed during catalog generation; retry"}
		}
		return ItemCatalog{}, err
	}
	return catalogView(catalog, locale), nil
}

// catalogReadFailure 把目录读取的三种失败翻成可分辨的错误码；不归它管时返回 nil。
//
// 旧实现只认 store.ErrConflict，于是「从没构建过目录」和「目录过期」都是
// 409 catalog_stale，包不存在也被报成 409（链路测试缺陷 O9）。
func catalogReadFailure(err error) error {
	switch {
	case errors.Is(err, store.ErrCatalogNotBuilt):
		return &DomainError{Status: 409, Code: "catalog_not_built", Message: "这个包还没有构建过物品目录，点「重建目录」开始"}
	case errors.Is(err, store.ErrConflict):
		return &DomainError{Status: 409, Code: "catalog_stale", Message: "包内容已变化，物品目录需要重建后才有数据"}
	case errors.Is(err, store.ErrNotFound):
		return NotFoundError("pack_not_found", "pack not found")
	}
	return nil
}

func (a *API) GetItemCatalog(ctx context.Context, packID, locale string) (ItemCatalog, error) {
	locale = catalogLocale(locale)
	if !catalogLocalePattern.MatchString(locale) {
		return ItemCatalog{}, ErrInvalidArgument
	}
	if err := a.ready(); err != nil {
		return ItemCatalog{}, err
	}
	catalog, err := a.repo.ReadCatalog(ctx, packID)
	if err != nil {
		if mapped := catalogReadFailure(err); mapped != nil {
			return ItemCatalog{}, mapped
		}
		return ItemCatalog{}, err
	}
	return catalogView(catalog, locale), nil
}

type CatalogStatus struct {
	SourceRevision int64    `json:"sourceRevision"`
	BuiltRevision  int64    `json:"builtRevision"`
	Status         string   `json:"status"`
	BuiltAt        int64    `json:"builtAt"`
	LastError      string   `json:"lastError"`
	Warnings       []string `json:"warnings"`
	Stale          bool     `json:"stale"`
}

func (a *API) GetCatalogStatus(ctx context.Context, packID string) (CatalogStatus, error) {
	if err := a.ready(); err != nil {
		return CatalogStatus{}, err
	}
	state, err := a.repo.ReadCatalogState(ctx, packID)
	if err != nil {
		return CatalogStatus{}, err
	}
	return CatalogStatus{SourceRevision: state.SourceRevision, BuiltRevision: state.BuiltRevision, Status: state.Status, BuiltAt: state.BuiltAt, LastError: state.LastError, Warnings: state.Warnings, Stale: state.SourceRevision != state.BuiltRevision}, nil
}

func (a *API) GetCatalogItem(ctx context.Context, packID, itemID, locale string) (CatalogItemView, error) {
	catalog, err := a.GetItemCatalog(ctx, packID, locale)
	if err != nil {
		return CatalogItemView{}, err
	}
	for _, item := range catalog.Items {
		if item.ID == itemID {
			return item, nil
		}
	}
	// 目录里没有这个物品 ≠ 包不存在：以前裸 store.ErrNotFound 会被兜底翻译成 404 pack_not_found。
	return CatalogItemView{}, NotFoundError("catalog_item_not_found", "目录里没有这个物品/方块条目: "+itemID)
}

func (a *API) GetCatalogTag(ctx context.Context, packID, registry, tagID, locale string) (CatalogTagView, error) {
	if registry == "" {
		registry = "item"
	}
	catalog, err := a.GetItemCatalog(ctx, packID, locale)
	if err != nil {
		return CatalogTagView{}, err
	}
	for _, tag := range catalog.Tags {
		if tag.Registry == registry && tag.ID == tagID {
			return tag, nil
		}
	}
	return CatalogTagView{}, NotFoundError("catalog_tag_not_found", "目录里没有这个标签: "+registry+":"+tagID)
}

func (a *API) GetCatalogIcon(ctx context.Context, packID, itemID string) (store.CatalogIcon, error) {
	if err := a.ready(); err != nil {
		return store.CatalogIcon{}, err
	}
	if !catalogIDPattern.MatchString(itemID) {
		return store.CatalogIcon{}, ErrInvalidArgument
	}
	icon, err := a.repo.ReadCatalogIcon(ctx, packID, itemID)
	if err != nil {
		if mapped := catalogReadFailure(err); mapped != nil {
			return store.CatalogIcon{}, mapped
		}
		return store.CatalogIcon{}, err
	}
	return icon, nil
}
