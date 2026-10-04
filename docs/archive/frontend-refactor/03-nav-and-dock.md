# 03 · 左栏三目录 + 版本修订 + 日志窗口

> 目标来自用户拍板（2026-10-04）：左栏 = **模组管理 / 任务书管理 / 自建内容管理**；
> 找回「包版本修订」入口；底部日志窗口改「大标题 + 左列表右 Detail」。

## 1. 现状（读代码得到，不是推测）

| 位置 | 现在是什么 |
|---|---|
| `app/IconRail.tsx` | 窄图标轨，5 个 tool：来源 / 章节 / 商店 / 构建 / 运行（`TOOLS` 在 `app/url.ts`） |
| `panels/ToolPanel.tsx` | 左宽面板（320px），按 `?tool=` 渲染 SourcesPanel / QuestPanel / StorePanel / BuildPanel / RunPanel |
| `dock/BottomDock.tsx` | 左任务列表 + 右日志；头部只有一行小字「日志」，没有大标题；右侧只有日志行，没有 Detail |
| `api/releases.ts` | **版本修订的 API 早就齐了**：`listVersions` / `createVersion` / `listArtifacts` / `buildPack` / `publishPack`。界面入口在迭代中丢了，不是后端缺能力 |

## 2. 硬约束：自建内容只有 3 种 kind

`apps/server/internal/service/content.go:293`

```go
func validContentKind(k string) bool { return k == "recipe" || k == "structure" || k == "ore" }
```

后端只认 **配方 / 结构 / 矿脉** 三种。`POST /content` 传别的 kind 直接 422。

用户列的二级目录是「配方 / 物品结构 / 多方块结构 / 地形群系 / 生物资源群系变更」——
**后四项后端没有对应的 kind**，也不是前端能补出来的：每加一种要在后端写
`canonicalContentPayload` 的字段白名单 + `validateContentSemantics` 的必填校验 + payload schema，
否则存进去的文档过不了校验、也解释不了 payload。

**本轮落地：配方 / 结构 / 矿脉**（按后端枚举）。其余四项列为后端待办，前端预留分组位。

## 3. 目标结构

```
┌ 左栏（现 ToolPanel 位置，目录树常驻在最上）────────┐
│ ▾ 模组管理                                        │
│     · 已装模组（分类树）      → tool=sources       │
│     · 添加模组（商店）        → tool=store         │
│ ▾ 任务书管理                                      │
│     · 章节列表                → tool=quest        │
│ ▾ 自建内容管理                                    │
│     · 配方   recipe           → tool=content      │
│     · 结构   structure        → tool=content      │
│     · 矿脉   ore              → tool=content      │
│ ─────────────────────────────────────────────     │
│ ▾ 版本修订（找回的入口）                           │
│     · 包版本列表 + 新建版本   → tool=releases      │
│     · 构建 / 发布             → tool=build        │
│     · 运行                    → tool=run          │
└───────────────────────────────────────────────────┘
   下面是当前 tool 自己的内容面板（不变）
```

- 目录树是**常驻导航**，点节点只切 `?tool=`（+ 内容面板切 `?kind=`），不动编辑区。
- 交付动作（构建 / 发布 / 运行）从顶层图标轨收进「版本修订」——它们是「某个版本要做什么」，
  和「管理什么内容」不是一层。
- `IconRail` 保留（顶栏模式切换与它无关），但构建 / 运行不再占顶层轨道。

## 4. 日志窗口（BottomDock）

```
Log · <任务名>                                  ← 大标题（一级字重，不再是 12px 小字）
┌ 左：任务列表 ──────┬ 右：Detail ─────────────┐
│ ● 解析依赖   进行中 │ 类型 / 状态 / 进度 / 时间  │
│ ● 构建       成功   │ ─────────────────────    │
│ ○ 目录构建   失败   │ 日志行（NDJSON 流）        │
└────────────────────┴──────────────────────────┘
```

- 标题：无选中任务 = `Log`；选中构建类任务 = `Build Log`（按 `task.type` 判）。
- 右侧 Detail 上半是任务字段（现在完全没有，只有日志行），下半是日志流。
- 数据：任务字段来自 `PackSummaryContext.tasks`（已有）；日志仍是 `fetchTaskLog` 3s 轮询。

## 5. 实施步骤

| # | 动作 | 文件 |
|---|---|---|
| 1 | `TOOLS` 增 `content` / `releases`；`?kind=` 参数收口 | `app/url.ts` |
| 2 | 新建 `app/NavTree.tsx`：目录树（唯一实现，分组配置化），渲染在 ToolPanel 顶部 | 新增 |
| 3 | `ToolPanel` 挂 `NavTree`，并把构建 / 运行从 `IconRail` 摘掉 | `panels/ToolPanel.tsx`、`app/IconRail.tsx` |
| 4 | 新建 `panels/ContentPanel.tsx`：按 kind 列 `listContent`，新建走 `Prompt`，编辑走现有 revision 流程 | 新增 |
| 5 | 新建 `panels/ReleasesPanel.tsx`：`listVersions` / `createVersion`（版本 + channel + changelog），下挂构建 / 发布 / 运行入口 | 新增 |
| 6 | `BottomDock` 改大标题 + 右侧 Detail 区 | `dock/BottomDock.tsx` |
| 7 | 样式：目录树 `.nav-tree*`、Detail `.dock-detail*` 进 `ui/ui.css` 或 `panels.css`（面板专属留面板） | CSS |
| 8 | 验收：真浏览器点每个目录节点 → URL 与右面板都对；版本列表能列、能新建；日志窗口标题与 Detail 正确 | CDP |

## 6. 顺序上的依赖

- 第 2 步的 `NavTree` 依赖 `ui/` 已有组件（Icon / Button），**已就绪**（B3-d 已完成）。
- 第 4 步依赖后端 kind 枚举 —— 只有 3 种，见 §2。
- I18N（B6）在目录结构定稿之后做，否则文案键要跟着树改两遍。

## 7. 待用户确认 / 后端待办

- 自建内容要「物品结构 / 多方块结构 / 地形群系 / 生物资源群系变更」→ **后端先扩 kind 枚举与校验**，前端再加树节点。
- 「版本修订」里是否要把「发布到 CurseForge / Modrinth」也放进来（现有 `publishPack` 有，但无凭证时后端 by-design 拒绝）——本轮先列出来，不做表单。
