# 数据库 DBA 审查报告（v7 开工前）

> 审查角色：DBA / 数据一致性负责人
>
> 审查范围：`docs/architecture/backend-architecture-v7.md`、`docs/architecture/backend-architecture.md`（v6 参考稿）、`docs/standards/development-standards.md`、`docs/standards/test-acceptance-standards.md`、`docs/standards/development-priority.md`，以及 `apps/server/internal/store/*`。
>
> 本报告是开工前的只读审查结果。当前没有修改业务代码、schema 或迁移文件。

## 1. 结论

v7 的数据方向是合理的：单 SQLite、按 `pack_id` 组织业务数据、JAR blob 按 SHA-1 共享、追加式 lock/revision、任务幂等与 fencing、outbox 和文件巡检，这些原则可以支撑 P0–P7。

但是当前还不能直接按 v7 开始写 repository/service。原因不是表数量不够，而是“唯一权威 DDL、旧库升级路径和跨表不变量”尚未冻结。现在存在三套互相不完全一致的事实来源：

1. v7 只给出“核心表 + 追加表”的组合说明，没有一份可直接执行的完整 v7 DDL。
2. v6 参考 DDL 与 v7 在 `owner_id`、`current_lock_id/current_version_id`、任务删除策略和 `pack_locks.pack_version_id` 等处存在语义差异。
3. 当前工作树仍是 v1 的 8 张表基线，`schema_migrations` 只有 `version/applied_at`，没有 checksum，也没有正式 migration runner。

**允许开工的范围：** 可以先做不依赖业务表结构的构建脚本、测试脚手架、迁移 runner 设计和 fixture；不得把现有 v1 schema 当作稳定契约继续实现 P3–P7 业务。

**进入 P2 数据库实现前的硬条件：** 见第 7 节。未完成这些条件，任何“先实现接口、以后补约束”的做法都会把跨包归属、锁定快照和恢复语义写死在错误的表结构上。

## 2. 当前基线核对

### 2.1 当前 SQLite 基线

`apps/server/internal/store/schema.sql` 目前包含：

- `packs`
- `pack_mods`
- `jar_index`
- `tasks`
- `conflicts`
- `activities`
- `settings`
- `remote_cache`

当前基线的主要缺口：

- `packs` 没有 `status/current_version_id/current_lock_id`。
- `pack_mods` 没有 source CHECK、启停/固定字段、唯一去重、`sha1 → jar_index` 外键；未下载值是空字符串，不是 v7 规定的 `NULL`。
- `tasks` 没有 kind/status/progress/attempt/lease/epoch 的 CHECK，也没有活跃去重索引。
- `conflicts` 没有 fingerprint、updated_at 和幂等唯一约束。
- `activities` 没有 origin event 去重列，也没有 detail JSON。
- 没有 `pack_locks`、`task_events`、`task_idem_keys`、`outbox_events`、`audit_events`、`secrets`、`blob_grace`、v7 内容/任务书/构建发布表。
- `store.go` 将 schema 字符串作为一次性 baseline 执行，当前版本号固定为 1；不存在编号迁移、checksum、`quick_check`/`foreign_key_check` 或读写连接分离。

### 2.2 v7 设计中已经正确的部分

- 业务作用域是整合包，不引入 tenant/workspace 数据隔离。
- `pack_mods` 是包内已选择模组的唯一权威来源。
- JAR 文件使用全局内容寻址，包只保存引用。
- 任务有 lease、heartbeat、`lease_epoch` fencing、客户端幂等键和活跃任务去重两条防线。
- 内容/任务书/lock/pack version 采用追加式 revision/snapshot 思路。
- 看板敏感写入通过 outbox，活动流不直接依赖 outbox 的短留存。
- 文件遵循 temp → 校验 → 短事务登记 → rename，并由巡检/宽限期补偿。

这些原则可保留；问题在于还需要把它们变成可以被 SQLite 强制或被 service 明确验证的具体约束。

## 3. P0 风险（开始数据库和业务实现前必须解决）

### P0-1：没有唯一权威的 v7 DDL，迁移无法审计

v7 表 5.2 只说“创建 v6 核心表以及本节新增表”，而不是完整 `0001_init_v7.sql`。v6 又是保留不改的参考文档，当前 `schema.sql` 仍是 v1。实现者必须自行拼接三份内容，极易出现字段、CHECK、外键和保留策略不一致。

**必须修改：** 在架构文档或数据库文档中给出一份可执行的完整 v7 DDL；`schema.sql` 不再作为独立真相，改为迁移生成/校验产物。所有表、索引、触发器、部分唯一索引、保留责任和兼容策略必须来自同一版本清单。

### P0-2：current 指针和 lock/version 关系存在循环及跨包引用风险

文档同时出现 `packs.current_lock_id`、`packs.current_version_id`，并要求 `pack_locks` 可带 `pack_version_id`、`pack_versions` 又带 `lock_id`。若全部做外键，会形成循环；若只做普通单列外键，则允许 A 包的 version 指向 B 包的 lock。v7 目前只要求 service 在事务中校验“属于同一包”，数据库本身无法防止误写。

**必须修改：** 选一个规范化关系并写入最终 DDL。推荐：

- `pack_versions(pack_id, id)` 建复合唯一键；`pack_versions.lock_id` 使用 `(pack_id, lock_id) → pack_locks(pack_id, id)` 复合外键。
- 删除 `pack_locks.pack_version_id` 反向列；通过 `pack_versions` 查询归属，避免循环。
- current version 用单独的 `pack_current_version(pack_id PRIMARY KEY, pack_version_id)` 指针表，或使用带复合外键的单向指针；不要用两个互相引用的 nullable 列。
- 所有 pointer 更新、lock 创建和 outbox 写入放在同一短事务中。

若决定只在 service 校验而不在数据库加复合 FK，必须有 ADR 说明原因，并把跨包伪造测试列为 P0 门禁；默认不建议接受。

### P0-3：revision/lock/build 不能证明可复现

当前 v7 的 revision 表只保存 payload 或子表，`active_revision_id` 没有 FK/同文档约束；同一个 document/book 可以有多个 `applied` revision；`source_revision_id` 也可以指向另一个 document。`pack_versions` 只有一个 `lock_id`，没有记录构建时采用的内容 revision 和 quest revision。这样即使 artifact 有 `source_fingerprint`，也无法从数据库重建其精确输入。

**必须修改：**

- `active_revision_id` 使用同文档/同任务书的复合 FK，或用受控指针表；增加“每个 document/book 至多一个 applied revision”的部分唯一索引。
- `content_revisions.source_revision_id` 强制同 document；quest chapter/node/edge 强制同 revision，`position` 在同一父级内唯一。
- 增加 `pack_version_inputs`（或等价 snapshot manifest），记录 pack version 使用的 lock、所有 applied content revision、quest revision、构建配置和各自 hash。
- delivery check 必须带 `input_fingerprint`/`run_id`，build 只能接受与指定 pack version 输入完全一致且未过期的检查。
- lock 文件登记 `snapshot_sha256`、格式版本和不可变路径；artifact 的 `source_fingerprint` 必须由上述输入规范化计算。

### P0-4：包内模组去重和“未下载”状态与 v7 不一致

当前基线用空字符串表示未下载；v7 不变量要求 `NULL`。v6 的 `UNIQUE(pack_id, source, project_id)` 对 local 模组无效，因为 SQLite 中多个 NULL 彼此不相等。仅靠 service 查重无法防止并发事务插入重复 local JAR。

**必须修改：** 迁移时把空字符串规范化为 NULL；`pack_mods.sha1` 建外键。增加两个部分唯一索引：

- 非 local：`(pack_id, source, project_id)`，要求 project_id 非 NULL。
- local：`(pack_id, sha1)`，仅在 `source='local' AND sha1 IS NOT NULL` 时生效。

同时在同一事务完成 SHA-1 计算结果、`jar_index` 登记和 `pack_mods.sha1` 回填；重复添加必须稳定返回 `duplicate_mod`。

### P0-5：导入 preview token 没有持久化语义

v7 要求 inspect/preview → confirm/import，两次请求之间要验证短 TTL、一次性、输入 hash 和 Idempotency-Key；当前数据模型没有 preview/import 表，也没有定义 token 消费状态。只把 token 放内存会在重启、并发确认或多进程误启动时失去一次性保证。

**必须修改：** 增加 `import_previews`（或等价持久化/签名 token 方案），至少包含 `token_hash`、`input_hash`、`source`、`staged_path`、`expires_at`、`consumed_at`、`created_at`，并建立 token hash 唯一索引。confirm 必须在一个短事务中校验并消费 token；过期、重复消费、hash 不匹配和不同 payload 必须返回稳定错误。临时文件仍由 blobstore 负责，数据库只保存受 path policy 约束的句柄。

### P0-6：任务删除、幂等键、outbox 的生命周期相互矛盾

v7 要求删包时取消活跃任务、任务终态保留 30 天、幂等键永久保留、业务写入经 outbox；但 v6 DDL 使用 `tasks.pack_id ON DELETE CASCADE`，删除包会直接删任务和 task_events，无法留下取消/恢复证据。`outbox_events.pack_id ON DELETE CASCADE` 又可能吞掉尚未投递的动态。

**必须修改：** 在 DDL 中明确删除策略：推荐 tasks 使用 `ON DELETE SET NULL`（任务 payload/detail 保留脱敏的 pack 标识），删除包事务先以当前 epoch fencing 取消活跃任务，再删除/归档业务数据；task_events 不因 pack 删除立即丢失。outbox 使用 `SET NULL` 或在删除前写不带 pack 外键的 system event，确保删除行为可追踪。只有明确不需要保留的包级活动才使用 CASCADE。

`task_idem_keys` 可以没有 FK（因为 task 行 30 天后会清理），但必须保存 endpoint/kind 作用域、canonical payload hash 和长度上限；同键不同 payload 永远返回冲突。需要对永久表的写入速率和最大占用设定防滥用策略。

### P0-7：现有 v1 数据没有可验证的升级路径

当前 `schema_migrations` 由 `store.go` 临时创建，只有 `version/applied_at`；v6/v7 要求 `checksum`、逐迁移事务和已应用文件不可变。若直接把 `schema.sql` 换成 v7，既无法证明旧数据库来源，也无法在失败时安全恢复。

**必须修改：** 先明确“当前工作树数据库是否视为已发布版本”。若需要保留，迁移 runner 必须能识别 v1 形状、在备份后补齐 checksum，再按编号迁移；若明确尚未发布，可定义一次性开发基线重建，但也必须在文档中声明数据丢失边界，不能让程序静默覆盖。两种模式都需要：

- 每个 migration 文件的精确 checksum 和顺序校验。
- 空库、现有 v1 库、重复启动、迁移失败和高版本数据库拒绝的测试。
- `quick_check`、`foreign_key_check`、JSON1 探针和 migration 事务失败回滚。

## 4. P1 风险（P2–P7 前应解决）

### P1-1：跨表 FK 不完整，且缺少 FK 索引

以下关系不能只靠单列 FK 或 service 约定：

- `mod_dependencies.pack_id/from_pack_mod_id/lock_id` 必须同包、同 lock。
- `pack_mod_updates.pack_mod_id/candidate` 必须绑定本包模组和一次求解快照；当前 candidate_version_id 是无来源的自由文本。
- `quest_nodes.chapter_id` 必须与 node 的 revision 相同；`quest_edges` 两端必须同 revision。
- `pack_versions.lock_id`、`delivery_checks.pack_version_id`、`artifacts.pack_version_id`、`releases.pack_version_id` 必须同包。
- content/quest active pointer 必须指向所属对象。

为每个 child FK 增加对应索引（至少所有 `pack_id`、`task_id`、`revision_id`、`document_id`、`pack_version_id`、`pack_mod_id`、`lock_id`、edge node 列），否则包删除、巡检和 dashboard 聚合会在 WAL 单写者下放大锁持有时间。

### P1-2：状态约束没有落到 SQLite

建议增加或由受控 trigger/service 明确保证：

- task `progress` 在 0..100，attempt/recover_count 非负，`max_attempts` 有界。
- `leased/running` 必须有 owner、epoch、expiry；终态清空 lease 字段；paused 不持有 lease。
- `delivery_checks` 的 nullable `pack_version_id` 不能依赖普通 UNIQUE（SQLite 对 NULL 允许重复），应使用两个部分唯一索引或规范化 scope 列。
- `onboarding_state` 应保证 acknowledged 与 acknowledged_at 一致，并在 migration 中预置三个 step。
- `releases.idempotency_key` 非空且有长度上限，非空 remote_id 在 provider 内唯一。

### P1-3：JSON 只有 json_valid，没有领域边界

`content payload`、quest prerequisites/rewards/mod_refs、conflicts/detail、outbox payload、remote_state` 只验证 JSON 可解析，不能限制深度、大小、数组数量或必需字段。架构已经要求 service 级 schema 校验，这是正确的，但必须补充：

- 每类 JSON 的 `schema_version`。
- 单字段/总 payload 字节上限、数组长度和嵌套深度上限。
- 未知顶层字段拒绝，`metadata` 作为唯一扩展区。
- repository/service 错误路径测试；不能把 JSON 约束削弱成任意字符串。

### P1-4：outbox 没有投递重试和失败可观测字段

仅有 `delivered_at` 时，持续失败的事件会被 500ms 扫描重复尝试，既无法退避，也无法显示最后一次错误。建议增加 `attempts`、`next_attempt_at`、`last_error_code`、`last_attempt_at`，必要时增加短 lease；dispatcher 仍需每事件短事务、activity 的 origin_event_id 幂等。

### P1-5：永久幂等表存在资源耗尽风险

`task_idem_keys` 永不删除符合“历史键不复用”原则，但任意请求可制造无限行。必须限定 key 长度/字符集、canonical hash 长度、endpoint/kind 作用域，并在 HTTP 层做 token 级速率限制和总量监控。若需要清理，必须另行定义不可复用证明，不能悄悄按时间删除。

### P1-6：blob、remote cache 与 artifact 的数据库/文件两阶段语义还不够精确

“事务登记 → rename”必须明确 commit 边界：推荐 temp fsync → 短事务登记 → commit → 原子 rename；启动巡检发现“有索引无最终文件但有 part”时先续接 rename，确认没有可恢复 part 才解除引用。删除则应先在事务中标记/解除引用，commit 后删文件，删除失败由巡检重试，不能在回滚可能发生的事务内先删唯一文件。

`remote_cache` 需要文件大小/hash 或可验证的完整性字段；`artifacts` 建议以 `(pack_version_id, kind, source_fingerprint)`（或等价业务键）去重，保留 sha256、size 和生成配置。artifact 不应因 task GC 丢失来源关系。

### P1-7：保留/GC 责任没有覆盖所有 v7 表

当前文档明确了 task/outbox/activity/audit/conflict/blob/cache 的大方向，但 onboarding、content/quest revisions、mod_dependencies、pack_alerts、pack_mod_updates、delivery_checks、import_previews、allowed_export_dirs 的责任人、留存期和删除触发条件没有逐表落地。

建议迁移注释和文档各自列出：

| 表/对象 | 建议策略 |
|---|---|
| onboarding_state/settings/secrets/allowed_export_dirs | 随实例永久保留；显式删除/重置才改变 |
| content/quest revisions、pack_locks、pack_versions | 随包保留；只追加，不 GC 历史 |
| mod_dependencies、delivery_checks | 随 lock/version 保留或只保留最新 run，必须可追溯输入 hash |
| pack_alerts/pack_mod_updates | open/ignored 保留；resolved/过期候选按明确窗口清理 |
| import_previews | consumed/expired 短留存（如 24–48h），文件先清理再删元数据 |
| artifacts/releases | 随包保留；用户显式删除产物时保留审计摘要 |

### P1-8：SQLite 连接与升级策略要按“每连接”验证

`foreign_keys=ON` 是连接级设置；未来读连接池不能只在写连接设置。WAL、busy_timeout、synchronous、checkpoint 和读快照行为也必须在真实 `modernc.org/sqlite` 驱动下验证。迁移期间应使用 OS 单实例锁 + `BEGIN IMMEDIATE`，并在升级前完成备份；高版本 schema 必须拒绝启动而非降级运行。

## 5. 可接受取舍（需写入 ADR，不应被误判为缺陷）

以下选择可以接受，但要明确边界、证据和测试：

1. **SHA-1 作为 JAR 寻址键。** 这是 Provider/模组生态兼容要求；建议同时保存 SHA-256 做本地完整性校验，且不把 SHA-1 当作安全签名。
2. **content/quest 领域 payload 使用 JSON。** SQLite 只负责 `json_valid` 和大小边界，领域 schema 由 service 校验；不要为了把全部业务规则塞进 CHECK 而引入不可维护触发器。
3. **删除包不在数据库事务内递归删文件。** 采用清理清单、引用检查和 grace 宽限是正确的；但必须验证崩溃、磁盘满和重复 GC。
4. **`activities.origin_event_id` 不反向 FK 到 outbox。** outbox 30 天、activity 90 天不一致时保留快照是合理的；需要 origin 唯一约束和脱敏 payload。
5. **当前不实现本地账号/成员表。** `Principal(local)` 和启动 token 足够支撑单机版本；未来 GitHub identity 作为入口，不应为了预留而新增无用途的 tenant/account 数据列。
6. **冲突和交付检查的部分唯一索引。** SQLite 对 NULL 的 UNIQUE 语义必须被部分索引或 scope 规范化明确处理，不能依赖调用方记忆。

## 6. 建议的 DDL / migration 顺序

顺序目标是：先消除循环，再建立父表和跨表约束，最后接入高风险文件/任务语义。下面是建议的发布序列；具体编号可在 ADR 中调整，但已发布编号不可改。

### 0001：可执行 v7 初始基线

为新空库创建完整 DDL，而不是引用 v6 文本。顺序建议：

1. `schema_migrations`（含 `version`, `name`, `checksum`, `applied_at`）。
2. `packs`（不放指向 pack_versions 的循环 FK；保留状态、版本和 loader 字段）。
3. `pack_locks`（只引用 packs；包含 snapshot hash/format）。
4. `pack_versions`（复合 FK `(pack_id,lock_id)`；唯一 `(pack_id,id)` 和 `(pack_id,version)`）。
5. `pack_current_version`（若采用指针表，用复合 FK 保证同包）。
6. `jar_index`、`pack_mods`（规范化 NULL sha1、部分唯一索引、FK 索引）。
7. `conflicts`、`mod_dependencies`、`pack_alerts`、`pack_mod_updates`。
8. `content_documents`、`content_revisions`、`content_validation_runs`。
9. `quest_books`、`quest_revisions`、`quest_chapters`、`quest_nodes`、`quest_edges`（复合 FK 和 position 唯一）。
10. `tasks`、`task_events`、`task_idem_keys`（严格状态约束和活跃去重）。
11. `outbox_events`、`activities`、`audit_events`。
12. `settings`、`secrets`、`onboarding_state`、`allowed_export_dirs`。
13. `remote_cache`、`blob_grace`、`import_previews`。
14. `delivery_checks`、`artifacts`、`releases`、`pack_version_inputs`。
15. 所有 FK/查询索引、部分唯一索引和必要的受控触发器。

### 0002：现有 v1 库采用/升级

如果当前 v1 数据需要保留，先备份并验证表形状；补齐 migration checksum 元数据，然后在同一迁移事务中：

- 统一空字符串与 NULL（尤其 `pack_mods.sha1/project_id`）。
- 给现有数据填充合法默认 status/loader/version，并把无法推断的行标记为需要修复，而不是静默猜测。
- 重建需要增加 FK/CHECK/部分唯一索引的表（SQLite 不支持任意 `ALTER TABLE ADD CONSTRAINT`）。
- 将现有任务状态映射到 v7 枚举；未知状态直接阻止迁移。

若产品明确当前数据库未发布且允许丢弃，应另写“开发基线重建”ADR，命令必须显式、带绝对路径确认，不得由普通启动流程自动删除。

### 0003：任务/outbox/审计基础不变量

补充状态迁移约束、lease/fencing 字段、幂等双轨、task event 事件上限、outbox 投递重试字段和 audit 索引。先完成 repository 约束测试，再开放 service 写入。

### 0004：内容与任务书 revision

补充 applied 唯一性、active 指针、同 document/book/revision 的复合约束、canonical payload/hash、validation run 和 rollback 规则。

### 0005：模组依赖、冲突和更新快照

补充 dependency fingerprint、lock 归属、conflict UPSERT 语义、update candidate 快照与 provider/source 字段；为 `pack_mods.sha1` 与 `jar_index` 接入同事务登记。

### 0006：pack version、delivery、artifact、release

补充 `pack_version_inputs`、输入 fingerprint、artifact 唯一键/sha256、release 幂等与 remote_id 索引；delivery check 必须标记输入来源和 stale 规则。

### 0007：导入 preview 和文件句柄

增加 `import_previews`（或签名 token 的消费记录），写入 source/hash/TTL/消费状态；所有 path 只允许 blobstore/path policy 生成的相对句柄。

### 0008：保留/GC 辅助字段和运行校验

补充 GC 所需的 attempts/next run/grace 状态、缓存和 artifact 完整性字段；migration 完成后统一执行 `quick_check`、`foreign_key_check`、JSON1 探针和 checksum 校验。

### 每个 migration 的共同门禁

- 单事务；失败自动回滚，不能留下半应用 schema。
- 空库、当前 v1 库、重复启动和高版本拒绝各有测试。
- 应用后检查所有 FK/CHECK/UNIQUE、索引和 trigger。
- 不修改已应用 migration；schema 变化只新增编号文件。
- migration 注释写明表保留策略、GC 责任人、数据转换和不可逆操作。

## 7. 允许开工条件（数据库视角）

### 必须先完成（P0）

1. 评审并冻结一份完整、可执行的 v7 DDL，解决 current pointer、lock/version、任务删除策略和身份边界的文档冲突。
2. 选择并记录当前 v1 数据的升级政策（保留升级或明确的开发基线重建），不得隐式覆盖。
3. 完成 migration runner 设计：编号、checksum、单事务、失败回滚、高版本拒绝、每连接 `foreign_keys`、quick/foreign key check。
4. 落实同包复合约束、local 模组部分唯一、revision active/applied 唯一和 pack version 输入快照。
5. 明确 task/outbox/idem 的生命周期和删包语义，补齐 preview token 的持久化/消费方案。
6. 为上述不变量写 repository 级失败测试，再允许 service 使用。

### 可以并行（不改变数据契约）

- 构建/打包脚本、依赖锁定、测试 fixture 框架。
- migration/SQLite 测试工具、假时钟、临时目录和 crash-restart 测试 harness。
- 不写业务 SQL 的 HTTP 错误映射和 API fixture 结构。

### 不应提前开始

- P3 Pack CRUD 直接依赖当前 `schema.sql`。
- P4 任务 runner 在没有最终 tasks/outbox/idem DDL 前写状态持久化。
- P5 Provider/download 在 `sha1` 空字符串、local 去重和 blob 生命周期未冻结前落库。
- P6 content/quest 在 revision 同域约束和 pack version snapshot 未冻结前设计 service DTO。
- P7 build/publish 在 artifact/release 输入 fingerprint 与幂等语义未冻结前实现。

## 8. DBA 最终意见

**数据库方向：可行。当前实现基线：不具备按 v7 开工的验收条件。**

只要按第 7 节先冻结 canonical DDL、升级政策和跨表不变量，SQLite 单库方案可以覆盖 P0–P7；不需要换数据库，也不需要提前引入 tenant/workspace 或本地账号表。最重要的修正不是继续增加表，而是让“同包归属、不可变快照、任务终态、幂等键、outbox、文件两阶段”在数据库和 service 之间各有明确、可测试的责任。

## 9. 与高级开发侧的交叉讨论结论

高级开发侧报告（`docs/reviews/database-senior-dev-review.md`）与本 DBA 报告逐项对照后，双方没有发现需要推翻 SQLite 单库方案的理由。以下是对其意见的明确回应。

### 9.1 双方一致同意的阻断项

1. **完整核心 DDL：同意，P0。** v7 必须产出一份可执行的完整核心 + v7 DDL；不能让实现者把 v6、v7 和当前 `schema.sql` 手工拼接。列表契约、错误 envelope、任务枚举和导入响应虽属于 API 层，但必须与 DDL 的 kind/status/source 约束同一轮冻结。
2. **不可变 lock snapshot：同意，P0（P5/P7 前）。** 仅保存 lock id 或从可变 `pack_mods` 读取不能证明可复现。DBA 的最小基线建议先采用 `snapshot_json + snapshot_schema_version + snapshot_sha256` 的规范化 JSON 快照，构建只读快照；若依赖查询或局部更新确实需要关系表，再在 P5 前追加 `pack_lock_mods`/依赖快照表。两种形式都必须保留 canonical hash，不能回退为“构建时重新查询当前清单”。
3. **`pack_mods.sha1` 使用 NULL：同意，P0。** 空字符串和 NULL 不能并存；迁移要先验证和转换，缺失 blob 的巡检才能安全置 NULL。
4. **revision applied 唯一性：同意，P0/P1。** content/quest 每个父对象至多一个 applied revision，active 指针必须同父对象且指向 applied；优先使用部分唯一索引 + 复合 FK/指针表，剩余 JSON 规则由 service 校验并留负例测试。
5. **列表 envelope、任务映射、import source/202：同意，API 开工前 P0。** 这些不是数据库表本身，但如果不先冻结，repository DTO 和状态 CHECK 会被前端适配反向污染。统一采用 v7 envelope，由 web adapter 向组件提供数组；领域 `succeeded/canceled` 与展示 `success/cancelled` 集中映射；import preview/confirm 的 token、hash、202 响应和 source 枚举固定在契约中。
6. **PID 文本锁风险：同意，启动可靠性 P0。** 当前 `internal/instlock` 是“创建文件 + PID 存活探测”，不是内核持有的排他锁；两个进程可能在 stale 判定和删除之间竞态，PID 重用也可能误判。实现前必须改为平台原生句柄锁（Windows `LockFileEx` 或等价 API，Unix `flock`/`fcntl`），或写 ADR 明确接受风险并补竞态测试；DBA 建议前者。
7. **health 副作用边界：同意，API/运行时 P1，进入 E1 前冻结。** `GET /api/system/health/status` 不应隐式触发 Provider 调用或改变业务状态，应读取最近探测快照并区分 `unknown/unavailable/ok`；只有显式 `POST /settings/providers/{provider}/test` 或受控的 mod-search/meta GET 才能产生上游副作用，并受全局限速。此规则影响 remote_cache 写入和 outbox，不可留给各 handler 自行决定。

### 9.2 DB 侧对“最小开工基线”的最终取舍

双方意见合并后，允许进入 P2 数据库实现的**最小基线**为：

- 完整 `0001` DDL（或等价可审计的 0001–000N）已经评审；核心表字段、CHECK、FK、部分唯一索引和 retention 清单齐全。
- `pack_locks` 已有独立、不可变、带 schema version 和 SHA-256 的 snapshot 载荷；`pack_versions` 以单向关系绑定 lock；不保留 `pack_locks.pack_version_id` 反向循环列。
- `pack_mods.sha1` 可空且有 `jar_index` FK；local 模组有 `sha1` 部分唯一，远端模组有 project 唯一；空值语义和迁移回填已测试。
- content/quest revision 的 applied/active 和同 revision 归属可由 DB 索引/FK 加 service 断言共同保证。
- tasks/outbox/task_idem_keys 至少具备 v7 状态字段、epoch、payload hash、活跃去重、投递幂等；删包策略已决定为保留可追踪的任务摘要（推荐 SET NULL），不因 CASCADE 静默抹掉取消证据。
- import preview 的一次性消费和 TTL 有持久化记录或等价可验证方案；不能只放进程内存。
- migration runner 支持 checksum、失败回滚、旧 v1 改造政策、quick/foreign key check 和高版本拒绝。

这个基线足以让 `repository → service → httpapi` 开始编写和测试；Provider、复杂 JSON 业务 schema、artifact 发布细节和 GitHub identity 可以按里程碑延期，但不能改变上述主键、外键、状态和快照语义。

### 9.3 可以延期、但必须登记的项目

- `pack_lock_mods` 规范化子表：若 P2 先采用 canonical JSON snapshot，关系化依赖快照可在 P5 前追加；不得因此让构建读取可变清单。
- crash report 专用表：dashboard 可按契约稳定返回“未知/0”，但导入和交付检查前必须定义来源。
- metrics 导出协议、读模型物化和高并发优化：先有结构化日志、task/pack/request 关联及关键计数。
- Provider 真实平台差异、远端发布状态和跨平台安装器：分别在 P5/P7 前完成 fixture 和幂等测试。
- GitHub OAuth/device-flow 和协作数据：P8 之前只保留 `Principal(local)`/identity 接口，不建本地账号、session、成员或 tenant 表。

### 9.4 交叉讨论后的放行结论

**DBA 与高级开发侧结论一致：当前数据库设计方向可行，但当前 v1 实现不能作为 P2–P7 的稳定地基。** 在完整 DDL、迁移策略、lock snapshot、NULL/枚举/同包约束、任务/outbox/preview 一次性语义以及 PID 锁和 health 副作用边界冻结并通过失败测试前，只允许推进不依赖最终业务 schema 的脚本、fixture、迁移 harness 和文档；不允许直接堆叠 Pack CRUD、任务持久化、Provider 下载或构建发布代码。
