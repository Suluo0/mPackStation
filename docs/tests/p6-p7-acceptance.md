# P6/P7 内容、任务书与交付验收矩阵

本矩阵记录 P6 service/repository 与独立 HTTP 契约证据。P7 构建/发布端点仍需
独立复核；P6 当前因两项 HTTP 响应契约缺陷，不能标记最终合格。

| 编号 | 场景 | 预期 | 自动化证据 | 状态 |
|---|---|---|---|---|
| P6-01 | 创建 recipe/structure/ore 文档 | 产生 revision=1 draft；JSON canonical 化 | `TestP6ContentRevisionLifecycleAndEvidence` | 已覆盖 |
| P6-02 | 同 payload 重复保存 | 不新增 revision；stale If-Match 返回 revision conflict | 同上 | 已覆盖 |
| P6-03 | 未知字段、非法 schema、ore 范围 | 稳定 invalid_argument 或 validation failed；apply 不改变 active 指针 | `TestP6ContentValidationRejectsUnknownAndBlocksApply` | 已覆盖 |
| P6-04 | apply/rollback/history | 追加式 revision、active 指针、delivery-check 和三类证据同事务 | `TestP6ContentRevisionLifecycleAndEvidence` | 已覆盖 |
| P6-05 | 任务书完整快照 | chapter/node/edge 持久化；revision 单调 | `TestP6QuestGraphLifecycleAndValidation` | 已覆盖 |
| P6-06 | 环、孤立节点、奖励/引用校验 | cycle 为阻断错误；孤立节点为 warning；跨包 mod ref 拒绝 | `TestP6QuestRejectsCycleOrCrossPackReference` | 已覆盖 |
| P6-07 | HTTP content/quest routes | 成功/错误 envelope、request-id、If-Match、前端 zod | `TestP6HTTPContentRevisionContract`, `TestP6HTTPContentValidationAndErrorEnvelope`, `TestP6HTTPQuestRevisionContract`, `TestP6HTTPQuestGraphValidation` | 不合格：draft revision.state 为空；跨包引用 error_code=invalid_argument |
| P7-01 | delivery checks/build artifact | 稳定输入 fingerprint、可复现 zip、SHA-256 登记 | `TestP7BuildIsReproducibleAndIdempotent`, `TestP7BuildRejectsUnsafeInputsAndBlockedDelivery` | 已覆盖，待独立复核 |
| P7-02 | publish retry/status | 非幂等发布不自动重试；远端状态先查询 | `TestP7PublishLocalIsIdempotentAndFailedRetryIsExplicit` | 已覆盖，待独立复核 |

执行命令（Go 工具链可用时）：

```text
go test ./internal/service ./internal/store -run '^TestP6' -count=1
go test ./...
go vet ./...
```

独立 Luna HTTP 证据（2026-08-29）：

```text
.tools/go/bin/go.exe test ./internal/httpapi -run '^TestP6HTTP' -count=1 -timeout=120s
FAIL（P6 HTTP 契约）
.tools/go/bin/go.exe test ./... -count=1 -timeout=180s
FAIL（仅新增 P6 HTTP 验收断言失败；其余已执行包显示通过）
```

待修复项：

- 修复 content draft 成功响应中的 `revision.state`，确保为 `draft`。
- 将跨包引用错误映射为 `cross_pack_reference`，保留 400 与 request-id。
- 修复后重新执行四个 `TestP6HTTP*`，并确认错误 envelope、If-Match、request-id
  与前端 adapter 一致。
