# P7 构建、产物与发布验收

## 范围与依据

本验收覆盖 v7 `backend-architecture-v7.md` §6.6（构建与发布）和 §11（测试、契约与验收），以及 P7 优先级中的交付检查、pack version、可复现 ZIP、artifact、CurseForge/Modrinth 发布和失败恢复。测试只读业务公共边界；不通过放宽安全检查、自动重试或绕过 service/store/provider 分层来获得通过。

自动化入口：`apps/server/internal/httpapi/p7_acceptance_test.go`。测试使用临时 SQLite、临时导出目录和确定性的 provider adapter，不依赖网络、开发机残留目录或执行顺序。

## 用例索引

| 编号 | 验收断言 | 证据 |
|---|---|---|
| P7-HTTP-001 | §6.6 所列 delivery-check、versions、build、artifacts、releases、CF/MR publish HTTP 路由必须存在；资源错误可以是领域错误，但不能是 generic 404/405 | `TestP7HTTPContractRoutesMatchV7` |
| P7-BUILD-001 | blocked delivery check 阻断构建且不得登记 artifact | `TestP7DeliveryChecksBlockAndAllowBuild` |
| P7-BUILD-002 | passed/warning 检查允许构建；检查绑定 pack/version 和 source fingerprint | `TestP7DeliveryChecksBlockAndAllowBuild` |
| P7-BUILD-003 | pack version 必须属于目标 pack；跨 pack version 被拒绝；lock/content/quest/build-config 四种来源持久化到目标 version | `TestP7BuildBindsPackVersionAndCapturesSource` |
| P7-BUILD-004 | 输入和 manifest 稳定排序，ZIP 条目时间/Extra 固定，重复构建字节、fingerprint、artifact ID 和登记行稳定 | `TestP7BuildIsStableAndZipMetadataIsReproducible` |
| P7-ARTIFACT-001 | 登记的 SHA-256/size 与文件一致；下载端点返回原始字节和下载处置头，不能泄漏路径 | `TestP7ArtifactHashAndDownloadContract` |
| P7-FILE-001 | 未批准、文件系统根、符号链接导出目录和 archive traversal 均被拒绝 | `TestP7ExportDirectorySafety` |
| P7-PUBLISH-001 | CurseForge 与 Modrinth 使用统一 provider DTO；发布进入 publishing，轮询远端 succeeded | `TestP7CurseForgeAndModrinthPublishTasksAndPolling` |
| P7-PUBLISH-002 | 非幂等失败只调用 provider 一次；重复同一 key 不自动重试；显式 retry 才允许第二次调用 | `TestP7PublishFailureDuplicateAndExplicitRetry` |
| P7-PUBLISH-003 | 轮询 404/不可达等 provider 状态失败返回稳定 unavailable，且不丢失 publishing；后续显式轮询可恢复 | `TestP7PollingFailurePreservesPublishingState` |
| P7-TASK-001 | publish task 的 canonical payload 重复提交复用同一任务；不同 payload 返回 idempotency conflict | `TestP7TaskDuplicateCancelAndRecovery` |
| P7-TASK-002 | queued 任务可取消，终态重复取消被拒绝 | `TestP7TaskDuplicateCancelAndRecovery` |
| P7-TASK-003 | 过期 lease 重启恢复为 queued、清除 owner 并增加 recover count | `TestP7TaskDuplicateCancelAndRecovery` |
| P7-HTTP-002 | token、Host、错误输入的 HTTP 状态、稳定 error.code、message、request_id、响应头一致 | `TestP7HTTPErrorEnvelopeIsStable` |

## 执行与结论规则

在 `apps/server` 执行：

```text
go test ./internal/httpapi -run '^TestP7' -count=1
go test ./...
go vet ./...
```

P7 HTTP 路由测试按公开 v7 契约先行编写；若实现尚未提供 §6.6 的 canonical route，`P7-HTTP-001` 或 artifact download 用例应明确红灯，记录为实现阻断，不得改写测试为当前的非契约路径。只有上述用例全部可重复通过，且既有测试、vet、构建和验收证据包均通过，才可判定 P7“合格”。

## 证据要求

验收记录必须包含当前工作树、Go/OS 版本、实际命令与退出码、每个编号的 PASS/FAIL、失败响应 envelope、产物 SHA-256/size、ZIP 条目顺序和时间、provider 调用计数，以及失败、重复、取消、恢复状态。任一路由缺失、错误 envelope 不稳定、artifact 文件与登记不一致、自动重试非幂等发布或恢复后永久假 `running`，均为不合格。
