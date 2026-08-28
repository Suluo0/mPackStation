# P3/P4 独立测试验收矩阵

> 执行身份：Luna｜test/p3-p4-acceptance
>
> 本矩阵是 P3（真实看板与 Pack CRUD）和 P4（持久化任务系统）的验收门禁。测试只验证公开契约、数据库不变量和可恢复语义，不为当前实现缺口降低断言。实现未完成时，红灯是可接受的开发证据，但不能作为里程碑通过依据。

## 判定规则

P3/P4 的正式结论必须同时满足：

1. `go test ./...`、`go vet ./...` 和适用的构建验证通过。
2. 失败路径、重复提交、并发竞争、取消和恢复路径都有可重复证据。
3. 没有为了绿灯删除测试、跳过测试、弱化契约或放宽安全/数据约束。
4. 测试输出能给出稳定的 `error.code`、`request_id`、`task_id` 和关键对象标识，便于人机共读。
5. P3 的契约测试使用真实 SQLite 和 HTTP handler；P4 的 lease/fencing 测试使用真实数据库条件更新，不用内存状态模型代替持久化行为。

## P3：看板、系统状态、迎新和 Pack CRUD

测试文件：`apps/server/internal/httpapi/p3_acceptance_test.go`

| 编号 | 人机共读场景 | 合格证据 | 失败含义 |
|---|---|---|---|
| P3-HTTP-001 | 首批看板、任务、活动、系统、迎新、MC 版本、Pack CRUD 和任务控制路由逐一请求 | 每个方法/路径均不是 404/405，且由真实 handler 处理 | 路由缺失、错误方法注册或仍是空壳 |
| P3-HTTP-002 | 空数据库读取 dashboard | 200；`packs`、`lastEditedPackId`、`todayResolvedCount` 存在；每个包含前端规格要求的嵌套计数 | 前端无法解析或聚合来源不完整 |
| P3-HTTP-003 | 读取最近任务和活动，并提交非法分页参数 | 列表字段和状态映射可解析；进度在 0..100；非法分页返回 400 `invalid_argument`，不静默改变语义 | 分页不稳定、内部枚举泄漏、错误契约不稳定 |
| P3-HTTP-004 | 带 token 创建 Pack，再提交缺失/非法字段 | 合法请求 201 并返回 `id/name`；非法请求 400 `invalid_argument`，不写半条数据 | Pack 写入绕过校验、事务或稳定错误码 |
| P3-HTTP-005 | 查询不存在的 Pack；无 token 提交写请求 | 不存在资源返回 404 `pack_not_found`；无 token 返回 401 `unauthorized`；两者都有非空 message 和 request-id | 越权/存在性泄漏、浏览器安全边界缺失 |
| P3-HTTP-006 | 读取 system health/status 和 onboarding | 200；布尔、容量、迎新三步字段完整且类型固定 | 看板/迎新无法从真实状态渲染 |

列表契约必须同时遵守 v7 的分页语义：排序固定、limit 有上限、cursor 绑定查询过滤条件；若 HTTP 适配层将 `{items,next_cursor,total}` 转成前端数组，转换必须集中在 adapter，并有对应 fixture 测试。

## P4：任务状态、幂等、lease 和恢复

数据库验收测试文件：`apps/server/internal/store/p4_task_acceptance_test.go`。

runner/service 验收测试文件：`apps/server/internal/task/p4_runner_acceptance_test.go`。

| 编号 | 人机共读场景 | 合格证据 | 失败含义 |
|---|---|---|---|
| P4-DB-001 | queued/running/leased/paused/终态与 lease 字段组合 | 非法组合被数据库拒绝；leased/running 要求 owner、epoch、expiry；paused/终态不持有 lease | 任务状态与租约可能分裂 |
| P4-DB-002 | 同一活跃任务重复 Idempotency-Key；终态任务和永久 registry | 活跃重复被唯一索引拦截；终态不靠活跃索引误拦；永久 key 不能第二次登记；payload hash 长度校验生效 | 重复副作用或重启后重复执行 |
| P4-DB-003 | 心跳使用错误 owner 或旧 epoch；正确 owner/epoch 更新 | 错误条件影响 0 行；当前 owner/epoch 恰好影响 1 行 | 旧 worker 可续租或持有任务 |
| P4-DB-004 | epoch 已变更后旧 worker 写 progress | 旧 epoch 更新影响 0 行，任务数据保持新 worker 的值 | fencing 不完整，旧进程可覆盖新状态 |
| P4-DB-005 | 终态任务执行取消谓词 | succeeded/failed/canceled 均不可被 active cancel 条件改变 | 终态可逆、任务历史不可信 |
| P4-DB-006 | task event 记录完整状态轨迹与 sequence | 合法状态可记录；同 task 重复 sequence 和非 canonical 状态被拒绝 | 任务历史不可审计或前端无法解释 |

| P4-RUN-001 | durable queue happy path | Submit → Lease → Begin → Progress → Succeed 后状态持久为 succeeded、进度为 100、lease 清空，事件 sequence 连续 | 正常完成无法持久化或历史不可解释 |
| P4-RUN-002 | 过期 worker fencing | 错误 owner/epoch 的 Begin、Heartbeat、Progress 全部返回 `ErrLeaseLost`，正确租约仍可更新 | 旧 worker 可以续租或覆盖进度 |
| P4-RUN-003 | canonical payload 幂等 | JSON 键顺序/空白不同但语义相同的请求返回同一任务；不同 payload 返回 `ErrIdempotencyConflict` | 重复请求产生副作用或哈希不稳定 |
| P4-RUN-004 | 并发双提交 | 两个 goroutine 同时提交同 key 同 payload，均得到同一 task ID，数据库只有一条任务 | 并发竞争下出现两个逻辑任务 |
| P4-RUN-005 | pause/resume/cancel/retry/recover | 合法控制动作可恢复；终态不被非法修改；lease 过期后重新排队并递增 recover_count，旧 epoch 写入被拒绝 | 状态机只覆盖成功路径或重启后永久 running |
| P4-RUN-006 | RunOnce handler | 注册 handler 的任务经 RunOnce 执行，handler 的 progress 和终态事件可见，最终状态成功 | queue 与领域 handler 边界失效 |

以下 runner/service 级用例必须在 P4 实现后由任务实现负责人接入同一测试门禁，不得用上面的数据库测试替代：

| 编号 | 场景 | 必须证明 |
|---|---|---|
| P4-SVC-001 | 全状态迁移矩阵 | 仅允许 `queued→leased→running→succeeded/failed/canceled`、`paused↔queued`，终态不可逆；非法边返回 `task_invalid_transition` |
| P4-SVC-002 | 双提交与同键异 payload | 同 key 同 payload 返回同一逻辑任务；同 key 不同 payload 返回 409；并发双提交至多产生一个活跃副作用 |
| P4-SVC-003 | lease/heartbeat/fencing | TTL 30s、heartbeat 10s；0 行 heartbeat 立即停止副作用；所有进度、日志、业务回写带 epoch |
| P4-SVC-004 | 取消/暂停/恢复竞争 | cancel 是协作式且最终可观测；不允许暂停 publish；暂停后可安全回 queued；取消与完成竞争不产生双终态事件 |
| P4-SVC-005 | 失败重试与 deadline | 自动退避 1/2/4/8 秒并受上限约束；显式 retry 才允许重置失败/取消任务；deadline 产生稳定内部错误 |
| P4-SVC-006 | kill-restart | 进程在 leased/running 任意阶段终止后，重启扫描在一个恢复周期内重新排队或明确失败；无永久假 running；旧 epoch 写入被拒绝 |
| P4-SVC-007 | task/outbox/activity 一致性 | 任务业务结果、task event、outbox 和 activity 在短事务中保持可追踪；投递失败可退避并恢复，不重复投影 |

## 执行与证据

在 `apps/server` 目录执行：

```text
go test ./internal/httpapi -run '^TestP3'
go test ./internal/store -run '^TestP4'
go test ./...
go vet ./...
```

每次报告必须记录：提交/工作树、操作系统、Go 版本、命令、退出码、每个编号的通过/失败、失败响应（去除敏感值）和对应 issue。P3/P4 的红灯要保留稳定错误信息，禁止以“当前功能未实现”替代 issue 或删除测试。
