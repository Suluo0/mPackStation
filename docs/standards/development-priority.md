# mPackStation 开发优先级（基于 v7）

> 当前目标：先完成单机版本的工程闭环，再逐步接入真实业务。账号、协作和 GitHub identity 只保留入口，不进入前期实现。

## P0：工程基座冻结

1. 以 `backend-architecture-v7.md` 为唯一架构基线。
2. 将 `AGENTS.md`、开发规范、构建部署规范接入项目流程。
3. 固定目录、模块边界、配置来源、数据目录和端口。
4. 固定 Go/npm 依赖及升级规则。
5. 明确 schema/migration、API、错误码、任务状态和 Provider DTO 的命名契约。

交付物：项目规范、ADR 模板、依赖策略、变更模板。

## P1：构建与部署

1. `dev.ps1`：启动前后端开发环境。
2. `build.ps1`：构建前端和 Go 服务。
3. `verify.ps1`：测试、vet、格式化、契约和依赖检查。
4. `package.ps1`：生成可运行的单机分发目录。
5. 新目录启动、已有数据升级、异常退出和重新启动 smoke test。

交付标准：在干净目录完成构建、初始化、启动、探活、关闭和再次启动。

## P2：后端框架与数据库

1. config、store、repository、service、httpapi、task、provider、blobstore、obs、platform 骨架。
2. 正式 migration runner、checksum、quick check、foreign key check。
3. v7 全量 schema 和一致性不变量。
4. 统一错误、request-id、日志、配置校验和基础安全中间件。
5. 单实例 OS lock、SQLite 读写连接策略、启动恢复顺序。

## P3：第一条真实纵向闭环

```text
创建包 → 查询包 → dashboard 聚合 → 查看状态 → 修改/归档/删除包
```

实现 dashboard、pack CRUD、system health/status、onboarding、MC versions、activities 和基础 tasks，让前端可以关闭 `USE_MOCK`。

## P4：任务框架与导入

1. queued/leased/running/paused/succeeded/failed/canceled 状态机。
2. lease、heartbeat、epoch fencing、取消、重试、恢复和幂等。
3. task events、日志流、outbox/activities。
4. CurseForge manifest、Modrinth mrpack、local zip 的 preview/confirm/import 任务。
5. zip slip、symlink、压缩炸弹、SSRF 和磁盘预检。

## P5：模组主链路

```text
Provider → 搜索 → 加入包 → 下载 → SHA-1/JAR 索引
         → 依赖求解 → 锁定快照 → 冲突与健康摘要
```

先完成统一 Provider DTO 和 fixture，再做 CurseForge/Modrinth 适配器；不让平台 SDK 类型进入 service 或前端。

## P6：内容与任务书

1. content document/revision/validation/apply/rollback。
2. recipe、structure、ore 三种内容类型。
3. quest book/revision/chapter/node/edge/reward。
4. 环检测、孤立节点、引用校验和预览模型。
5. 应用 revision 纳入 delivery checks。

## P7：构建、产物与发布

1. 交付检查。
2. 可复现本地 zip。
3. artifact 登记、校验值和来源快照。
4. CurseForge/Modrinth 发布任务和远端状态查询。
5. 发布失败恢复；非幂等发布不自动重试。

## P8：GitHub 与协作入口

前期只保留接口和 Principal 扩展点，不做本地账号管理。后续再按实际需求接入：

- GitHub OAuth/device-flow/API identity。
- GitHub 仓库读取或发布。
- 协作身份、权限和审计。

这些能力不得引入 tenant/workspace 数据隔离，除非未来需求明确改变产品边界并重新评审架构。

## P9：前端最后收工

真实接口稳定后，再集中处理视觉瑕疵、错误态、响应式细节、加载反馈和真实数据下的布局问题。

## 当前第一步

先评审：

- [development-standards.md](development-standards.md)
- [backend-architecture-v7.md](backend-architecture-v7.md)
- 本优先级文档

评审通过后，进入 P0/P1，不直接跳到 Provider 或内容编辑。
