# ISSUE-P6-CONTENT-QUEST

状态：P6 HTTP 契约复验通过；整体验收仍不合格（2026-08-29）
负责人：Sol｜P6 领域开发  
独立验收：Luna｜P6 测试负责人

## 范围

- recipe/structure/ore 文档、revision、校验、应用、历史与回滚。
- quest book/revision/chapter/node/edge/reward 及预览模型。
- revision 应用前的引用校验、环检测、孤立节点检查和 delivery-check 接口。

## 验收门槛

- 同一文档/任务书最多一个 applied revision；revision 编号单调且不可覆盖。
- 非法 JSON、未知内容类型、跨包引用、重复位置、环和孤立节点均返回稳定错误码。
- apply/rollback 在事务中更新 active 指针并留下 activity/outbox/audit。
- 删除、重复提交、并发应用和失败回滚有 repository/service 测试。
- 测试矩阵记录命令、结果和 N/A 理由；不得用放宽约束替代修复。

## 交付物

- P6 生产代码与 repository 边界：`apps/server/internal/service/p6_content.go`、
  `apps/server/internal/store/p6_repo.go`；没有修改既有 migration。
- 自动化测试：`apps/server/internal/service/p6_content_test.go`，覆盖 revision
  乐观并发、规范化去重、校验/apply/rollback、图环检测、孤立节点、跨包引用、
  delivery-check 以及 activity/outbox/audit 证据。
- HTTP 路由已在工作树接入，但尚未按 v7 契约验收；前端 adapter 仍未接入。
  不得将本 issue 标记为最终合格，直到独立 HTTP 契约测试全绿。

## 当前验收记录

- 代码边界：service 不写 SQL，所有数据库操作集中在新增 `store/p6_repo.go`。
- 适用门禁：Go 格式化、单测、vet、真实 SQLite FK/CHECK/UNIQUE、事务证据。
- N/A：Provider、文件落盘和 P7 交付由其他阶段负责；HTTP 契约已纳入本阶段独立验收；
  前端 zod adapter 仍由 web 契约验收负责；本阶段没有引入新依赖。
- 独立 Luna 2026-08-29 复验：`.tools/go/bin/go.exe test ./internal/httpapi
  -run '^TestP6HTTP' -count=1 -timeout=120s` 通过；上次发现的两个 HTTP
  缺陷均已修复（draft state=`draft`，跨包错误码=`cross_pack_reference`）。
- P6 service/store 定向复验未通过：
  `.tools/go/bin/go.exe test ./internal/service ./internal/store -run '^TestP6'
  -count=1 -timeout=120s`。`TestP6QuestRejectsCycleOrCrossPackReference` 仍断言
  旧的 `ErrInvalidArgument`，而当前实现已返回契约要求的 `ErrCrossPackReference`。
- 全量复验 `.tools/go/bin/go.exe test ./... -count=1 -timeout=180s` 同样仅在上述
  service 测试失败；`go vet ./...` 通过，P6 HTTP 四项测试与格式检查通过。
- 另已验证：非法 JSON 返回 `invalid_json`、无 token 返回 401 `unauthorized`、
  ore 范围校验返回 `validation_failed`、If-Match 冲突返回 `revision_conflict`，
  apply/rollback/history/preview、环和孤立节点路径均已实际触达；这些路径的
  request-id 可追踪。

## 独立验收发现（2026-08-29，Luna）

- `P6-HTTP-001`：已修复并由独立 HTTP 测试确认 `revision.state=draft`。
- `P6-HTTP-002`：已修复并由独立 HTTP 测试确认跨包引用为 400
  `cross_pack_reference`。
- `P6-TEST-001`：生产错误语义已切换到 `ErrCrossPackReference`，但既有 service
  回归测试仍断言 `ErrInvalidArgument`，导致 `go test ./...` 失败。应更新该测试
  的预期（或提供兼容的错误链），然后由 Luna 重跑全量验收。
- 本 Luna 复验未修改生产路径和既有 service 测试，仅更新本 issue 与验收矩阵。
