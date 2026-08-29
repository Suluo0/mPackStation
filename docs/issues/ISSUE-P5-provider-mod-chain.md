# ISSUE-P5-001 Provider 与模组主链路

状态：实现已提交，独立验收阻断（2026-08-29）。HTTP 搜索分页字段与 resolve 审计证据已修复；必须补齐 CurseForge 真实响应 envelope/字段归一化并重验后再标记 P5 合格。

## 范围

- `internal/provider` 提供统一领域 DTO、Provider 接口、注册表、标准库 HTTP CurseForge/Modrinth adapter 和离线 fixture adapter。
- `internal/service` 编排包作用域搜索、模组加入/移除/启停/版本选择、SHA-1/JAR 索引、依赖求解、不可变锁快照、冲突与健康摘要。
- `internal/store` 是唯一 SQL 边界；写入模组、JAR 索引、锁、依赖、冲突同时产生 outbox/activity。
- `internal/httpapi` 暴露 `/api/packs/{packId}/mods`、`mod-search`、`resolve`、`locks`、`conflicts`、`health` 端点。

## 安全与边界

Provider 平台形状不泄漏到 service/httpapi；fixture 下载只登记 `jar://<sha1>` 对象句柄，不接受客户端服务器路径。当前不引入 tenant/workspace、账号管理或多实例协作。

## 后续风险

真实运行时需把 download 接入持久化 task 和 blobstore 的 temp→校验→事务登记→rename 流程，并补统一限流/熔断、离线缓存与远端发布任务；HTTP adapter 已具备 endpoint、超时和 404/401/429/5xx 映射，默认不启用任何真实端点。

## 独立验收记录

- 执行身份：Luna｜P5 独立测试负责人。
- 验证提交：`d7bbba3 feat(p5): add provider and mod resolution chain`（工作树若有后续未提交生产改动，以最终提交为准）。
- 通过：Provider fixture/Modrinth HTTP adapter 的搜索、项目/版本/元数据/下载 DTO 映射和错误映射；双包 SHA-1 共享与包级隔离；模组加入、启停、版本切换、移除；依赖、锁 hash、冲突幂等和错误信封成功路径。
- 已修复并复验：`service.TestP5AcceptanceLockSnapshotAndResolutionEvidence` 初次发现 resolve 前后 `outbox_events` 与 `activities` 数量不变；`httpapi.TestP5HTTPModChainAndStableProviderErrors` 初次发现搜索响应省略 `nextCursor`。当前两项复验均通过。
- 后续阻断：`provider.TestHTTPAdapterCurseForgeEnvelopeNormalization` 无法解析 CurseForge 实际 `{data:...}` 响应、数值平台 ID、`downloadCount`、`fileLength` 和 `hashes` 字段；这会使真实 CurseForge 项目/版本链路不可用。
- 重验命令：`go test ./internal/provider ./internal/store ./internal/service ./internal/httpapi -count=1`、`go vet ./internal/provider ./internal/store ./internal/service ./internal/httpapi`、`gofmt -l internal/provider internal/store internal/service internal/httpapi`。
