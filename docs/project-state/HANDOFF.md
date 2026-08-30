# Project Handoff

## 当前目标

mPackStation —— Minecraft 整合包工作台:模板开局 → 模组/依赖锁定 → 内容/任务书 → 构建 .mrpack → 部署冒烟 → 便携 Prism 导入并启动 Minecraft。**流水线终局必须通向启动一个 Minecraft**(用户 2026-08-30 定调,取代旧的"不启动游戏"表述)。

## 当前状态

分支 `fix/responsive-lowres`,远端 `origin = github.com/Suluo0/mPackStation`。**HEAD = `c52b82c`,工作区干净,本地与远端同步**(本轮 8 个提交已全部推送:模组链路 4 个 + Prism/onboarding 2 个 + 存档 1 个 + docs/api 草稿 1 个)。


- 前端 dev:`http://127.0.0.1:5273/`;后端 dev:`http://127.0.0.1:18871/api/health`
- 便携工具:`.tools/go`(1.27.0)、`.tools/prism`(11.0.3,调试启动器)、`.tools/prism-data`(便携数据目录)
- 写操作需 header `X-MPack-Token`;CURSEFORGE_API_KEY 已配置,Modrinth 免 key

## 项目功能进展

### 2026-08-30 · 看板视觉问题收尾

- 状态:已完成
- 进展:用户确认将看板视觉问题标记为完成;第三轮图形丰富度、间距、响应式避让和首屏布局调整纳入完成记录。
- 关联任务:`task-f6b5c7d8`
- 验证:`npm run build` 已通过;桌面与 390×844 移动视口截图复测无横向溢出。
- 验收:用户明确确认标记为已完成。
- 相关产物:`apps/web/src/features/dashboard/OnboardingView.tsx`、`apps/web/src/features/dashboard/dashboard.css`、`apps/web/src/ui/workbench/workbench.css`

### 2026-08-30 · 模组页双平台真实链路(搜索/版本/添加)

- 状态:已验证(未推送)
- 进展:后端装配真实 provider registry(MR 免 key 默认启用,CF 需 CURSEFORGE_API_KEY);`ModSearchAll` 并发扇出双平台,单平台失败只做错误隔离;新增 `GET /api/packs/{id}/mod-versions`;修复 CF 适配器三个 fixture 造假型 bug(搜索参数是 MR 方言缺 gameId/logo 是对象/gameVersions 混排游戏版本与 loader);前端单框模糊名称搜索;清除写死的假包 tech(路由/侧栏/顶栏)。
- 关联任务:`task-690ee8ef`;关联问题:`issue-ac56a422`(已解决)
- 验证:curl 实测 sodium/JEI 双平台返回;CF 与 MR 同版本 sha1 一致交叉验证;go test/vet/gofmt 全绿;前端 tsc/build 通过。证据绑定提交 `76fb818`、`05aefa7`(+`6764228` gofmt、`a827e9b` Codex 尾巴)。
- 相关产物:`apps/server/internal/service/p5_mods.go`、`apps/server/internal/provider/http_adapter.go`、`apps/web/src/pages/PackPages.tsx`、`apps/web/src/main.tsx`

### 2026-08-30 · Prism 便携安装脚本 + 上手四步 + tool_install 任务化

- 状态:已验证(未提交)
- 进展:`scripts/prism-install.{sh,bat}` 便携安装 Prism 11.0.3 到 `.tools/prism`(零系统接触、相对路径、代理探测→GitCode 国内镜像兜底);上手三步变四步,第 4 步只登录(唤起便携 Prism GUI,检测 `accounts.json` 自动打勾);安装本体按用户拍板为一等任务 `tool_install`(脚本输出写任务日志,成败以任务终态+exe 存在为准);迁移 `0003` 扩展 tasks.kind CHECK;修掉租约回收(30s 租约 vs 静默下载,加 10s 心跳)与幂等键阻塞重装。
- 关联任务:`task-d0e1ded6`、`task-1bd4148d`;关联问题:`issue-670c4eeb`(已解决)
- 验证:e2e 三轮真实执行(删→装→验版本;任务日志 11 条闭环;唤起 GUI 便携目录生成);后端 build/vet/test、前端 tsc/build 全绿。
- 相关产物:`scripts/prism-install.bat`、`scripts/prism-install.sh`、`apps/server/internal/store/migrations/0003_task_kind_tool_install.sql`、`apps/web/src/features/dashboard/OnboardingChecklist.tsx`

### 2026-08-30 · 路线重定义:终局 = 启动 Minecraft;否决嵌 Prism 代码

- 状态:设计决策
- 进展:用户定调流水线终局必须启动 Minecraft(决策 `decision-9a0c8004`);调研否决嵌入 Prism 源码(实测 11.6 万行 C++/Qt 耦合/GPL-3.0-only 传染,`decision-f3ee0d92`);自研启动内核进入分段评估(`idea-ddc96879`,Phase 0 spike 待拍板);superpowers v6.3.0 已安装启用(`decision-7859b14d`,安装时曾被安全扫描拦截,227 条全为误报,抽审后直接装)。
- 验证:调研均有实测证据(LOC 统计、许可证原文、mrpack-install MIT/pkg.go.dev)。

### 2026-08-29 · P7 构建与发布回滚(后被 Codex 08f916a 重新交付)

- 状态:已重新交付 / 缺口未复核
- 进展:P7 曾按用户要求回滚(9cab5c4);2026-08-30 午间 Codex 轮以 `08f916a` 重新交付构建/发布/worker 基础。取消语义/重启恢复测试/HTTP 契约测试三项缺口未复核。
- 关联任务:`task-p7-build-publish`
- 验证:后续全量 go test/vet/gofmt 通过(含 task 包);三缺口未单独复验。

### 2026-08-29 · P6 内容编辑与任务书闭环

- 状态:已交付(暂停点)
- 进展:完成内容文档/revision 与任务书图模型的 repository/service/HTTP 闭环;支持 canonical 校验、If-Match 乐观并发、apply/rollback/history、环/孤立/跨包引用校验,以及 activity/outbox/audit 证据。
- 关联任务:`task-p6-content-quest`
- 验证:Luna 独立复验通过;P6 定向测试、HTTP 契约测试、全量 `go test ./...`、`go vet ./...`、`gofmt` 均通过,证据绑定提交 `7d5bf91`。
- 相关产物:`apps/server/internal/service/p6_content.go`、`apps/server/internal/store/p6_repo.go`、`docs/issues/ISSUE-P6-CONTENT-QUEST`

### 2026-08-28 · 引入 frontend-design-codex 并启动看板第三轮视觉重构

- 状态:已完成（2026-08-30 用户确认）
- 进展:引入 `frontend-design-codex`,建立 workbench 间距令牌,重整欢迎页 Hero、入口卡片、灵感卡片和工作台流程的布局规则;修复 CSS 预览组件样式误删,预览继续使用 HTML/CSS 绘制。
- 关联任务:`task-f6b5c7d8`
- 验证:`npm run build` 已通过;桌面与 390×844 移动视口截图复测无横向溢出。
- 相关产物:`C:/Users/<user>/.codex/skills/frontend-design-codex/SKILL.md`、`apps/web/src/ui/workbench/workbench.css`、`apps/web/src/features/dashboard/dashboard.css`、`apps/web/src/features/dashboard/OnboardingView.tsx`

### 2026-08-28 · 侧边栏菜单、包工作台页面与后端架构盲审

- 状态:页面已实现(mock)/后端审查完成
- 进展:恢复完整侧边栏菜单,新增包工作台、模组、依赖与冲突、内容编辑、任务书、打包与发布、设置路由页面;补充 `docs/architecture/backend-capability-draft.md`。三路 GPT-5.6-sol 盲审已发起,已回收两份完整报告。
- 结论:后端只有 `/api/health` 和 SQLite 初始 schema;耦合性、可维护性、可扩展性、健壮性、鉴权就绪度均不足,综合约 2.7/10。鉴权前必须完成 workspace/principal、migration、repository 授权边界、任务幂等与密钥隔离。
- 验证:`apps/web` `npm run build` 通过;源码紫色规则扫描零命中;Go `test/vet` 通过但无测试文件。
- 相关产物:`apps/web/src/app/AppShell.tsx`、`apps/web/src/pages/PackPages.tsx`、`apps/web/src/pages/pack-pages.css`、`docs/architecture/backend-capability-draft.md`

### 2026-08-28 · 欢迎页首屏响应式迭代

- 状态:已实现 / 待用户验收
- 进展:修正主标题中文语义断行;为独立悬浮的上手三步增加中等宽度避让;移动端限制装饰溢出并调整浮层位置与比例。
- 验证:桌面与 390×844 移动视口真实截图通过;移动 `scrollWidth=375`,未出现横向滚动;`npm run build` 通过。
- 相关产物:`apps/web/src/features/dashboard/OnboardingView.tsx`、`apps/web/src/features/dashboard/dashboard.css`

### 2026-08-27 · 看板视觉精修(redesign-skill 第二轮)

- 状态:已实现
- 进展:数据列等宽数字、健康信号两行堆叠(消除折行)、继续卡进度语义修正(模组安装 130/142)、全站动效令牌统一与按压/浮起反馈、空态按钮钉底对齐、环境状态数值层次。
- 关联任务:`task-e5a4b6c7`
- 验证:tsc 零错误、vite build 通过、双态截图回看确认;用户验收未通过("还是丑")
- 相关产物:`apps/web/src/features/dashboard/dashboard.css`、`apps/web/src/app/shell.css`、`.shots/dash-populated.png`、`.shots/dash-empty.png`(`.before.png` 为迭代前对比)

### 2026-08-27 · 看板视觉重做第一轮(对齐设计稿)

- 状态:已实现
- 进展:新增应用外壳(220px 侧边栏+顶栏,路由共享,可收起);紫色主题令牌化;空态 hero+三彩色入口卡+选锁改装流程条+上手三步;有包态 2:1 栅格、继续卡、行式包列表、右侧任务面板;封面按包 id 稳定渐变。
- 关联任务:`task-d4f3a5b6`
- 验证:tsc/build 通过、双态截图风格对齐设计稿框架;用户验收未通过
- 相关产物:`apps/web/src/app/AppShell.tsx`、`apps/web/src/features/dashboard/cover.ts`、`docs/design/dashboard-page-prompt.md` 6.1 节

### 2026-08-27 · Go 后端空壳与 SQLite schema

- 状态:已验证
- 进展:`apps/server` 可运行,18871 端口出 `/api/health`;初始 schema 落地 8 张表并写入四条纪律注释(单库/pack_id 分域/jar_index 共享/原始 JSON 不落库)。
- 关联任务:`task-c3d29e4a`
- 验证:go build 通过;curl 直连与 vite 代理均返回 `{"status":"ok","db":true}`
- 相关产物:`apps/server/cmd/server/main.go`、`apps/server/internal/store/`、`data/mpackstation.db`

### 2026-08-26 · 项目骨架与看板页功能

- 状态:已验证(功能)/ 视觉后被否决返工
- 进展:mPackStation monorepo 从零搭起;看板页按 `docs/design/dashboard-page-prompt.md` 实现全部功能(mock 驱动)。
- 关联任务:`task-a1f07c2e`、`task-b2e18d3f`
- 验证:tsc/build 通过、双态截图渲染正确;功能未被用户否定
- 相关产物:`apps/web/`、`README.md`、`docs/design/dashboard-page-prompt.md`

## 下一步

1. **superpowers v6.3.0 已装好**;启动内核 Phase 0 待拍板:拍板后按 superpowers brainstorming 流程立项,先出设计文档,一行代码不先写。
2. 模板预制开局(`idea-2bc71628`)与 mrpack-install 服务端冒烟(`idea-28f0096b`)待讨论拍板。

## 阻塞与已知问题

- **前端残余假数据**:工作台右栏"包健康"(86/142/3 写死)、概览页模组列表 4 条写死(PackPages.tsx `const mods = [`)、设置页"已连接"/缓存演戏。
- **P7 三缺口未复核**:取消语义/重启恢复测试/HTTP 契约测试(08f916a 之后)。
- **视觉验收**:第三轮已由用户确认完成；后续若提出新的视觉问题，另立新任务，不回写本任务历史状态。
- vite 993 kB chunk 警告为既有现象,未处理。

## 关键决策

- **流水线终局通向启动 Minecraft**(`decision-9a0c8004`)
- **模组搜索:单框模糊名称、双平台并发、不加锁、错误隔离**(`decision-da2110dc`,用户钦定)
- **否决嵌入 Prism 代码**;CLI 调用不传染 GPL,保留兜底(`decision-f3ee0d92`)
- **引入 superpowers 方法论**(proposed,`decision-7859b14d`)
- 技术栈:前端 React19+TS+Vite7+antd6+zod4,后端 Go+SQLite(`decision-3af1a2b3`)
- SQLite 纪律:单库分域、jar_index 按 sha1 跨包共享、原始 JSON 不落库、pack_mods 唯一权威清单(`decision-4ba2b3c4`)
- 流程:每页 docs 提示词 → mock 先行 → 截图验收 → 接后端;AI 效果图内部文字不可信(`decision-5cb3c4d5`)
- 端口:前端 5273 / 后端 18871;旧项目端口一律不碰(`decision-6dc4d5e6`)
- 紫色视觉规范已成文(6.1 节)但标记 needs_review(`decision-7ed5e6f7`)

## 不要重复尝试

- **直连 go.dev / golang.google.cn 下载 Go**:被墙;用阿里云镜像 `mirrors.aliyun.com/golang/`。Go module 代理用 `GOPROXY=https://goproxy.cn,direct` + `GOSUMDB=sum.golang.google.cn`。
- **直连 github.com git clone**:TLS 被断;GitCode(gitcode.com)镜像与官方 release 同路径、等大、高速,为首选;gh-proxy/ghfast/gitclone 实测不可用或限速。
- **api.github.com 走代理**:会被 403;探活用 `https://www.gstatic.com/generate_204`,API 直连优先、代理兜底。
- **git-bash 里 GNU tar 解 zip**:不支持,必须调 `%SYSTEMROOT%\System32\tar.exe`(bsdtar);原生 curl/tar 只认 Windows 路径;`tasklist /FO CSV` 参数会被 MSYS 转换吃掉(用裸 tasklist 或 `MSYS_NO_PATHCONV=1`)。
- **bat 写中文**:UTF-8→GBK 转换丢引号;bat 一律纯 ASCII + CRLF;`()` 块内 `%CD%` 不更新要用 `!CD!`。
- **迁移 SQL 里写 BEGIN/COMMIT**:迁移执行器自带事务,会报 "cannot start a transaction within a transaction"。
- **模组页改回单平台单选**:用户钦定并行设计,绝不允许退化。
- **后端占用 18766** / **杀 5173 上的 PID**:旧项目端口,一律不碰。
- **每包一个 SQLite 库**:已被否决,单库分域。

## 首先读取

- `docs/project-state/state.json` —— 机器可读状态(任务/问题/想法/决策全量)
- `docs/project-state/history/2026-08-30-session-summary.md` —— 本会话详录
- `docs/design/dashboard-page-prompt.md` —— 看板规格 + 6.1 视觉规范
- `apps/server/internal/service/api.go` —— Prism 工具链段落(workbenchRoot/prismAccountReady/tool_install handler)

## 验证状态

- 已运行并通过(2026-08-30,绑定工作树):后端 `go build/vet/test ./internal/...` 全绿、`gofmt -l` 零不合规;前端 `tsc -b`、`vite build` 通过
- e2e 实测:模组双平台搜索/版本/添加;Prism 删装重装三轮;tool_install 任务日志闭环;Prism 登录唤起
- 用户验收:模组页功能用户实测中(未发现新问题);视觉两轮 rejected 第三轮进行中;上手四步未演示给用户
