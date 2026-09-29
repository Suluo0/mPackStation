# 配方浏览器（JEI 对等）设计方案

- 状态：设计稿 v1（**待评审，未开发**）
- 日期：2026-09-21
- 产品目标：在 mPackStation 内提供与 **JEI 行为对等**的配方/物品浏览体验——含**快捷键**、**收藏**，并额外具备**最近访问**
- 范围：包内已解析物品/配方目录的浏览与交互；不嵌入真实 Minecraft 进程
- 非范围：游戏内注入、JEI 服务端协议、完整 mod 配方模拟器（special 以展示模板逼近）

---

## 0. 用户预期（验收基准）

1. **与 JEI 行为相同**（交互心智、快捷键、收藏）  
2. **最近访问**（JEI 无此功能，我们补上）  
3. special 配方（如 `banner_duplicate`）展示**接近 JEI**，而不是「暂不支持」文案墙  
4. 先方案，评审通过后再开发  

---

## 1. 信息架构

```text
配方浏览器（独立路由或内容编辑内「配方」主 Tab）
├─ 顶栏：搜索框 · 模式徽章（配方/用途）· 收藏数 · 最近访问入口
├─ 左栏：物品索引列表（可滚动，按搜索过滤）
├─ 中央：配方画布（JEI 式槽位 + 页码/类型切换）
├─ 右栏/浮层：收藏夹 · 最近访问
└─ 底栏（可选）：当前物品 id · 来源模组 · 快捷键提示
```

### 路由建议

| 路由 | 说明 |
|---|---|
| `/packs/:id/recipes` | 包级配方浏览器（主入口） |
| `/packs/:id/content` 内「配方」Tab | 可嵌入同一组件，或跳转到上面路由 |

### 与现有关系

- 替代当前 `ModContentPage` 里「JEI 视图」弹窗的**碎片化体验**  
- 数据仍来自 `mod_content` + `pack_catalog_*`（物品名/图标/配方 payload）  
- special 配方用 **JEI 式展示模板**（见 §6），不是运行时 Java  

---

## 2. JEI 行为对照（必须对齐）

### 2.1 物品索引与搜索

| JEI | 我们 |
|---|---|
| 物品列表（可滚动） | 左栏 catalog 物品列表（图标 + 中文名 + 模组前缀） |
| 搜索框过滤 | 同；支持子串 + 前缀修饰（§2.2） |
| 点击物品 → 物品面板/配方 | 点击 → 中央显示该物品「如何合成」 |
| 无结果空态 | 「无匹配物品」+ 清除搜索 |

### 2.2 搜索前缀（JEI 惯用，我们做子集）

| 前缀 | 语义 | 示例 |
|---|---|---|
| （无） | 名称/id 模糊 | `安山岩` / `andesite` |
| `@` | 按模组/命名空间 | `@minecraft` `@ae2` |
| `#` | 按标签/键名 | `#planks`（对齐 catalog tags） |
| `$` | 按技术 id 后缀 | `$stairs` |

实现：解析前缀 → 过滤 catalog；无效前缀当普通字面量。

### 2.3 配方 / 用途 双模式（核心快捷键）

| 键（对齐 JEI） | 行为 |
|---|---|
| **`R`** | 对当前选中物品：显示 **Recipes（怎么合成/获得）** |
| **`U`** | 对当前选中物品：显示 **Uses（可被谁用掉/作为材料）** |
| **`Esc`** | 关闭浮层/清空选中；再按可收起侧栏 |
| **`←` / `→`** 或 **`[` / `]`** | 同一物品多个配方/用途**翻页** |
| **`F`** 或 **`Ctrl+D`** | **收藏/取消收藏**当前物品 |
| **`H`** | 打开/关闭 **收藏夹**（History/Bookmarks 面板） |
| **`T`** | 打开/关闭 **最近访问**（Today/History；我们定 `T`=最近） |
| **`/`** | 聚焦搜索框 |
| **`Enter`** | 搜索框提交；列表上 = 打开选中项 |
| **`↑` `↓`** | 物品列表移动选中 |
| **`Ctrl+Shift+R`** | 对当前物品强制刷新配方数据（重查 API） |

> 快捷键在搜索框/输入框内**不劫持**字母；仅在画布/列表焦点或全局非输入时生效（与 infinite-canvas Space 规则一致）。

### 2.4 配方类型切换（JEI catalyst / 类别）

顶部或配方区显示**处理类型 chip**（来自 payload `type`）：

| chip | 对应 |
|---|---|
| 合成 | `crafting_shaped` / `shapeless` |
| 切石机 | `stonecutting` |
| 熔炉/高炉/烟熏/营火 | `smelting` 等 |
| 锻造台 | `smithing_*` |
| 特殊 | `crafting_special_*`（模板展示） |

- 点击 chip 过滤该物品的配方子集  
- 有多条时左右翻页，角标 `2/7`  

### 2.5 槽位多物品轮换（JEI 可点 slot cycle）

- 同一槽位若是**标签/多种候选**：点击或 **`C`** 循环候选  
- 角标显示 `n/m`  
- v1：对 `#tag` 与 special「任意染料」类做轮换  

### 2.6 收藏（Bookmarks）

| 项 | 设计 |
|---|---|
| 入口 | 物品卡星标 · `F` · 右栏收藏列表 |
| 内容 | `itemId` + 快照（name/icon/modId）+ 添加时间 |
| 排序 | 手动拖拽（P2）/ 默认添加时间倒序 |
| 操作 | 取消收藏 · 点击跳转到该物品 Recipes · 清空 |
| 空态 | 「尚未收藏，选中物品按 F」 |

### 2.7 最近访问（我们额外要求）

| 项 | 设计 |
|---|---|
| 记录时机 | 打开某物品的 Recipes/Uses、搜索并选中、从收藏跳转 |
| 条目 | itemId、名称、图标、**上下文**（recipes\|uses）、时间戳 |
| 容量 | 默认 **50** 条，FIFO 去重（同 item+context 只保留最新） |
| 展示 | 右栏「最近」列表；顶栏可展开时间线 |
| 操作 | 单条删除 · 清空 · 点击回放 |
| 与收藏关系 | 可一键从最近 → 收藏 |

### 2.8 槽位交互（对齐 JEI 细节）

| 行为 | 说明 |
|---|---|
| 左键槽位物品 | 选中该物品，进入其 Recipes（`R` 语义） |
| 右键槽位 | Uses（`U` 语义） |
| Hover | 物品名 + id + 来源模组 |
| 输出槽点击 | 同左键 → 该产物的 Recipes |
| 空槽 | 可点「选择物品」（从搜索/收藏插入，便于作者配包）——**可选 P2** |

---

## 3. 配方展示（JEI 视觉语义）

### 3.1 合成类

```text
┌───────┬───────┬───────┐
│ slot  │ slot  │ slot  │
├───────┼───────┼───────┤      →    [ output ]
│ slot  │ slot  │ slot  │
├───────┼───────┼───────┤
│ slot  │ slot  │ slot  │
└───────┴───────┴───────┘
        处理类型 chip · 页码 1/3
```

- `crafting_shaped`：pattern 映射到 3×3  
- `crafting_shapeless`：ingredients 顺序填入，余下空槽  
- 图标优先，名称辅助（可 `N` 切换显示/隐藏名称）  

### 3.2 机器类

```text
[slot 原料]  →(设备图标)  [slot 产物]
     chip: 切石机 · 额外：经验/时间
```

### 3.3 Special / 动态（JEI 对等的关键）

**原则：** 与 JEI 相同——**不假装从 JSON 生成网格**，而是加载 **「JEI 式展示模板表」**。

#### 模板表（v1 原版）

| recipe id / type | 槽位示意 | 文案 |
|---|---|---|
| `banner_duplicate` / `crafting_special_bannerduplicate` | `[任意旗帜] + [任意染料]` → `[同款旗帜 ×2]` | 图案一并复制；染料颜色不改外观 |
| `book_cloning` | `[书与笔] + [已写成的书]` → `[已写成的书 ×2…]` | 复制墨水消耗 |
| `map_cloning` | `[已有地图] + [空地图]` → `[地图 ×2]` | |
| `map_extending` | `[地图] + [纸]` → `[更大比例地图]` | |
| `armor_dye` | `[皮革盔甲] + [染料…]` → `[染色盔甲]` | 可混色 |
| `firework_rocket` | `[纸] + [火药…] + [烟火之星?]` → `[烟花]` | |
| `firework_star` / fade | `[火药] + [染料…]` → `[烟火之星]` | |
| `tipped_arrow` | `[箭 ×8] + [滞留药水]` → `[药箭 ×8]` | |
| `shield_decoration` | `[盾] + [旗帜]` → `[带图案盾]` | |
| `shulker_box_coloring` | `[潜影盒] + [染料]` → `[同色盒]` | |
| `suspicious_stew` | `[碗]+[红蘑菇]+[棕蘑菇]+[花?]` → `[迷之炖菜]` | 效果随花 |
| `repair_item` | `[同类工具 A] + [同类工具 B]` → `[合并耐久]` | |
| `decorated_pot` | 四面陶片环绕 → `装饰陶罐` | |

- 槽位用 **真实 catalog 图标** + 「任意」角标  
- 点击「任意 X」打开该类代表物品（如所有染料列表）  
- **无法识别的 special**：显示 JEI 式空表 + `type` + 「未收录展示模板」（与 JEI 对未知 special 一致）  

#### 与 JEI 的差距声明（设计诚实项）

| 能力 | JEI | 我们 v1 |
|---|---|---|
| 原版 special 示例网格 | 有 | **有（模板表）** |
| 模组 SpecialRecipe 自动网格 | 常不全 | **默认不全**，可后续加模板 |
| 运行时 NBT 产物 | 精确 | 示例产物 |
| 快捷键/收藏/翻页 | 有 | **对齐** |
| 最近访问 | 无 | **有** |

---

## 4. 数据模型（可追溯、可持久化）

### 4.1 浏览器会话（前端 state）

```ts
type RecipeBrowserState = {
  query: string;
  selectedItemId: string | null;
  mode: 'recipes' | 'uses';
  recipePage: number;
  recipeTypeFilter: string | null; // 'minecraft:stonecutting' | ...
  highlightSlot: { recipeId: string; slot: number } | null;
};
```

### 4.2 收藏

```ts
type RecipeBookmark = {
  itemId: string;          // minecraft:andesite_stairs
  displayName: string;
  iconKey: string | null;
  modId: string;           // namespace
  createdAt: number;       // epoch ms
  // 可选：pin 顺序
  order?: number;
};
```

持久化：**优先包级**（`localStorage` key 或后端 settings）：

```text
mpack:bookmarks:<packId>   // 包内作者常用物品
mpack:bookmarks:global     // 可选全局
```

v1 默认 **localStorage per pack**；后端 API（P2）：

```text
GET/PUT /api/packs/{id}/recipe-browser/bookmarks
GET/PUT /api/packs/{id}/recipe-browser/history
```

### 4.3 最近访问

```ts
type RecipeHistoryEntry = {
  itemId: string;
  displayName: string;
  iconKey: string | null;
  context: 'recipes' | 'uses';
  at: number;
};
// key: `${itemId}|${context}` 去重，上限 50
```

### 4.4 配方视图模型（API 聚合）

```ts
type RecipeSlotVM = {
  kind: 'item' | 'tag' | 'any';
  itemIds: string[];     // 轮换候选
  display: string;       // 「任意染料」
  iconItemId: string | null;
};
type RecipeViewVM = {
  id: string;            // content key / recipe id
  sourceType: string;    // minecraft:stonecutting
  category: 'crafting' | 'machine' | 'special' | 'unknown';
  machine?: string;      // stonecutter / furnace...
  templateId?: string;   // banner_duplicate
  inputs: RecipeSlotVM[] | { grid: (RecipeSlotVM | null)[][] };
  output: RecipeSlotVM | null;
  extra?: { experience?: number; cookTime?: number };
  usesCount?: number;    // 用途模式
};
```

来源：

1. `mod_content` recipe payload → 网格/机器槽  
2. `special_template` 表（前端常量或 `docs` 同步 JSON）→ special 槽  
3. Uses：由 **配方输入反查**（谁的 ingredient 含当前物品）  

---

## 5. 页面布局与视觉

对齐 `design-system.md` + JEI 心智，但用本工作台语言：

| 区 | 视觉 |
|---|---|
| 物品列表 | 图标 28–32px + 名称；选中高亮品牌色 |
| 配方画布 | 浅矿物纸底；槽位 JEI 式小方格；箭头/设备图标叠在过程上（已实现的机器配方语义） |
| 收藏/最近 | 深色或纸色侧栏；时间用 caption 级 |
| 快捷键提示 | 底栏或 `?` 弹层（列表与 §2.3 一致） |

**图标：** 使用现有 `getItemIcon` / catalog；缺失用首字占位（不阻塞）。

---

## 6. 交互流程（用户故事）

### A. 查「安山岩楼梯怎么获得」

1. `/` 聚焦搜索 → 输入 `安山岩楼梯`  
2. 列表选中 → 默认进入 **Recipes（R）**  
3. 看到：工作台合成表 / **切石机**（安山岩→楼梯）/ 其它  
4. 点切石机 chip 只看机器配方；`←/→` 翻页  
5. 点输出槽 → 继续看楼梯的 **Uses（U）**  
6. `F` 收藏；记录进入 **最近访问**

### B. 查旗帜复制

1. 搜索 `banner_duplicate` 或物品「旗帜」→ `R`  
2. special 模板：**任意旗帜 + 任意染料 → ×2**  
3. 点染料「任意」→ 轮换/列出染料  

### C. 作者连续配包

1. 右栏「最近」点开昨天看过的 AE2 物品  
2. `F` 取消不关心的收藏  
3. `Esc` 回到内容编辑  

---

## 7. 与 mPackStation 集成

| 点 | 策略 |
|---|---|
| 入口 | 内容编辑页顶栏「打开配方浏览器」+ 侧栏路由 `/recipes` |
| 图标 | 复用 `resolveModContentIcons` + catalog |
| 名称 | `translateKey` + lang |
| Uses 数据 | 服务端后续可加 `recipe-uses?itemId=`；v1 前端对已加载配方建反查索引 |
| 权限 | 只读浏览；不写配方（内容编辑仍走 revision 流程） |

### Uses 的数据来源（重要）

- v1：对当前包已解析 `recipe` 建 `ingredient → recipeIds` 倒排（内存）  
- 限制：未解析模组/动态配方的 uses 不全——UI 标明「基于包内已解析配方」  
- P2：后端预计算 `recipe_ingredients` 表  

---

## 8. 快捷键实现约束

1. 监听挂载在浏览器容器；**输入框/contenteditable 内不拦截字母**  
2. 与任务书 MpCanvas 的 Space pan **不冲突**（配方浏览器不启用 Space pan，或 Space 仅在按住时平移画布）  
3. 与系统/浏览器冲突时：`?` 弹层可显示替代键  
4. `preventDefault` 仅在确实处理了该键时  

---

## 9. 分期

| 阶段 | 内容 |
|---|---|
| **M1** | 路由+搜索前缀+物品列表+Recipes 画布（合成/机器）+ `R/U` + 翻页 + **收藏(F)** + **最近(T)** + special 模板表（原版）+ 快捷键 `?` |
| **M2** | 槽位多候选轮换 `C` · Uses 服务端 API · 收藏后端同步 · 显示/隐藏物品名 `N` · 从 JEI 心智的 ore/tag 更深过滤 |
| **P2** | 模组 special 模板库 · 导出收藏清单 · 与任务书 icon picker 打通 |

---

## 10. 验收标准

1. 搜索 + 点选物品 → 默认 Recipes；**`U` 切 Uses**，**`R` 切回**  
2. **`←/→`** 多配方翻页；chip 过滤机器/合成/special  
3. **`F`** 收藏/取消；**`H`** 打开收藏；刷新页面后收藏仍在（localStorage）  
4. **`T`** 打开最近访问；有条目、可点击回放、可清空；同物品去重  
5. `banner_duplicate` 等 special **有槽位示意图**，不是「暂不支持」大段报错  
6. `stonecutting` 等机器配方：原料→设备图标→产物  
7. 快捷键在搜索框打字时 **不会** 误触发 R/U/F  
8. Uses 列表来自包内已解析配方反查，并有范围说明  
9. `tsc` / `build` / 相关单测；**盲审/评审通过后再开发**  

---

## 11. 风险

| 风险 | 缓解 |
|---|---|
| Uses 不全 | UI 标明「已解析范围」；后续后端倒排 |
| special 模板与版本差异 | 模板表带 `mcVersion` 字段，可按 1.21.x 切换 |
| 快捷键与其它页冲突 | 仅配方路由激活；或容器 focus 时激活 |
| localStorage 多包串号 | key 含 packId |
| 与「内容编辑」职责重叠 | 浏览器=只读 JEI；编辑仍在原 revision 流程 |

---

## 12. 开放问题（评审可拍板）

1. 配方浏览器是 **独立路由** 还是内容编辑内全屏模式？（本稿：独立路由 + 可嵌入）  
2. 最近访问 key：`T` 是否可接受？（JEI 无标准键）  
3. 收藏 v1 是否必须同步到后端，还是 localStorage 即可？  
4. special 模板是否 M1 就做满原版十余条，还是先做 banner/book/map 三条？  
5. Uses 是否 M1 必做（反查），可否 M1 只做 Recipes + 占位？  

---

## 13. 参考

- JEI 交互惯例：物品列表 / R·U / 收藏 / 多配方翻页 / catalyst 类型  
- 原版 `BannerDuplicateRecipe`（SpecialCraftingRecipe，运行时 matches）  
- 本仓：`docs/design/quest-book-ftb-experience.md`、`mod-content-extraction.md`、`design-system.md`  
- 已实现：机器配方箭头设备图标、special 文案分支（待升级为模板槽位）  

**当前交付 = 本设计方案，等你评审后再动代码。**
