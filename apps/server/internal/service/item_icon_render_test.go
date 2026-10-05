package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func pngBytes(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidIconTexture(c)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func solidIconTexture(c color.NRGBA) image.Image {
	im := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
	return im
}
func TestCubeIconUpright(t *testing.T) {
	im := renderIsometricCube(solidIconTexture(color.NRGBA{255, 255, 255, 255}))
	if im == nil {
		t.Fatal("no cube")
	}
	r, _, _, a := im.At(16, 7).RGBA()
	if a == 0 || r < 60000 {
		t.Fatalf("upper diamond must be bright top, got %v", im.At(16, 7))
	}
	r, _, _, a = im.At(16, 24).RGBA()
	if a == 0 || r >= 60000 {
		t.Fatalf("lower region must be shaded side, got %v", im.At(16, 24))
	}
	if _, _, _, a = im.At(0, 0).RGBA(); a != 0 {
		t.Fatal("background not transparent")
	}
}
func TestIconModelInheritanceAndNamespace(t *testing.T) {
	r := newIconResources()
	var buf bytes.Buffer
	png.Encode(&buf, solidIconTexture(color.NRGBA{240, 120, 30, 255}))
	r.textures["test:block/base"] = buf.Bytes()
	r.models["test:block/base"] = json.RawMessage(`{"parent":"block/cube_all","textures":{"all":"#base","base":"test:block/base"}}`)
	r.models["test:item/base"] = json.RawMessage(`{"parent":"test:block/base"}`)
	icons, missing := r.icons()
	if len(icons) != 1 || len(missing) != 0 || icons[0].Key != "test:base" {
		t.Fatalf("identity/parent failure: %v %v", icons, missing)
	}
	r.models["test:block/base"] = json.RawMessage(`{"parent":"test:item/base"}`)
	if im, _ := r.icon("test:item/base"); im != nil {
		t.Fatal("cyclic parent accepted")
	}
}
func TestCubeIconUsesCorrectFaces(t *testing.T) {
	r := newIconResources()
	m, _ := r.model("block/cube_all", map[string]bool{})
	for name, f := range m.Elements[0].Faces {
		f.Texture = name
		m.Elements[0].Faces[name] = f
	}
	im, _ := renderBlockModel(m, func(ref string) image.Image {
		switch ref {
		case "up":
			return solidIconTexture(color.NRGBA{255, 0, 0, 255})
		case "north":
			return solidIconTexture(color.NRGBA{0, 255, 0, 255})
		case "east":
			return solidIconTexture(color.NRGBA{0, 0, 255, 255})
		default:
			return solidIconTexture(color.NRGBA{255, 255, 255, 255})
		}
	})
	seen := map[string]bool{}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				continue
			}
			if c.R > 0 && c.G > 0 && c.B > 0 {
				t.Fatal("hidden face visible")
			}
			if c.R > 0 {
				seen["top"] = true
			}
			if c.G > 0 {
				seen["north"] = true
			}
			if c.B > 0 {
				seen["east"] = true
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("missing faces %v", seen)
	}
}
func TestVanillaIconsCachedAndPackScoped(t *testing.T) {
	a, pack, mod := modContentFixture(t)
	runParse(t, a, pack, mod)
	var buf, tex bytes.Buffer
	z := zip.NewWriter(&buf)
	png.Encode(&tex, solidIconTexture(color.NRGBA{200, 100, 50, 255}))
	for _, id := range []string{"iron_ingot", "redstone"} {
		addZipFile(t, z, "assets/minecraft/models/item/"+id+".json", `{"parent":"item/generated","textures":{"layer0":"item/`+id+`"}}`)
		w, e := z.Create("assets/minecraft/textures/item/" + id + ".png")
		if e != nil {
			t.Fatal(e)
		}
		w.Write(tex.Bytes())
	}
	z.Close()
	dir := filepath.Join(a.dataDir, "cache", "minecraft-assets")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "1.20.1.jar"), buf.Bytes(), 0644)
	res, err := a.ResolveModContentIcons(context.Background(), pack, mod)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, it := range res.Items {
		found[it.Key] = true
	}
	if !found["minecraft:iron_ingot"] || !found["minecraft:redstone"] || len(res.Warnings) != 0 {
		t.Fatalf("missing vanilla: %v %v", found, res.Warnings)
	}
	if _, err = a.ResolveModContentIcons(context.Background(), "other-pack", mod); err == nil {
		t.Fatal("cross-pack access accepted")
	}
}
func TestBlockUVAndSlabGeometry(t *testing.T) {
	r := newIconResources()
	m, _ := r.model("block/cube_all", map[string]bool{})
	scale := [3]float64{.625, .625, .625}
	m.Display["gui"] = modelTransform{Scale: &scale}
	m.GUILight = "front"
	tex := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := color.NRGBA{255, 0, 0, 255}
			if y >= 8 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			tex.SetNRGBA(x, y, c)
		}
	}
	im, _ := renderBlockModel(m, func(string) image.Image { return tex })
	top := color.NRGBAModel.Convert(im.At(16, 8)).(color.NRGBA)
	bottom := color.NRGBAModel.Convert(im.At(16, 23)).(color.NRGBA)
	if top.R != 255 || bottom.B != 255 {
		t.Fatalf("UV inverted: top=%v bottom=%v", top, bottom)
	}
	m.Elements[0].To[1] = 8
	im, _ = renderBlockModel(m, func(string) image.Image { return tex })
	if _, _, _, a := im.At(16, 8).RGBA(); a != 0 {
		t.Fatal("slab rendered as full cube")
	}
	if _, _, _, a := im.At(16, 23).RGBA(); a == 0 {
		t.Fatal("slab lower half absent")
	}
}
func TestGeneratedLayersAndAnimationFrame(t *testing.T) {
	r := newIconResources()
	base := solidIconTexture(color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, base)
	r.textures["test:item/base"] = append([]byte(nil), b.Bytes()...)
	overlay := image.NewNRGBA(image.Rect(0, 0, 16, 32))
	overlay.SetNRGBA(0, 0, color.NRGBA{0, 255, 0, 255})
	for y := 16; y < 32; y++ {
		for x := 0; x < 16; x++ {
			overlay.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
		}
	}
	b.Reset()
	png.Encode(&b, overlay)
	r.textures["test:item/overlay"] = append([]byte(nil), b.Bytes()...)
	r.models["test:item/layered"] = json.RawMessage(`{"parent":"item/generated","textures":{"layer0":"test:item/base","layer1":"test:item/overlay"}}`)
	im, _ := r.icon("test:item/layered")
	if im == nil {
		t.Fatal("no layered icon")
	}
	c := color.NRGBAModel.Convert(im.At(0, 0)).(color.NRGBA)
	if c.G != 255 {
		t.Fatal("overlay lost")
	}
	c = color.NRGBAModel.Convert(im.At(20, 25)).(color.NRGBA)
	if c.R != 255 {
		t.Fatal("animation strip squeezed or transparency lost")
	}
}
func TestIconTagRepresentativesAndCycles(t *testing.T) {
	r := newIconResources()
	r.addCommonTagRepresentatives("1.21.1")
	icons := []ContentItem{{Key: "minecraft:iron_ingot"}, {Key: "minecraft:redstone"}, {Key: "minecraft:diamond"}}
	r.addTag("data/test/tags/item/nested.json", []byte(`{"values":["#test:cycle","#c:ingots/iron"]}`))
	r.addTag("data/test/tags/item/cycle.json", []byte(`{"values":["#test:nested"]}`))
	aliases := r.tagIcons(icons)
	if aliases["#c:ingots/iron"] != "minecraft:iron_ingot" || aliases["#c:dusts/redstone"] != "minecraft:redstone" || aliases["#test:nested"] != "minecraft:iron_ingot" {
		t.Fatalf("tag resolution %v", aliases)
	}
	r.addTag("data/c/tags/item/ingots/iron.json", []byte(`{"replace":true,"values":[]}`))
	if r.tagIcons(icons)["#c:ingots/iron"] != "" {
		t.Fatal("tag replace ignored")
	}
	older := newIconResources()
	older.addCommonTagRepresentatives("1.20.1")
	if len(older.tags) != 0 {
		t.Fatal("common tag seed leaked across versions")
	}
}

/*
0025 后置 + 图标渲染器补齐方案 R1/R2（2026-10-03）：
  - loader 不再一票否决；item_layers / separate_transforms / composite 各有通路；
  - composite 子模型的 #变量 就地解引用，合并按 key 字典序，贴图只补缺不覆盖。
*/
func TestSeparateTransformsAndItemLayers(t *testing.T) {
	r := newIconResources()
	r.models["minecraft:item/generated"] = []byte(`{}`)
	r.models["minecraft:block/block"] = []byte(`{"gui_light":"side"}`)
	r.textures["minecraft:item/layer0"] = pngBytes(t, color.NRGBA{200, 60, 20, 255})
	r.textures["minecraft:item/layer1"] = pngBytes(t, color.NRGBA{20, 60, 200, 255})
	r.models["mekanism:item/portable_qio_dashboard"] = []byte(`{
		"loader":"neoforge:separate_transforms",
		"base":{"parent":"minecraft:item/generated","textures":{"layer0":"minecraft:item/layer0"}},
		"textures":{"layer1":"minecraft:item/layer1"},
		"display":{"gui":{"scale":[1.2,1.2,1.2]}}
	}`)
	img, source := r.icon("mekanism:item/portable_qio_dashboard")
	if img == nil {
		t.Fatalf("separate_transforms model rejected")
	}
	if source != "generated" {
		t.Fatalf("source = %q, want generated", source)
	}
	// layer1 覆盖在 layer0 上：中心像素应是 layer1 的蓝
	if got := img.At(16, 16); got != (color.RGBA{20, 60, 200, 255}) {
		t.Fatalf("top layer pixel = %#v", got)
	}

	// item_layers：parent + layer0 直接可画
	r2 := newIconResources()
	r2.models["minecraft:item/generated"] = []byte(`{}`)
	r2.textures["minecraft:item/flat"] = pngBytes(t, color.NRGBA{9, 99, 9, 255})
	r2.models["mekanism:item/qio_panel"] = []byte(`{"loader":"neoforge:item_layers","parent":"minecraft:item/generated","textures":{"layer0":"minecraft:item/flat"}}`)
	img2, source2 := r2.icon("mekanism:item/qio_panel")
	if img2 == nil || source2 != "generated" {
		t.Fatalf("item_layers rejected: img=%v source=%q", img2, source2)
	}
}

func TestCompositeChildMerge(t *testing.T) {
	r := newIconResources()
	// 真实 block/block 自带标准 gui 旋转；六面齐全避免特定角度背面剔除全灭
	r.models["minecraft:block/block"] = []byte(`{"gui_light":"side","display":{"gui":{"rotation":[30,225,0],"scale":[0.625,0.625,0.625]}}}`)
	r.models["mekanism:block/factory/base"] = []byte(`{"elements":[{"from":[0,0,0],"to":[16,8,16],"faces":{
		"north":{"texture":"#side","uv":[0,8,16,16]},"south":{"texture":"#side","uv":[0,8,16,16]},
		"east":{"texture":"#side","uv":[0,8,16,16]},"west":{"texture":"#side","uv":[0,8,16,16]},
		"up":{"texture":"#top","uv":[0,0,16,16]},"down":{"texture":"#top","uv":[0,0,16,16]}}}]}`)
	r.models["mekanism:block/factory/front"] = []byte(`{"elements":[{"from":[4,8,4],"to":[12,14,12],"faces":{
		"north":{"texture":"#front","uv":[0,0,8,6]},"south":{"texture":"#front","uv":[0,0,8,6]},
		"east":{"texture":"#front","uv":[0,0,8,6]},"west":{"texture":"#front","uv":[0,0,8,6]}}}]}`)
	r.models["mekanism:item/advanced_crushing_factory"] = []byte(`{
		"loader":"neoforge:composite","parent":"minecraft:block/block",
		"textures":{"side":"minecraft:block/iron","top":"minecraft:block/iron","front":"minecraft:block/gold"},
		"children":{"base":{"parent":"mekanism:block/factory/base"},
		            "front_led":{"parent":"mekanism:block/factory/front"}}
	}`)
	r.textures["minecraft:block/iron"] = pngBytes(t, color.NRGBA{120, 120, 130, 255})
	r.textures["minecraft:block/gold"] = pngBytes(t, color.NRGBA{230, 180, 40, 255})

	img, source := r.icon("mekanism:item/advanced_crushing_factory")
	if img == nil {
		t.Fatalf("composite model rejected (source=%q)", source)
	}
	if source != "elements" {
		t.Fatalf("source = %q, want elements", source)
	}
	// front 子模型的金色贴图必须出现在图中（证明 #变量 已就地解引用，
	// 否则 #front 找不到贴图会被剔除或整体 missing）
	gold, iron, blank := 0, 0, 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				blank++
				continue
			}
			// 侧面光照系数 0.65：金色 (230,180,40) 渲染后 ≈(149,117,26)，
			// 按暖色序判（铁灰 R≈G 不命中）
			if c.R > c.G+20 && c.G > c.B+40 {
				gold++
			} else if c.R < 180 && c.B >= c.R {
				iron++
			}
		}
	}
	t.Logf("pixels gold=%d iron=%d blank=%d", gold, iron, blank)
	if gold == 0 {
		t.Fatalf("front (gold) texture never rendered — composite child variables not resolved")
	}
}
