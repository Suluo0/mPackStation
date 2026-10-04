package service

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"sort"
	"strings"
)

// Resource locations retain item/ and block/: these are different models even
// when the registry id is identical. Only item models become inventory icons.
type iconResources struct {
	models        map[string]json.RawMessage
	textures      map[string][]byte
	tags          map[string][]json.RawMessage
	resourceFiles map[string]json.RawMessage
}

func newIconResources() *iconResources {
	return &iconResources{resourceFiles: map[string]json.RawMessage{}, models: map[string]json.RawMessage{}, textures: map[string][]byte{}, tags: map[string][]json.RawMessage{}}
}
func resourceID(s string) string {
	if s != "" && !strings.Contains(s, ":") {
		return "minecraft:" + s
	}
	return s
}
func assetID(p, category, ext string) string {
	parts := strings.SplitN(p, "/", 3)
	if len(parts) != 3 || parts[0] != "assets" || !strings.HasPrefix(parts[2], category) || !strings.HasSuffix(p, ext) {
		return ""
	}
	return parts[1] + ":" + strings.TrimSuffix(strings.TrimPrefix(parts[2], category), ext)
}
func (r *iconResources) addContent(out *ExtractedContent) {
	for _, it := range out.Tags {
		r.addTag(it.Path, it.Payload)
	}
	for _, it := range out.Items {
		if key := assetID(it.Path, "models/", ".json"); key != "" {
			r.models[key] = it.Payload
		}
	}
	for _, it := range out.Textures {
		var p struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(it.Payload, &p) != nil {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(p.Data)
		if key := assetID(it.Path, "textures/", ".png"); err == nil && key != "" {
			r.textures[key] = raw
		}
	}
}
func (r *iconResources) addClientJar(raw []byte) error {
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	if len(z.File) > modContentMaxEntries {
		return ErrJarTooManyEntries
	}
	var total uint64
	for _, f := range z.File {
		if isUnsafeZipPath(f.Name) {
			continue
		}
		model := assetID(f.Name, "models/", ".json")
		texture := assetID(f.Name, "textures/", ".png")
		tag := itemTagID(f.Name)
		lang := assetID(f.Name, "lang/", ".json")
		catalogFile := isCatalogResourcePath(f.Name)
		if model == "" && texture == "" && tag == "" && lang == "" && !catalogFile {
			continue
		}
		total += f.UncompressedSize64
		if total > modContentMaxTotalBytes {
			return ErrJarTooLarge
		}
		if f.UncompressedSize64 > modContentMaxEntryBytes {
			continue
		}
		data, e := readModContentZipEntry(f)
		if e != nil {
			return e
		}
		if catalogFile {
			r.resourceFiles[f.Name] = data
		}
		if tag != "" {
			r.addTag(f.Name, data)
		}
		if model != "" {
			r.models[model] = data
		} else if texture != "" {
			r.textures[texture] = data
		}
	}
	return nil
}

type modelFace struct {
	UV        []float64 `json:"uv"`
	Texture   string    `json:"texture"`
	Rotation  int       `json:"rotation"`
	TintIndex *int      `json:"tintindex"`
	// FixedTint 是合成几何（builtin/entity 家族）专用的固定乘色。JSON 模型的着色
	// 走 tintindex + 生物群系默认色那条路，这里给手写几何一条直接通路 —— 旗帜的
	// 白底图要乘染料色，没有别的表达方式。
	FixedTint *[3]float64 `json:"-"`
}
type modelElement struct {
	From     [3]float64           `json:"from"`
	To       [3]float64           `json:"to"`
	Faces    map[string]modelFace `json:"faces"`
	Rotation *struct {
		Origin  [3]float64 `json:"origin"`
		Axis    string     `json:"axis"`
		Angle   float64    `json:"angle"`
		Rescale bool       `json:"rescale"`
	} `json:"rotation"`
	Shade *bool `json:"shade"`
}
type modelTransform struct {
	Rotation    [3]float64  `json:"rotation"`
	Translation [3]float64  `json:"translation"`
	Scale       *[3]float64 `json:"scale"`
}
type iconModel struct {
	Parent    string                    `json:"parent"`
	Textures  map[string]string         `json:"textures"`
	Elements  []modelElement            `json:"elements"`
	Display   map[string]modelTransform `json:"display"`
	Loader    string                    `json:"loader"`
	GUILight  string                    `json:"gui_light"`
	Children  map[string]modelChildRef  `json:"children"`
	Base      *modelChildRef            `json:"base"`
	generated bool
}

/* neoforge:composite 的命名子模型引用 / neoforge:separate_transforms 的 base。 */
type modelChildRef struct {
	Parent   string            `json:"parent"`
	Loader   string            `json:"loader"`
	Textures map[string]string `json:"textures"`
}

func (r *iconResources) model(id string, seen map[string]bool) (iconModel, bool) {
	id = resourceID(id)
	if seen[id] || len(seen) >= 32 {
		return iconModel{}, false
	}
	seen[id] = true
	defer delete(seen, id)
	if id == "minecraft:builtin/generated" || id == "minecraft:item/generated" || id == "minecraft:item/handheld" {
		return iconModel{Textures: map[string]string{}, generated: true}, true
	}
	raw, exists := r.models[id]
	if !exists {
		// Standard cube fallback lets standalone mod extraction work without vanilla.
		if id == "minecraft:block/cube_all" {
			uv := []float64{0, 0, 16, 16}
			faces := map[string]modelFace{}
			for _, f := range []string{"up", "down", "north", "south", "east", "west"} {
				faces[f] = modelFace{UV: uv, Texture: "#all"}
			}
			scale := [3]float64{.625, .625, .625}
			return iconModel{Textures: map[string]string{}, Elements: []modelElement{{To: [3]float64{16, 16, 16}, Faces: faces}}, Display: map[string]modelTransform{"gui": {Rotation: [3]float64{30, 225, 0}, Scale: &scale}}}, true
		}
		return iconModel{}, false
	}
	var child iconModel
	if json.Unmarshal(raw, &child) != nil {
		return iconModel{}, false
	}
	// 0025 后置：loader 不再一票否决，按 loader 分派（图标渲染器补齐方案 R1/R2）。
	if child.Loader != "" {
		switch child.Loader {
		case "neoforge:item_layers", "mekanism:data_based":
			// 可画内容就在本 JSON（parent/layer0…），loader 只影响发光与光照，图标忽略
		case "neoforge:separate_transforms":
			if child.Base == nil || child.Base.Parent == "" {
				return iconModel{}, false
			}
			base, baseOK := r.model(child.Base.Parent, seen)
			if !baseOK {
				return iconModel{}, false
			}
			for k, v := range child.Base.Textures {
				base.Textures[k] = v
			}
			for k, v := range child.Textures {
				base.Textures[k] = v
			}
			if child.Elements != nil {
				base.Elements = child.Elements
			}
			if child.GUILight != "" {
				base.GUILight = child.GUILight
			}
			return base, true
		case "neoforge:composite":
			return r.composite(child, seen)
		default:
			return iconModel{}, false
		}
	}
	result := iconModel{Textures: map[string]string{}, Display: map[string]modelTransform{}}
	if child.Parent != "" {
		var ok bool
		result, ok = r.model(child.Parent, seen)
		if !ok {
			return iconModel{}, false
		}
	}
	if result.Textures == nil {
		result.Textures = map[string]string{}
	}
	if result.Display == nil {
		result.Display = map[string]modelTransform{}
	}
	for k, v := range child.Textures {
		result.Textures[k] = v
	}
	for k, v := range child.Display {
		result.Display[k] = v
	}
	if child.Elements != nil {
		result.Elements = child.Elements
	}
	if child.GUILight != "" {
		result.GUILight = child.GUILight
	}
	return result, true
}
/* composite（R2）：命名子模型求并。子模型的 #变量 先就地解成本模型贴图
   （否则子模型间同名变量互相踩），按 key 字典序遍历保证确定性；
   合并结果只补缺不覆盖外层贴图。 */
func (r *iconResources) composite(child iconModel, seen map[string]bool) (iconModel, bool) {
	result := iconModel{Textures: map[string]string{}, Display: map[string]modelTransform{}}
	if child.Parent != "" {
		parent, ok := r.model(child.Parent, seen)
		if !ok {
			return iconModel{}, false
		}
		result = parent
		if result.Textures == nil {
			result.Textures = map[string]string{}
		}
		if result.Display == nil {
			result.Display = map[string]modelTransform{}
		}
	}
	for k, v := range child.Textures {
		result.Textures[k] = v
	}
	for k, v := range child.Display {
		result.Display[k] = v
	}
	keys := make([]string, 0, len(child.Children))
	for k := range child.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ref := child.Children[k]
		if ref.Parent == "" {
			continue
		}
		sub, ok := r.model(ref.Parent, map[string]bool{})
		if !ok {
			continue
		}
		for kk, vv := range ref.Textures {
			sub.Textures[kk] = vv
		}
		sub = inlineVars(sub)
		result.Elements = append(result.Elements, sub.Elements...)
	}
	if len(result.Elements) == 0 {
		return iconModel{}, false
	}
	return result, true
}

func inlineVars(m iconModel) iconModel {
	for i, el := range m.Elements {
		for dir, f := range el.Faces {
			f.Texture = lookupVar(f.Texture, m.Textures)
			m.Elements[i].Faces[dir] = f
		}
	}
	return m
}

func lookupVar(ref string, variables map[string]string) string {
	seen := map[string]bool{}
	for strings.HasPrefix(ref, "#") {
		if seen[ref] || len(seen) > 16 {
			return ref
		}
		seen[ref] = true
		next, ok := variables[strings.TrimPrefix(ref, "#")]
		if !ok {
			return ref
		}
		ref = next
	}
	return ref
}

func (r *iconResources) texture(ref string, variables map[string]string) image.Image {
	// 1.21 起面贴图允许把变量名写成不带 # 的裸名字（vanilla 的 block/heavy_core
	// 就是 "texture": "all"）。不认这一档的话，一个 pure 方块模型会整块判成
	// missing_texture —— 而这里的其它路径都按 "#var" 处理，差别只在开头那个字符。
	if !strings.HasPrefix(ref, "#") && !strings.Contains(ref, ":") {
		if _, ok := variables[ref]; ok {
			ref = "#" + ref
		}
	}
	seen := map[string]bool{}
	for strings.HasPrefix(ref, "#") {
		if seen[ref] || len(seen) > 32 {
			return nil
		}
		seen[ref] = true
		ref = variables[strings.TrimPrefix(ref, "#")]
	}
	raw := r.textures[resourceID(ref)]
	if len(raw) == 0 {
		return nil
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 32768 || int64(cfg.Width)*int64(cfg.Height) > 16*1024*1024 {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	// Animation strips use their first square frame, never a squeezed whole strip.
	if img.Bounds().Dy() > img.Bounds().Dx() {
		return img.(interface {
			SubImage(image.Rectangle) image.Image
		}).SubImage(image.Rect(0, 0, cfg.Width, cfg.Width))
	}
	return img
}
func (r *iconResources) icon(id string) (image.Image, string) {
	img, reason := r.iconModel(id)
	if img != nil {
		return img, reason
	}
	/* builtin/entity 家族（箱子 / 床 / 旗帜 / 潜影盒 / 头颅）：模型 JSON 里没有几何，
	   只有一句「交给方块实体渲染器」。这里用合成几何补上，形状与配色都是真的。 */
	if m, ok := r.entityModel(id); ok {
		entityImg, entityReason := renderBlockModel(m, func(ref string) image.Image { return r.texture(ref, m.Textures) })
		if entityImg != nil {
			return entityImg, "entity_model"
		}
		if entityReason == "missing_texture" {
			reason = entityReason
		}
	}
	/* 床 / 旗帜 / 箱子 / 告示牌这类物品用的是 builtin/entity：模型里没有几何，
	   只有一句「交给方块实体渲染器」。退回同名方块模型 —— 形状和贴图都是真的，
	   少的只是运行时那部分（旗帜图案、床的枕头朝向）。给不出图的时候才叫真缺。 */
	for _, alt := range blockModelFallbacks(id) {
		if altImg, _ := r.iconModel(alt); altImg != nil {
			return altImg, "block_fallback"
		}
	}
	return nil, reason
}

// blockModelFallbacks lists the block model candidates for an item model that
// carries no geometry of its own; beds are split into head/foot models.
func blockModelFallbacks(id string) []string {
	if !strings.Contains(id, ":item/") {
		return nil
	}
	base := strings.Replace(id, ":item/", ":block/", 1)
	return []string{base, base + "_head", base + "_foot"}
}

func (r *iconResources) iconModel(id string) (image.Image, string) {
	m, ok := r.model(id, map[string]bool{})
	if !ok {
		return nil, "unsupported_model"
	}
	if m.generated {
		out := image.NewRGBA(image.Rect(0, 0, 32, 32))
		found := false
		for i := 0; i < 5; i++ {
			ref := m.Textures["layer"+string(rune('0'+i))]
			if ref == "" {
				continue
			}
			tex := r.texture(ref, m.Textures)
			if tex == nil {
				return nil, "missing_texture"
			}
			found = true
			layer := image.NewRGBA(out.Bounds())
			b := tex.Bounds()
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					layer.Set(x, y, tex.At(b.Min.X+x*b.Dx()/32, b.Min.Y+y*b.Dy()/32))
				}
			}
			draw.Draw(out, out.Bounds(), layer, image.Point{}, draw.Over)
		}
		if found {
			return out, "generated"
		}
		return nil, "missing_texture"
	}
	if len(m.Elements) == 0 || len(m.Elements) > 256 {
		return nil, "unsupported_model"
	}
	return renderBlockModel(m, func(ref string) image.Image { return r.texture(ref, m.Textures) })
}
func encodeItemIcon(img image.Image, id, source string) ContentItem {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	payload, _ := json.Marshal(map[string]any{"mime": "image/png", "data": base64.StdEncoding.EncodeToString(buf.Bytes()), "size": buf.Len(), "source": source})
	return ContentItem{Kind: "item_icon", Key: id, Path: "generated://item_icon/" + id, Payload: payload}
}
func (r *iconResources) icons() ([]ContentItem, map[string]string) {
	keys := []string{}
	for key := range r.models {
		if strings.Contains(key, ":item/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	icons := []ContentItem{}
	missingReasons := map[string]string{}
	for _, key := range keys {
		id := strings.Replace(key, ":item/", ":", 1)
		img, reason := r.icon(key)
		if img == nil {
			missingReasons[id] = reason
			continue
		}
		icons = append(icons, encodeItemIcon(img, id, reason))
	}
	return icons, missingReasons
}
