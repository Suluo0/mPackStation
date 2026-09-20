# 进度界面体验与数据设计（仿原版 Minecraft）

- 状态：设计稿 v1.1（盲审有条件通过后的修订；**本稿为进度体验/Tab/requirements/手势唯一权威**）
- `advancement-tree-canvas.md`：仅作历史手势细节参考；**验收与实现以本稿为准**（supersede）
- 原则：**体验接近原版进度界面；实现为 Web 工作台增强；数据结构可追溯到 advancement JSON**

---

## 0. 用户要求

1. 读原始实现后再设计  
2. 体验/流程仿原版（L 键进度页的感觉）  
3. 数据可追溯到 advancement JSON  
4. 产出「能看」的进度浏览，不是工程树调试器  

---

## 1. 原版体验（1.21.1 事实）

| 项 | 原版行为 |
|---|---|
| 打开 | 进度键 L / 暂停菜单 |
| Tab | **有 `display` 的 root** 才成为 Tab；1.21.1 可见 5 个：story/nether/end/adventure/husbandry |
| recipes | `advancement/recipes/**` **无 display**，**不进进度 UI**；靠 rewards.recipes 解锁配方书 |
| 画布 | 有界视口；**左键拖 = 平移**；**滚轮 = 平移**；**无缩放** |
| 布局 | JSON **无 x/y**；客户端按 parent 树布局（左→右，同父纵向堆叠） |
| 节点 | 26×26 框 + 图标；标题盒常显；frame：task(默认)/goal/challenge |
| 完成态 | obtained/unobtained 贴图 + 图标变暗；**无对勾**；多条件有进度条 |
| 详情 | **悬停浮层**：标题（frame 色）+ 描述 + 条件/进度 + rewards |
| requirements | **OR(AND)** 二维数组：`[[a,b],[c]]`=(a且b)或c |

---

## 2. 数据模型（可追溯）

### 2.1 Tab / 树权威（对齐原版）

```text
节点集合 S = 当前数据源（包内或单模组）kind=advancement
根 root ⇔ (parent ∉ S 或 parent 为空) ∧ display 存在
Tab = 每个 root 一棵树；label=display.title(lang)，icon=display.icon
parent ∈ S → 挂到对应 root 的子树
parent ∉ S 且无 display → 不伪造 Tab；进「未归组/外部前置」
无 display 的 recipes/** → 默认不进「玩家进度」视图；可选折叠分组「配方解锁」
path 启发式（如 ns:adventure/root）：**禁止创建 Tab**；仅可用于「未归组/外部前置」的分组标签
包级数据源：选定 S 后该 S 为唯一权威，不混用多源 parent
```

### 2.2 字段映射（payload → UI）

| advancement JSON | UI |
|---|---|
| `id` / content key | 节点 id、详情主键 |
| `parent` | 连线、树布局、父链 |
| `display.title` | 节点标题 / hover / Tab 名（lang 解析） |
| `display.description` | hover + Inspector |
| `display.icon.item\|id` | 节点图标（catalog resolve） |
| `display.frame` | task/goal/challenge 样式；缺省 task |
| `display.background` | Tab 画布底纹（可选） |
| `display.hidden` | 详情元数据；树上可 dim |
| `display.show_toast` `announce_to_chat` | Inspector 元数据 |
| `criteria` | 条件键列表 + 展开 trigger（后续） |
| `requirements` | Inspector：**「任一/全部/混合」** 语义 |
| `rewards.experience/recipes/loot/function` | Inspector 奖励区 |

### 2.3 requirements 展示规则（OR(AND)，与原版一致）

原版语义：**外层数组 OR，内层数组 AND**（内层组内全部满足 → 该组成功；任一内层组成功 → 整体完成）。

| JSON | 语义 | UI 文案 |
|---|---|---|
| `[[a],[b],[c]]` | a ∨ b ∨ c | **任一条件**（a / b / c） |
| `[[a,b,c]]` | a ∧ b ∧ c | **全部条件**（a 且 b 且 c） |
| `[[a,b],[c]]` | (a∧b) ∨ c | **(a 且 b) 或 c** |

验收抽查必须覆盖上表三种，禁止写反。

### 2.4 我们增强字段（非原版，须标明）

| 字段 | 用途 |
|---|---|
| 运行时布局 x/y | 算法计算，**不写入** mod_content |
| k 缩放 | Web 编辑器增强，**非原版** |
| `done` 演示开关 | 仅 fixture；默认关闭，避免假完成 |

---

## 3. 界面结构

```text
内容编辑 → 进度
├─ 顶栏：数据源（当前模组 / 包级汇总若有）· 搜索 · 视图[树|表格]
├─ Tab 条：root 图标 + 标题（原版语义，不用 path key）
├─ 画布（对齐原版 + 受控增强）
│    · 左键拖空白/节点 = 平移（有界 pan，对齐 minPan/maxPan）
│    · 滚轮 = 平移（原版）
│    · Ctrl+滚轮 / 工具条 = 缩放（增强，UI 标注「缩放：工作台增强」）
│    · 节点：框 + 图标 + 标题盒；frame 三态
│    · 连线：父子；可选「原版折线」主题
│    · hover：短卡（标题 frame 色 / 描述 2–3 行 / frame / rewards 摘要）
├─ 右栏 Inspector（编辑器主详情，保留）
│    id · parent · 父链 · frame
│    description
│    requirements 语义化
│    criteria 列表
│    rewards
│    display 元数据 hidden/toast/chat/background
└─ 表格视图：现有 kind 筛选列表
```

---

## 4. 手势事实（避免错误宣传）

| 手势 | 原版 | 我们 |
|---|---|---|
| 拖画布 | 是（有界） | 是（有界 pan） |
| 滚轮 | **平移** | 平移（无 ctrl） |
| Ctrl+滚轮缩放 | **无** | **有**（增强） |
| 拖节点改坐标 | 原版运行时布局，玩家不可改 | **否**（只读树） |
| hover 详情 | 是（浮层） | 是（短卡）+ 侧栏 |

---

## 5. 完成态策略

- 默认：**无玩家存档 → 全树「结构预览」**，不画勾、不伪造 obtained  
- fixture/演示：显式 `done` 时用「亮框/暗框 + 饱和度」对齐原版，**不用对勾贴图**  
- UI 标注：「演示完成态 · 非玩家存档」  

---

## 6. 实现优先级

| 优先级 | 项 |
|---|---|
| P0 | Tab=有 display 的 root；recipes 默认不进玩家树 |
| P0 | hover 短卡 + Inspector 补 rewards/requirements 语义/display 元数据 |
| P0 | 节点标题盒常显（root/选中/搜索命中）；frame 默认 task |
| P0 | pan 有界；滚轮=平移；缩放保留并标注增强 |
| P0 | 图标 icon.id/item 解析 + 缺省占位 |
| P1 | 包级多模组合并数据源（若 API 有） |
| P1 | 搜索高亮；requirements 展开 trigger |
| P2 | 原版模式（禁缩放）；折线主题；背景纹理 per Tab |

---

## 7. 验收标准

1. Tab 列表 = 有 display 的根；path 启发式不得生成 Tab  
2. 无 display 的 recipes 不出现在默认玩家 Tab（或单独折叠且默认收起）  
3. 悬停短卡；Inspector：`[[a],[b],[c]]`→任一，`[[a,b,c]]`→全部，`[[a,b],[c]]`→混合；rewards/display 元数据  
4. 拖动平移有界；滚轮平移；Ctrl 滚轮缩放且 UI 标明增强  
5. 无 done 时无「已完成」暗示；演示态有图例标注  
6. 字段与 advancement JSON 可抽查追溯  
7. **实现与验收以本稿为唯一权威**（canvas 旧稿不单独作验收依据）  
8. 盲审条件已闭合后开发；测试+验收代理签字  

---

## 8. 来源

- 本地：`data/cache/minecraft-assets/1.21.1.jar`（1399 advancement + GUI + zh_cn）  
- yarn 1.21.1：AdvancementsScreen / AdvancementTab / AdvancementWidget / AdvancementDisplay  
- 我们现网：`AdvancementTreeView` + `advLayout` + `mod_content` payload  
