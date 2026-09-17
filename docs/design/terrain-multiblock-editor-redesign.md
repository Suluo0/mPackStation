# 地形 / 多方块编辑能力重设计

- 状态：设计稿（VK3，待用户拍板后实现）
- 日期：2026-09-18
- 关联：
  - 遗留资产：`legacy-assets/frontend/multiblocks/*`（IsoPreview / GridPreview / MultiblocksPage）
  - 产品内容域：P6 `content` kind = `recipe | structure | ore`（`docs/architecture/backend-architecture-v7.md`）
  - 包内目录：物品/方块/配方/标签/多语言（migrations 0013–0014）
  - 接入计划：`docs/design/legacy-assets-integration-plan.md`
  - 现状缺口：`ContentEditorPage` 未挂路由；M6 `ModContentPage` 只做「解析浏览」

## 1. 设计目标

把「地形生成（ore/worldgen）」与「多方块结构（structure）」从**只读解析/JSON 原文**升级为整合包工作台内的一等编辑能力：

1. **包作用域**：所有编辑对象挂在 `pack_id` 下，与模组清单、目录、任务书同一权威域  
2. **修订式安全**：draft → validate → apply → history/rollback，If-Match 乐观锁（后端 P6 已具备）  
3. **所见即所得**：2D 分层网格 + 2.5D 轴测预览共用同一份 structure 数据  
4. **目录驱动选块**：方块/物品候选来自包内 catalog（含模组与原版），禁止手滑写错 ID  
5. **信号驱动**：校验问题用红/黄/青信号汇总，不让用户面对原始堆栈  

非目标（本期不做）：完整游戏内地形仿真、运行时 dump 动态结构、跨包模板市场。

## 2. 信息架构

```text
包工作台
└─ 内容编辑（一级）
   ├─ 浏览（现有 ModContentPage：jar 解析结果）
   ├─ 结构 Structure Editor（新）
   └─ 地形 Ore / Worldgen Editor（新）
        └─ 共享：修订条、校验面板、目录选择器、预览画布
```

路由建议：

| 路由 | 组件 | 说明 |
|---|---|---|
| `/packs/:id/content` | `ModContentPage` | 默认 Tab「模组内容」 |
| `/packs/:id/content/structures` | `StructureEditorPage` | 多方块结构 |
| `/packs/:id/content/ores` | `OreEditorPage` | 矿脉/地形生成 |
| `/packs/:id/content/docs/:docId` | `ContentDocPage` | P6 文档修订（recipe/structure/ore 通用壳） |

## 3. 数据模型（对齐后端，不新造平行真相）

### 3.1 Structure 文档 payload（P6 `structure`）

在现有校验字段上**扩展可选编辑元数据**，保持向后兼容：

```jsonc
{
  "schema_version": 2,
  "file": "ae2:spatial_pylon",          // 必填：逻辑结构 ID
  "size": {"x": 3, "y": 3, "z": 3},     // 必填
  "anchor": {"x": 1, "y": 0, "z": 1},   // 控制器/锚点
  "rotation": 0,                          // 0/90/180/270
  "layers": [                             // layers[y][z][x] = blockId | tag | null
    [["ae2:sky_stone_block", null, "..."], "..."],
    ["..."]
  ],
  "palette": {                            // 可选：调色板，编辑器用
    "P": "ae2:sky_stone_block",
    "C": "ae2:controller"
  },
  "cells": [                              // 可选：稀疏表示，与 layers 二选一或互相同步
    {"x":1,"y":0,"z":1,"block":"C"}
  ],
  "rules": {                              // 可选：多方块匹配规则提示
    "require_anchor": true,
    "symmetry": "none"                    // none|x|z|xz
  },
  "preview": {"mode": "iso", "showAir": false},
  "metadata": {"source": "editor", "modId": "ae2"}
}
```

**约定**：
- `layers` 与 `cells` 编辑器内部可互转；**落库 canonical 以 `layers` 为准**（校验器已有 size/file 强制项）
- 空气统一 `null` 或 `"minecraft:air"`，规范化时折叠为 `null`
- 方块 ID 必须能被包 catalog 解析，或显式 `tag:` 前缀

### 3.2 Ore / Worldgen 文档 payload（P6 `ore`）

```jsonc
{
  "schema_version": 2,
  "dimension": "minecraft:overworld",
  "block": "ae2:quartz_ore",
  "min_y": 0,
  "max_y": 60,
  "count": 8,
  "frequency": 0.5,
  "distribution": "uniform",      // uniform | triangle | gaussian
  "biomes": ["#minecraft:is_overworld"],
  "replace": ["minecraft:stone"],
  "vein": {"min": 4, "max": 12},  // 可选增强
  "metadata": {"source": "editor"}
}
```

**校验信号（前端即时）**：
- `min_y > max_y` → 红（后端已有 `ore_range_invalid`）
- `block` 不在包目录 → 黄「未知方块，应用可能失败」
- `dimension` 非法 → 红
- 频率/次数 ≤0 → 红

## 4. 界面设计

### 4.1 结构编辑器（Structure）

```text
┌──────────────────────────────────────────────────────────┐
│ 包上下文 │ 内容编辑 / 结构  [修订 3 · draft] [校验] [应用] │
├───────────────┬──────────────────────────────────────────┤
│ 文档列表       │  2D 分层网格（主编辑）                     │
│ · pylon       │  Y= [0|1|2]  旋转 [↶][↷]  显示空气 [ ]     │
│ · meteorite   │  ┌─────────────────────┐                  │
│ + 新建结构     │  │ 网格 cell → 调色板选择 │                  │
│               │  └─────────────────────┘                  │
│ 过滤: 关键字   │  选中: (1,0,1) = C 控制器                   │
│               ├──────────────────────────────────────────┤
│               │  2.5D 轴测预览（只读/可拖控制器）            │
│               │  [全部层|当前层] 占用 N 方块                │
├───────────────┴──────────────────────────────────────────┤
│ 右侧：调色板（catalog 搜索）· 控制器坐标 · 规则 · 校验问题   │
└──────────────────────────────────────────────────────────┘
```

**交互要点**（继承 legacy IsoPreview/GridPreview，但升级）：
| 能力 | v1 | 说明 |
|---|---|---|
| 分层网格点选/刷子 | ✓ | 单格、整层填充、矩形刷 |
| 拖拽交换 | ✓ | 层内交换；跨层移动（legacy 仅层内） |
| 调色板 | ✓ | 来自 `GET /api/packs/:id/catalog`，支持模组过滤、最近使用 |
| 控制器拖拽 | ✓ | 锚点与 `anchor` 字段绑定 |
| 对称绘制 | ✓ | 沿 X/Z 镜像 |
| 轴测预览 | ✓ | 纯 CSS 3D，上限渲染块数，超限提示 |
| 模板 | ○ | 从已应用 structure 复制为新文档 |
| 导入 nbt/schematic | ✗ | 二期 |

### 4.2 地形/矿脉编辑器（Ore）

```text
┌──────────────────────────────────────────────────────────┐
│ 修订条：draft r2 · 校验 0 错 · [校验] [应用] [历史]        │
├─────────────────────────────┬────────────────────────────┤
│ 表单                         │  分布可视化（示意图）         │
│ 维度 [overworld ▾]          │   Y 轴范围条 min——max       │
│ 方块 [catalog 搜索]          │   ●●●●●  脉簇示意          │
│ Y  [min] [max]              │   频率/次数读数              │
│ 次数 count                  │                            │
│ 频率 frequency              │  生物群系标签 chips          │
│ 分布 [uniform ▾]            │  替换方块列表                │
├─────────────────────────────┴────────────────────────────┤
│ 校验面板：红/黄问题列表，点击定位到字段                      │
└──────────────────────────────────────────────────────────┘
```

不做假 3D 地形模拟；Y 范围条 + 统计读数 + 生物群系 chips 足够指导设计，且不撒谎。

### 4.3 共用「修订条」

所有 P6 文档页统一：
- 状态：`draft / validated / applied` + revision 号  
- 操作：校验、应用、回滚、历史抽屉  
- 冲突：412 → 「内容已被修改，刷新后重试」（已有前端语义）  
- 信号：错误数红、警告数黄、干净青绿  

## 5. 后端接入（最小增量）

### 5.1 已具备（不重写）
- `POST/GET /api/packs/:id/content[...]` 全套修订 API  
- structure/ore canonical 字段校验  
- catalog：物品/方块/标签/中文名/图标  
- 任务：目录重建、内容解析  

### 5.2 需要的增量

| 项 | 说明 |
|---|---|
| A. 前端路由与页面 | 挂上 Structure/Ore 编辑页；`ContentEditorPage` 退役或改壳 |
| B. payload 规范化 | service 侧 canonical 时保留 `layers/palette/cells/rules` 扩展字段（校验器忽略未知可选字段即可） |
| C. catalog 选块 API | 已有 `GET /catalog?locale=`；补充 `q`/`modId` 过滤参数以服务调色板搜索 |
| D. 预览图标 | 复用 `item_icon` / catalog icon resolve；structure cell 显示方块图标而非纯文本 |
| E. 示例种子 | 为测试包提供 1 个 structure + 1 个 ore 样例文档，便于验收 |
| F. 契约测试 | content structure/ore 扩展字段的 fixture + curl 矩阵两项 |

**明确不做**：新表、新 kind、独立 multiblocks API（legacy 的 `/api/multiblocks` 不迁移，避免双真相）。

## 6. 与 legacy 资产的映射

| Legacy | 去向 |
|---|---|
| `IsoPreview.tsx` | 迁到 `apps/web/src/features/structure/IsoPreview.tsx`，props 改为 P6 payload + catalog 图标 |
| `GridPreview` / `GridEditor` | 合并为 `LayerGrid.tsx`，增加刷子/对称/跨层 |
| `MultiblocksPage` 主从布局 | 参考 Master-Detail，但数据源换成 `listContent(packId,'structure')` |
| `styles-multiblock.css` | 并入 `pack-pages.css` 或 `structure.css`，沿用工作台令牌 |
| `/api/multiblocks` | **废弃**，不实现 |

原则（接入计划原文）：**前端可平移，后端只重写逻辑**；平台/内容真相在 P6 与 catalog。

## 7. 验收标准

### 结构
1. 在包内创建 structure 文档，尺寸 3×3×3，网格可点选调色板方块  
2. 设置控制器坐标后轴测预览高亮 C  
3. 保存 draft → 校验通过 → 应用 → history 可见  
4. 故意破坏 size 与 layers 不一致 → 校验红灯且不能应用  
5. 调色板能搜到包内模组方块（如 AE2）并显示中文名/图标  

### 地形
1. 创建 ore：dimension/block/min_y/max_y 必填校验生效  
2. `min_y>max_y` 前后端均拒绝  
3. 未知方块 ID 给黄警告但仍允许保存 draft（应用前可再拦）  
4. 应用后 delivery-checks 的 content 项保持可计算  

### 回归
1. `go test ./...`、`npm run build` 全绿  
2. 原 M6 浏览页不回归  
3. 契约矩阵新增 structure/ore 编辑字段用例  

## 8. 分期

| 期 | 内容 | 产出 |
|---|---|---|
| **S1 设计冻结** | 本文件评审 | 拍板 payload 与路由 |
| **S2 结构 MVP** | 路由 + LayerGrid + IsoPreview + catalog 调色板 + P6 读写 | 可编辑 structure |
| **S3 地形 MVP** | Ore 表单 + Y 可视化 + 校验面板 | 可编辑 ore |
| **S4 打磨** | 图标、对称刷、模板复制、契约测试、截图验收 | 可演示闭环 |

## 9. 风险与决策点（需拍板）

1. **canonical 表示**：layers 优先还是 cells 稀疏优先？（建议 layers）  
2. **是否允许 tag 方块**在 structure 中？（建议允许 `#ns:tag`，校验时 warning）  
3. **Ore 是否要做多矿脉列表**在一个文档内？（建议 v1 单脉/文档，多脉多文档）  
4. **编辑器是否写回 jar 解析的只读内容**？（建议只编辑 P6 文档，jar 内容保持只读溯源）  
5. **视觉密度**：继续工作台信息密度，还是允许更大的画布比例？（建议画布区 ≥50% 宽）  

## 10. 成功标准（产品）

用户在不启动游戏、不打开外部编辑器的前提下，能在 mPackStation 内：
- 搭一个多方块结构草案并看到轴测预览  
- 定一条矿脉分布并通过校验  
- 应用修订后进入打包检查链路  

—— 与「流水线终局通向启动 Minecraft」一致：内容编辑是可验证的中间产物，不是装饰。
