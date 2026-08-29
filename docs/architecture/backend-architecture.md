# mPackStation 后端架构设计 v6

> 唯一权威设计文档。v6 相对 v5 的变更全部来自第四轮三路盲审的 P0/P1 清单，逐条内联可溯。
> 标准：机制必须落到可执行的接口/SQL/DDL；DDL 即语义；自称修复必须经得起独立推演；
> 不变量集中收录于 §3.3，其他章节引用而不复述（v6 起纪律，防改一漏二）。

## 0. 前提与非目标

- 本机单用户服务，监听回环；授权、密钥、路径、浏览器攻击面按"恶意网页/未来多用户"设防。
- 非目标：分布式部署、多副本、WebSocket 推送、插件系统。
- 技术栈：Go 1.27 + `net/http` + SQLite（WAL + JSON1，`modernc.org/sqlite`，DSN 一律用 `_pragma=` 语法；M0 用 `SELECT json_extract('{"a":1}','$.a')` 探针验证）。
- **存量数据**：v1 开发库作废重建，0001_init.sql 即 §3.2 全量 schema。
- **全局口径**：所有落库时间戳一律 **unix 毫秒**；本文档公式中的常量（如退避秒数）为秒，落库前 ×1000（v6 定死）。
- **API 演进策略**：无 `/api/v1` 前缀；响应只增字段不改语义，破坏性变更才引入 `/api/v2`（v6 声明）。
- **里程碑范围**：M3（内容/任务书）与 M4（构建发布落库模型）的 schema/接口在 M2 冻结评审后按本标准另补；§7.1 对应端点标注 [M4 待定]。

## 1. 总体架构与依赖规则

### 1.1 分层

```text
cmd/server            进程装配（唯一允许 log.Fatalf）
internal/config       配置加载、校验
internal/httpapi      路由、解码/校验、错误信封、中间件（含 §9.5 三防线）
internal/service/*    领域服务：事务编排、业务规则、授权检查、任务 handler 实现（§5.1）
internal/store        migration + repository（唯一直接触 SQL）
internal/task         任务队列、worker 池、恢复扫描（依赖 store；不认识任何领域逻辑）
internal/provider/*   CurseForge / Modrinth 适配器（唯一直接触外部 HTTP，含文件下载）
internal/blobstore    文件暂存、校验、共享对象存储、GC 文件操作（内部全局互斥锁 §11）
internal/obs          日志、request-id、audit、metrics
internal/platform     时钟、ID 生成、路径策略、KeyProtector
```

依赖方向：`cmd → httpapi → service → {store, task, provider, blobstore}`；`service → obs/platform`；`task → store`；store/provider/blobstore 互不引用。**httpapi 不允许 import store**（现状 `httpapi.go` 直持 `*sql.DB` 是 M0 必须消除的已知差距）。

### 1.2 强制手段（depguard 全量，v6 补齐 task 与全部反向）

CI depguard 拒绝清单：
- `internal/httpapi`：拒 `database/sql`、`internal/store`、`internal/task`、`internal/provider/*`、`internal/blobstore`。
- `internal/store`：拒 `net/http`、`internal/service/*`、`internal/task`。
- `internal/task`：拒 `net/http`、`internal/provider/*`、`internal/blobstore`、`internal/service/*`。
- `internal/provider/*`：拒 `database/sql`、`internal/store`。
- `internal/blobstore`：拒 `net/http`、`database/sql`。
- 全部非 cmd 包：拒标准库 `log`。

grep 检查：**`tasks` 表的一切写（INSERT/UPDATE/DELETE）只出现在 `internal/task`**（v6 扩展到 DELETE）；`internal/httpapi` 内 SQL 关键字零命中。

### 1.3 其他纪律

- 领域错误 = sentinel error + 错误码；HTTP 映射只在 `httpapi` 错误中间件。
- 时间经 `platform.Clock`：wall-clock（落库/跨进程比较）与 monotonic（进程内判活/计时）分离；恢复类测试注入假时钟。
- **GET 纪律（v6 重定义，消除被证伪的前提）**：GET 不变更业务域状态；**豁免类副作用仅限**且必须枚举：① 触发上游 provider 调用；② 写 remote_cache。带这两种副作用的 GET（§7.1 中 mod-search、providers/*、meta/*）受进程级全局限速（默认 30 req/min，可配）保护，防恶意网页烧上游配额。路由 CI 审查按此枚举清单执行。
- 威胁模型如实声明：不防同机同 OS 用户进程；读端点无 token 意味着同机其他会话可读本机服务数据——接受并文档化。

## 2. 配置与进程生命周期

### 2.1 配置（`internal/config`）

```go
type Config struct {
    ListenAddr          string        // 默认 127.0.0.1:18871；仅回环（§9.4）
    AllowLAN            bool          // 默认 false；§9.4 强制约束
    DataDir             string        // 启动 resolve 绝对路径 + 可写校验
    FrontendOrigin      string        // §9.5；默认 http://127.0.0.1:5273
    DownloadConcurrency int           // 任务内文件级下载并发（默认 4，上限 16）；
                                      // 与 §5.6 任务级配额 download≤3 是两个正交旋钮（v6 写明）
    ReadOnlySideEffectQPS int         // §1.3 豁免 GET 的全局限速，默认 30 req/min
    Providers           map[string]ProviderConf  // 每 provider 限流/熔断数值（§6.2 默认值在此可覆盖）
    LogLevel            string
    TaskRecoverInterval time.Duration // 默认 30s
}
```

加载顺序：默认值 → `data/config.toml` → `MPACK_*` 环境变量 → flag。非法值拒绝启动。密钥不入配置文件。

### 2.2 生命周期

- 启动序列：config → 目录准备 → 单实例锁 → **写连接打开 + migration** → `PRAGMA quick_check`（v6：启动完整性快速校验；失败即退出）→ `foreign_key_check`（失败即退出；readyz 只覆盖运行期降级）→ 读连接池打开（**必须在写连接完成 WAL 恢复后**，v6 写明顺序）→ 一致性巡检（§11，先于 temp 清理）→ temp 孤儿清理 → 密钥轮换续跑检测（§9.1）→ 任务恢复扫描（§5.4）→ listen。
- **单实例锁**：对 `data/server.lock` 施加 OS 级排他文件锁（Windows `LockFile` / Unix `flock`），内核随进程死亡自动释放；文件内容仅供人类排查。
- 优雅退出：`Shutdown`（10s）→ queue 停派新任务 + cancel 所有 running ctx → 等 worker 退出（≤15s，超时留给重启恢复；重启后最坏一个 lease 周期内完成恢复，fencing 兜底双跑，v6 写明该叠加窗口）→ 关 DB → 释放文件锁。
- **HTTP server 超时（v6 新增节）**：`ReadHeaderTimeout=5s` 全局；`ReadTimeout/WriteTimeout` 默认 30s/60s，但**流式端点豁免**：`POST /packs/import`、`POST /packs/{id}/mods/local`（读侧不限时，受 §7.3 上限与信号量约束）、`GET /tasks/{id}/log`（写侧不限时）——逐 handler 设置，不用全局 `http.TimeoutHandler`。
- `/api/healthz` 存活 / `/api/readyz` 就绪（DB ping + 目录可写）分离；`/api/health` 兼容别名。

### 2.3 SQLite 连接策略

- DSN：`_pragma=journal_mode(WAL)`、`_pragma=foreign_keys(1)`、`_pragma=busy_timeout(5000)`。
- 写连接全局单一（`SetMaxOpenConns(1)`）；读连接池独立且 `_pragma=query_only(1)`（连接级只读强制）。
- 事务不跨网络/磁盘 I/O 持有（先 I/O，后短事务登记）。

## 3. 数据层

### 3.1 编号迁移

`internal/store/migrations/NNNN_name.sql`，编号单调、不可复用、已应用文件不可改（CI 检查）。`schema_migrations` 启动校验 checksum。每迁移单事务；`foreign_keys=ON` 全连接强制。

### 3.2 全量 DDL（0001_init.sql 即此全文；时间戳均为 unix ms）

```sql
CREATE TABLE packs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    owner_id TEXT NOT NULL DEFAULT 'local',  -- v6：一列成本换多租户迁移量级下降；查询条件升级只在 store
    mc_version TEXT NOT NULL CHECK (mc_version GLOB '[0-9]*.[0-9]*'),  -- 弱形态校验，完整校验在 service
    loader TEXT NOT NULL CHECK (loader IN ('forge','neoforge','fabric','quilt')),
    loader_version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    current_lock_id TEXT REFERENCES pack_locks(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
-- 循环 FK 约定：pack_locks append-only 永不单独删除；建锁顺序 = INSERT lock →
-- UPDATE packs.current_lock_id（同事务）；"锁属于本包" service 同事务校验，失败回滚。
-- archived 规约：写操作一律 pack_archived(409)；唯一例外：PATCH 解归档（status→active）。
-- 推论（v6 写明）：archived 包不可 DELETE/duplicate，须先解归档——产品口径如此，有意为之。
-- pack_locks 与 snapshot 文件保留（v6 补）：随包生命周期，不单独清理；包删除时随 §11 清单清理。

CREATE TABLE pack_locks (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    snapshot_path TEXT NOT NULL,
    resolved_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (pack_id, version)
);

CREATE TABLE jar_index (
    sha1 TEXT PRIMARY KEY CHECK (length(sha1) = 40 AND NOT sha1 GLOB '*[^0-9a-f]*'),
    fingerprint_cf INTEGER,                  -- CF MurmurHash2（参数 §6.1），索引时与 sha1 同步计算
    file_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    mod_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(mod_ids)),
    loaders TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(loaders)),
    mc_versions TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(mc_versions)),
    raw_meta_path TEXT NOT NULL DEFAULT '',
    parsed_at INTEGER NOT NULL
);

CREATE TABLE pack_mods (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('curseforge','modrinth','local')),
    project_id TEXT,
    CHECK ((source = 'local') = (project_id IS NULL)),
    version_id TEXT NOT NULL DEFAULT '',     -- CF 落库为 "<modId>:<fileId>"；service 写入时校验
                                             -- 复合串前缀 == project_id（v6 声明一致性职责）
    display_name TEXT NOT NULL,
    pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0,1)),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    dep_origin TEXT NOT NULL DEFAULT 'user' CHECK (dep_origin IN ('user','required','optional')),
    sha1 TEXT REFERENCES jar_index(sha1),    -- 可空：NULL=未下载/未解析
    added_at INTEGER NOT NULL,
    UNIQUE (pack_id, source, project_id)
);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    payload_hash TEXT NOT NULL DEFAULT '',   -- canonical JSON（键排序、无空白）的 sha1，§5.5
    pack_id TEXT REFERENCES packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN
        ('resolve','download','index','build','publish','import','cache_gc')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN
        ('queued','leased','running','paused','succeeded','failed','canceled')),
    priority INTEGER NOT NULL DEFAULT 100,
    progress REAL NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    attempt INTEGER NOT NULL DEFAULT 0,
    recover_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 10),
    lease_owner TEXT,
    lease_epoch INTEGER NOT NULL DEFAULT 0,
    lease_expires_at INTEGER,
    run_after INTEGER NOT NULL DEFAULT 0,
    deadline_at INTEGER NOT NULL DEFAULT 0,  -- v6：任务级兜底 deadline（领取时 = now + 2h），超时置 failed
    payload_path TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_tasks_poll ON tasks(status, run_after, priority, created_at);
-- v6：服务端活跃去重的原子载体（部分唯一索引，SQLite 支持）：
-- 同 kind+包+payload 的非终态任务至多一行；无包任务（pack_id NULL）按 kind 去重。
CREATE UNIQUE INDEX idx_tasks_active_dedup
    ON tasks(kind, COALESCE(pack_id,''), payload_hash)
    WHERE status IN ('queued','leased','running','paused');
-- 保留：终态且 updated_at 超过 30 天的行由 cache_gc 删除（task_events 随 CASCADE，
-- payload/日志文件走 §11 清理清单；幂等键不受影响，见下表）。

-- v6 新增：幂等键独立留档表，永不删除（行小：键+哈希+任务指针）
CREATE TABLE task_idem_keys (
    key TEXT PRIMARY KEY,                    -- 客户端生成，契约 §5.5
    payload_hash TEXT NOT NULL,
    task_id TEXT NOT NULL,                   -- 逻辑指向 tasks.id；任务行被 30 天清理后
                                             -- 本行保留，重提同键返回"任务已清理"应答而非重复执行
    created_at INTEGER NOT NULL
);

CREATE TABLE task_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('status','progress','log_ref','recovered','retry')),
    data TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data)),
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_task_events_task ON task_events(task_id, id);
-- 保留：每任务最近 200 条（写入方同事务裁剪 + cache_gc 兜底；
-- 进度写入已被 §5.3 节流限频，裁剪成本低，v6 评估结论：接受）

CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY,
    pack_id TEXT REFERENCES packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN
        ('pack','mod','conflict','task','build','content','quest','system')),
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    delivered_at INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_outbox_pending ON outbox_events(delivered_at) WHERE delivered_at IS NULL;
-- 保留：delivered_at 非空且超过 30 天的行由 cache_gc 删除（activities 无 FK，不阻塞）

CREATE TABLE activities (
    id TEXT PRIMARY KEY,
    origin_event_id TEXT NOT NULL UNIQUE,   -- 快照列（去重），故意无 FK：90 天留存独立于 outbox 30 天
    pack_id TEXT REFERENCES packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN
        ('pack','mod','conflict','task','build','content','quest','system')),
    text TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_activities_time ON activities(created_at DESC);
-- 保留：90 天，cache_gc 清理

CREATE TABLE conflicts (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('dependency','version','loader','duplicate','file','distribution')),
    fingerprint TEXT NOT NULL,          -- 冲突对象（排序后 mod/版本组合）的 sha1 前 16 位
    severity TEXT NOT NULL DEFAULT 'error' CHECK (severity IN ('error','warning')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','resolved','ignored')),
    summary TEXT NOT NULL,
    detail_path TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,        -- 首见时间，UPSERT 不覆盖
    updated_at INTEGER NOT NULL,        -- 最近命中时间，UPSERT 刷新
    resolved_at INTEGER,
    UNIQUE (pack_id, kind, fingerprint)
);
CREATE INDEX idx_conflicts_pack ON conflicts(pack_id, status);
-- UPSERT 列清单（v6 修复活漏洞）：命中同指纹时更新 summary/detail_path/severity/updated_at，
-- status = CASE WHEN status='ignored' THEN 'ignored' ELSE 'pending' END
-- （ignored 不复活；resolved 复活为 pending——重跑仍存在的冲突必须重新拦构建门禁）；
-- resolved_at 在非 ignored 复活时清 NULL。本轮未命中的 pending 行同事务置 resolved。
-- 保留：resolved 超 90 天由 cache_gc 删除（pending/ignored 永不自动清）。

CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    principal TEXT NOT NULL DEFAULT 'local',
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_audit_time ON audit_events(created_at DESC);
-- 保留：90 天，cache_gc 清理；历史 principal='local' 视为系统主体

CREATE TABLE secrets (
    key TEXT PRIMARY KEY,
    value_enc TEXT NOT NULL,            -- v1:<keyId>:<base64 nonce>:<base64 ct>（§9.1）
    updated_at INTEGER NOT NULL
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

CREATE TABLE remote_cache (
    cache_key TEXT PRIMARY KEY,         -- <provider>:<method>:<全量参数哈希>
    payload_path TEXT NOT NULL,
    fetched_at INTEGER NOT NULL,
    ttl_seconds INTEGER NOT NULL CHECK (ttl_seconds > 0)
);
-- 过期处置（v6 修矛盾）：过期后仍保留 24h 宽限供 §6.3 stale 回退；
-- cache_gc 只删 fetched_at + ttl + 24h 之后的条目（文件 + 行同事务）。

CREATE TABLE blob_grace (
    sha1 TEXT PRIMARY KEY REFERENCES jar_index(sha1) ON DELETE CASCADE,
    orphaned_at INTEGER NOT NULL
);

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL,
    checksum TEXT NOT NULL
);
```

**新增表/列的同步义务（v6  checklist）**：每张新表必须同时给出保留策略/GC 责任人；新增任务/事件类型必须同步 tasks.kind、outbox/activities kind 三处 CHECK 的迁移。

### 3.3 一致性不变量（唯一收录处，其他章节引用本节编号）

1. 包至多一个 `current_lock_id` 且锁属于本包（同事务校验，失败回滚）。
2. 包删除：业务表级联清；jar/blob 走 §11 孤儿判定 + 宽限期，不直接删。
3. 任务状态迁移只允许 §5.2 的边；tasks 表的一切写只在 `internal/task`。
4. **一切业务表写**（含巡检修复写，如 jar_index 行删除导致的 pack_mods.sha1 置 NULL）凡需看板感知，同事务写 `outbox_events`（巡检修复用 kind='system'）。无例外（v6 消除巡检例外）。
5. `pack_mods.sha1` 未下载写 NULL；下载完成按 §5.3 顺序（temp→登记→rename），**pack_mods.sha1 回填与 jar_index 登记同一 InTx**（v6 写明）。
6. local 模组去重：添加时同步计算 sha1（流程 §6.4），查重 + 插入在同一 InTx；同包同 sha1 重复添加返回 `duplicate_mod`。
7. 冲突 UPSERT 按 §3.2 列清单执行（ignored 不复活、resolved 复活 pending）。
8. 幂等键终生有效：键存 `task_idem_keys` 永不删除；任务行 30 天清理不影响键语义（v6）。

## 4. 领域服务层

`packsvc`/`searchsvc`/`resolvesvc`/`contentsvc`/`questsvc`/`buildsvc`/`systemsvc`，统一协议：

```go
func (s *PackService) AddMod(ctx context.Context, p Principal, in AddModInput) (ModView, error)
```

- 输入校验在 service 入口；跨资源写在单个 `store.InTx`；构造函数显式注入依赖。
- **授权资源模型（定型）**：
  ```go
  type Resource struct {
      Domain string  // "pack" | "settings" | "secrets" | "task" | "system"
      PackID string  // Domain=pack/task 时非空
      Action string  // "read" | "write" | "delete" | "execute"
  }
  ```
- 写方法必须过 authorize；读方法当前依赖 repository 强制 packID 兜底。**未来工作清单（如实）**：member 权限矩阵；读路径 authorize；dashboard 跨包聚合多用户口径（重做聚合层）；实例级资源 ownership；token→多用户 session 整体替换（无中间态）。
- **守护测试**：反射扫描各 service 导出写方法，在 deny 探针（`MPACK_AUTH_DENY_PROBE=1`）下逐一调用，断言全部 `permission_denied`。

## 5. 异步任务系统（`internal/task`）

### 5.1 结构与 handler 归属（v6 落地）

- `task` 包只认识队列语义：`Queue` 暴露 `Submit/Cancel/Pause/Resume/Retry/CancelByPack(ctx, tx, ...)`（全部接受 `store.Tx`，事务所有权在调用方）与 worker 池。
- **handler 接口** `type Handler interface { Kind() string; Run(ctx context.Context, t Task, prog ProgressFunc) error }`，**实现在 service 层**（各 service 按需要组合 provider/blobstore/store），启动时注册进 `task.Registry`。task 包通过 Registry 拿 handler，自身不 import 任何领域包（§1.2 depguard 保证）。
- blobstore 锁的持锁主体：**blobstore 包级方法内部**（`RegisterFromTemp`/`GCDelete` 等公共方法各自持包内全局互斥锁，调用方无感知，v6 写明）。

### 5.2 状态机

```text
submit → queued ──配额门+lease(epoch+1)──▶ leased ──begin──▶ running ──成功──▶ succeeded（终态）
           ▲                            │                     ├─失败, attempt<max──▶ queued+run_after（自动重试，不落 failed）
           │                            │                     ├─attempt>=max / deadline──▶ failed（终态）
           │                            │                     └──cancel──▶ canceled（终态）
           └──── paused ◀──▶ queued（仅 queued 可暂停/恢复）
```

- **终态 = succeeded/failed/canceled，永远不可逆**。自动重试不经过 failed。
- 唯一例外：`POST /api/tasks/{id}/retry`（用户显式）：`failed/canceled → queued`，同时 `attempt=0, recover_count=0, lease_owner=NULL, lease_expires_at=NULL, deadline_at=0`（v6 补清 lease 字段），写 task_events(kind='retry')。
- 迁移 SQL 一律 `UPDATE ... WHERE id=? AND status='<期望值>'`（及 epoch 条件），0 行 → `task_invalid_transition`。
- **取消**：`UPDATE tasks SET status='canceled', lease_epoch=lease_epoch+1 WHERE id=? AND status IN ('queued','leased','running','paused')`。

### 5.3 调度、lease、心跳与 fencing

- **调度顺序（v6 定义，防持 lease 等锁死锁）**：worker 先过配额门（§5.6 kind 计数/mutex 的 try 获取，拿不到则跳过该任务扫描下一 queued），**拿到配额后才 lease**；任务结束/中止时先释放 lease 语义（终态 UPDATE）再释放配额。
- **参数**：lease TTL 30s，心跳 10s（≤TTL/3），恢复扫描 30s，优雅退出等待 15s；恢复期上界 60s。任务级兜底：领取时 `deadline_at = now + 2h`，心跳时发现超 deadline → 置 failed（`error_code='deadline_exceeded'`，内部码）。
- 领取（单事务）：`UPDATE tasks SET status='leased', lease_owner=?, lease_epoch=lease_epoch+1, lease_expires_at=?, deadline_at=? WHERE id=? AND status='queued' AND run_after<=?`，读回 epoch。
- 心跳（独立 goroutine，短事务）：`UPDATE ... SET lease_expires_at=? WHERE id=? AND lease_owner=? AND lease_epoch=? AND status IN ('leased','running')`。**0 行或出错 = 立即冻结副作用并退出**，handler 返回 `lease_lost`（内部码）。
- **fenced 写**：worker 每笔写（含进度 UPDATE）在 `InTx` 内先校验 `SELECT status, lease_epoch FROM tasks WHERE id=?`，要求 `status IN ('leased','running') AND lease_epoch=<持有值>`，否则整体回滚。单写连接串行化使校验与写原子。
- **文件副作用顺序**：temp（`temp/<sha1>.part`）→ InTx(epoch 校验 + jar_index 登记 + pack_mods.sha1 回填，不变量 5) → rename。blobstore 方法全程持包级锁（§5.1）。
- 取消协作式：queue 维护 `taskID → cancelFunc`；handler 阶段边界与长循环每 N item 查 `ctx.Done()`。
- **任务日志 fencing（v6）**：日志文件按 `taskId` 持有权写入——非 fenced 追加前必须确认本 worker 仍持 lease（与心跳同一判断），僵尸 worker 不得再写。
- 进度节流：≥500ms 或 Δ≥5%。
- 时钟：lease_expires_at 落库 wall-clock；同进程判活用 monotonic；时钟回拨误判由 fencing 兜底（接受并文档化）。

### 5.4 崩溃恢复

```sql
-- ① 重置失联任务
UPDATE tasks SET status='queued', lease_owner=NULL, lease_epoch=lease_epoch+1,
                 recover_count=recover_count+1, updated_at=?
WHERE status IN ('leased','running') AND lease_expires_at < ?;

-- ② 崩溃预算耗尽置 failed（限 queued，不误杀已重新领取的）
UPDATE tasks SET status='failed', error_code='recovery_exhausted',
                 error_message='lease lost too many times', updated_at=?
WHERE status='queued' AND recover_count >= 10;
```

每条恢复写 task_events(kind='recovered')。业务失败 `attempt+1`、`run_after = now + min(2^attempt, 300)s`。kill-restart 测试注入假时钟，**列入 M1 验收**。

### 5.5 幂等（v6 双轨定稿）

- `payload_hash` = 任务 payload 的 canonical JSON（键排序、无空白、UTF-8）的 sha1。
- **客户端键**：`task_idem_keys` 表**永不删除**（不受 tasks 30 天清理影响，§3.2）。提交时查键：命中且 payload_hash 一致 → 返回原任务（任务行已被清理时返回"任务已过期清理"应答 + 原任务终态摘要，**不重复执行**）；不一致 → `idempotency_conflict`。
- **服务端活跃去重**：`idx_tasks_active_dedup` 部分唯一索引兜底（§3.2）；提交时"查 + 插在同一 InTx"（v6 声明），冲突返回已存在的活跃任务。无包任务按 kind 去重（COALESCE 已处理 NULL）。
- handler 按"至少执行一次"设计：副作用前先查已完成标记。

### 5.6 并发配额与级联

- worker 池 4；`build/publish/import` 各一把 kind-mutex；`download ≤ 3`、`resolve ≤ 2`（任务级，与 §2.1 任务内文件级并发正交）；`cache_gc` 仅活跃任务数为 0 时执行——**运行期互斥**（v6 修"一次性检查"漏洞）：gc 持有调度器暂停标记，新任务提交在 gc 运行期间不入队派发（进 queued 等待）。
- 包删除：service 事务内调 `task.CancelByPack(ctx, tx, packID)`（同事务 epoch+1 + 事件），提交后 queue 对 running 任务调 cancelFunc。

### 5.7 outbox 投递

- dispatcher 每 500ms 批扫 pending，每事件单事务：INSERT activities（origin_event_id UNIQUE 冲突即跳过；kind 直拷）→ 标记 delivered_at。
- 指标：pending 量、最老 pending 年龄、跳过数、投递失败数。

## 6. Provider 适配层（`internal/provider`）

### 6.1 接口与领域结构体（M2 冻结对象，v6 终稿）

```go
type Provider interface {
    Name() string
    Search(ctx context.Context, q SearchQuery) (SearchPage, error)
    GetProject(ctx context.Context, id string) (Project, error)   // id 为平台项目 ID；slug 查询走 Search（v6 声明入参域）
    BatchGetProjects(ctx context.Context, ids []string) ([]Project, error)
    ListVersions(ctx context.Context, projectID string, f VersionFilter) (VersionPage, error)
    GetVersion(ctx context.Context, projectID, versionID string) (Version, error)
    ResolveDownload(ctx context.Context, projectID, versionID string) (DownloadRef, error)
    Download(ctx context.Context, ref DownloadRef, dst io.Writer) error  // v6：文件下载执行体归属 provider
                                                                         //（限流/熔断/重试覆盖）；blobstore 只负责 temp/校验/登记
    LookupByFingerprint(ctx context.Context, refs []FileRef) ([]FingerprintMatch, error)
    ListMCVersions(ctx context.Context) ([]string, error)
    ListLoaders(ctx context.Context, mcVersion string) ([]string, error)  // v6：带 MC 版本维度
}

type Project struct {
    ID, Name, Slug, Summary, Author, IconURL string
    Downloads int64
    UpdatedAt int64
    Extra     map[string]any   // 键强制 "<provider>.<field>" 前缀，适配层单测校验
}

type Version struct {
    ID          string   // 裸平台 ID；复合编码只在落库瞬间由 service 拼（§3.2 pack_mods.version_id）
    ProjectID   string
    Name        string
    MCVersions  []string // CF 适配：gameVersions 扁平数组与 meta 列表求交拆分（v6 写明该机制）
    Loaders     []string // 同上
    Channel     string
    ReleasedAt  int64
    Dependencies []Dependency
    Files        []VersionFile
    Extra        map[string]any
}

type Dependency struct {
    ProjectID string   // 可空：MR 允许 version-only 依赖
    VersionID string   // v6 补：MR 钉死到具体文件的依赖
    Type      string   // required|optional|incompatible|embedded
}

type VersionFile struct { FileName string; Size int64; Sha1 string; Primary bool }
type FileRef struct { Sha1 string; FingerprintCF uint32 }
type FingerprintMatch struct { Ref FileRef; Project Project; Version Version }

type SearchQuery struct {
    MCVersion, Loader string
    Query, Cursor     string   // 不透明游标：base64(provider 原生分页位 + filter 哈希)，
                              // CF index/pageSize 与 MR offset/limit 由适配层互转；filter 变化判 invalid_argument
    Limit             int
}
type SearchPage struct { Items []Project; NextCursor string; Total int64 }

type VersionFilter struct { MCVersion, Loader, Channel, Cursor string; Limit int }
type VersionPage struct { Items []Version; NextCursor string }

type DownloadRef struct {
    URL        string  // 空 = 受限
    Restricted bool    // CF downloadUrl=null
    ManualURL  string  // 人工下载后走 §6.4 上传通道归源（v6 写明后续路径）
    FileName   string
    Size       int64
    Sha1       string  // MR 多 files 时适配层选定 primary 后填入
}
```

- **CF 指纹算法（按官方规范写死）**：MurmurHash2 x86_32，seed = 1，输入为剔除字节 9/10/13/32 后的文件内容。
- 上游 404 映射（v6）：项目/版本不存在 → `project_not_found` / `version_not_found`（404 透传语义，见 §7.2）。
- 受限文件：`Restricted=true` 登记 `conflicts(kind='distribution')`；无人工路径报 `distribution_restricted`。
- 新 provider checklist：实现接口 → 注册 `provider.Registry` → 登记缓存命名空间、限流配置（入 `config.Providers`）、密钥键名 `provider.<name>.api_key` → 补契约 fixture（分页/受限/指纹/meta 样例）。

### 6.2 限流 / 熔断 / 重试（默认值入 config.Providers，可覆盖）

- 限流：CF 5 rps burst 10 + 窗口 200 req/min；MR 4 rps burst 10。排队上限 50，单请求总 deadline 30s，溢出/超时 → `provider_throttled`（502；v6 保留 502 并文档化理由：对客户端而言是"依赖的 upstream 暂时不可用"，非本服务过载；响应带 `Retry-After` 头供前端退避）。
- 429：有 `Retry-After` 从其值，无则默认暂停 60s；refill 减半 5 分钟；不计入熔断。
- 熔断：连续 5 次 5xx/超时 → open 30s → half-open 单探测。open 期间读回退缓存，写返回 `provider_unavailable`。
- **重试按幂等性**：幂等请求（全部 GET + CF `POST /v1/mods`、`POST /v1/fingerprints` 等适配器显式枚举的幂等读）退避 1s/2s/4s 上限 3 次；非幂等写（发布）永不自动重试。
- 优先级：熔断 > 限流 > 重试。

### 6.3 缓存与离线

- TTL：搜索 15min、项目详情 6h、版本列表 1h、版本详情 24h、指纹反查 24h、meta 24h；**过期后保留 24h 宽限**供 stale 回退，宽限过后才由 cache_gc 删除（§3.2，v6 修矛盾）。
- 缓存键含全量参数哈希；并发回源 singleflight；同 sha1 并发下载在 blobstore 锁下后者复用，登记 `INSERT ... ON CONFLICT(sha1) DO NOTHING`。
- 熔断 open/网络失败：未过期直接返回；宽限期内过期返回 `stale:true`；无缓存 `provider_unavailable`。

### 6.4 本地模组导入流程

1. `[写] POST /packs/{packId}/mods/local` 流式上传（独立上限 256MiB；token + 全局导入信号量覆盖上传全程；blobstore 全程持锁，登记 `ON CONFLICT DO NOTHING`）。
2. 落 temp → 计算 sha1 + CF 指纹 → InTx 内查重（不变量 6）+ 登记 jar_index + 插 pack_mods（source='local', project_id=NULL, sha1 直填）→ rename。
3. 可经 `LookupByFingerprint` 归源（回填平台信息到 detail，不改 source）。

## 7. HTTP API 层

### 7.1 端点清单（全部 `/api` 前缀；[写]=需 token+Origin 校验；†=§1.3 豁免副作用 GET，受限速）

```text
GET    /healthz /readyz /health
GET    /dashboard
GET    /packs                                     分页
[写] POST   /packs
GET    /packs/{packId}
[写] PATCH  /packs/{packId}                       含唯一 archived 豁免：解归档
[写] POST   /packs/{packId}/duplicate
[写] DELETE /packs/{packId}
[写] POST   /packs/import                         512MiB 流式；导入信号量覆盖上传全程；读侧超时豁免
†GET   /packs/{packId}/mod-search                 触发上游；进程级限速
†GET   /providers/{provider}/projects/{projectId}
†GET   /providers/{provider}/projects/{projectId}/versions?mc_version=&loader=
†GET   /meta/mc-versions  /meta/loaders?mc_version=
GET    /packs/{packId}/mods                       分页
[写] POST   /packs/{packId}/mods
[写] POST   /packs/{packId}/mods/local            256MiB 流式；信号量同上；读侧超时豁免
[写] PATCH  /packs/{packId}/mods/{modId}
[写] DELETE /packs/{packId}/mods/{modId}
[写] POST   /packs/{packId}/resolve               → task
GET    /packs/{packId}/lock
GET    /packs/{packId}/conflicts                  分页
[写] POST   /packs/{packId}/conflicts/{conflictId}/resolve
GET    /packs/{packId}/delivery-checks            [M4 待定]
[写] POST   /packs/{packId}/builds                → task（幂等 §5.5）
GET    /packs/{packId}/artifacts                  [M4 待定]
[写] POST   /packs/{packId}/publish/{provider}    [M4 待定]
GET    /packs/{packId}/releases                   [M4 待定]
GET    /tasks?cursor=&limit=
GET    /tasks/{taskId}
[写] POST   /tasks/{taskId}/pause /resume /cancel /retry
GET    /tasks/{taskId}/log                        流式；写侧超时豁免
GET    /activities?cursor=&limit=
GET    /settings
[写] PATCH  /settings
[写] PUT    /settings/secrets/{key}               不回显
[写] POST   /settings/providers/{provider}/test
[写] PUT    /settings/export-dirs                 §9.2 登记（写标记文件）
[写] DELETE /settings/export-dirs/{name}
GET    /system/health /system/status /system/metrics /system/audit?cursor=&limit=&action=
GET    /onboarding   [写] PUT /onboarding
[写] POST   /cache/cleanup                        → task
```

成功响应：领域 JSON；列表 `{"items":[...], "next_cursor":"...", "total":null}`。主要写端点请求体字段在 M1 契约测试中逐端点冻结（fixture 即规格，v6 声明载体而不是在本文件内联字段清单）。

### 7.2 错误码完整映射（唯一权威）

```json
{ "error": { "code": "...", "message": "...", "request_id": "...", "details": {} } }
```

| code | HTTP | 触发 |
|---|---|---|
| `invalid_argument` | 400 | 输入校验失败（details 字段级） |
| `unauthorized` | 401 | X-MPack-Token 缺失/错误 |
| `permission_denied` | 403 | authorize 拒绝 |
| `forbidden_origin` | 403 | Origin/Host 校验失败 |
| `not_found` / `method_not_allowed` | 404 / 405 | 路由兜底 |
| `pack_not_found` | 404 | 含跨 pack 越权（不泄漏存在性） |
| `project_not_found` / `version_not_found` | 404 | 上游平台对象不存在（透传语义） |
| `pack_archived` | 409 | 对 archived 包的写（解归档除外） |
| `pack_name_conflict` | 409 | UNIQUE(name) |
| `duplicate_mod` | 409 | 不变量 6 |
| `conflict_unresolved` | 409 | 构建前存在 pending error 冲突 |
| `distribution_restricted` | 422 | 受限文件且无人工路径 |
| `task_not_found` | 404 | |
| `task_invalid_transition` | 409 | 非法迁移/终态操作（显式 retry 除外） |
| `task_kind_busy` | 409 | 独占配额占用中 |
| `idempotency_conflict` | 409 | 同键不同 payload |
| `provider_unavailable` | 502 | 熔断 open 且无缓存 |
| `provider_throttled` | 502 | 上游限流/排队超时（带 Retry-After 头） |
| `provider_auth_failed` | 502 | 凭据被平台拒绝 |
| `provider_not_configured` | 412 | API key 未配置 |
| `quota_exceeded` | 429 | §1.3 豁免 GET 限速命中 |
| `payload_too_large` | 413 | 超端点上限（默认 8MiB；import 512MiB；mods/local 256MiB） |
| `import_unsafe_archive` | 422 | §9.3 防线触发（details 带条目） |
| `disk_full` | 507 | §11 预检失败 |
| `internal_error` | 500 | panic/未分类 |

映射唯一入口 `httpapi.ErrorMiddleware`。**内部码（不经 HTTP 返回，只落 tasks.error_code/日志）可不入表**，当前清单：`lease_lost`、`recovery_exhausted`、`deadline_exceeded`（v6 声明范围规则）。code 只增不改。

### 7.3 请求纪律

- 默认请求体 8MiB；豁免：import 512MiB、mods/local 256MiB（流式 + 全局导入信号量）。
- 分页：`?cursor=&limit=`（默认 20，上限 100）；cursor = base64(filter_hash, sort_key, id)，排序 `(created_at, id)` 降序；filter_hash 不匹配 → `invalid_argument`。§7.1 标"分页"的端点都走此契约；provider 游标封装进同一 cursor 字段。
- `X-Request-ID` 客户端可传，仅 `[a-zA-Z0-9._-]` ≤64 字符，非法重新生成。
- 审计查询见 §7.1；敏感操作审计写失败整体回滚。

## 8. 鉴权与多租户就绪

- `Principal{ID, WorkspaceID, Role}`（当前恒 `local/local/admin`）；`packs.owner_id` 列已预留（§3.2）。接鉴权 = 换中间件 + authorize 实现 + member 权限矩阵 + store 查询条件升级（`owner_id`/`workspace_id`），service 签名零改动。
- Resource 形态 §4 定型；deny 探针守护测试；repository 强制 packID 防 IDOR。
- 未来工作清单（如实）：member 权限矩阵；读路径 authorize；dashboard 跨包聚合多用户口径（重做聚合层）；实例级资源 ownership；token→session 整体替换。

## 9. 安全边界

### 9.1 密钥存储

威胁模型：防"db/备份单独外泄"与"日志/payload 意外记录"；不防同机同用户进程（如实声明）。**平台权限表述（v6 修正）**：0600 仅 POSIX 有效；Windows 上降级文件靠 ACL + 威胁模型接受，文档不作权限承诺。

- KeyProtector：Windows DPAPI（当前用户作用域）→ macOS Keychain → Linux 0600 文件（降级，启动警告）。
- 多 keyId 文件形态：`data/master.key.<keyId>.wrapped`（降级时 `.key.<keyId>` 明文），`data/master.key.current` 记当前 keyId。密文 `v1:<keyId>:<base64 nonce>:<base64 ct>`（AES-256-GCM，96bit 随机 nonce）。
- 两阶段轮换：① 新 key 文件 + 更新 current（旧 key 保留可读）；② 后台任务逐行重加密（每行独立事务），完成删旧 key + `key.rotate` 审计。**重启续跑检测（v6）**：启动时发现 current 之外仍存在 key 文件 → 恢复②的后台任务。
- 主密钥全丢 = secrets 不可恢复，口径"重新录入 API key"。
- API 只回 `{"configured":true,"masked":"tp-cb…"}`；更新不回显。
- 脱敏过滤器：注册唯一入口 `secrets.Write` 覆盖值的原值 + URL 编码 + Base64 形态；**session-token 启动时也注册进同一过滤器**（v6 补）。

### 9.2 路径策略

- `pathpolicy.Resolve(base, userPath)`：拒绝对路径、`..`、符号链接逃逸；落盘 open 用 O_NOFOLLOW 等价物复检（Windows `GetFinalPathNameByHandle`）。
- 写只许 `data/` 与 `allowed_export_dirs`。登记：`PUT /settings/export-dirs`（§7.1 已列）写入标记文件 `.mpackstation-export`，每次导出前复检存在；登记端点受 token 保护。标记可被同机进程伪造——威胁模型内接受，文档化。
- 库中路径存相对 data 的相对路径。

### 9.3 导入防护

归档格式集合：**zip**（mrpack 即 zip）。单文件 ≤512MiB、解压总量 ≤2GiB（边解压边累计，超限即中断）、文件数 ≤50000、逐条目穿越检查、拒绝 symlink/hardlink 条目、压缩比 >100:1 拒绝（**单条目与总量双重判定**，v6）；落盘逐条目过 §9.2 复检。违反 → `import_unsafe_archive` + 条目详情。

### 9.4 监听地址与 LAN

仅回环；`allow_lan=true` 强制：token 覆盖全部端点（含 GET/metrics/audit）+ 启动大字警告 + 声明"LAN 仅限可信网段，token 为静态凭证、无 TLS 传输保护"的风险接受语。缺任一条件 config.Load 拒绝。

### 9.5 浏览器攻击面

1. **启动 token**：128bit 随机，明文写 `data/session-token`（POSIX 0600；Windows 见 §9.1 平台表述）。所有 `/api` 写请求必须携带 `X-MPack-Token`（自定义头强制 CORS 预检）；服务端常数时间比较；错误 → `unauthorized`。
2. **Host 白名单**：仅 `127.0.0.1:*`/`localhost:*`/`[::1]:*`，其他 → `forbidden_origin`。
3. **Origin 校验**：写请求带 Origin 必须等于 `config.FrontendOrigin`；CORS 仅对该 origin 发允许头。
4. **豁免 GET 的残余风险（v6 写明）**：†端点免 token 但受 §1.3 进程级限速（默认 30 req/min）约束，恶意网页可消耗的上游配额被封顶；缓存写入磁盘量同受 TTL+宽限约束。这是接受的残余风险，不是未定义行为。
5. **注入通道**：vite dev proxy 启动钩子读 token 注入；打包形态壳应用读文件注入。§12 集成测试覆盖。

## 10. 可观测性

### 10.1 结构化日志

JSONL → `data/logs/server-YYYYMMDD.log`；写入时日期切换，启动清理 14 天前。固定字段 `ts level msg request_id task_id pack_id component duration_ms`；request-id/task-id 经 ctx 贯穿；脱敏 §9.1。

### 10.2 指标

`GET /api/system/metrics` 固定 schema：

```json
{ "http": {"requests": {"<route_template>": 0},
           "latency_ms_bucket_le": {"<route_template>": {"50": 0, "100": 0, "500": 0, "1000": 0, "+Inf": 0}}},
  "tasks": {"queue_depth": {"queued": 0, "leased": 0, "running": 0, "paused": 0},
            "by_kind": {"<kind>": {"running": 0, "waiting": 0}},
            "terminal_total": {"succeeded": 0, "failed": 0, "canceled": 0},
            "outbox_pending": 0, "outbox_oldest_age_seconds": 0,
            "fencing_rejects_total": 0, "recoveries_total": 0},
  "providers": {"<name>": {"ok_total": 0, "fail_total": 0, "breaker": "closed",
            "breaker_opens_total": 0, "throttled_total": 0, "singleflight_hits_total": 0,
            "latency_ms_bucket_le": {"50": 0, "200": 0, "1000": 0, "+Inf": 0}}},
  "blobstore": {"lock_waits_total": 0, "upload_bytes_total": 0, "download_bytes_total": 0},
  "db": {"open_conns": 0, "slow_queries_total": 0, "slowest_query_1h": "<指纹+耗时>"},
  "gc": {"blobs_deleted_total": 0, "cache_files_deleted_total": 0, "task_rows_deleted_total": 0},
  "process": {"goroutines": 0, "uptime_seconds": 0},
  "disk": {"data_free_bytes": 0} }
```

- bucket 为 le 累计计数；慢查询 driver 包装计时 >100ms 计数，保留**近 1 小时**最慢一条指纹（时间窗滑动，v6）。
- 回环免 token；LAN 强制 token（§9.4）。

### 10.3 任务日志

每任务 `data/task-payloads/<taskId>.log` + task_events 阶段流；`GET /tasks/{id}/log` 流式回读；随任务行 30 天保留期一并清理。

## 11. 文件布局与 GC 闭环

```text
data/
  mpackstation.db  master.key.current  master.key.<keyId>[.wrapped]  session-token
  config.toml  server.lock
  blobs/sha1/<ab>/<sha1>   provider-cache/<provider>/   locks/<packId>/
  task-payloads/<taskId>.{json,log}   artifacts/<packId>/   temp/<sha1>.part   logs/
```

- **blobstore 包级互斥锁**：`RegisterFromTemp`（InTx 登记 + rename）与 `GCDelete`（InTx 删行 + 删文件）全程持锁，两条路径完全串行（v6 持锁主体落地，§5.1）。
- temp 孤儿清理：启动（巡检之后）+ 每日，删 mtime>24h。
- **一致性巡检（启动时 + cache_gc）**：① blobs 有文件而 jar_index 无行 → 删文件；② jar_index 有行而 blobs 无文件 → 按 file_path 找 `temp/<sha1>.part` 重做 rename，找不到则 InTx 内**先 UPDATE pack_mods SET sha1=NULL 再 DELETE jar_index 行**，并同事务写 outbox（kind='system'，不变量 4）；③ payload 文件存在而任务行不存在 → 删文件。
- **blob GC**（持 blobstore 锁）：孤儿判定 → 登记 blob_grace；重新引用时同事务删 grace 行；`orphaned_at > 7d` 且复查仍无引用 → InTx 删 jar_index 行 → 删 blob 文件。
- **包删除磁盘清理**：同事务删业务行后登记清理清单，cache_gc 删 `locks/<packId>/`、`artifacts/<packId>/`、相关 task payload/日志。
- **remote_cache**：过 `fetched_at + ttl + 24h` 的条目由 cache_gc 删文件 + 删行（同事务）。
- **tasks 行**：终态超 30 天由 cache_gc 删行（task 包函数，§1.2 单边纪律）；`task_idem_keys` 不动。
- 磁盘预检：下载/构建前 `data_free_bytes < 需求×2` → `disk_full`。

## 12. 测试策略

- **单元**：repository SQL（内存 SQLite）、状态机全边矩阵、fencing 条件矩阵（含进度/日志写）、pathpolicy 对抗、错误码映射逐行、限流/熔断状态机、脱敏三形态（含 session-token）、KeyProtector 两阶段轮换（含①后崩溃重启续跑②检测）、CF 指纹官方样例向量、canonical JSON payload_hash 稳定性。
- **集成**（临时目录 + 真实 SQLite，恢复/计时注入假时钟）：
  1. kill-restart：一次扫描恢复 queued 并续跑（M1 验收）。
  2. fencing 窗口：lease 过期/取消/恢复/retry 四种时序下过期 epoch 写全部回滚；blobs 无无主文件。
  3. 幂等：同键同参返回原任务；同键不同参 409；任务行 30 天清理后同键重提**不重复执行**（v6 关键用例）；活跃去重并发双提交只建一行；无包任务按 kind 去重。
  4. 迁移链：空库 → latest → quick_check + foreign_key_check 通过 → checksum 通过。
  5. 越权形态：跨 pack 访问 → `pack_not_found`。
  6. 浏览器防护：无 token 写 401、错误 Origin/Host 403、†GET 限速命中 429、前端注入通道端到端可写。
  7. 包删除级联：活跃任务取消、blob 进宽限期、outbox/activities 保留策略下删包不炸、磁盘目录清理。
  8. GC 巡检三分支 + blob GC 与重下载并发（持锁串行）+ remote_cache 宽限期语义（stale 可回退、宽限后删除）。
  9. 求解器冲突 UPSERT：ignored 不复活、resolved 复活 pending、消失冲突置 resolved（v6 新增）。
- **契约**：端点快照（含错误信封，M1 起逐端点冻结请求体字段）；provider fixture（CF 受限文件、MR 多 files、双平台分页游标、指纹反查、meta、CF gameVersions 拆分、MR version-only 依赖）。
- 验收线：全绿 + vet + gofmt + depguard。

## 13. 里程碑

- **M0 地基**：config、instlock、编号迁移（0001=§3.2 全文，旧库作废）、错误信封+码表、探针、结构化日志、pathpolicy、浏览器三防线+†GET 限速+token 注入通道、depguard、连接策略、JSON1 探针、server 超时设计。验收：集成 4/6。
- **M1 看板真实化**：pack CRUD、dashboard 聚合、tasks/activities（outbox）、settings/secrets（KeyProtector 含轮换与续跑检测）、onboarding、audit 查询、任务系统全量（§5 含幂等双轨）。验收：集成 1/2/3/5/7/8 + `USE_MOCK=false` 看板全功能。
- **M2 求解闭环**：provider 双适配器、搜索→加包（含本地上传）→求解→锁快照→下载→JAR 索引。**入口门禁：契约冻结评审**（错误码表 + provider 接口与结构体 + 分页契约）。验收：集成 9。
- **M3 内容与任务书**：M2 冻结评审后按本标准补 schema/接口设计（含 §3.2 新增表同步义务），再动工。
- **M4 构建发布**：交付检查→可复现构建→产物登记→发布；[M4 待定] 端点落库模型届时定稿。

## 14. 决策记录（v2→v6 累计）

1. 任务系统：lease(30s)/心跳(10s)/扫描(30s) + epoch fencing（cancel/恢复 epoch+1，校验含 status，进度/日志写纳入）+ 配额门先于 lease（防持 lease 等锁）+ 计数分离 + retry 清双计数与 lease 字段 + deadline 2h 兜底。
2. 幂等双轨：`task_idem_keys` 独立表永不删除（任务行 30 天清理不影响键语义，清理后重提同键返回"已清理"应答）+ `idx_tasks_active_dedup` 部分唯一索引 + 查插同 InTx + canonical JSON payload_hash。
3. 文件副作用：temp(含 sha1)→InTx 登记+回填→rename；blobstore 包级锁持锁主体为 blobstore 方法内部；巡检三分支先于 temp 清理；巡检修复写也过 outbox（不变量 4 无例外）。
4. 密钥：KeyProtector 多 keyId 文件 + 两阶段轮换 + 启动续跑检测 + 全形态脱敏（含 session-token）。
5. 浏览器：三防线 + †GET 豁免副作用枚举 + 进程级限速封顶残余风险 + token 明文 0600（Windows 权限不实承诺已删）+ 常数时间比较。
6. 鉴权：Principal + Resource 定型 + `packs.owner_id` 预留列 + deny 探针；读路径/dashboard 多用户成本如实列未来工作。
7. provider：十方法接口（Download 执行体归属 provider）+ Dependency.VersionID + CF gameVersions meta 求交拆分 + meta 方法带 MC 版本维度 + 上游 404 透传码 + 重试按幂等性 + CF 指纹参数写死 + 限流数值入 config.Providers。
8. DDL 即语义：联动 CHECK、反向 GLOB、origin_event_id 快照列无 FK、conflicts UPSERT 列清单（ignored 不复活/resolved 复活 + created_at 不覆盖）、全表保留策略 + 新增表同步义务 checklist。
9. 错误码表全量 + 内部码范围规则；请求体分档（8MiB/256MiB/512MiB）+ server 超时逐端点设计；全局时间戳 unix ms。
