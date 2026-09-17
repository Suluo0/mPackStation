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
	generated bool
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
	if json.Unmarshal(raw, &child) != nil || child.Loader != "" {
		return iconModel{}, false
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
func (r *iconResources) texture(ref string, variables map[string]string) image.Image {
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
func (r *iconResources) icons() ([]ContentItem, []string) {
	keys := []string{}
	for key := range r.models {
		if strings.Contains(key, ":item/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	icons := []ContentItem{}
	missing := []string{}
	for _, key := range keys {
		id := strings.Replace(key, ":item/", ":", 1)
		img, source := r.icon(key)
		if img == nil {
			missing = append(missing, id)
			continue
		}
		icons = append(icons, encodeItemIcon(img, id, source))
	}
	return icons, missing
}
