package service

import (
	"bytes"
	"image/png"
	"strings"
)

/* builtin/entity 家族的合成几何。
 *
 * 箱子 / 床 / 旗帜 / 潜影盒 / 头颅 这些物品的模型 JSON 里只有一句
 * "parent": "builtin/entity"，意思是「形状交给方块实体渲染器」。几何写在游戏代码里，
 * 资源包里根本没有对应的 elements，所以照常理走方块模型那条路永远是
 * unsupported_model。工作台要出图，只能按原版实体模型的已知版式手写等价几何 + 图集 UV。
 *
 * 坐标系沿用方块模型的 0..16 立方体空间（Y 向上、8 是中心）。图集 UV 以
 * 「1/16 图集边长」为单位 —— 与 renderBlockModel 的采样口径一致（pixel * 16 / 边长），
 * 所以下面全部直接用像素矩形描述，不再手工折半。
 */

// entityAtlas 是一张实体图集贴图及其真实像素尺寸。尺寸必须从贴图本身读：
// 头颅图集有 64×32（骷髅/苦力怕）与 64×64（僵尸/猪灵/玩家）两种版式，
// 写死任何一档都会把 V 轴算错，采样点整体跑偏。
type entityAtlas struct {
	id     string
	width  float64
	height float64
}

// rect 把图集上的像素矩形换算成 renderBlockModel 认的 UV。
func (a entityAtlas) rect(x, y, w, h float64) [4]float64 {
	return [4]float64{
		x * 16 / a.width, y * 16 / a.height,
		(x + w) * 16 / a.width, (y + h) * 16 / a.height,
	}
}

// atlasOf 取图集的真实像素尺寸；贴图缺失时退回给定尺寸。
func (r *iconResources) atlasOf(id string, fallbackW, fallbackH float64) entityAtlas {
	a := entityAtlas{id: id, width: fallbackW, height: fallbackH}
	if raw := r.textures[id]; len(raw) > 0 {
		if cfg, err := png.DecodeConfig(bytes.NewReader(raw)); err == nil && cfg.Width > 0 && cfg.Height > 0 {
			a.width, a.height = float64(cfg.Width), float64(cfg.Height)
		}
	}
	return a
}

// entityBoxDef 是一个轴对齐盒子：三个方向的区间 + 六面的图集像素矩形。
// faces 的键沿用方块模型的 down/up/north/south/west/east。
type entityBoxDef struct {
	from, to [3]float64
	faces    map[string][4]float64
	tint     *[3]float64
}

// entityModelDef 是一个合成模型：一张图集 + 若干盒子 + GUI 变换。
type entityModelDef struct {
	atlas    entityAtlas
	boxes    []entityBoxDef
	tint     *[3]float64
	guiScale float64
	guiRot   [3]float64
	guiTrans [3]float64
}

// iconModel 把合成几何降级成渲染器认识的形状。六面统一引用同一张图集，
// 各面用 UV 切自己的矩形；需要着色的面（旗帜布料、旗杆）挂 FixedTint。
func (d entityModelDef) iconModel() iconModel {
	scale := [3]float64{.625, .625, .625}
	if d.guiScale > 0 {
		scale = [3]float64{d.guiScale, d.guiScale, d.guiScale}
	}
	rot := d.guiRot
	if rot == [3]float64{} {
		rot = [3]float64{30, 225, 0}
	}
	m := iconModel{
		Textures: map[string]string{"tex": d.atlas.id},
		Display:  map[string]modelTransform{"gui": {Rotation: rot, Translation: d.guiTrans, Scale: &scale}},
	}
	for _, b := range d.boxes {
		el := modelElement{From: b.from, To: b.to, Faces: map[string]modelFace{}}
		tint := b.tint
		if tint == nil {
			tint = d.tint
		}
		for dir, rect := range b.faces {
			uv := rect
			el.Faces[dir] = modelFace{Texture: "#tex", UV: uv[:], FixedTint: tint}
		}
		m.Elements = append(m.Elements, el)
	}
	return m
}

// cubeFaces 给一个盒子铺同一张矩形（六个面都一样），用于木头这类无方向材质。
func cubeFaces(rect [4]float64) map[string][4]float64 {
	return map[string][4]float64{
		"down": rect, "up": rect, "north": rect, "south": rect, "west": rect, "east": rect,
	}
}

/* 原版 DyeColor 的 RGB。床与潜影盒的贴图本身就是按颜色分文件的，
 * 只有旗帜是一张白底图乘染料色 —— 那一步需要真数字。 */
var dyeColors = map[string][3]float64{
	"white":      {0xF9 / 255.0, 0xFF / 255.0, 0xFE / 255.0},
	"orange":     {0xF9 / 255.0, 0x80 / 255.0, 0x1D / 255.0},
	"magenta":    {0xC7 / 255.0, 0x4E / 255.0, 0xBD / 255.0},
	"light_blue": {0x3A / 255.0, 0xB3 / 255.0, 0xDA / 255.0},
	"yellow":     {0xFE / 255.0, 0xD8 / 255.0, 0x3D / 255.0},
	"lime":       {0x80 / 255.0, 0xC7 / 255.0, 0x1F / 255.0},
	"pink":       {0xF3 / 255.0, 0x8B / 255.0, 0xAA / 255.0},
	"gray":       {0x47 / 255.0, 0x4F / 255.0, 0x52 / 255.0},
	"light_gray": {0x9D / 255.0, 0x9D / 255.0, 0x97 / 255.0},
	"cyan":       {0x16 / 255.0, 0x9C / 255.0, 0x9C / 255.0},
	"purple":     {0x89 / 255.0, 0x32 / 255.0, 0xB8 / 255.0},
	"blue":       {0x3C / 255.0, 0x44 / 255.0, 0xAA / 255.0},
	"brown":      {0x83 / 255.0, 0x54 / 255.0, 0x32 / 255.0},
	"green":      {0x5E / 255.0, 0x7C / 255.0, 0x16 / 255.0},
	"red":        {0xB0 / 255.0, 0x2E / 255.0, 0x26 / 255.0},
	"black":      {0x1D / 255.0, 0x1D / 255.0, 0x21 / 255.0},
}

// woodTint 是旗帜的木杆/横梁乘色 —— 图集里它们是白的，得染成木色。
var woodTint = [3]float64{0x8A / 255.0, 0x66 / 255.0, 0x3E / 255.0}

/* -------- 箱子 / 末影箱 / 陷阱箱 --------
 * 图集 entity/chest/{normal,trapped,ender}.png（64×64），版式来自原版 ModelChest：
 * 盖子 14×5×14 @ 图集偏移 (0,0)，箱体 14×10×14 @ (0,19)，锁扣 2×4×1 @ (0,0)。
 * 原版渲染这只箱子时会做一次 Y/Z 翻转，所以「外侧顶面」落在 ModelBox 的 down 槽位
 * —— 也就是图集 x28..42,y0..14 那一块（比另一块亮），不能想当然按 up 取。 */
func (r *iconResources) chestModel(name string) entityModelDef {
	id := "minecraft:entity/chest/normal"
	switch name {
	case "trapped_chest":
		id = "minecraft:entity/chest/trapped"
	case "ender_chest":
		id = "minecraft:entity/chest/ender"
	}
	a := r.atlasOf(id, 64, 64)
	lidFaces := map[string][4]float64{
		"up":    a.rect(28, 0, 14, 14), // 外侧顶面（亮木）
		"down":  a.rect(14, 0, 14, 14), // 盖子的内侧面（暗）
		"west":  a.rect(0, 14, 14, 5),
		"north": a.rect(14, 14, 14, 5),
		"east":  a.rect(28, 14, 14, 5),
		"south": a.rect(42, 14, 14, 5),
	}
	baseFaces := map[string][4]float64{
		"up":    a.rect(14, 19, 14, 14),
		"down":  a.rect(28, 19, 14, 14),
		"west":  a.rect(0, 33, 14, 10),
		"north": a.rect(14, 33, 14, 10),
		"east":  a.rect(28, 33, 14, 10),
		"south": a.rect(42, 33, 14, 10),
	}
	return entityModelDef{
		atlas: a,
		boxes: []entityBoxDef{
			{from: [3]float64{1, 10, 1}, to: [3]float64{15, 15, 15}, faces: lidFaces},
			{from: [3]float64{1, 0, 1}, to: [3]float64{15, 10, 15}, faces: baseFaces},
			// 锁扣：正面那一小块金属，只画朝外的 north 面就够。
			{from: [3]float64{7, 5.5, 1}, to: [3]float64{9, 10, 2}, faces: map[string][4]float64{"north": a.rect(1, 1, 2, 4)}},
		},
	}
}

/* -------- 潜影盒 --------
 * 图集 entity/shulker/shulker[_<color>].png（64×64），版式来自原版 ShulkerModel：
 * 盖子 16×12×16 @ (0,0)，箱体 16×8×16 @ (0,28)。总高 20 > 一个方块，靠 scale 收回去。 */
func (r *iconResources) shulkerModel(color string) entityModelDef {
	id := "minecraft:entity/shulker/shulker"
	if color != "" {
		id = "minecraft:entity/shulker/shulker_" + color
	}
	a := r.atlasOf(id, 64, 64)
	lidFaces := map[string][4]float64{
		"up":    a.rect(16, 0, 16, 16),
		"down":  a.rect(32, 0, 16, 16),
		"west":  a.rect(0, 16, 16, 12),
		"north": a.rect(16, 16, 16, 12),
		"east":  a.rect(32, 16, 16, 12),
		"south": a.rect(48, 16, 16, 12),
	}
	baseFaces := map[string][4]float64{
		"up":    a.rect(16, 28, 16, 16),
		"down":  a.rect(32, 28, 16, 16),
		"west":  a.rect(0, 44, 16, 8),
		"north": a.rect(16, 44, 16, 8),
		"east":  a.rect(32, 44, 16, 8),
		"south": a.rect(48, 44, 16, 8),
	}
	return entityModelDef{
		atlas: a,
		boxes: []entityBoxDef{
			{from: [3]float64{0, 8, 0}, to: [3]float64{16, 20, 16}, faces: lidFaces},
			{from: [3]float64{0, 0, 0}, to: [3]float64{16, 8, 16}, faces: baseFaces},
		},
		guiScale: .5, // 20 单位高，按 16/20 折回标准方块占位
	}
}

/* -------- 床 --------
 * 图集 entity/bed/<color>.png（64×64）。原版 BedModel 的头/脚两段几何写在代码里，
 * 这里按「木框 + 彩色床垫 + 白枕头」还原成一个可辨认的床：
 *   枕头（白）   图集 x5..21,  y2..13
 *   床垫（彩色） 图集 x2..25, y26..44
 *   木框与床腿   图集 x26..44, y0..22 */
func (r *iconResources) bedModel(color string) entityModelDef {
	id := "minecraft:entity/bed/red"
	if _, ok := dyeColors[color]; ok {
		id = "minecraft:entity/bed/" + color
	}
	a := r.atlasOf(id, 64, 64)
	wood := cubeFaces(a.rect(26, 0, 18, 22))
	quilt := a.rect(2, 26, 23, 18)
	quiltSide := a.rect(2, 14, 23, 8)
	quiltFaces := map[string][4]float64{
		"up": quilt, "north": quilt, "south": quilt,
		"east": quiltSide, "west": quiltSide, "down": quiltSide,
	}
	pillowFaces := cubeFaces(a.rect(5, 2, 16, 11))
	boxes := []entityBoxDef{}
	for _, p := range [][2]float64{{0.5, 0.5}, {13.5, 0.5}, {0.5, 13.5}, {13.5, 13.5}} {
		boxes = append(boxes, entityBoxDef{
			from:  [3]float64{p[0], 0, p[1]},
			to:    [3]float64{p[0] + 2, 3, p[1] + 2},
			faces: wood,
		})
	}
	boxes = append(boxes,
		entityBoxDef{from: [3]float64{0, 3, 0}, to: [3]float64{16, 4.5, 16}, faces: wood},
		entityBoxDef{from: [3]float64{0.5, 4.5, 0.5}, to: [3]float64{15.5, 10, 15.5}, faces: quiltFaces},
		entityBoxDef{from: [3]float64{1.5, 10, 1.5}, to: [3]float64{14.5, 13, 6.5}, faces: pillowFaces},
	)
	return entityModelDef{atlas: a, boxes: boxes}
}

/* -------- 旗帜 --------
 * 图集 entity/banner/base.png（64×64），白底图乘染料色。版式来自原版 BannerModel：
 * 旗面 20×40×1 @ (0,0)，旗杆 2×42×2 @ (44,0)，横梁 20×2×2 @ (0,42)。
 * 原版把整棵模型 Y 翻过来渲染，所以竖起来看是「横梁在上、旗面下垂、杆顶探出」。
 * 杆与梁在图集里没有独立贴图（都是布料那一块的白），靠 woodTint 乘成木色。 */
func (r *iconResources) bannerModel(color string) entityModelDef {
	a := r.atlasOf("minecraft:entity/banner/base", 64, 64)
	tint := dyeColors["white"]
	if c, ok := dyeColors[color]; ok {
		tint = c
	}
	cloth := map[string][4]float64{
		"north": a.rect(1, 1, 20, 40),
		"south": a.rect(22, 1, 20, 40),
		"west":  a.rect(0, 1, 1, 40),
		"east":  a.rect(21, 1, 1, 40),
		"up":    a.rect(1, 0, 20, 1),
		"down":  a.rect(1, 41, 20, 1),
	}
	// 木杆/横梁只有 1 单位宽（屏幕上约 2px），细节无所谓，用布料块乘木色即可。
	shaft := cubeFaces(a.rect(1, 1, 20, 40))
	return entityModelDef{
		atlas: a,
		tint:  &tint,
		boxes: []entityBoxDef{
			{from: [3]float64{3.7, 0, 7.6}, to: [3]float64{12.3, 12.8, 8.2}, faces: cloth},
			{from: [3]float64{2.7, 12.8, 7.5}, to: [3]float64{13.3, 13.7, 8.5}, faces: shaft, tint: &woodTint},
			{from: [3]float64{7.5, 13.7, 7.5}, to: [3]float64{8.5, 16, 8.5}, faces: shaft, tint: &woodTint},
		},
	}
}

// skullTextures 是头颅物品到实体图集的映射。原版把「头」这个 8×8×8 的方块放在
// 图集左上角（偏移 0,0），所以六面 UV 是固定的，换图集即可换生物。
// 尺寸不写死：骷髅与苦力怕是 64×32，僵尸/猪灵/玩家是 64×64。
var skullTextures = map[string]string{
	"skeleton_skull":        "minecraft:entity/skeleton/skeleton",
	"wither_skeleton_skull": "minecraft:entity/skeleton/wither_skeleton",
	"zombie_head":           "minecraft:entity/zombie/zombie",
	"creeper_head":          "minecraft:entity/creeper/creeper",
	"piglin_head":           "minecraft:entity/piglin/piglin",
	"player_head":           "minecraft:entity/player/wide/steve",
}

/* -------- 头颅 --------
 * 原版 head 图集布局：8×8×8 的盒子放在图集偏移 (0,0)，六个面依次是
 *   up(8,0) down(16,0) west(0,8) north(8,8) east(16,8) south(24,8)
 * north 就是脸。头颅物品只显示这个头，所以把盒子放到 4..12（中心 8）再放大。 */
func (r *iconResources) skullModel(name string) (entityModelDef, bool) {
	tex, ok := skullTextures[name]
	if !ok {
		return entityModelDef{}, false
	}
	a := r.atlasOf(tex, 64, 64)
	return entityModelDef{
		atlas: a,
		boxes: []entityBoxDef{{
			from: [3]float64{4, 4, 4}, to: [3]float64{12, 12, 12},
			faces: map[string][4]float64{
				"up":    a.rect(8, 0, 8, 8),
				"down":  a.rect(16, 0, 8, 8),
				"west":  a.rect(0, 8, 8, 8),
				"north": a.rect(8, 8, 8, 8),
				"east":  a.rect(16, 8, 8, 8),
				"south": a.rect(24, 8, 8, 8),
			},
		}},
		guiScale: 1.35,
	}, true
}

/* -------- 盾牌 --------
 * 图集 entity/shield_base.png（64×64）。版式来自原版 ShieldModel：
 *   盾面 12×22×1 @ (0,0) —— 竖着比一个方块还高，GUI 角 15/-25/-5 把它转出立体感
 *   握把 2×6×6  @ (26,0) */
func (r *iconResources) shieldModel() entityModelDef {
	a := r.atlasOf("minecraft:entity/shield_base", 64, 64)
	plate := map[string][4]float64{
		"up":    a.rect(1, 0, 12, 1),
		"down":  a.rect(13, 0, 12, 1),
		"west":  a.rect(0, 1, 1, 22),
		"north": a.rect(1, 1, 12, 22),
		"east":  a.rect(13, 1, 1, 22),
		"south": a.rect(14, 1, 12, 22),
	}
	handle := map[string][4]float64{
		"up":    a.rect(32, 0, 2, 6),
		"down":  a.rect(34, 0, 2, 6),
		"west":  a.rect(26, 6, 6, 6),
		"north": a.rect(32, 6, 2, 6),
		"east":  a.rect(34, 6, 6, 6),
		"south": a.rect(40, 6, 2, 6),
	}
	return entityModelDef{
		atlas: a,
		boxes: []entityBoxDef{
			{from: [3]float64{2, -3, 7}, to: [3]float64{14, 19, 8}, faces: plate},
			{from: [3]float64{7, 5, 8}, to: [3]float64{9, 11, 14}, faces: handle},
		},
		guiRot:   [3]float64{15, -25, -5},
		guiScale: .65,
		guiTrans: [3]float64{2, 3, 0},
	}
}

/* -------- 潮涌核心 --------
 * 图集 entity/conduit/base.png（32×16），原版渲染器里是一颗会转的小方块。
 * 工作台不需要那层动画，取图集里那颗核心的正面块贴到一个 8×8×8 立方体上。 */
func (r *iconResources) conduitModel() entityModelDef {
	a := r.atlasOf("minecraft:entity/conduit/base", 32, 16)
	core := cubeFaces(a.rect(6, 0, 12, 6))
	return entityModelDef{
		atlas:  a,
		boxes:  []entityBoxDef{{from: [3]float64{4, 4, 4}, to: [3]float64{12, 12, 12}, faces: core}},
		guiRot: [3]float64{30, 45, 0},
	}
}

/* -------- 饰纹陶罐 --------
 * 图集 entity/decorated_pot/decorated_pot_base.png（32×32）。原版把罐身、罐颈、
 * 四个耳分开建模；这里收成「窄颈 + 鼓腹」两段，够认出是个陶罐。
 *   罐腹贴图 x2..14, y13..26
 *   罐颈贴图 x0..12, y8..11 */
func (r *iconResources) decoratedPotModel() entityModelDef {
	a := r.atlasOf("minecraft:entity/decorated_pot/decorated_pot_base", 32, 32)
	body := cubeFaces(a.rect(2, 13, 12, 12))
	neck := cubeFaces(a.rect(0, 8, 12, 4))
	return entityModelDef{
		atlas: a,
		boxes: []entityBoxDef{
			{from: [3]float64{1, 0, 1}, to: [3]float64{15, 12, 15}, faces: body},
			{from: [3]float64{2, 12, 2}, to: [3]float64{14, 15, 14}, faces: neck},
		},
		guiRot:   [3]float64{30, 45, 0},
		guiScale: .7,
	}
}

/* -------- 末影龙之首 --------
 * 图集 entity/enderdragon/dragon.png（256×256）。龙头不是「方块实体头颅」那套
 * 8×8×8 的版式，得用原版 EnderDragonModel 的头部盒子：12×18×24 @ 图集偏移
 * (176,44)，龙脸朝 north。 */
func (r *iconResources) dragonHeadModel() entityModelDef {
	a := r.atlasOf("minecraft:entity/enderdragon/dragon", 256, 256)
	return entityModelDef{
		atlas: a,
		boxes: []entityBoxDef{{
			from: [3]float64{2, -1, -4}, to: [3]float64{14, 17, 20},
			faces: map[string][4]float64{
				"up":    a.rect(200, 44, 12, 24),
				"down":  a.rect(212, 44, 12, 24),
				"west":  a.rect(176, 68, 24, 18),
				"north": a.rect(200, 68, 12, 18),
				"east":  a.rect(212, 68, 24, 18),
				"south": a.rect(236, 68, 12, 18),
			},
		}},
		guiRot:   [3]float64{30, 225, 0},
		guiScale: .45,
	}
}

// entityModel 按物品 id 分派到对应的合成模型。命中返回 true。
// 入参可能是物品 id（minecraft:chest）也可能是模型键（minecraft:item/chest）——
// icons() 走的是后者，而 reason 里对外暴露的是前者，两种都得认。
func (r *iconResources) entityModel(id string) (iconModel, bool) {
	key := resourceID(id)
	if !strings.HasPrefix(key, "minecraft:") {
		return iconModel{}, false
	}
	name := strings.TrimPrefix(strings.TrimPrefix(key, "minecraft:"), "item/")
	switch {
	case name == "chest" || name == "trapped_chest" || name == "ender_chest":
		return r.chestModel(name).iconModel(), true
	case name == "shulker_box":
		return r.shulkerModel("").iconModel(), true
	case strings.HasSuffix(name, "_shulker_box"):
		return r.shulkerModel(strings.TrimSuffix(name, "_shulker_box")).iconModel(), true
	case strings.HasSuffix(name, "_bed"):
		return r.bedModel(strings.TrimSuffix(name, "_bed")).iconModel(), true
	case strings.HasSuffix(name, "_banner"):
		return r.bannerModel(strings.TrimSuffix(name, "_banner")).iconModel(), true
	case strings.HasSuffix(name, "_head"), strings.HasSuffix(name, "_skull"):
		if m, ok := r.skullModel(name); ok {
			return m.iconModel(), true
		}
		// 末影龙的首不在头颅图集那套版式里，单独一支。
		if name == "dragon_head" {
			return r.dragonHeadModel().iconModel(), true
		}
	case name == "shield":
		return r.shieldModel().iconModel(), true
	case name == "conduit":
		return r.conduitModel().iconModel(), true
	case name == "decorated_pot":
		return r.decoratedPotModel().iconModel(), true
	}
	return iconModel{}, false
}
