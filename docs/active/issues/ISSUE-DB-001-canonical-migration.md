# ISSUE-DB-001：canonical migration 尚未形成可执行闭环

- 类型：P0 / 数据库基座
- 状态：Open
- 责任：数据库实现负责人

## 现象

当前数据库仍由一次性 v1 `schema.sql` 初始化，缺少 v7 要求的完整表集、编号 migration、`name/checksum` 元数据和启动完整性检查。

## 完成条件

1. 提供有序、可嵌入 binary 的 canonical migration manifest。
2. 已应用 migration 的 checksum/name 可校验；缺失、篡改、跳号和高版本均有确定失败结果。
3. 空库、历史 v1 库、重复启动、迁移失败回滚均有可重复证据。
4. 每次启动通过 `quick_check`、`foreign_key_check` 和 JSON1 探针后才进入 ready。

## 验收证据

- `V7-DB-001`、`V7-DB-002`、`V7-DB-003` 测试通过。
- 保存空库及历史库升级日志、迁移 checksum 和失败回滚结果。
