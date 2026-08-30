# 错误码规范

统一错误信封：

```json
{"error":{"code":"revision_conflict","message":"resource revision is stale","request_id":"req_...","details":{}}}
```

四个字段始终存在；`details` 无附加信息时为 `{}`。

| HTTP | code | 使用场景 |
|---:|---|---|
| 400 | `invalid_argument` | JSON、类型、必填字段或参数格式错误 |
| 401 | `unauthorized` | token 缺失或错误 |
| 403 | `forbidden` / `invalid_origin` | 已认证但 Origin/目录不允许 |
| 404 | `not_found` | 资源不存在 |
| 408 | `request_timeout` | 客户端取消或请求超时 |
| 409 | `state_conflict` / `idempotency_conflict` | 当前状态冲突；同幂等键对应不同输入 |
| 410 | `preview_expired` | 一次性导入预览已过期 |
| 412 | `revision_conflict` | `If-Match` 版本过期 |
| 413 | `body_too_large` | 请求体超过 8 MB |
| 415 | `unsupported_media_type` | 非 JSON 写请求 |
| 422 | 领域错误 | 格式正确但业务语义不成立 |
| 502 | `provider_unavailable` | Modrinth/CurseForge 上游不可用 |
| 503 | `not_ready` / `auth_not_configured` | 服务或鉴权尚未就绪 |

领域错误必须使用稳定、可枚举的 code，并在 `details.issues` 中返回结构化问题；禁止用 HTTP 200 携带业务失败，也禁止把错误吞成空列表。
