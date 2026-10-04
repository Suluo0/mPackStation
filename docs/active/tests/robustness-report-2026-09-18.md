# mPackStation 健壮性检查报告

- 日期：2026-09-18
- 分支：`DEV_2609-VK3` @ `b21c7bd`（检查后另有本地修复，见下）
- 环境：macOS（darwin/arm64），Go 1.27.1，Node v22/v24，本地 dev：后端 `127.0.0.1:18871`，前端 `127.0.0.1:5273`
- 数据目录：`data/mpackstation.db`（单库，schema 含 migrations 0001–0016）

## 结论摘要

| 维度 | 结论 |
|---|---|
| 后端单元/验收测试 | 修复导出目录符号链接校验后，`go test ./...` **全绿** |
| 前端生产构建 | `tsc -b && vite build` **通过**（chunk 1093 kB 警告仍在） |
| 本地服务 | 前后端可启动，`/api/health` ready，vite 代理连通 |
| 运行时数据面 | Pack CRUD / 模组搜索(MR) / 依赖 / 内容修订 / 任务书 / 发布检查 / 启动器入队 **可用** |
| 关键风险 | 任务列表曾过滤掉新 kind；launcher/catalog 任务曾长时间 queued；前端残余 mock；启动器参数校验在空表单下直接 failed |

## 已验证项

1. **静态/构建门禁**
   - `go build ./cmd/server`、`go vet ./...` 通过
   - `go test ./...`（修复后）全部 `ok`：blobstore/config/httpapi/obs/platform/provider/service/store/task
   - 前端 `npm run build` 通过
2. **服务探活**
   - `GET /api/health` → `{"status":"ready","db":true,"storageWritable":true}`
   - `GET /api/healthz` / `/api/readyz` 正常
   - 前端 `5273` → 200，`/api` 代理到 18871 成功
3. **鉴权**
   - 无 `X-MPack-Token` 的 POST `/api/packs` → 401
   - 写操作 token 来自 `data/runtime-token`，前端 vite 构建期注入，无硬编码兜底
4. **领域闭环抽样**
   - 创建包 → 内容 draft/validate/apply（If-Match 乐观锁，陈旧写 412 `revision_conflict`）
   - 任务书 draft/validate/apply
   - 发布页 delivery-checks（content/quest passed）
   - CF key 无效保存 → 400（明确错误信息），DELETE → 204
5. **任务 worker**
   - 重启后 worker 能领取任务；launcher 任务可走完 queued→leased→running→failed（参数不全时）
   - `catalog_init` 有 success 记录

## 发现的问题（按严重度）

### P0 — 已修复（本轮）

#### R1. 导出目录校验在 macOS 上必失败
- **现象**：P7 相关测试全部报 `export directory is not allowed`（`RegisterExportDirectory` 即失败）
- **根因**：`verifyExportDir` 用 `EvalSymlinks(dir) == dir` 做校验；macOS `t.TempDir()` 与常见路径父级是 `/var` → `/private/var` 符号链接，永远不相等
- **影响**：跨平台构建/发布验收在 macOS 无法通过；真实用户若导出目录位于符号链接父路径下同样无法注册
- **修复**：`apps/server/internal/service/build.go` — 仅拒绝「导出路径本身是符号链接」，不再拒绝父级符号链接；仍要求目录存在且含 `.mpackstation-export` 标记文件
- **验证**：修复后 `go test ./...` 全绿

#### R2. 任务列表 SQL 过滤过时，新 kind 从 API 消失
- **现象**：`GET /api/tasks` 恒为空数组，但 SQLite `tasks` 表里有 `launcher_install` / `catalog_init` 等；看板任务面板与启动台进度轮询永远看不到任务
- **根因**：`pack_repo.ListTasks` 写死 `kind IN ('index','build','import','resolve')`，未包含 migration 0003/0007/0008/0014 新增 kind
- **影响**：前端任务域、LauncherPage 轮询、底边栏任务语义全部失真（与 HANDOFF「底边栏 mock」问题叠加）
- **修复**：扩展为 `index,build,import,resolve,catalog_init,tool_install,launcher_install,launcher_launch,parse_mod_content`
- **验证**：重启后 `GET /api/tasks?recent=15` 返回 12 条真实任务

### P1 — 未修，建议尽快处理

#### R3. 前端残余硬编码 mock（交互可见）
| 位置 | 现象 |
|---|---|
| `PackHealthRail`（包概览右栏） | 写死 86/100、142 锁定、3 冲突、5 可更新、26 内容编辑 |
| `PacksPage` 页头「导入/新建」按钮 | 无 `onClick`，点击无响应（真正入口在看板 `PackModals`） |
| 底边栏「查看日志 / 输出目录」 | 按钮无处理函数 |
| `PackWorkbenchPage` 包设置 | `message.info('包设置将在此处打开')` 占位 |
| 概览「模组数」列 | 页内显示 `—`，未接真实 `modCount` |

#### R4. 启动器空表单仍发起请求
- **现象**：LauncherPage 未填游戏目录/账号时，链路测试仍 POST 成功入队；任务执行期 failed：`version, username and minecraft_dir are required`
- **风险**：用户误点产生失败任务噪音；前端校验与后端校验不一致（前端仅拦了「游戏目录」部分场景）
- **建议**：前端必填校验全覆盖；后端入队前校验并 400，避免脏任务

#### R5. 任务长时间 `queued`
- **现象**：链路测试写入的 launcher/catalog 任务在 worker 可用前长期 queued；部分请求 `GET /api/*` 出现 700–1100ms 延迟
- **可能原因**：SQLite 锁竞争（worker Recover/Lease 与 HTTP 写同库）、handler 阻塞、或启动瞬间 Recover 批量重排队
- **建议**：为任务表操作加超时与 busy_timeout；worker 心跳日志；对 launcher 缺 binary 的场景快速 failed 并写清 suggestion

#### R6. `ContentEditorPage`（P6 recipe/structure/ore）未挂路由
- `main.tsx` 中 `/packs/:id/content` 指向 `ModContentPage`（M6 解析浏览）
- P6 修订式编辑器组件存在但不可达，structure/ore 文档编辑能力在 UI 上断档（与多方块重设计强相关）

### P2 — 工程债 / 体验

| ID | 说明 |
|---|---|
| R7 | vite 构建 chunk ≈1093 kB，未做代码分割 |
| R8 | `dev.sh`/`dev-stop.sh` 强依赖 Windows（`pwd -W`、`taskkill`、`netstat`），macOS/Linux 需手写启动 |
| R9 | `dev` 脚本后台运行与文档「必须前台 PowerShell」冲突；本机曾出现双实例抢 `server.lock` |
| R10 | 前端 `taskSchema.status` 枚举为 `success`，后端包内常量为 `succeeded`——需确认 API 映射层是否始终归一，否则 zod 会静默丢任务 |
| R11 | 动态配方、复杂模型图标覆盖缺口（历史 open issue）仍在 |
| R12 | 网络依赖断言（CF）在未配置 key 时为预期 404/not_configured，契约矩阵需标注环境前提 |

## 分层健康度

```text
契约/API 层     ████████░░  8/10  核心域闭环稳；任务列表曾失真（已修）
任务/异步层      ██████░░░░  6/10  worker 可用，kind 扩展与失败语义需打磨
存储/迁移层      ████████░░  8/10  migration 0016 在位；export-dir 跨平台已修
前端集成层       ██████░░░░  6/10  真实链路多，mock 与死按钮仍多
启动器链路       █████░░░░░  5/10  API 通，参数校验/binary 部署/进度展示不完整
跨平台可移植性   ████░░░░░░  4/10  Windows 脚本中心；macOS export-dir 曾阻断
```

## 建议优先级（下一步）

1. 清掉 `PackHealthRail` / Packs 页死按钮 / 底边栏死按钮 mock，接 dashboard 真实读模型  
2. 启动器前后端参数校验对齐 + 缺 binary 时的明确错误与安装指引  
3. 打通 P6 structure 编辑 UI（与地形/多方块重设计合并推进）  
4. 任务状态枚举全链路对齐 + worker 可观测日志  
5. 补 macOS/Linux 的 `scripts/dev.sh` 与契约验收入口  

## 复现命令

```bash
# 测试
go -C apps/server test ./...
# 构建
cd apps/web && npm run build
# 健康
curl -s http://127.0.0.1:18871/api/health
# 任务
TOKEN=$(cat data/runtime-token)
curl -s -H "X-MPack-Token: $TOKEN" 'http://127.0.0.1:18871/api/tasks?recent=10'
```
