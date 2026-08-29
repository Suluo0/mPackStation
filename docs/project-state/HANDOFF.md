# Project Handoff

## 当前目标

mPackStation —— Minecraft 整合包制作工具：从搜索到打包，不启动游戏完成整合包（双平台检索 · 依赖自动锁定 · 冲突自动解决 · 配方/结构/任务书在线配置 · 一键打包）。

## 当前状态

当前暂停在后端 P6 完成点。Git 当前分支为 `fix/responsive-lowres`，无远端；用户 UI/文档改动仍在工作区。P6 已完成并通过独立验收，P7 已回滚并暂不继续推进。

2026-08-27 已按 Taster anti-slop 方法完成一轮视觉语言重设计：暖纸网格底、矿物中性色、ember 橙单一品牌色、紧凑工作台密度；移除全局“模组搜索”导航，普通示例入口不再使用绿色 CTA。详见 `docs/design/taster-visual-language.md`。当前已进入第三轮 skill 引导的视觉重构；本轮改动尚未获得用户最终验收。

2026-08-28 追加完成侧边栏完整菜单与包工作台 mock 路由；同时完成后端架构盲审。后端仍是仅含 `/api/health` 的 Go+SQLite 骨架，综合评分约 2.7/10。鉴权必须以后端数据域改造为前置条件，不能直接在现有路由外包 JWT/session。

随后已完成后端 M0 第一轮迭代：数据目录绝对化、HTTP 生命周期保护、存活/就绪探针分离、SQLite 连接级约束、事务化 v1 schema 基线、request-id 和基础单测。复审认为健壮性有改善，但鉴权与业务闭环仍未达到接入条件。

设计迭代随后完成真实浏览器复核：标题改为语义明确的两行；中等桌面宽度下三步走浮层缩放避让 CSS 预览图；390×844 移动视口消除横向溢出，并将浮层缩小固定到右下。桌面/移动截图均无首屏横向溢出，用户最终验收待确认。

- 前端 dev：`http://127.0.0.1:5273/`（`?mock=empty` 切空态；USE_MOCK=true，未接真后端）
- 后端 dev：`http://127.0.0.1:18871/api/health`
- 端口纪律：5173 / 18765 / 18766 是旧项目与 LLM 栈，一律不动

## 最近完成

- monorepo 骨架 + 便携 Go 1.27.0（`.tools/`，走阿里云镜像；直连 go.dev 被墙）
- 看板页全功能（二态/轮询/弹窗/删除确认/环境自检条），tsc+build+双态截图验证通过
- Go 后端空壳 + SQLite 初始 schema（8 表），`/api/health` 双链路验证通过
- 视觉两轮迭代：第一轮补外壳+紫主题对齐设计稿框架；第二轮 redesign-skill 精修 6 项（等宽数字/健康列堆叠/进度语义/动效令牌/按钮钉底/状态层次）
- 用户级 skill 安装：taste-skill、redesign-skill、project-checkpoint（`~/.kimi-code/skills/`）

## 项目功能进展

### 2026-08-29 · P7 构建与发布回滚

- 状态：已回滚 / 延后
- 进展：按用户要求移除 P7 构建、发布 service/repository/测试及 HTTP 接线，保留 P6 内容与任务书实现。
- 关联任务：`task-p7-build-publish`
- 验证：回滚后全量 `go test ./...`、`go vet ./...`、`gofmt` 均通过；回滚提交为 `9cab5c4`。
- 相关产物：`apps/server/internal/httpapi/httpapi.go`、P7 生产文件已从当前版本移除。

### 2026-08-29 · P6 内容编辑与任务书闭环

- 状态：已交付（暂停点）
- 进展：完成内容文档/revision 与任务书图模型的 repository/service/HTTP 闭环；支持 canonical 校验、If-Match 乐观并发、apply/rollback/history、环/孤立/跨包引用校验，以及 activity/outbox/audit 证据。
- 关联任务：`task-p6-content-quest`
- 验证：Luna 独立复验通过；P6 定向测试、HTTP 契约测试、全量 `go test ./...`、`go vet ./...`、`gofmt` 均通过，证据绑定提交 `7d5bf91`。当前工作树因未提交 P7 验收测试的未使用导入而不能宣称全量通过。
- 相关产物：`apps/server/internal/service/p6_content.go`、`apps/server/internal/store/p6_repo.go`、`docs/issues/ISSUE-P6-CONTENT-QUEST`

### 2026-08-28 · 引入 frontend-design-codex 并启动看板第三轮视觉重构

- 状态：进行中
- 进展：引入 `frontend-design-codex`，建立 workbench 间距令牌，重整欢迎页 Hero、入口卡片、灵感卡片和工作台流程的布局规则；修复 CSS 预览组件样式误删，预览继续使用 HTML/CSS 绘制。
- 关联任务：`task-f6b5c7d8`
- 验证：`npm run build` 已通过；浏览器截图验收未完成（localhost URL 策略拦截）；用户验收待确认。
- 相关产物：`C:/Users/<user>/.codex/skills/frontend-design-codex/SKILL.md`、`apps/web/src/ui/workbench/workbench.css`、`apps/web/src/features/dashboard/dashboard.css`、`apps/web/src/features/dashboard/OnboardingView.tsx`

### 2026-08-28 · 侧边栏菜单、包工作台页面与后端架构盲审

- 状态：页面已实现（mock）/后端审查完成
- 进展：恢复完整侧边栏菜单，新增包工作台、模组、依赖与冲突、内容编辑、任务书、打包与发布、设置路由页面；补充 `docs/architecture/backend-capability-draft.md`。三路 GPT-5.6-sol 盲审已发起，已回收两份完整报告。
- 结论：后端只有 `/api/health` 和 SQLite 初始 schema；耦合性、可维护性、可扩展性、健壮性、鉴权就绪度均不足，综合约 2.7/10。鉴权前必须完成 workspace/principal、migration、repository 授权边界、任务幂等与密钥隔离。
- 验证：`apps/web` `npm run build` 通过；源码紫色规则扫描零命中；Go `test/vet` 通过但无测试文件。
- 相关产物：`apps/web/src/app/AppShell.tsx`、`apps/web/src/pages/PackPages.tsx`、`apps/web/src/pages/pack-pages.css`、`docs/architecture/backend-capability-draft.md`

### 2026-08-28 · 欢迎页首屏响应式迭代

- 状态：已实现 / 待用户验收
- 进展：修正主标题中文语义断行；为独立悬浮的上手三步增加中等宽度避让；移动端限制装饰溢出并调整浮层位置与比例。
- 验证：桌面与 390×844 移动视口真实截图通过；移动 `scrollWidth=375`，未出现横向滚动；`npm run build` 通过。
- 相关产物：`apps/web/src/features/dashboard/OnboardingView.tsx`、`apps/web/src/features/dashboard/dashboard.css`

### 2026-08-27 · 看板视觉精修（redesign-skill 第二轮）

- 状态：已实现
- 进展：数据列等宽数字、健康信号两行堆叠（消除折行）、继续卡进度语义修正（模组安装 130/142）、全站动效令牌统一与按压/浮起反馈、空态按钮钉底对齐、环境状态数值层次。
- 关联任务：`task-e5a4b6c7`
- 验证：tsc 零错误、vite build 通过、双态截图回看确认；用户验收未通过（"还是丑"）
- 相关产物：`apps/web/src/features/dashboard/dashboard.css`、`apps/web/src/app/shell.css`、`.shots/dash-populated.png`、`.shots/dash-empty.png`（`.before.png` 为迭代前对比）

### 2026-08-27 · 看板视觉重做第一轮（对齐设计稿）

- 状态：已实现
- 进展：新增应用外壳（220px 侧边栏+顶栏，路由共享，可收起）；紫色主题令牌化；空态 hero+三彩色入口卡+选锁改装流程条+上手三步；有包态 2:1 栅格、继续卡、行式包列表、右侧任务面板；封面按包 id 稳定渐变。
- 关联任务：`task-d4f3a5b6`
- 验证：tsc/build 通过、双态截图风格对齐设计稿框架；用户验收未通过
- 相关产物：`apps/web/src/app/AppShell.tsx`、`apps/web/src/features/dashboard/cover.ts`、`docs/design/dashboard-page-prompt.md` 6.1 节

### 2026-08-27 · Go 后端空壳与 SQLite schema

- 状态：已验证
- 进展：`apps/server` 可运行，18871 端口出 `/api/health`；初始 schema 落地 8 张表并写入四条纪律注释（单库/pack_id 分域/jar_index 共享/原始 JSON 不落库）。
- 关联任务：`task-c3d29e4a`
- 验证：go build 通过；curl 直连与 vite 代理均返回 `{"status":"ok","db":true}`
- 相关产物：`apps/server/cmd/server/main.go`、`apps/server/internal/store/`、`data/mpackstation.db`

### 2026-08-26 · 项目骨架与看板页功能

- 状态：已验证（功能）/ 视觉后被否决返工
- 进展：mPackStation monorepo 从零搭起；看板页按 `docs/design/dashboard-page-prompt.md` 实现全部功能（mock 驱动）。
- 关联任务：`task-a1f07c2e`、`task-b2e18d3f`
- 验证：tsc/build 通过、双态截图渲染正确；功能未被用户否定
- 相关产物：`apps/web/`、`README.md`、`docs/design/dashboard-page-prompt.md`

## 下一步

1. 恢复工作时先修复并独立复验 P7 验收测试文件的编译问题，再决定是否继续 P7。
2. 不要在本检查点继续开发；保留 P6 已通过证据与当前工作区未提交改动。
3. 恢复后重新运行全量 Go 测试、vet、gofmt，并将结果绑定新的工作树指纹。

## 阻塞与已知问题

- **视觉验收两轮未过**：用户基准是 AI 生成设计稿（真实插画、3D 图标、光晕）。无外链图/字体约束下需手工 SVG 资产补齐，这是第三轮的核心工作量。
- **P7 尚未完成**：当前检查点明确暂停于 P6；P7 验收测试文件仍未纳入提交且有编译问题。
- 包行「可更新」折行已修；继续卡「项目进度」语义已改为模组安装率，接真数据时再定最终口径。
- vite 993 kB chunk 警告为既有现象，未处理。

## 关键决策

- 技术栈：前端 React19+TS+Vite7+antd6+zod4，后端 Go+SQLite（否决 Node/Java）；发布形态后定（`decision-3af1a2b3`）
- SQLite 纪律：单库分域、jar_index 按 sha1 跨包共享、原始 JSON 不落库、pack_mods 唯一权威清单（`decision-4ba2b3c4`）
- 流程：每页 docs 提示词 → mock 先行 → 截图验收 → 接后端；AI 效果图内部文字不可信（`decision-5cb3c4d5`）
- 端口：前端 5273 / 后端 18871；旧项目端口一律不碰（`decision-6dc4d5e6`）
- 紫色视觉规范已成文（6.1 节）但标记 needs_review，第三轮可能修订（`decision-7ed5e6f7`）

## 不要重复尝试

- **直连 go.dev / golang.google.cn 下载 Go**：被墙；用阿里云镜像 `mirrors.aliyun.com/golang/`。Go module 代理用 `GOPROXY=https://goproxy.cn,direct` + `GOSUMDB=sum.golang.google.cn`。
- **直连 github.com git clone**：TLS 被断；用 `gh-proxy.com` 前缀镜像（gitclone.com 502 也不可用）。
- **后端占用 18766**：是旧 mc_dev 后端的端口，bind 必失败。
- **杀 5173 上的 PID**：那是旧项目前端进程，特意避开。
- **taste-skill 直接套 dashboard**：其 SKILL.md 自述不适用 dashboard/数据表；用 redesign-skill。
- **每包一个 SQLite 库**：已被否决，单库分域。

## 首先读取

- `docs/project-state/state.json` —— 机器可读状态（任务/问题/想法/决策全量）
- `docs/design/dashboard-page-prompt.md` —— 看板规格 + 6.1 视觉规范（后续页面的风格基准）
- `apps/web/src/features/dashboard/dashboard.css` —— 所有 `--mc-*` 令牌集中地
- `docs/design/baseline/dashboard-baseline.png` —— 视觉基准设计稿

## 验证状态

- 已运行并通过：前端 `tsc --noEmit`、`vite build`、无头 Chrome 双态截图（最近一轮迭代后）
- 已运行并通过：`go build ./...`、`/api/health` 直连+代理双链路
- 未运行：任何自动化测试（项目尚无测试）；eslint 未配置
- 用户验收：看板功能未明确验收；视觉两轮均 rejected，第三轮未开工
