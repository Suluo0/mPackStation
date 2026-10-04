# ISSUE-HTTP-001：前端业务 HTTP 路由尚未覆盖

- 类型：P1 / API 契约
- 状态：Open
- 责任：HTTP/service 负责人

## 现象

当前 router 仅提供 health/readiness 探针，前端所需 dashboard、tasks、activities、system、onboarding、meta、packs 和 import 路由尚未进入真实 service 契约。

## 完成条件

- 业务路由只调用 service，不由 handler 直接访问 SQL、Provider 或业务文件。
- 首批路由的成功响应、错误 envelope、request-id、token/Origin/Host 校验和请求体边界均有 fixture/契约测试。
- 不用 200 空壳响应掩盖未实现业务；未支持操作返回稳定错误码。

## 验收证据

- `V7-HTTP-001` 通过。
- 每个端点至少包含成功、非法输入、资源不存在和内部失败的可读用例。
