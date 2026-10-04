# ISSUE-DB-002：跨包引用与 revision 不变量未落库

- 类型：P0/P1 / 数据一致性
- 状态：Open
- 责任：数据库与领域 service 负责人

## 现象

当前基线使用空字符串表示未下载 SHA-1，缺少 `pack_mods.sha1 → jar_index.sha1`、local/remote 去重、同包 lock/version 指针和 applied revision 唯一性约束。

## 完成条件

- 未下载模组使用 `NULL`，local JAR 和 remote project 的并发重复插入均稳定失败。
- lock/version/current pointer 不能跨包引用；数据库复合 FK 或明确受控 invariant 必须有负例测试。
- 每个 content document、quest book 至多一个 applied revision；active 指针只能指向同对象的 applied revision。
- 约束失败不改变已提交数据，错误可被 service 映射为稳定领域错误。

## 验收证据

- `V7-DB-004`、`V7-DB-005`、`V7-DB-006` 通过。
- 补充真实 insert/update/delete 负例和并发测试。
