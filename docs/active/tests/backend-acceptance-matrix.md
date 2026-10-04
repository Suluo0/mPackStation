# 后端数据库与 HTTP 验收测试矩阵

> 测试负责人：Luna（独立测试）
>
> 本文件只描述可执行的验收证据和当前缺口，不放宽 v7 约束。测试代码位于 `apps/server/internal/store/v7_schema_acceptance_test.go` 与 `apps/server/internal/httpapi/contract_acceptance_test.go`。

## 使用方式

在 `apps/server` 目录执行：

```text
go test ./internal/store -run 'V7|Acceptance'
go test ./internal/httpapi -run 'FrontendContract'
go vet ./...
```

当前 v1 基线预计会失败；失败本身是待实现的可读证据，不得通过跳过测试、改变期望或删除约束来消除。

## 测试矩阵

| 编号 | 验收要求 | 证据 | 当前状态 |
|---|---|---|---|
| V7-DB-001 | canonical migration 创建核心、任务、内容、任务书、构建发布和导入表 | `TestV7CanonicalSchemaHasRequiredTables` | 待 canonical migration |
| V7-DB-002 | `schema_migrations` 保存 version/name/checksum/applied_at，checksum 非空 | `TestMigrationMetadataContainsImmutableIdentity` | 当前缺 checksum/name |
| V7-DB-003 | 每次启动可验证 `foreign_keys=ON`、`quick_check=ok`、无 FK 违规、JSON1 可用 | `TestSQLiteStartupIntegrityChecks` | 当前只有部分启动检查 |
| V7-DB-004 | `pack_mods.sha1` 可为 NULL；local/remote 去重和 `sha1 → jar_index` 约束可证明 | `TestPackModsUsesNullableSHA1AndUniqueIndexes` | 当前用空字符串且无上述约束 |
| V7-DB-005 | 每个 content document / quest book 至多一个 applied revision | `TestRevisionAppliedStateHasDatabaseUniqueness` | 当前无 revision 表/索引 |
| V7-DB-006 | current version 与 lock/version 不允许跨包引用 | `TestPackVersionPointersAreSamePackConstrained` | 当前无 pointer/composite FK |
| V7-DB-007 | import preview 可重启消费，build 输入可追溯 | `TestImportPreviewAndBuildInputProvenanceExist` | 当前缺表 |
| V7-HTTP-001 | 前端首批 dashboard、system、onboarding、meta、pack、import 路由存在 | `TestFrontendContractRoutesExist` | 当前只注册探针 |

## 需要实现后再补强的行为测试

这些不是通过“表存在”即可放行的项目，进入对应阶段前必须追加成功与负例：

- 空库、历史 v1 库、重复启动、checksum 篡改、高版本数据库拒绝、迁移失败事务回滚。
- FK/CHECK/UNIQUE 的实际 insert/update/delete 负例，包括跨包 lock/version、NULL SHA-1、重复 local JAR 和重复 remote project。
- lock snapshot、content/quest revision 的不可变性、active 必须指向同对象的 applied revision、rollback 只追加新 revision。
- task 状态迁移、幂等键冲突、lease epoch fencing、取消/超时/重启恢复，以及 task/outbox/activity 的最终一致性。
- import preview 的过期、重复消费、输入 hash 不匹配、文件句柄恢复和恶意归档拒绝。
- HTTP 成功/错误 envelope、request-id、token/Origin/Host、body 上限、非法参数和资源不存在的稳定错误码。

## 已登记问题

### ISSUE-DB-001：canonical migration 尚未落地

- 影响：无法证明 v7 表、约束和升级路径可执行；阻塞 P2 数据库验收。
- 处理要求：提供有序 migration、checksum manifest、空库与历史 v1 升级策略；已应用文件不可变。
- 验收：V7-DB-001/002/003 全部通过，并有迁移失败回滚证据。

### ISSUE-DB-002：pack/mod/lock/revision 跨表不变量未落库

- 影响：并发写入可能造成重复 local JAR、跨包引用或多个 applied revision；阻塞后续 P3/P5/P6。
- 处理要求：使用 NULL SHA-1、部分唯一索引、同包复合 FK/受控指针和 applied 唯一索引；不能只依赖 handler 约定。
- 验收：V7-DB-004/005/006 通过，并补充实际约束负例。

### ISSUE-HTTP-001：前端业务 HTTP 路由尚未覆盖

- 影响：前端仍无法切换到真实 API；阻塞 P3 真实纵向闭环。
- 处理要求：路由只调用 service，补齐响应/错误契约、权限安全测试和 fixture；不得以 200 空壳响应充数。
- 验收：V7-HTTP-001 通过，并有成功与拒绝路径契约证据。

