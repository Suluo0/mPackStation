# P4 任务引擎测试用例（人机共读）

这些用例以 `task_test.go` 为可执行证据，描述持久化任务队列必须保持的协议不变量。

| 用例 | Given | When | Then |
| --- | --- | --- | --- |
| 状态机与 fencing | 一个 queued 任务被 worker-a 领取并 begin | worker-a 写进度后，用户取消；worker-a 再提交完成 | 取消成功、epoch 增长、旧 worker 的写入返回 `ErrLeaseLost`，事件 sequence 连续 |
| 幂等键 | 同一 key 提交等价 JSON payload | 再次提交等价 JSON，随后提交不同 JSON | 等价请求复用同一 task；不同 payload 返回 `ErrIdempotencyConflict` |
| 重试退避 | 可重试失败、`max_attempts=2` | 第一次失败后立即领取，再经过 1 秒领取；第二次失败后手动 retry | 立即领取被拒绝；1 秒后可领取；达到尝试次数进入 failed；retry 重新进入 queued |
| 崩溃恢复 | running 任务 lease 过期 | 执行恢复扫描 | 任务回到 queued、`recover_count` 增长、lease 三元组清空；旧 worker heartbeat 被拒绝 |
| handler registry | queued 任务和已注册 handler | worker 执行 `RunOnce` | handler 被调用；进度事件写入；handler 返回后任务进入 succeeded |

验收时还应检查：终态不可逆、错误路径不绕过 lease 条件、每个状态变化均有 task event，且任务表约束拒绝无效 progress/status。
