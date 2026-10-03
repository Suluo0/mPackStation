# 物品图标渲染器补齐方案（2026-10-03）

目标：把目录里 184 个「没有图标」的物品按可核查的成本逐层拿下，并让界面能回答「这个为什么没有图标」。
范围：**全部是后端 Go 改动**（`apps/server/internal/service/`），前端零改动即可显示。按用户规矩，未点名授权不动后端；本文件只是方案。

## 0. 计量口径

- 数据面：本机干净实例 `127.0.0.1:18880`，数据目录 `/tmp/mpack-clean`，包 `pack-06afb18714826bf9c213416d`，目录 revision 19730。
- 分母：`pack_catalog_items` 全表 1711 行（不是「模组声称的物品数」，是当前目录已解析出的物品）。
- 图标数：`pack_catalog_items.icon_status`；渲染来源：`pack_catalog_item_icons.source`。
- 分类方法：按 `item_model_resources.go` + `item_icon_render.go` 的判定规则用 Python 复刻一遍（`/tmp/mkn/icon_classify.py`），对 1711 条逐条判 ready/missing，**与生产 `icon_status` 零不一致**，所以下面的原因分布可信。

## 1. 现状（实测，先纠正旧结论）

| 事实 | 数 |
|---|---|
| 目录物品 | 1711 |
| `icon_status=ready` | 1527（89.2%） |
| `icon_status=missing` | 184（10.8%） |
| ready 里走 `generated`（layer0 平面贴图） | 773 |
| ready 里走 `elements`（3D 等距烘焙） | 754 |

**渲染器已经存在并且在跑**：`apps/server/internal/service/item_icon_render.go:32 renderBlockModel` 是手写的软件光栅器——32×32、逐面 UV、元素旋转（含 `rescale`）、`display.gui` 变换继承、`gui_light`、面剔除、alpha 混合、深度缓冲消隐、动画条图取首帧。产物已肉眼核验：`/tmp/mkn/icon_mekanism_enrichment_chamber.png`（富集仓，正面端口可见）、`/tmp/mkn/icon_minecraft_stone.png`（石头立方体）。
所以「看不到图标 ≠ 缺渲染器」。上一轮我说「方块模型要 3D bake 所以今天渲染不了」，此句作废。

## 2. 根因（就一行）

`apps/server/internal/service/item_model_resources.go:167`

```go
if json.Unmarshal(raw, &child) != nil || child.Loader != "" {
    return iconModel{}, false
}
```

只要模型 JSON 带 `loader` 字段就整体放弃。184 个缺口的原因分布：

| 原因 | n | 代表物品（官方简中名，取自 jar 内 `assets/mekanism/lang/zh_cn.json`） |
|---|---|---|
| `builtin/entity` 父模型无文件 | **80** | 原版 62：箱子、床×16、潜影盒×17、旗帜×16、头颅×5、盾牌、潮涌核心、饰纹陶罐、切石机、重型核心；Mekanism 18：能量立方×4、流体储罐×4、喷气背包、地面轻行者、火焰弹射器、原子拆解器、meka 套装 |
| `neoforge:composite` | **44** | 六类工厂全档位（如「高级粉碎工厂」）、热中子合成器 |
| `mekanism:transmitter` | **22** |  Universal 电缆 / 机械管道 / 物流传送器 / 液压管 ×4 档 |
| `neoforge:fluid_container` | **18** | 氢氟酸桶、液态钠桶、重水桶、蒸汽桶… |
| `requires_tint` | **15** | 树叶×8、草方块、QIO 面板/驱动阵列/导出器/导入器/红石适配器 |
| 单例 | 5 | `neoforge:item_layers` 1、`neoforge:separate_transforms` 1、`mekanism:robit` 1、`bounding_block`（父链无 elements）、`heavy_core`（引用非贴图资产） |

JEI 为什么全都有：它在游戏进程内跑原版 ModelBakery + modded loader + BER + tint 源，几何和颜色由 Java 代码现算。我们离线，只能逐个 loader 逆向复刻。

## 3. 工单（按性价比排序，每条独立可验收）

### R0 把「为什么没有」落库（收益 0 个图标，但这是本轮问题的产品化）

现状 `icons()` 只返回 id 列表，原因（`unsupported_model` / `missing_texture` / `requires_tint` / `model_missing:xxx` / `loader:xxx`）在 `item_icon_render.go` 里算完就丢。

- `item_model_resources.go:270-290`：`missing = append(missing, id)` → 同时记 `reason[id] = source`。
- `mod_content_icons.go:15-21` `ModContentIcons` 增字段（**纯新增，不破 JSON 契约**）：

```go
MissingReasons map[string]string `json:"missingReasons,omitempty"`
```

- `item_catalog.go:790-793` 写 `IconStatus="missing"` 处一并写原因；新增迁移 `internal/store/migrations/0026_catalog_item_icon_reason.sql`：

```sql
ALTER TABLE pack_catalog_items ADD COLUMN icon_reason TEXT NOT NULL DEFAULT '';
```

- 前端可选（1 行）：`ItemGrid.tsx:62` 的占位 `title` 挂上 `it.iconReason`，鼠标停上去就说清「运行时生成模型，静态无法还原」。

验收：`sqlite3 /tmp/mpack-clean/mpackstation.db "select icon_reason,count(*) from pack_catalog_items where pack_id='pack-06afb18714826bf9c213416d' and icon_status='missing' group by 1 order by 2 desc"` 有值且分布与本文件 §2 表一致。

### R1 loader 白名单放行（+2 → 1529）

`item_layers` / `separate_transforms` 的 JSON 本身就带可画内容（`portable_qio_dashboard.json`：`parent: minecraft:item/generated` + `layer0/layer1` 两张现成贴图）。把 `:167` 的「见 loader 即死」改成按 loader 分派：

```go
switch child.Loader {
case "neoforge:item_layers", "mekanism:data_based":
    // JSON 自带 textures/elements，loader 只影响发光与光照，图标忽略。
case "neoforge:separate_transforms":
    if child.Base != nil { /* 用 base 子模型继续解析 */ }
case "neoforge:composite":
    // 见 R2
default:
    return iconModel{}, false
}
```

`iconModel` 增字段（`item_model_resources.go:132-140`）：

```go
Children map[string]modelChildRef  `json:"children"`
Base     *modelChildRef            `json:"base"`
```

```go
type modelChildRef struct {
    Parent   string            `json:"parent"`
    Loader   string            `json:"loader"`
    Textures map[string]string `json:"textures"`
}
```

验收：新增测试 `TestSeparateTransformsAndItemLayers`，断言 `mekanism:item/portable_qio_dashboard` 出图且 `source=="generated"`。

### R2 composite 子模型合并（+44 → 1573，91.9%）

实测 `neoforge:composite` 的 JSON 就是「命名子模型求并」，子模型全是现成 block 模型，几何数据齐：

```json
// assets/mekanism/models/block/factory/crushing/advanced.json
{"loader":"neoforge:composite","parent":"block/block",
 "textures":{"particle":"mekanism:block/factory/crushing/crushing_factory_front"},
 "children":{"base":{"parent":"mekanism:block/factory/crushing/base"},
             "front_led":{"parent":"mekanism:block/factory/front_led/advanced"}}}
```

依赖已核实：`assets/minecraft/models/block/block.json` 在库（提供 `gui` 变换与 `gui_light:side`）；`mekanism:block/factory/crushing/base.json` 在库且自带 `elements`。

实现两件事：
1. 合并前把每个子模型的 `#变量` 就地解成具体贴图（否则子模型间同名变量会互相踩）：

```go
func inlineVars(m iconModel) iconModel {
    for i, el := range m.Elements {
        for dir, f := range el.Faces {
            f.Texture = lookupVar(f.Texture, m.Textures)
            m.Elements[i].Faces[dir] = f
        }
    }
    return m
}
```

2. 按 key 字典序遍历 `Children`（**保证确定性**），`r.model(ref.Parent, freshSeen)` 解析后 `result.Elements = append(result.Elements, cm.Elements...)`，`Textures` 只补缺不覆盖。渲染来源标记为 `composite`（`encodeItemIcon` 第三参）。

不处理：`render_type`（cutout/transparent 混合模式）与 `neoforge_data.block_light/sky_light`（自发光）——图标层面不影响形状，`front_led` 的发光差异标在 R0 的 reason 里备查。

验收：新增 `TestCompositeChildMerge`，断言 `mekanism:item/advanced_crushing_factory` 出图、非透明像素 > 30%、`source=="composite"`；前端 `http://127.0.0.1:5275/?pack=pack-06afb18714826bf9c213416d&mode=index` 搜「工厂」，六档图标齐。

### R3 `builtin/entity`：不做，只说清（0，剩 80）

jar 里**没有任何几何数据**——顶点由 Java 代码运行时生成（箱子开合、旗帜图案、盾牌纹章、潜影盒配色都是 BER/代码）。静态复刻只有两条路：
- 游戏内捕获（渲染每类 BER 一帧存 PNG 资产包）→ 被 **E2 缺 `mpack-launcher` 二进制**卡住；
- 自绘近似几何 → **明确禁止**，等于用我们画的图冒充官方素材（与「中文名必须来自 lang 文件」同源的原则）。

本方案对这 80 个只做一件事：R0 把 reason 写成 `runtime_generated_model`，界面显示带说明的占位（`ItemGrid.tsx` 现在是首字占位，够用）。

### R4 OBJ 网格（+22 → 1595，93.2%）

好消息：`.mek` **就是标准 Wavefront OBJ**，实测 `assets/mekanism/models/block/transmitter_large.obj.mek` 开头 `# Blender v2.76 (sub 0) OBJ File:`，360 个 `v`、233 个 `vt`、270 个面且**全是四边形**（`f` 顶点数分布 Counter({4: 270})），组名 `o downNONE / eastPULL / …` 编码连接朝向。

坏消息（成本都在这里，必须写进排期）：
1. 网格资产现在根本没入库：`mod_content_kind.go:61` 只放行 `.json/.nbt/.png`，实测 `mod_content` 里 `.obj/.mtl/.mek` 行数 = **0**。要改扩展名白名单 + 新增 kind `mesh` + 迁移改 `mod_content.kind` 的 CHECK 约束（约束是硬编码枚举，见 `internal/store/schema.sql`）+ `appendContentItem`（`mod_content_extract.go:169-194`）+ `iconResources` 装 `meshes map[string][]byte`（`item_model_resources.go:40-61` 的 `addContent` 与 `:62-107` 的 `addClientJar` 两处）。
2. 光栅器现在只吃 `modelElement`（轴对齐 from/to）。需要加一条通用四边形通路：`iconModel` 增 `Raw []rawQuad{P [4][3]float64; UV [4][2]float64; Tex string; Light float64}`，`renderBlockModel`（`item_icon_render.go:47` 的 `for _, el := range m.Elements` 之后）把 Raw 也 append 进 `faces`。
3. 材质→贴图：优先用 JSON `textures` 映射（`#center_down` 等），`.mtl` 的 `map_Kd` 兜底。
4. 朝向：inventory 默认取四向 `*NONE` + center 组（无连接直管姿态）。**这条必须拿 JEI 实图对一遍**再定，对不上就在 `source` 里写 `obj_approx` 并走 R0 的 reason 标注。

验收：`TestObjMeshQuads` 断言 `mekanism:item/advanced_universal_cable` 出图；与 JEI 截图人工比对一次，比对结论写进本文件 §8。

### R5 tint 近似（+15 → 1610，94.1%）

`item_icon_render.go:59` 现在是主动拒绝（注释原话：染色处理器在游戏代码里，不要把未染色图当准确）。改成「按固定色乘一遍 + 明确标近似」：

```go
if f.TintIndex != nil && *f.TintIndex >= 0 {
    if tint == (color.NRGBA{}) { return nil, "requires_tint" } // 表里没有仍拒绝
    c = mulTint(c, tint)
}
```

颜色表**不许我拍**：新建 `internal/service/icon_tint.go`，每个值必须带来源注释（原版 `ColorResolvers#GRASS` 一类常量或官方 wiki），实现时先核实再写死；核实不到的键保持拒绝。`source` 记 `elements_tint` / `generated_tint`，前端角标（可选 1 行）显示「近似」。

### R6 化学桶（+18 → 1628，95.1%，只能到「空桶轮廓」）

`brine_bucket.json` = `{"parent":"neoforge:item/bucket","fluid":"mekanism:brine","loader":"neoforge:fluid_container"}`。两个硬事实：
- `neoforge:item/bucket` 模型**既不在模组 jar 也不在原版客户端 jar**（实测 jar 内只有 18 个 `*_bucket.json` 物品模型），得像 `item_model_resources.go:154-162` 那段 `minecraft:block/cube_all` 一样内置兜底模型；
- Mekanism jar 内**流体 still/flowing 贴图 = 0 张**（实测），液体颜色在化学 registry 的 Java 代码里。

所以 R6 交付的是「桶轮廓」，填充色推后（要嘛化学颜色表，要嘛 R3 的游戏内捕获）。价值判断：链路视图里「氢氟酸桶」这类节点能出现形状但没颜色，比整格空白好，但必须标 `fluid_container_approx`。

## 4. 累计与顺序

| 顺序 | 工单 | 新增图标 | ready 累计 | 覆盖率 | 主要成本 |
|---|---|---|---|---|---|
| — | 现状 | — | 1527 | 89.2% | — |
| 1 | R1 loader 放行 | +2 | 1529 | 89.4% | ~15 行 |
| 2 | R2 composite | +44 | 1573 | 91.9% | ~40 行 |
| 3 | R0 原因落库 | 0 | 1573 | 91.9% | 迁移 + 3 处 |
| 4 | R5 tint 近似 | +15 | 1588 | 92.8% | ~40 行 + 颜色表核实 |
| 5 | R4 OBJ 网格 | +22 | 1610 | 94.1% | ~180 行 + 抽取/mesh kind + 迁移 |
| 6 | R6 化学桶 | +18 | 1628 | 95.1% | ~120 行，且只到近似 |
| 剩余 | R3 `builtin/entity` 80 + `robit` + `bounding_block` + `heavy_core` | — | 1628 | 95.1% | 静态无解，等 E2 |

建议先只做 R1+R2+R0（约 55 行 + 1 迁移）：一次拿下 44 个工厂图标（链路视图里最显眼的一类节点），并让剩下的一律「有原因可查」。R4/R6 涉及抽取层与迁移，单独排期。

## 5. 明确不做 + grep 归零

- 不碰解析器类型白名单（`item_catalog.go:298-329`，那是 E9）、不碰配方边、不碰链路引擎（另见 `craft-chain-engine-plan-2026-10-03.md`）。
- 不自绘 `builtin/entity` 几何冒充官方素材。
- 不引入任何前端渲染依赖（不在浏览器里跑 WebGL/three.js 复刻模型——数据都在后端，前端拿 PNG 是现有契约）。
- 不做 i18n 框架（本文件里的中文名一律来自 jar 的 `lang/zh_cn.json`）。
- 回滚核查（改完必须只剩预期命中）：

```
grep -rn "neoforge:composite\|inlineVars\|rawQuad\|MissingReasons\|icon_reason" apps/server/internal apps/web3/src
grep -rn "requires_tint\|unsupported_model" apps/web3/src   # 必须为空：原因文案不硬编码在前端
```

## 6. 总验收

```
cd /Volumes/Evo/code/mPackStation/apps/server && go build ./... && go test ./internal/service/ -run 'Icon|Cube|Block|Generated|Composite|Obj|Tint' -count=1
```

现有 7 个渲染测试必须继续全绿：`TestCubeIconUpright`、`TestIconModelInheritanceAndNamespace`、`TestCubeIconUsesCorrectFaces`、`TestVanillaIconsCachedAndPackScoped`、`TestBlockUVAndSlabGeometry`、`TestGeneratedLayersAndAnimationFrame`、`TestIconTagRepresentativesAndCycles`（`internal/service/item_icon_render_test.go`）。

渲染器行为一变，`store/catalog_generation_repo.go:158` 写死的 `'item-icon-v2'` 必须升到 `v3`，否则旧图标按 `renderer_version` 复用、新代码不生效。

前端（唯一验收入口，5275）：
1. `http://127.0.0.1:5275/?pack=pack-06afb18714826bf9c213416d&mode=index` → 索引网格搜「工厂」，六档粉碎/富集/压缩/注入/纯化/锯木工厂出图；
2. 搜「氢氟酸」看桶（R6 之后）；
3. 悬停无图标格 → 标题里能看到原因（R0 之后）；
4. 复核 SQL：

```
sqlite3 /tmp/mpack-clean/mpackstation.db "select icon_status,count(*) from pack_catalog_items where pack_id='pack-06afb18714826bf9c213416d' group by 1"
```

期望 ready ≥ 1573（R1+R2 后）、missing 里 `builtin/entity` 恒为 80。

## 7. 风险

- 图标接口 `Cache-Control: private, max-age=3600`（`httpapi/routes_catalog.go:16`），验证时要么换 itemId 要么硬刷，别把缓存当成「没生效」。
- 重建目录要显式触发（`POST .../catalog/rebuild`，界面 = 模组行 ▸ → 解析 → 目录重建）；E8 的 jar 不持久化会让每次解析重新下 jar，这是既有缺陷，不是本方案引入。
- R4 的 transmitter 姿态若与 JEI 不一致，宁可退回 `missing` 也不要出错误形状——用户拿它认机器，认错比空白更糟。

## 8. 待填（实现后补，不许留空交付）

- [ ] transmitter inventory 姿态与 JEI 比对结论（截图路径 + 判定）
- [ ] tint 颜色表逐键来源（原版类名/官方 wiki 链接）
- [ ] R1–R6 跑完后的真实 `icon_status` 分布 + `source` 分布（替换 §4 的预测值）
