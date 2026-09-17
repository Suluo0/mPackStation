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
