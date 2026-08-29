# mPackStation 后端架构设计 v7（可靠性与能力契约版，非权威草案）

> 本文件已被 `docs/architecture/backend-architecture-v7.md` 最终合并版取代；如与最终版冲突，以最终版为准。

> 本文是对 v6 的独立迭代草案，不修改、不替代 `backend-architecture.md`。v7 以现有前端页面规格为能力基线，把 v6 尚未展开的内容编辑、任务书、看板读模型、设置/上手、构建/发布契约补齐，并将鉴权与安全合并为一个设计维度。
>
> v7 的部署模型只有本机单用户服务。文档不保留 workspace、tenant、member、owner 等概念；所有业务数据属于当前本机实例。仍然需要启动 token，是为了防御恶意网页和非预期浏览器请求，而不是为了做用户隔离。

## 0. 目标与边界

### 0.1 产品目标

mPackStation 是本机运行的整合包编排工作台。后端把以下链路组织成可恢复、可解释、可复现的流水线：

```text
包生命周期 → 模组检索与选择 → 依赖求解 → 下载与 JAR 索引
→ 内容/任务书编辑 → 交付检查 → 构建产物 → 本地导出或平台发布
```

整合包是唯一业务作用域；`pack_mods` 是包内选择与安装状态的权威来源；SHA-1 对象是可跨包复用的唯一文件来源；前端只消费后端生成的健康信号、冲突结论和任务状态，不自行推断领域规则。

### 0.2 非目标

- 不做分布式、多副本、云端部署或跨实例同步。
- 不做账号注册、用户切换、成员权限矩阵或组织管理。
- 本期不做 WebSocket；前端以条件轮询消费任务状态。
- 不做插件运行时加载。Provider、内容导出器和发布适配器均是编译期注册的接口实现。

### 0.3 设计标准

1. 所有持久化状态都有明确的来源、生命周期和恢复语义。
2. 业务写入、任务状态、文件副作用和动态流之间的先后顺序必须可证明。
3. 对外 API 以页面规格的字段和状态为契约；内部命名差异只允许存在于适配层。
4. 任何新增表、任务类型、事件类型或文件目录，都必须同步不变量、保留策略、GC 责任和测试。

## 1. 依赖方向与运行时装配

```text
cmd/server
  → httpapi
    → service/{pack,mod,resolver,content,quest,build,system}
      → {store, task, provider, blobstore, obs, platform}
task → store
store ↔ database/sql（唯一 SQL 入口）
provider → net/http（唯一外部平台 HTTP 入口）
blobstore → os/io（唯一业务文件入口）
```

### 1.1 包职责

- `cmd/server`：读取配置、创建依赖、注册路由和任务 handler；唯一允许使用 `log.Fatal`。
- `internal/httpapi`：路由、解码、请求大小限制、API schema 校验、错误信封和中间件；禁止 SQL、Provider、文件系统调用。
- `internal/service/*`：领域规则、资源校验、事务编排、任务提交和 outbox 事件；禁止把网络/磁盘 I/O 放进数据库事务。
- `internal/store`：migration、repository、短事务和读模型查询；唯一直接接触 SQL 的包。
- `internal/task`：持久化队列、状态机、lease、fencing、worker 和恢复；不认识任何领域服务。
- `internal/provider/*`：CurseForge、Modrinth 和以后平台的协议适配、限流、熔断、缓存和下载。
- `internal/blobstore`：临时文件、SHA-1 校验、原子提交、共享对象和 GC。
- `internal/obs`：结构化日志、审计、指标和敏感信息脱敏。
- `internal/platform`：时钟、ID、路径策略、OS 单实例锁和密钥保护。

### 1.2 强制守护

CI 必须拒绝以下依赖：

- `httpapi` 导入 `database/sql`、`store`、`task`、`provider`、`blobstore`。
- `store` 导入 `net/http`、`service`、`task`。
- `task` 导入 `net/http`、`service`、`provider`、`blobstore`。
- `provider` 导入 `database/sql`、`store`。
- `blobstore` 导入 `database/sql`、`service`。
- 所有非 `cmd` 包直接使用标准库 `log`。

所有 SQL 关键字在 `httpapi` 为零命中；`tasks` 表写操作只允许出现在 `internal/task`；所有跨领域写必须经过 service 的短事务。

## 2. 单机运行、配置与生命周期

### 2.1 配置

```go
type Config struct {
    ListenAddr string              // 默认 127.0.0.1:18871
    AllowLAN bool                  // 默认 false；开启时所有 API 均需 token
    DataDir string                 // 解析为绝对路径并验证可写
    FrontendOrigin string          // 默认 http://127.0.0.1:5273
    Provider map[string]ProviderConfig
    DownloadConcurrency int        // 文件级，默认 4，上限 16
    RequestRateLimit int           // provider-side-effect GET，默认 30/min
    TaskRecoverInterval time.Duration // 默认 30s
    LogLevel string
}
```

加载顺序：内置默认值 → `data/config.toml` → `MPACK_*` 环境变量 → 命令行 flag。密钥永不进入配置文件。非法路径、非法地址、越界并发或 LAN 缺少 token 策略时拒绝启动。

### 2.2 目录布局

```text
data/
  mpackstation.db
  schema/                    # 可选导出，不作为运行时事实来源
  config.toml
  session-token
  server.lock
  master.key.current
  master.key.<keyId>[.wrapped]
  blobs/sha1/<ab>/<sha1>
  provider-cache/<provider>/
  task-payloads/<taskId>.{json,log}
  artifacts/<packId>/
  locks/<packId>/
  temp/
  logs/
```

库内存储相对 `DataDir` 的路径；所有用户路径都经过 `pathpolicy.Resolve` 和打开句柄后的最终路径复核。

### 2.3 启动和退出顺序

启动：配置 → 目录 → OS 文件锁 → 单一写连接 → migration/checksum → `quick_check` → `foreign_key_check` → 读连接池 → 一致性巡检 → temp 清理 → 密钥轮换续跑 → 任务恢复扫描 → HTTP listen。

退出：停止接收新任务 → 取消 running context → 等待 worker 最多 15 秒 → 关闭数据库 → 释放 OS 锁。未完成任务由 lease 恢复，旧 worker 的写入由 epoch fencing 拒绝。

SQLite 使用 WAL、外键、5 秒 busy timeout；写连接 `MaxOpenConns=1`，读连接独立且 `query_only=1`。数据库事务绝不跨网络或磁盘 I/O。

## 3. 数据模型和一致性

### 3.1 核心实体

| 实体 | 责任 | 生命周期 |
|---|---|---|
| `packs` | 包名称、描述、MC/加载器作用域、归档状态、当前包版本/锁 | 用户显式删除 |
| `pack_versions` | 面向用户的语义版本（如 `1.2.0`）及发布状态 | 随包保留 |
| `pack_locks` | 依赖求解产生的不可变快照 | 随包追加、随包删除 |
| `pack_mods` | 用户选择和传递依赖的包内清单 | 随包级联 |
| `mod_dependencies` | 求解器输入、来源和解释信息 | 随锁快照或包清理 |
| `jar_index` | SHA-1 对象、JAR 元数据和可分析资源索引 | 全局共享、引用归零后 GC |
| `content_items/revisions` | 配方、结构、矿脉草稿/应用版本 | 随包级联，历史可清理 |
| `quest_*` | 任务书章节、节点、边、奖励及版本 | 随包级联 |
| `artifacts/releases` | 构建文件和平台发布记录 | 随包保留，文件按策略清理 |
| `tasks/task_events` | 后台工作状态、阶段和日志引用 | 终态 30 天，幂等键永久 |
| `outbox_events/activities` | 可靠动态投递和看板流 | outbox 30 天、activity 90 天 |

### 3.2 DDL 约束摘要

`packs` 至少包含：`id`、`name UNIQUE`、`description`、`icon_path`、`mc_version`、`loader`、`loader_version`、`status(active|archived)`、`current_pack_version_id`、`current_lock_id`、`created_at`、`updated_at`。

`pack_versions`：`id`、`pack_id FK`、`version`、`state(draft|ready|released)`、`created_at`、`updated_at`，唯一 `(pack_id, version)`。

`pack_locks`：`id`、`pack_id FK`、单调 `version`、`snapshot_path`、`resolved_by`、`created_at`，唯一 `(pack_id, version)`；写入顺序是 INSERT lock → 同事务更新 `packs.current_lock_id`。

`pack_mods`：`source(curseforge|modrinth|local)`、`project_id`、`version_id`、`display_name`、`pinned`、`enabled`、`dep_origin(user|required|optional)`、可空 `sha1`。local 必须没有 project id；同包同源同项目唯一；同包同 SHA-1 的本地添加由 service 拒绝。

`mod_dependencies`：`lock_id`、`from_mod_id`、可空 `to_project_id`、可空 `to_version_id`、`type(required|optional|incompatible|embedded)`、`constraint`、`reason`；用于回答“为什么选中/锁定/冲突”。

内容表：

- `content_items(id, pack_id, type(recipe|structure|ore), key, draft_revision_id, applied_revision_id, status, created_at, updated_at)`，唯一 `(pack_id, type, key)`。
- `content_revisions(id, item_id, revision, document JSON, validation JSON, state(draft|validated|applied|superseded), created_at)`，唯一 `(item_id, revision)`。
- `content_history(id, pack_id, item_id, revision_id, action(save|validate|apply|rollback), created_at)`。

任务书表：

- `quest_books(id, pack_id, draft_revision, applied_revision, updated_at)`。
- `quest_chapters(id, quest_book_id, title, description, color, position)`，同一本任务书内 position 唯一。
- `quest_nodes(id, chapter_id, key, title, description, icon_ref, prerequisite JSON, rewards JSON, mod_refs JSON, position)`，同一本任务书 key 唯一。
- `quest_edges(id, quest_book_id, from_node_id, to_node_id)`，唯一 `(from_node_id,to_node_id)`，禁止自环。
- `quest_revisions(id, quest_book_id, revision, snapshot_path, validation JSON, state, created_at)`。

构建和发布表：

- `delivery_checks(id, pack_id, check_key, severity(error|warning|ok), status, detail JSON, checked_at)`。
- `artifacts(id, pack_id, pack_version_id, kind(local_zip|curseforge_manifest|modrinth_mrpack), path, sha1, size_bytes, source_lock_id, source_content_revision, source_quest_revision, status, created_at)`。
- `releases(id, pack_id, artifact_id, provider, remote_id, status(draft|uploading|processing|published|failed|canceled), request JSON(脱敏), response JSON(脱敏), error_code, created_at, updated_at)`。

### 3.3 一致性不变量

1. `current_lock_id` 和 `current_pack_version_id` 只能指向同一包的记录；切换在一个事务完成。
2. 已归档包拒绝所有写操作，只有显式解归档例外。
3. 所有业务写操作若影响看板，必须同事务写 outbox；outbox 投递失败不得回滚原业务写。
4. `pack_mods.sha1` 的回填与 `jar_index` 登记在同一短事务完成；文件先写 temp，提交后 rename。
5. 锁快照、内容应用版本、任务书应用版本和构建产物来源不可变；修改只能产生新版本。
6. 内容应用前必须 validation 通过；任务书应用前必须通过无环、无孤立阻塞节点、引用完整检查。
7. 交付检查必须引用确定的 lock/content/quest revision；构建期间这些 revision 不能被覆盖。
8. 任务状态只能沿状态机迁移；每次迁移必须带 `lease_epoch` 条件并写 task event。
9. 冲突以 `(pack_id, kind, fingerprint)` 幂等 UPSERT；忽略状态不复活，已解决但再次出现时复活为 pending。
10. 删除包只级联业务记录；共享 blob 进入 grace，复查无引用并超过 7 天才删除。
11. 外部发布记录不能因网络重试创建重复远端发布；以本地 release idempotency key 和 provider 返回的 remote id 双重保护。

## 4. 领域服务

服务统一接收 `context.Context`，由 service 负责输入校验、包状态检查、事务和 outbox；repository 只返回数据，不决定权限或 HTTP 状态。

```go
type LocalPrincipal struct { ID string } // 始终为当前本机主体
type Resource struct { Domain, PackID, Action string }
```

`authorize` 只验证当前启动会话是否能执行该资源操作；不存在跨用户查询、owner 条件或 workspace 条件。所有写请求必须经过 authorize，读请求也经过统一资源检查以防误暴露内部端点。服务导出写方法由 deny 探针逐一覆盖。

服务集合：`packsvc`、`modsvc`、`resolvesvc`、`contentsvc`、`questsvc`、`buildsvc`、`publishsvc`、`systemsvc`。任务 handler 实现在这些 service 中，由 `task.Registry` 注册。

## 5. 前端能力到后端契约

所有列表统一返回：

```json
{"items": [], "next_cursor": null, "total": null}
```

时间对外 ISO-8601，内部 unix ms。错误统一为：

```json
{"error":{"code":"invalid_argument","message":"可读提示","request_id":"...","details":{}}}
```

### 5.1 看板和环境

`GET /api/dashboard` 返回：

```json
{
  "packs": [{
    "id":"p1", "name":"科技魔法", "iconUrl":null,
    "mcVersion":"1.20.1", "loader":"fabric", "packVersion":"1.2.0",
    "modCount":{"total":42,"installed":40,"selected":42},
    "conflicts":{"resolved":3,"pending":0},
    "edits":{"recipes":8,"structures":1,"ores":2,"quests":4},
    "alerts":{"crashes":0,"updatable":2},
    "lastEditedAt":"2026-08-28T10:32:00Z", "createdAt":"2026-08-20T08:00:00Z"
  }],
  "lastEditedPackId":"p1", "todayResolvedCount":3
}
```

该读模型由 `dashboard repository` 一次性聚合生成：包版本来自 `packs/pack_versions`，模组来自 `pack_mods`，健康信号来自 `conflicts`，内容/任务书来自各自应用版本的计数，`updatable` 来自最近一次 provider version check，`crashes` 来自导入的日志诊断结果。缺失诊断数据返回 0 仅在确知没有记录时使用；接口失败由前端显示 `-`。

`GET /api/system/health`：`curseforgeKeyConfigured`、`modrinthReachable`、`curseforgeReachable`、`storageWritable`、`storageFreeBytes`。

`GET /api/system/status`：平台连通性、`cacheSizeBytes`、`storageFreeBytes`、索引对象数、当前运行任务数。

`GET /api/tasks?recent=20` 返回前端任务模型：

```json
{"id":"t1","type":"index-mod","title":"索引 Create 模组数据层","packId":"p1","packName":"科技魔法","status":"running","progress":42,"error":null,"startedAt":"...","finishedAt":null}
```

公开任务类型固定为 `index-mod|build-pack|import-pack|update-preflight|resolve-pack|download-mod|publish-pack|cache-cleanup`；公开状态固定为 `running|success|failed|cancelled|paused`。内部 `queued|leased` 映射为前端不可见的 `running` 或过滤掉。统一使用 `cancelled` 拼写。

`GET /api/activities?limit=10` 返回 `kind(add-mod|resolve|build|alert|edit|import)`、`text`、`packId`、`at`。数据库事件保留领域 kind 和 action，activity projector 负责映射展示 kind，避免前端承担转换。

### 5.2 包生命周期

```text
GET    /api/packs
POST   /api/packs
GET    /api/packs/{packId}
PATCH  /api/packs/{packId}
POST   /api/packs/{packId}/duplicate
DELETE /api/packs/{packId}
POST   /api/packs/import
GET    /api/meta/mc-versions
GET    /api/meta/loaders?mc_version=1.20.1
```

创建字段：`name`（≤50）、`description`（≤200）、`mcVersion`、`loader`、`loaderVersion`、可选 `packVersion`。修改 MC 版本或 loader 必须使当前 lock 失效并提交 `resolve-pack` 预演任务；不能让旧锁继续被构建使用。导入支持 CurseForge manifest、Modrinth `.mrpack` 链接和本地 zip；确认请求返回 `202 {taskId,packId}`，解析完成才将模组清单和版本写入包。

### 5.3 包内模组、依赖和冲突

```text
GET    /api/packs/{packId}/mod-search?query=&provider=&cursor=&limit=
GET    /api/providers/{provider}/projects/{projectId}
GET    /api/providers/{provider}/projects/{projectId}/versions?mc_version=&loader=
GET    /api/packs/{packId}/mods
POST   /api/packs/{packId}/mods
POST   /api/packs/{packId}/mods/local
PATCH  /api/packs/{packId}/mods/{modId}
DELETE /api/packs/{packId}/mods/{modId}
POST   /api/packs/{packId}/resolve
GET    /api/packs/{packId}/lock
GET    /api/packs/{packId}/conflicts
POST   /api/packs/{packId}/conflicts/{conflictId}/resolve
```

添加平台模组先验证项目/版本属于当前 MC/loader 作用域，再写用户选择，随后提交索引和求解任务。求解器输出候选解释、传递依赖、冲突指纹和 lock snapshot。受限下载必须显示 `distribution_restricted` 并提供人工文件上传入口。

### 5.4 内容编辑

内容模型与编辑器解耦：编辑器保存规范化 JSON 文档，导出器把文档转换为目标格式。三种类型共用 revision/validation/apply/history 语义。

```text
GET    /api/packs/{packId}/content?type=
POST   /api/packs/{packId}/content/items
GET    /api/packs/{packId}/content/items/{itemId}
PATCH  /api/packs/{packId}/content/items/{itemId}
DELETE /api/packs/{packId}/content/items/{itemId}
POST   /api/packs/{packId}/content/validate
POST   /api/packs/{packId}/content/apply
GET    /api/packs/{packId}/content/history?item_id=
POST   /api/packs/{packId}/content/rollback
GET    /api/packs/{packId}/content/preview?item_id=
```

配方文档包含输入槽位、输出槽位、数量、namespace、条件和目标导出器；结构文档包含生成器、尺寸、锚点和参数；矿脉文档包含维度、分布、生成高度、数量和权重。validation 返回稳定的 `path/code/severity/message` 数组、受影响模组和冲突指纹。保存草稿不改变 applied revision；应用是单事务，成功后写 outbox 并使构建检查重新变为 stale。撤销只创建一个新的 revision，不删除历史。

### 5.5 任务书

```text
GET    /api/packs/{packId}/quests
POST   /api/packs/{packId}/quests/chapters
PATCH  /api/packs/{packId}/quests/chapters/{chapterId}
DELETE /api/packs/{packId}/quests/chapters/{chapterId}
POST   /api/packs/{packId}/quests/nodes
PATCH  /api/packs/{packId}/quests/nodes/{nodeId}
DELETE /api/packs/{packId}/quests/nodes/{nodeId}
POST   /api/packs/{packId}/quests/edges
DELETE /api/packs/{packId}/quests/edges/{edgeId}
POST   /api/packs/{packId}/quests/validate
POST   /api/packs/{packId}/quests/apply
GET    /api/packs/{packId}/quests/preview
```

节点可引用模组项目、版本、资源键和其他节点；奖励是可校验的结构化 union（物品、经验、命令、解锁）。校验器拒绝自环、循环依赖、孤立节点、跨书引用、缺失奖励和不存在的模组引用；章节颜色只用于分组，不参与业务状态。预览只读，应用产生不可变 quest revision，并参与构建 fingerprint。

### 5.6 构建、导出和发布

```text
GET    /api/packs/{packId}/delivery-checks
POST   /api/packs/{packId}/builds
GET    /api/packs/{packId}/artifacts
POST   /api/packs/{packId}/publish/{provider}
GET    /api/packs/{packId}/releases
```

交付检查必须逐项返回 `key|status(ok|warning|blocked)|severity|message|details`，至少覆盖 lock、依赖、冲突、缺失 JAR、内容 validation、任务书 validation、包版本和目标平台凭据。任何 `blocked` 都禁止 build。

build 请求必须携带 `packVersion` 和客户端幂等键，服务端计算 `build_fingerprint = sha1(lock_id + content_revision_ids + quest_revision + pack_version + exporter_version)`。相同 fingerprint 直接复用已有成功产物；构建任务创建 staging 目录，按稳定排序和固定时间规则写 zip，完成后登记 SHA-1/大小，再原子移动到 artifacts。

发布先创建 release 记录并进入 `publish-pack` 任务；非幂等上传不自动重试，查询状态可以重试。请求和响应严格脱敏；远端 id 写入 release，重启后凭 release 状态继续轮询或标记人工处理。

### 5.7 设置、上手与系统

```text
GET    /api/settings
PATCH  /api/settings
PUT    /api/settings/secrets/{key}
POST   /api/settings/providers/{provider}/test
PUT    /api/settings/export-dirs
DELETE /api/settings/export-dirs/{name}
GET    /api/onboarding
PUT    /api/onboarding
POST   /api/cache/cleanup
```

普通设置使用白名单 key：默认 MC 版本、默认 loader、默认 loader version、下载并发、缓存 TTL、代理配置、界面偏好。凭据只存在 secrets 表，不与普通 settings 混放；API 只返回 configured/masked。上手步骤由事实计算：凭据已配置、至少一个包、至少一个包内模组；`PUT /onboarding` 仅保存 dismissed/提示状态，不允许伪造业务事实。

## 6. 异步任务可靠性

### 6.1 状态机

```text
queued --lease(epoch+1)--> leased --> running --success--> succeeded
   ▲                         │          ├--retry--> queued(run_after)
   │                         │          ├--budget/deadline--> failed
   │                         └----------└--cancel--> cancelled
paused <------------------------------> queued
```

终态不可逆；用户显式 retry 可将 failed/cancelled 重置为 queued 并清理 lease 字段。每次 UPDATE 都带预期 status 和 epoch，0 行即 `task_invalid_transition`。

### 6.2 Lease、fencing、恢复

- lease TTL 30s，心跳 10s，恢复扫描 30s；领取时设置 2h deadline。
- 心跳失败、epoch 不符或 lease 丢失时，handler 立即停止外部副作用，并以 `lease_lost` 退出。
- 所有任务写、进度写、日志追加都必须先在同一短事务校验 owner+epoch。
- 崩溃将 leased/running 重置为 queued 并递增 `recover_count`；连续 10 次恢复失败转 failed。
- 业务失败按 1/2/4/8 秒退避，上限 300 秒；自动重试不经过 failed。
- kill-restart 和过期 worker 写入回滚是集成验收必测项。

### 6.3 幂等和级联

任务 payload 采用 canonical JSON，SHA-1 作为 `payload_hash`。客户端 `Idempotency-Key` 永久保存；同键不同参数返回 409。活跃任务以 `(kind, pack_id, payload_hash)` 唯一索引去重。删除包在同一事务取消该包所有任务并递增 epoch，提交后再取消内存 context。

任务类型至少包括 `resolve-pack`、`download-mod`、`index-mod`、`import-pack`、`update-preflight`、`build-pack`、`publish-pack`、`cache-cleanup`、`key-rotation`。`task` 包不解释 payload；领域 service handler 负责具体工作。

## 7. Provider 适配和可靠性

```go
type Provider interface {
    Name() string
    Search(context.Context, SearchQuery) (SearchPage, error)
    GetProject(context.Context, string) (Project, error)
    BatchGetProjects(context.Context, []string) ([]Project, error)
    ListVersions(context.Context, string, VersionFilter) (VersionPage, error)
    GetVersion(context.Context, string, string) (Version, error)
    ResolveDownload(context.Context, string, string) (DownloadRef, error)
    Download(context.Context, DownloadRef, io.Writer) error
    LookupByFingerprint(context.Context, []FileRef) ([]FingerprintMatch, error)
    ListMCVersions(context.Context) ([]string, error)
    ListLoaders(context.Context, string) ([]string, error)
}
```

适配器输出统一 Project/Version/Dependency/VersionFile/DownloadRef；平台特有字段放在带 provider 前缀的 `Extra` 中。分页游标封装原生 offset/page，携带 filter hash，过滤条件变化返回 invalid_argument。MR 多文件选择 primary；CF `downloadUrl=null` 标记 restricted。

每 provider 独立 token bucket、并发上限、请求 deadline、熔断器和缓存 namespace：429 依据 Retry-After 退避，连续 5 次 5xx/超时打开 30s，half-open 只放一个探针；GET 和明确幂等查询最多重试 3 次，发布上传永不自动重试。搜索 15m、项目 6h、版本列表 1h、版本详情/指纹 24h；过期缓存保留 24h stale 宽限，缓存不可用时返回可行动的 provider_unavailable。

## 8. 鉴权与安全边界（单一维度）

安全目标是保护本机服务免受恶意网页、错误 Origin、路径穿越、危险压缩包、密钥泄漏和同一服务的并发竞态；不承诺防御同机同 OS 用户进程。

### 8.1 浏览器请求

启动生成 128-bit 随机 token，写入 `data/session-token`。所有写请求、任务控制、敏感读请求和 LAN 模式下的全部 API 必须携带 `X-MPack-Token`，服务器常数时间比较。Host 仅允许 localhost/127.0.0.1/[::1]；带 Origin 的写请求必须等于配置的 FrontendOrigin；CORS 只允许该 origin 和必要的自定义头。token 不进日志、payload、activity 或错误详情。

回环下的公开健康探针和明确枚举的 provider-side-effect GET 可以免 token，但受全局 30/min 限速；LAN 开启时不得保留免 token 例外，并显示无 TLS 风险警告。

### 8.2 密钥与日志

KeyProtector 优先 Windows DPAPI、macOS Keychain、Linux 0600 文件降级；AES-256-GCM 密文带 key id 和随机 nonce。密钥轮换分为新 key 写入、逐行重加密、旧 key 删除三步，重启可续跑。日志和任务 payload 通过统一脱敏器过滤原文、URL 编码和 Base64 形态。

### 8.3 路径和导入

拒绝绝对路径、`..`、符号链接逃逸；导出目录登记必须写入 `.mpackstation-export` 标记且每次操作复检。zip/mrpack 单文件 ≤512MiB、解压总量 ≤2GiB、条目 ≤50000、压缩比 ≤100:1，拒绝 symlink/hardlink，边解压边验证最终路径。

### 8.4 磁盘与文件副作用

下载/构建前检查至少两倍预计空间；temp → 短事务登记 → commit → rename。blobstore 内部全局锁串行登记与 GC，防止重下载和回收交错。所有导出、发布 staging 和日志目录都禁止接受任意用户路径。

## 9. 可观测性

JSONL 日志固定字段：`ts level msg request_id task_id pack_id component duration_ms error_code`；请求、任务和 provider 调用共享 request/task context。审计记录 action、target、request_id、detail 和时间，敏感操作审计写失败则整体回滚。

`GET /api/system/metrics` 至少提供：HTTP 按路由请求量/延迟桶，任务队列深度/终态/恢复/fencing/outbox，Provider 成功/失败/限流/熔断/singleflight，blob 上传下载/锁等待，DB 慢查询，GC 删除量，进程 uptime/goroutine，磁盘剩余。指标不含完整 URL、query、token、API key 或用户文档内容。

任务日志按 task id 单独文件并保留最近 200 条阶段事件；`GET /api/tasks/{id}/log` 流式读取且受 token 保护。活动 projector 监控 pending 数、最老事件年龄、投递失败和重复跳过。

## 10. 文件一致性与 GC

启动和 cache-cleanup 执行巡检：blob 有文件无索引则删除；索引有文件缺失则先在事务中清空 pack_mods 引用再删索引，并写 system outbox；孤儿 temp 超 24h 删除；payload 无任务行则删除。

blob 引用归零先写 `blob_grace`，7 天后复查仍无引用才删索引和文件。缓存使用 TTL+24h stale 宽限；任务终态行和 outbox 按保留期删除；`task_idem_keys` 永不删除。包删除目录清理采用清单，不在事务中做递归磁盘删除。

## 11. 测试和契约冻结

### 11.1 单元测试

- migration checksum、quick_check、foreign_key_check 和约束失败回滚。
- repository 查询和 dashboard 聚合的空数据/删除/归档边界。
- 内容 schema、配方/结构/矿脉 validation、revision/apply/rollback。
- 任务书图环检测、孤立节点、引用、奖励 union。
- 状态机全边、lease/epoch fencing、deadline、恢复预算、幂等键。
- Provider 分页、429、熔断、stale cache、受限文件和指纹样例。
- pathpolicy、zip 防护、KeyProtector 轮换、脱敏三形态。

### 11.2 集成和契约测试

使用临时目录和真实 SQLite：kill-restart、旧 worker 写回滚、下载/GC 竞态、包删除级联、outbox 重投、同键并发提交、build fingerprint 复用、发布恢复。所有前端 API 以 JSON fixture 冻结请求/响应；前端 zod 与后端契约测试共用字段定义。

验收门槛：`gofmt`、`go vet`、Go tests、前端 `tsc`/eslint、depguard、迁移链、契约 fixture、关键集成测试全部通过。

## 12. 文档和代码变更纪律

- SQL 只能通过编号 migration；已应用 migration 禁止修改，checksum 失败必须停止启动。
- `backend-architecture.md`、本文件、页面规格、API fixture 和受保护类型改动必须同时更新相关 decision 记录。
- `store` 是唯一 SQL 入口；provider 和 blobstore 是唯一各自 I/O 入口；HTTP handler 不得绕过 service。
- 新任务/事件/错误码必须同步状态机、DDL CHECK、API 映射、指标和测试。
- 前端 mock 与真实 API 共用同一 schema；组件不得直接 `fetch` 或解释内部状态。
- 新依赖必须说明用途、许可证、升级策略和构建影响，并更新 lockfile。

## 13. 维度自评（按评审图，鉴权与安全合并）

| 维度 | 评分 | 判断 |
|---|---:|---|
| 耦合性 | 8.5/10 | 进程装配、HTTP、service、store、task、provider、blobstore 边界明确；任务 handler 通过 registry 注入，仍需依靠 CI 守护避免回流。 |
| 可维护性 | 8.5/10 | 领域服务、稳定错误码、migration、契约 fixture、统一 revision 和测试策略完整；文档规模较大，需要严格保持单一权威来源。 |
| 可扩展性 | 8.5/10 | Provider、内容导出器、发布适配器和任务 handler 均为接口；新增领域表/事件有同步义务，不需要改核心队列。 |
| 健壮性 | 9/10 | 超时、优雅退出、lease、deadline、恢复、fencing、磁盘预检、导入边界和 GC 闭环均有落点；外部平台不可控部分仍需人工失败处理。 |
| 数据一致性 | 9/10 | 不变量、短事务、outbox、不可变 revision/lock、文件两阶段提交、epoch 条件和引用宽限均有明确顺序；跨进程文件系统无法获得真正原子事务，靠巡检补偿。 |
| 异步任务能力 | 9/10 | 状态机、幂等、配额、重试、暂停/取消、恢复、日志 fencing、级联取消全部定义；SSE 不在本期范围，但轮询契约明确。 |
| Provider 扩展能力 | 8.5/10 | 统一接口、缓存、限流、熔断、重试、受限下载、分页游标和 fixture checklist 齐全；不同平台发布语义仍需各自适配。 |
| 可观测性 | 8.5/10 | request/task/pack 关联日志、审计、指标、任务日志和 outbox 投递指标完整；尚未规定告警阈值和指标持久化后端。 |
| 鉴权与安全 | 8.5/10 | 单机模型无多余身份层，token/Host/Origin、密钥、路径、归档导入、LAN、限速和敏感操作统一覆盖；不防同机高权限进程，且 Linux 密钥保护是降级方案。 |

**综合评价：8.7/10。** v7 的主要提升不是堆接口，而是把前端已承诺的能力都绑定到可持久化模型、不可变版本、任务状态机和可验证的安全边界上；真正达到该评分仍以契约测试、kill-restart、文件竞态和真实 provider fixture 全部通过为准。

## 14. v7 决策摘要

1. 单机单用户是唯一部署模型，移除所有 workspace/tenant/owner/member 设计。
2. 启动 token 是浏览器攻击面防护，不承担业务多用户身份。
3. `packVersion`、`lockVersion`、Minecraft 版本、loader 版本和构建 fingerprint 是五个不同概念，分别落库。
4. 前端任务和 activity 使用稳定展示契约，领域事件到展示事件由 projector 映射。
5. 内容和任务书采用草稿/应用/历史 revision；构建只接受确定 revision。
6. 构建和发布是可恢复任务，产物和 release 记录永远指向其输入快照。
7. “鉴权与安全”作为同一维度评审，安全策略按攻击面、文件副作用和凭据保护统一设计。
