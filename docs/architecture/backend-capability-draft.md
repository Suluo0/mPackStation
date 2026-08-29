# mPackStation 后端能力初版构思

## 1. 后端定位

mPackStation 后端应是运行在本机的“整合包编排服务”，而不是单纯的数据 CRUD 服务。它负责把平台元数据、包内选择、依赖解析、本地文件、编辑草稿和最终产物组织成一条可恢复、可观察的流水线。

边界原则：

- 整合包是唯一业务作用域，业务接口优先以 `/api/packs/:packId/...` 组织。
- `pack_mods` 是包内模组选择与安装状态的唯一权威来源。
- JAR 文件与解析结果按 SHA-1 全局共享，避免跨包重复下载和解析。
- CurseForge、Modrinth 和 JAR 原始元数据落文件，SQLite 只保存结构化字段与文件路径。
- 搜索、读取、表单保存可以同步完成；导入、下载、索引、重解、构建、发布必须进入后台任务系统。
- 后端负责产生“已解决 / 待解决”的产品信号，前端不自行推断依赖与冲突。

## 2. 能力域

### 2.1 本地运行与基础设施

- 配置加载：数据目录、缓存目录、下载并发、代理、平台凭据。
- SQLite schema migration，而不是只在启动时重复执行一份初始 schema。
- 统一 JSON 错误格式、请求 ID、结构化日志和版本信息。
- 路径安全：所有写入必须限制在配置的数据目录和用户明确选择的导入/导出目录内。
- 单实例锁，避免两个服务同时写同一个 SQLite 数据库。
- 健康检查：数据库、目录可写性、剩余空间、平台连通性和凭据状态。

### 2.2 整合包生命周期

- 创建、读取、修改、复制、归档和删除整合包。
- 包版本、Minecraft 版本、加载器与加载器版本管理。
- 版本或加载器变化时触发全包重新求解，而不是直接修改字段后继续使用旧锁定结果。
- 导入 CurseForge manifest、Modrinth `.mrpack` 和本地 zip。
- 包摘要聚合：模组数、冲突数、内容编辑量、告警和最后编辑时间。

核心接口初稿：

```text
GET    /api/dashboard
GET    /api/packs
POST   /api/packs
GET    /api/packs/:packId
PATCH  /api/packs/:packId
POST   /api/packs/:packId/duplicate
DELETE /api/packs/:packId
POST   /api/packs/import
```

### 2.3 双平台检索与元数据适配

- CurseForge / Modrinth 搜索统一模型：项目、版本、加载器、类别、作者、图标、下载量、更新时间。
- 平台项目与版本详情适配，保留平台特有字段但不泄漏到页面组件。
- 限流、超时、重试、TTL 缓存和离线回退。
- CurseForge Key 安全存储；日志与 API 响应不能返回完整密钥。
- 搜索必须带当前包的 MC 版本和加载器作用域，避免返回不可加入的结果。

核心接口初稿：

```text
GET /api/packs/:packId/mod-search
GET /api/providers/:provider/projects/:projectId
GET /api/providers/:provider/projects/:projectId/versions
GET /api/meta/mc-versions
GET /api/meta/loaders
```

### 2.4 包内模组、依赖锁定与冲突求解

- 加入、移除、启用/停用、设为可选模组。
- 版本候选筛选：MC 版本、加载器、平台版本、发布日期和用户固定版本。
- 依赖图：required / optional / incompatible / embedded。
- 自动锁定传递依赖，检测循环、缺失版本、跨加载器、重复 mod ID 和文件冲突。
- 求解结果必须可解释：系统选择了什么版本、为何选择、哪些问题已自动解决、哪些必须由用户决定。
- 保存锁定快照，使打包结果可复现。

核心接口初稿：

```text
GET    /api/packs/:packId/mods
POST   /api/packs/:packId/mods
PATCH  /api/packs/:packId/mods/:modId
DELETE /api/packs/:packId/mods/:modId
POST   /api/packs/:packId/resolve
GET    /api/packs/:packId/lock
GET    /api/packs/:packId/conflicts
POST   /api/packs/:packId/conflicts/:conflictId/resolve
```

### 2.5 下载、缓存与 JAR 索引

- 流式下载到临时文件，完成后校验 SHA-1/大小，再原子移动到对象存储目录。
- 下载去重、失败重试、断点策略和并发控制。
- 解析 `mods.toml`、`fabric.mod.json` 等元数据并建立共享 JAR 索引。
- 读取配方、数据包、结构、标签、Mixin 和其他可分析资源，为冲突检测与内容编辑提供数据层。
- 缓存引用计数和安全清理：删除包不能直接删除其他包仍在使用的 JAR。

建议目录：

```text
data/
  blobs/sha1/...
  provider-cache/...
  task-payloads/...
  artifacts/...
  temp/...
```

### 2.6 内容编辑

- 配方、结构和矿脉规则的草稿 CRUD。
- 草稿与“已应用到包”版本分离，支持保存、校验、应用和撤销。
- 配方输入输出引用验证、命名空间检查、重复资源位置检查。
- 结构与矿脉参数校验、预览所需的规范化数据输出。
- 导出为数据包/KubeJS/CraftTweaker 等目标格式时使用适配器，不让编辑模型绑定某一种输出格式。

核心接口初稿：

```text
GET  /api/packs/:packId/content
POST /api/packs/:packId/content/items
PATCH /api/packs/:packId/content/items/:itemId
POST /api/packs/:packId/content/validate
POST /api/packs/:packId/content/apply
GET  /api/packs/:packId/content/history
```

### 2.7 任务书编辑

- 章节、节点、边和排序的 CRUD。
- 节点前置条件、奖励、关联模组与资源引用。
- 图结构校验：循环依赖、孤立节点、缺失奖励和跨章节引用。
- 预览模型与最终导出适配器分离，未来可支持 FTB Quests 等不同目标。

核心接口初稿：

```text
GET    /api/packs/:packId/quests
POST   /api/packs/:packId/quest-chapters
PATCH  /api/packs/:packId/quest-chapters/:chapterId
POST   /api/packs/:packId/quest-nodes
PATCH  /api/packs/:packId/quest-nodes/:nodeId
POST   /api/packs/:packId/quests/validate
GET    /api/packs/:packId/quests/preview
```

### 2.8 构建、导出与发布

- 交付前检查：锁文件、依赖、冲突、缺失文件、内容校验、任务书校验和版本号。
- 可复现构建：相同锁文件和编辑版本生成相同逻辑内容。
- 输出本地 zip、CurseForge manifest 和 Modrinth `.mrpack`。
- 产物登记：文件、大小、校验值、创建时间、构建来源和状态。
- 发布前预演；平台上传、轮询发布状态和失败重试放入后台任务。

核心接口初稿：

```text
GET  /api/packs/:packId/delivery-checks
POST /api/packs/:packId/builds
GET  /api/packs/:packId/artifacts
POST /api/packs/:packId/publish/:provider
GET  /api/packs/:packId/releases
```

### 2.9 后台任务与活动流

- 持久化队列：queued / running / paused / success / failed / cancelled。
- 服务重启后将未完成任务恢复为可重试状态，不能永远停在 running。
- 支持取消、暂停、重试、进度、阶段、错误详情和日志文件。
- 初版前端沿用 3 秒条件轮询；后续可增加 SSE 推送，不急于引入 WebSocket。
- 业务成功或失败产生结构化 activity，供看板展示。

核心接口初稿：

```text
GET  /api/tasks?recent=20
GET  /api/tasks/:taskId
POST /api/tasks/:taskId/pause
POST /api/tasks/:taskId/resume
POST /api/tasks/:taskId/cancel
POST /api/tasks/:taskId/retry
GET  /api/tasks/:taskId/log
GET  /api/activities?limit=10
```

### 2.10 设置、上手状态与系统状态

- 平台连接、代理、存储目录、缓存策略和默认包配置。
- 敏感字段与普通设置分离；API 只返回 `configured: true/false`。
- 上手三步由真实业务状态计算为主，必要时保存一次性完成提示状态，而不是允许前端任意勾选。
- 存储统计、缓存占用、索引状态和平台连通性。

核心接口初稿：

```text
GET   /api/settings
PATCH /api/settings
POST  /api/settings/providers/:provider/test
GET   /api/system/health
GET   /api/system/status
GET   /api/onboarding
PUT   /api/onboarding
POST  /api/cache/cleanup
```

## 3. 当前 schema 的主要缺口

现有 8 张表可支撑看板空壳，但不足以支撑完整设计。建议后续 migration 增加：

- `schema_migrations`：数据库版本管理。
- `pack_versions` / `pack_locks`：包版本和可复现锁定快照。
- `mod_dependencies`：规范化依赖图与求解来源。
- `content_items` / `content_revisions`：内容草稿、应用版本和历史。
- `quest_chapters` / `quest_nodes` / `quest_edges`：任务书图模型。
- `artifacts` / `releases`：构建产物和平台发布记录。
- `task_events`：任务阶段、日志索引与恢复信息。
- `provider_accounts` 或安全凭据存储层：避免 API Key 与普通设置混放。

`packs` 还需要 `pack_version`、归档状态和可选的当前锁版本；`pack_mods` 需要用户固定版本、依赖来源、启用状态和锁定原因；`tasks.kind` 应与前端任务类型统一。

## 4. 服务内部建议分层

```text
cmd/server                 进程启动与依赖装配
internal/httpapi           路由、请求校验、统一错误
internal/store             migration 与 repository
internal/pack              包生命周期
internal/provider          CurseForge / Modrinth 适配器
internal/resolver          版本与依赖求解
internal/blobstore         下载、校验、共享文件存储
internal/indexer           JAR 与数据层索引
internal/content           配方/结构/矿脉草稿与校验
internal/quest             任务图与校验
internal/build             构建、产物和发布适配器
internal/task              持久化任务队列与 worker
internal/system            设置、健康、存储和 onboarding
```

HTTP handler 不直接写 SQL，也不直接调用平台 SDK；业务 service 负责事务与领域规则，repository 只负责持久化。

## 5. 建议落地顺序

### M0：后端地基

- [部分已落地] 配置目录绝对化、HTTP 读写/空闲超时、8 MiB 请求体上限、panic 恢复、SIGINT/SIGTERM 优雅退出。
- [部分已落地] `/api/healthz` 存活探针、`/api/readyz` 就绪探针；数据库失败时就绪返回 503；保留 `/api/health` 兼容入口。
- [部分已落地] SQLite 外键/WAL/busy timeout 连接参数、事务包裹的 v1 基线和唯一 `schema_migrations` 版本记录；后续仍需增加真正的编号迁移脚本与版本校验测试。
- [未完成] 统一错误信封（当前仅有错误骨架）、request context/request-id 贯穿日志、路由拆分、目录权限与单实例锁。

### M1：先替换看板 mock

- 包 CRUD、dashboard 聚合、tasks、activities、system health/status、onboarding、MC 版本。
- 前端仍保持现有适配层，只将 `USE_MOCK` 切换为配置项。

### M2：包工作台主闭环

- 双平台搜索、加入包、版本候选、依赖求解、下载与 JAR 索引。
- 先完成“新建包 → 搜索 → 加入 → 自动锁依赖 → 可看到健康结果”。

### M3：内容与任务书

- 草稿/版本模型、校验器、预览模型和导出适配器。

### M4：构建与发布

- 交付检查、本地构建、产物管理，再接 CurseForge / Modrinth 发布。

## 6. 初版验收线

第一阶段后端不是“接口能返回 JSON”就算完成，至少应满足：

- 服务重启后包、任务结果和上手状态不丢失。
- 所有包域数据都校验 `packId`，删除包能级联清理业务记录但不误删共享 JAR。
- 平台失败、磁盘不足、校验失败都返回稳定错误码和可执行提示。
- 异步任务可取消、失败可重试、服务重启后不遗留假 running。
- 导入 zip 有大小、路径穿越和压缩炸弹防护。
- 平台密钥不出现在日志、任务 payload 或普通设置响应中。
- build、Go 单元测试和关键 repository 集成测试通过。
