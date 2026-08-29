# mPackStation 开发规范（v0.1 草案）

> 本文是最终后端架构 v7 的执行规范草案，供评审。未评审通过前，不作为强制变更门禁。

## 1. 适用范围与基本原则

- 产品是本机单实例应用；不引入 tenant/workspace 之间的数据隔离模型。前期只做本机 token，预留未来 GitHub OAuth/API identity 入口，不做本地账号管理。
- 整合包是唯一业务作用域；`pack_mods` 是包内模组选择的唯一权威来源。
- 前端页面先遵循页面规格，组件只依赖 API 适配层和 zod 类型，不直接读取数据库字段。
- 业务规则只存在于 service；HTTP handler 不写 SQL、不调用 Provider、不直接操作业务文件。
- 任何会影响数据、任务、文件或 API 语义的修改，都必须留下可复核的测试或决策记录。
- 所有资源授权必须在后端完成；前端隐藏按钮、路由限制和本机部署形态都不能替代账号/成员权限检查。

## 2. 目录职责

```text
apps/server/cmd/server/             进程装配；禁止业务逻辑
apps/server/internal/config/        配置来源、默认值、校验
apps/server/internal/httpapi/       路由、解码、HTTP 错误、安全中间件
apps/server/internal/service/       领域规则、授权、事务编排、任务 handler
apps/server/internal/store/         migration、repository；唯一直接写 SQL 的层
apps/server/internal/task/          队列、worker、lease、恢复；不依赖领域 service
apps/server/internal/provider/      CurseForge/Modrinth 适配器和外部 HTTP
apps/server/internal/blobstore/     temp、blob、校验、rename、GC
apps/server/internal/platform/      时钟、ID、路径策略、密钥保护
apps/server/internal/obs/           日志、审计、指标、request-id
apps/web/src/api/                   fetch、错误处理、zod 适配
apps/web/src/features/              页面业务组件
docs/                               架构、契约、页面规格、决策记录
scripts/                            开发、构建、验证、打包脚本
```

依赖方向固定为：

```text
cmd → httpapi → service → {store, task, provider, blobstore}
service → {platform, obs}
task → store
```

禁止反向依赖。非 `cmd` 包禁止直接使用标准库 `log`。

## 3. 受保护文件

以下文件不是永远不可修改，但修改必须同时提交对应证据：

```text
docs/backend-architecture-v7.md
docs/page-specs/**
apps/server/internal/store/migrations/*.sql
apps/server/internal/store/schema.sql
apps/web/src/api/**/types.ts
apps/web/src/api/http.ts
apps/server/go.mod
apps/server/go.sum
apps/web/package.json
apps/web/package-lock.json
scripts/build.ps1
scripts/verify.ps1
```

规则：

- 架构变化：新增 `docs/decisions/ADR-*.md`，说明动机、影响和替代方案。
- 数据库变化：新增递增 migration；已应用 migration 禁止修改。
- API 变化：同步更新契约、前端 zod、fixture 和契约测试。
- 依赖变化：说明用途、许可证、体积/启动影响和替代方案。
- 构建/验证脚本变化：在干净目录重新执行完整验证。
- 不允许为了让测试通过而削弱约束、删除测试或放宽错误处理。

## 4. 命名与代码风格

### Go

- 提交前必须通过 `gofmt`、`go vet` 和 `go test ./...`。
- 包名使用小写单词；导出类型和方法必须有 GoDoc。
- 错误使用 sentinel/domain error + 稳定 `error_code`；禁止用字符串比较判断业务错误。
- 所有接收外部输入的 service 方法必须校验输入并接受 `context.Context`。
- 时间由 `platform.Clock` 提供；落库统一 unix milliseconds。
- SQL 使用参数绑定；禁止拼接用户输入。
- 所有事务必须短小，不得在事务内调用网络或执行长时间磁盘 I/O。

### TypeScript/React

- 提交前必须通过 `npm run build`；组件不得直接使用 `fetch`。
- 接口响应先经过 zod 校验，再进入组件。
- 业务状态使用明确的领域命名；展示文案不得成为数据库枚举。
- 页面状态必须显式覆盖 loading、empty、error、success 和不可用状态。
- 样式使用设计 token；禁止为单个组件散落同义硬编码颜色、间距和圆角。

## 5. 数据库规范

- SQLite 单库；业务表通过 `pack_id` 分域；JAR 通过 SHA-1 全局共享。
- schema 只由 migration 演进；migration 编号单调、不可复用、已应用文件不可改。
- 每张表必须声明主键、外键、唯一约束、CHECK 约束和保留/清理策略。
- 所有需要被看板感知的业务写入，必须在同一事务写入 outbox。
- 任务表的 INSERT/UPDATE/DELETE 只能位于 `internal/task`。
- `pack_mods.sha1` 与 `jar_index` 登记必须在同一事务完成。
- 删除包只做数据库级联和清理清单登记；不得在数据库事务中递归删除磁盘文件。
- 数据库字段不能为了迎合 UI 文案而改变领域语义。

## 6. API 与前端契约

- API 全部使用 `/api` 前缀；破坏性变化才引入新版本。
- 成功响应、列表分页、错误信封、request-id 和 HTTP 状态码必须有固定契约。
- 写请求统一使用 token、Origin 和 Host 校验。
- API 对外可以使用前端展示字段，但必须由 adapter 映射到领域模型。
- 任务内部 `kind/status` 与前端 `type/status` 的映射必须集中定义，禁止组件自行转换。
- 新增端点必须同时补：请求/响应 schema、错误用例、权限/安全测试和前端适配测试。
- 不在 API 中暴露绝对路径、密钥、内部栈或 SQL 错误。

## 7. 任务、Provider 与文件纪律

- 任务 handler 位于 service，通过 registry 注册；task 包不包含业务逻辑。
- 任务状态迁移必须带期望状态和 lease epoch；旧 worker 不得继续写入。
- 任务必须支持幂等、取消、失败重试和服务重启恢复。
- Provider 类型必须使用领域 DTO，禁止泄漏平台 SDK 类型。
- 所有外部调用必须经过统一的 deadline、限流、熔断、重试和缓存策略。
- 文件写入固定遵循 `temp → 校验 → 短事务登记 → rename`。
- 所有用户路径必须经过 path policy；禁止 `..`、符号链接逃逸、junction 和危险归档条目。
- blob 删除必须经过引用检查和 grace 宽限期。

## 8. 身份入口纪律

- 当前版本只支持本机启动 token；不得为了“未来协作”提前引入本地账号、成员表或角色管理。
- `Principal` 保持可替换接口；未来 GitHub OAuth/device-flow/API identity 适配器放在独立 identity/provider 边界，不改变现有 pack 数据模型。
- 启动 token、未来 GitHub 凭据、Provider API Key 均不得写入日志、任务 payload 或普通 API 响应。
- 未来引入身份后，资源授权必须在 service 层完成；身份、协作和权限变化沿用 audit 入口。

## 9. 测试门禁

每个功能至少覆盖对应层次：

- 单元测试：纯规则、状态机、错误映射、输入校验。
- repository 测试：SQL、约束、分页、事务回滚。
- service 测试：授权、跨表事务、outbox、幂等。
- 集成测试：真实 SQLite、任务恢复、文件巡检、启动/退出。
- 契约测试：前端 zod、API fixture、Provider fixture、错误信封。
- 安全测试：token、Origin/Host、路径、SSRF、导入限制、敏感信息脱敏。

以下任一情况不得标记完成：

- 只有 endpoint 没有 repository/service 测试。
- 只有成功路径，没有失败、取消、恢复和重复提交测试。
- 只有数据库写入，没有文件一致性测试。
- 只有 Provider 正常响应，没有 404、429、5xx、超时和离线缓存测试。

## 10. 变更流程

1. 先确认变更属于哪个领域和哪个契约。
2. 更新架构/ADR、API 或 migration 设计。
3. 先写失败测试或 fixture，再实现代码。
4. 完成 `gofmt`、`go test`、`go vet`、前端 build 和契约测试。
5. 检查受保护文件是否有相应证据。
6. 在干净数据目录执行启动、迁移、健康检查和关闭验证。
7. 更新项目状态与交接文档。

## 11. 固定验证命令

```text
scripts/verify.ps1
```

该脚本最终应统一执行：

```text
go test ./...
go vet ./...
gofmt -l .
npm ci
npm run build
架构依赖检查
迁移和契约测试
```

本规范草案评审通过后，才将上述规则接入 CI 和提交门禁。
