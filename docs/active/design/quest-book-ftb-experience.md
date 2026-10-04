# 任务书体验与数据设计（仿 FTB Quests）

- 状态：设计稿 v1.1（盲审有条件通过后的修订；可开发）
- 原则：**体验与流程贴近 FTB，实现不要求一致；数据结构可追溯到 FTB 字段**
- 权威来源见 §9

---

## 0. 用户要求（验收基准）

1. 读过原始实现后再设计，禁止拍脑袋表单壳  
2. 玩家/作者体验与 FTB 接近：书→章→图→任务详情→依赖锁定→奖励  
3. 数据结构可追溯：每个我们的字段能映射到 FTB SNBT/概念（或明确标为自有扩展）  
4. 交付后「能看」：像任务书产品，不像后台 CRUD  

---

## 1. FTB 原始体验（设计必须对齐的流程）

### 1.1 玩家流程

```text
打开任务书
  → 左侧章节栏（可 pin）
  → 中央章节任务图（画布可拖）
  → 点击节点 → 详情：描述 / 任务目标 tasks / 奖励 rewards
  → 锁定节点：前置未完成；可展开前置/后继列表（跨章标 [章节名] 可跳转）
  → 完成全部 tasks → toast + 「!」角标（书/章节/节点）
  → 领奖：Claim all 或节点内领取；choice 需点选
```

### 1.2 编辑器流程（我们的主场景）

```text
编辑模式
  → 先建 Chapter（+ 可 Chapter Group）
  → 画布空白处创建 Quest：先选 task type（完成条件）再填参数
  → 拖节点写 x/y
  → 依赖：quest.dependencies[] + requirement(AND/OR/AND_N) + min_required
  → 属性面板分组：文案 / 外观 / 可见性 / 依赖 / 进度规则
  → Tasks 区与 Rewards 区各自「+」类型化条目
```

### 1.3 关键体验点（实现时逐条对照）

| # | 体验 | 我们必须有 |
|---|---|---|
| E1 | 书→章→图 三级，不是表单页 | 章节 rail + 可拖画布 + Inspector |
| E2 | 节点是图标牌：icon + 标题 + 形状感 | 不是纯文字行 |
| E3 | 锁定态可解释 | 半透明/锁角标 + 「还缺哪些前置」列表 |
| E4 | 跨章依赖可跳转 | 前置列表标章节名 + 点击跳转 |
| E5 | 画布拖节点改 x/y | 权威布局不是 grid |
| E6 | 连线=依赖，与面板双向一致 | 禁止画布与 Inspector 各说各话 |
| E7 | 编辑/预览双模式一眼可辨 | 顶栏模式切换 + 徽章 |
| E8 | Tasks 与 Rewards 分区编辑 | 后端字段要露出来 |
| E9 | 图例：锁定/可选/编辑中 | UI 内图例，不靠口头 |
| E10 | 大书可浏览 | pan/zoom/搜索/章节 pin |

---

## 2. 数据模型（可追溯）

### 2.1 权威源约定

- **图权威**：`draft.edges`（我们的实体边）  
- **FTB 映射**：每条 edge 的 `fromNodeId → toNodeId` 表示 **to 依赖 from**，导出/对照时写成 to 节点的 `dependencies: [fromNodeId]`  
- **节点 `prerequisites`**：保存时由 edges **派生同步**（入边 from 列表），与 FTB `dependencies[]` 等价；禁止双轨手改  
- **依赖逻辑**：节点级 `dependencyRequirement` + `minRequiredDependencies`（对齐 FTB）

### 2.2 字段映射表（追溯核心）

| 我们字段（payload） | FTB SNBT | 说明 |
|---|---|---|
| `book.title` | `data.snbt` `title` | 书名 |
| `book.icon` | `data.snbt` `icon` | |
| `book.progressionMode` | `data.snbt` / quest `progression_mode` | 字面量：`default` \| `linear` \| `flexible` |
| `chapters[].id` | `chapter.id` | |
| `chapters[].title` | `chapter.title` | |
| `chapters[].description` | chapter subtitle/自有说明 | **不是** quest.subtitle |
| `chapters[].coverColor` | （自有观感） | FTB 用 icon/images |
| `chapters[].icon` | `chapter.icon` | `modid:item` |
| `chapters[].position` | `chapter.order_index` | |
| `nodes[].id` | `quest.id` | |
| `nodes[].chapterId` | 所属 chapter | |
| `nodes[].title` | `quest.title` | |
| `nodes[].subtitle` | `quest.subtitle` | 节点副标题，≠章节 description |
| `nodes[].description` | `quest.description[]` | string ↔ 数组 |
| `nodes[].icon` | `quest.icon` | |
| `nodes[].x` `y` | `quest.x` `y` | 拖拽落盘 |
| `nodes[].position` | （排序辅助，非 FTB 画布坐标） | 仅列表排序/初始化，**布局权威=x/y** |
| `nodes[].shape` | `quest.shape` | `circle`/`square`/`rounded`/`diamond`/`hexagon`/… |
| `nodes[].size` | `quest.size` | number |
| `nodes[].optional` | `quest.optional` | bool |
| `nodes[].invisible` | `quest.invisible`（简化；hide_* 细分后续） | bool |
| `nodes[].dependencyRequirement` | `quest.dependency_requirement` | **FTB token**：`all_completed`\|`one_completed`\|`all_started`\|`one_started` |
| `nodes[].minRequiredDependencies` | `quest.min_required_dependencies` | int，AND_N |
| `nodes[].prerequisites` | `quest.dependencies[]` | **由 edges 派生，只读** |
| `edges[]` | （无独立表；UI/校验） | from→to = to 依赖 from |
| `nodes[].tasks[]` | `quest.tasks[]` | 见 §2.3 |
| `nodes[].rewards[]` | `quest.rewards[]` | 见 §2.3 |
| `nodes[].modRefs` | （自有） | 包内模组关联 |

### 2.3 Task / Reward 映射（枚举与字段闭合）

**Task type（FTB token 原文）→ 我们字段**

| type | 我们字段 | FTB |
|---|---|---|
| `item` | `itemId`, `count` | `item`/`count` |
| `checkmark` | `title` | `checkmark` |
| `advancement` | `advancementId` | `advancement` |
| `dimension` | `dimension` | `dimension` |
| `kill` | `entityId` | `kill` |
| `location` | `x`,`y`,`z` | `location` |
| `stat` | `statId`,`value` | `stat` |
| `xp` | `xp` | xp / xp_levels |

**不在本期枚举**（勿在 schema 里空挂）：`biome`/`observation`/`fluid`/`gamestage`/`energy`/`custom` → P2。

**Reward**

| 我们 kind | FTB type token |
|---|---|
| `item` | `item` |
| `experience` | `xp` 或 `xp_levels`（字段 experience 数值） |
| `command` | `command` |
| `unlock` | `advancement`（unlockId） |

P2：`choice`/`random`/`loot`/tables。

### 2.3b 权威写路径（强制）

- 仅 **edges** 可写；`prerequisites` 保存时派生，UI 只读展示  
- 保存后必须：`∀ n: set(n.prerequisites) == {e.fromNodeId | e.toNodeId==n.id}`  
- 手改 prerequisites 不入库；导出/对照 FTB 时用派生列表写 `dependencies[]`

### 2.4 完整 draft 结构（目标 schema）

```jsonc
{
  "book": { "title": "整合包任务书", "icon": "", "progressionMode": "flexible" },
  "chapters": [
    {
      "id": "ch1", "title": "开始", "description": "", "coverColor": "#C9783B",
      "icon": "minecraft:book", "position": 0
    }
  ],
  "nodes": [
    {
      "id": "n1", "chapterId": "ch1",
      "title": "木镐时代", "subtitle": "石器之前", "description": "获得一把木镐",
      "icon": "minecraft:wooden_pickaxe",
      "x": 0, "y": 0, "shape": "circle", "size": 1,
      "optional": false, "invisible": false,
      "dependencyRequirement": "all_completed",
      "minRequiredDependencies": 0,
      "prerequisites": [],
      "tasks": [{ "id": "t1", "type": "item", "itemId": "minecraft:wooden_pickaxe", "count": 1 }],
      "rewards": [{ "kind": "experience", "experience": 10 }],
      "modRefs": [],
      "position": 0
    }
  ],
  "edges": []
}
```

**向后兼容**：旧 draft 无新字段时前端默认值；后端 zod/校验允许 optional 字段。

---

## 3. 信息架构与界面

```text
任务书页
├─ 顶栏：包名 · 模式 [编辑|预览] · 保存 · 校验 · 应用 · 图例
├─ 左栏 ChapterRail
│    章节列表（色/图标 + 标题 + 任务数 + 可选 !）
│    + 新建章节 · 上移/下移 · 重命名 · 删除（有任务需确认）
├─ 中央 QuestCanvas（编辑）
│    深色画布 · pan/zoom（复用进度树手势）
│    节点：圆形/多边形轮廓 + 图标 + 标题 + optional 虚线边
│    连线：前置 → 后继（箭头）
│    空白右键/工具「+ 任务」→ 先选类型再落点
│    拖节点 → 写 x/y
├─ 中央 QuestCanvas（预览）
│    同布局；锁定节点降饱和 + 锁
│    点击 → 详情浮层/侧栏：任务目标 + 奖励 + 「还缺前置」列表
└─ 右栏 Inspector（编辑）
     分组：
     1 基础：title/subtitle/description/icon
     2 外观：shape/size/x/y（只读显示+微调）
     3 依赖：prerequisites 列表 · requirement 下拉（全部/任一/至少N）· min N
     4 可见：optional/invisible
     5 任务：tasks[] 增删改（按 type 表单）
     6 奖励：rewards[] 增删改
```

### 3.1 节点视觉（体验）

| 状态 | 视觉 |
|---|---|
| 编辑选中 | 金色描边 |
| 预览-锁定 | 降饱和 + 锁图标 + 点击看缺失前置 |
| 预览-可接（前置已满足模拟） | 正常饱和 |
| optional | 虚线边框 |
| 连线 | 细线 + 箭头；选中高亮 |

### 3.2 预览模拟（无游戏存档）

本地模拟器（纯前端）：

1. 维护 `simulatedCompleted: Set<nodeId>`  
2. 点击节点可「标记完成/取消」（预览模式）  
3. 节点解锁条件：对 prerequisites 按 `dependencyRequirement` + `minRequiredDependencies` 求值  
4. 锁定节点详情列出未满足前置（含跨章章节名）  

→ 体验接近 FTB 锁定/解锁，数据不伪造玩家存档。

---

## 4. 与现网差距 → 实现优先级

| 优先级 | 项 | 体验 |
|---|---|---|
| P0 | 界面按 §3 重做：rail+画布+分组 Inspector+图例+编辑/预览 | 「能看」 |
| P0 | 节点拖拽 x/y；pan/zoom | FTB 画布感 |
| P0 | icon 显示与 catalog 选择；shape/size | 图标牌 |
| P0 | tasks[] / rewards[] 编辑（type 下拉+字段） | 完成条件可配 |
| P0 | 预览模拟 + 锁定解释 + 前置列表跳转 | FTB 锁定体验 |
| P1 | dependencyRequirement + minRequired 保存/校验/同步 edges | AND/OR/AND_N |
| P1 | 章节 icon/重命名/排序/删除 | 章是对象 |
| P1 | 搜索节点；跨章依赖跳转 | 大书可用 |
| P2 | ChapterGroup、shape 全集、SNBT 导出、choice 奖励表 | 对齐 FTB 全量 |

**数据结构**：P0 即引入 tasks/rewards 完整编辑 + 必要节点字段；P1 依赖语义；后端校验同步扩展。

---

## 5. 验收标准（体验 + 可测）

1. 任务书页为「章节 rail + 画布 + 分组 Inspector + 图例 + 编辑/预览徽章」；无 JSON dump 主视图  
2. 新建章节、新建任务（先选 type）、拖节点写 x/y、连线前置、保存刷新仍在  
3. Inspector 可改 icon/shape/optional/tasks/rewards；画布节点显示 **icon 图**而非仅 id 文本  
4. 预览：标记完成 → 节点与章节「!」+ toast；锁定节点列出未满足前置（跨章含章节名）；可「领取奖励」  
5. payload：保存后 edges↔prerequisites 一致；旧 draft 无新字段时默认值可加载  
6. **依赖语义矩阵**（预览求值）：\|req\|前置\|期望\|：all_completed+2 全完成→解锁；one_completed+1/2 完成→解锁；all_completed+1/2→锁定；min_required=1+2 前置完成 1→解锁  
7. dependencyRequirement UI 文案「全部完成/任一完成/至少 N」与 payload FTB token 一致  
8. 模式切换徽章与图例可见（E7/E9 可观测 DOM）  
9. 字段映射表可抽查（book/chapter/node/task/reward）  
10. `go test` / `tsc` / build / 图与依赖求值单测绿；盲审条件闭合后开发  

## 5b. 「能看」可观测断言（非纯主观）

- 节点元素含 `<img>` icon 或 aria-label 图标占位，且含 title 文本  
- 画布存在 SVG/线连接层且选中节点后边数 >0（有前置时）  
- 预览模式 body/page 含模式徽章文案「预览」  
- 完成后节点 class 含 completed/badge，章节列表出现 !  
- Inspector 存在分组标题：基础/外观/依赖/可见/任务/奖励（或等价中文）  

---

## 6. 非目标 / 预览反馈范围（盲审闭合）

| 项 | 决定 |
|---|---|
| 预览「完成」反馈 | **P0 进交付**：标记完成后节点「!」角标 + 章节列表角标 + 轻量 toast（模拟，非游戏存档） |
| Claim all / 节点领奖 | **P0 预览模拟**：可完成的任务显示「领取奖励」按钮；领取后状态「已领取」；choice 类仅占位文案 |
| 章节 pin | **P1**（非 P0 阻塞） |
| SNBT 导出/导入 | **P2 非本设计承诺** |
| 队伍进度 / loot crate / reward table UI | **P2** |
| 真实 MC 存档同步 | **非目标** |  

---

## 9. 原始实现来源

| 用途 | URL |
|---|---|
| FTB 玩家导航 | https://docs.feed-the-beast.com/mod-docs/mods/suite/Quests/Player/Questbook/Navigating |
| FTB 锁定/依赖 | https://docs.feed-the-beast.com/mod-docs/mods/suite/Quests/Player/Questbook/Tips_Tricks |
| FTB 任务设置/依赖语义 | https://docs.feed-the-beast.com/mod-docs/mods/suite/Quests/Developer/Quests/Settings |
| FTB Task/Reward 类型 | 同站 Types / Rewards 页 |
| 源码 DependencyRequirement | https://github.com/FTBTeam/FTB-Quests/.../DependencyRequirement.java |
| ATM-9 真实 SNBT | https://github.com/AllTheMods/ATM-9/tree/main/config/ftbquests/quests |
