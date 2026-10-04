# ISSUE-P3-HTTP-002：动态路由验收夹具误把业务 404 判为缺路由

- **责任域**：Luna｜test/p3-p4-acceptance
- **身份**：独立测试负责人
- **优先级**：P2（测试质量；不改变产品契约）
- **状态**：已修正测试夹具

## 问题

Pack 的详情、归档、恢复和删除是带 `{packId}` 的动态路由。请求不存在的资源时，正确结果是 404 `pack_not_found`，不能把这个业务 404 当成 mux 没有注册路由。任务控制路由同理，应返回 `task_not_found`。

此前的路由探针只判断 HTTP 404，导致动态路由即使已正确注册也会被错误标红。

## 修正

P3/P4 路由验收现在按稳定错误码区分：

- fallback 404 `not_found`：判定为缺路由；
- 动态业务 404 `pack_not_found` / `task_not_found`：判定为路由存在且资源错误映射正确；
- 405：始终判定为方法注册错误。

修正涉及：

- `apps/server/internal/httpapi/p3_acceptance_test.go`
- `apps/server/internal/httpapi/p4_task_http_acceptance_test.go`
