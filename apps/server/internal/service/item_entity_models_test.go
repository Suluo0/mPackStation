package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// solidPNG 造一张纯色 PNG，用来喂 atlasOf 验尺寸读取。
func solidPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetNRGBA(x, y, color.NRGBA{200, 100, 50, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// 图集尺寸必须从贴图本身读。头颅图集有 64×32（骷髅/苦力怕）与 64×64 两档，
// 曾经写死 64×64，导致 64×32 那几张的 V 轴被压成一半 —— 采样点整体跑偏，
// 出图是一片糊，而不是报错，所以只有断言尺寸才拦得住。
func TestAtlasOfReadsTextureSize(t *testing.T) {
	r := newIconResources()
	r.textures["minecraft:entity/skeleton/skeleton"] = solidPNG(t, 64, 32)
	r.textures["minecraft:entity/zombie/zombie"] = solidPNG(t, 64, 64)
	if a := r.atlasOf("minecraft:entity/skeleton/skeleton", 64, 64); a.height != 32 {
		t.Fatalf("64x32 atlas height = %v, want 32", a.height)
	}
	if a := r.atlasOf("minecraft:entity/zombie/zombie", 64, 64); a.height != 64 {
		t.Fatalf("64x64 atlas height = %v, want 64", a.height)
	}
	// 贴图缺失时退回给定尺寸，不 panic。
	if a := r.atlasOf("minecraft:entity/missing", 64, 48); a.width != 64 || a.height != 48 {
		t.Fatalf("fallback = %vx%v", a.width, a.height)
	}
}

// 图集 UV 的换算口径必须与 renderBlockModel 的采样一致：pixel * 16 / 边长。
func TestAtlasRectScalesToSixteen(t *testing.T) {
	a := entityAtlas{id: "x", width: 64, height: 32}
	got := a.rect(8, 8, 8, 8)
	want := [4]float64{2, 4, 4, 8} // 8*16/64, 8*16/32, 16*16/64, 16*16/32
	if got != want {
		t.Fatalf("rect = %v, want %v", got, want)
	}
}

func TestEntityModelDispatch(t *testing.T) {
	r := newIconResources()
	hit := []string{
		"minecraft:chest", "minecraft:trapped_chest", "minecraft:ender_chest",
		"minecraft:shulker_box", "minecraft:black_shulker_box", "minecraft:lime_shulker_box",
		"minecraft:black_bed", "minecraft:white_bed", "minecraft:black_banner",
		"minecraft:white_banner",
		"minecraft:skeleton_skull", "minecraft:wither_skeleton_skull",
		"minecraft:zombie_head", "minecraft:creeper_head", "minecraft:piglin_head",
		"minecraft:player_head", "minecraft:dragon_head",
		"minecraft:shield", "minecraft:conduit", "minecraft:decorated_pot",
	}
	for _, id := range hit {
		if _, ok := r.entityModel(id); !ok {
			t.Errorf("entityModel(%q) missed", id)
		}
	}
	miss := []string{
		"minecraft:stone", "minecraft:iron_ingot", "minecraft:oak_leaves",
		"minecraft:air", "minecraft:heavy_core", "minecraft:trident",
		"other:black_bed", // 非 minecraft 命名空间一律不管
	}
	for _, id := range miss {
		if _, ok := r.entityModel(id); ok {
			t.Errorf("entityModel(%q) should not match", id)
		}
	}
}

// icons() 传进来的是模型键（item/xxx），而 reason 对外暴露的是物品 id。
// 两种形态都必须命中，否则线上永远是 unsupported_model。
func TestEntityModelAcceptsItemAndModelKey(t *testing.T) {
	r := newIconResources()
	for _, id := range []string{"minecraft:chest", "minecraft:item/chest"} {
		if _, ok := r.entityModel(id); !ok {
			t.Fatalf("entityModel(%q) missed", id)
		}
	}
}

// 每个家族都要有真实几何，且每个面都带 length-4 的 UV —— 缺一个面渲染器会直接跳过，
// 箱子会变成没盖子的盒子而测试却全绿。
func TestEntityModelGeometryWellFormed(t *testing.T) {
	r := newIconResources()
	for _, id := range []string{
		"minecraft:chest", "minecraft:ender_chest", "minecraft:shulker_box",
		"minecraft:red_bed", "minecraft:black_banner", "minecraft:skeleton_skull",
	} {
		m, ok := r.entityModel(id)
		if !ok {
			t.Fatalf("entityModel(%q) missed", id)
		}
		if len(m.Elements) == 0 {
			t.Fatalf("%s: no elements", id)
		}
		if len(m.Textures) == 0 || m.Textures["tex"] == "" {
			t.Fatalf("%s: no texture binding", id)
		}
		if _, ok := m.Display["gui"]; !ok {
			t.Fatalf("%s: no gui transform", id)
		}
		for i, el := range m.Elements {
			if el.To == el.From {
				t.Fatalf("%s: element %d is degenerate", id, i)
			}
			if len(el.Faces) == 0 {
				t.Fatalf("%s: element %d has no faces", id, i)
			}
			for dir, f := range el.Faces {
				if len(f.UV) != 4 {
					t.Fatalf("%s: element %d face %s uv=%v", id, i, dir, f.UV)
				}
			}
		}
	}
}

// 端到端冒烟：拿真实客户端 jar 把所有 builtin/entity 物品渲染一遍。
// jar 不在就跳过（CI 里没有这份缓存是正常的）。
func TestEntityIconsRenderFromClientJar(t *testing.T) {
	candidates := []string{
		os.Getenv("MC_JAR"),
		filepath.Join("..", "..", "..", "..", "data", "cache", "minecraft-assets", "1.21.1.jar"),
	}
	jar := ""
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			jar = c
			break
		}
	}
	raw, err := os.ReadFile(jar)
	if err != nil {
		t.Skipf("client jar unavailable: %v", err)
	}
	r := newIconResources()
	if err := r.addClientJar(raw); err != nil {
		t.Fatal(err)
	}
	ids := []string{
		"chest", "trapped_chest", "ender_chest", "shulker_box",
		"white_shulker_box", "black_shulker_box",
		"white_bed", "red_bed", "black_bed",
		"white_banner", "red_banner", "black_banner", "blue_banner",
		"skeleton_skull", "wither_skeleton_skull", "zombie_head",
		"creeper_head", "piglin_head", "player_head", "dragon_head",
		"shield", "conduit", "decorated_pot",
	}
	for _, id := range ids {
		img, reason := r.icon("minecraft:item/" + id)
		if img == nil {
			t.Errorf("%s: no icon (%s)", id, reason)
			continue
		}
		if reason != "entity_model" {
			t.Errorf("%s: reason = %q, want entity_model", id, reason)
		}
	}
}
