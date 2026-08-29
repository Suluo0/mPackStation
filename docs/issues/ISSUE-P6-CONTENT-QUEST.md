# ISSUE-P6-CONTENT-QUEST

状态：未开始（P5 完成后进入）  
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

- P6 生产代码与递增 migration（如需要）。
- P6 API 契约/fixture、自动化测试、人机共读测试矩阵。
- 本 issue 更新为可复核的通过/阻断结论，并附 commit。
