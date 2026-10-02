# web2 开工提示词（绝对路径版，粘到新窗口用）

> 用法：把下面 `=== 复制起点 ===` 与 `=== 复制终点 ===` 之间的全部内容整段粘贴到新窗口。
> 前提：新窗口打开的工作区必须是 `/Volumes/Evo/code/mPackStation`（Mac）。若不是，先切工作区，否则下面的绝对路径全部无效。

=== 复制起点 ===

本次会话的工作目录固定为 `/Volumes/Evo/code/mPackStation`。所有路径一律用绝对路径，命令都用 `cd /Volumes/Evo/code/mPackStation` 起手。开工前先跑一次环境核实，把结果写进报告，不要凭这段提示词里的描述假设环境状态：

```bash
ls /Volumes/Evo/code/mPackStation/apps/web2 2>/dev/null || echo "apps/web2 尚不存在（预期）"
lsof -nP -iTCP:5274 -sTCP:LISTEN || echo "5274 空闲"
lsof -nP -iTCP:18872 -sTCP:LISTEN || echo "18872 未监听 → 停下来报告，不要自己起后端"
git -C /Volumes/Evo/code/mPackStation branch --show-current
```

若 18872 没在监听，**停下来报告并等用户指令**——那是用户的隔离链路基座，起它需要 `-data /tmp/mpack-chain` 与编译产物，不该由你猜着做。

任务：按下面四份已经写好、并经用户确认的方案文档，**新开**一份前端 `/Volumes/Evo/code/mPackStation/apps/web2/`，实现工作台交互设计 v2.1。不要自己另想方案，不要"优化"方案，方案写了什么就做什么。

## 一、必读文件（绝对路径，按顺序读）

1. `/Volumes/Evo/code/mPackStation/docs/design/web2-step1-framework.md` —— 第一步：框架层。**本次只做这一步。**
2. `/Volumes/Evo/code/mPackStation/docs/design/web2-step2-shell-and-pages.md` —— 第二步：外壳与页面容器。先通读了解状态层与插槽契约，第一步验收通过前不要动手写第二步的代码。
3. `/Volumes/Evo/code/mPackStation/docs/design/web2-step3-domain-widgets.md` —— 第三步：领域叶子组件。本次只读不写。
4. `/Volumes/Evo/code/mPackStation/docs/design/workbench-interaction-design.md` —— 设计权威（v2.1）。方案与它冲突时以方案为准（方案已核对过后端真实契约），但要在报告里指出冲突点。

**路径换算规则**：方案文档里出现的相对路径（如 `apps/web2/src/app/router.tsx`、`apps/web/src/api/mods.ts`）一律相对 `/Volumes/Evo/code/mPackStation`，读和写之前都要补成绝对路径。
方案里的行号引用（如 `/Volumes/Evo/code/mPackStation/apps/web/src/api/mods.ts:158`）是给用户复核用的证据，不是让你去读旧实现。

## 二、第一步要产出的文件（绝对路径清单，共 10 个新建 + 1 个迁入目录）

新建（内容见 `web2-step1-framework.md` §4.1–§4.10，照抄该节代码块）：

1. `/Volumes/Evo/code/mPackStation/apps/web2/package.json`
2. `/Volumes/Evo/code/mPackStation/apps/web2/vite.config.ts`
3. `/Volumes/Evo/code/mPackStation/apps/web2/tsconfig.json`
4. `/Volumes/Evo/code/mPackStation/apps/web2/index.html`
5. `/Volumes/Evo/code/mPackStation/apps/web2/src/vite-env.d.ts`
6. `/Volumes/Evo/code/mPackStation/apps/web2/src/styles/tokens.css`
7. `/Volumes/Evo/code/mPackStation/apps/web2/src/styles/base.css`
8. `/Volumes/Evo/code/mPackStation/apps/web2/src/main.tsx`
9. `/Volumes/Evo/code/mPackStation/apps/web2/src/app/router.tsx`
10. `/Volumes/Evo/code/mPackStation/apps/web2/src/app/Placeholder.tsx`

迁入（整目录 / 单文件复制，见方案 §5，命令是 `cp` 不是重写）：

- `/Volumes/Evo/code/mPackStation/apps/web/src/api/` → `/Volumes/Evo/code/mPackStation/apps/web2/src/api/`（全量 1025 行，零改动）
- `/Volumes/Evo/code/mPackStation/apps/web/public/paper-noise.svg` → `/Volumes/Evo/code/mPackStation/apps/web2/public/paper-noise.svg`
- 设计令牌：从 `/Volumes/Evo/code/mPackStation/apps/web/src/features/dashboard/dashboard.css` 的 `:root` 块（第 4-58 行）搬进上面第 6 项 `tokens.css`；删掉两个侧栏宽度变量 `--mc-sider-w` / `--mc-sider-w-fold`（实测在第 18、19 行，第二步做顶栏后它们没有消费者）；加上外壳的五个变量 `--mc-topbar-h` / `--mc-rail-w` / `--mc-rail-w-fold` / `--mc-src-rail-w` / `--mc-src-rail-w-fold`（见方案 §4.6）。行号以文件实际内容为准，动手前先 Read 看清现状。

## 三、铁律（违反任何一条即视为失败，用户会退回重做）

1. **禁改后端**。`/Volumes/Evo/code/mPackStation/apps/server/` 与 `/Volumes/Evo/code/mPackStation/launcherCore/` 一行都不许动。前端契约不合用就在报告里提，不要自己改。
2. **禁改旧前端**。`/Volumes/Evo/code/mPackStation/apps/web/` 全程只读，用户批准替换之前不许动。新代码只准写在 `/Volumes/Evo/code/mPackStation/apps/web2/` 下。
3. **禁读旧实现**（第二步铁律 1）：除了 §5 要求的 `apps/web/src/api/` 整目录迁移和设计令牌源文件，不许打开 `/Volumes/Evo/code/mPackStation/apps/web/src/` 下的 `pages/`、`app/`、`features/`、`ui/`。规格看不懂就在报告里问，不要翻旧代码反推。
4. **禁碰这些端口和这份数据**：`5173`、`18765`、`18766`、`18871`，以及数据库目录 `/tmp/mpack-data`。那是用户正在跑的栈。
   - web2 dev server 固定 **5274**；代理目标固定 **18872**（隔离链路基座，数据在 `/tmp/mpack-chain`）。
   - 起后端/查端口只用这些隔离值；若 18872 没在跑，先报告，不要自己起一个新后端去占别的端口。
5. **令牌绝不落盘**。写接口需要 `X-MPack-Token`，只走环境变量，禁写进任何文件、禁提交：
   ```bash
   cd /Volumes/Evo/code/mPackStation
   export VITE_MPACK_TOKEN=$(grep -o 'chain-token-[0-9]*' /Volumes/Evo/code/mPackStation/scripts/chain-test-run.sh | head -1)
   ```
   `/Volumes/Evo/code/mPackStation/apps/web2/vite.config.ts` 已用 `define` 注入成 `__MPACK_WRITE_TOKEN__`，照方案抄即可。收尾会 grep 确认整个仓库没有硬编码令牌。
6. **禁新增依赖**。`/Volumes/Evo/code/mPackStation/apps/web2/package.json` 的依赖集合必须与方案第一步列出的一致（react 19 / react-dom / react-router-dom ^7.9 / antd 6.6 / zod 4 / vite 7 / typescript 5.9 等）。要装新包先停下来问。
7. **禁 git 操作**。不要 `git add`、`git commit`、`git push`、`git stash`、`git checkout`。当前分支实测是 `DEV_2609-VK4`（2026-10-01 `git branch --show-current` 所得；开工前自己再跑一次确认，别照抄这里写的值），这是共享工作区，有别的写入者，`git add -A` 会把别人的东西带进来。改完就停，让用户自己看 diff。
8. **禁假 affordance**。没有后端支撑的按钮 / Dropdown / 卡片一律不渲染，也不许渲染"敬请期待""后续版本提供"。空态必须指向下一个真实动作。
9. 若需要碰 Rust（本次不需要）：`CARGO_TARGET_DIR` 必须是本地盘 `/tmp/mpack-launcher-target`，绝不设在 `/Volumes/Evo`（SMB 挂载）上。
10. **验收不通过就不算完成**。逐条跑、逐条贴实际命令输出，不许"先往下走回头再修"。

## 四、决策点（全部已拍板，任何一个都不许停下来问）

- 第一步 §8：`api/` **整目录迁移**（1025 行照搬，不从 Go handler 重写 zod）。
- 第三步 D-3A：纯函数层**迁移**，不重写。
- 第三步 D-3B：**进度树砍掉**。`advLayout.ts`(805) / `AdvancementTreeView.tsx`(591) / `advTreeDemo.tsx`(108) / `test-adv-layout.ts`(454) / `adv-tree-e2e.mjs`(220) 全部不迁不写、不建文件、不留插槽；索引态只有网格与表格两种视图。但内容类型筛选里的「进度」kind 保留（那是内容文档类型，与进度树画布无关）。
- 第三步 D-3C：任务书 8 型任务 / 4 型奖励**不精简**。
- 第三步 D-3D：`api/` 的 zod 逐字段声明**不放宽**。

唯一需要等用户明确指令的是第三步 §9（用 `apps/web2` 替换 `apps/web`）——做到那一步只出对照报告然后停。

## 五、本次范围与产出判据

只做第一步：按 `/Volumes/Evo/code/mPackStation/docs/design/web2-step1-framework.md` §4 建出上述 10 个文件，§5 迁入 `api/` 与令牌，§6 在隔离环境跑起来，§7 完成验收 A–H + 3 条人工点验。

命令一律这样写：

```bash
cd /Volumes/Evo/code/mPackStation/apps/web2 && npm install
cd /Volumes/Evo/code/mPackStation/apps/web2 && npm run build
cd /Volumes/Evo/code/mPackStation/apps/web2 && npx tsc --noEmit
cd /Volumes/Evo/code/mPackStation/apps/web2 && npm run dev   # 必须起在 5274，代理 18872
```

判据：
- `/Volumes/Evo/code/mPackStation/apps/web2/` 能 `npm run dev` 起在 5274，6 条路由（`/`、`/packs`、`/packs/:id`、`/packs/:id/content`、`/packs/:id/delivery`、`/settings`）都能看到占位页。
- `npm run build` 与 `npx tsc --noEmit` 全绿。tsconfig 的 strict 开关一个都不许关；`noUnusedLocals` 报错就用方案里给的 `void x` 写法绕，不许改配置。
- 方案 §7 的 A–H 每条命令的实际输出贴进报告，不要只写"通过"。

做完第一步就**停下来报告**，等用户放行再进第二步。报告必须包含：新建/修改文件的绝对路径清单、验收命令的实际输出、遇到的与方案不符之处、以及对第二步的疑问。

## 六、报告要求

- 中文，简洁，先结论再证据。
- 每条声称都要有可复现的绝对路径或命令，用户会抽验。
- 不要往 `/Volumes/Evo/code/mPackStation/docs/` 写进度文档或总结文档，报告直接发在对话里。
- 做不到就如实说做不到 + 原因，不要用近似的东西冒充。

=== 复制终点 ===
