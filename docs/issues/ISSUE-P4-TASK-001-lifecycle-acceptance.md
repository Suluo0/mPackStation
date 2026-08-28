# ISSUE-P4-TASK-001：P4 持久化任务生命周期和恢复验收未通过

- **责任域**：Luna｜test/p3-p4-acceptance
- **身份**：测试负责人；不修改生产实现
- **优先级**：P1（阻塞所有依赖异步任务的导入、求解、构建和发布）
- **状态**：待 P4 实现负责人处理

## 问题

任务必须是可恢复的持久化状态机，而不是只有一个 status 字段。验收需要证明状态边界、双重幂等、租约心跳、epoch fencing、取消/暂停/重试和 kill-restart 恢复都可重复执行。

## 已建立的数据库门禁

`apps/server/internal/store/p4_task_acceptance_test.go` 覆盖 P4-DB-001..006：

- lease 字段与任务状态的组合约束；
- 活跃任务唯一键和永久 idempotency registry；
- 错误 owner/epoch heartbeat 拒绝；
- 旧 epoch progress 写入拒绝；
- 终态不可被 active cancel 谓词修改；
- task event 状态枚举和 sequence 唯一性。

## 实现后必须补的 runner 证据

P4-SVC-001..007 必须使用真实 runner/service、临时 SQLite、假时钟和可控 worker，至少覆盖：

- 全状态迁移矩阵和非法迁移错误码；
- 并发双提交、同键异 payload 和重启后的幂等；
- lease/heartbeat/fencing 的旧 worker 进度、日志和业务写回；
- 取消/暂停/恢复竞争；
- 退避、最大尝试、deadline 和显式 retry；
- kill-restart 后无永久 running；
- task、outbox、activity 的最终一致性和重复投影保护。

## 复现

```text
cd apps/server
go test ./internal/store -run '^TestP4'
```

测试输出中的编号对应 `docs/tests/p3-p4-acceptance.md`。红灯是可读的实现缺口，不得通过跳过、降级或删除测试消除。

## 当前首次证据（2026-08-29）

`go test ./internal/task -run '^TestP4' -count=1 -v -timeout 8s`：

- **通过**：canonical payload 幂等、并发双提交（P4-RUN-003/004）。
- **失败**：happy path、RunOnce、pause/resume/cancel/recover（P4-RUN-001/005/006），现有任务写入在切换到非 leased/running 状态时没有清空 `lease_epoch`，触发 tasks 的 lease/state CHECK。
- **失败**：stale Begin 未及时返回 `ErrLeaseLost`（P4-RUN-002）；当前 fenced 错误路径在事务仍持有单连接时再次读取数据库，导致上下文超时，不能留下连接/事务等待。

上述是实现需要修复的具体证据，不是测试降级理由。

## 复验更新（2026-08-29）

生产侧已修复初始的 terminal lease CHECK、RunOnce 终态和 stale fencing 阻塞问题；当前自有 runner 测试 5/6 通过，数据库约束测试 6/6 通过。剩余明确缺口：

1. `Retry` 目前只接受 `failed`，未覆盖 v7 reliability 规定的 `canceled → queued` 显式重试。
2. HTTP task detail/control/log 路由尚未注册，P4-HTTP-001/002 仍收到 fallback `not_found`。

`go vet ./...` 已通过；完整 task/http 组合仍因以上红灯不能作为 P4 通过证据。
