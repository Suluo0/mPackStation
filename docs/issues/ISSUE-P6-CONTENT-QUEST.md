# ISSUE-P6-CONTENT-QUEST

状态：P6 service/repository 已实现；HTTP 接入待 P7 集成验收
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
- HTTP 路由和前端 adapter 尚未接入；不得将本 issue 标记为最终合格，直到 P6
  路由契约测试补齐。

## 当前验收记录

- 代码边界：service 不写 SQL，所有数据库操作集中在新增 `store/p6_repo.go`。
- 适用门禁：Go 格式化、单测、vet、真实 SQLite FK/CHECK/UNIQUE、事务证据。
- N/A：Provider、文件落盘和 HTTP 契约由 P4/P5/P7 负责；本阶段没有引入新依赖。
- 阻断项：需要在 P5 provider 编译恢复后执行 `go test ./...`、`go vet ./...`，
  并由独立 Luna 完成 HTTP 接入后的回归矩阵。
