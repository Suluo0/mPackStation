# ISSUE-P6-CONTENT-QUEST

状态：P6 service/repository 已实现；HTTP 契约验收不合格（2026-08-29）
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
- N/A：Provider、文件落盘和 HTTP 契约由 P4/P5/P7 负责；本阶段没有引入新依赖。
- 独立 Luna 2026-08-29 执行：`.tools/go/bin/go.exe test ./... -count=1 -timeout=180s`
  通过；P6 service/store 测试通过。
- 独立 HTTP 契约执行：`.tools/go/bin/go.exe test ./internal/httpapi -run '^TestP6HTTP'`
  不合格，当前文档规定的 content/quest 路由已经可访问，但暴露两个阻断：
  content draft 成功响应的 `revision.state` 为空；跨包模组引用返回
  `invalid_argument`，而契约要求稳定的 `cross_pack_reference`。
- 另已验证：非法 JSON 返回 `invalid_json`、无 token 返回 401 `unauthorized`、
  ore 范围校验返回 `validation_failed`、If-Match 冲突返回 `revision_conflict`，
  apply/rollback/history/preview、环和孤立节点路径均已实际触达；这些路径的
  request-id 可追踪。

## 独立验收发现（2026-08-29，Luna）

- `P6-HTTP-001`：`PUT /api/packs/{packId}/content/{documentId}/draft` 返回
  revision=2，但 `state` 是空字符串。响应必须反映持久化 revision 的 `draft`
  状态，否则前端无法可靠判断是否可 apply。
- `P6-HTTP-002`：跨包 `modRefs` 的请求 HTTP 400，但错误码是
  `invalid_argument`；P6 验收门槛要求稳定 `cross_pack_reference`，调用方无法
  区分跨包安全拒绝与普通参数错误。
- 以上均为生产 service/store/httpapi 问题，本 Luna 测试任务未修改生产路径；
  修复后必须重新执行 P6 HTTP 测试和全量 Go 测试。
