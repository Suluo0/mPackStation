# API 契约追平改造设计(2026-08-30)

> 流程:superpowers brainstorming(用户授权代理拍板)→ 本文档 → 契约文档同步 → 分批实施 → curl 实测。
> 范围纪律:只做契约已点名或审计已记录的差距;不造契约没有的新功能;改实现不改契约(除本文明确拍板修改契约的两处)。

## 1. 目标

让现有系统实然状态追平 docs/api 契约:鉴权、错误码粒度、状态码语义、Task DTO 归一、列表信封、null 语义、前端 zod 校验与设置页接线。验收 = 后端 go build/vet/test/gofmt 全绿 + 前端 tsc/build 通过 + 起真实服务 curl 全链路实测通过。

## 2. 本轮拍板决策(用户授权代理决定)

| 编号 | 决策 | 结论 | 理由 |
|---|---|---|---|
| D-5 | 错误码细粒度 | **采用** | 契约已按细粒度写;前端需要按 code 分支(revision_conflict 提示等);通用码无法支撑 |
| D-9 | 契约先行、代码分批追平 | **采用分批** | 本轮即第一批;implementation-status.md + 日期化审计跟踪 |
| D-10 | `POST /api/packs/{id}/duplicate` | **删除路由** | 契约无此接口;前端菜单只是 message 占位未调用;501 占位违反"不允许 200 包失败/占位演戏"精神。需要时另行立项实现 |
| D-11 | 设置页 | **接真实 system/status;砍掉无后端支撑的 UI** | "平台连接""存储与缓存"接 `GET /api/system/status`;"清理缓存""默认包配置""恢复默认""界面"无对应接口,移除 UI;后续需要时再立项 `/api/settings` 与 cache purge |
| D-12 | 异步发布响应 | **改契约:只返 `{taskId, reused}`** | release 记录由 worker 执行时才创建,入队时刻拿不到 releaseId;预建 release 会改动发布状态机,超出本轮范围。契约相应调整并注明 release 事后从 `GET /api/packs/{id}/releases` 查 |
| D-13 | 任务 status 映射 | **queued 不再折叠进 running** | 契约枚举含 queued;当前 queued/leased 都映射 running 丢失语义。queued→`queued`,leased→`running` |
| D-14 | 413 错误码命名 | **统一 `payload_too_large`** | contract.md 与 errors.md 原本不一致(一个 payload_too_large 一个 body_too_large),取更贴 HTTP 语义的前者,改 errors.md |
| D-15 | 分页参数越界 | **`limit`/`recent` >100 返回 400** | 契约写"越界 → 400 invalid_argument",当前实现 clamp 到 100  silently,违反契约 |

## 3. 未拍板但本轮确认的现状记录

- `mod_incompatible_version`(422):PATCH mods 的版本兼容性校验需要平台版本数据支撑,当前实现没有该校验。本轮不硬造,记入 implementation-status 缺口。
- `GET /api/packs/{packId}/quests/preview` 出参结构契约注明"单独定义",本轮不动。
- `PUT quests/draft` 当前返回 `{revision, issues}` 包裹,契约未细化,保持现状。

## 4. 分批方案

### 批 A — 鉴权 token(后端 + 前端注入)

- `cmd/server/main.go`:启动时解析写令牌:`MPACK_TOKEN` 环境变量优先;否则读 `<data>/runtime-token`;不存在则 crypto/rand 生成 32 字节 hex 写入(0600)。把 token 传入 httpapi。
- `httpapi.go`:`NewRouterWithProviders`/`newRouter` 增加 token 参数;`securityMiddleware` 改为闭包持 token;删除 `"test"` 兜底;token 为空 → 503 `auth_not_configured`。
- `apps/web/vite.config.ts`:dev 时 `fs.readFileSync` 读 `../../data/runtime-token`,经 `define` 注入 `__MPACK_TOKEN__`;`VITE_MPACK_TOKEN` 环境变量优先。
- `apps/web/src/api/http.ts`:删除 `'test'` 回退,token 只来自注入;`vite-env.d.ts` 注释更新。

### 批 B — 错误码与状态码(后端)

- 引入 `service.DomainError{Status int, Code string, Message string, Details map[string]any}`;`writeServiceError` 优先识别并按其输出,哨兵映射保留为兜底。
- revision 冲突 409 → **412** `revision_conflict`。
- 校验失败:`service.ValidationError{Domain("content"|"quest"), Issues []ValidationIssue}`;→ 422,content 域 `content_invalid`,quest 域按主 issue code 映射 `quest_cycle`/`quest_orphan_node`/`quest_invalid_reference`(兜底 `quest_invalid`);`details.issues` 原样携带。`ErrCrossPackReference` → 422 `quest_invalid_reference`。
- 包:`CreatePack`/`UpdatePack` 服务层查重 → 422 `pack_name_duplicate`;`mcVersion` 不在 `MCVersions()` 候选 → 422 `pack_unsupported_mc_version`。
- onboarding:未知步骤 → 422 `onboarding_unknown_step`;写 `prismAccount` → 422 `onboarding_step_readonly`(当前统统 400)。
- 内容:slug 同包重复(服务层先查)→ 422 `content_duplicate_slug`。
- 构建:`ErrDeliveryBlocked` 在 build 路径 → 422 `build_blocked`,`details` 带阻塞检查项。
- 导入:
  - `validateImportURL` 拆两层:非 https/解析失败 → 400 `invalid_argument`;域名不属于平台 → 422 `import_invalid_source`。
  - Confirm:token 与 previewId 不匹配 → 400 `invalid_argument`;`inputHash` 不符 → 422 `import_input_mismatch`;预览过期 → **410** `import_preview_expired`;已消费且同输入 → **409** `import_preview_consumed` 并返回原结果(见下)。
  - 迁移 `0004`:`import_previews` 加 `consumed_task_id TEXT`;Confirm 消费成功后回写 taskId;重复确认同输入时查出原任务返回 `{importId, taskId, packId, reused: true}`。
- 资源级 404 细分:not found 按资源返回 `pack_not_found`/`mod_not_found`/`conflict_not_found`/`content_not_found`/`task_not_found`(已有)/`release_not_found`/`artifact_not_found`,经 DomainError 或路由层包装实现。
- `POST /api/tools/prism/install` 已有安装任务在跑 → 409 `task_invalid_transition`(契约已写,核对现状)。
- 删除 duplicate 501 路由。
- `queryLimit`:>100 → -1(由调用方返 400)。
- `decodeJSON`:命中 `*http.MaxBytesError` → 413 `payload_too_large`;非法 JSON 的 code 从 `invalid_json` 统一为 `invalid_argument`(errors.md 只有 invalid_argument)。
- `WriteError` 公共函数的 `request_id` 不再写空串(接受 request 或并入 apiError)。

### 批 C — Task DTO 归一 + 信封 + null 语义(后端)

- `task.HTTPAdapter.view` 改输出契约 `Task` 结构:`{id,type,title,packId,packName,status,progress(int 0-100),error,startedAt,finishedAt}`;额外字段(message/attempt/maxAttempts/recoverCount/errorCode/createdAt/updatedAt)从公开响应移除,需要细节走 `/api/tasks/{id}/log`。
- kind/status 映射函数从 task 包导出(`PublicKind`/`PublicStatus`),`service.ListTasks` 复用,删除自己的映射表;未知 kind 输出原始值(S-9);status 按 D-13。
- `GET /api/tasks`、`GET /api/activities` 改 `ListEnvelope` `{items, next_cursor: null, total}`。
- 导入确认响应去掉内嵌 `task` 对象,只留 `{importId, taskId, packId, reused}`。
- 异步发布(`/api/releases/async`、`/api/packs/{id}/publish/{provider}/async`)响应改 `{taskId, reused}`(D-12,契约同步改)。
- DTO null 语义(D-4):`Pack` 的 `iconUrl/loaderVersion/description` 改 `*string` 无 omitempty;`createdAt/updatedAt` 去 omitempty(库列必有值);`Mod` 的 `projectId/versionId/sha1` 改 `*string`;内容 `ContentDocument.activeRevisionId` 输出 null 而非省略。前端 schema 同步。
- `POST content/{id}/apply` 出参补 `revision`(契约要求 `{status, revision}`)。
- `ModSearchResult.NextCursor` json tag `nextCursor` → `next_cursor`;聚合搜索响应补 `next_cursor: null`。

### 批 D — 前端追平

- `features/dashboard/types.ts`:`taskListSchema`/`activityListSchema` 改信封,fetch 后取 `.items`;`taskSchema.status` 枚举加 `queued`;时间字段 `z.iso.datetime()`;null 字段 `.nullable()`。
- 所有 `page` 信封 schema:`next_cursor: z.string().nullable()`、`total: z.number().int()` 必填(去 optional)。
- `packSchema`/`modSchema` 等对齐批 C 的 null 语义;`modSearchSchema.nextCursor` → `next_cursor`。
- 任务控制 `taskAction` 用 `taskSchema` 解析(C-1);`del()` 复用 `writeHeaders()`(C-2)。
- `api/http.ts`:抛 `ApiError{status, code, message}`(从错误信封取 code);内容/任务书编辑器捕获 `revision_conflict` → 提示"内容已变更,请刷新重试"(C-5)。
- 设置页重写(D-11):平台连接与存储接 `fetchStatus` 真实数据(三态:ok/unavailable/unknown);移除"清理缓存""默认包配置""恢复默认""界面"。
- `OnboardingChecklist` 轮询加组件卸载清理(useRef + useEffect cleanup)(C-3 残余)。
- `vite.config.ts` token 注入(批 A 前端部分)。

### 批 E — 验证与文档

- `go build ./... && go vet ./... && go test ./... && gofmt -l .` 全绿;前端 `tsc -b` + `vite build`。
- 起真实服务(18871)curl 矩阵实测(见 §5)。
- 更新 `docs/api/implementation-status.md`;新增 `docs/api/audit/2026-08-30-conformance-round-1.md` 记录本轮修复清单与残余缺口;`standards.md` §9 D-5/D-9 去掉"待确认"。
- 更新 `docs/project-state`(checkpoint 由用户确认后另行执行)。

## 5. curl 验收矩阵(批 E 执行)

| # | 场景 | 期望 |
|---|---|---|
| 1 | POST 无 token | 401 `unauthorized` |
| 2 | POST 错误 token | 401 |
| 3 | POST 非 JSON Content-Type | 415 `unsupported_media_type` |
| 4 | POST 非法 JSON | 400 `invalid_argument` |
| 5 | GET 不存在包 | 404 `pack_not_found` |
| 6 | POST /api/packs 缺 name | 400 |
| 7 | POST /api/packs loader 非法 | 400 |
| 8 | POST /api/packs mcVersion 不在候选 | 422 `pack_unsupported_mc_version` |
| 9 | POST /api/packs 重名 | 422 `pack_name_duplicate` |
| 10 | POST /api/packs 正常 | 201,DTO 含 null 字段不省略 |
| 11 | GET /api/tasks | 信封 `{items,next_cursor:null,total}` |
| 12 | GET /api/activities | 同上 |
| 13 | GET /api/tasks?recent=101 | 400 |
| 14 | PUT onboarding 写 prismAccount | 422 `onboarding_step_readonly` |
| 15 | PUT onboarding 未知步骤 | 422 `onboarding_unknown_step` |
| 16 | 内容 draft 缺 If-Match | 400 |
| 17 | 内容 draft 陈旧 If-Match | 412 `revision_conflict` |
| 18 | 内容 slug 重复 | 422 `content_duplicate_slug` |
| 19 | 导入 inspect 非 https | 400 |
| 20 | 导入 inspect 域名不匹配 | 422 `import_invalid_source` |
| 21 | 导入 confirm token 错 | 400 |
| 22 | 导入 confirm inputHash 错 | 422 `import_input_mismatch` |
| 23 | 导入 confirm 正常 → 重复同输入 | 202 → 409 `import_preview_consumed` 且返原 taskId/reused |
| 24 | 任务控制(pause 已完成任务) | 409 `task_invalid_transition`,响应为统一 Task DTO |
| 25 | DELETE 包 | 204 空体 |
| 26 | GET /api/system/status | 三态字段存在 |
| 27 | 超 8MB 请求体 | 413 `payload_too_large` |
| 28 | mod-search 聚合 | `{items, errors, total, next_cursor}` 形状 |

## 6. 明确不做

- 启动内核、模板开局、mrpack-install 冒烟(另有待拍板事项,不在本轮)。
- `/api/settings`、cache purge 接口(随 D-11 UI 一并推迟)。
- `mod_incompatible_version` 真实校验(缺数据源,记缺口)。
- duplicate 功能本身(只删占位)。
- 视觉、假数据清扫中未被契约点名的部分(包健康右栏等,仍记录在案)。
