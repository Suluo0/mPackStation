# 前端页面与 API 映射

页面只能通过 `apps/web/src/features/*/api.ts` 访问后端，禁止在组件中直接 `fetch` 或保留同名 mock 数据。

| 页面 | 主要 API | 当前状态 |
|---|---|---|
| Dashboard | `/api/dashboard`、`/api/tasks`、`/api/activities`、`/api/system/health` | 已接真实 API |
| Packs | `/api/packs`、`/api/packs/{id}` | 已接真实 API |
| Pack Mods | `/mods`、`/mod-search`、`/mod-versions`、冲突/锁 | 主要路径已接 |
| Content | content 列表、draft、validate、rollback | 基础路径已接 |
| Quests | quests draft、validate、rollback、history | 基础路径已接，schema 待完善 |
| Build/Publish | versions、build、artifacts、publish、poll/retry | API 层部分完成，页面闭环待验收 |
| Settings | system status、设置读写、cache purge | 后端/页面均未闭合 |

接线完成的判定必须同时满足：真实 API 调用、成功响应校验、错误响应展示、加载/空态/重试状态，以及至少一条可复现的浏览器验收记录。
