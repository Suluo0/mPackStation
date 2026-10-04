# 数据库与后端架构开工前评审（高级开发侧）

> 评审身份：高级后端架构开发 / 评审对手
>
> 评审范围：`docs/architecture/backend-architecture-v7.md`、`docs/standards/development-standards.md`、`docs/standards/test-acceptance-standards.md`、`docs/standards/development-priority.md`、当前 `apps/server` 数据库与 HTTP 结构，并对照前端现有 zod/API 契约。
>
> 本报告是 DBA 评审的另一条独立意见；本阶段只读，不修改运行代码，不提交业务实现。

## 1. 结论先行

当前产品方向和 v7 分层方向是合理的，SQLite 单库、WAL、JAR 内容寻址共享、追加式 revision、任务 lease/fencing、Provider/blobstore 边界都可以支撑 P0–P7。

但是，当前还不能直接把 v7 当作“可执行数据库基线”开工。原因不是功能缺失，而是数据库闭环和前后端契约仍有几处互相矛盾或缺少落点：

1. v7 只给出部分领域表 DDL；它要求的核心表（`pack_locks`、`tasks`、`task_events`、`task_idem_keys`、`outbox_events`、`activities`、`conflicts`、`audit_events`、`secrets`、`blob_grace` 等）没有完整字段、索引和约束，不能直接生成一套可迁移数据库。
2. 当前 `schema.sql` 是一次性 `IF NOT EXISTS` 的 v1 基线，不是 v7 migration runner；没有 checksum、逐迁移应用、失败恢复或 schema 漂移检测。
3. 当前 schema 与 v7 的数据不变量不相容，最明显的是 `pack_mods.sha1 NOT NULL DEFAULT ''`，而 v7 巡检要求丢失 blob 时将它置为 `NULL`。
4. 前端契约与 v7 的列表 envelope、任务状态映射、导入响应和 import source 枚举没有最终统一；现在实现会在真实接口接入时产生 zod 解析失败或错误的状态展示。
5. 包归档、锁快照条目、同包归属的复合一致性等业务约束尚未有数据库或 service 可验证的完整方案。

因此建议：先完成“数据库/契约冻结修订 + 可执行 migration 设计评审”，再开始 P2 框架与数据库实现；P0/P1 的工程脚本、文档和干净目录验证可以并行，但不得假设现有 v1 schema 是最终起点。

## 2. 反对意见（需要在架构定稿前回答）

### 2.1 v7 的核心 schema 不是可执行规格

第 5.2 节使用“初始 migration 必须创建……”列出核心表，但只展示了 v7 新增领域表的 SQL。以下关键表没有完整 DDL：

```text
packs（含 current_version_id 的最终定义）
pack_mods
jar_index
pack_locks 及其不可变快照条目
tasks / task_events / task_idem_keys
outbox_events / activities
conflicts / audit_events
secrets / settings / remote_cache / blob_grace
schema_migrations（checksum 字段及迁移状态）
```

缺少这些定义时，无法证明：字段类型、默认值、CHECK 枚举、外键动作、部分唯一索引、清理字段、任务租约字段、幂等 payload hash 和 outbox 投递字段是否与后文一致。必须补一份完整的 0001 DDL（或明确的多个有序 migration），不能只保留表名清单。

### 2.2 追加式锁快照没有承载构建所需的事实

v7 要求 `pack_locks` 不可变、构建绑定 lock，但没有定义 lock 的快照载荷或子表。若构建仍读取可变的 `pack_mods`，则“锁定”不具备复现意义。

必须明确以下至少一种方案：

- 推荐：新增 `pack_lock_mods`（每个 lock 的 project/version/source/file_name/sha1/required 等）及需要的依赖快照表；
- 或者：在 `pack_locks` 保存规范化、版本化、不可变的 JSON snapshot，并保存 canonical hash，构建只读取该 snapshot。

无论采用哪种方案，`pack_versions.lock_id`、lock 与 pack 的归属、snapshot hash 和构建 fingerprint 都必须能被 service/repository 验证。仅有一个 `pack_locks.id` 外键不够。

### 2.3 SQLite 外键只能覆盖一部分“同包”不变量

以下关系在文档中要求“service 事务内校验属于同一包/同一 revision”，但 DDL 本身没有复合外键或唯一支持：

- `pack_versions.lock_id` 与 `pack_locks.pack_id`；
- `delivery_checks.pack_id` 与 `pack_version_id`；
- `artifacts.pack_id` 与 `pack_version_id`；
- `releases.pack_id` 与 `pack_version_id/artifact_id`；
- `mod_dependencies.pack_id` 与 `from_pack_mod_id`；
- `quest_nodes.revision_id` 与 `chapter_id`；
- `quest_edges.revision_id` 与两端 node。

这可以由 service 强制，但必须在架构中明确“哪些不变量故意只由 service 保证、为何不使用复合 FK”，并为每个边界写 repository/service 负例测试。否则一个漏掉的写入口就会产生跨包污染。

### 2.4 revision 的“唯一 active/applied”没有数据库保障

`content_revisions` 和 `quest_revisions` 允许同一文档/任务书有多行 `state='applied'`；`active_revision_id` 也没有 FK。后文要求 active 必须指向同文档的 applied revision，但目前只能靠约定。

至少需要：

- `UNIQUE(document_id) WHERE state='applied'` 和 `UNIQUE(quest_book_id) WHERE state='applied'` 的部分索引；
- active 引用的事务更新规则；
- `active_revision_id` 缺失、跨文档、指向 draft 的启动巡检/repair 行为。

若 SQLite 版本或复合 FK 方案不适合直接约束，也必须把 invariant check 做成可执行的 repository 测试和启动检查，而不是只写在 prose 中。

### 2.5 当前列定义与后文操作直接冲突

当前 [schema.sql](../../apps/server/internal/store/schema.sql) 的 `pack_mods.sha1 TEXT NOT NULL DEFAULT ''` 与 v7 第 7.3 节“索引无文件时将引用它的 `pack_mods.sha1` 置 NULL”矛盾。应改为可空，并将“未下载/损坏/缺失”的含义从空字符串统一为 `NULL`；同时为 `pack_mods.sha1` 增加对 `jar_index(sha1)` 的 FK（或明确为何由巡检事务手工维护）。

同类问题还包括：

- 当前任务状态有 `success`，v7 领域状态是 `succeeded`；
- 当前任务 kind 注释只有 `pack/index/sync/cache`，v7 需要 `resolve/download/index/build/publish/import/cache_gc`；
- 当前 `activities` 只有 `kind/text`，但 v7 要求领域 `kind + action`、request/task/pack 关联和 outbox 来源；
- 当前 `packs` 没有归档状态字段，但 v7 提供 archive/unarchive API；
- 当前旧 schema 没有 `current_version_id`，而 v7 要求它存在。

这些不能留到各模块自行解释，必须在 0001/0002 migration 和领域类型中统一。

### 2.6 `pack_versions.version` 的 SQLite GLOB 检查不等于 semver

`CHECK (version GLOB '[0-9]*.[0-9]*.[0-9]*')` 中 `*` 是任意字符，`.` 只表示字面点；它允许诸如 `1abc.2.3` 一类非规范字符串。建议由 service 使用严格的 SemVer 解析器并保存 canonical 值，数据库 CHECK 只承担非空/字符集的底线；若产品只接受 `MAJOR.MINOR.PATCH`，应在 migration 中采用可证明的组合 CHECK 或明确将严格性放在 service 并配套负例测试。

### 2.7 nullable unique 的语义未定

`delivery_checks` 定义 `UNIQUE(pack_id, pack_version_id, kind)`，但 `pack_version_id` 可空。SQLite 对 NULL 不视为相等，因此同一个包级（无版本）kind 可以插入多行。需要二选一：

- 包级检查和版本级检查拆成不同表/不同唯一约束；或
- 使用规范化 sentinel/表达式唯一索引，并在 service 固定 null 语义。

同样要为 `pack_alerts.source_ref=''`、空 provider id 等“空值是否代表同一对象”的语义写清楚。

### 2.8 单机身份边界的文档口径仍有冲突

v7 开头称“保留账号、会话、成员、角色和协作能力”，但第 0.2、5.1 和项目 `AGENTS.md` 又明确当前不做本地账号管理，只保留未来 GitHub identity/Principal 入口。用户当前要求也是先做单机版本、后续再接 GitHub 接口。该冲突虽不影响表结构立即执行，却会导致开发者误建 `accounts/sessions/members` 表或提前实现登录流程。

必须在 v7 顶部统一为：当前仅本机启动 token + `Principal(local)`；不建本地账号、session、成员、角色或 tenant/workspace 表；未来 GitHub identity 只通过独立适配器接入，并另行评审其协作/授权数据模型。数据库评审不应把未来身份表混入当前 0001。

## 3. 前端/API/读模型兼容性问题

### 3.1 列表 envelope 与现有 zod 不一致

v7 第 6.1 节规定所有列表返回：

```json
{"items": [], "next_cursor": null, "total": null}
```

但当前前端 `taskListSchema`、`activityListSchema`、`mcVersionListSchema` 都直接解析数组，且 dashboard API 也直接把真实响应交给数组 schema。必须选定一个权威策略：

- 推荐保留 v7 envelope，并在 `apps/web/src/api` 统一 adapter 后向组件暴露数组；
- 或者为明确的轻量静态列表保留数组，但需要在 v7 契约中列出例外端点。

不能让每个 endpoint 自由决定，否则分页、错误和 zod 适配会分叉。

### 3.2 任务状态和 kind 映射不完整

前端允许：

```text
type: index-mod | build-pack | import-pack | update-preflight
status: running | success | failed | cancelled | paused
```

v7 领域任务还包括 `download/publish/cache_gc`，状态包括 `queued/leased/running/succeeded/failed/canceled`。文档目前只明确 `canceled → cancelled`，没有明确：

- `succeeded → success`；
- `leased/queued` 是否映射为 running、隐藏，还是增加前端状态；
- dashboard 是否过滤 download/publish/cache_gc，或增加“other”展示类型；
- `error`、`startedAt` 在 queued 任务时的 null 语义。

建议在 API adapter 章节建立完整双向映射表，并为每个领域状态写 fixture；未经统一，真实任务列表不能验收。

### 3.3 导入输入和响应与前端当前实现不一致

v7 规定 `source=curseforge_url|modrinth_url|local_zip`，inspect/confirm 分两阶段，确认返回 `202 {importId,taskId,packId}`。当前前端输入仍是 `curseforge|modrinth|local`，并按 `packSchema {id,name}` 解析 `POST /api/packs/import` 的响应。

这是设计冲突，不应由后端偷偷兼容多个含义。请在 `api-contracts.md` 固定：source 枚举、multipart/URL 输入、preview token、202 响应、任务轮询和失败显示；然后让前端 adapter 负责迁移。

### 3.4 Dashboard 聚合字段缺少一致性定义

`modCount`、`edits`、`alerts`、`todayResolvedCount` 的来源需要写成可执行查询语义：

- `pack_mods` 的 total/installed/selected 是否排除 removed/pending；
- content/quest 数量是 document 数、active revision 数还是 draft 数；
- `todayResolvedCount` 是冲突从 pending→resolved 的转移数，还是所有成功 validation run；重复校验不得重复计数；
- `lastEditedPackId` 在并列时间下的稳定 tie-break；
- crash/updatable 的“未知”和“0”如何区分。

当前 `pack_alerts` 可以承载 crash/update 摘要，但没有 crash report 索引表；若暂时返回 0，必须在契约中明确它是“未接入来源”而不是“已确认没有崩溃”。

### 3.5 health/status 不应隐式触发上游副作用

前端 health/status 是 GET，而 v7 又规定 provider test 使用显式 POST、GET 不偷发上游请求。应明确 health/status 读取最近探测缓存/状态，而不是每次 dashboard 加载实时请求 CurseForge/Modrinth；否则页面刷新会消耗配额并破坏限流设计。

## 4. 必须修改项（开工阻断）

以下项目完成并经过评审前，不建议进入依赖业务的 P2–P7：

### A. 发布完整数据库基线

1. 补齐 0001（或有序 0001–000N）全部核心表 DDL、字段说明、CHECK、FK、索引、保留策略和清理责任。
2. 明确 `pack_locks` 的不可变 snapshot 载荷/子表、canonical hash 和构建读取路径。
3. 把 `packs` 归档状态、`current_version_id`、pack/version/lock 归属和所有任务租约字段纳入 DDL。
4. 将 `pack_mods.sha1` 改为可空，统一 NULL/空字符串语义，并定义 jar_index FK/巡检修复流程。
5. 为 revisions 的唯一 applied、quest 同 revision、delivery check nullable unique 等边界补上索引或可执行 service invariant。
6. 对任务、outbox、activity、audit、secret、remote cache、blob grace 给出与后文一致的完整字段，而非仅列表名。

### B. 实现真正的 migration 闭环

1. migration 文件嵌入 binary，编号单调，已应用文件禁止修改。
2. `schema_migrations` 至少保存 version/name/checksum/applied_at；启动校验 checksum，拒绝缺失、篡改、跳号或未来版本。
3. 每个 migration 在短事务中执行；失败时不 ready，并能在修复后重新启动续跑。
4. 对旧 v1 数据明确迁移路线（含 `success→succeeded`、空 sha1→NULL、旧列补齐、数据回填/拒绝条件），不能简单删除旧数据库。
5. 每次迁移后运行 `PRAGMA quick_check` 和 `PRAGMA foreign_key_check`，并验证关键不变量查询。
6. 读/写连接若分离，两个连接都必须启用 `foreign_keys`、busy timeout 和 WAL 相关策略；写事务只能使用写连接。

### C. 冻结 API/前端适配契约

1. 明确列表 envelope 及例外端点，并实现统一 adapter。
2. 发布完整 task kind/status 映射（含 queued/leased/succeeded、publish/download/cache_gc）和 null 字段规则。
3. 发布 import inspect/confirm 的输入、202 响应、source 枚举和 zod fixture。
4. 发布 dashboard 聚合字段的 SQL/业务定义，以及 health/status 的缓存探测语义。
5. 错误 envelope 必须包含 v7 要求的 `request_id` 和 `details`；当前实现的简化错误体不能作为最终契约。

### D. 让单实例边界达到文档承诺

1. 当前 PID 文本锁不是内核持有的 OS 文件锁；需决定改为 Windows `LockFileEx`/Unix `flock` 等真正的持锁句柄，或在 v7 中明确 PID 锁的接受风险与验证方式。
2. 锁、数据库、日志、blob、导入和导出路径全部由同一绝对 `DataDir` 解析；不能依赖 `main.go` 当前的相对默认路径。
3. 启动顺序要可测试：配置→目录→实例锁→写连接→迁移/检查→读连接→blob/temp 巡检→任务恢复→listen。

### E. 消除身份与数据域歧义

1. 删除 v7 开头关于当前“保留账号、会话、成员、角色和协作能力”的表述，改成与 `AGENTS.md` 一致的单机 token 边界。
2. 明确 `Principal` 是未来扩展接口，不代表现在必须创建身份表；P8 之前不得引入本地账号管理或协作表。

## 5. 可延期项（不阻断 P0/P1，但必须登记）

这些事项不应阻止工程基座、构建脚本、部署 smoke 和迁移 runner 开发，但在进入对应里程碑前必须补齐：

- Provider 的真实 CF/MR 差异、fixture 规模和发布远端状态细节（进入 P5/P7 前）；
- crash report 专用索引领域（若 dashboard 先稳定返回 0，进入导入/交付前补充）；
- GitHub identity/OAuth/device-flow 的实际实现；当前只需保留不泄漏 token 的 Principal/identity 接口入口；
- metrics 导出协议和具体监控后端；结构化日志、request/task/pack 关联及关键计数应先落地；
- 内容/任务书 JSON schema 的所有业务字段和导出适配器（进入 P6 前）；
- artifact 去重策略、发布幂等状态查询和跨平台安装器细节（进入 P7 前）；
- 高并发读优化、读模型物化和历史数据归档，在真实数据规模证明需要前不提前复杂化。

## 6. 建议的数据库分层与 migration 顺序

为避免 P2 时一次性解决全部领域，建议将可执行 migration 拆成以下顺序，每一步都可独立启动和回滚验证：

```text
0001_core_runtime
  schema_migrations、packs、settings、secrets、audit_events

0002_pack_mods_blobs
  pack_mods、jar_index、blob_grace、remote_cache

0003_tasks_events
  tasks、task_events、task_idem_keys、outbox_events、activities、conflicts

0004_pack_locks_versions
  pack_locks、pack_lock_mods（或 snapshot 载荷）、pack_versions、pack_alerts、pack_mod_updates

0005_content_quest
  content_*、quest_*，并建立 applied/active invariant

0006_delivery_release
  delivery_checks、artifacts、releases、allowed_export_dirs

0007_onboarding_indexes
  onboarding_state、聚合查询所需索引、历史数据回填和 checksum 固化
```

实际编号可调整，但不能把互相依赖的所有表塞进一个未验证的巨型 `schema.sql`。若采用单一 0001，仍需在代码库提供同等粒度的 SQL/测试分段。

## 7. 与 DBA 讨论时的必问问题

1. 是否接受同包一致性主要由 service 保证，还是愿意通过复合 FK/复合唯一索引加强数据库保护？每个选择的维护成本和 SQLite 限制是什么？
2. lock snapshot 采用规范化子表还是 canonical JSON？如何让构建不依赖可变 `pack_mods`？
3. revisions 的 active/applied 是否允许数据库部分索引？如果不允许，启动 repair 如何保证不会自动选错版本？
4. pack 归档是 `archived_at`、`status` 还是软删除状态机？删除/归档与运行中任务的关系如何在事务内表达？
5. delivery check 的包级/版本级唯一性如何实现？NULL 是否有明确业务语义？
6. 是否采用独立读连接池？在现代 SQLite 驱动下如何保证每个连接都启用 FK/WAL/busy timeout？
7. v1 现有数据是否需要升级兼容？升级失败、备份失败和不可逆字段变更的处理边界是什么？

## 8. 开工前验收条件

高级开发侧建议把以下条件作为“数据库合理性评审通过”的硬门槛：

- [ ] 一份可执行的完整核心 + v7 DDL 已评审，所有表名、字段、枚举、外键、索引和保留策略可追溯。
- [ ] `pack_locks` snapshot 能独立重建构建输入；修改 `pack_mods` 不会改变历史 lock。
- [ ] 关键同包/同 revision invariant 有数据库约束或明确的 service 断言，并有成功与负例测试计划。
- [ ] NULL/空字符串、status/kind 枚举、归档/删除、active/applied revision 的语义已冻结。
- [ ] migration runner 能在空库、当前 v1 库、重复启动、checksum 篡改、失败重试场景下给出确定结果。
- [ ] API adapter 已决定列表 envelope、task 状态/kind、import 202 和错误体；前端 zod fixture 可以解析真实契约。
- [ ] Dashboard 字段的来源和重复计数规则已写成可执行查询或 service 规则。
- [ ] 启动、实例锁、数据库连接、迁移检查和读写路径在干净临时目录可验证。
- [ ] DBA 与高级开发对上述阻断项达成书面结论；未达成前只做不依赖最终 schema 的工程脚本/文档工作。

## 9. 评审意见等级

| 项目 | 等级 | 处理意见 |
|---|---:|---|
| v7 核心 DDL 缺失 | P0 | 必须先补齐并评审 |
| migration/checksum/升级路线缺失 | P0 | 必须先补齐并做空库/旧库 smoke |
| lock snapshot 未定义 | P0（P5/P7 前） | 必须在模组/构建开发前冻结 |
| sha1 NULL 冲突、任务/活动旧枚举 | P0（数据库基线） | migration 与契约一起修正 |
| 列表 envelope、import、任务映射不一致 | P0（真实 API 前） | 先冻结 adapter 和 fixture |
| revisions/同包复合不变量 | P1 | 进入对应领域开发前必须有约束/测试 |
| dashboard 聚合/health 缓存语义未定 | P1 | 进入 E1 真实看板前冻结 |
| Provider 细节、GitHub、metrics 后端、安装器细节 | P2/可延期 | 按 P5/P7/P8 阶段补齐 |

## 10. 最终建议

当前数据库思路“单库 + SQLite WAL + pack 分域 + SHA-1 blob 共享”可以保留，不建议因为这些问题更换数据库或引入多租户模型。需要做的是把 v7 从架构叙述提升为可执行规格：补齐核心 DDL、明确锁 snapshot、修正 NULL/枚举/归档语义、建立 migration 闭环，并先统一前端契约。

在这些事项完成并由 DBA 与高级开发签字前，项目不应把当前 `apps/server/internal/store/schema.sql` 视为可继续叠加业务的稳定地基；它只能作为历史 v1 原型，迁移时必须显式处理而不是静默覆盖。

## 11. 交叉讨论结论（与 DBA 报告逐项对照）

已阅读 [database-dba-review.md](database-dba-review.md)，高级开发侧与 DBA 的结论如下。两份报告没有发现会迫使项目更换 SQLite 或引入 tenant/workspace 的问题；分歧主要是实现节奏，而不是数据原则。

| DBA 议题 | 高级开发侧意见 | 最终取舍 |
|---|---|---|
| canonical v7 DDL 缺失 | 完全同意。这是 P0，不能由实现者自行拼接 v6/v7/当前 schema。 | 先冻结一份可执行 migration manifest/DDL；`schema.sql` 只作为迁移生成或校验产物。 |
| current/lock/version 循环与跨包引用 | 完全同意不保留双向 nullable 指针。普通单列 FK 无法阻止跨包引用。 | 采用 `pack_current_version(pack_id PK, pack_version_id)` 指针表；`pack_versions` 通过 `(pack_id, lock_id)` 复合 FK 指向 `pack_locks(pack_id,id)`；删除 `pack_locks.pack_version_id` 反向列。该取舍需要同步修改 v7 第 5.2/5.3 节。 |
| revision active/applied 与构建输入快照 | 完全同意。没有快照，artifact fingerprint 不能证明输入。 | 增加 applied 部分唯一索引、同域 FK/受控指针，并增加 `pack_version_inputs`（或等价 canonical manifest），记录 lock、content/quest revision、构建配置和输入 hash。 |
| local 模组 NULL sha1 与部分唯一 | 完全同意。当前空字符串方案与巡检和并发去重都冲突。 | `sha1` 可空；非 local 用 `(pack_id,source,project_id)` 部分唯一，local 用 `(pack_id,sha1)` 且仅 sha1 非 NULL 生效；迁移时规范化空字符串。 |
| import preview token 持久化 | 完全同意。只放内存会在重启/并发确认时失去一次性保证。 | 增加 `import_previews`（token_hash/input_hash/source/staged handle/expiry/consumed_at）；confirm 在短事务内消费并通过 Idempotency-Key 去重。 |
| task/outbox/idem 与删包生命周期 | 完全同意。任务 30 天留存、幂等键永久留存与级联删包不可同时使用。 | 活跃任务先 fencing + cancel；tasks/task_events/outbox 不因 pack cascade 静默消失，改 `SET NULL` 或无 pack 外键的 system 事件；永久幂等键增加长度/速率/占用上限。 |
| v1 升级与 checksum runner | 完全同意。当前 version=1 不能被静默替换。 | 默认安全升级；若当前 v1 被认定为未发布开发基线，只能提供带绝对路径确认的显式重建命令，不得由普通启动自动丢弃数据。 |
| 建议 0001–0008 顺序 | 原则同意，但不要求一次性实现所有领域模块。 | 先冻结全局顺序和依赖；P2 只落 core/runtime + migration，进入 P4/P5/P6/P7 前再落对应分层 migration。已发布编号和 checksum 不变。 |

### 11.1 对 migration 编号的补充取舍

当前工作树已经写入 `schema_migrations.version=1`，因此不能把一个内容不同的“v7 新基线”重新命名为版本 1。推荐采用以下兼容政策：

1. 将当前 8 表形状视为历史 `0001_legacy_baseline`，固定其 checksum；它不是新的业务契约。
2. 新数据库和旧 v1 数据都由同一个 runner 走到最新版本；`0002_v7_core` 负责表重建、NULL/枚举规范化和新核心列补齐，后续编号按领域递增。
3. 如果项目明确当前数据库从未发布且允许丢弃，另提供 `--rebuild-dev`：先备份/移动指定绝对路径，再从 v7 基线初始化；该命令不能在普通启动路径触发。
4. 无论采用安全升级还是显式重建，空库、当前 v1、重复启动、checksum 篡改、migration 失败回滚和高版本拒绝都必须有可重复测试。

这样既保留了 DBA 要求的可审计升级，又避免为了“看起来是 0001”而篡改已存在的版本事实。

### 11.2 最小可开工数据库基线

高级开发侧建议把“允许开始 P2”与“允许进入 P3–P7”分开，不把未使用的领域表提前写成半成品：

**开始 P2（框架/迁移实现）前必须冻结并可执行：**

- migration manifest、checksum、失败/高版本策略和 v1 升级政策；
- `packs`（归档/删除状态、MC/loader、时间字段）以及 `settings/secrets/audit_events`；
- `schema_migrations`、SQLite 每连接 PRAGMA、quick/foreign-key check；
- `tasks/task_events/task_idem_keys/outbox_events/activities` 的字段、状态、租约、幂等和删除策略；
- `pack_mods/jar_index` 的 NULL sha1、部分唯一和 blob 引用语义；
- `Principal(local)`/启动 token 只作为 service 授权入口，不创建账号、成员或租户表。

**开始 P3（真实 Pack/Dashboard）前还必须完成：**

- `pack_versions` + `pack_current_version` 指针及 dashboard 聚合字段定义；
- `conflicts/pack_alerts/pack_mod_updates` 的 fingerprint、状态和统计语义；
- 列表 envelope、task/activity adapter、错误 envelope 的前端 fixture。

**进入 P4–P7 前按阶段追加硬门槛：**

- P4：`import_previews`、导入阶段/文件句柄、任务恢复和 zip/SSRF 安全测试；
- P5：`pack_locks` snapshot、`pack_lock_mods`（或 canonical snapshot）、`mod_dependencies` 与 blob 两阶段测试；
- P6：content/quest revision 同域约束、applied 唯一、validation/rollback；
- P7：`pack_version_inputs`、delivery check 输入 hash、artifacts/releases 幂等与发布恢复。

这个分层不是降低 DBA 的要求：所有后续表的字段和跨表原则必须在架构层冻结，代码实现可以按里程碑逐步落 migration，但不能先用当前 v1 表“占位”再事后补约束。

### 11.3 联合开工意见

高级开发与 DBA 的联合意见为：

> 数据库方案可行，但当前实现基线和 v7 文档还未达到 P2 数据库开发的开工条件。先完成 canonical DDL/迁移政策/跨表不变量/契约映射的冻结与验收；在此之前只允许构建脚本、测试 harness、migration runner 设计和不依赖业务 schema 的工程工作。P3–P7 必须依次满足对应阶段门槛，不得跨越未解决的 lock、task、文件或恢复语义。

另外，v7 顶部“保留账号、会话、成员、角色和协作能力”的表述必须与当前单机 token、未来 GitHub identity 入口统一；这项文档修正与数据库基线同属开工前 P0，避免实现者误建身份或多租户表。
