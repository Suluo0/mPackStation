# API 文档与当前实现核对（2026-08-30）

本记录按当前工作树核对 `docs/api/standards.md`、`contract.md`、`inventory.md`，结论区分“当前仍存在”和“历史问题/已修复”。

## 当前仍成立

| 编号 | 结论 | 证据 |
|---|---|---|
| S-1 | **成立（部分）**：写请求确实校验 `X-MPack-Token`，但服务端为空时回退硬编码 `test`，前端也有同样回退；没有启动生成并持久化令牌。 | `apps/server/internal/httpapi/httpapi.go` `securityMiddleware`；`apps/web/src/api/http.ts` |
| S-2 | **成立**：revision 冲突仍返回 HTTP 409，规范要求 412。 | `writeServiceError` 的 `ErrRevisionConflict` 分支 |
| S-3 | **成立**：通用错误映射仍使用 `validation_failed`、`conflict` 等粗粒度错误码。 | `writeServiceError` |
| S-4 | **成立**：导入预览过期/已消费仍合并为 409 `preview_expired`，服务层也把存储冲突统一映射为该错误。 | `writeImportError`、`ImportService.Confirm` |
| S-5 | **成立**：导入来源语义错误当前映射 400，规范要求 422。 | `writeImportError` |
| S-7 | **成立**：`POST /api/packs/{id}/duplicate` 仍返回 501。 | `httpapi.go` duplicate 路由 |
| S-8 | **成立（前端契约层）**：多个分页 schema 仍把 `total` 定义为 optional，规范要求必填整数。 | `features/content/api.ts`、`features/pack/api.ts`、`features/release/api.ts` |
| S-9 | **成立（看板列表）**：`service.ListTasks` 的 kind 映射不完整，未知 kind 可能输出空字符串；任务详情适配器已有 fallback，二者仍不一致。 | `service/api.go` `ListTasks`；`task/http_adapter.go` `publicKind` |
| T-1 | **成立**：列表使用 `service.Task`，详情/控制使用 `task.TaskView`，未形成全局唯一 DTO。 | `service/api.go`、`task/http_adapter.go` |
| T-2 | **成立**：导入确认响应仍内嵌 `task` 对象，且同时返回 `taskId`；规范要求只返回任务 ID。 | `/api/packs/import` 路由 |
| C-1 | **成立**：任务控制响应仍使用 `z.unknown()`。 | `features/dashboard/api.ts` |
| C-2 | **成立**：前端 `del()` 没有复用 `writeHeaders()`，虽然确实发送了 token。 | `apps/web/src/api/http.ts` |
| C-4 | **成立**：多个 DTO 的时间字段仍只校验 `z.string()` 或 optional string，未统一 datetime/null 语义。 | `features/content/api.ts`、`features/pack/api.ts` 等 |
| C-5 | **成立**：通用 HTTP 层只把错误转成文本，编辑页面没有按 `revision_conflict` 做专门提示。 | `features/content/*`、`api/http.ts` |
| C-6 | **成立**：设置页仍没有完整的设置读写/缓存清理后端闭环。 | `docs/api/contract.md` §4；当前路由无 `/api/settings`、cache purge |

## 已修复或不再准确

| 编号/指控 | 当前结论 | 说明 |
|---|---|---|
| “完全没有 token 处理” | **不准确** | 后端对所有非 GET/HEAD/OPTIONS 写请求校验 `X-MPack-Token`，缺失/错误分别返回 401，未配置返回 503；问题是令牌来源和 `test` 回退不符合新规范。 |
| S-6“幂等键已消费必然返回 409” | **已部分修复** | 相同幂等键且输入一致时队列返回原任务并携带 `reused`；不同输入仍冲突。导入响应仍多返回了内嵌 task，且幂等语义尚未统一到所有副作用 POST。 |
| C-3“Prism 轮询无退出条件” | **已修复** | `OnboardingChecklist` 有 120 次、每 5 秒的 10 分钟上限；仍可进一步补组件卸载清理，避免卸载后继续触发回调。 |
| “前后端完全没有接上” | **不准确** | 看板、整合包、模组、依赖、内容、任务书、发布等主路径已经调用真实 API；设置页、部分编辑/历史/下载/任务详情等仍未完整接线。 |
| Provider 健康提示“双平台都无法连接” | **历史问题，已修复代码** | 当前实现增加了 `unknown/ok/unavailable` 三态，前端只把 `unavailable` 显示为失败；需重启运行中的旧服务进程后才能看到新行为。 |

## 不是“返回码全部错误”，而是局部不符合

成功码中，创建/异步任务/发布等主要路由已分别使用 201/202，查询使用 200；问题集中在 revision（409 应为 412）、导入过期（409 应为 410）、导入来源语义（400 应为 422）、错误码粒度以及 501 占位路由。因此“接口返回码未按照定义方向返回”作为总体风险判断成立，但不能表述为每个接口都错。

## 本次可复现验证

- 后端：`.tools/go/bin/go.exe test ./... -count=1 -timeout=180s` 通过；`cmd/server`、`instlock` 无测试文件。
- 前端：`npm run build` 通过；仅有 Vite 产物超过 500KB 的警告。

测试通过只证明当前自动化用例和构建通过，不等于上述契约缺口已经消失，也不等于所有真实 provider、浏览器页面交互都已端到端验收。
