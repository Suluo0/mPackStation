# DTO 约定

同一领域对象在所有接口复用同一对外结构。Go DTO 必须显式声明 `json` tag；前端响应必须经过对应 zod schema。

## Task

```json
{"id":"task_...","type":"build-pack","title":"构建整合包","packId":"pack_...","packName":"示例包","status":"running","progress":42,"error":null,"startedAt":"2026-08-30T03:58:54Z","finishedAt":null}
```

`status`：`queued` / `running` / `paused` / `success` / `failed` / `cancelled`。`type` 是开放字符串，未知值原样返回，不得变成空串。无值字段返回 `null`，不省略。

## ListEnvelope

所有列表统一返回：

```json
{"items":[],"next_cursor":null,"total":0}
```

即使当前实现不分页，`next_cursor` 也返回 `null`。`total` 始终为整数。
