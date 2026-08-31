# 接口契约

> 本文件定义每个接口的**应然形态**：请求参数、入参与出参校验、异常处理。
>
> - 通用规则（状态码语义、命名、分页、错误粒度、鉴权）见 [`standards.md`](./standards.md)，本文不重复
> - 实现与本文冲突时**改实现**，不允许改本文去迁就现状
> - 当前实现尚未达标的项记录在 [`implementation-status.md`](./implementation-status.md) 和带日期的 `audit/`；本文不因实现现状降低要求
>
> 每个接口包含三段：**(a) 请求参数 · (b) 校验 · (c) 异常处理**

---

## 1. 本项目特有约定

| 项 | 值 |
|---|---|
| 后端基址 | `http://127.0.0.1:18871` |
| 前端开发服务器 | `http://127.0.0.1:5273`，`/api` 由 vite 代理到后端（`changeOrigin: false`） |
| 写鉴权 | 非 GET 请求需 `X-MPack-Token`，缺失 401，未配置 503 `auth_not_configured` |
| Host 白名单 | `localhost` / `127.0.0.1` / `::1`，否则 400 `invalid_host` |
| Origin 白名单 | `http://127.0.0.1:5273` / `http://localhost:5273`，否则 403 `invalid_origin` |
| 请求体上限 | 8 MB，超出 413 |

不使用的路径：API 不带 `/api/v1` 前缀；只增字段，破坏性变更才开 `/api/v2`。

---

## 2. 领域数据结构

集中定义，接口章节直接引用。

### `Pack`

```json
{"id":"pack-...","name":"...","iconUrl":null,"mcVersion":"1.21.8","loader":"neoforge","loaderVersion":null,"description":null,"status":"active","packVersion":"0.1.0","createdAt":"2026-08-30T03:58:54.252Z","updatedAt":"2026-08-30T03:58:54.252Z"}
```

### `DashboardPack`

在 `Pack` 基础上带聚合统计：`modCount{total,installed,selected}`、`conflicts{resolved,pending}`、`edits{recipes,structures,ores,quests}`、`alerts{crashes,updatable}`、`lastEditedAt`。

### `Task`

**全局唯一结构**，所有任务相关接口都必须返回它。

```json
{"id":"task-...","type":"import-pack","title":"导入整合包","packId":"pack-...","packName":"我的包","status":"running","progress":42,"error":null,"startedAt":"2026-08-30T03:58:54.252Z","finishedAt":null}
```

- `status` 枚举：`queued` / `running` / `paused` / `success` / `failed` / `cancelled`
- `type` **开放字符串**：已映射 `index-mod` / `build-pack` / `import-pack` / `update-preflight`；未映射的 kind 输出原始值，**不允许输出空串**
- `progress` 整数 0–100

### `Activity`

```json
{"id":"activity-...","kind":"edit","text":"创建了整合包「...」","packId":"pack-...","at":"2026-08-30T03:58:54.252Z"}
```

`kind` 枚举：`add-mod` / `resolve` / `build` / `edit` / `import` / `alert`

### `Mod`

```json
{"id":"...","packId":"...","source":"modrinth","projectId":"...","versionId":"...","displayName":"JEI","fileName":"jei.jar","sha1":"...","status":"installed","required":true,"addedAt":"...","updatedAt":"..."}
```

### `Lock` / `Conflict`

```json
{"id":"...","packId":"...","schemaVersion":1,"snapshot":"<json 字符串>","sha256":"...","createdAt":"..."}
```

```json
{"id":"...","packId":"...","fingerprint":"...","kind":"...","severity":"high","status":"pending","summary":"...","detailPath":null,"detail":{},"resolvedAt":null}
```

### `ContentDocument` / `Revision` / `Validation`

```json
{"id":"...","packId":"...","kind":"recipe","slug":"...","title":"...","activeRevisionId":null,"createdAt":"...","updatedAt":"..."}
```

```json
{"id":"...","documentId":"...","state":"draft","sourceRevisionId":null,"revision":3,"payload":{},"createdAt":"..."}
```

```json
{"id":"...","revisionId":"...","status":"passed","issues":[{"code":"...","severity":"error","path":"...","message":"...","details":{}}],"affectedMods":[],"createdAt":"..."}
```

### `PackVersion` / `Artifact` / `Release` / `DeliveryCheck`

```json
{"id":"...","packId":"...","version":"0.1.0","channel":"release","changelog":"","source":"manual","lockId":"...","createdAt":"...","updatedAt":"..."}
```

```json
{"id":"...","packId":"...","packVersionId":"...","fileName":"...","sha256":"...","sourceFingerprint":"...","status":"...","kind":"...","sizeBytes":0,"createdAt":"..."}
```

```json
{"id":"...","packId":"...","packVersionId":"...","provider":"modrinth","status":"...","remoteId":"...","idempotencyKey":"...","remoteState":"...","artifactId":"...","errorCode":"","errorMessage":"","createdAt":"...","updatedAt":"..."}
```

```json
{"kind":"...","status":"passed","detail":"..."}
```

---

## 3. 接口

### 3.1 看板与系统

#### `GET /api/dashboard`

看板聚合读模型：包列表、最近编辑的包、今日已解决问题数。

**(a) 请求参数**

无。

**(b) 校验**

- 入参：无
- 出参：

```json
{"packs":[DashboardPack],"lastEditedPackId":"pack-...|null","todayResolvedCount":0}
```

`lastEditedPackId` 无值时为 `null`，不省略键。

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/tasks`

后台任务列表。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 默认 | 约束 |
|---|---|---|---|---|---|
| query | `recent` | int | 否 | 20 | 1–100 |

**(b) 校验**

- 入参：`recent` 非整数或越界 → 400 `invalid_argument`
- 出参：`ListEnvelope<Task>`（见 [`dto.md`](./dto.md)）；即使当前页面只取 `items`，也必须保留 `next_cursor` 和 `total`

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | `recent` 非法 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/activities`

最近操作动态。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 默认 | 约束 |
|---|---|---|---|---|---|
| query | `limit` | int | 否 | 10 | 1–100 |

**(b) 校验**

- 入参：`limit` 非整数或越界 → 400 `invalid_argument`
- 出参：`ListEnvelope<Activity>`（见 [`dto.md`](./dto.md)）；当前页面只取 `items`，但必须返回 `next_cursor: null` 和 `total`

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | `limit` 非法 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/system/health`

环境自检，驱动看板顶部横幅。

**(a) 请求参数**

无。

**(b) 校验**

- 入参：无
- 出参：

```json
{"curseforgeKeyConfigured":false,"modrinthReachable":false,"curseforgeReachable":false,"storageWritable":true,"storageFreeBytes":370096562176}
```

- 平台可达性探测失败时返回 `false`，**不允许**因探测失败而让整个接口 500

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/system/status`

平台可达性、缓存体积、剩余空间。设置页「平台连接」「存储与缓存」应接此接口。

**(a) 请求参数**

无。

**(b) 校验**

- 入参：无
- 出参：

```json
{"modrinthReachable":false,"curseforgeReachable":false,"cacheSizeBytes":0,"storageFreeBytes":370096562176}
```

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/onboarding`

迎新四步完成状态。

**(a) 请求参数**

无。

**(b) 校验**

- 入参：无
- 出参：

```json
{"steps":{"curseforgeKey":false,"firstPack":false,"firstMod":false,"prismAccount":false}}
```

四个键**必须全部存在**，未知步骤不出现在 `steps` 里。

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `PUT /api/onboarding`

迎新步骤打勾。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 约束 |
|---|---|---|---|---|
| body | `steps` | object | 是 | 键为步骤名，值为 boolean |

```json
{"steps":{"firstPack":true}}
```

**(b) 校验**

- 入参：
  - `steps` 缺失或不是对象 → 400 `invalid_argument`
  - 键不在已知步骤集合内 → 422 `onboarding_unknown_step`
  - `prismAccount` 由后端根据本地 `accounts.json` 自动置位，**前端写入该键会被拒绝** → 422 `onboarding_step_readonly`
- 出参：最新的 onboarding 状态（同 GET）

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 结构错误 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 422 | `onboarding_unknown_step` | 未知步骤名 | 否 |
| 422 | `onboarding_step_readonly` | 写入只读步骤 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/meta/mc-versions`

MC 版本候选，创建包下拉用。

**(a) 请求参数**

无。

**(b) 校验**

- 入参：无
- 出参：`string[]`，降序排列

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `POST /api/tools/prism/install`

后台任务方式安装 Prism 启动器。

**(a) 请求参数**

空对象 `{}`。支持 `Idempotency-Key`。

**(b) 校验**

- 入参：body 必须是对象（可为空）
- 出参（202）：`{"started":true,"taskId":"task-...","reused":false}`
- 成败以后台任务日志为准，本接口只表示"已入队"

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | body 不是对象 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 409 | `task_invalid_transition` | 已有安装任务在跑 | 否 |
| 422 | `idempotency_conflict` | 同键不同输入 | 否 |
| 503 | `not_ready` | 安装器未装配 | 是 |

---

#### `POST /api/tools/prism/login`

唤起 Prism GUI 让用户登录微软账号。

**(a) 请求参数**

空对象 `{}`。

**(b) 校验**

- 入参：body 必须是对象
- 出参（200）：`{"launched":true}`
- 前端随后轮询 `GET /api/onboarding` 等待 `prismAccount` 自动置位

> **规范**：轮询必须有终止条件——页面卸载、组件销毁，或超过 10 分钟。实现状态记录在 `implementation-status.md`。

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | body 不是对象 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 503 | `not_ready` | Prism 未安装 | 是 |

---

### 3.2 整合包

#### `GET /api/packs`

包列表。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 默认 | 约束 |
|---|---|---|---|---|---|
| query | `limit` | int | 否 | 20 | 1–100 |
| query | `cursor` | string | 否 | — | 不透明游标 |

**(b) 校验**

- 入参：`limit` 越界 → 400 `invalid_argument`；`cursor` 非法 → 400 `invalid_argument`
- 出参：信封 `{items: Pack[], next_cursor, total}`，支持分页

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 分页参数非法 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `POST /api/packs`

新建整合包。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 约束 |
|---|---|---|---|---|
| body | `name` | string | 是 | 1–128 字符（按 rune 计），不可与同实例内其他包重名 |
| body | `mcVersion` | string | 是 | 必须是 `/api/meta/mc-versions` 返回的取值之一 |
| body | `loader` | string | 是 | `forge` / `neoforge` / `fabric` / `quilt` |
| body | `loaderVersion` | string | 否 | 留空由平台选择匹配稳定版 |
| body | `description` | string | 否 | ≤ 2000 字符 |

支持 `Idempotency-Key`。

**(b) 校验**

- 入参：
  - 字段缺失或类型错 → 400 `invalid_argument`
  - `loader` 不在枚举内 → 400 `invalid_argument`（结构层面，因为是封闭枚举）
  - 名称重复 → 422 `pack_name_duplicate`（语义层面）
  - `mcVersion` 不在候选内 → 422 `pack_unsupported_mc_version`
- 出参（201）：`Pack`

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 必填缺失 / 类型错 / loader 非法 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 415 | `unsupported_media_type` | Content-Type 非 json | 否 |
| 422 | `pack_name_duplicate` | 同名包已存在 | 否 |
| 422 | `pack_unsupported_mc_version` | MC 版本不在候选 | 否 |
| 422 | `idempotency_conflict` | 同键不同输入 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `GET /api/packs/{packId}`

包详情。`packId` 缺失或不存在 → **404** `pack_not_found`（不是 400）。

**(a) 请求参数** — 路径参数 `packId`
**(b) 校验** — 出参 `Pack`
**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 404 | `pack_not_found` | 包不存在 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `PATCH /api/packs/{packId}`

更新包。字段均可选，未提供的不变。

**(a) 请求参数** — `packId`；body：`name` / `mcVersion` / `loader` / `loaderVersion` / `description`，约束同 `POST`
**(b) 校验** — 同 `POST`（结构 400，语义 422）；出参 `Pack`
**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 结构错误 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 404 | `pack_not_found` | 包不存在 | 否 |
| 422 | `pack_name_duplicate` | 重名 | 否 |
| 422 | `pack_unsupported_mc_version` | 版本不支持 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `DELETE /api/packs/{packId}`

删除包。

**(a) 请求参数** — 路径参数 `packId`
**(b) 校验** — 出参：**204，响应体必须为空**（前端 `del()` 不解析 body）
**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 401 | `unauthorized` | 缺令牌 | 否 |
| 404 | `pack_not_found` | 包不存在 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

---

#### `POST /api/packs/import/inspect`

导入第一步：解析来源，产出待确认预览。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 约束 |
|---|---|---|---|---|
| body | `source` | string | 是 | `curseforge_url` / `modrinth_url` / `local_zip` |
| body | `url` | string | 条件 | `source` 为链接类时必填，必须 https |
| body | `content` | string | 条件 | `source` 为 `local_zip` 时必填，zip 的 base64（不含 `data:` 前缀） |

**(b) 校验**

- 入参：
  - 字段缺失/类型错 → 400 `invalid_argument`
  - `source` 不在枚举 → 400 `invalid_argument`
  - URL 不是 https 或无法解析 → 400 `invalid_argument`（结构）
  - URL 是 https 但域名不属于对应平台 → **422** `import_invalid_source`（语义）
  - zip 内容非法或解压后条目数超限 → 422 `import_invalid_source`
- 出参：

```json
{"id":"import-...","token":"...","inputHash":"...","source":"local_zip","expiresAt":"2026-08-30T04:03:54.252Z","entryCount":42,"packName":"我的包"}
```

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 结构/格式错误 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 413 | `payload_too_large` | zip 超 8 MB | 否 |
| 422 | `import_invalid_source` | 域名不匹配 / 内容非法 | 否 |
| 422 | `unsafe_archive` | 未通过安全检查（路径穿越等） | 否 |
| 502 | `provider_unavailable` | 上游平台不可达 | **是** |
| 503 | `not_ready` | 导入器未装配 | 是 |

---

#### `POST /api/packs/import`

导入第二步：确认并入队。**必须支持幂等**。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 约束 |
|---|---|---|---|---|
| body | `previewId` | string | 是 | 来自 inspect |
| body | `token` | string | 是 | 一次性凭证 |
| body | `inputHash` | string | 是 | 绑定输入内容 |
| header | `Idempotency-Key` | string | **是** | 本接口强制要求 |

**(b) 校验**

- 入参：
  - 字段缺失 → 400 `invalid_argument`
  - 幂等键缺失 → 400 `invalid_argument`
  - `token` 与 `previewId` 不匹配 → 400 `invalid_argument`
  - `inputHash` 与预览记录不符 → 422 `import_input_mismatch`
- 出参（202）：

```json
{"importId":"import-...","taskId":"task-...","packId":"pack-...","reused":false}
```

> **规范**：响应中**不允许**内嵌 `task` 对象。任务信息统一用 `taskId` 去 `GET /api/tasks` 取。实现偏差记录在日期化审计中。

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 结构错误 / token 不匹配 / 缺幂等键 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 409 | `import_preview_consumed` | 一次性凭证已用（同输入）→ 应返回原结果 | 否 |
| 410 | `import_preview_expired` | 预览已过期 | 否 |
| 422 | `import_input_mismatch` | inputHash 不符 | 否 |
| 422 | `idempotency_conflict` | 同键不同输入 | 否 |
| 503 | `not_ready` | 导入器未装配 | 是 |

---

### 3.3 模组

#### `GET /api/packs/{packId}/mods`

包内模组清单。

**(a) 请求参数** — `packId`；query `limit`（默认 50，上限 200）、`cursor`
**(b) 校验** — 出参信封 `{items: Mod[], next_cursor, total}`，分页
**(c) 异常处理** — 400 `invalid_argument` / 404 `pack_not_found` / 503 `not_ready`

#### `POST /api/packs/{packId}/mods`

添加模组。

**(a) 请求参数** — `packId`；body 由调用方构造，至少含来源与项目标识；支持 `Idempotency-Key`
**(b) 校验** — 结构错 400；引用的模组不属于该包作用域 422 `mod_invalid_reference`；出参 `Mod`
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 422 `mod_invalid_reference` / 422 `idempotency_conflict` / 502 `provider_unavailable`（可重试）/ 503

#### `PATCH /api/packs/{packId}/mods/{modId}`

修改模组（启停、版本选择）。

**(a) 请求参数** — `packId`、`modId`；body 含待改字段
**(b) 校验** — 结构错 400；目标版本与包的 MC 版本/加载器不兼容 → 422 `mod_incompatible_version`；出参 `Mod`
**(c) 异常处理** — 400 / 401 / 404 `mod_not_found` / 422 `mod_incompatible_version` / 502（可重试）/ 503

#### `DELETE /api/packs/{packId}/mods/{modId}`

移除模组。出参 **204 空响应体**。

**(c) 异常处理** — 401 / 404 `mod_not_found` / 503

#### `GET /api/packs/{packId}/mod-search`

双平台聚合搜索。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 默认 | 约束 |
|---|---|---|---|---|---|
| path | `packId` | string | 是 | — | — |
| query | `q` | string | 否 | — | 关键词 |
| query | `limit` | int | 否 | 20 | 1–100 |
| query | `cursor` | string | 否 | — | 不透明游标 |
| query | `sort` | string | 否 | relevance | relevance / downloads / updated |

**(b) 校验**

- 入参：参数类型错 → 400 `invalid_argument`；`sort` 不在枚举 → 400
- 出参：

```json
{"items":[Project + provider],"errors":{"modrinth":"..."},"total":0,"next_cursor":null}
```

- `errors` 记录单平台失败原因，**另一侧结果照常返回**；两侧都失败才报错
- `Project`：`{id, slug, name, summary, iconUrl, downloads}`，聚合版多一个 `provider`

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 参数非法 | 否 |
| 404 | `pack_not_found` | 包不存在 | 否 |
| 502 | `provider_unavailable` | **两侧平台都失败** | **是** |
| 503 | `not_ready` | 服务未就绪 | 是 |

#### `GET /api/packs/{packId}/mod-versions`

某模组的版本候选。

**(a) 请求参数** — `packId`；query `provider`（必填）、`projectId`（必填）
**(b) 校验** — 缺参 400 `invalid_argument`；出参 `{items: ModVersion[]}`，不分页
**(c) 异常处理** — 400 / 404 `pack_not_found` / 502（可重试）/ 503

---

### 3.4 依赖与冲突

#### `POST /api/packs/{packId}/resolve`

重新解析依赖与冲突。

**(a) 请求参数** — `packId`；body `{}`；支持 `Idempotency-Key`
**(b) 校验** — 出参 `{"lock":Lock, "status":"resolved"}`
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 422 `idempotency_conflict` / 502（可重试）/ 503

#### `GET /api/packs/{packId}/locks`

依赖锁定快照。

**(a) 请求参数** — `packId`；query `limit`、`cursor`
**(b) 校验** — 出参信封 `{items: Lock[], ...}`，分页
**(c) 异常处理** — 400 / 404 `pack_not_found` / 503

#### `GET /api/packs/{packId}/conflicts`

冲突列表。

**(a) 请求参数** — `packId`；query `status`（可选过滤）、`limit`、`cursor`
**(b) 校验** — 出参信封 `{items: Conflict[], ...}`，分页
**(c) 异常处理** — 400 / 404 `pack_not_found` / 503

#### `GET /api/packs/{packId}/health`

包健康摘要。

**(a) 请求参数** — `packId`
**(b) 校验** — 出参 `{packId, mods, installed, pendingErrors, pendingWarnings, healthy}`
**(c) 异常处理** — 404 `pack_not_found` / 503

#### `POST /api/packs/{packId}/conflicts/{conflictId}/resolve` · `/ignore`

单个冲突标记已解决 / 忽略。**当前前端未接**，规范如下。

**(a) 请求参数** — `packId`、`conflictId`；body `{}`
**(b) 校验** — 出参更新后的 `Conflict`
**(c) 异常处理** — 401 / 404 `conflict_not_found` / 409 `conflict_already_resolved` / 503

---

### 3.5 内容编辑

#### `GET /api/packs/{packId}/content`

内容文档列表。

**(a) 请求参数** — `packId`；query `kind`（可选，`recipe` / `structure` / `ore`）、`limit`、`cursor`
**(b) 校验** — `kind` 不在枚举 → 400 `invalid_argument`；出参信封 `{items: ContentDocument[], ...}`
**(c) 异常处理** — 400 / 404 `pack_not_found` / 503

#### `GET /api/packs/{packId}/content/{documentId}`

单文档 + 当前修订。

**(a) 请求参数** — `packId`、`documentId`
**(b) 校验** — 出参 `{document: ContentDocument, revision: Revision|null}`
**(c) 异常处理** — 404 `content_not_found` / 503

#### `POST /api/packs/{packId}/content`

新建内容文档。

**(a) 请求参数** — `packId`；body `kind`、`slug`、`title`；支持 `Idempotency-Key`
**(b) 校验** — 结构错 400；`slug` 在同包内重复 → 422 `content_duplicate_slug`；出参（201）`ContentDocument`
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 422 `content_duplicate_slug` / 422 `idempotency_conflict` / 503

#### `PUT /api/packs/{packId}/content/{documentId}/draft`

存草稿，**强制乐观锁**。

**(a) 请求参数**

| 位置 | 名称 | 类型 | 必填 | 约束 |
|---|---|---|---|---|
| path | `packId` / `documentId` | string | 是 | — |
| header | `If-Match` | string | 是 | `"<revision号>"`，带英文双引号 |
| body | `payload` | any | 是 | 文档内容 |

**(b) 校验**

- 入参：
  - 缺 `If-Match` → **400** `invalid_argument`（这是结构要求，不是前置条件失败）
  - `If-Match` 值与当前 revision 不符 → **412** `revision_conflict`
  - `payload` 结构错 → 400 `invalid_argument`
  - `payload` 能解析但不符合该 kind 的 schema → 422 `content_invalid`，`details.issues` 携带逐项问题
- 出参：`Revision`

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 缺 If-Match / payload 结构错 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 404 | `content_not_found` | 文档不存在 | 否 |
| 412 | `revision_conflict` | revision 已陈旧 | 否 |
| 422 | `content_invalid` | 内容 schema 校验失败 | 否 |
| 503 | `not_ready` | 服务未就绪 | 是 |

#### `POST /api/packs/{packId}/content/{documentId}/validate`

校验草稿。

**(a) 请求参数** — `packId`、`documentId`；query `revisionId`（可选，默认校验当前草稿）
**(b) 校验** — 出参 `Validation`；`details` 里的 `issues` 逐项列出问题
**(c) 异常处理** — 404 `content_not_found` / 404 `validation_revision_not_found` / 503

#### `POST /api/packs/{packId}/content/{documentId}/apply`

应用草稿为正式修订。

**(a) 请求参数** — `packId`、`documentId`；query `revisionId`（可选）；支持 `Idempotency-Key`
**(b) 校验** — 出参 `{"status":"applied","revision":Revision}`
**(c) 异常处理** — 401 / 404 `content_not_found` / 409 `content_not_validated`（未先校验，视实现而定）/ 412 `revision_conflict` / 422 `content_invalid` / 422 `idempotency_conflict` / 503

#### `POST /api/packs/{packId}/content/{documentId}/rollback`

回滚到指定修订。

**(a) 请求参数** — `packId`、`documentId`；body `{"revisionId":"..."}`
**(b) 校验** — 出参新建的 `Revision`（回滚是创建新修订，不改写历史）
**(c) 异常处理** — 400 / 401 / 404 `content_not_found` / 404 `revision_not_found` / 503

#### `GET /api/packs/{packId}/content/{documentId}/history`

修订历史。

**(a) 请求参数** — `packId`、`documentId`；query `limit`、`cursor`
**(b) 校验** — 出参信封 `{items: Revision[], ...}`
**(c) 异常处理** — 404 `content_not_found` / 503

---

### 3.6 任务书

#### `GET /api/packs/{packId}/quests`

任务书当前内容。

**(a) 请求参数** — `packId`
**(b) 校验** — 出参任务书图模型（章节 / 节点 / 边 / 奖励）+ 当前 revision 元信息
**(c) 异常处理** — 404 `pack_not_found` / 503

#### `PUT /api/packs/{packId}/quests/draft`

存草稿，**强制乐观锁**（同内容草稿）。

**(a) 请求参数** — `packId`；header `If-Match`（必填）；body 图模型
**(b) 校验**
- 缺 `If-Match` → 400
- revision 不符 → **412** `revision_conflict`
- 结构错 → 400
- 图模型语义问题（有环、孤立节点、跨包引用）→ **422** `quest_cycle` / `quest_orphan_node` / `quest_invalid_reference`，`details.issues` 逐项列出
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 412 `revision_conflict` / 422 `quest_cycle` / 422 `quest_orphan_node` / 422 `quest_invalid_reference` / 503

#### `POST /api/packs/{packId}/quests/validate`

校验草稿。出参 `Validation`，`details.issues` 逐项列出环、孤立节点、跨包引用。

#### `POST /api/packs/{packId}/quests/apply`

应用。支持 `Idempotency-Key`。出参 `{"status":"applied"}`。

**(c) 异常处理** — 401 / 404 `pack_not_found` / 412 `revision_conflict` / 422 `quest_cycle` / 422 `idempotency_conflict` / 503

#### `POST /api/packs/{packId}/quests/rollback`

回滚。body `{"revisionId":"..."}`，出参新建的 `Revision`。

#### `GET /api/packs/{packId}/quests/history`

修订历史，出参信封 `{items: Revision[], ...}`。

#### `GET /api/packs/{packId}/quests/preview`

任务书预览数据（给前端渲染流程图用），出参结构由契约单独定义。

---

### 3.7 打包与发布

#### `GET /api/packs/{packId}/delivery-checks`

交付前检查结果。

**(a) 请求参数** — `packId`；query `packVersionId`（可选）
**(b) 校验** — 出参信封 `{items: DeliveryCheck[], ...}`
**(c) 异常处理** — 404 `pack_not_found` / 503

#### `POST /api/packs/{packId}/delivery-checks/run`

重新执行交付检查。

**(a) 请求参数** — `packId`；body `{"packVersionId":"..."}`（可选）
**(b) 校验** — 出参 `{items: DeliveryCheck[]}`
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 404 `pack_version_not_found` / 503

#### `POST /api/packs/{packId}/versions`

新建版本。

**(a) 请求参数** — `packId`；body `version`、`channel`、`changelog`；支持 `Idempotency-Key`
**(b) 校验** — 结构错 400；版本号格式非法或已存在 → 422 `pack_version_conflict`；出参（201）`PackVersion`
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 422 `pack_version_conflict` / 422 `idempotency_conflict` / 503

#### `GET /api/packs/{packId}/versions`

版本列表，出参信封 `{items: PackVersion[], ...}`。

#### `POST /api/packs/{packId}/build`

构建可复现产物。

**(a) 请求参数** — `packId`；body `packVersionId`（必填）、`files`（可选）；支持 `Idempotency-Key`
**(b) 校验**

- 结构错 400
- 存在未解决的冲突 → 422 `build_blocked`，`details` 说明阻塞原因
- 导出目录未登记 → **403** `export_dir_not_allowed`
- 出参（201）：`{"artifact":Artifact, "sourceFingerprint":"..."}`

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 结构错误 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 403 | `export_dir_not_allowed` | 导出目录未登记 | 否 |
| 404 | `pack_version_not_found` | 版本不存在 | 否 |
| 422 | `build_blocked` | 有未解决冲突 / 内容未应用 | 否 |
| 422 | `idempotency_conflict` | 同键不同输入 | 否 |
| 503 | `not_ready` | 构建器未装配 | 是 |

#### `GET /api/packs/{packId}/artifacts`

产物列表。query `packVersionId` 可选。出参信封 `{items: Artifact[], ...}`。

#### `GET /api/packs/{packId}/artifacts/{artifactId}/download`

下载产物。出参二进制流，`Content-Disposition` 带文件名。

**(c) 异常处理** — 404 `artifact_not_found` / 410 `artifact_expired`（产物已被 GC）/ 503

#### `POST /api/packs/{packId}/publish/{provider}`

发布到 `curseforge` 或 `modrinth`。

**(a) 请求参数** — `packId`、`provider`；body `packVersionId`、`artifactId`；支持 `Idempotency-Key`
**(b) 校验** — 结构错 400；`provider` 不支持 → 400；产物未就绪 → 422 `release_artifact_not_ready`；出参（202）`Release`
**(c) 异常处理** — 400 / 401 / 404 `artifact_not_found` / 422 `release_artifact_not_ready` / 422 `idempotency_conflict` / **502 `provider_unavailable`（可重试）** / 503

#### `POST /api/packs/{packId}/publish/{provider}/async`

异步发布，出参（202）`{"taskId":"...","reused":false}`。release 记录由后台任务执行时才创建，入队响应不含 `releaseId`；事后用 `GET /api/packs/{packId}/releases` 或任务日志查询。异常同上。

#### `GET /api/packs/{packId}/releases`

发布记录。query `packVersionId` 可选。出参信封 `{items: Release[], ...}`。

#### `GET /api/releases/{releaseId}`

发布详情，出参 `Release`。异常：404 `release_not_found` / 503。

#### `POST /api/releases/{releaseId}/poll`

轮询远端发布状态，出参更新后的 `Release`。

**(c) 异常处理** — 401 / 404 `release_not_found` / **502（可重试）** / 503

#### `POST /api/releases/{releaseId}/retry`

重试失败发布，出参 `Release`。异常：404 / 409 `release_not_retryable`（状态不允许重试）/ 502（可重试）/ 503。

#### `POST /api/export-dirs`

登记导出目录。

**(a) 请求参数** — body `{"name":"...","path":"<绝对路径>"}`
**(b) 校验** — 结构错 400；路径是根目录或未登记 → 403 `export_dir_not_allowed`；路径是符号链接 → 403 `export_dir_not_allowed`
**(c) 异常处理** — 400 / 401 / 403 `export_dir_not_allowed` / 503

---

### 3.8 任务控制

四个接口参数与异常完全一致，合并说明。

`POST /api/tasks/{taskId}/pause` · `/resume` · `/cancel` · `/retry`

**(a) 请求参数** — 路径参数 `taskId`；body `{}`；支持 `Idempotency-Key`

**(b) 校验**

- 出参：**`Task`**（全局唯一结构，见 §2）
- 返回全局唯一的 `Task` DTO；实现差异记录在 `implementation-status.md`，不在此处改变契约

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | body 不是对象 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 404 | `task_not_found` | 任务不存在 | 否 |
| 408 | `request_canceled` | 上下文取消或超时 | 是 |
| 409 | `task_invalid_transition` | 状态流转非法（如暂停已完成的任务） | 否 |
| 409 | `task_lease_lost` | 租约失效 | **是** |
| 422 | `idempotency_conflict` | 同键不同输入 | 否 |
| 500 | `task_unknown_kind` | 任务类型未注册 | 否 |
| 503 | `not_ready` | 任务服务未装配 | 是 |

#### `GET /api/tasks/{taskId}` · `GET /api/tasks/{taskId}/log`

任务详情与日志。出参分别为 `Task` 与日志条目数组。异常：404 `task_not_found` / 503。

---

## 4. 前端调用约定

### 4.1 非人工触发的调用

以下请求无需用户点击即发生，排查"莫名在刷接口"时先看这里。

| 触发时机 | 请求 | 频率 | 位置 |
|---|---|---|---|
| 应用启动 | `GET /api/onboarding` | 一次 | `AppShell.tsx:34` |
| 路由不在包内页面 | `GET /api/packs` —— 侧栏导航定位第一个真实包 | 路由变化 | `AppShell.tsx:42` |
| 打开看板 | 并发 5 个：dashboard / tasks / activities / health / status | 一次 | `DashboardPage.tsx` |
| 存在 running 任务 | `GET /api/tasks` | 每 3 秒，页面不可见时跳过 | `DashboardPage.tsx:61` |
| 唤起 Prism 登录后 | 重复拉 onboarding | 每 5 秒，最多 120 次 | `OnboardingChecklist.tsx:30` |
| 进入包内页面 | 并发加载（发布页一次 3 个） | 每次进页面 | `PackPages.tsx` |

**规范要求**：所有轮询必须带终止条件（组件卸载 / 页面隐藏 / 超时）。当前 Prism 轮询只靠计数上限，违反此条。

### 4.2 设置页

`SettingsPage` 只保留有真实后端的区块（决策 D-11）：

| 界面元素 | 接口 |
|---|---|
| 平台连接状态 | `GET /api/system/status` 的 `modrinthStatus` / `curseforgeStatus`（三态 unknown/ok/unavailable）与 reachability |
| 缓存体积 / 剩余空间 | `GET /api/system/status` 的 `cacheSizeBytes` / `storageFreeBytes` |

「清理缓存」「默认包配置」「恢复默认」「界面」四个无后端支撑的 UI 区块已移除；需要时另行立项 `/api/settings` 与 cache purge 接口。

---

## 5. 历史实现符合度索引

历史缺口的详细状态不再维护在接口正文中，统一见 [`implementation-audit-2026-08-30.md`](./implementation-audit-2026-08-30.md) 及后续 `audit/` 文件。本节仅保留索引，避免契约与状态重复漂移。

当前待决策项和执行顺序记录在项目计划/issue 中，不作为接口契约的一部分。
