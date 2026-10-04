# P6/P7 内容、任务书与交付验收矩阵

本矩阵记录 P6 与 P7 的 service/repository/HTTP 契约证据。P6 与 P7 均已完成本轮独立测试；前端 zod adapter 和真实在线 Provider 仍不属于本后端门禁。

| 编号 | 场景 | 预期 | 自动化证据 | 状态 |
|---|---|---|---|---|
| P6-01 | 创建 recipe/structure/ore 文档 | 产生 revision=1 draft；JSON canonical 化 | `TestP6ContentRevisionLifecycleAndEvidence` | 通过 |
| P6-02 | 同 payload 重复保存 | 不新增 revision；stale If-Match 返回 revision conflict | 同上 | 通过 |
| P6-03 | 未知字段、非法 schema、ore 范围 | 稳定 invalid_argument 或 validation failed；apply 不改变 active 指针 | `TestP6ContentValidationRejectsUnknownAndBlocksApply` | 通过 |
| P6-04 | apply/rollback/history | 追加式 revision、active 指针、delivery-check 和三类证据同事务 | `TestP6ContentRevisionLifecycleAndEvidence` | 通过 |
| P6-05 | 任务书完整快照 | chapter/node/edge 持久化；revision 单调 | `TestP6QuestGraphLifecycleAndValidation` | 通过 |
| P6-06 | 环、孤立节点、奖励/引用校验 | cycle 为阻断错误；孤立节点为 warning；跨包 mod ref 拒绝 | `TestP6QuestRejectsCycleOrCrossPackReference` | 通过 |
| P6-07 | HTTP content/quest routes | 成功/错误 envelope、request-id、If-Match；前端 zod adapter 另行验收 | `TestP6HTTPContentRevisionContract`, `TestP6HTTPContentValidationAndErrorEnvelope`, `TestP6HTTPQuestRevisionContract`, `TestP6HTTPQuestGraphValidation` | 通过（2026-08-29 最终复验） |
| P7-01 | delivery checks/build artifact | 稳定输入 fingerprint、可复现 zip、SHA-256 登记 | `TestP7BuildIsStableAndZipMetadataIsReproducible`, `TestP7DeliveryChecksBlockAndAllowBuild` | 通过（Luna） |
| P7-02 | publish retry/status | 非幂等发布不自动重试；远端状态先查询 | `TestP7PublishFailureDuplicateAndExplicitRetry`, `TestP7PollingFailurePreservesPublishingState` | 通过（Luna） |

执行命令（Go 工具链可用时）：

```text
go test ./internal/service ./internal/store -run '^TestP6' -count=1
go test ./...
go vet ./...
```

独立 Luna 复验证据（2026-08-29）：

```text
.tools/go/bin/go.exe test ./internal/httpapi -run '^TestP6HTTP' -count=1 -timeout=120s
PASS（P6 HTTP 契约）
.tools/go/bin/go.exe test ./internal/service ./internal/store -run '^TestP6' -count=1 -timeout=120s
PASS
.tools/go/bin/go.exe test ./... -count=1 -timeout=180s
PASS
go vet ./...
PASS
gofmt -l internal
PASS（无输出）
go test ./internal/httpapi -run '^TestP7' -count=1 -timeout=180s
PASS（P7 HTTP、构建、产物、发布、任务验收）
```

最终结论：P6 与 P7 service/repository、HTTP 路由、错误契约及任务/产物核心行为均通过；前端 zod adapter、真实在线 Provider 和干净部署 smoke 仍分别记录为后续验收范围。
