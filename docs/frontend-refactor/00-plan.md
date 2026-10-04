# 前端重构 · 主执行方案

> **本文件的用途**：上下文被压缩后，从本文件第 1 节读回「要做什么、做到哪、下一步是什么」，
> 然后从第 9 节的进度表继续。不要靠记忆推进。
>
> 关联文档：
> - 规范：`docs/standards/frontend-standards.md`
> - I18N 专项：`docs/frontend-refactor/01-i18n.md`
> - 主题专项：`docs/frontend-refactor/02-theme.md`

**执行约束（用户 2026-10-04 定）**：本轮不开子代理，全部独立完成；I18N 本轮一起做完，不留口子。

---

## 0. 已完成的前置诊断（结论先记下来，别重复查）

### 0.1 搜索链路故障 F1/F2 —— **不是代码回归，是后端进程没有代理环境变量**

| 步骤 | 事实 |
|---|---|
| 现象 | `thermal` / `mek` / `jei` 搜索全部返回 `total=0`，降级区也没有 |
| 后端健康接口 | `modrinthReachable:false`、`curseforgeReachable:false` |
| 单平台搜索报错 | `provider_unavailable` |
| 直连验证 | `curl https://api.modrinth.com/v2/search?query=mekanism` → **200**，0.7s，数据正常 |
| 根因 | shell 有 `HTTP_PROXY=http://127.0.0.1:52135`，但后端进程（旧 pid 66019）**环境里没有任何 proxy 变量** → Go 出不了网 |
| 验证修复 | 带代理环境重启后端（新 pid 38734）后：`modrinthReachable:true`，搜索全部恢复 |

重启后实测（证明后端代码正确、降级没被改丢）：

```
q=thermal   主 0 条    降级 2 条   Thermal: Extra / Thermal Vision Goggles (forge,neoforge)
q=mek       主 2 条    降级 10 条  Mekanism / Generators / Tools …(forge,neoforge)
q=jei       主 25 条   降级 0 条   含 Just Enough Items (JEI) (fabric,forge,neoforge)
```

**结论**：
- `thermal` 主结果 0 是**正确行为** —— Thermal 系列 1.21.1 只有 Forge/NeoForge 版，降级区正确兜住了。
- `mek` 降级 10 条正常，**降级逻辑没有被改丢**。
- 因此 **F1/F2 不进入本轮改动**，只需要在开发脚本里固化代理（见 0.2）。

### 0.2 顺带要固化的一件事

`scripts/dev.sh` 启动后端时**没有传递代理环境变量**，任何人重启服务都会重现「搜索全空」。
本轮在 `scripts/dev.sh` 里补一段：若系统存在 `HTTP(S)_PROXY`，则透传给后端进程（`NO_PROXY` 保留回环）。

### 0.3 现状事实（写方案时读到的，避免重复探索）

- 源码在 `apps/web3/src`，现有子目录：`api`(16) `app`(23) `chain`(5) `dock`(1) `editor`(17) `panels`(9) `styles`(2)，根目录散着 `main.tsx` / `vite-env.d.ts`。
- **已有两套右键菜单并存**：`app/GlobalContextMenu.tsx` 与 `panels/ContextMenu.tsx` —— 这就是「菜单没封装」的实证。
- **底边栏已存在**：`dock/BottomDock.tsx`（单文件），日志相关还有 `panels/RunPanel.tsx`、`panels/BuildPanel.tsx`、`panels/ActivityFooter.tsx`。
- **主题 token 已存在且规范**：`styles/tokens.css`（69 行，`:root` 上 `--mc-*` 前缀，含中性色/品牌色/间距/圆角/布局尺寸）。换主题的基础已经具备 80%。
- 左侧图标栏是 `app/IconRail.tsx`，图标注册表是 `app/Icon.tsx`（语义名 → lucide 组件）。
- 样式分 4 处：`editor/editor.css`(1086) `app/frame.css`(471) `panels/panels.css`(287) `styles/tokens.css`(69) `styles/base.css`(28)。

---

## 1. 目标目录结构（按用途划分）

用户要求：把 JS/TS/TSX 收进统一的大目录，按用途划分（与后端交互的一类、前端交互的一类）。

```
apps/web3/src/
├── api/          与后端交互（不变，16 个文件保留）
│                  规则：只放请求函数 + zod schema + 类型，不放 JSX
├── ui/           ★新建 · 基础组件层（零业务，任何层可引）
│                  Icon Button TextInput SearchInput Select Menu ContextMenu
│                  Popover Modal Tree Row Empty Chip Tooltip SplitPanel
├── components/   ★新建 · 组合层（带语义、跨面板复用）
│                  ItemPicker ItemIcon ModRow SearchBar HealthDock
│                  LogDock ProviderLabel VersionRail
├── panels/       业务面板（SourcesPanel StorePanel QuestPanel BuildPanel …）
├── editor/       编辑器（保留，内部改用 ui/ 组件）
├── app/          应用骨架（瘦身：只留 AppFrame router url useHotkeys 等上下文与路由）
├── i18n/         ★新建 · 语言包与初始化
├── styles/       设计令牌与主题（保留并扩充）
└── main.tsx
```

**依赖方向**：`ui → components → panels/editor → app`，只能自上而下，禁止反向引用、禁止跨层。

### 1.1 迁移映射表（现有文件 → 新位置）

| 现在的位置 | 迁到 | 说明 |
|---|---|---|
| `app/Icon.tsx` | `ui/Icon.tsx` | 图标唯一入口 |
| `app/ItemIcon.tsx` | `components/ItemIcon.tsx` | 带业务语义（物品图标） |
| `app/ItemIdInput.tsx` | `components/ItemPicker.tsx` | 收编 `editor/FilterBuilder.tsx` 里的私有建议框 |
| `app/StubItemSearch.tsx` | `components/ItemPicker.tsx` | 合并，消除桩 |
| `app/GlobalContextMenu.tsx` | **`ui/ContextMenu.tsx`**（与下面合并） | 两套菜单合成一套 |
| `panels/ContextMenu.tsx` | **`ui/ContextMenu.tsx`** | 同上 |
| `app/StatusBar.tsx` + `app/ProblemsPopover.tsx` | `components/HealthDock.tsx` | 合并为底边栏入口 |
| `dock/BottomDock.tsx` | `components/LogDock.tsx` | 按用户要求重做布局 |
| `app/IconRail.tsx` | `components/VersionRail.tsx` | 找回版本修订入口 |
| `app/catalogSearch.ts` | `api/catalog.ts` 或 `components/` 侧 | 判定后定，倾向留在 api |
| `chain/` (5) | `editor/chain/` | 归入编辑器域 |
| `app/Onboarding*`、`SettingsModal`、`CommandPalette` | `app/` 保留 | 属应用骨架 |
| 其余 `app/*` 上下文 | `app/` 保留 | `PackSummaryContext` `CatalogContext` `url` `router` `useHotkeys` |

> 迁移用 `git mv` 逐个做，每迁一个跑一次 `tsc --noEmit`，不要一次性大搬（滚雪球式排错成本太高）。

---

## 2. 需要封装的组件

### A. 基础层 `ui/`

| 组件 | 收编什么 | 解决 |
|---|---|---|
| `Icon` | 已有映射表 | 图标唯一入口（禁自绘 path、禁 emoji） |
| `Button` | `.p-btn` + `.tp-icon-btn` 两套 | 图标/文字/危险/加载态一致 |
| `TextInput` | 20+ 处裸 `<input>` | 输入框能力不一致的总根 |
| `SearchInput` | `TextInput` + 放大镜图标按钮 | **商店/来源搜索按钮，改完入库不留在面板里** |
| `Menu` / `ContextMenu` | 两套菜单 | 一处组件，各页注入条目文本与事件 |
| `Popover` / `Modal` | 搜索弹层、分类菜单、mini-modal | Portal + 定位 + Esc 统一 |
| `Tree` | 侧栏 + 分类头 | 可折叠节点、右键、New 子菜单 |
| `Row` / `Empty` / `Chip` / `Tooltip` | 面板原子类 | 视觉一致 |
| `SplitPanel` | 日志窗口的左右分栏 | 左列表 / 右 Detail |

### B. 组合层 `components/`

| 组件 | 收编什么 | 解决 |
|---|---|---|
| `ItemPicker` | `ItemIdInput` + `FilterBuilder` 私有建议框 + `StubItemSearch` | 物品 ID 搜索下拉统一 |
| `ModRow` | 模组行（状态、展开内容统计、右键注入） | 展开显示内容统计；右键加「重新解析」 |
| `SearchBar` | 商店 + 来源两处搜索框/结果列表/降级区 | 两处行为对齐 |
| `HealthDock` | 左下健康分 + 右上感叹号 + 问题弹层 | 三者合成底边栏一个入口 |
| `LogDock` | `BottomDock` + `RunPanel` + `BuildPanel` + `ActivityFooter` | 大标题 + 左列表/右 Detail |
| `ProviderLabel` | 平台名/加载器显示名 | 界面不再出现 `modrinth` 原始标识 |
| `VersionRail` | `IconRail` | **找回包的版本修订入口** |

---

## 3. 入口迁移表（迁完顺带消掉哪些问题）

| 现有入口 | 迁到 | 消掉 |
|---|---|---|
| 商店搜索按钮「搜」字 | `SearchInput` | V1 |
| 来源面板搜索框 | `SearchInput` | 两处一致 |
| 左下健康 + 右上感叹号 + 问题弹层 | `HealthDock` | V2、F5（冲突可见） |
| 侧栏图标列 | `VersionRail` + `Tree`（三目录） | F6、C3 |
| 分类头折叠 + 「新建分类」按钮 | `Tree` 右键 → New 子菜单 | C1/C2 |
| 6 处 `onContextMenu` / 2 套菜单 | `ui/ContextMenu` | C4 |
| 20+ 处裸 `<input>` | `TextInput` | C5 |
| `FilterBuilder` 私有建议框 | `ItemPicker` | 物品 ID 下拉 |
| `BottomDock` 左右分栏 | `LogDock`（大标题 + 左列表/右 Detail） | 日志窗口改造 |
| JEI 解析行 | `ModRow` 内容统计 + i18n | V3/V4 |

---

## 4. 行为优化项（编号沿用，便于对照）

**功能**
| # | 项 | 状态 |
|---|---|---|
| F1 | `thermal` 搜不到 | **已诊断：环境问题，非代码**。仅固化 dev.sh 代理 |
| F2 | `mek` 降级消失 | **已诊断：同上，降级代码正常**。同上 |
| F3 | 搜索结果双击无反应 | 待做 |
| F4 | 搜索结果没有「添加」按钮 | 待做 |
| F5 | 加装后冲突无反馈 | 随 `HealthDock` 一起做 |
| F6 | 左栏版本修订入口缺失 | 待做（`VersionRail`） |
| F7 | 模组右键缺「重新解析」 | 待做（`ModRow` 右键注入） |

**展示**
| # | 项 | 状态 |
|---|---|---|
| V1 | 搜索按钮改放大镜 SVG | 待做（→ `SearchInput`） |
| V2 | 健康弹层位置错 | 待做（→ `HealthDock` 底边栏） |
| V3 | `jei已解析0 文件0` → `已解析 · 文件 0` | 待做 |
| V4 | 删「全靠运行时代码」那类括号说明 | 待做 |
| V5 | 平台名/加载器显示名 | 已做，收编进 `ProviderLabel` |

---

## 5. 侧栏信息架构（用户拍板）

```
<包名>                          ← 根（Tree 根）
├─ 模组管理                      ← Minecraft 本体 + 已分类目录
├─ 任务书管理                    ← 原独立入口收编进来
└─ 自建内容管理                  ← 加进整合包的自有内容
    ├─ 配方
    ├─ 物品结构
    ├─ 多方块结构
    ├─ 地形 / 群系
    └─ 生物 / 资源 / 群系变更
```

- 右侧统一是编辑区，任务书不再单独占侧栏入口。
- 根与目录右键 → 「新建…」子菜单（含「添加模组」「新建目录」）。
- 左栏**包的版本修订**入口找回（用户确认此功能与图标都曾存在，某次迭代丢失）—— 在 `IconRail.tsx` 与 `router.tsx` 的历史里找。

---

## 6. 底边栏与日志窗口（用户拍板）

- 底边栏范式确认（IDEA 式，从底部展开）。
- **日志窗口改造**：现在是无标题的左右分栏，改为
  ```
  ┌ Log ─────────────────────────────┐   ← 大标题（Log / Build Log / Run Log，按当前上下文切）
  │ 左侧：每一条数据（列表） │ 右侧：这一条的 Detail │
  └──────────────────────────────────┘
  ```
  用 `ui/SplitPanel` 实现，左列选中项驱动右侧 Detail。
- 健康分与问题列表共用同一个底边栏容器（`HealthDock`），不再右下角悬浮。

---

## 7. 执行顺序（严格按此推进，不要跳）

| 批次 | 内容 | 完成判据 |
|---|---|---|
| **B0** | `dev.sh` 固化代理 + 记录诊断结论 | 重启后 `modrinthReachable:true` |
| **B1** | 写规范 `docs/standards/frontend-standards.md` | 文档落盘 |
| **B2** | 主题专项（先做：其它组件都依赖 token） | 见 `02-theme.md` |
| **B3** | 建 `ui/` 基础层（Icon→Button→TextInput→SearchInput→Menu/ContextMenu→Popover→Tree→SplitPanel） | 每个组件迁一个调一个 |
| **B4** | 建 `components/` 组合层（ItemPicker / ModRow / SearchBar / HealthDock / LogDock / VersionRail） | 同上 |
| **B5** | 侧栏 `Tree` 三目录 + 版本修订入口找回 | 界面可见三目录 |
| **B6** | I18N 专项 | 见 `01-i18n.md` |
| **B7** | 行为优化收尾 F3/F4/F7 + V1/V3/V4 | 逐项验收 |
| **B8** | 全量验收（tsc + build + 真浏览器 CDP） | 0 错、截图确认 |

---

## 8. 验收口径

- 每批结束：`cd apps/web3 && ./node_modules/.bin/tsc --noEmit` 必须 0 错。
- 涉及交互的改动必须真浏览器验收（CDP），不能只跑类型检查。
- 后端改动：`go test ./internal/...` 全绿 + curl 实测接口。
- 服务：后端 18872（带代理）、前端 5271，数据在 `/tmp/mpack-data`。

---

## 9. 进度追踪（每完成一项就勾掉，上下文丢失后从这里续）

> 最新验收：批次 B3-d **CDP PASS 9 / FAIL 0**（2026-10-04 下午，/tmp/rv-b3d.mjs）；
> 此前 B2-c + B3 **PASS 14 / FAIL 0**。

- [x] 诊断 F1/F2（结论：后端进程缺代理，代码正常）
- [x] B0 `dev.sh` 固化代理（已改，带注释说明踩坑）
- [x] B1 前端规范文档（→ `docs/standards/frontend-standards.md`）
- [x] B2-a 主题扫描（结论：CSS 全走 `var(--mc-*)`，裸色值 0 处，无需收编）
- [x] B2-b 主题层（`themes.css` light/dark + `app/theme.ts` + `main.tsx` 初始化 + 设置弹窗切换入口；CDP 验收 PASS 8/0，深浅两套 body 背景实测切换正确）
- [x] B2-c antd 深色适配（做法比原计划更彻底：`app/ThemeContext.tsx` 把主题做成可订阅 state，
      ConfigProvider 挂 `darkAlgorithm/defaultAlgorithm` + 令牌在 layout effect 里同步重读；
      SettingsModal 改用 `useTheme()`。原「渲染前取一次值」的写法切主题时 antd 不会跟着变）
- [x] B3-a 右键菜单统一（**本次核心**）：新建 `ui/ContextMenu.tsx` —— 原 `panels/ContextMenu.tsx`
      改为转发；新增 `useContextMenu()` 钩子（调用方不再自己存 x/y）。全站 6 处 `onContextMenu`
      （SourcesPanel ×3 / QuestCanvas ×2 / QuestPanel / ModeIndex / GlobalContextMenu）全部接入。
      升级点：菜单项支持图标、判别联合 `MenuItem = MenuEntry | MenuSeparator`、
      贴边自动翻转进视口、子菜单可翻向左侧。样式从 panels.css 迁到 ui/ui.css。
- [x] B3-b `SearchInput` 接入商店面板（V1）+ 来源面板顶栏搜索的「搜」字按钮换成同款放大镜
- [x] B3-c 商店搜索结果加「添加」按钮 + 双击加装（F3/F4，quickAdd 取第一个兼容版本，
      口径与 SourcesPanel.isCompatible 一致；验收：25/25 行带添加按钮）
- [x] B3-e 空白区右键「新建…」子菜单（新建目录…/添加模组…），与目录右键同一模型（C2）
- [x] B3-f 搜索加载指示（用户新需求「搜索过程没有指引」）：新建 `ui/Spinner.tsx`
      （全站唯一 spinner，形状 = lucide `loader-pinwheel` 三段等距弧，动画在 ui.css；
      刻意不用 antd Spin —— 四圆点转法 + 不跟 --mc-* 令牌）；`SearchInput` 加
      `.ui-search-field` 定位层，busy 时在**输入框内部右端**（挨着放大镜按钮）转；
      `StorePanel` 加 `searching` state（过期响应不关圈，后发搜索还在飞），
      结果区同步显示「搜索中…」。验收：spinner 出现→动画名 ui-spin-rotate→
      位置在 input 右缘内、按钮左侧→搜索完成消失，tsc 通过。
- [x] B3-d `ui/` 基础组件：Button / TextInput / Modal / Prompt 已立（皮肤沿用
      `.p-btn` / `.p-input` / `.tp-icon-btn`，90 处调用点渐进迁移，新增一律走 ui/）；
      两处自写弹窗（SourcesPanel 的新建分类、BuildPanel 的目录选择）已收进
      `ui/Prompt` + `ui/Modal`，弹层样式 `.modal-backdrop` / `.mini-modal` 迁到 ui.css。
      **验收 9/9 PASS**（portal 到 body、自动聚焦、Esc 关闭且不切视图、
      创建分类生效、目录弹窗宽度未被 mini-modal 抢走）。
      Tree / Popover 归 B5 目录树一起做（见 `03-nav-and-dock.md`）。
- [x] 03 方案文档：左栏三目录 + 版本修订 + 日志窗口（`docs/frontend-refactor/03-nav-and-dock.md`）。
      **关键发现**：后端 `validContentKind` 只认 recipe / structure / ore 三种，
      用户列的「物品结构 / 多方块结构 / 地形群系 / 生物资源群系变更」无后端支撑，
      需后端先扩枚举与校验（已在文档 §2 / §7 记明）。
- [ ] B4 `components/` 组合层（ItemPicker / ModRow / SearchBar / HealthDock / ProviderLabel）
- [x] B5-a 左栏目录树（**已整棵删除**，见 B5-c）
- [x] B5-c 六区骨架落地（2026-10-04 第四轮反馈，口径见 `04-align-pack-root.md` §8）：
      用户定案六区 = 顶边栏 / 侧边按钮栏 / 侧边栏 / 主编辑区 / 底边栏 / 状态栏，
      侧边栏**只能是一块区域**（曾把 NavTree 常驻 ToolPanel 顶部切成上下两块，错）。
      执行：删 `app/NavTree.tsx` 与 `.nav-*`/`.tp-scroll` 样式，ToolPanel 还原单区域；
      `RAIL_TOOLS` = 项目管理(sources) / 新增模组(store) / 任务书(quest) /
      Get Version(releases)，顺序即轨道顺序；`Icon.tsx` 登记 `releases: GitBranch`
      （补回丢失的分支图标——手绘 `build` path 迁 lucide 时漏登记，按
      「未登记名返回 null」规则变成空白）与 `build: Hammer`；
      自建内容 / 构建 / 运行 定为**顶边栏右侧**的动作（尚未实施）。
      验收 **PASS 10/11**（唯一 FAIL 为偶发 404，复跑不复现）：
      无树、无 tp-scroll、4 按钮职责与图标齐全、四条导航 URL/高亮/内容正确。
      下一步：SourcesPanel 重构成单一列表（旧内容全部搬进新侧边栏）、顶栏右侧三按钮。
- [x] B5-d 修正（2026-10-04 用户反馈两处做错，均已修）：
      ① Get Version 图标：lucide `GitBranch` 只有 2 个点（右上+左下，左上是线头），
      不是历史上的三点分支图。改用 `GitFork` 逆时针转 90°（Icon.tsx 新增 ROTATE 表）
      = 左两点右一点，与原手绘 build 同形；**转库图标 ≠ 手绘 path**。
      ② 「构建/发布」「运行」曾被塞进 ReleasesPanel 的「交付动作」段——
      一级能力降成二级入口，已整段删除；顶栏右侧新增三个独立按钮
      自建内容(wrench)/构建(Hammer)/运行(Play)，`.tb-act`+`.tb-btn.on` 高亮。
      查证「包管理内容被删」：库里该包只有 1 个模组（JEI），侧边栏显示的就是
      全部数据，未删内容。验收 PASS 12/13（唯一 FAIL 为间歇 404，复跑不复现）。
      （B5-d 后半被用户再次纠正：GitFork 旋转仍是「我自己画」，见 B5-e①）
- [x] B5-e 侧边栏换血 + 图标原样取回（2026-10-04 用户再次纠偏后）：
      ① **Get Version 图标 = 从 git 历史原样取回**（`git show HEAD:.../Icon.tsx`
      的 PATHS.build，16 栅格三点分支图：左 4.5,4 / 4.5,12、右 11.5,4）。
      Icon.tsx 新增 `LEGACY` 表：**渲染走自己的 16 栅格 + stroke 1.5**，
      不迁移、不重画、不旋转 —— lucide 没有等价形状，改一个像素等于换图标。
      删除 B5-d 的 ROTATE/GitFork 方案。
      ② **侧边栏 = 包根目录树**（B5-c 把树删了是搞反了新旧）：重建
      `app/NavTree.tsx`，根 = 当前包（名 + MC 版本 · 加载器）→ 三分组
      （模组管理 installed/total / 任务书管理 / 自建内容管理，计数取自
      dashboard 聚合读模型），**展开分组即内联该类的面板**——旧「来源」
      面板整块搬进「模组管理」下，不再单独占屏；侧边栏只有一块区域
      （ToolPanel 仅 tool=sources 时渲染 NavTree，其余 tool 照旧各面板）。
      SourcesPanel 的「来源 N」标题删除（树分组已出标题，双标题冗余）。
      样式：.nav-tree 自己滚（flex:1+overflow），无需 .tp-scroll 包裹层。
      ③ **数据事故与恢复**：JEI 曾处于 removed（ListPackMods 的 SQL
      `status<>'removed'` 使 API 层够不到该行，PATCH 也报错）——备份库后
      直接 UPDATE status='installed' 恢复，再 catalog/rebuild（revision
      14846，stale:false）。备份：/tmp/mpack-data/mpackstation.db.bak-before-restore。
      移除发生在两轮验收之间、原因无法追溯；此前多轮验收的「间歇 404」
      实为该 removed 引发的 catalog 409 与其余偶发 404 的混合。
      验收 PASS 16/17（唯一 FAIL 仍为偶发 404，逐工具页轮询抓不到）。
- [x] B5-b 日志窗口改造（大标题 Log + 左任务列表 + 右 Detail）。
      顺手立了通用骨架 `ui/MasterDetail.tsx`（大标题 + 左列表 + 右详情 +
      可拖分隔条 + 选中项消失回落 + 空态/加载态/错误态；受控选中，
      ui 层不碰 URL）与只读详情栅格 `ui/DetailFields.tsx`（字段→值，
      支持等宽/整行/语义色；**刻意只做只读**，可编辑表单不进）。
      `BottomDock` 已改为第一个调用点：详情 = DetailFields（类型/状态/进度/
      起止/耗时/错误）+ 日志流 + 复制日志（剪贴板）。frame.css 的
      `.dock-tasks/.dock-log/.dock-log-head` 已删。验收 **PASS 11/12**
      （唯一 FAIL 是一次偶发 404，复跑不复现，与改动无关）。
      下一个吃这套骨架的：ModeEdit 的文档列表 + payload。
- [x] B5-f 三件小事（2026-10-04 17:3x）：
      ① 本体（Minecraft 原版行）复用 `ModRow`，有了展开箭头，展开后和模组一样
      能按解析出的内容类型下钻；圆点固定蓝色（不是装/停状态灯），右键菜单只有
      「浏览全部物品 / 展开收起」（本体不能停用/移除）。
      ② 模组行的「停用 / ✕」按钮删除，收进右键菜单（菜单项动态显示
      「停用 X / 启用 X」，pending 时禁用；移除沿用原项）。ModGroups 的
      toggleMod/onRemove 两条 prop 链随之删除。
      ③ 「来源」标题行删除后遗留的三个孤儿按钮（分类/解析依赖/搜物品）用
      portal 送到包根目录树的根行（`.nav-acts`，与包名同行）。插槽机制在
      `app/navRootSlot.ts`（Context 传 DOM 节点）：**状态不往上抬**——树不知道
      catOpen/mode，面板用 `createPortal` 把按钮送上去，两边各管各的。
      `.tp-head` 整条删除，`.tp-top` 只承载两个下拉。
      验收 **PASS 13/13**，pageerror 0。中间 vite broker 又 500（老毛病），
      kill 后经 /tmp/detach-run.py 重启恢复。
- [ ] B6 I18N 专项
- [ ] B7 行为优化收尾
- [ ] B8 全量验收
