# mPackStation 前端能力全链路调用测试报告

- 日期：2026-09-18
- 分支：`DEV_2609-VK3`
- 目标实例：后端 `http://127.0.0.1:18871`，前端 `http://127.0.0.1:5273`（vite 代理 `/api`）
- 方法：
  1. **API 链路**：按 `apps/web/src/api/*` 与页面交互路径，对活后端发起完整请求序列（脚本 `.tmp/chain-test/api-chain-test.mjs`）
  2. **UI 走查**：Playwright 打开全部路由，读取页面文本与 console（证据 `.tmp/chain-test/ui-walkthrough.txt`）
- 结果文件：`.tmp/chain-test/api-chain-results.json`

## 总览

| 套件 | 通过 | 失败 | 警告/环境项 |
|---|---:|---:|---:|
| API 链路 | **48** | **0** | 3 |
| UI 路由走查 | 10/10 可打开 | 0 路由崩溃 | 多处 mock/死按钮（见缺陷） |
| Go 回归（后端支撑） | 全绿 | 0 | — |
| 前端生产构建 | 通过 | 0 | chunk >500kB 警告 |

**结论**：前端所依赖的后端主链路在本机 **可用且契约基本对齐**；主要问题不在「接口挂了」，而在 **UI 残余 mock、部分按钮未接线、启动器参数校验不完整**。

## 覆盖矩阵（前端可交互能力）

| 页面/路由 | 交互能力 | 链路结果 | UI 结果 | 备注 |
|---|---|---|---|---|
| `/` 看板 | 环境横幅、继续卡、包列表、任务面板、新建/导入入口 | dashboard/tasks/health/status/onboarding 200 | 有包态渲染正确；CF key 未配置横幅正确 | 任务面板曾因 ListTasks 过滤为空（已修 SQL） |
| `/welcome` 空态引导 | 上手步骤 | onboarding 200 | — | forceEmpty 路由存在 |
| `/packs` 包列表 | 筛选、行点击进包 | GET packs 200（3 包） | 列表有数据 | **页头「导入/新建」按钮无 onClick** |
| 新建包（看板弹窗） | 表单创建 | POST packs 201 | — | 校验：无 token 401；同名由前端拦截 |
| `/packs/:id` 概览 | 已选模组、开始打包、包健康 | mods list 200 | 「当前还没有模组」 | **包健康栏 86/142/3/5/26 为写死 mock** |
| `/packs/:id/mods` 模组 | 搜索、版本选择、添加、推荐、移除 | mod-search MR 20（CF not_configured）；推荐 Polymorph/Sinytra 可见 | 搜索框/推荐卡/已安装区渲染 | 依赖外网 MR；CF 需 key |
| `/packs/:id/dependencies` | 锁快照、冲突、重新解析 | locks/health/conflicts 200；POST resolve 202 | 锁定 1、冲突 0「没有冲突」 | 真实数据 |
| `/packs/:id/content` M6 | 选模组、解析、按 kind 过滤、目录重建 | content/catalog status/rebuild 202 | 原版 Minecraft 解析中；分类 Tab 齐全 | catalog 未建完时 items 409 `catalog_stale`（预期） |
| P6 内容文档 | create/draft/validate/apply/rollback | 全链路 PASS（含 412 陈旧锁） | **UI 不可达**（路由指向 ModContentPage） | 契约字段是 `payload`，kind∈recipe/structure/ore |
| `/packs/:id/quests` | 任务书读写校验应用 | draft/GET/preview/history/validate/apply PASS | **仅 JSON dump**，无图形编辑器 | 首次 GET 404 后可由 draft 自举 |
| `/packs/:id/publish` | 交付检查、版本、产物、构建 | checks/versions/artifacts/releases 200 | content+quest passed；版本 0.1.0 | 构建依赖 export dir 注册 |
| `/packs/:id/launcher` | 安装/启动、日志轮询 | POST install/launch 202 | 任务失败：`version, username and minecraft_dir are required` | 空表单也会入队（缺陷 R4） |
| `/settings` | 平台状态、CF key 保存/清除 | health/status 200；无效 key 400；DELETE 204 | Modrinth 已连接；CF 未探测 | 路径 `/api/system/providers/curseforge/key` |
| 侧栏导航 | 包上下文切换、折叠 | listPacks 200 | 包页内导航指向当前包 | 看板无包上下文时创作工具落到 `/packs` |
| 底边栏 | 模组数/已安装/索引/告警 | dashboard 聚合 | 有数值 | **「查看日志/输出目录」无处理** |
| 上手清单 | 去设置/去登录 | PUT onboarding 200（合法 step） | 全页浮层可见 | step 键：`curseforgeKey/firstMod/firstPack/prismAccount` |
| 导入 | inspect/confirm | 无文件 POST → 400 | — | 非 500，符合预期 |
| 鉴权 | 写操作 token | 无 token → 401 | — | 注入空 token 时前端会如实收到 401 |

## API 链路明细（摘要）

### 通过（核心）
- system：health / healthz / readyz / dashboard / tasks / activities / system.health / system.status / onboarding / mc-versions / packs
- packs：create / get / patch / delete（delete 在有活跃任务时 409，已标 WARN）
- mods：list / mod-search（Modrinth 20 结果；CF `not_configured` 为环境预期）
- deps：locks / health / conflicts / resolve(202)
- content：create → get → draft(If-Match) → stale 412 → validate → history → apply
- quests：draft(If-Match=0) → get → preview → history → validate → apply
- publish：delivery-checks / versions / artifacts / releases
- catalog：status / rebuild(202)；items 在 stale 时 409
- launcher：install(202) / launch(202)
- settings：CF key 无效 400；clear 204
- onboarding：`firstPack` 更新成功
- import：缺参 400

### 警告（非失败）
1. `GET quests` 在尚无任务书时 404 —— 随后 draft 可创建，属自举语义  
2. `GET catalog?locale` 在 rebuild 前 409 `catalog_stale` —— 契约设计如此  
3. `DELETE pack` 409 —— 存在引用/活跃任务时的资源冲突保护  

## UI 走查证据（摘录）

```text
/          → 3 个整合包 · 继续设计 · 包列表 · CF key 横幅
/packs     → 列表 3 行；页头导入/新建按钮无行为
/packs/:id → 概览；包健康 mock 数字；「当前还没有模组」
/mods      → 推荐 Polymorph / Sinytra；已安装(0)
/deps      → 锁定快照 1 · 待处理冲突 0 · 没有冲突
/content   → M6 解析页；原版 Minecraft；kind 分类 Tab
/quests    → JSON 原文展示（非编辑器）
/publish   → content/quest delivery passed；版本 0.1.0
/launcher  → 任务 failed：参数不完整；日志 queued→leased→running→failed
/settings  → CF 未探测 / Modrinth 已连接；缓存 0 B；剩余 3961 GB
```

## 缺陷清单（链路测试视角）

| ID | 级别 | 描述 | 建议 |
|---|---|---|---|
| T1 | P1 | 包概览「包健康」全 mock，与依赖页真实数据矛盾 | 接 `/health` + `/locks` + dashboard.edits |
| T2 | P1 | `/packs` 页头新建/导入死按钮 | 复用 `CreatePackModal`/`ImportPackModal` |
| T3 | P1 | 底边栏日志/输出目录死按钮 | 打开任务抽屉 / 系统状态或导出目录 API |
| T4 | P1 | 启动器空表单可入队并在执行期失败 | 前端全字段校验 + 后端提交前 400 |
| T5 | P1 | 任务书 UI 仅 JSON，无法完成「编辑」产品语义 | 图编辑器或至少表单化章节/节点 |
| T6 | P1 | P6 structure/ore 编辑器未接入路由 | 与多方块重设计一并交付 |
| T7 | P2 | 内容页对「解析中/0 条」缺少空态与失败原因 | 增加 run 状态与错误详情 |
| T8 | P2 | 发布页「开始构建」依赖 export 目录，UI 无注册入口 | 设置页增加允许导出目录管理 |
| T9 | P2 | 概览模组列表/模组数未与 mods API 完全一致展示 | 统一 pack_mods 口径 |
| T10 | P3 | favicon 404 等前端 console 噪音 | 补静态资源 |

## 环境相关说明

- CurseForge：未配置 API Key → 搜索与设置显示 not_configured/未探测（**预期**）
- Modrinth：可达，搜索可用
- 启动器内核：本机未见 `.tools/launcher` release binary，install/launch 任务缺少可执行文件时会失败（属部署缺口，不单是前端问题）
- 服务：后台运行曾出现锁冲突；当前以前台/nohup 单实例方式运行

## 如何重放

```bash
# 1) 服务
cd apps/server && go run ./cmd/server -addr 127.0.0.1:18871 -data ../../data
cd apps/web && npm run dev -- --host 127.0.0.1 --port 5273

# 2) API 链路
node .tmp/chain-test/api-chain-test.mjs

# 3) UI（需 PATH 含 node/npx）
npx --yes --package @playwright/cli playwright-cli open http://127.0.0.1:5273/
```

## 后端配套测试

- `go test ./...` 全绿（含 P7 export-dir 修复后的构建/发布验收）
- 证据脚本与 JSON：`.tmp/chain-test/`
