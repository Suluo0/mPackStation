# Project Handoff

## 当前目标

mPackStation —— Minecraft 整合包制作工具：从搜索到打包，不启动游戏完成整合包（双平台检索 · 依赖自动锁定 · 冲突自动解决 · 配方/结构/任务书在线配置 · 一键打包）。

## 当前状态

早期搭建阶段。已初始化 Git 仓库（main 分支，初始提交 `0c6edbe`，暂无远端）。看板页功能完成、后端空壳完成；视觉质量未通过用户验收，是当前主要矛盾。

2026-08-27 已按 Taster anti-slop 方法完成一轮视觉语言重设计：暖纸网格底、矿物中性色、ember 橙单一品牌色、紧凑工作台密度；移除全局“模组搜索”导航，普通示例入口不再使用绿色 CTA。详见 `docs/taster-visual-language.md`。当前仅完成代码与本地双态回看，尚未获得用户最终验收。

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
- 相关产物：`apps/web/src/app/AppShell.tsx`、`apps/web/src/features/dashboard/cover.ts`、`docs/dashboard-page-prompt.md` 6.1 节

### 2026-08-27 · Go 后端空壳与 SQLite schema

- 状态：已验证
- 进展：`apps/server` 可运行，18871 端口出 `/api/health`；初始 schema 落地 8 张表并写入四条纪律注释（单库/pack_id 分域/jar_index 共享/原始 JSON 不落库）。
- 关联任务：`task-c3d29e4a`
- 验证：go build 通过；curl 直连与 vite 代理均返回 `{"status":"ok","db":true}`
- 相关产物：`apps/server/cmd/server/main.go`、`apps/server/internal/store/`、`data/mpackstation.db`

### 2026-08-26 · 项目骨架与看板页功能

- 状态：已验证（功能）/ 视觉后被否决返工
- 进展：mPackStation monorepo 从零搭起；看板页按 `docs/dashboard-page-prompt.md` 实现全部功能（mock 驱动）。
- 关联任务：`task-a1f07c2e`、`task-b2e18d3f`
- 验证：tsc/build 通过、双态截图渲染正确；功能未被用户否定
- 相关产物：`apps/web/`、`README.md`、`docs/dashboard-page-prompt.md`

## 下一步

1. **视觉第三轮（`task-f6b5c7d8`，最高优先）**：图形丰富度是用户两次否定的核心差距——SVG 像素风 MC 场景封面、3D 质感图标块、hero 渐变光晕背景、排印字重字号拉大。差距分析已完成，开工即可。
2. 视觉过关后：写包工作台（`/packs/:id`）提示词文档进 `docs/`（`idea-29e0f1a2`）。
3. 后端接真实端点替换 USE_MOCK（从 dashboard 六个接口开始）。

## 阻塞与已知问题

- **视觉验收两轮未过**：用户基准是 AI 生成设计稿（真实插画、3D 图标、光晕）。无外链图/字体约束下需手工 SVG 资产补齐，这是第三轮的核心工作量。
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
- `docs/dashboard-page-prompt.md` —— 看板规格 + 6.1 视觉规范（后续页面的风格基准）
- `apps/web/src/features/dashboard/dashboard.css` —— 所有 `--mc-*` 令牌集中地
- `C:\Users\suluo\Downloads\ChatGPT Image 2026年8月26日 19_58_15.png` —— 视觉基准设计稿

## 验证状态

- 已运行并通过：前端 `tsc --noEmit`、`vite build`、无头 Chrome 双态截图（最近一轮迭代后）
- 已运行并通过：`go build ./...`、`/api/health` 直连+代理双链路
- 未运行：任何自动化测试（项目尚无测试）；eslint 未配置
- 用户验收：看板功能未明确验收；视觉两轮均 rejected，第三轮未开工
