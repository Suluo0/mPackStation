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
| 后端基址 | `http://127.0.0.1:18872`（只绑回环） |
| 前端开发服务器 | `127.0.0.1:5271`（只绑回环），`/api` 由 vite 代理到后端 |
| 写鉴权 | **无**（用户 2026-10-03 定稿）：本机单用户工具，前后端都只监听回环，不引入写令牌。安全边界见 `docs/active/api/auth.md` |
| Host 白名单 | `localhost` + 字面 IP 的回环与私有网段（`127/8`、`::1`、`10/8`、`172.16/12`、`192.168/16`、`fc00::/7`），主机名需另经 `MPACK_ALLOWED_HOSTS`（逗号列表）放行，否则 400 `invalid_host` |
| Origin 白名单 | 与请求 Host 同源即合法；跨站来源（浏览器里打开的任意网页）一律 403 `invalid_origin` —— 同源策略挡得住「读响应」挡不住「发请求」，这层是 CSRF 的实际防线 |
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
{"id":"...","canonicalModId":"jei","selectionId":"selection-...","packId":"...","source":"modrinth","projectId":"...","versionId":"...","displayName":"JEI","fileName":"jei.jar","sha1":"...","status":"installed","required":true,"origin":"manual","addedAt":"...","updatedAt":"..."}
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

#### `PUT /api/system/providers/curseforge/key`

保存 CurseForge API Key(设置页「平台连接」)。保存前用一次真实平台调用验证 key
有效性;验证通过才持久化(secrets 表)并即时热注册适配器,无需重启。环境变量
`CURSEFORGE_API_KEY` 优先级高于此处保存的 key。key 值任何读取接口都不回传。

**(a) 请求参数**

```json
{"key":"<curseforge-api-key>"}
```

**(b) 校验**

- 入参:`key` 必填非空
- 出参:204 No Content

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | key 为空,或平台拒绝(key 无效/过期) | 否 |
| 502 | `provider_unavailable` | 平台不可达,未能完成验证 | 是 |
| 503 | `not_ready` | 服务未就绪 | 是 |

#### `DELETE /api/system/providers/curseforge/key`

清除保存的 key 并即时注销适配器。出参 204 No Content;异常仅 503 `not_ready`。

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

包内普通模组清单。`canonicalModId` 来自已验证 JAR 的模组声明；尚未取得文件声明时为空字符串，平台项目号不会被当作模组 ID。

**(a) 请求参数** — `packId`；内容选择器传 `includeBuiltin=true` 时返回包括内置 `canonicalModId=minecraft` 在内的完整内容来源列表。
**(b) 校验** — 出参信封 `{items: Mod[], next_cursor, total}`，分页
**(c) 异常处理** — 400 `invalid_argument` / 404 `pack_not_found` / 503 `not_ready`

#### `POST /api/packs/{packId}/mods`

添加模组。**添加即钉版**：主源版本由调用方指定并固定；服务端同时尽力解析另一平台的对应项目与版本（身份表 → 名称精确匹配），钉入 `mirrorSource`/`mirrorProjectId`（+ 库内 `mirror_version_id`），之后**永不追新**——重建包复现的是调试过的版本。镜像查不到不阻塞添加，`Mod.mirrorSource` 为 null 表示"仅单平台"。

**(a) 请求参数** — `packId`；body 由调用方构造，至少含来源与项目标识；支持 `Idempotency-Key`
**(b) 校验** — 结构错 400；引用的模组不属于该包作用域 422 `mod_invalid_reference`；出参 `Mod`（含 `mirrorSource`/`mirrorProjectId`，恒发 null 而非缺省）
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
{"items":[Project + provider + 可选 mirror],"errors":{"modrinth":"..."},"total":0,"next_cursor":null}
```

- `errors` 记录单平台失败原因，**另一侧结果照常返回**；两侧都失败才报错
- `Project`：`{id, slug, name, summary, iconUrl, downloads}`，聚合版多一个 `provider`
- **跨平台合并**：同一模组在两个平台的命中合并为一张卡——身份表（本机已确认配对 + 随二进制分发的只读知识库）优先，名称规范化（忽略大小写/空格/括号）完全相同兜底；slug 不参与配对。合并卡的 `downloads` 为两边之和，并携带 `mirror: {provider, projectId, slug, downloads}` 指向另一平台；未配对时 `mirror` 缺省。

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

#### `GET /api/packs/{packId}/mod-recommendations`

兼容知识库推荐：适用于当前包（按 MC 版本/loader 过滤、已在包内的不再出现）的常见兼容性模组，全部来自人工核实的内置知识库。

**(a) 请求参数** — `packId`
**(b) 校验** — 出参 `{items: [{name, reason, provider, projectId}], next_cursor, total}`；`provider`/`projectId` 指向可直接走添加链路的平台（Modrinth 优先，仅 CF 适配器可用时用 CF）
**(c) 异常处理** — 404 `pack_not_found` / 503 `not_ready`

**兼容知识库行为总则**：添加模组后服务端自动扫描已知问题——有 `install_mod` 解法且解法模组不在包内时**自动加装**（`Mod.origin = "compat-fix"`，活动日志注明，可移除）；无解法或解法装不上的已知问题在下次 `POST /resolve` 时以 `kind = "known_issue"` 进冲突列表（fatal→severity error，其余 warning），解法写进 summary/detail。格式与入库纪律见 `docs/active/compat-knowledge.md`。

---

### 3.4 依赖与冲突

#### `POST /api/packs/{packId}/resolve`

重新解析依赖与冲突。

**(a) 请求参数** — `packId`；body `{}`；支持 `Idempotency-Key`
**(b) 校验** — 出参 `{"lock":Lock, "status":"resolved"}`
**(c) 异常处理** — 400 / 401 / 404 `pack_not_found` / 422 `idempotency_conflict` / 502（可重试）/ 503

**依赖冲突的语义**（写进 `conflicts`，`kind='dependency'`）：

| 平台声明的依赖类型 | 目标不在包里 | 目标在包里 |
|---|---|---|
| `required` | error「缺少依赖模组：X 需要 Y」 | 无冲突 |
| `optional` | **warning**「可选依赖未安装」 | 无冲突 |
| `embedded` | 无冲突（已打进宿主 jar） | 无冲突 |
| `incompatible` | 无冲突 | error「模组互斥：X 与 Y 不能同时安装」 |

只有 error 参与构建闸门（见 `POST /build` 的 409 `build_unresolved_conflicts`），
warning 与 `status='ignored'` 都不拦。

**网络态不是包缺陷**（`kind='provider_unavailable'`，固定 `severity='warning'`）：
`resolve` 时某个模组取不到平台适配器（本机 jar 模组、CurseForge 未配 Key）或元数据拉取
失败，写的是「平台暂不可用，本轮未校验依赖：X」/「模组元数据拉取失败，本轮未校验依赖：X」。
它必须留痕（这一轮确实没校验过这个模组的依赖），但**绝不拦构建**——一次 Modrinth 抖动
把包锁成"不许构建"，用户在界面上既看不懂也修不了（缺陷 O25，schema 0024 起该 kind 才进
CHECK 枚举；在此之前它被 `conflict()` 的白名单悄悄改写成 `dependency`，从此长得像依赖问题）。

**冲突生命周期**：冲突按 `(pack_id, fingerprint)` upsert。每轮 `POST /resolve`
重新检出后，本轮**没再检出**且此前 `pending` 的冲突自动置 `resolved` 并写
`resolved_at`（例：按提示补上 fabric-api 后，那条"缺少依赖模组"就该消失，
而不是永远挂着把构建闸门堵死）。用户显式 `ignore` 的记录不被覆盖，
本轮再次检出时仍保持 `ignored`。

#### `GET /api/packs/{packId}/locks`

依赖锁定快照。

**(a) 请求参数** — `packId`；query `limit`、`cursor`
**(b) 校验** — 出参信封 `{items: Lock[], ...}`，分页
**(c) 异常处理** — 400 / 404 `pack_not_found` / 503

#### `GET /api/packs/{packId}/conflicts`

冲突列表。

**(a) 请求参数** — `packId`；query `status`（可选过滤）、`limit`、`cursor`
**(b) 校验** — 出参信封 `{items: Conflict[], ...}`，分页。
  `kind ∈ dependency | version | loader | duplicate | crash | known_issue | provider_unavailable`
  （`known_issue` 由兼容知识库写出，schema 0023 起库内 CHECK 才收下它；
  `provider_unavailable` 是平台/元数据不可用的留痕，恒为 warning，schema 0024 起入枚举）
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
**(c) 异常处理** — 404 `pack_not_found` / 404 `quest_book_not_found` / 503

空态语义（缺陷 O4，2026-09-30）：**包存在但还没保存过任务书**时返回 404 `quest_book_not_found`，
`message` 提示「在任务编辑器里保存一次草稿即可创建」，不再和「找不到整合包」混成一个码——
否则前端新包首次打开任务页只会说"整合包不存在"。同一口径也用于 `/quests/preview`、
`/quests/validate`、`/quests/apply`。

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
`revisionId` 不属于本包任务书 → 404 `quest_revision_not_found`；任务书本身不存在 → 404 `quest_book_not_found`。

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

**(a) 请求参数** — `packId`；body `packVersionId`（必填）、`files`（可选）、`exportDirName`（必填）；支持 `Idempotency-Key`
**(b) 校验**

- 结构错 400
- 存在未解决的冲突 → 422 `build_blocked`，`details` 说明阻塞原因
- 导出目录未登记 → **403** `export_dir_not_allowed`
- 出参（201）：`{"artifact":Artifact, "sourceFingerprint":"..."}`

`files` 省略时由**服务端按包内权威清单装配**（基线 D1，2026-09-30）：

- 权威链路是 `pack_mods.current_selection_id → pack_mod_selections →
  selection_platform_pins(role='primary') → platform_release_files`，调用方传什么文件都不作数；
- 产物是 `.mrpack`（`Artifact.kind = "mrpack"`），内含 `modrinth.index.json`。顶层字段名以
  **真实产物为准**（2026-09-30 实测 Fabulously Optimized v15.0.0-alpha.4）：
  `formatVersion:1 / game:"minecraft" / versionId / name / files[] / dependencies`，
  `files[]` 为 `path / hashes{sha1,sha512} / env{client,server} / downloads[] / fileSize`。
  `dependencies` 除 `minecraft` 外还带加载器（`fabric-loader`/`quilt-loader`/`forge`/`neoforge`），
  缺它安装方会把整合包当原版装、模组全不加载。jar 字节不入包，由安装方按 URL 下载。
  历史字段名 `manifestVersion`/`version` 内核仍按别名接受（`launcherCore/src/mrpack.rs`），
  但**新产物一律写 `formatVersion`/`versionId`**——严格读方（Prism 等）只认规范名。
- `hashes.sha512` 是规范必填项，来自**下载字节实测**（`measuredArchive`）而不是平台字段：
  Modrinth 的 `files[].hashes` 实测给 `{sha1,sha512}`，解析器已把它接住
  （`provider.File.SHA512`）；CurseForge 只给 sha1/sha256，那部分由实测补算。
  2026-09-30 之前入库的旧行没有 sha512（manifest 里是空串），需要重新添加模组才有。
- 包内有任何一个模组拿不到下载地址（只有本机 jar、或没有 ready 选中项）→ **422
  `build_mod_source_unresolved`**，`details.mods` 列出名字，且**不产出任何 artifact 行**；
- 一个可装配模组都没有 → **422 `build_no_mods`**。
- 只有本机 jar、没有平台选中项的本地模组（`pack_mods.current_selection_id IS NULL`）也算「无可下载地址」：
  取来源用 `LEFT JOIN pack_mod_selections` 而不是 `JOIN`，否则它会从装配清单里凭空消失，
  产出"构建成功但包里没有它"的包。当前 `.mrpack` 不嵌入本地 jar 字节，所以只能阻塞；
  把本机 jar 装进 `overrides/mods/` 是后续项。
- 禁止静默产出"构建成功但缺模组"的包：这两条 422 是 D1 的反向保障。

**(c) 异常处理**

| 状态码 | 错误码 | 触发 | 可重试 |
|---|---|---|---|
| 400 | `invalid_argument` | 结构错误 | 否 |
| 401 | `unauthorized` | 缺令牌 | 否 |
| 403 | `export_dir_not_allowed` | 导出目录未登记 | 否 |
| 404 | `pack_version_not_found` | 版本不存在 | 否 |
| 409 | `build_unresolved_conflicts` | 省略 `files` 时包里还有 error 级未结案冲突，`details.conflicts` 列出摘要 | 否（先按提示补依赖，或 `conflicts/{id}/resolve`·`/ignore`） |
| 422 | `build_blocked` | 交付检查（delivery checks）有 blocked | 否 |
| 422 | `build_mod_source_unresolved` | 省略 `files` 时有模组无下载地址 | 否（先补齐来源） |
| 422 | `build_no_mods` | 省略 `files` 时包内无可装配模组 | 否 |
| 422 | `build_lock_mismatch` | 版本已绑定锁快照，请求里的 `lockSnapshot` 与它不一致（比较时对两侧做 JSON 归一化，`GET /locks` 取回的 `snapshot` 原样回传必然一致） | 否（先取回锁快照） |
| 409 | `build_input_conflict` | 同一版本已按另一份构建输入记录在案 | 否（新建版本） |
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

**(a) 请求参数** — body `{"name":"...","directory":"<绝对路径>"}`
  （实现读的是 `directory`；此前文档写成 `path`，按 `path` 提交等于提交空目录）
**(b) 校验** — 服务端存的是 canonical 路径（macOS 上 `/tmp` → `/private/tmp`）；
  结构错 400；路径是根目录或未登记 → 403 `export_dir_not_allowed`；路径是符号链接 → 403 `export_dir_not_allowed`；
  出参 201 `{"name":"...","status":"ready"}`
**(c) 异常处理** — 400 / 401 / 403 `export_dir_not_allowed` /
  409 `export_dir_conflict`（同一绝对路径已用另一个名字批准过：`absolute_path` 上有 UNIQUE，
  改名重登记不会新增第二条，回 409 让界面列出已批准目录给用户挑，而不是 500）/ 503

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

## 6. 模组物品图标解析（2026-09-07）

`POST /api/packs/{packId}/mods/{modId}/content/icons/resolve`，请求体 `{}`，要求 Host/Origin 安全校验（无写令牌）。

响应 `200`：

- `items: ModContentItem[]`：32×32 PNG 图标，kind=`item_icon`，payload 包含 mime/data/size/source。source 为 generated 或 elements。
- `mcVersion: string`：当前包的 Minecraft 版本，原版 client JAR 严格按该版本取资源，不回退到其他版本。
- `tagIcons: Record<string,string>`：`#namespace:tag` 到代表物品 ID 的展示映射，**不是标签全集，也不改变配方语义**。
- `missing: string[]`：不能静态生成图标的模型对应 ID，可能包含辅助模型，不能当作缺失注册物品数。
- `warnings: string[]`：原版资源获取失败时返回明确提示，保留可生成的模组图标。可重复调用重试。

缺失/跨包模组返回 `404 mod_not_found`，未授权返回 `401`，非法游戏版本返回既有 invalid_argument 错误。

只从已有解析结果分页读取模型、纹理和标签，重新渲染；不会重解析模组、重写配方、删除历史或改变解析任务状态。旧 item_icon 列表是旧解析时的快照；界面图标应使用此接口以避免旧渲染缓存。模组资源覆盖同名原版资源。

首次缺少原版缓存时，从 Mojang HTTPS manifest 下载并验证元数据 SHA-1、client JAR 长度与 SHA-1；Provider 是唯一外部 HTTP 边界。下载阶段最多 40 秒；失败不留下可用的半文件。共享缓存为 `data/cache/minecraft-assets/{mcVersion}.jar`，经 temp/write/sync/rename 落盘，不捆绑到发布包，不增加数据库迁移。

静态支持：父模型继承、纹理变量、逐面 UV/旋转、GUI 变换、elements/局部旋转/缩放、深度遮挡、generated 多层透明合成、常见纵向动画条首帧。自定义 loader、运行时染色、实体渲染以及 1.21.4+ 新 client-item 动态选择语义不保证覆盖。

物品标签支持原版和当前模组声明的递归引用与 replace；仅 1.21.1 额外加入经 NeoForge 源码核实的 c:ingots/iron、c:dusts/redstone、c:gems/diamond 代表成员。未加载的其他模组/加载器标签不猜测。依据：[铁锭](https://raw.githubusercontent.com/neoforged/NeoForge/1.21.1/src/generated/resources/data/c/tags/item/ingots/iron.json)、[红石粉](https://raw.githubusercontent.com/neoforged/NeoForge/1.21.1/src/generated/resources/data/c/tags/item/dusts/redstone.json)、[钻石](https://raw.githubusercontent.com/neoforged/NeoForge/1.21.1/src/generated/resources/data/c/tags/item/gems/diamond.json)。

真实响应 fixture：`apps/web/src/api/fixtures/mod-content-icons.json`。

---

## 7. 包内物品、方块、配方与标签目录（2026-09-07）

创建整合包或修改 Minecraft／加载器版本后，服务自动提交 `catalog_init` 任务。任务先把 Minecraft 当作内置模组写入精确版本、文件、来源和解析批次，再由相同的包内解析来源生成目录。模组内容成功解析后也会提交目录重建。目录同时使用 source/built revision 与 pack config generation；过期目录读取返回 `409 catalog_stale`，构建失败不会发布为当前代次。

- `GET /api/packs/{packId}/catalog/status`：返回 `sourceRevision`、`builtRevision`、`status`、`builtAt`、`lastError`、`warnings` 和 `stale`。
- `POST /api/packs/{packId}/catalog/rebuild?locale=zh_cn`：提交持久化重建任务，返回 `202 {taskId,status}`；要求写令牌和 Host/Origin 校验。
- `GET /api/packs/{packId}/catalog?locale=zh_cn`：返回物品、方块、两类标签、配方、可用语言和构建警告。
- `GET /api/packs/{packId}/catalog/items/{itemId...}`：返回物品名称、来源、模型、图标状态及反向标签。
- `GET /api/packs/{packId}/catalog/tags/{tagId...}?registry=item`：返回物品或方块标签的展开成员和诊断。
- `GET /api/packs/{packId}/catalog/icon?itemId=minecraft:iron_ingot`：返回当前目录中生成的 PNG。

读取目录的错误码口径（缺陷 O9，2026-09-30，原先三种情况都报 409 `catalog_stale`）：

| 状态码 | 错误码 | 触发 |
|---|---|---|
| 404 | `pack_not_found` | 整合包不存在 |
| 409 | `catalog_not_built` | 包从未构建过目录（`built_revision=0`），提示点「重建目录」 |
| 409 | `catalog_stale` | 目录建过但包内容已变化，需要重建 |
| 404 | `catalog_item_not_found` | 目录里查无此物品 |
| 404 | `catalog_tag_not_found` | 目录里查无此标签 |

配方引用中 `kind=item` 表示精确物品，`kind=item_tag` 表示可以使用标签展开后的任一候选物品。显示名不改变这一语义。目录的 zod fixture 为 `apps/web/src/api/fixtures/item-catalog.json`。

---

## 8. 启动器内核接口（2026-09-30 补，基线 D6）

`POST /api/launcher/*` 早已实现但契约零覆盖，前端与后端各自的字段名/终态全靠猜，本节按 `apps/server/internal/httpapi/routes_system.go`、`internal/service/launcher_task.go` 与 `launcherCore/src/cli.rs` 的实测行为补写。

### 8.1 请求

- `POST /api/launcher/install`：`{version?, loader?, loader_version?, mirror?, java_path?, minecraft_dir, pack_id?, artifact_id?}`。
  `minecraft_dir` 必填；`version` 与 `artifact_id` **至少要有一个**，都缺即在**同步**阶段
  `400 invalid_argument`（不再入队后异步失败，基线 D4 已修）。
  `loader` 取 `vanilla|fabric|forge|neoforge|quilt`；`loader_version` 留空时启动器取 `latest`。
- `artifact_id` = 构建产物 id（`kind='mrpack'`），走**整合包导入安装**：mc 版本、加载器与模组清单
  全部来自包内 `modrinth.index.json`，调用方不再自报版本。只接受 id、不接受文件系统路径，
  这样「装的是哪一次构建」落在任务载荷与 `tasks` 行里可追溯。服务端提交时（同步）与执行时（异步）
  各校验一次：
  - 产物不存在 → **404 `artifact_not_found`**
  - `kind != 'mrpack'`（历史 zip 产物）→ **422 `install_artifact_not_mrpack`**
  - `status != 'ready'` → **409 `install_artifact_not_ready`**
  - 产物不属于请求里的 `pack_id` → **409 `install_artifact_pack_mismatch`**
  - 产物行无路径 / 文件不在磁盘 → **422 `install_artifact_path_missing`** / **409 `install_artifact_file_missing`**
  以上全部**不入队**（`[22] R6–R9` 断言 `tasks` 行数不变）。给了 `artifact_id` 且给了 `pack_id` 时，
  服务端会把 `version` 回填成包的 `mcVersion`，仅用于任务标题可读性。
  内核侧命令形如 `mpack-launcher install --mrpack <绝对路径> --dir <minecraft_dir>`。
- `POST /api/launcher/launch`：`{version, username, minecraft_dir, java_path?, xmx_mb?, pack_id?}`。
  `username` 必填（离线模式账号名），`xmx_mb` 为**纯数字 MB**（Go 侧不做 `2G` 这类单位换算，Rust `parse_memory_mb` 也接受裸数字）。
  `version` 可留空：服务端在**入队前**用 `pack_id`/`minecraft_dir` 查该目录最近一次安装记录，把
  内核实际装出来的版本目录 ID 填进任务载荷（任务因此仍可复现）。查不到 → **409
  `launcher_not_installed`**，不入队（缺陷 O2，2026-09-30）。
- `GET /api/launcher/installs?packId=&minecraftDir=`：返回 `{"installs":[{id,versionId,loader,mcVersion,packId,taskId,installedAt}]}`，
  按 `installedAt` 倒序。`versionId` 是启动要用的版本目录 ID：**带加载器时是
  `fabric-loader-<加载器版本>-<MC 版本>`**（`launcherCore/src/loader/fabric.rs:43-45`），
  不是包的 `mcVersion`——前端拿 `mcVersion` 去猜必然 `version_not_found`，这就是 O2。
- `pack_id` 只作发起上下文：`launcher_installs` 的唯一键是 `(minecraft_dir, version_id)`，
  随包删除置空（`0022_launcher_installs.sql`）。
- 两者都需要写令牌与 Host/Origin 校验，与全站一致。

### 8.2 响应

- `202 {taskId}`：任务入队（`kind = launcher_install` / `launcher_launch`），进度与日志走任务域 `GET /api/tasks/{id}`、任务日志端点。
- `400 invalid_argument`：必填项缺失或版本名非法。
- `409 duplicate_inflight`：同一 `(version, minecraft_dir)` 已有活跃安装任务，`{taskId, status, reused:true}` 带回在跑的那条。
- `409 launcher_not_installed`：`launch` 省略 `version` 且该目录没有安装记录，`message` 指向「先执行安装」；同步拒绝、不入队。
- `503 launcher_binary_missing`：**同步**拒绝，不当 202 骗前端。触发条件是 `MPACK_LAUNCHER_BIN`、`<data>/.tools/launcher/mpack-launcher(.exe)`、PATH 三处都找不到二进制（基线 D5 已修：原先硬编码 `.exe` 且在 macOS 上永远解析不到）。
- 任务终态沿用任务域枚举 `queued/running/paused/success/failed/cancelled`；失败时 `error` 为启动器错误码+消息（如 `version_not_found: 没有 1.99.0`）。

### 8.3 二进制 argv 契约（Go → Rust）

旗标名以 `launcherCore/src/cli.rs` 的 clap 定义为准，写错即 `exit 2`：

- `install --mc <version> --dir <dir> [--loader <l>] [--loader-version <v>] [--mirror <m>] [--java <path>]`
  —— install 子命令**没有** `--version`，字段名是 `mc`（2026-09-30 修复：Go 侧此前发 `--version`，真实二进制必然被 clap 拒绝，见报告 D9）。
- `launch --version <id> --dir <dir> --username <name> [--java <path>] [--xmx <mb>]`
  —— 默认 detach（`detach = !args.wait`），Go 侧不传 `--wait`，因此任务在拿到 `{"pid":N}` 后即 `success`，不等游戏退出。

### 8.4 事件协议（Rust → Go）

`launcherCore/src/protocol.rs`：stdout 为 JSON Lines，stderr 为 tracing 日志。

- `{"type":"phase","phase":<name>,"message":<str>}`，phase 取值 `preparing/resolving_version/downloading_libraries/downloading_assets/installing_loader/verifying/authenticating/await_user/authenticated/launching`；Go 侧 `phaseProgress` 把它映射成 5→100 的进度百分比，未知 phase 记 50%。
- `{"type":"result","success":true,"data":{...}}` 或 `{"type":"result","success":false,"error":<code>,"message":<str>}`，整场只有一条 result。
- 非 JSON 行（debug 构建里 cargo 泄漏的输出）直接跳过。
- result 为 `success:false` 时任务判 `failed`，**即使进程退出码是 0**；取消任务返回 `mpack-launcher 已取消: <ctx.Err>`。

协议硬约束：**整场只有一条 `result`**（即本节开头那句「整场只有一条 result」）。`Protocol::phase`
可以在库函数里多发，`result` 只能由 `main.rs` 打。2026-09-30 真机验证时发现
`install::install_vanilla` 与 `install::launch_game` 各自也打了一条 `success`，靠 Go 侧
`scanStdout` 取最后一条才没出错——已改为库函数只发 phase。

验证方式两级：
- **协议桩**（`scripts/chain-test-run.sh` 默认）：按 `cli.rs` 的 clap 语义校验 argv、按本节形状回事件，
  不下载文件、不起游戏。它证明「请求 → 同步校验 → 入队 → 入库 → worker → fork/exec → 终态」。
- **真实内核**（`scripts/verify-terminal-chain.sh`，本机 cargo 可用：brew rustup，
  toolchain `stable-aarch64-apple-darwin`，cargo 1.96；`CARGO_TARGET_DIR` 指到本地盘避开 SMB）：
  `cargo build` 出 `mpack-launcher`，用第四套隔离环境（:18874 / `/tmp/mpack-terminal`）跑完
  建包 → 加模组 → 锁 → 构建 `.mrpack` → `install --mrpack` → 对磁盘上每个 jar 复核 manifest 的 sha1，
  加 `--launch` 时再真起一次 Minecraft 并确认进程存活。
