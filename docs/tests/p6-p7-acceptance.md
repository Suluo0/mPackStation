# P6/P7 内容、任务书与交付验收矩阵

本矩阵只记录 P6 当前 service/repository 证据；P7 构建/发布端点和 P6 HTTP
adapter 尚未接入，因此不能将整项标记为最终合格。

| 编号 | 场景 | 预期 | 自动化证据 | 状态 |
|---|---|---|---|---|
| P6-01 | 创建 recipe/structure/ore 文档 | 产生 revision=1 draft；JSON canonical 化 | `TestP6ContentRevisionLifecycleAndEvidence` | 已覆盖 |
| P6-02 | 同 payload 重复保存 | 不新增 revision；stale If-Match 返回 revision conflict | 同上 | 已覆盖 |
| P6-03 | 未知字段、非法 schema、ore 范围 | 稳定 invalid_argument 或 validation failed；apply 不改变 active 指针 | `TestP6ContentValidationRejectsUnknownAndBlocksApply` | 已覆盖 |
| P6-04 | apply/rollback/history | 追加式 revision、active 指针、delivery-check 和三类证据同事务 | `TestP6ContentRevisionLifecycleAndEvidence` | 已覆盖 |
| P6-05 | 任务书完整快照 | chapter/node/edge 持久化；revision 单调 | `TestP6QuestGraphLifecycleAndValidation` | 已覆盖 |
| P6-06 | 环、孤立节点、奖励/引用校验 | cycle 为阻断错误；孤立节点为 warning；跨包 mod ref 拒绝 | `TestP6QuestRejectsCycleOrCrossPackReference` | 已覆盖 |
| P6-07 | HTTP content/quest routes | 成功/错误 envelope、request-id、If-Match、前端 zod | 待 P6 HTTP adapter | 待接入 |
| P7-01 | delivery checks/build artifact | 稳定输入 fingerprint、可复现 zip、SHA-256 登记 | 待 P7 | 未开始 |
| P7-02 | publish retry/status | 非幂等发布不自动重试；远端状态先查询 | 待 P7 | 未开始 |

执行命令（Go 工具链可用时）：

```text
go test ./internal/service ./internal/store -run '^TestP6' -count=1
go test ./...
go vet ./...
```
