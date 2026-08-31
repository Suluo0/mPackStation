# API 契约追平实施计划(2026-08-30)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans(本轮由作者 inline 执行,逐批自验)。Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 现有系统追平 docs/api 契约(鉴权/错误码/状态码/Task DTO/信封/null 语义/前端 zod/设置页),curl 实测到全通。

**Architecture:** 后端 Go(net/http 标准库,httpapi→service→store 三层);前端 React+zod 契约校验层。改动按五批推进,每批结束跑构建与测试。

**Tech Stack:** Go 1.27(.tools/go)、React19+TS+Vite7+zod4、SQLite(modernc)。

**Spec:** `docs/superpowers/specs/2026-08-30-api-contract-conformance-design.md`(决策 D-5~D-15、分批细节、curl 矩阵全在其中,本计划只做执行跟踪,不重复内容)

## Global Constraints

- 契约优先:实现与 docs/api 冲突时改实现(例外仅 spec §2 拍板的 D-12 改契约)。
- 验收纪律:每批 `gofmt` 零不合规;批 E 前 `go test ./...`、`go vet ./...`、前端 `tsc -b`/`vite build` 必须全绿。
- 迁移 SQL 文件内不写 BEGIN/COMMIT(执行器自带事务)。
- 端口:后端 18871/前端 5273;旧项目端口 5173/18765/18766 不碰。
- 提交前向用户展示确切 commit 方案。

## Tasks

- [ ] **批 A 鉴权 token** — main.go 启动生成/复用 `data/runtime-token`(0600,MPACK_TOKEN 优先);httpapi 删 `test` 兜底;vite.config.ts define 注入;http.ts 删回退。自验:无 token 写请求 401、服务重启 token 稳定、前端 dev 拿到注入值。
- [ ] **批 B 错误码与状态码** — DomainError/ValidationError;412/410/409/422/413/400-clamp;pack 查重与 mcVersion 候选;onboarding 两个 422;content slug 查重;build_blocked;导入四层拆分 + 迁移 0004 consumed_task_id + 重复确认返原结果;资源级 404;删 duplicate;`invalid_json`→`invalid_argument`。自验:相关 go test 更新并全绿。
- [ ] **批 C Task DTO 归一 + 信封 + null 语义** — HTTPAdapter.view 输出契约 Task;导出 PublicKind/PublicStatus 复用;tasks/activities 信封;导入响应去内嵌 task;异步发布响应 {taskId,reused};Pack/Mod null 语义;content apply 补 revision;mod-search next_cursor。自验:p3/p4/p5/p7 验收测试更新并全绿。
- [ ] **批 D 前端追平** — types/api schema 全量对齐(信封必填、datetime、nullable、queued);taskAction 用 taskSchema;del() 复用 writeHeaders;ApiError{status,code} + revision_conflict 提示;设置页重写(D-11);轮询卸载清理。
- [ ] **批 E 验证与文档** — 全量测试绿;起 18871 服务跑 spec §5 curl 矩阵 28 条;更新 implementation-status.md;写 docs/api/audit/2026-08-30-conformance-round-1.md;向用户展示 commit 方案。
