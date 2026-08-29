# P6 内容与任务书验收矩阵

本矩阵记录 P6 service/repository 与独立 HTTP 契约证据。P7 已按项目经理要求回滚，待后续重新规划。

| 编号 | 场景 | 预期 | 自动化证据 | 状态 |
|---|---|---|---|---|
| P6-01 | 创建 recipe/structure/ore 文档 | 产生 revision=1 draft；JSON canonical 化 | `TestP6ContentRevisionLifecycleAndEvidence` | 通过 |
| P6-02 | 同 payload 重复保存 | 不新增 revision；stale If-Match 返回 revision conflict | 同上 | 通过 |
| P6-03 | 未知字段、非法 schema、ore 范围 | 稳定 invalid_argument 或 validation failed；apply 不改变 active 指针 | `TestP6ContentValidationRejectsUnknownAndBlocksApply` | 通过 |
| P6-04 | apply/rollback/history | 追加式 revision、active 指针、delivery-check 和三类证据同事务 | `TestP6ContentRevisionLifecycleAndEvidence` | 通过 |
| P6-05 | 任务书完整快照 | chapter/node/edge 持久化；revision 单调 | `TestP6QuestGraphLifecycleAndValidation` | 通过 |
| P6-06 | 环、孤立节点、奖励/引用校验 | cycle 为阻断错误；孤立节点为 warning；跨包 mod ref 拒绝 | `TestP6QuestRejectsCycleOrCrossPackReference` | 通过 |
| P6-07 | HTTP content/quest routes | 成功/错误 envelope、request-id、If-Match；前端 zod adapter 另行验收 | `TestP6HTTPContentRevisionContract`, `TestP6HTTPContentValidationAndErrorEnvelope`, `TestP6HTTPQuestRevisionContract`, `TestP6HTTPQuestGraphValidation` | 通过（2026-08-29 最终复验） |

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
```

最终结论：P6 service/repository、HTTP 路由与错误契约均通过；前端 zod adapter
仍属于 web 契约验收范围，不作为本后端独立验收的隐性通过项。
