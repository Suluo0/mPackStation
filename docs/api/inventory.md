# 接口现状与复用清单

> 收尾阶段的能力闭合盘点。用于人机分工：先看清楚有什么、哪些能直接拿来用，再决定补什么。
>
> - **清单一**：现状与可复用性（本文件上半部分）
> - **清单二**：接口文档细化项（待补，附在下方）
>
> 全部闭合后本文件可删除。

---

# 清单一：接口现状与可复用性

## 1.1 盘点口径

| 指标 | 含义 |
|---|---|
| 后端已实现 | `httpapi.go` 中注册的路由 |
| 前端已覆盖 | `features/*/api.ts` 中出现的路径 |
| 可直接复用 | 后端已实现且契约明确，前端补个函数 + 接 UI 即可，无需改后端 |
| 需前端补函数 | 后端有，前端 api 层还没有对应函数 |
| 需后端新建 | 后端根本没有，或返回 501 等于没有 |

## 1.2 总量

| 项 | 数量 |
|---|---:|
| 后端已实现路由 | 74 |
| 前端 api 层已覆盖路径 | 44 |
| 组件实际调用的函数 | 41 |
| 前端有函数但无 UI 调用 | 13 |
| 后端有、前端连函数都没有 | 19（其中 1 条返回 501） |

## 1.3 分域现状

| 域 | 后端 | 说明 |
|---|---:|---|
| 探针 | 3 | `health` / `healthz` / `readyz`，前端不调 |
| 看板与系统 | 8 | 全部已接，且已端到端实测 |
| 工具（Prism） | 2 | 已接 |
| 整合包 | 8 | CRUD 已接；archive / unarchive / duplicate 未接 |
| 导入 | 2 | 已接，两阶段流程 |
| 模组 | 7 | 已接 6 条；`mods/local` 未接 |
| 依赖与冲突 | 5 | 列表已接；单条 resolve / ignore 未接 |
| 交付检查 | 2 | 已接 |
| 内容编辑 | 8 | 已接 5 条；新建、`history` 未接 |
| 任务书 | 7 | 已接 3 条；`history` / `preview` 未接 |
| 版本与构建 | 4 | `versions` 查询、`build` 已接；新建版本、按版本构建未接 |
| 产物 | 2 | 列表已接；下载未接 |
| 发布 | 8 | 已接 0 条（函数有定义但无 UI）；另有 2 条异步发布 |
| 任务 | 7 | 列表与控制已接；详情、日志未接 |
| 导出目录 | 1 | 未接 |

## 1.4 接口复用情况

### 1.4.1 跨组件直接复用

同一个 api 函数被多个组件直接引用：

| 接口 | 函数 | 调用组件 | 处数 |
|---|---|---|---:|
| `GET /api/packs` | `listPacks` | `AppShell`（侧栏定位第一个真实包）、`PacksPage`（包列表） | 2 |
| `GET /api/packs/{packId}` | `getPack` | `PackWorkbenchPage`、`PackModsPage` | 2 |
| `GET /api/packs/{packId}/mods` | `listMods` | `PackWorkbenchPage`、`PackModsPage` | 2 |

### 1.4.2 通过 props 回调的间接复用

父组件持有刷新函数，以 prop 形式传给子组件触发——这类复用在代码里看函数名是看不出来的：

| 接口 | 定义处 | 间接触发方 | 场景 |
|---|---|---|---|
| `GET /api/onboarding` | `AppShell.refreshOnboarding` | `OnboardingChecklist`（`onRefresh`） | 挂载时加载；唤起 Prism 登录后每 5 秒轮询 |
| `GET /api/system/health` | `DashboardPage.loadHealth` | `EnvHealthBanner`（`onRetry`） | 初始加载；点「重试」 |
| `GET /api/tasks` | `DashboardPage.refreshTasks` | `TaskPanel`（`onChanged`） | 暂停/取消/重试后刷新 |

### 1.4.3 组件内多次调用

| 接口 | 组件 | 调用点 | 次数 |
|---|---|---|---:|
| `GET /api/tasks` | `DashboardPage` | ① 初始加载 ② 3 秒轮询 ③ 导入完成后 ④ 任务操作完成后 | 4 |
| `DELETE /api/packs/{id}` | `DashboardPage` | 删除后刷新列表 | 1 |

### 1.4.4 重复定义（应合并）

同一件事在两处各写了一份，是真正的浪费，也是漂移隐患：

| 对象 | 定义位置 | 说明 |
|---|---|---|
| `deletePack` | `dashboard/api.ts:78`、`pack/api.ts:24` | 两份实现，指向同一接口 `DELETE /api/packs/{id}` |
| `packSchema` | `dashboard/types.ts`、`pack/api.ts:5` | 两份 Pack 契约，**校验强度不同**：前者 `loader` 用封闭枚举 `loaderEnum`，后者用 `z.string()` 不做校验 |
| `page()` 分页包装器 | `pack/api.ts:4`、`release/api.ts:6` | 同一份辅助函数各写一遍，均未导出 |

`packSchema` 那份差异值得单独说：看板侧会校验 loader 取值，包工作台侧不校验。后端若返回了非法的 loader，两个页面表现会不一致——一个报错，一个静默接受。

## 1.5 可复用性分档

**B 类（最省力）**：前端函数已写好，只差 UI 调用 —— 13 项
**A 类（中等）**：后端已实现，前端补函数 + 接 UI —— 18 项
**C 类（需后端动）**：后端没有或返回 501 —— 3 项

**结论：绝大多数缺口靠前端就能闭合，真正需要后端新写的只有 3 项。**

## 1.6 逐项对照

### A 类：后端已实现，前端需补函数 + UI（18 项）

| 能力 | 可复用接口 | 备注 |
|---|---|---|
| 打开单个内容文档 | `GET /api/packs/{id}/content/{docId}` | 契约已定义 |
| 新建内容文档 | `POST /api/packs/{id}/content` | 契约已定义 |
| 内容修订历史 | `GET /api/packs/{id}/content/{docId}/history` | 契约已定义 |
| 任务书预览数据 | `GET /api/packs/{id}/quests/preview` | 出参结构待细化 |
| 新建包版本 | `POST /api/packs/{id}/versions` | 契约已定义 |
| 按版本构建 | `POST /api/packs/{id}/versions/{versionId}/build` | 与 `/build` 有重叠，需确认取舍 |
| 产物下载 | `GET /api/packs/{id}/artifacts/{artifactId}/download` | 二进制流 |
| 添加本地 jar | `POST /api/packs/{id}/mods/local` | 契约待细化 |
| 单条冲突标记解决 | `POST /api/packs/{id}/conflicts/{conflictId}/resolve` | 契约已定义 |
| 单条冲突忽略 | `POST /api/packs/{id}/conflicts/{conflictId}/ignore` | 契约已定义 |
| 归档包 | `POST /api/packs/{id}/archive` | 契约待细化 |
| 取消归档 | `POST /api/packs/{id}/unarchive` | 契约待细化 |
| 任务详情 | `GET /api/tasks/{taskId}` | 契约已定义 |
| 任务日志 | `GET /api/tasks/{taskId}/log` | 出参结构待细化 |
| 登记导出目录 | `POST /api/export-dirs` | 契约已定义 |
| 发布详情 | `GET /api/releases/{releaseId}` | 契约已定义 |
| 创建发布记录 | `POST /api/releases` | 契约待细化 |
| 异步发布 | `POST /api/packs/{id}/publish/{provider}/async`、`POST /api/releases/async` | 契约待细化 |

### B 类：前端函数已有，只差 UI 接线（13 项）

| 能力 | 函数 | 接口 |
|---|---|---|
| 迎新步骤打勾 | `acknowledgeOnboarding` | `PUT /api/onboarding` |
| 更新包信息 | `updatePack` | `PATCH /api/packs/{id}` |
| 包健康摘要 | `packHealth` | `GET /api/packs/{id}/health` |
| 单平台模组搜索 | `searchMods` | `GET /api/packs/{id}/mod-search` |
| 内容文档详情 | `getContent` | `GET .../content/{docId}` |
| 内容存草稿 | `saveContentDraft` | `PUT .../content/{docId}/draft` |
| 内容回滚 | `rollbackContent` | `POST .../content/{docId}/rollback` |
| 任务书存草稿 | `saveQuestDraft` | `PUT .../quests/draft` |
| 任务书回滚 | `rollbackQuest` | `POST .../quests/rollback` |
| 任务书历史 | `questHistory` | `GET .../quests/history` |
| 发布记录列表 | `listReleases` | `GET .../releases` |
| 发布到平台 | `publishPack` | `POST .../publish/{provider}` |
| 轮询发布状态 | `pollRelease` | `POST /api/releases/{id}/poll` |
| 重试发布 | `retryRelease` | `POST /api/releases/{id}/retry` |

### C 类：需后端新建（3 项）

| 能力 | 现状 | 选项 |
|---|---|---|
| 清理缓存 | 无任何接口 | 新增 `POST /api/system/cache/purge`，或撤掉设置页这个按钮 |
| 默认包配置 | 无任何 `/api/settings` | 新增 settings 读写接口，或撤掉设置页这块 UI |
| 复制整合包 | `POST /api/packs/{id}/duplicate` 返回 **501** | 实现，或从设置/菜单中移除该入口 |

### 另需处理的契约缺陷（不新增能力，只是纠偏）

| 编号 | 缺口 | 属 A/B/C |
|---|---|---|
| T-1 | 任务 DTO 三套并存，未归一 | 后端（影响 A 类任务详情、日志） |
| T-2 | 导入响应内嵌 `task` 对象（Go 字段名） | 后端 |
| S-1 | 写令牌硬编码 `test` | 后端 |
| S-2 | 乐观锁返回 409，应为 412 | 后端 |
| S-3 | 错误码过粗 | 后端（工作量取决于粒度决策） |
| S-4 | 预览过期与已消费同为 409 | 后端 |
| S-5 | 导入来源域名校验应为 422 | 后端 |
| S-6 | 幂等键已消费应返回原结果 | 后端 |
| S-7 | `duplicate` 返回 501 | 后端 |
| S-8 | `total` 字段时有时无 | 后端 |
| S-9 | 任务 `type` 未映射时输出空串 | 后端 |
| C-1 | 前端 `z.unknown()` 用于任务控制 | 前端（依赖 T-1） |
| C-2 | `del()` 未复用 `writeHeaders()` | 前端 |
| C-3 | Prism 轮询无退出条件 | 前端 |
| C-4 | 时间字段只校验 `z.string()` | 前端 |
| C-5 | 未处理 `revision_conflict` | 前端 |
| C-6 | 设置页零接口调用 | 前端（依赖 C 类决策） |

## 1.7 建议的执行顺序

1. **先合并重复定义** —— `deletePack` 两份、`packSchema` 两份（校验强度还不一样）、`page()` 两份。这是纯收益，半小时能清完，且能消掉一处真实的不一致
2. **再定 C 类那 3 项怎么处理** —— 产品决策，定不下来后面没法排期
3. **清 B 类 13 项** —— 纯前端接线，风险最低
4. **补 A 类 18 项** —— 按契约写函数
5. **契约缺陷按依赖排** —— T-1（任务 DTO 归一）优先，它卡住 C-1 和 A 类的任务详情/日志；S 类后端项可并行

---

# 清单二：接口文档细化项

待补。将逐条列出需要补充或细化的文档内容，作为 filling 契约文档的执行清单。
