# mPackStation 后端架构设计 v7（工程化与前端能力对齐版，非权威草案）

> 本文件已被 `docs/architecture/backend-architecture-v7.md` 最终合并版取代；如与最终版冲突，以最终版为准。

> 本文是 v7 草案，不修改 `docs/architecture/backend-architecture.md`。它将 v6 的运行时、数据、任务、Provider、文件与安全约束，补成可直接开工的工程设计，并把前端页面规格中的全部业务能力纳入同一套后端契约。
>
> v7 的部署目标仍是本机单用户服务。按要求删除所有多租户、workspace、owner/member 设计；“鉴权”和“安全边界”合并为一个维度与一个章节。本文中的接口、表、状态、脚本和验收门槛是实现时的约束，不是建议清单。

## 0. v7 目标与边界

### 0.1 目标

后端必须覆盖以下前端能力闭环：

```text
看板/迎新
  → 创建、导入、复制、归档、删除整合包
  → 包内模组搜索、筛选、加入、移除、启停、版本选择
  → 依赖求解、冲突处理、锁定快照
  → 配方/结构/矿脉草稿、校验、应用、历史、撤销
  → 任务书章节/节点/边/奖励、校验、预览数据
  → 交付检查、可复现构建、本地导出、平台发布
  → 设置、密钥、缓存、导出目录、环境自检
```

### 0.2 非目标

- 不做多租户、workspace、member、组织、云同步或远程数据库。
- 不做分布式 worker、多副本协调、WebSocket 推送或插件市场。
- 不把前端视觉细节复制进后端；后端只提供稳定语义和可验证数据。
- 不允许为了“未来多用户”保留 `owner_id`、`workspace_id` 等空壳列；将来若真的需要，另起迁移和架构评审。

### 0.3 继承的技术基线

- Go 1.27、标准库 `net/http`、SQLite WAL + JSON1、`modernc.org/sqlite`，纯 Go 无 cgo。
- 所有持久化时间为 unix 毫秒；HTTP 时间字段为 ISO 8601。
- API 不使用 `/api/v1`；只增字段，破坏性变更才开 `/api/v2`。
- JAR 以 SHA-1 内容寻址并跨包共享；`pack_mods` 是包内模组选择的唯一权威来源。
- v6 的任务 lease、fencing、幂等双轨、outbox、Provider 限流/熔断、blob GC 规则继续有效；本文件补足其工程落点和遗漏领域。

## 1. 前端能力—后端覆盖矩阵

| 前端页面/能力 | 后端领域服务 | 持久化核心 | 必须提供的能力 |
|---|---|---|---|
| Dashboard/迎新 | `dashboardsvc`、`systemsvc` | `packs`、`pack_mods`、`conflicts`、`tasks`、`activities`、`settings` | 聚合读模型、最近编辑包、今日解决数、环境状态、onboarding 状态 |
| Pack 工作台 | `packsvc`、`searchsvc`、`resolvesvc` | `packs`、`pack_mods`、`jar_index`、`conflicts`、`pack_locks` | 包作用域搜索、加入/移除/启停、依赖和冲突、健康摘要 |
| 导入 | `importsvc` | `packs`、`pack_mods`、`tasks`、`jar_index` | CurseForge manifest、Modrinth mrpack、URL、本地 zip、安全扫描、失败可恢复 |
| 内容编辑 | `contentsvc` | `content_documents`、`content_revisions`、`content_validation_runs` | recipe/structure/ore 三种文档、草稿、应用、历史、撤销、校验和影响分析 |
| 任务书 | `questsvc` | `quest_books`、`quest_revisions`、`quest_chapters`、`quest_nodes`、`quest_edges` | 章节排序、节点属性、边、奖励、引用模组、环检测、预览 |
| 打包发布 | `buildsvc`、`publishsvc` | `pack_versions`、`delivery_checks`、`artifacts`、`releases` | 交付检查、版本、可复现 zip、产物登记、目标发布、重试与失败原因 |
| 设置 | `settingssvc`、`systemsvc` | `settings`、`secrets`、`allowed_export_dirs` | Provider 连接测试、目录登记、缓存统计、默认包偏好、密钥不回显 |

所有页面使用的原始响应必须先经过 `apps/web/src/api` 的 zod 适配器；组件不得读取数据库字段名或自行拼接业务状态。

## 2. 仓库框架与模块边界

### 2.1 目标目录

```text
mPackStation/
├─ apps/
│  ├─ server/
│  │  ├─ cmd/server/main.go              # 仅进程装配与退出码
│  │  ├─ internal/
│  │  │  ├─ config/                      # 配置来源、默认值、校验
│  │  │  ├─ httpapi/                     # 路由、解码、HTTP 错误、中间件
│  │  │  ├─ service/                     # 领域规则、事务编排、任务 handler
│  │  │  │  ├─ dashboardsvc/
│  │  │  │  ├─ packsvc/
│  │  │  │  ├─ searchsvc/
│  │  │  │  ├─ resolvesvc/
│  │  │  │  ├─ importsvc/
│  │  │  │  ├─ contentsvc/
│  │  │  │  ├─ questsvc/
│  │  │  │  ├─ buildsvc/
│  │  │  │  ├─ publishsvc/
│  │  │  │  ├─ settingssvc/
│  │  │  │  └─ systemsvc/
│  │  │  ├─ store/                       # migration + repository，唯一触 SQL
│  │  │  ├─ task/                        # 队列、worker、lease、恢复
│  │  │  ├─ provider/                    # CF/MR 适配器，唯一外部 HTTP
│  │  │  ├─ blobstore/                   # temp、blob、导出目录和 GC
│  │  │  ├─ obs/                         # 日志、audit、metrics、trace context
│  │  │  └─ platform/                    # clock、id、路径、密钥保护、磁盘
│  │  ├─ migrations/                     # 发布后的迁移文件（嵌入 binary）
│  │  └─ testdata/                       # SQL、Provider、导入、API fixtures
│  └─ web/
│     ├─ src/api/                        # HTTP client + zod contract adapters
│     ├─ src/features/                   # 页面和领域 UI
│     └─ src/mocks/                      # 仅开发/截图，不能成为生产回退
├─ docs/
│  ├─ backend-architecture.md            # v6，保留不改
│  ├─ backend-architecture-v7-engineering.md
│  ├─ api-contracts.md                   # 实现时按本文拆出，契约唯一入口
│  ├─ engineering-rules.md
│  ├─ build-and-deploy.md
│  └─ decisions/ADR-*.md
├─ scripts/                              # dev/build/test/verify/package
├─ deploy/                               # 本机安装、升级、卸载说明和模板
├─ data/                                 # 运行期生成，永不入库
└─ .github/workflows/                    # CI（若使用托管 CI）
```

### 2.2 依赖方向

```text
cmd/server → httpapi → service → {store, task, provider, blobstore, obs, platform}
task      → store + platform + obs
store     → platform（仅迁移所需 clock）
provider  → platform + obs
blobstore → platform + obs
```

强制规则：

- `httpapi` 不得 import `database/sql`、`store`、`provider`、`blobstore` 或 `task`；handler 只调用 service 接口。
- `store` 是唯一直接执行 SQL 的包；repository 不返回 `*sql.Rows`，而返回领域 DTO。
- `task` 只认识任务状态、租约、心跳和 handler registry，不认识包、模组或 Provider。
- Provider 是唯一直接发外部 HTTP/下载请求的包；service 不得自行创建 `http.Client`。
- blobstore 是唯一直接操作业务文件的包；service 只处理对象句柄和校验结果。
- 非 `cmd` 包禁止标准库 `log`；全部使用注入的结构化 logger。
- 任务、outbox、audit 的写入必须经过明确的 service/task API，不允许业务包旁路写表。

CI 用 `go list -deps` + depguard/自定义脚本验证禁依赖；用 `rg` 检查 `httpapi` SQL 关键字和 `tasks` 表写入位置。

## 3. 进程、配置、构建和部署

### 3.1 配置契约

配置顺序固定为：内置默认值 → `data/config.toml` → `MPACK_*` 环境变量 → CLI flag。非法值拒绝启动并给出字段名；密钥禁止出现在配置文件、命令行、日志和 crash dump。

最小配置模型：

```go
type Config struct {
    ListenAddr string // 默认 127.0.0.1:18871
    AllowLAN bool // 默认 false；开启时必须配置随机 auth token
    DataDir string
    FrontendOrigin string // 默认 http://127.0.0.1:5273
    ReadOnlySideEffectQPS int // provider/meta GET 全局限速
    DownloadConcurrency int // 默认 4，上限 16
    TaskRecoverInterval time.Duration // 默认 30s
    HTTPReadHeaderTimeout time.Duration // 5s
    HTTPReadTimeout time.Duration // 30s，流式上传按 handler 限制
    HTTPWriteTimeout time.Duration // 60s，日志流按 handler 限制
    Provider map[string]ProviderConfig
    Build BuildConfig
}
```

启动顺序固定：配置校验 → 绝对路径解析和目录创建 → OS 单实例锁 → 写连接 → migration/checksum → `quick_check`/`foreign_key_check` → 读连接池 → blob 一致性巡检 → temp 孤儿清理 → 密钥轮换续跑 → 任务恢复扫描 → HTTP listen。

### 3.2 工程构建

根目录脚本是跨平台入口，PowerShell 与 POSIX wrapper 只做参数转发，业务命令保持一致：

```text
scripts/dev.ps1       # 启动 web + server，固定 5273/18871
scripts/build.ps1     # npm ci/build + go build，注入 version/commit/build_time
scripts/test.ps1      # go test ./...、go vet ./...、npm test
scripts/verify.ps1    # migration checksum、depguard、contract、lint、产物检查
scripts/package.ps1   # 生成可分发目录/zip，不携带 data/和 secrets
```

构建输出：

```text
dist/mpackstation-<version>/
  mpackstation(.exe)
  web/                       # Vite 静态产物
  README.txt
  LICENSE
  VERSION
```

服务可由 `--frontend-dir` 指向静态资源；开发环境仍允许 Vite 独立运行。生产包不携带数据库、缓存、JAR、API Key 或用户导出物。版本由 Git tag/显式 `-version` 注入，禁止运行时从工作树推断。

### 3.3 部署与升级

- 默认监听回环；单机安装生成 `data/`、日志目录、锁文件和配置模板。
- Windows 支持直接运行和计划任务/服务包装；Unix 提供 systemd 示例；二者都必须使用绝对 `DataDir`。
- 升级采用“停服务 → 备份 SQLite 与配置元数据 → 启动新 binary 跑迁移 → quick check/foreign key check → readyz”流程。
- migration 失败时进程拒绝 ready，不得半启动；备份失败不得继续升级。
- 卸载不删除 `data/` 和导出物；删除数据必须由用户显式执行，并先显示绝对路径和大小。
- 部署验收在干净临时目录执行，不依赖开发机已存在的 Node、Go、数据库或缓存。

## 4. 依赖治理

- `go.mod/go.sum`、`package.json/package-lock.json` 是唯一依赖清单；构建使用 `go mod download` 与 `npm ci`，禁止 `npm install` 改 lockfile 后直接提交。
- 现有技术栈之外的依赖必须写 ADR，说明安全、体积、许可证、纯 Go/cgo、维护状态和替代方案；本文不自动引入新框架。
- Provider 优先使用标准库 HTTP；第三方 SDK 必须通过 `internal/provider` 隔离，禁止在 service 或 HTTP 层暴露 SDK 类型。
- 依赖升级分“补丁/次版本/主版本”记录，升级后必须重跑单测、契约测试、导入恶意样本、构建和干净部署 smoke test。
- CI 固定执行漏洞扫描、许可证扫描（MIT/BSD/Apache 优先，GPL/AGPL 需 ADR）、重复依赖检查和可复现 lockfile 检查。
- 生产 binary 不下载运行时依赖；前端资源在构建期打包，Provider 连接仅运行时访问官方端点。

## 5. 数据模型与迁移规则

### 5.1 单用户模型

数据库没有 `owner_id`、`workspace_id`、`tenant_id`、member 或 role 表。所有业务表按业务主键和 `pack_id` 分域；全局设置属于本机实例。HTTP 主体只有：

```go
type Principal struct { Kind string } // "local" 或 "lan"
```

它只用于审计和安全策略，不用于多租户过滤。资源授权只判断 endpoint 的读/写/执行动作和当前请求 token，不向 service 传 workspace/owner。

### 5.2 v6 表的 v7 变更

保留 v6 的 `packs`、`pack_mods`、`jar_index`、`pack_locks`、`tasks`、`task_events`、`task_idem_keys`、`outbox_events`、`activities`、`conflicts`、`audit_events`、`secrets`、`settings`、`remote_cache`、`blob_grace`、`schema_migrations`。

v7 必须追加并纳入 migration：

```sql
CREATE TABLE onboarding_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  dismissed INTEGER NOT NULL DEFAULT 0 CHECK (dismissed IN (0,1)),
  updated_at INTEGER NOT NULL
);

CREATE TABLE content_documents (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('recipe','structure','ore')),
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  active_revision_id TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(pack_id, kind, slug)
);

CREATE TABLE content_revisions (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL REFERENCES content_documents(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CHECK (revision > 0),
  state TEXT NOT NULL CHECK (state IN ('draft','applied','archived')),
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  source_revision_id TEXT REFERENCES content_revisions(id),
  created_at INTEGER NOT NULL,
  UNIQUE(document_id, revision)
);

CREATE TABLE content_validation_runs (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES content_revisions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('passed','warning','failed')),
  issues TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(issues)),
  affected_mods TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(affected_mods)),
  created_at INTEGER NOT NULL
);

CREATE TABLE quest_books (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL UNIQUE REFERENCES packs(id) ON DELETE CASCADE,
  active_revision_id TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE quest_revisions (
  id TEXT PRIMARY KEY,
  quest_book_id TEXT NOT NULL REFERENCES quest_books(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL CHECK (revision > 0),
  state TEXT NOT NULL CHECK (state IN ('draft','applied','archived')),
  created_at INTEGER NOT NULL,
  UNIQUE(quest_book_id, revision)
);

CREATE TABLE quest_chapters (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES quest_revisions(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  cover_color TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL CHECK (position >= 0)
);

CREATE TABLE quest_nodes (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES quest_revisions(id) ON DELETE CASCADE,
  chapter_id TEXT NOT NULL REFERENCES quest_chapters(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  icon TEXT NOT NULL DEFAULT '',
  x REAL NOT NULL DEFAULT 0,
  y REAL NOT NULL DEFAULT 0,
  prerequisites TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(prerequisites)),
  rewards TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(rewards)),
  mod_refs TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(mod_refs)),
  position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0)
);

CREATE TABLE quest_edges (
  id TEXT PRIMARY KEY,
  revision_id TEXT NOT NULL REFERENCES quest_revisions(id) ON DELETE CASCADE,
  from_node_id TEXT NOT NULL REFERENCES quest_nodes(id) ON DELETE CASCADE,
  to_node_id TEXT NOT NULL REFERENCES quest_nodes(id) ON DELETE CASCADE,
  CHECK(from_node_id <> to_node_id),
  UNIQUE(revision_id, from_node_id, to_node_id)
);

CREATE TABLE pack_versions (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  version TEXT NOT NULL,
  changelog TEXT NOT NULL DEFAULT '',
  lock_id TEXT REFERENCES pack_locks(id),
  created_at INTEGER NOT NULL,
  UNIQUE(pack_id, version)
);

CREATE TABLE delivery_checks (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT REFERENCES pack_versions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('dependency','conflict','missing_file','content','version','quest')),
  status TEXT NOT NULL CHECK (status IN ('passed','warning','blocked')),
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  checked_at INTEGER NOT NULL,
  UNIQUE(pack_id, pack_version_id, kind)
);

CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT REFERENCES pack_versions(id) ON DELETE SET NULL,
  path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  kind TEXT NOT NULL CHECK (kind IN ('zip','manifest','log')),
  created_at INTEGER NOT NULL
);

CREATE TABLE releases (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  pack_version_id TEXT REFERENCES pack_versions(id) ON DELETE SET NULL,
  provider TEXT NOT NULL CHECK (provider IN ('curseforge','modrinth','local')),
  status TEXT NOT NULL CHECK (status IN ('pending','publishing','succeeded','failed','canceled')),
  remote_id TEXT NOT NULL DEFAULT '',
  artifact_id TEXT REFERENCES artifacts(id),
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE allowed_export_dirs (
  name TEXT PRIMARY KEY,
  absolute_path TEXT NOT NULL UNIQUE,
  marker_verified_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
```

### 5.3 数据不变量

- `pack_locks`、content/quest revisions、pack versions 追加式；已应用版本不可原地改。
- `active_revision_id` 必须指向同一 document/book 的 `applied` revision；切换 active 与 outbox 在同一事务。
- quest edge 的两端必须属于同一 revision；应用前必须无环、无悬挂章节、奖励 schema 合法。
- content payload 按 kind 使用版本化 JSON schema；未知字段允许保留但禁止影响构建。
- `pack_versions.lock_id` 必须属于同一包；构建只能使用指定 lock 与已通过交付检查的版本。
- `artifacts` 文件先写临时路径，事务登记后原子 rename；DB 行与文件任一缺失由启动巡检标记并修复。
- 任何业务写入若影响看板，必须与 `outbox_events` 同事务；GC/巡检修复写 `kind=system`。
- migration 文件编号单调、已应用文件不可改、checksum 启动校验；任何 schema CHECK/enum 改动都必须新 migration。

### 5.4 数据保留

任务事件/终态任务 30 天、outbox 30 天、activities/audit 90 天、resolved conflicts 90 天；应用中的锁、内容/任务书 revision、pack versions、artifacts/releases 随包保留。缓存按 v6 宽限策略清理，blob 先进入 grace 再删除。每一张新表在 migration 注释和本文同时写保留责任人。

## 6. HTTP 契约与前端适配

### 6.1 通用响应

成功响应直接返回领域 JSON；列表统一：

```json
{"items": [], "next_cursor": null, "total": null}
```

错误统一为：

```json
{"error":{"code":"conflict_unresolved","message":"存在待处理冲突","request_id":"...","details":{}}}
```

写请求支持 `Idempotency-Key`；分页 cursor 绑定过滤条件和排序；所有写请求禁止跨包批量操作。

### 6.2 Dashboard/系统/迎新

```text
GET  /api/dashboard
GET  /api/tasks?recent=20
GET  /api/activities?limit=10
GET  /api/system/health
GET  /api/system/status
GET  /api/onboarding
PUT  /api/onboarding
GET  /api/meta/mc-versions
```

dashboard 的 `packs[]` 必须一次返回前端规格所需的 `id/name/iconUrl/mcVersion/loader/packVersion/modCount/conflicts/edits/alerts/lastEditedAt/createdAt`；`packVersion` 来自最新 `pack_versions.version`，无版本返回 `"0.1.0"`，该默认值由 service 定义而不是前端猜测。

`todayResolvedCount` 来自当天（按配置时区）被标记 resolved 的 conflict 与成功 validation 的计数，去重以事件 ID；`crashes` 来自导入的 crash report 索引，若尚无 crash 领域则稳定返回 0 并在 schema 中保留来源说明。`updatable` 来自最近一次 provider 版本预演快照，未知时返回 0 但 `system/status` 的 provider 健康必须能区分 unavailable。

任务 API 对前端提供适配后的枚举：

| 前端 | 领域 task kind | 说明 |
|---|---|---|
| `index-mod` | `index` | 模组进入包后的 JAR/元数据索引 |
| `build-pack` | `build` | 生成整合包产物 |
| `import-pack` | `import` | 导入、扫描、解析和入包 |
| `update-preflight` | `resolve` | 更新候选和依赖预演 |

内部 `canceled` 对外序列化为前端既有的 `cancelled`，服务端内部永远只保留一种拼写。activities 的领域 `kind + action` 由 adapter 映射为 `add-mod/resolve/build/alert/edit/import`，text 由 service 生成，前端不拼业务句子。

### 6.3 Pack 与模组

```text
GET    /api/packs
POST   /api/packs
GET    /api/packs/{packId}
PATCH  /api/packs/{packId}
POST   /api/packs/{packId}/duplicate
POST   /api/packs/{packId}/archive
POST   /api/packs/{packId}/unarchive
DELETE /api/packs/{packId}
GET    /api/packs/{packId}/mods
POST   /api/packs/{packId}/mods
POST   /api/packs/{packId}/mods/local
PATCH  /api/packs/{packId}/mods/{modId}
DELETE /api/packs/{packId}/mods/{modId}
GET    /api/packs/{packId}/mod-search?... 
GET    /api/packs/{packId}/conflicts
POST   /api/packs/{packId}/conflicts/{conflictId}/resolve
POST   /api/packs/{packId}/conflicts/{conflictId}/ignore
POST   /api/packs/{packId}/resolve
GET    /api/packs/{packId}/locks
GET    /api/packs/{packId}/health
```

创建时 MC 版本和 loader 必填；修改二者会创建全包重解任务，不直接篡改已应用 lock。删除二次确认由前端负责，后端仍要求包存在、非运行任务或先取消级联任务，并明确不删除共享 JAR blob。

### 6.4 内容编辑

```text
GET    /api/packs/{packId}/content/summary
GET    /api/packs/{packId}/content?kind=recipe|structure|ore
POST   /api/packs/{packId}/content
GET    /api/packs/{packId}/content/{documentId}
PUT    /api/packs/{packId}/content/{documentId}/draft
POST   /api/packs/{packId}/content/{documentId}/validate
POST   /api/packs/{packId}/content/{documentId}/apply
POST   /api/packs/{packId}/content/{documentId}/rollback
GET    /api/packs/{packId}/content/{documentId}/history
```

recipe payload：输入/输出槽位、数量、连接/配方类型；structure payload：结构文件引用、尺寸、旋转、参数和预览元数据；ore payload：维度、方块、生成层、数量、频率、分布和 biome 条件。三种 payload 都必须有 `schema_version`。

校验 issue 统一为 `{code,severity,path,message,details}`，并返回 `affectedMods[]`。`save draft` 只追加 draft revision；`apply` 在一个短事务中校验最新 draft、切 active、写 outbox，并触发必要的 resolve/build invalidation；磁盘预览和 Provider 查询不得持有事务。

### 6.5 任务书

```text
GET    /api/packs/{packId}/quests
PUT    /api/packs/{packId}/quests/draft
POST   /api/packs/{packId}/quests/validate
POST   /api/packs/{packId}/quests/apply
POST   /api/packs/{packId}/quests/rollback
GET    /api/packs/{packId}/quests/history
GET    /api/packs/{packId}/quests/preview
```

draft 请求包含 chapters、nodes、edges 的完整快照；service 在一次事务中检查 ID 唯一、章节顺序、节点章节归属、边同 revision、无环、前置条件存在、奖励结构、mod refs 是否指向包内模组。preview 只返回渲染所需的稳定视图，不改变业务状态。

### 6.6 构建与发布

```text
GET    /api/packs/{packId}/delivery-checks
POST   /api/packs/{packId}/delivery-checks/run
GET    /api/packs/{packId}/versions
POST   /api/packs/{packId}/versions
POST   /api/packs/{packId}/build
GET    /api/packs/{packId}/artifacts
GET    /api/packs/{packId}/artifacts/{artifactId}/download
GET    /api/packs/{packId}/releases
POST   /api/packs/{packId}/publish/{provider}
```

build 请求必须绑定 `packVersionId` 和 lock；构建输入清单按稳定排序，zip 条目时间、压缩参数和 manifest 顺序固定，产物计算 SHA-256 后登记。发布任务引用已登记 artifact；非幂等发布不自动重试，用户显式 retry 前先查询远端状态，避免重复发布。

### 6.7 设置、密钥和导入

```text
GET    /api/settings
PATCH  /api/settings
PUT    /api/settings/secrets/{key}
POST   /api/settings/providers/{provider}/test
GET    /api/settings/storage
POST   /api/settings/cache-gc
GET    /api/settings/export-dirs
PUT    /api/settings/export-dirs
DELETE /api/settings/export-dirs/{name}
POST   /api/packs/import/inspect
POST   /api/packs/import
```

import 支持 `source=curseforge|modrinth|local|url`；inspect 只解析预览不落业务数据，confirm/import 创建任务。输入为 multipart 文件或受限 URL，不允许前端直接传任意服务器路径。导入过程中先写 temp、扫描 zip、拒绝路径穿越/符号链接/超大条目/压缩炸弹，再事务写包和模组，最后原子 rename。

## 7. 任务、Provider、文件与恢复

### 7.1 任务统一协议

所有会超过 HTTP 请求生命周期或触碰大量文件/上游的工作都进入持久化任务：`resolve/download/index/build/publish/import/cache_gc`。任务状态、合法迁移、lease、`lease_epoch` fencing、幂等键、恢复预算、进度节流和取消语义沿用 v6；v7 增加领域验收要求：

- task handler 在 service 实现，task runner 不导入领域包。
- handler 的每个外部调用、文件阶段、事务阶段写结构化 task event。
- cancel 是协作式：不再领取新工作，停止可中断 I/O，最后一次状态写必须带当前 epoch。
- pause 只允许可安全暂停的 resolve/download/index/build 阶段；publish 不可暂停。
- 任务失败必须给稳定 `error_code`、用户 message 和可展开 detail；日志文件路径不直接暴露绝对系统路径。

### 7.2 Provider 合同

Provider 接口至少覆盖 search、project、versions、metadata、download、publish、remote-status；使用领域 DTO，禁止泄漏 CF/MR SDK 类型。每个 provider 自带分页 fixture、限流/熔断、404、鉴权失败、离线缓存和文件指纹测试。CF/MR 的 loader、MC version、依赖类型映射在适配器内部完成，service 只看到统一模型。

### 7.3 文件和 GC

```text
data/
  mpackstation.db
  config.toml
  blobs/sha1/<ab>/<sha1>
  provider-cache/<provider>/
  imports/tmp/
  tasks/<taskId>/
  locks/<packId>/
  exports/<packId>/<version>/
  logs/
```

所有用户路径先 resolve 为绝对路径，再检查必须位于 data 或经过 marker 验证的 export dir；禁止跟随符号链接。文件删除以引用计数/SQL 查询和 grace 宽限为依据，不按目录名猜测。启动巡检、cache_gc、用户清理三条路径复用同一个 blobstore API。

## 8. 鉴权与安全边界（合并维度）

安全目标是防恶意网页、跨源浏览器请求、任意文件读写、凭据泄露、资源耗尽和 Provider 配额滥用；不防同机同 OS 用户进程。单用户本地访问默认读端点可读，所有写/执行/带上游副作用的请求必须带启动 token。

- 启动生成高熵 token，写入仅当前运行期可读的内存和本地受保护文件；API 不回显 token。浏览器由启动页一次注入，写请求带 `X-MPack-Token`。
- 校验 `Host`、`Origin`、方法和 Content-Type；CORS 默认只允许配置的 FrontendOrigin。`AllowLAN=true` 时所有端点均需 token，并拒绝通配 Origin。
- Provider/meta/search 等副作用 GET 走全局限速；Provider 自身再按 CF/MR 限流、熔断、deadline 和 Retry-After。
- JSON body 默认 8 MiB；导入流式上限 512 MiB，限制条目数、解压比、单文件大小、总展开大小和解析深度。
- 路径策略拒绝 `..`、绝对路径越界、NTFS alternate data stream、符号链接和 junction；导出目录必须存在 marker 文件且每次导出复检。
- API Key 使用 OS credential store 或平台密钥保护；数据库只存版本化密文，轮换可续跑；日志、错误、活动和前端响应永不出现明文 key。
- 错误信息不泄漏 SQL、绝对路径、上游 token 或内部栈；详细栈只进受控日志。
- 每个写/执行动作写 audit：principal kind、action、target、request_id、detail、时间；审计查询只返回脱敏后的稳定字段。
- 读写 timeout、优雅退出、磁盘预检、单实例 OS lock、SQLite busy timeout、WAL checkpoint 和 crash recovery 属于同一安全/健壮性边界，不可在“本地应用”中省略。

## 9. 可观测性

结构化日志采用 JSON，至少含 `timestamp, level, component, request_id, task_id?, pack_id?, operation, duration_ms, result, error_code?`；禁止记录密钥、完整用户路径和未脱敏上游响应。

指标最小集：

```text
http_requests_total / http_request_duration_ms
task_queued / task_running / task_failed / task_recovered
provider_requests / provider_failures / provider_throttled / breaker_state
db_busy / db_txn_duration_ms / migration_failures
blob_bytes / cache_bytes / disk_free_bytes
outbox_pending / outbox_delivery_failures
```

`GET /api/system/status` 输出面向用户的聚合状态；内部 debug/metrics 不作为前端契约。每个 task 有最近 200 条事件，日志流端点按 task 授权和 token 策略提供。

## 10. 开发规范、受保护文件与变更流程

### 10.1 受保护文件

以下文件不是绝对不可改，而是“必须伴随相应证据修改”：

```text
docs/architecture/backend-architecture.md                 # v6，禁止顺手改
docs/architecture/backend-architecture-v7-engineering.md  # v7 变更需 ADR/评审记录
apps/server/migrations/*.sql                 # 已发布 migration 禁止修改
apps/server/internal/store/schema.sql        # 仅由 migration 生成/校验
apps/web/src/api/**/types.ts                 # 必须同步 zod + contract fixture
apps/web/src/api/http.ts                     # 必须跑 CORS/token/timeout 测试
apps/server/go.mod, go.sum
apps/web/package.json, package-lock.json
scripts/build.ps1, scripts/verify.ps1
```

### 10.2 代码规则

- Go 必须 `gofmt`、`go vet`；导出 API 有注释；错误使用 sentinel/typed error + 稳定 code。
- service 输入先校验，事务只包含 SQL 和短内存操作；任何网络/磁盘 I/O 在事务外。
- repository 方法命名体现查询边界（如 `ListPackMods(ctx, packID, filter)`），不接收任意 SQL。
- JSON 用明确 DTO 和 schema version；时间、枚举、分页和错误不由调用方自由发挥。
- 前端组件不直接 `fetch`；mock 与真实 API 共享 zod schema；适配器负责命名转换和显示枚举。
- 业务常量集中在领域包；禁止在 handler、React 组件和 SQL 中散落魔法字符串。

### 10.3 变更流程

1. API 变化：更新本文/`api-contracts.md`、zod schema、fixture、契约测试和迁移说明。
2. DB 变化：新建编号 migration，写不变量、保留策略、回滚/备份说明，运行 checksum 和升级 smoke test。
3. 架构/依赖变化：新增 ADR，列出替代方案、影响层、测试与发布风险。
4. 任务/Provider/错误枚举变化：同步 DB CHECK、领域类型、HTTP 映射、前端 adapter、fixture。
5. 每个 PR/交付单必须包含验证结果；没有测试证据的“已完成”不算完成。

## 11. 测试、契约与验收

### 11.1 测试层次

- 单元：service 规则、状态机、路径策略、错误映射、zod adapter。
- repository：临时 SQLite、FK/check/unique、migration checksum、读写连接隔离。
- 集成：启动顺序、单实例锁、readyz、优雅退出、任务 kill-restart、outbox 投递、blob rename。
- Provider fixture：搜索分页、404、限流、熔断、缓存 stale、指纹、依赖映射、发布状态。
- 导入安全：zip slip、symlink/junction、压缩炸弹、深目录、超大文件、恶意 JSON/manifest。
- 契约：每个端点请求/响应/错误 envelope 快照；前端 mock 与真实 fixture 双向解析。
- E2E：空态/有包态、创建包、加入模组、冲突、内容/任务书应用、构建、下载 artifact、设置保存。

### 11.2 固定验收命令

```text
go test ./...
go vet ./...
npm ci
npm test
npm run build
scripts/verify.ps1
scripts/package.ps1
```

验收必须覆盖：新目录启动、已有数据库升级、异常 migration、重复请求、服务 kill 后恢复、磁盘不足、Provider 不可达、前端 `USE_MOCK=false`。前端页面视觉验收仍按各 `docs/specs` 执行，视觉瑕疵可留到收工阶段，但接口和状态契约不能留到最后。

### 11.3 里程碑门槛

- E0 工程基座：目录、脚本、配置、migration runner、部署 smoke、依赖规则。
- E1 看板真实化：dashboard/tasks/activities/system/onboarding + pack CRUD，前端可关闭 mock。
- E2 模组闭环：Provider、搜索、加包、下载、索引、求解、锁、冲突。
- E3 内容与任务书：完整 revision/校验/apply/rollback/preview。
- E4 交付发布：delivery checks、pack version、可复现 artifact、local/CF/MR release。

每个门槛必须满足单测、集成、契约、迁移和干净部署五类证据；只实现 endpoint 而没有数据不变量或恢复测试，不得标记门槛通过。

## 12. 自评（按图片十维度；按要求合并鉴权与安全）

评分对象是“本文 v7 设计完整度”，不是当前代码完成度。第 9/10 项按要求合并为一个维度，并增加工程化交付作为第十项，保持十项评分表。

| 维度 | 评分 | 判断 |
|---|---:|---|
| 耦合性 | 9/10 | cmd、HTTP、service、store、task、provider、blobstore 边界和 CI 禁依赖已明确；仍需落地 depguard 才能证明。 |
| 可维护性 | 9/10 | 领域服务、repository、统一错误、migration、ADR、受保护文件和变更流程齐全；规范文件与脚本仍待实现。 |
| 可扩展性 | 9/10 | 内容/任务书/构建发布均有独立领域模型，Provider 用统一接口；未来新增领域仍会增加迁移和契约成本。 |
| 健壮性 | 9/10 | timeout、优雅退出、启动检查、文件巡检、磁盘预检、导入边界、升级备份和恢复验收完整。 |
| 数据一致性 | 9/10 | FK、CHECK、revision、lock 归属、outbox、temp→事务→rename、checksum 和 GC 闭环均有规则。 |
| 异步任务能力 | 9/10 | lease/fencing/幂等/恢复/取消/暂停/重试/事件和 handler 归属明确；仍需 kill-restart 测试兑现。 |
| Provider 扩展能力 | 8.5/10 | 统一 DTO、适配器隔离、限流熔断缓存、fixture 和发布状态覆盖；各平台差异仍需实现时验证。 |
| 可观测性 | 8.5/10 | request/task/pack 关联日志、审计、指标、任务日志和用户状态聚合齐全；尚未指定具体 metrics 导出实现。 |
| 鉴权与安全边界 | 9/10 | 去掉多租户和伪 ownership，明确单用户 token、Origin/Host、LAN、密钥、路径、导入和资源耗尽防护。 |
| 工程化交付 | 8.5/10 | 仓库框架、依赖、脚本、部署、升级、受保护文件、契约测试和验收流程明确；跨平台安装器细节留给 build/deploy 实作。 |

**总评：88.5/100（8.85/10）。** 主要扣分项不是设计缺口，而是“规范、脚本、migration、契约 fixture、恢复测试尚未变成仓库中的可执行资产”。

## 13. v7 决策记录

1. 删除 `owner_id/workspace_id/tenant_id` 及多租户就绪设计；本应用是本机单用户服务。
2. 将鉴权与安全边界合并成一个设计维度：token 是访问保护手段，资源授权不再承担租户隔离职责。
3. 将 v6 留下的 M3/M4 schema/接口补为正式 revision、任务书、pack version、delivery check、artifact、release 模型。
4. 将前端展示枚举与后端领域枚举分离，通过 API adapter 固定映射，避免数据库字段迁就 UI 文案。
5. 将工程构建/部署与产品构建/发布明确区分；两者均必须可在干净目录验证。
6. 将“受保护文件 + 变更证据 + 契约测试”纳入架构，而不是依赖开发者记忆。
