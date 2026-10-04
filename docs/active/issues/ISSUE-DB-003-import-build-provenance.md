# ISSUE-DB-003：导入确认与构建输入缺少持久化来源

- 类型：P1 / 恢复与可复现构建
- 状态：Open
- 责任：导入与构建负责人

## 现象

架构要求导入 preview token 在重启后仍可一次性确认，并要求 artifact 能追溯 lock、内容/任务书 revision 和构建配置；当前 schema 未提供对应持久化来源。

## 完成条件

- `import_previews` 保存 token hash、输入 hash、source、暂存句柄、过期/消费状态；确认操作在短事务中原子消费。
- `pack_version_inputs` 或等价 canonical manifest 保存构建精确输入和 fingerprint。
- 过期、重复确认、输入 hash 不匹配和重启恢复均有负例/恢复测试。

## 验收证据

- `V7-DB-007` 通过。
- 补充导入文件清理与构建 fingerprint 的集成证据。
