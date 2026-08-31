# 契约追平审计(2026-08-30 round 1)

> 性质:本轮是"代码追平契约"的执行记录与证据,不是新的差距清单。设计决策见
> `../superpowers/specs/2026-08-30-api-contract-conformance-design.md`;
> 逐条实施计划见 `../superpowers/plans/2026-08-30-api-contract-conformance.md`。

## 本轮决策(用户授权代理拍板)

| 编号 | 决策 | 结果 |
|---|---|---|
| D-5 | 错误码细粒度 | 采用,已落地(DomainError/ValidationError) |
| D-9 | 契约先行、代码分批追平 | 采用,本轮即第一批 |
| D-10 | 删除 `POST /api/packs/{id}/duplicate` 501 占位 | 已删,实测 404 |
| D-11 | 设置页只留真接口区块 | 平台连接/存储接 system/status;假区块已删 |
| D-12 | 异步发布入队出参 {taskId, reused} | 契约已改,代码已对齐 |
| D-13 | 413 错误码统一 `payload_too_large` | errors.md 与实现已统一 |
| D-14 | 任务状态 queued 不再折叠为 running | PublicStatus 已拆分 |
| D-15 | 分页参数越界 400 | queryLimit 已改,curl 实测 |

## 落地清单与验证方式

| 审计条目(旧) | 处置 | 验证 |
|---|---|---|
| S-1 token test 兜底 | 启动生成+持久化 data/runtime-token(0600),兜底删除;前端 vite define 注入 | curl 401;重启 token 复用;构建产物内含注入 token |
| S-2 409→412 revision | ErrRevisionConflict → 412 | curl If-Match: "99" → 412 revision_conflict;缺 If-Match → 400 |
| S-3 粗粒度错误码 | DomainError{Status,Code,Details}+ValidationError{issues} | curl: pack_name_duplicate / pack_unsupported_mc_version / content_duplicate_slug / content_invalid(details.issues)/ onboarding_step_readonly / onboarding_unknown_step 全部 422 实测 |
| S-4 导入 409 混合 | 拆 410 import_preview_expired / 409 import_preview_consumed(任务丢失时);同输入重放返原任务 202 reused:true | curl+sqlite 三场景实测 PASS |
| S-5 导入来源 400/422 拆分 | 结构错 400 invalid_argument;域名不符 422 import_invalid_source | curl 实测 |
| S-7 duplicate 501 | 路由删除(D-10) | curl 404 |
| S-8 前端分页 schema total optional | page()/listEnvelope:total、next_cursor 恒在 | tsc + 后端信封实测 |
| S-9 未知 kind 空串 | task.PublicKind 原样透传,统一映射函数 | 任务详情 type=import-pack 实测 |
| T-1 Task DTO 三套 | service.Task = task.TaskView 别名,列表/详情/控制同一结构 | curl 任务详情九键与 dto.md 一致 |
| T-2 导入响应内嵌 task | 响应仅 {importId, taskId, packId, reused} | curl 实测键集合 |
| C-1 任务控制 z.unknown() | taskAction 按 taskSchema 校验 | tsc 通过 |
| C-2 del() 不复用 writeHeaders | 已复用 | 代码审查 |
| C-3 轮询无卸载清理 | useRef+useEffect cleanup,10 分钟上限保留 | 代码审查 |
| C-4 时间字段仅 z.string() | 全面改 z.iso.datetime(),可空字段 .nullable() | tsc 通过 + 信封实测 |
| C-5 revision_conflict 无提示 | ApiError 携带 status/code;内容/任务书编辑器 412 专门提示 | 代码审查(浏览器验收待做) |
| C-6 设置页零接口 | D-11:接 system/status,假区块删除 | tsc + build 通过 |

## 顺带修复

- 错误信封 `details` 恒为对象( typed-nil map 曾输出 null )。
- prism-install 任务进度 0-1 刻度 → 0-100(与 import/publish 统一)。
- publish async 对不存在的包由 500 internal_error → 404 pack_not_found。
- RetryPublish 对非 failed release 由静默返回 → 409 release_not_retryable。
- publish 时 artifact 未就绪由错挂 idempotency_conflict → 422 release_artifact_not_ready。

## 端到端验证(2026-08-31,应用户要求补做)

方式:esbuild 原样打包前端真实 API 层(`src/api/http.ts` + `features/dashboard/api.ts` +
`features/pack/api.ts`,含全部 zod schema),Node 里直接调用这些函数打活后端;
任何响应与 zod schema 不符会当场抛错。harness 存于 hermes_workIn/temp/e2e-harness.ts。

- 直连后端(18872):**22/22 PASS**(health/status/dashboard/tasks/activities/mcVersions/
  onboarding、createPack/listPacks/getPack/updatePack、重名 422、mods/locks/conflicts/health、
  onboarding 写与 readonly 422、task 404、导入两阶段+重放返原任务、删除后 404)。
- 经 vite dev 代理(5274 → 18872, VITE_API_TARGET 覆盖):**22/22 PASS**。
- vite 构建期 token 注入:构建产物内含 data/runtime-token 值,已验证。

**E2E 抓到的真 bug(已修复)**:同一幂等键换不同输入重复 confirm 时,
`task.ErrIdempotencyConflict` 在 HTTP 映射层没有分支,落成 500 internal_error。
修复:writeServiceError 新增映射 → 422 idempotency_conflict(契约语义),
curl 复测 422 + details 为对象。go test/vet 全绿。

## 仍未闭合(下一批)

- mod_incompatible_version(422):契约已定义,实现需要平台版本数据,未做。
- 内容/任务书草稿保存的浏览器级验收、P7 页面闭环、真实 provider 凭据发布。
- /api/settings 与缓存清理接口:等产品需要时立项(D-11 已移除假 UI)。
- contract §3.4 异步 publish 的 Idempotency-Key 在 worker 链路上的完整重放语义。
