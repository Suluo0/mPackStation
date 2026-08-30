# ISSUE-FE-BE-INTEGRATION：前后端真实对接

状态：进行中。加入当前 P0–P7 目标，未验收。

## 范围

保留用户正在修改的 Dashboard 接入与现有页面视觉；将 PackPages 中已有后端能力对应的 mock 数据、假保存、假构建替换成领域 adapter + zod + 真实 API。

## 当前已确认

- Dashboard 已包含真实 API 调用，不能再描述为整站完全 mock。
- PackPages 的 Pack、模组、依赖、内容、任务书、发布多数仍为静态数据和假动作。
- 导入确认 packId 在后台创建前为 null；前端原先只接受 string，会把成功入队误报为失败。本轮已允许 null。
- HTTP 封装补充 PATCH 与 If-Match/Idempotency-Key 请求头支持。
- Settings 持久化/Provider 配置等缺少后端路由，不能标为已接入。

## 完成标准

- 每个页面列出真实请求与尚不可用动作。
- GET、写入、错误状态、并发版本冲突、刷新后持久性有联调证据。
- 无生产 mock fallback，无假成功提示。
- 前端 build、后端 test/vet 均实际执行，浏览器结果另列。
- 测试数据不污染用户 data 目录；外部真实发布需单独授权。
