# mPackStation 项目记忆

## 权威文档

- 后端架构基线：`docs/architecture/backend-architecture-v7.md`
- 开发规范：`docs/standards/development-standards.md`
- 测试验收：`docs/standards/test-acceptance-standards.md`
- 开发顺序：`docs/standards/development-priority.md`
- 页面规格：`docs/specs/**`、`docs/design/dashboard-page-prompt.md`

开始任何开发任务前，先读取与任务相关的上述文档；文档冲突时，以最终 v7 和本文件为准。

## 当前产品边界

- 先做本机单实例版本。
- 不引入 tenant/workspace 数据隔离。
- 前期只使用本机启动 token，不做本地账号管理。
- 为未来 GitHub OAuth/API identity 保留可替换入口，但不提前实现账号、协作和 GitHub 业务。
- 前端视觉瑕疵暂缓，后端真实闭环优先。

## 不可违反的工程规则

- `httpapi` 不写 SQL、不调用 Provider、不直接操作业务文件。
- `store` 是唯一直接访问 SQL 的层。
- `service` 承担业务规则、事务编排和授权入口。
- `task` 只负责队列语义，不包含领域逻辑。
- `provider` 是唯一外部平台 HTTP 边界。
- 所有数据库演进使用新 migration；已应用 migration 禁止修改。
- API、数据库、任务状态或 Provider DTO 变化必须同步契约、fixture 和测试。
- 不得为了通过测试删除约束、放宽安全检查或跳过恢复路径。

## 固定验证

完成任何后端变更前，至少运行：

```text
go test ./...
go vet ./...
npm run build
```

详细规则见 `docs/standards/development-standards.md`。
