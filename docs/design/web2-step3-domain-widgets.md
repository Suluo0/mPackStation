# web2 第三步 · 叶子组件层实施方案（领域组件）

> 三步「新开」路线的第 3 步。前置：第二步 `web2-step2-shell-and-pages.md` 已验收通过（§7 全绿）。
> 顶层权威 = `docs/design/workbench-interaction-design.md` v2.1；每个领域另有自己的设计文档（§1.2）。
>
> **本步的性质：新写为主，可酌情参考。** 用户原话：「最后的组件可以酌情参考一下，但还是以新写为主。」
> 本步之所以敢新写，是因为叶子层**有独立的规格来源**（§1）：每个领域都有自己的设计文档 + 后端契约文档 + 1733 行 headless 测试。
> 参考旧实现只用于一件事：**确认某个后端字段的真实形状**。不用它来决定交互怎么做——交互一律以设计文档为准。

---

## 0. 铁律

1. **一次只做一个批次**（§3 的 3A–3G），该批次验收不绿不进下一批。
2. **不改容器**。第二步 §9 定死了插槽签名，本步只把 `stubs/Stub*` 换成真实组件；`pages/`、`app/` 下的文件**一行都不许改**。若发现插槽签名不够用，停下来问，不要擅自改容器。
3. **不改后端**；不碰端口 5173/18765/18766/18871；不碰 `/tmp/mpack-data`；不改 `apps/web/`。
4. **API 契约以 `apps/web2/src/api/*.ts` 为准**，字段名/可空性有疑问时去读 `apps/server/internal/httpapi/routes_*.go` 与 `docs/api/dto.md`，**不要靠猜、不要靠翻旧前端**。只准追加导出，禁改已有导出。
5. **不留假 affordance**（设计文档 §5-8）：没有后端支撑的按钮不渲染；无图形预览的类型要**在文案里说清**，不要显示一个空框。
6. **每个组件的样式写在自己的同名 css 里**，尺寸颜色只引用 `tokens.css` 变量；禁往 `styles/base.css` 堆。
7. 令牌只走环境变量；不 `git add` / `git commit`；不装新依赖。
8. **批次结束时对应的 `stubs/Stub*` 文件必须删除**，不许留着"以防万一"。全部批次做完后 `apps/web2/src/stubs/` 目录为空并删除。

---

## 1. 本步的规格来源（这就是"敢新写"的依据）

### 1.1 契约层文档

| 文档 | 用途 |
|---|---|
| `docs/api/contract.md` | 端点总契约 |
| `docs/api/dto.md` | DTO 字段定义 |
| `docs/api/errors.md` | 错误码语义（`revision_conflict` / `content_not_validated` / `missing_mod_reference` / `cross_pack_reference` …） |
| `docs/api/inventory.md` · `integration-matrix.md` | 端点清单与前端集成矩阵 |
| `docs/api/auth.md` | 令牌与 Host/Origin 规则 |
| `apps/server/internal/httpapi/routes_*.go` | **字段形状的最终事实源**（catalog / content / mods / mod_content / publish / tasks / packs / dashboard / fs / imports / system） |

### 1.2 领域设计文档（每个叶子组件都有自己的权威）

| 领域 | 设计文档 | 行数 |
|---|---|---|
| 关系态 / JEI 配方浏览 | `docs/design/jei-recipe-browser.md` | 410 |
| 无限画布（任务书用） | `docs/design/mp-infinite-canvas.md` | 399 |
| 任务书 FTB 体验 | `docs/design/quest-book-ftb-experience.md` | 285 |
| ~~进度树画布~~ | `docs/design/advancement-tree-canvas.md` | 221 · **D-3B 已拍板砍掉，本文档不实现** |
| ~~原版进度体验~~ | `docs/design/advancement-vanilla-experience.md` v1.1 | 165 · **同上，不实现** |
| 模组内容抽取 | `docs/design/mod-content-extraction.md` | 252 |
| 地形多方块编辑器 | `docs/design/terrain-multiblock-editor-redesign.md` | 246 |
| 视觉规范 | `docs/design/design-system.md` · `dashboard-page-prompt.md` · `taster-visual-language.md` | 235 / 418 / 25 |

### 1.3 功能规格

`docs/specs/content-editor.md` · `quest-editor.md` · `pack-workbench.md` · `publish.md` · `settings.md`

### 1.4 headless 测试脚本 = 纯函数层的可执行规格（迁入 5 个 / 共 1733 行）

| 脚本 | 行数 | 测什么 | 被测模块 |
|---|---|---|---|
| ~~`test-adv-layout.ts`~~ | 454 | 进度树布局纯函数 | **D-3B 已砍，不迁**（连同被测模块一起留在 `apps/web`） |
| `test-viewport.ts` | 279 | 共享画布 viewport 纯函数（`boundsOf`/`clampZoom`/`fitBounds`/`gridMetrics` + 缩放常量） | `ui/canvas/viewport.ts` |
| `test-quest-deps.ts` | 115 | FTB 依赖判定矩阵（`all_completed`/`one_completed`/`min_required`；`depPayloadToUi`/`depUiToPayload`/`evaluateDependencies`/`isNodeAvailable`） | `features/quest/questDeps.ts` |
| `test-quest-canvas-edges.ts` | 108 | 连线数据流：语义 A→B = B 依赖 A；edges 是唯一图权威；保存前派生 prerequisites；自环/重复/环三类校验；跨章允许 | `features/quest/questGraph.ts` |
| `questGraph.check.ts` | 91 | quest graph 纯函数断言 | 同上 |
| `questM1SaveBody.ts` | 19 | 保存 payload 形状（edges → prerequisites 同步） | 同上 |
| ~~`adv-tree-e2e.mjs`~~ | 220 | Playwright 无头跑进度树画布 | **D-3B 已砍，不迁**（它依赖的 demo 页也不迁） |

**这五个脚本随纯函数层一起迁入 web2**（§5），迁完必须全绿。它们的价值是：新写的渲染层如果改坏了纯函数契约，测试会立刻红——这是"不靠肉眼判断对错"的机制。

---

## 2. 分层判定：什么迁、什么新写

### 2.1 纯函数层（迁入 676 行）—— **迁移**，不重写

| 模块 | 行数 | web2 新位置 | 理由 |
|---|---|---|---|
| ~~`features/content/advLayout.ts`~~ | 805 | **不迁** | D-3B 已砍掉进度树，这 805 行没有消费者 |
| `features/quest/questGraph.ts` | 394 | `src/domain/quest/graph.ts` | 图语义（edges 权威/环检测/prerequisites 派生），199 行测试覆盖 |
| `ui/canvas/viewport.ts` | 155 | `src/domain/canvas/viewport.ts` | 缩放/平移/fit 数学，279 行测试覆盖 |
| `features/quest/questDeps.ts` | 127 | `src/domain/quest/deps.ts` | FTB 依赖矩阵，115 行测试覆盖 |

它们**不含任何交互与渲染**，是纯计算 + 常量。设计文档 v2.1 对它们零改动要求。

> **D-3A 已拍板（2026-10-01，用户："按推荐的来"）：迁移，不重写。** 测试就是规格，重写无产品收益。

### 2.2 渲染 / 交互层（约 2642 行）—— **新写**

| 旧组件 | 行数 | web2 新组件 | 规格出处 |
|---|---|---|---|
| `ui/canvas/MpCanvas.tsx` + `MpZoomBar.tsx` | 784 + 133 | `src/widgets/canvas/InfiniteCanvas.tsx` + `ZoomBar.tsx` | `mp-infinite-canvas.md` |
| `features/quest/QuestBookEditor.tsx` | 1313 | `src/widgets/quest/QuestPanel.tsx`（+ `QuestCanvas` / `QuestInspector` / `ChapterRail`） | `quest-book-ftb-experience.md` |
| ~~`features/content/AdvancementTreeView.tsx`~~ | 591 | **不迁不写** | D-3B 已砍（2026-10-01 用户拍板）。索引态只留网格 / 表格两种视图 |
| `features/recipe/RecipeViewer.tsx` | 301 | `src/widgets/recipe/RecipeView.tsx` | `jei-recipe-browser.md` |
| `features/catalog/ItemCatalogGrid.tsx` + `ItemPickerModal.tsx` | 52 + 59 | `src/widgets/catalog/ItemGrid.tsx` + `ItemPicker.tsx` | 设计文档 §4.3 索引态 |
| `pages/ItemCatalogPage.tsx` 的浏览部分 | 197 | `src/widgets/catalog/ItemIndexPanel.tsx` | 设计文档 §4.3 索引态 |
| `pages/RecipeBrowserPage.tsx` | 180 | `src/widgets/recipe/RecipeGraphPanel.tsx` | `jei-recipe-browser.md` |
| `pages/RecipeTweakPage.tsx` | 311 | `src/widgets/content/ContentEditPanel.tsx` | `docs/specs/content-editor.md` |
| `pages/ModContentPage.tsx` | 514 | 拆进 `ItemIndexPanel` / `ModSourceRail` / `FocusInspector` | `mod-content-extraction.md` |
| `features/focus/InspectorRail.tsx` | 165 | `src/widgets/focus/FocusInspector.tsx` | 设计文档 §4.5 |
| `pages/PackPages.tsx` 的 8 个页面片段 | 723 | 拆进 §3 的 3E/3F/3G 各组件 | `docs/specs/pack-workbench.md` · `publish.md` · `settings.md` |
| `pages/LauncherPage.tsx` | 259 | `src/widgets/delivery/InstallSection.tsx` + `LaunchSection.tsx` | `docs/architecture/launcher-core-spec.md` |

**参考旧实现的唯一许可场景**：确认某个后端字段怎么用（例：`CatalogRecipe.payload` 里 shaped 配方的 key 名）。此时读旧代码是为了拿字段名，**读完照设计文档写新实现**，不是抄结构。

### 2.3 不迁的残渣

`advTreeDemo.tsx`(108) · `adv-tree-demo.html` · `public/design-previews.html` · `ui/workbench/*`(60) · `ModulePlaceholder` · `InspectorRecipePreview` 的旧壳。

⚠ `adv-tree-e2e.mjs` 依赖 `adv-tree-demo.html`。**处理方式：D-3B 砍掉进度树后，这条脚本和 demo 页都不迁**，两者留在 `apps/web` 里跟着旧前端一起等替换。§6.10 的收尾 grep 因此不检查它们。

---

## 3. 批次与顺序

依赖关系决定顺序：**先目录，再配方，再编辑，再任务书，再模组，再交付，最后工作台**。

| 批次 | 内容 | 替换的 Stub | 依赖 |
|---|---|---|---|
| **3A** | 目录与索引域：`ItemGrid` · `ItemIndexPanel` · `FocusInspector`（含多语言名称表/标签/配方预览） | `StubItemIndex` · `StubFocusInspector` | `useCatalog()`（第二步） |
| **3B** | 配方域：`RecipeView` · `RecipeGraphPanel` · 「魔改此配方」入口 · 已魔改标记 | `StubRecipeGraph` | 3A 的焦点 |
| **3C** | 内容编辑域：`ContentEditPanel`（JSON 编辑 + 实时预览 + `If-Match` + validate/apply + 修订历史/回滚 + **生成任务节点**） | `StubContentEdit` | 3B 的配方渲染（预览复用） |
| **3D** | 任务书域：`InfiniteCanvas` · `QuestPanel` · `QuestInspector` · `ChapterRail` · **`modRefs` 编辑** · 回滚/历史 · 图标默认取物品图标 | `StubQuest` · `StubQuestChapters` | 3A 的 `ItemPicker`、3C 的 modRefs 来源 |
| **3E** | 模组与来源域：`ModSourceRail` · `ModSearchDrawer` · 解析触发 · 内容类型筛选（**进度树已砍，不在本批**） | `StubModRail` · `StubModSearch` | 3A |
| **3F** | 交付域：五段 `ChecksSection` / `BuildSection`（含产物下载）/ `InstallSection` / `LaunchSection`（NDJSON 日志）/ `PublishSection` | `StubDeliverySections` | `usePackSummary()` 的闸门 |
| **3G** | 工作台域：`PackList` · `TaskPanel` · `ActivityFeed` · `PackModals`（新建/导入）· `HealthPanel` · `ConflictList`（行级 resolve/ignore） | `StubPackList` · `StubTasks` · `StubActivity` · `StubConflicts` · `StubHealth` | 无 |

每批做完：删对应 Stub → `npm run test` → `npm run build` → 跑该批的 §6 验收 → 贴输出 → 才进下一批。

---

## 4. 逐组件规格：必须保住的语义

> 这一节是本步的核心。每条都是**可核查的语义**，不是"做得更好看一点"。
> 「出处」列的行号指旧实现，用途仅限于：确认这条语义在旧代码里真的存在过（防止我把设计文档的意图写错）。**不要照着抄代码。**

### 3A 目录与索引域

**`ItemGrid`**（物品网格）
- 职责：图标 + 名称 + ID 的网格；支持选中、方向键移动、Enter 进关系态。
- 数据：`useCatalog().catalog.items`，禁自己发请求。
- 必须保住：图标缺失时按 `iconStatus` 显示占位而不是破图（`CatalogItem.iconStatus` 是契约字段）；`displayName` 已按 `resolvedLocale` 解析好，**不要在前端再挑一次 locale**。
- 键盘：方向键在网格内移动焦点（设计文档 §5-10），Enter = `setFocus(item, {mode:'graph'})`。

**`ItemIndexPanel`**（索引态）
- 必须保住的能力（设计文档 §4.3 索引态表）：
  1. 名称/ID 搜索
  2. 命名空间过滤 + **计数**（数据来自 `useCatalog().namespaces`，已派生好）
  3. 内容类型筛选 10 类 kind（配方/物品模型/结构/地形/战利品/进度/标签/元数据/语言 …，以 `api/modContent.ts` 的 `listModContent` 参数与 `docs/design/mod-content-extraction.md` 为准）→ 写 `?type=`
  4. 网格 / 表格视图切换（**进度树视图已按 D-3B 砍掉，不做**；注意第 3 条的 `进度` kind 是内容文档类型，仍保留在筛选里，与进度树画布无关）
  5. `?ns=` 生效：只显示该模组贡献的物品
- 空态：无物品 → 主按钮「重建目录」调 `useCatalog().rebuild()`（设计文档 §5-9）。

**`FocusInspector`**（右栏焦点段）
- 必须保住：
  1. **多语言名称表**——设计文档 §4.3 明确 ⚠「这是全站唯一有多语言名称表的地方 → 搬进 Inspector，不要随页面删除而丢失」。数据 = `CatalogItem.names[{locale,name,key,source}]`。**这条丢了就是能力回退，验收必查。**
  2. 所属标签可点 → 设 `?type=tag` 或聚焦该标签（设计文档 §4.3：旧的标签↔物品关联是**页内死胡同 Modal**，必须改成 Inspector 里的传送门）
  3. 来源 evidence（`CatalogItem.evidence`）
  4. 焦点为物品时**直接在右栏预览其主配方**：数据 = `useCatalog().recipesByOutput.get(id)?.[0]`，渲染复用 3B 的 `RecipeView`
  5. 传送门按钮**全部带焦点参数**（第二步的 Stub 已如此，替换时不许退化）
- 焦点类型分支：`item` / `recipe` / `mod` / `node` / `doc` 五种各有不同字段；拿不到详情时显示 kind+id+传送门，**不要显示空白**。

### 3B 配方域

**`RecipeView`**（JEI 网格渲染）
- 必须支持 6 类（设计文档 §4.3 关系态表，出处 `RecipeViewer.tsx:90-150,233-266`）：
  `shaped` · `shapeless` · 机器配方 · `ae2 transform` · `matter_cannon` · 特殊配方（无固定网格，显示中文说明）
- 特殊/动态配方必须有**中文说明**，不能只丢一个空网格（出处 `RecipeViewer.tsx:13-21,161-172`）
- 网格点击材料 → 回调 `onPick(itemId)`；关系态用它实现"重新居中"

**`RecipeGraphPanel`**（关系态）
- 必须保住：
  1. **双向视图**：作为产物 N 条 / 作为原料 N 条（数据 = `recipesByOutput` / `recipesByInput`）
  2. **材料点击 → 重新居中，无限下钻不换页**——设计文档 §4.3 标注「这是全站联动感最强的交互，必须保住」（出处 `RecipeBrowserPage.tsx:44-49,153,168`）。实现 = `setFocus({kind:'item', id})`，`mode` 保持 `graph`
  3. 读焦点：`useFocus()` 的 `item`（设计文档 §4.3 ⚠：旧实现全站只有这一页读焦点；web2 要求四态全读）
  4. **配方来源标记（原版 vs 已魔改）**——设计文档 §4.3 ⚠ 标为「未实现，本轮补」。判定需要内容文档的 applied 覆盖：查 `api/content.ts` 的列表接口，把已 apply 的 recipe id 集合与 `catalog.recipes` 比对，命中的标「已魔改」。**若后端拿不到 applied 集合，停下来问，不要用 catalog 的 `status` 字段猜。**
  5. **「魔改此配方」入口**——设计文档 §4.3 ⚠「未实现，本轮补」：点击 = `setSearchParams(p => {p.set('mode','edit'); p.set('doc', 对应文档 id 或新建); return p})`，**必须带配方标识**，跳过去要正好是这条 payload（设计文档 §5-3 的判据）
  6. 物品选择器（选物品用，与来源栏的"管模组"职责不同，设计文档 §3 末段明确保留）

### 3C 内容编辑域

**`ContentEditPanel`**（写态）
- 规格：`docs/specs/content-editor.md`
- 必须保住（设计文档 §4.3 写态表）：
  1. 文档列表 + kind 过滤，选中写 `?doc=`（旧实现是页面 `useState`，离开即丢——设计文档 §5-5 的违反点）
  2. 新建文档：kind / slug / 标题 + recipe 默认模板
  3. payload JSON 编辑 + **就地解析错误**
  4. **实时图形预览**：改 JSON 即重画 JEI 网格（复用 3B 的 `RecipeView`）
  5. 存草稿：`If-Match` 乐观锁；`revision_conflict` → 提示"内容已被他人修改，刷新后重试"并支持重取（`api/http.ts` 的 `ApiError.code` 就是为这个设计的）
  6. 校验：issues 按 `severity` / `code` / `path` 展示；**点一条 issue 定位到编辑器对应行**（设计文档 §5-7 的判据，旧实现未做联动）
  7. 应用：`content_not_validated` 要拦得住并说清
  8. **apply 成功后必须做两件事**：① `useCatalog().rebuild()` 或触发目录刷新，让关系态立刻看到结果（设计文档 §4.3 ⚠ + §5-3 判据「apply 成功 → 关系态该配方立刻带已魔改标记，无需手动刷新」）；② `usePackSummary().refresh()`
  9. 修订历史 + **回滚**：`api/content.ts` 已有 rollback/history 导出，设计文档 §4.3 ⚠ 指出「已定义但 UI 未接（全仓 grep 无调用）」→ 本轮接上
  10. **「应用后生成任务节点」**——设计文档 §4.3 ⚠ + §7 链路 E，**v2 的核心新增，前端完全没有**：
      - 后端已就绪：`quest_nodes.mod_refs`（落库 `content_repo.go:347`，校验 `content.go:842-844` 的 `missing_mod_reference` / `cross_pack_reference`），前端 zod 已有 `modRefs`（`api/content.ts:106`）
      - apply 成功面板上出现「生成任务节点」按钮 → 一次点击后编排态多出一个节点，**标题/图标/奖励/前置全部预填**，作者只改文案（设计文档 §6 的判据原话：「我在改配方，顺手就把它变成了一条任务」）
      - 图标默认取该物品的 catalog 图标
  11. 非 recipe 类型（structure / ore）无图形预览 → **文案里说清**，不要显示空框（设计文档 §4.3 ⚠）
  12. 原始 payload JSON 查看：设计文档 §4.3 索引态表要求把它移到写态的「源 JSON」折叠区，读写共用

### 3D 任务书域

**`InfiniteCanvas`**（共享画布，本步只有任务书在用；进度树已砍）
- 规格：`docs/design/mp-infinite-canvas.md`（399 行，是本组件的权威）
- 必须保住：拖拽 + **吸附参考线** + 双击建节点 + 右键菜单 + 缩放条；缩放范围用 `viewport.ts` 的 `QUEST_ZOOM_MIN/MAX`、`ADV_ZOOM_MIN/MAX`，**不要在组件里另写常量**
- 数学一律调 `src/domain/canvas/viewport.ts`（`boundsOf`/`clampZoom`/`fitBounds`/`gridMetrics`），组件里不许重复实现

**`QuestPanel`**（编排态）
- 规格：`docs/design/quest-book-ftb-experience.md`
- 必须保住（设计文档 §4.3 编排态表）：
  1. SVG 边层：箭头 + **跨章节标签** + 端口拖拽连线 + Shift 点击连线
  2. **edges 是唯一图权威，保存前派生 prerequisites** —— 调 `domain/quest/graph.ts` 的 `draftWithSyncedPrerequisites`，禁在组件里手写同步（`test-quest-canvas-edges.ts` 会红）
  3. **环检测**（`findCycle` / `wouldCreateCycle` / `canAddEdge`）+ 自环/重复边拦截
  4. 语义方向：**A→B 表示 B 依赖 A**，别搞反（`test-quest-canvas-edges.ts` 明确断言）
  5. Inspector 分组：基础 / 外观 / **依赖（含 `min_n`）** / 可见 / **任务 8 型** / **奖励 4 型**；依赖判定调 `domain/quest/deps.ts`（FTB 矩阵 `all_completed` / `one_completed` / `min_required`，`test-quest-deps.ts` 是规格）
  6. 编辑/预览切换 + **纯前端模拟完成/领奖**（预览态不发请求）
  7. 草稿/校验/应用走 `If-Match`；apply 422 自动再 validate
  8. Delete 键删节点/边
  9. 奖励/目标选物品**复用 3A 的 `ItemPicker`**（设计文档 §4.3 标注旧实现「已有」，web2 必须继续复用，不许各写一套选择器）
  10. **回滚 + 历史**：`api/content.ts` 已有导出，设计文档 §4.3 ⚠「已定义但 UI 未接」→ 本轮接上，与写态同一套交互
  11. **`modRefs` 编辑 UI**：设计文档 §4.3 ⚠「schema 有、后端落库并校验、前端无编辑 UI」→ 本轮补：Inspector 显示该节点引用的配方与模组，**可点回关系态/写态**（带焦点参数）
  12. **节点图标**：设计文档 §4.3 ⚠「与 catalog 图标是两套」→ 改为**默认取物品图标，emoji 作为兜底**

**`ChapterRail`**（编排态的左列）
- 章节列表 + 新增 + 计数 + 告警徽标
- 编排态时它**取代**来源栏（设计文档 §3 末段），不是并存——第二步的 `SourceRail` 已实现这个切换，本步只提供内容

### 3E 模组与来源域

**`ModSourceRail`**（来源栏）
- 必须保住（设计文档 §4.3 页内左列表）：
  1. 包内模组清单 + **启停** + **钉版** + 依赖状态
  2. 本地 jar 模组
  3. 解析状态点 + 抽屉里的**解析指标**（已解析 / 动态配方 / 错误 / 总文件）
  4. 「贡献物品」反查 → `setSearchParams(p => {p.set('ns', modId); p.set('mode','index'); return p})`
  5. **目录加载只准走 `useCatalog()`**——设计文档 §4.3 ⚠ 指出旧实现这一页自己拉了第二套 catalog，是并行加载逻辑；web2 里出现第二处 `getItemCatalog(` 调用就是违规（第二步 §7.2 检查 J 会抓到）

**`ModSearchDrawer`**
- 双平台**并发**搜索（`api/mods.ts` 的 mod-search 端点；`searchAllItemSchema` 带 `provider` 与可选 `mirror`）
- 部分平台失败时：`errors` 字段是 `Record<string,string>`，要**如实显示哪个源失败了**，不要静默吞掉
- 搜到 → 查兼容版本（`listModVersions(packId, provider, projectId)`）→ **添加即钉版**
- 兼容知识库推荐（`listModRecommendations`）：人工核实条目，显示"适用于当前包的常见兼容性模组"

**长任务：解析 / 重建**
- 触发：`parseModContent` / `rebuildItemCatalog` / `resolveModContentIcons`
- **轮询必须在 context 层**（第二步的 `CatalogContext` 已实现 status 轮询；`PackSummaryContext` 已实现 tasks 轮询）。组件里不许再开 `setInterval`——设计文档 §5-6 的违反点就是"轮询在页面内，离开页面即丢进度"
- 图标解析（`resolveModContentIcons`）也要走任务 + 轮询

**~~`AdvancementTree`~~**（进度树视图）—— **D-3B 已拍板砍掉（2026-10-01，用户："按推荐的来"），本步不实现，也不要建文件**
- 砍掉的范围：`advLayout.ts`(805) + `AdvancementTreeView.tsx`(591) + `advTreeDemo.tsx`(108) = 1504 行，以及 `test-adv-layout.ts`(454) / `adv-tree-e2e.mjs`(220) 两个脚本。全部留在 `apps/web`，不迁不删。
- 保留的范围：索引态的 `进度` 内容 kind 筛选（那是内容文档类型，走 `listModContent`），以及 `InfiniteCanvas`（任务书仍在用）。
- 理由：进度树是"浏览原版进度"的能力，不是整合包作者的核心工作流；砍掉后 3E 少一半工作量。
- 若日后要恢复：按 `advancement-tree-canvas.md` + `advancement-vanilla-experience.md` v1.1 新写，并把 `advLayout.ts` 与 `test-adv-layout.ts` 一起迁入（测试就是规格）。这是独立的一批，不塞进 3E。

### 3F 交付域

规格：设计文档 §4.4 + `docs/specs/publish.md` + `docs/architecture/launcher-core-spec.md`

**`ChecksSection`**
- `delivery-checks` 列表 + 「重新检查」
- **禁做假数据**：设计文档 §4.4 ⚠ 指出旧实现在无记录时 POST 一个占位 check。没有记录就显示空态 + 「重新检查」按钮
- **闸门联动**：有 error 级未解决冲突时构建按钮禁用 + 「去概览处理」链接（`/packs/:id?panel=conflicts`）。数据来自 `usePackSummary().pendingConflicts`。设计文档 §4.4 ⚠：后端有 `assertPackBuildable` 闸门，前端不反映的话作者点了构建才吃 409

**`BuildSection`**
- 导出目录注册（`api/fs.ts`；目录浏览端点 **需令牌**，用 `tokenHeaders()`）
- 版本登记（版本号 + draft/release 渠道）
- 开始构建：**省略 `files[]`**，服务端按权威链装配真 `.mrpack`（设计文档 §4.4 明确）
- 产物清单：文件名 / kind / 大小 / sha256
- **产物下载**：设计文档 §4.4 ⚠「后端 `GET /api/packs/:id/artifacts/{aid}/download` 已存在，前端无 wrapper、页面无按钮」→ 本轮补。
  **实现方式**：直接 `<a href={url} download>`，**不要**走 `api/http.ts` 的 fetch 封装（该端点免令牌，且返回二进制，`parseResponse` 会当 JSON 解析而报错）

**`InstallSection`**
- 选游戏目录 → 查该目录已装版本 → 安装（`artifact_id` 或版本）
- 契约：`api/launcher.ts`；内核协议见 `docs/architecture/launcher-core-spec.md`
- **API 名以文件为准（已核对，全部存在）**：
  - `installLauncher(input: LauncherInstallInput): Promise<{taskId}>` → `POST /api/launcher/install`（202 入队，返回 taskId，前端拿它去轮询任务 + 拉日志）
  - `listLauncherInstalls(packId?, minecraftDir?): Promise<LauncherInstall[]>` → `GET /api/launcher/installs`
  - `LauncherInstall = {id, minecraftDir, versionId, loader, mcVersion, packId, taskId, installedAt}`
- **入参 `version` 必须来自 `listLauncherInstalls()` 的 `versionId`，不是包的 `mcVersion`**（`launcher.ts:57` 的注释明确：带加载器时它是 `fabric-loader-<ver>-<mc>`）。用错就是装不上还报不明错误
- 请求体是 snake_case（`loader_version` / `java_path` / `minecraft_dir` / `pack_id`），但**这层已经在 `api/launcher.ts` 里做完了**，组件只传 camelCase 的 input，不要在组件里再手写 snake_case
- 游戏目录选择用 `api/fs.ts` 的浏览端点，**需令牌**（`tokenHeaders()`）
- 没装 `mpack-launcher` 时后端同步返回 503 → **如实显示**"未检测到启动器内核"，不要伪装成成功

**`LaunchSection`**
- 离线用户名 + 内存 + Java 路径 → 启动 → **日志逐行呈现**
- 契约（已核对，全部存在于 `api/launcher.ts`）：
  - `launchLauncher(input: LauncherLaunchInput): Promise<{taskId}>` → `POST /api/launcher/launch`，input = `{version?, username, minecraftDir, javaPath?, xmxMb?, packId?}`；`version` 留空由后端解析该目录实际装出来的版本
  - `fetchTaskLog(taskId): Promise<TaskLogEvent[]>` → `GET /api/tasks/{taskId}/log`
  - `TaskLogEvent = {id, taskId, sequence, status, message, detail: unknown, createdAt}`
- ⚠ **`fetchTaskLog` 不是流式读**：它是一次性 `fetch` + `res.text()` + `split('\n')` + 每行 `JSON.parse`（`launcher.ts:88-95`）。所谓"实时日志"= **按 1s 左右轮询这个端点并重渲染**，用 `sequence` 去重/排序。不许写成 `ReadableStream` 读法，也不许改 api 层去造一个流式版本
- 日志轮询只在起窗任务处于活跃态（`queued`/`running`）时开；终态停轮询并保留最后一次结果
- **错误高亮按 `status` 分类**（`failed`/`error` 之类的真实取值），`detail` 是 `unknown` → 原样 `JSON.stringify` 折叠展示，**不要断言它的字段名**
- ⛔ 设计文档 §4.4 里写的"按后端 `errorKind` 高亮"是**设想，后端没有这个字段**（全仓 grep `errorKind|error_kind` 只命中文档本身）。不许编字段；能拿到的分类只有 `status` + `message` 文本
- 这是"流水线终局必须通向启动一个 Minecraft"的那一步，**不许降级成只显示按钮**

**`PublishSection`**
- 设计文档 §4.4 ⚠：`api/releases.ts` 的 `publishPack` / `pollRelease` / `retryRelease` **全仓无 UI 调用** → 本轮接上
- 发布到 CurseForge / Modrinth + 轮询 + 重试
- **无凭证时按后端语义拒绝并说明**（这是 by-design，不是缺陷）；禁显示假的"发布成功"

### 3G 工作台域

规格：设计文档 §4.1 + `docs/design/dashboard-page-prompt.md` + `docs/specs/pack-workbench.md`

**`PackList`**
- 包列表 + 「只看待处理」开关 + **删除确认**
- **禁做**：重命名 / 复制 / 一键打包三个 Dropdown——设计文档 §4.1 ⚠ 指出旧实现只弹「后续版本提供」。要么接真实端点，要么不渲染

**`TaskPanel`**
- 暂停 / 继续 / 取消 / 重试 / 错误详情（`api/tasks.ts`）
- `Task.status` 枚举 = `queued|running|success|failed|cancelled|paused`，**没有 `pending`**（以 `api/tasks.ts` 为准）
- **禁做**：「全部任务」占位入口（设计文档 §4.1 ⚠）
- 概览页要能看到**本包**任务：`tasks.filter(t => t.packId === packId)`（设计文档 §4.2「后台任务（本包）」）

**`PackModals`**
- 新建包 / 导入包（**两阶段 + 幂等键**，见 `api/imports.ts` 与设计文档 §4.1）
- 导入支持 `.mrpack`（后端已用真实 Rust 内核实现）

**`HealthPanel`**
- 数据只从 `usePackSummary()` 取；健康分公式**在 context 里**，组件不许再算一遍（设计文档 §4.2 ⚠：公式别只活在组件里）

**`ConflictList`**
- 行级 **resolve / ignore**（第二步已建骨架，本步做完整呈现：冲突类型、涉及的模组、建议动作）
- 处理后必须 `refresh()`，让健康分 / 顶栏告警 / 交付闸门三处同时更新（设计文档 §5-4 判据）
- 「重新解析依赖」调 `resolvePack`
- ⚠ 冲突摘要**不要暴露原始 provider id**（这是已修过的缺陷 O8，别在新实现里退回去）

**`ActivityFeed`**
- 最近动态 + 今日已解决计数

---

## 5. 迁移命令（§2.1 的纯函数 + §1.4 的测试）

```bash
cd /Volumes/Evo/code/mPackStation
mkdir -p apps/web2/src/domain/{quest,canvas} apps/web2/scripts

# 纯函数层（迁移，零改动）。advLayout 不迁：D-3B 已砍进度树
cp apps/web/src/features/quest/questGraph.ts    apps/web2/src/domain/quest/graph.ts
cp apps/web/src/features/quest/questDeps.ts     apps/web2/src/domain/quest/deps.ts
cp apps/web/src/ui/canvas/viewport.ts           apps/web2/src/domain/canvas/viewport.ts

# headless 测试（迁移后必须改 import 路径）。test-adv-layout / adv-tree-e2e 不迁
cp apps/web/scripts/test-viewport.ts            apps/web2/scripts/
cp apps/web/scripts/test-quest-deps.ts          apps/web2/scripts/
cp apps/web/scripts/test-quest-canvas-edges.ts  apps/web2/scripts/
cp apps/web/scripts/questGraph.check.ts         apps/web2/scripts/
cp apps/web/scripts/questM1SaveBody.ts          apps/web2/scripts/
```

改 import 路径（**只改路径，不改断言**）：

| 脚本 | 旧 import | 新 import |
|---|---|---|
| `test-viewport.ts` | `../src/ui/canvas/viewport.ts`（以文件实际内容为准） | `../src/domain/canvas/viewport.ts` |
| `test-quest-deps.ts` | `../src/features/quest/questDeps.ts` | `../src/domain/quest/deps.ts` |
| `test-quest-canvas-edges.ts` · `questGraph.check.ts` · `questM1SaveBody.ts` | `../src/features/quest/questGraph.ts` | `../src/domain/quest/graph.ts` |

在 `apps/web2/package.json` 的 `scripts` 里**追加**（第一步故意没加，见 D6）：

```json
"test:viewport": "node --experimental-strip-types scripts/test-viewport.ts",
"test:quest": "node --experimental-strip-types scripts/questGraph.check.ts && node --experimental-strip-types scripts/test-quest-deps.ts && node --experimental-strip-types scripts/test-quest-canvas-edges.ts"
```

**迁移即刻验收**（这一步必须在写任何渲染代码之前跑绿）：

```bash
cd apps/web2 && npm run test:viewport && npm run test:quest && echo "纯函数层全绿"
```

不绿就停下来查 import 路径，**不要改断言去迁就**。断言是规格。

---

## 6. 每批验收

### 6.0 每批通用（都要跑）

```bash
cd /Volumes/Evo/code/mPackStation/apps/web2
npm run test && npm run build                       # 0 错误
npm run test:viewport && npm run test:quest                          # 纯函数层不退化
cd .. && cd ..
grep -rln 'Stub' apps/web2/src/pages apps/web2/src/app || echo "本批容器里无 Stub 残留"
ls apps/web2/src/stubs/                              # 对应 Stub 文件必须已删除
grep -rniE "后续版本提供|敬请期待|coming soon|暂未实现" apps/web2/src || echo "无假 affordance"
grep -rn 'setInterval' apps/web2/src/widgets apps/web2/src/pages || echo "组件层无自建轮询"
grep -rln 'getItemCatalog(' apps/web2/src | grep -v 'api/catalog.ts'   # 只应命中 CatalogContext.tsx
```

### 3A 验收（浏览器）

1. 索引态显示真实物品网格（隔离库 257MB，有真数据），图标正常、缺图标显示占位不是破图
2. 搜索名称/ID 生效；命名空间过滤生效且**计数正确**
3. 类型筛选 10 类 kind 都能切，URL 的 `?type=` 跟着变
4. 点一个物品 → 右栏焦点段显示：图标 / 名称 / ID / 来源模组 / 配方数 / 被引用数 / 标签
5. **右栏出现多语言名称表**（`names[]` 的全部 locale）← 这条是能力不回退的关键，必查
6. 右栏标签可点 → 跳到该标签的成员视图，不是弹一个死胡同 Modal
7. 焦点为物品且它有配方时，右栏**直接显示主配方预览**
8. 方向键在网格内移动，Enter 进关系态且焦点不变
9. 空态（选一个没物品的命名空间）→ 文案含可点主按钮

### 3B 验收

1. 关系态显示"作为产物 N 条 / 作为原料 N 条"两个方向
2. 6 类配方都能渲染：`shaped` / `shapeless` / 机器配方 / `ae2 transform` / `matter_cannon` / 特殊配方（特殊配方显示**中文说明**而不是空网格）
3. **点材料 → 重新居中，连续下钻 5 层不换页**，URL 的 `item=` 一直在变、`mode=graph` 不变
4. 已魔改的配方带「已魔改」标记（若这条拿不到数据，明确报告"后端缺 applied 集合"，**不要跳过也不要造假**）
5. 点「魔改此配方」→ 写态打开的**正是这条 payload**（设计文档 §5-3 的判据）
6. 物品选择器可用，与来源栏职责不混

### 3C 验收

1. 文档列表选中态进 URL（`?doc=`），刷新后仍选中
2. 改 JSON → 预览实时重画
3. 存草稿：改坏 revision 触发 `revision_conflict` → 界面提示"内容已被他人修改，刷新后重试"
4. 校验失败：issue 按 severity/code/path 展示，**点一条定位到编辑器对应行**
5. 未校验就 apply → 被 `content_not_validated` 拦住且说清
6. apply 成功 → **不手动刷新**，切到关系态立刻看到「已魔改」标记；顶栏告警/健康分同步变化
7. 修订历史可见，回滚可用
8. **apply 成功面板上有「生成任务节点」**；点一次 → 编排态多出一个节点，标题/图标/奖励/前置已预填
9. structure / ore 类型明确写"无图形预览"，不是空框
10. 「源 JSON」折叠区能看到原始 payload

### 3D 验收

1. `npm run test:quest` 全绿（edges 权威 / 环检测 / prerequisites 派生 / FTB 依赖矩阵）
2. 画布：拖拽有吸附参考线、双击建节点、右键菜单、缩放条、Delete 删节点/边
3. 端口拖拽连线成功；自环被拦；重复边被拦；成环被拦并说清是哪条边
4. 跨章节连线允许，且边上有跨章标签
5. Inspector 五组齐全（基础/外观/依赖含 `min_n`/可见/任务 8 型/奖励 4 型）
6. 依赖矩阵行为符合 FTB 语义（`all_completed` / `one_completed` / `min_required`）
7. 奖励与目标选物品**复用 `ItemPicker`**（不是另一套选择器）
8. 预览态模拟完成/领奖，**不发任何请求**（Network 面板核查）
9. **`modRefs` 可编辑**：节点 Inspector 能添加/删除引用的配方与模组，点引用能跳回关系态/写态且带焦点
10. 节点图标默认取物品图标，无物品图标时才落 emoji
11. 保存的 payload 里 `prerequisites` 由 edges 派生（对照 `questM1SaveBody.ts` 的形状）
12. 回滚 + 历史可用

### 3E 验收

1. 来源栏默认折叠 56px，展开 260px，折叠态刷新后保持
2. 模组行显示：名称 + 启停开关 + 解析状态点；点开抽屉有解析指标（已解析/动态配方/错误/总文件）
3. 「贡献 N 物品」→ `?mode=index&ns=<modId>`，索引态只剩该模组物品
4. 添加模组抽屉：双平台**并发**搜索；某平台失败时**如实显示哪个源失败**
5. 搜到 → 查兼容版本 → 添加即钉版 → 触发解析任务 → **顶栏胶囊显示进度**
6. 触发解析后立刻切到别的态做别的事，再切回来**进度还在**（设计文档 §5-6 判据）
7. 重建目录 / 解析图标都走任务 + context 轮询，`grep setInterval` 在 widgets 下为 0
8. Network 里 `catalog?locale=` 全程只有 1 次（除重建后自动刷新）
9. `grep -rn 'AdvancementTree\|advLayout' apps/web2/src` 输出为空（D-3B 已砍，不许有半拉子实现或空壳文件）

### 3F 验收

1. 五段一屏纵向排布，`?step=install` 能锚定并高亮
2. 无检查记录时显示空态 + 「重新检查」，**没有假 check 被 POST**（Network 核查）
3. 有待解决冲突时构建按钮**禁用**且给出「去概览处理」链接；在概览 ignore 掉冲突后回来，按钮**自动放开**
4. 构建：选导出目录 → 填版本号 + 渠道 → 开始构建（请求体**不含 `files[]`**）→ 产物清单显示文件名/kind/大小/sha256
5. **产物下载是一个真按钮**，点了浏览器开始下载（`<a download>`，不走 fetch 封装）
6. 安装：选游戏目录 → 显示该目录已装版本 → 安装；`version` 入参取自 `listLauncherInstalls()` 的 `versionId`（不是包的 `mcVersion`）；没装内核时**如实报 503 语义**
7. 起窗：填离线用户名 + 内存 + Java 路径 → 启动 → 日志按 `sequence` 逐行出现（轮询 `GET /api/tasks/{id}/log`，Network 里能看到重复请求；任务终态后轮询停止），失败行按 `status` 高亮，`detail` 折叠展示原文
8. **终局验证：真的能启动一个 Minecraft 窗口**（这是用户的standing 要求；隔离环境里若不便真起，必须在开发栈上验一次并留证）
9. 发布：接上 `publishPack` / `pollRelease` / `retryRelease`；无凭证时按后端语义拒绝并说明，**不显示假的成功**

### 3G 验收

1. 工作台四块布局；空态迎新有「新建」「导入」两个真按钮；**没有不可点的灵感卡**
2. 包列表「只看待处理」开关生效；删除有二次确认
3. **没有**重命名/复制/一键打包的假 Dropdown
4. 任务面板：暂停/继续/取消/重试/错误详情都能点且有真实请求；**没有「全部任务」占位**
5. 概览显示本包任务（按 `packId` 过滤）
6. 新建包 / 导入包（两阶段 + 幂等键）成功；导入 `.mrpack` 走真实内核
7. 冲突行级 resolve / ignore → 健康分 + 顶栏告警 + 交付闸门**三处同时更新**
8. 冲突摘要**不暴露原始 provider id**
9. 健康分只有一个来源（`grep -rn 'computeScore' apps/web2/src` 只命中 `PackSummaryContext.tsx`）

### 6.9 总验收：端到端链路（设计文档 §7 的 A–F 全跑）

| 链路 | 判据 |
|---|---|
| **A** 从空机器到玩起来 | 新建包 → 加模组（真下 jar + 解析）→ 概览出现缺依赖信号 → **在概览就地**看到"JEI 需要 fabric-api" → 去添加 → 重新解析 → 冲突自动结案 → 健康分转绿 → 交付页构建 → 安装 → **启动 Minecraft** |
| **B** 处理冲突不跳页 | 概览冲突行上直接 resolve/ignore，处理后健康分、顶栏告警、交付闸门三处同时更新 |
| **C** 顺科技树查来源 | 关系态连续下钻 ≥5 层不换页，右栏焦点与面包屑一路一致，刷新不变 |
| **D** 魔改一条配方 | **全程只用键盘**：⌘K 设焦点 → Enter 进关系态 → 「魔改此配方」→ 改 JSON → 校验 → apply → 回关系态看到「已魔改」（设计文档 §5-10 判据） |
| **E** 魔改顺路生成任务 | apply 成功面板点「生成任务节点」→ 编排态多出预填节点，作者只改文案 |
| **F** 出包一屏走完 | 交付页从上到下五段一屏；构建完直接在同一屏安装、起窗；下载产物是一个按钮 |

### 6.10 最终 grep 归零

```bash
cd /Volumes/Evo/code/mPackStation
# Stub 全清
ls apps/web2/src/stubs 2>/dev/null && echo "❌ stubs 目录还在" || echo "✅ stubs 已清空删除"
grep -rn 'Stub' apps/web2/src && echo "❌ 有 Stub 残留" || echo "✅ 无 Stub 引用"
# 残渣没被带进来
grep -rn 'advTreeDemo\|ModulePlaceholder\|ui/workbench\|design-previews' apps/web2 || echo "✅ 无残渣"
# 假 affordance
grep -rniE "后续版本提供|敬请期待|coming soon|暂未实现" apps/web2/src || echo "✅ 无假 affordance"
# 单一事实源
grep -rln 'getItemCatalog(' apps/web2/src | grep -v 'api/catalog.ts'    # 只应 1 行 CatalogContext
grep -rn 'computeScore' apps/web2/src                                   # 只应命中 PackSummaryContext
grep -rn 'setInterval' apps/web2/src/widgets apps/web2/src/pages || echo "✅ 组件层无自建轮询"
# 令牌
grep -rniE "chain-token|X-MPack-Token:\s*['\"][A-Za-z0-9]" apps/web2 || echo "✅ 无硬编码令牌"
# 路由与导航数量
grep -c "path: '" apps/web2/src/app/router.tsx                          # 6
grep -c "scope: 'global'" apps/web2/src/app/nav.ts                      # 2
grep -c "scope: 'pack'" apps/web2/src/app/nav.ts                        # 3
# 体量（报告用，不是判据）
find apps/web2/src -name '*.ts' -o -name '*.tsx' | xargs wc -l | tail -1
find apps/web2/src -name '*.css' | xargs wc -l | tail -1
```

---

## 7. 禁止清单

- 禁改 `pages/` 与 `app/` 下的容器文件（插槽签名不够用就停下来问）
- 禁改 `api/` 已有导出（只准追加）
- 禁改 `src/domain/` 下的纯函数**断言**（可以改实现，但测试必须绿；实际上本步是迁移，不该改）
- 禁在组件里复制纯函数的常量字面值（`NODE_SIZE`、缩放上下限等一律 import）
- 禁在组件里开 `setInterval`（轮询只准在两个 context 里）
- 禁第二处 `getItemCatalog(` 调用
- 禁第二份健康分公式
- 禁渲染没有后端支撑的按钮 / Dropdown / 卡片
- 禁把二进制下载走 `api/http.ts` 的 fetch 封装
- 禁显示令牌值
- 禁 `git add` / `git commit`
- 禁实现进度树（`AdvancementTree` / `advLayout`）——D-3B 已拍板砍掉，写了就是超范围

---

## 8. 决策点（**已全部拍板，实施时不要再停下来问**）

2026-10-01 用户批复："按推荐的来就行"。四条全部按建议值执行，本文档其余章节已与之一致。

| 编号 | 决策 | 结论 |
|---|---|---|
| **D-3A** | 纯函数层迁移还是照测试重写 | **迁移**。测试已覆盖，重写无产品收益 |
| **D-3B** | 原版进度树砍不砍（`advLayout` 805 + `AdvancementTreeView` 591 + demo 108 = 1504 行） | **砍**。索引态只留网格 / 表格；3E 少一半工作量；最终体量 ~8.5k 而不是 ~10k。相关文件与脚本全部留在 `apps/web`，不迁不删（见 §4 的 3E 段） |
| **D-3C** | 任务书的 8 型任务 / 4 型奖励要不要精简 | **不精简**。这是产品能力，砍了作者做不出复杂任务链 |
| **D-3D** | `api/` 的 zod 逐字段声明要不要改宽松 | **不改**。172 条链路测试全绿就是这层换来的 |

第一步的 §8 决策点（`api/` 整目录迁移 vs 照 Go handler 重写 zod）同样按**迁移**执行。

---

## 9. 收尾：web2 替换 web（**必须用户拍板，不许自行执行**）

第三步全部验收通过后：

1. 出一份对照报告：web2 与 web 的能力对照（设计文档 §8「能力不丢对照表」逐条核对）+ 体量对比 + 链路 A–F 的验证证据
2. 报告交给用户，**等明确指令**
3. 得到指令后才执行替换：`apps/web` 移到备份位置（不删）→ `apps/web2` 改名 `apps/web` → 端口改回 5273 → proxy 默认目标由用户决定
4. 替换后再跑一遍 §6.9 的链路 A–F

**在用户拍板前，`apps/web` 一个字都不许动。**

---

## 10. 完成标志

- §5 的纯函数测试全绿
- 3A–3G 每批的 §6 验收逐条通过并留证
- §6.9 链路 A–F 全通，其中**链路 A 的终局是真的启动了一个 Minecraft**
- §6.10 grep 全部输出 ✅
- `apps/web2/src/stubs/` 已删除
- §8 四个决策点用户已回话
- §9 的对照报告已交付
