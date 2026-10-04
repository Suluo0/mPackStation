# mPackStation 项目记忆

## 服务与环境铁律（2026-10-03 用户定稿，优先级高于本文件其余各节）

- **唯一开发服务**：后端 `http://127.0.0.1:18872`（`-data /tmp/mpack-data`），前端 `http://127.0.0.1:5271`（`apps/web3`，代理回 18872）。
- **两端都必须只绑回环**。后端 `-addr 127.0.0.1:18872`、`apps/web3/vite.config.ts` 的 `server.host: '127.0.0.1'`。
  本工具是本机单用户 IDE，**无写令牌**（X-MPack-Token 已于 2026-10-03 移除，理由见 `docs/api/auth.md`）。
  把任一端改成 `0.0.0.0` 等于把数据库开放给整个局域网且没有任何凭据能拦住 —— 那种情况下必须先把写鉴权加回来。
  `scripts/dev.sh` 不再传 `--host`，以免命令行覆盖 vite 配置。
- **禁止另开实例、禁止使用任何历史端口**：5173 / 5273 / 5274 / 5275 / 5276 / 18871 / 18880 一律不再使用；
  需要隔离环境时先停下来问用户，不许自行起新端口或新数据目录。
- `scripts/dev.sh` / `dev-stop.sh` 是唯一起停方式；改动会话不得绕过它手工起服务。
- `/tmp/mpack-data` 是唯一共享开发数据目录；**对它做重启、迁移、目录重建之前，必须获得用户当轮明示**。
- 并行会话共用此工作区时：迁移编号先 `ls apps/server/internal/store/migrations/` 再取号；
  改任何文件前先重读（对方可能已改）；不碰对方正在跑的进程。
- 历史背景：2026-10-03 因双会话并行 + 环境重置，/tmp/mpack-data 与 /tmp/mpack-chain 数据被清空
  （见 `docs/tests/incident-2026-10-03-shared-env-wipe.md`）。本节即为防再发而设。

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
