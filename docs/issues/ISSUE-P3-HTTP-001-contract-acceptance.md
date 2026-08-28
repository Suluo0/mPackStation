# ISSUE-P3-HTTP-001：P3 HTTP 真实契约验收门禁未通过

- **责任域**：Luna｜test/p3-p4-acceptance
- **身份**：测试负责人；不修改生产实现
- **优先级**：P1（阻塞前端关闭 mock 和 P3 里程碑）
- **状态**：待 P3 实现负责人处理

## 问题

P3 的公开 HTTP 能力必须覆盖 dashboard、tasks、activities、system health/status、onboarding、MC versions、Pack CRUD、导入入口，以及任务控制路由。仅注册路由或返回 200 空 JSON 不能算通过。

## 验收条件

1. `p3_acceptance_test.go` 中 P3-HTTP-001..006 全部通过。
2. 成功响应能被前端契约解析，列表分页、排序和状态映射稳定。
3. 非法参数、资源不存在和无 token 请求返回稳定错误 envelope、错误码和 request-id。
4. 创建、归档、删除等写入不留下半条业务数据，并在需要时写 outbox/activity/audit。
5. 失败测试修复后不得删除或弱化原断言。

## 复现

```text
cd apps/server
go test ./internal/httpapi -run '^TestP3'
```

测试输出中的编号对应 `docs/tests/p3-p4-acceptance.md`。
