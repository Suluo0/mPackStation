# Project Handoff

## 当前目标

mPackStation —— Minecraft 整合包工作台:模板开局 → 模组/依赖锁定 → 内容/任务书 → 构建 .mrpack → **自研 mPackLauncher 安装并离线启动 Minecraft**（不要求正版；Prism 仅协议/CLI 兜底）。**流水线终局必须通向启动一个 Minecraft**。

> **进度定位（2026-10-03）**：这条终局链路已**真机跑通过一次**（`scripts/verify-terminal-chain.sh --launch`，43 断言全绿，Minecraft 1.21.1 + Fabric 起窗；脚本可重跑复现，但当时的证据日志被 `.gitignore:6` 的 `*.log` 排除、没有入库），所以项目不缺"能跑"，缺的是**覆盖面与工程性缺口**——唯一成体系的功能缺口是 overrides 未落盘（只有本机 jar 的模组/任务书进不了 `.mrpack`），其余是图标渲染器 R4-R6、73 个 `iconStatus=missing` 物品、16 个无名 banner 方块、以及 `resolver_version` 不参与 stale 判定这类收口项。端口/schema 等易变事实**不要只信本文，读 `AGENTS.md` + `store.go:41`**。

## 当前状态

分支 `DEV_2610-WK1`,远端 `origin = github.com/Suluo0/mPackStation`,HEAD `65cf1e5`。**本分支已推送**（2026-10-03 首推，upstream = `origin/DEV_2610-WK1`，远端 tip 实测 `65cf1e5`）：`6c0aa5d` = 10-02/10-03 两轮改造一次性入库（**190 个路径 = 117 新增 + 73 修改，19742 插入 / 940 删除**，含 `apps/web3/` 整目录、迁移 0021-0027、`launcherCore` Rust 改动、本轮修好的红夹具），`65cf1e5` = README 口径同步，checkpoint 文档在 `d738ec9`（`cp-2026-10-03-zcode-session` 三件套，本行的哈希由其后一条回填提交写下）。点击级 UI 回归 **33/33 PASS**(2026-09-18,本轮未重跑)。

**本轮(10-03)验证快照**（绑定工作区指纹 `sha256:a795433e…b7ef53`，提交前 HEAD `fe7c4ea`）：`go vet ./...` 干净；红项修复后 `go test ./... -count=1` = **10 包全绿**；`npm --prefix apps/web3 run build` 通过（883 kB chunk 警告为既有现象）。详录 `docs/project-state/history/2026-10-03-session-summary.md`。

### 2026-10-03 · 图标渲染器第一批 + 搜索/分类闭环 + 共享环境清空事故（Z Code 会话收尾）

- **事故先看**：`/tmp/mpack-data`(18871/18872) 与 `/tmp/mpack-chain` 的 `packs`/`pack_mods`/`mod_content` 三表全 0 行，13:20 发现。链路库 = `chain-test-run.sh` **设计内重置**（脚本每次运行重置隔离环境），不算事故；日常库是**真实数据丢失**（readd-test 的任务书/目录/魔改记录），**成因未定**——本会话动作可证无关（0025/0026 只动目录四表），但无日志不能指认并行会话。恢复已穷尽：`.recover` 只捞出 9 月旧孤儿数据、APFS 快照不覆盖 /tmp、唯一幸存副本 = 仓库 `data/mpackstation.db` 的 **09-19 快照**（schema 16，3 个旧链路测试包，未回灌）。全文 `docs/tests/incident-2026-10-03-shared-env-wipe.md`。
- **口径定稿（用户 13:36 钦定，已写进 `AGENTS.md` 顶部「服务与环境铁律」）**：唯一后端 `127.0.0.1:18872 -data /tmp/mpack-data`、唯一前端 `5271`(apps/web3)；5173/5273/5274/5275/5276/18871/18880 一律作废；`scripts/dev.sh`/`dev-stop.sh` 按新标准重写为唯一起停方式（Windows 版退役）；动共享数据目录须用户当轮明示；迁移编号先 `ls apps/server/internal/store/migrations/` 再取号。
- **图标渲染器 R1+R2+R0**（计划 `docs/design/item-icon-renderer-plan-2026-10-03.md`）：loader 不再一票否决（`item_layers`/`separate_transforms`/`composite` 各有通路），composite 子模型 `#` 变量就地解引用、按 key 字典序合并；`icon_reason` 落库（migration **0026**）+ 全链透传前端，无图标物品悬停可见原因（如 `runtime_generated_model`）；渲染器 v2→v3。两个合成夹具测试绿（金色 90 像素 / 铁 352 像素证实变量解引用与层叠）。**R4 OBJ、R5 tint、R6 化学桶未做**；R3 `builtin/entity` 按计划「不做，只说清」。
- **模组搜索修复**（用户 10-02 原始痛点「搜 ae2/精致存储 出不到正主」）：别名表 + 复合排序（slug 精确 100 > 别名命中 95 > 编辑距离≤1 容错 85 > 分词前缀 80）+ slug 直取 + 前端防抖。会话内真机三案例 `aer`/`精致存储`/`ae2` 首命中正主；单测 `TestExpandModSearchAlias`、`TestModSearchScoreOrdering` 本轮 `-count=1` 复跑 PASS。
- **包内模组自定义分类**（用户旧需求「优化/科技」类目）：migration **0027** + PATCH 透传（`service/mods.go:818`）+ 来源树分组 UI。真缺陷修在**读取侧**：前端走 `includeBuiltin=true` → `ListPackMembers` 是另一条 SELECT，没查 `category` 列（共用 `scanPackMod` 同步补列），修后浏览器验收见「Minecraft 原版」+「📁 优化 1」分组（JEI）。
- **catalog-v4 在清空后复跑闭环**：重建测试包得 1754 → **1330** 条真实物品、无名 0、`evidence` 全 `lang`、图标路径保留；18 个疑似残渣核对为真·纹饰锻造模板官方物品，非残渣。
- **本轮新暴露缺陷**：① `apps/server/internal/store/category_test.go:23` 夹具自己违反 `pack_mods` 建表 CHECK（`source='modrinth'` 却 `project_id=NULL`），确定性可复现 → **checkpoint 会话已改一行修绿**（`project_id` 给真值 `'proj-jei'`），`go test ./... -count=1` = 10 包全绿，随 `6c0aa5d` 入库；② README 口径双过期（`:22` 写 schema 26 而 `store.go:41` = 27；`:70-72` 仍写前端 5273 / 后端 18871，与铁律冲突）——**未修，仍是待办**（见「下一步」）。
- **未开工**：链路引擎 P0（`docs/design/craft-chain-engine-plan-2026-10-03.md`，纯前端 5 新文件 + 3 处挂载 ≈350 行，规格齐全、后端零改动）——实测 `apps/web3/src/` 下无 `chain/` 目录、`grep -ri chain apps/web3/src` 零命中；会话在 14:55 宣布进入该项后被打断。

### 2026-10-02 · 物品目录条目资格收口（catalog-v4）

- 用户报的 junk 全部定位并修掉：目录曾把「模型文件清单」当物品（v2 1754），改语言键后又漏进「官方语言表比版本超前的键」（v3 1493）。v4 判据 = **有名字键 ∧（有 `models/item` 资产 ∨ 被配方/标签引用）**，方块物品形态另需 blockstate，air/structure_void/light 进人工排除表。
- 实测基线（1.21.1 原版全量重解析，走 `POST /catalog/rebuild` 真接口）：物品 **1330** / 方块 1062 / 图标 1257 / 配方 1290 / 标签 331；含点 ID、非 `minecraft:` 命名空间、`model_path` 空、无名物品、图标孤儿、标签悬挂成员 **均为 0**；`catalog_generations` 里 v3 记录转 `superseded`。判据与逐条核查见 `docs/design/web3-ide-shell-v3.md` §「条目资格判据定稿」。
- 关键事实（反直觉、已 sha1 核对）：Mojang 的 `zh_cn.json` 比客户端 jar 超前且从不删键——1.21.1 的 zh_cn 有 803 个 `item.minecraft.*` 键、jar 内置 en_us 只有 645 个，en ⊂ zh。所以语言键单独不能作为物品证据。
- 未提交代码位：`internal/service/item_catalog.go`（`referenced` 集合 + finish 过滤 + `catalogResolverVersion = "catalog-v4"` + `langKeyToID` 拒绝含点路径 + 图标落库存在性守卫 + 方块名 `item.` 兜底）、`item_catalog_test.go`（fixture 增加遗留键与 `modifiers.*` 反例）。验证：`go build ./...`、`go vet`、`go test ./...` 全绿，`MPACKSTATION_TEST_VANILLA_JAR=... go test -run TestFullVanillaCatalogCoverage` 通过。
- 待办（本轮明确未做）：① 73 个真实物品 `iconStatus=missing`——渲染器不支持 `builtin/entity`（旗帜/床/潜影盒/头颅/饰纹陶罐/conduit）与 `requires_tint`（树叶/草方块）；② 16 个 `*_wall_banner` 方块在 Mojang 任何语言文件里都没有键，只能显示裸 ID；③ `resolver_version` 目前**不参与** stale 判定，规则升级后旧包不会自动重建（本次靠显式 POST rebuild）；④ 前端遗留问题（默认 `tool:'focus'` 空侧栏、目录 2.6MB 全量下发、locale 不入 URL、parse interval 泄漏）本轮按用户要求未动。
- 环境：`/tmp/mpack-chain` 4 个测试包已按用户指示经 API 删除（库仍 410MB，未 VACUUM）；验证用的探针实例仍在跑（127.0.0.1:18873，数据 `/tmp/mpack-rebuild-test`，包 `RebuildProbe`）。


### 2026-09-30 · 终局链路打通（构建 → 启动器）

- 全链路复测:`scripts/chain-test.py` + `scripts/chain-test-run.sh`,隔离环境(后端 18872 / 前端 5273 / `-data /tmp/mpack-chain`),**173 例 PASS 172 / FAIL 0 / SKIP 1**,证据 `docs/tests/evidence/chain-run15-2026-09-30.log`,报告 `docs/tests/chain-test-2026-09-30.md`,缺陷逐条台账 `docs/tests/defects-2026-09-30.md`
- **D1 已修**:构建改为服务端按权威链自行装配真实 `.mrpack`(隔离环境产物 539 字节,manifest 含 JEI 真实 Modrinth URL + sha1/sha512)。装配只读 SQLite + 写 zip,**不需要网络**,因此 Electron 化时同一份 Go 代码在主进程内直调即可
- 修掉一个静默数据丢失:`assembly_repo.go` 的 `JOIN pack_mod_selections` 会让「只有本机 jar、无平台选中项」的本地模组从装配结果里凭空消失(构建却报成功)→ 改 LEFT JOIN + `COALESCE`,单测 `build_input_codes_test.go:TestBuildDoesNotSilentlyDropModWithoutSelection` 锁住
- 错误码收敛:`build_lock_mismatch`(422,带 lockId 与取回路径)、`build_input_conflict`(409)、`catalog_not_built`(读未构建包目录)、`quest_book_not_found`/`quest_revision_not_found`、`launcher_not_installed`(409)。`httpapi.go:258/262` 的两个兜底映射保留但不再被这些路径依赖
- **D5 已修**:launcher 二进制走 `MPACK_LAUNCHER_BIN`;安装记录落 `launcher_installs`(migration 0022),版本 ID 契约 `fabric-loader-<loader>-<mc>` 对齐内核 `loader/fabric.rs:43-45`,`GET /api/launcher/installs?packId=&minecraftDir=` 可查并按已装 ID 启动
- 假数据残余清理:无真实数据源的块改为空态或移除,死 CSS 一并删;ASM 包任务书页曾疑似跨包串数据,经 DB(仅 1 行 `quest_books`)与 `useEditors.ts:33-45 defaultQuestDraft` 核查为前端空草稿,**非缺陷**
- ~~验证边界:`launcherCore` 本机无 cargo/rustc 从未编译,launcher 全链只用协议桩验证~~ **这条是误判**(见下方「2026-09-30 · 终局跑通」);`.mrpack` 的 sha512 现由 provider 解析带上真值;「已安装版本」UI 块经代码+API 核验,截图未取得(应用内浏览器无可见表面,且需原生目录选择器)
- ~~终局剩余一处:O14 —— `launcherCore` 无 `.mrpack` 读取能力,需用户决策~~ **已实现并真机验证,不是决策项**:内核 `launcherCore/src/mrpack.rs` 读 `.mrpack`,Go 侧 `install --mrpack` 走同一条任务链路

### 2026-09-30 · 流水线终局真的跑通了一次

- `bash scripts/verify-terminal-chain.sh --launch`(后端 18874 / 数据 `/tmp/mpack-terminal` / `MPACK_LAUNCHER_BIN` 指向 cargo 产物):**43 条断言 PASS 43 / FAIL 0**,证据 `docs/tests/evidence/terminal-run-2026-09-30.log`。链路是纯 HTTP 驱动:建包 → 面向包搜索 → 添加模组(真下 jar) → `/resolve` → 建版本 → `/build` 装配 `.mrpack` → `/api/launcher/install`(真内核) → 磁盘 sha1 复核 → `/api/launcher/launch` → **Minecraft 1.21.1 + Fabric 0.16.14 起窗**(进程存活、日志有 Fabric 横幅与 `Setting user`、无 `Mod resolution failed`)
- **"本机无 cargo/rustc"是误判**:工具链一直装着(brew rustup,`~/.rustup/toolchains/stable-aarch64-apple-darwin/bin`),只是不在默认 PATH。`verify-terminal-chain.sh` 里有探路兜底;`CARGO_TARGET_DIR` 固定在本地盘 `/tmp/mpack-launcher-target`(Evo 是 SMB 挂载,target 放上面会慢到不可用)
- 真机跑通之后一次性暴露并修掉 12 个缺陷:`O14`(内核读 `.mrpack`)、`O15`(导出目录换名重登记 500)、`O16`(加载器 url-only 库漏装 → `ClassNotFoundException: KnotClient`)、`O17`(macOS runtime 布局探测)、`O18`(下载 runtime 缺执行位)、`O19`(竞速下载残留 partial)、`O20`(致命冲突不拦构建 → 装上就崩)、`O21`(过期冲突不结案)、`O22`(原样回传锁快照仍被判不一致)、`O23`(`known_issue` 撞 CHECK)、`O24`(依赖类型不分)、`O25`(平台不可用被写成 error 级冲突)。逐条出处见 `docs/tests/defects-2026-09-30.md` §1
- 教训一条并写进记忆口径:**文档里出现"本机做不到/从未编译"这类结论,必须先跑一次工具探针**(`which cargo`、`rustup which cargo`)。这次误判把已经能做的事挂成了"等用户拍板",白白绕了一圈
- O5 已量化未改:删除空包同步耗时实测 9.61/9.79/9.82 s(257MB 库上疑缺 FK 覆盖索引),属 UX 决策项(改入队异步+任务进度 or 建索引), chain-test `[29]` 已把耗时写进 note

### 2026-09-29 端到端能力基线(`scripts/e2e-baseline.sh` + `docs/tests/e2e-baseline-2026-09-29.md`):**PASS=27 FAIL=3**
  - 真实可用:建包 → 面向包搜索 → `/mod-versions` 兼容版本 → 添加模组(真实 jar 名) → 锁依赖/冲突 → 包内容目录(zh_cn,物品/标签/图标) → 模组内容解析(83 条,终态 success) → 任务书 draft/validate/apply/preview/history → 导出目录注册
  - **终局断两处**(→ 均已修，见上方 2026-09-30 条目):①构建不装配包——`build.go:405` 要求调用方自带 `files[]`,产物实测 283 字节 zip 只含 1 个占位 `modrinth.index.json`,无 jar、非 .mrpack,全仓 Go 无 `mrpack`/`manifestVersion`;②启动内核当时在 macOS 不可用——`launcher.go:38` 硬编码 `mpack-launcher.exe` 且把 `-data` 根目录当 workbenchRoot,无 LookPath 兜底。~~本机 `launcherCore` 从未编译~~ 这句后半是误判(本机 cargo 一直可用,见上方「终局跑通」)
  - 其余缺陷:版本号冲突裸 500(应为 409,且建包自动写入 0.1.0/draft 使首次创建必撞)、`GET /quests` 把「没任务书」报成 `pack_not_found`、launcher install 必填项到异步才报错、启动器接口零契约、catalog 域无「按物品反查配方」端点
  - 撤销的误判(下轮别再报):解析成功后**确实**自动重投目录重建(`mod_content_task.go:129`);任务终态按契约 `:50` 就是 `success`(不是 `succeeded`);`sourceRevision` 是跨包全局批次号,不是泄漏
- 2026-09-19 special 配方:后端 parseRecipeFile 将 `minecraft:crafting_special_*`/`decorated_pot`/recipe 路径下无 ingredients·result 的 JSON 标为 IsDynamic;前端 RecipeViewer 增加 special 分支与「特殊/动态」列。需 force 重解析后生效。
- 2026-09-20 T14 开发包收口：
  - **列表 padding**：`.wb-card` 默认 `padding: var(--wb-panel-padding)`（24px）；`.mod-content-list` / `.mod-toolbar-card` 与设计系统刻度对齐
  - **进度树画布 M1**：`AdvancementTreeView` + `advLayout`（拖动/缩放/k_fit 分离/环打断/§5bis 分组）；内容编辑→进度 Tab 可切换树/表格；单测 `apps/web/scripts/test-adv-layout.ts` 18/18
  - **任务书连线 M1**：edges 权威 + 坐标 + 前置同步；见下方专题条目
- 2026-09-20 任务书 M1：**M1 任务图编辑：边+坐标+前置同步；非完整 FTB；AND/OR 与游戏内导出后续**。UI 以 draft.edges 为权威，保存同步 prerequisites；Shift+点击连线；保存前本地环检测。验证：`scripts/verify-quest-m1.sh`（tsc/build + go quest 测试 + API A→B GET edges 非空）。

### 2026-09-20 · 任务书/进度：读原始实现后的体验级重做

- 设计权威：`docs/design/quest-book-ftb-experience.md` v1.1、`docs/design/advancement-vanilla-experience.md` v1.1（进度以 vanilla 稿为准；canvas 旧稿仅历史参考）
- 任务书：QuestBookEditor — 章节 rail + 可拖画布 + 分组 Inspector + 图例 + 编辑/预览；tasks/rewards/依赖 token（FTB）+ 预览模拟完成/锁定/领奖；migration **0020** meta 列
- 进度：Tab=有 display 的 root；recipes 不进玩家树；requirements **OR(AND)** 三态；hover+Inspector；有界 pan；滚轮平移；缩放标「工作台增强」
- 验证：go test/vet、tsc+build、test:quest、layout 28/28；独立测试+验收代理 **允许结束**
- 残余：跨章依赖点击跳转、连线箭头、size 视觉、章节重命名/排序、服务端未强制 edges→prerequisites、HANDOFF 外文档需 checkpoint 刷新
- **非完整 FTB 导出/SNBT/真实存档**；AND/OR/AND_N UI 已进预览求值

### 2026-09-19 · 中断会话收尾

- 用户点名问题:任务书报错、内容页排版、游戏目录选择器、发布页按钮、模组增删解析链路 —— 均已修复
- 后端:removed 模组可再次添加(migration 0017 + AddPackMod 复活行);`GET /api/fs/browse` 目录浏览
- schema version **18**；上手清单去 Prism 化：第4步改为「配置启动台（离线即可）」，`prismAccount` 契约字段恒 true（废弃）
- 环境:Evo SMB 上 `data/` 曾卡死 server(进程 UN);本轮验证用本地 `-data /tmp/mpack-data`
- 产物:`docs/tests/ui-click-regression-2026-09-18.md`

- 一键启停:`scripts/dev.sh`(启动前后端)、`scripts/dev-stop.sh`(停止)。**2026-10-03 已按新口径重写**为 macOS 唯一起停入口(硬绑 18872/5271 + 端口占用检查);`.ps1`/`.bat` 为 Windows 侧遗留,同一套逻辑但已不再是标准
- 前端 dev:`http://127.0.0.1:5271/`(apps/web3);后端 dev:`http://127.0.0.1:18872/api/health`。**5273/18871 等历史端口一律禁用**,见 `AGENTS.md`「服务与环境铁律」
- **当前机器是 macOS(arm64),项目已从 `D:\workIn\mPackStation` 迁到 `/Volumes/Evo/code/mPackStation`**;Go 1.27.1 darwin/arm64
- 正在跑的 dev 后端:`/tmp/mpackstation-server -addr 127.0.0.1:18872 -data /tmp/mpack-data`(**数据不在仓库 `data/`**,因 Evo 是 SMB 挂载,`data/` 曾卡死进程)。2026-10-03 16:22 实测在跑:该后端 + `apps/web3` 的 vite 5271
- 启动器内核:`launcherCore/`(Rust)。**已用本机 cargo 真编译真运行**(产物 `/tmp/mpack-launcher-target/debug/mpack-launcher`,target 目录必须在本地盘,不要放 SMB)。~~本机从未编译过、4.77MB binary 是 Windows 遗物~~ 那句是误判:工具链一直装着(brew rustup),只是 `~/.rustup/toolchains/stable-aarch64-apple-darwin/bin` 不在默认 PATH,脚本已做兜底
- 写操作需 header `X-MPack-Token`
- 数据库:schema version **27**(migrations 0001-0027;`0025` 目录物品 evidence 加 `lang`、`0026` 图标缺失原因 `icon_reason`、`0027` 包内模组自定义分类 `category`,与 `store.CurrentSchemaVersion = 27` 一致)

## 项目功能进展

### 2026-10-03 · 新增：无图标物品「为什么没有」可见 + loader 拒绝类模型出图

- 状态：已实现
- 进展：来源树/目录里缺图标的物品悬停可见原因（`runtime_generated_model` 等，`icon_reason` 随目录条目落库并透传前端）；`item_layers`/`separate_transforms`/`composite` 三类原先被 loader 一票否决的模型现在能出图，composite 的 `#` 变量就地解引用并按 key 字典序合并；渲染器版本 v2 → v3。
- 关联任务：`task-icon-renderer-r0r2-20261003`
- 验证：已运行 `go test ./internal/service -count=1`（含两个新增合成夹具：金色 90 像素 + 铁 352 像素）结果通过；`go vet ./...` 干净。缺口未清零——R4 OBJ(+22)、R5 tint(+15)、R6 化学桶(+18) 未做，见 `issue-item-icon-coverage-gap`。
- 相关产物：`apps/server/internal/service/item_model_resources.go`、`mod_content_icons.go`、`item_icon_render_test.go`、`apps/server/internal/store/migrations/0026_catalog_item_icon_reason.sql`、`docs/design/item-icon-renderer-plan-2026-10-03.md`

### 2026-10-03 · 新增：模组搜索按「用户想找什么」排序（别名/复合打分/slug 直取）

- 状态：已实现
- 进展：搜 `ae2`、`aer`、`精致存储` 这类简称或中文名时，正主排第一位。做法 = 别名表展开 + 复合排序（slug 精确 100 > 别名命中 95 > 编辑距离≤1 容错 85 > 分词前缀 80）+ slug 直取，前端加防抖。取代原先「按平台下载量重排埋掉正主」的行为。
- 关联任务：`task-mod-search-alias-20261003`
- 验证：`go test ./internal/service -run "Alias|Search" -count=1` 通过（`TestExpandModSearchAlias`、`TestModSearchScoreOrdering`、`TestP5ModChainSearchAddResolveAndHealth`）；三案例真机首命中为会话内浏览器/API 实测，**未沉淀为可重放脚本**。
- 相关产物：`apps/server/internal/service/modsearch_alias.go`、`modsearch_alias_test.go`、`mods.go`、`apps/web3/src/panels/SourcesPanel.tsx`

### 2026-10-03 · 新增：包内模组自定义分类与来源树分组

- 状态：已实现，自带回归测试红（夹具数据不合法，非功能缺陷）
- 进展：给包内模组打自定义类目（「优化」/「科技」…），来源树按分类分组、空值归「未分类」；写入走 PATCH，两条读取路径（`ListPackMods` 与前端实际使用的 `includeBuiltin=true` → `ListPackMembers`）都带出 `category`。
- 关联任务：`task-pack-mod-category-20261003`、缺陷 `issue-category-testfixture-check`
- 验证：分类 PATCH 回读（JEI → 「优化」）与浏览器分组验收在会话内通过；`go test ./...` 当前**不为全绿** —— `TestPackModCategoryAcrossReads` FAIL（`category_test.go:23` 夹具 `source='modrinth'` + `project_id=NULL` 违反建表 CHECK），2026-10-03 16:22 `-count=1` 确定性复现。
- 相关产物：`apps/server/internal/store/migrations/0027_pack_mod_category.sql`、`store/mod_repo.go`、`store/pack_scoped_repo.go`、`store/category_test.go`、`service/mods.go`、`apps/web3/src/panels/SourcesPanel.tsx`

### 2026-09-19 · 任务书 M1 任务图编辑（诚实子集）

- 状态：已实现并通过 tsc/build + API 边回读验证
- 范围：**M1 任务图编辑：边+坐标+前置同步；非完整 FTB；AND/OR 与游戏内导出后续**
- 进展：
  - UI 以 `draft.edges` 为图权威；保存时把每个节点 `prerequisites` 同步为指向它的 `edges.fromNodeId` 列表
  - 画布使用 payload `x/y`（缺失时按 chapter 内 position 网格初始化并写回 draft）
  - SVG 父→子连线，画布可滚动；点击节点 A 再 Shift+点击节点 B = 创建 edge A→B
  - Inspector 显示 prerequisites 列表，可从画布/下拉添加、移除；可编辑 title/description
  - 选中边可删除；保存前本地 Kahn 环检测并提示（后端 validate/save 契约已有 cycle）
- 非目标（禁止扩 scope）：SNBT/FTB 导出、任务完成模拟/解锁运行时、AND/OR/AND_N UI
- 验证：`npx tsc -b` / `npm run build` 通过；纯函数断言通过；API 创建 A→B 保存后 GET 回来 `edges` 非空且 prerequisites 已同步
- 相关产物：`apps/web/src/features/quest/questGraph.ts`、`apps/web/src/pages/PackPages.tsx`（QuestEditorPage）、`apps/web/src/pages/pack-pages.css`、`scripts/verify-quest-m1.sh`、`docs/project-state/history/quest-m1-verify-*.log`

### 2026-09-07 · 包内物品/方块/配方/标签/多语言目录

- 状态：已实现并在现有数据上验证，待用户验收，未提交。
- 表：新增目录状态、物品、方块、物品-方块、多语言名称、物品/方块标签、标签原始定义、直接边、展开成员、配方、配方引用和 PNG 图标表。
- 初始化：创建整合包或修改 MC 版本后自动提交 `catalog_init`；模组解析成功后自动提交重建。原版资源按版本 SHA-1 校验，中文通过 Mojang asset index 获取。
- 语义：名称不参与等价判断；配方具体物品精确匹配，标签材料才展开候选；同一成员支持反查所有标签。
- 实测：原版 1.21.1 为 1754 物品/1062 方块/1290 配方/331 标签；现有 KingingProject 合并 AE2 后为 2133/1164/1778/424。`c:dusts/ender_pearl` 展开为 `ae2:ender_dust`，中文名和样例图标读取成功。
- 验证：全量 Go test/vet、前端生产构建、迁移/FK/级联/API 鉴权、原版全量覆盖、Mojang 中文资源和 zod fixture 均通过。
- 决策：`docs/decisions/ADR-pack-content-catalog.md`；任务 `task-pack-content-catalog-20260907`。

### 2026-09-07 · 方块图标修复 + 原版资源与标签图标

- 状态：已验证，待用户验收，未提交。
- 进展：修正投影方向/可见面/UV；支持模型继承、elements、逐面纹理、半砖/楼梯、透明图层。新增按 MC 版本共享原版资源缓存，界面使用新 resolve 接口避开旧图标快照。
- 实测：MC 1.21.1 + AE2 生成 1766 张图标，其中 AE2 349 张；29 个 AE2 item 模型暂不支持（含辅助模型）。铁锭、红石粉、钻石及高级卡配方标签图标已在真实页面核对。
- 验证：go test ./...、go vet ./...、npm run build 通过，含专项回归；完整旧四层网络矩阵未重跑。
- 限制：自定义 loader/运行时染色/实体图标、跨模组资源叠加仍待补齐；通用标签代表成员仅核实 1.21.1 的铁锭/红石粉/钻石。
- 记录：`history/2026-09-07-item-icons-fix.md`；任务 `task-item-icons-fix-20260907`。

### 2026-09-07 · M6 模组内容解析: lang/recipe/ammo/texture/item_icon + 等距投影渲染器

- 状态:已实现,待用户验收(部分缺陷已记录)
- 进展:
  - **五种内容类型**: lang(语言文件翻译)、recipe(配方)、ammo_definition(物质炮弹药定义,非配方)、texture(原始纹理PNG)、item_icon(物品栏图标)
  - **migrations 0009-0012**: 扩展 mod_content.kind CHECK 约束
  - **等距投影渲染器**(`item_icon_render.go`): rotation[30,225,0],三面亮度分级(top1.0/right0.82/front0.65),仿射纹理映射,输出32×32 PNG
  - **JEI风格配方展示**: 3x3输入网格+箭头+输出物品,支持crafting_shaped/shapeless/transform
  - **ammo_definition分类**: matter_cannon不是配方(weight是权重不是重量,fish是标签不是鱼),68条从recipe迁移
  - **支持模型类型**: generated/handheld(2D layer0)、cube_all(3D等距)
  - **启动台菜单和页面**: 左侧新增启动台菜单,右侧启动台页面
- 关联任务:`task-mod-content-parse-m6`
- 验证: AE2 19.2.17解析——2367条内容,488真正配方,68ammo,190item_icon(145generated+45cube_all),614texture,589item_model,18lang
- 已知缺陷(见 state.json issues):
  - `issue-item-icon-coverage-gap`: 329物品无icon(复杂模型:自定义elements/半砖/楼梯/线缆部件)
  - `issue-content-page-layout`: 内容编辑页面排版问题
  - `issue-service-stability`: 前后端服务经常挂掉
  - `issue-dynamic-recipe-parse`: 动态代码配方无法解析(如AE2线缆伪装合成)
  - `issue-bottombar-mock-data`: 底边栏使用mock数据

### 2026-09-04 · mPackLauncher M5 后端集成(Go 后端调用 Rust 内核)

- 状态:后端完成,前端待设计图确认
- 进展:migration 0007 添加 launcher_install/launcher_launch task kind;service/launcher/runner.go 封装 binary exec + JSON Lines 解析;install/launch handler + 心跳保活;POST /api/launcher/install、/api/launcher/launch 端点。go build+test 全通过。前端集成被用户打断回滚(要求先出设计图再改代码)。
- 关联任务:`task-launcher-m5`
- 验证:go build ./... 通过;go test ./... 全通过;7 files changed, 370 insertions
- 提交:`b78cd27`(M5代码)、`2412a1a`(state)、`a363d8a`(checkpoint)

### 2026-09-04 · mPackLauncher M3+M4(Java下载+OAuth+错误处理+体积优化)

- 状态:已完成验证
- 进展:M3 Java自动下载(Mojang/BMCLAPI runtime清单+2并发+指数退避)、微软OAuth device flow(XBL→XSTS→Minecraft,client_id借Prism公开ID)、keyring凭证存储、离线账号UUID v3。M4 natives解压、19种错误类型+exit_code+suggestion、release 4.77MB。91测试全通过。
- 关联任务:`task-launcher-m3`、`task-launcher-m4`
- 验证:Java 17.0.15下载成功可运行;微软登录验证成功(用户名Suluo0);release binary 4.77MB<10MB目标

### 2026-09-03 · mPackLauncher M0-M2(骨架+下载层+加载器)

- 状态:已完成验证
- 进展:M0 CLI/error/protocol/platform/lock骨架;M1 自研下载层(BMCLAPI镜像+断点续传+SHA1校验+双Semaphore并发)+Vanilla安装+Java检测+启动;M2 四种加载器安装(Fabric/Quilt/Forge/NeoForge)。端到端验证:1.20.1 Vanilla+Fabric安装+离线启动成功。
- 关联任务:`task-launcher-m1`、`task-launcher-m2`
- 验证:74测试全通过;Fabric 1.20.1安装+离线启动成功(PID=13036)

### 2026-09-01 · 四层契约验收体系 + 后端架构改造 B1-B7(B5 暂缓)

- 状态:已完成(B5 用户拍板暂缓)
- 进展:`scripts/verify-contract.bat` 可重放四层验收(静态门禁/curl 矩阵/E2E 22×2/每项专属机械证据);B1 消灭 source any、B3 错误翻译官归一(幂等冲突 422)、B2 httpapi 1395→385 行拆 8 域文件(73 路由 diff 一致)、B6 空值恒发 null、B7 pN 改域名、B4 TaskView 移出 task 包。
- 关联任务:`task-verify-contract-4layer`、`task-b-series-refactor`
- 验证:每项改造后四层全绿才提交;基线 41 项矩阵起步。
- 验收:用户全程看脚本输出验收,逐项提交。
- 相关产物:`scripts/verify-contract.sh`、`scripts/contract/`、`apps/server/internal/httpapi/routes_*.go`、`apps/server/internal/service/task_api.go`

### 2026-09-01 · 交互三项:版本倒序 + 兼容优先 + CF key 配置

- 状态:已完成
- 进展:平台版本统一按日期新→旧;版本下拉兼容当前包的提前、默认选中最新兼容版;设置页 CF key 全链路(保存前真实验证/无效 400/即存即效/任何接口不回传/env 优先);启动探测双平台可达性修掉"未探测"假象。搜索交互用户拍板不动。
- 关联任务:`task-ux-trio-20260901`
- 验证:实测无效 key 400、真 key 保存/清除/恢复全通;矩阵扩到 48 项。
- 相关产物:`apps/server/internal/service/system_config.go`、`apps/web/src/hooks/useModSearch.ts`、`docs/api/contract.md`

### 2026-09-01 · 双平台模组身份合并 + 版本镜像钉死

- 状态:已完成
- 进展:同名模组双平台合并一张卡(身份表优先、名称规范化兜底、slug 否决);合并卡打双平台标签、下载量合并;pack_mods 一行+镜像字段(迁移 0005),添加时另一平台版本立即钉死永不追新;查不到照常添加标"仅单平台";内置只读身份知识库随 exe 分发,用户库永不进包(package.ps1 断言);主版本换版镜像重钉。
- 关联任务:`task-dual-source-merge`;关联决策:`decision-dual-source-naming`
- 验证:实测 JEI 合并卡;添加 MR JEI 自动钉 CF fileId(jei-1.21.1-neoforge-19.51.0.417.jar 同版本号);矩阵 50/50。
- 验收:用户批准方案("可以 就这么干")后实现,实测通过。
- 相关产物:`apps/server/internal/service/mods.go`、`mod_identity_baseline.{go,json}`、`apps/server/internal/store/migrations/0005_mod_identity.sql`

### 2026-09-01 · 兼容知识库:自动修复 + 推荐位

- 状态:已完成(knownIssues 等用户整理数据)
- 进展:内置只读兼容知识库(人工核实条目,source 必填);添加模组自动扫描已知冲突,有解法自动加装兼容补丁(可见/可移除/日志注明);修不掉的进冲突列表 `known_issue`;推荐位按包 MC/loader 过滤一键添加;种子 Polymorph/Sinytra 双平台实测核实;`docs/compat-knowledge.md` 为维护者整理指南。
- 关联任务:`task-compat-knowledge`;关联决策:`decision-compat-knowledge-autofix`
- 验证:neoforge 包推荐返回 2 条、fabric 包正确滤掉 Sinytra;5 个单测;矩阵 53/53。
- 相关产物:`apps/server/internal/service/compat_knowledge.{go,json}`、`docs/compat-knowledge.md`

### 2026-09-01 · 新登记需求:客户端采集上报(仅记录,不实现)

- 状态:已登记(用户明确先不实现)
- 进展:`idea-client-telemetry-upload` —— 分发出去的包在客户端跑出的兼容问题/配对确认,未来应能采集回流服务端,喂给知识库。设计候选方向已记在 idea 里。


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
> 2026-10-03（checkpoint 会话）重排：1-3 已完成；4-5 是新的最高优先安全项（跑任何验证脚本前先做 4）；6-8 本轮新排；原 6-12 顺延为 9-14。
> 2026-09-30 重写：原先列在此处的「先拍板 D1/D5 两条终局断点」「Phase 0 待拍板」「小缺陷批次(版本号 500→409 / quests 错误码 / install 同步必填 / 启动器进契约 / 配方反查端点)」**全部已完成并验证**，不再是待办。

1. ~~**先修一行让 `go test ./...` 归绿**~~ **已完成于 `6c0aa5d`**：`category_test.go:23` 夹具的 `project_id` 改真值 `'proj-jei'`，复跑 `cd apps/server && go test ./... -count=1` = **10 包 ok**（第 11 个包 `cmd/server` 无测试文件），`go vet ./...` 干净。
2. ~~**提交积压**~~ **已完成**：190 个路径（117 新增 + 73 修改，19742 插入 / 940 删除）随 `6c0aa5d` 入库，**已推送 `origin/DEV_2610-WK1`（本分支首推，upstream 已建立）**。
3. ~~**README 同步**~~ **已完成于 `65cf1e5`**：`:22` schema 26 → 27（补 `0027` 说明）、`:58` 与端口段改 5271/18872 并列作废端口；同批补了第 4 条的撞口警告。
4. **修 `chain-test-run.sh` 撞口（最高优先安全项，`issue-chain-test-port-collision`）**：该脚本 `:8` 默认后端端口就是 **18872 = 唯一开发实例端口**（`CHAIN_PORT` 可改），而它的 `pkill`（`:29-30`）只匹配自己的 `mpack-chain-server` 二进制，杀不掉 `dev.sh` 起的 `go run`；`wait_up`（`:20-26`）只看 `/api/health` 是否 200 —— 开发后端会答这个 200，于是 173 个用例（含建包/删包）直接打在 `/tmp/mpack-data` 上。这与 10-03 日常库被洗空同型。**修法**：起服务前用健康响应体校验实例身份（或让 `/api/health` 带数据目录/实例标记），不合就硬失败。**在修好之前跑它必须先 `scripts/dev-stop.sh`**。
5. **重跑两套终局脚本对齐 schema 27**：`chain-test-run.sh` run15（173 例）与 `verify-terminal-chain.sh --launch`（43 断言）的绿灯绑的是 schema 24，之后目录判据/搜索排序/分类三处行为都变了，旧结论对现状只算历史证据。**先做第 4 条再跑**。
6. **链路引擎 P0**：`docs/design/craft-chain-engine-plan-2026-10-03.md` §1 规格齐全（`apps/web3/src/chain/` 5 新文件 + 3 处挂载，≈350 行，后端零改动，不需要再拍板），代码零落地。
7. **图标渲染器第二批**（可选）：R4 OBJ 网格(+22)、R5 tint 近似(+15)、R6 化学桶(+18)，工单与收益预估在 `docs/design/item-icon-renderer-plan-2026-10-03.md` §3-§4。
8. **决定 `docs/tests/evidence/*.log` 要不要入库**（`issue-evidence-logs-untracked`）：`.gitignore:6` 的 `*.log` 让 chain-run15 / terminal-run 的原始日志留在本机，远端 clone 出来的报告引用了拿不到的证据。改名（`.txt`）入库或改成脚本自产可读报告，二选一。
9. **overrides 落盘**（唯一成体系的剩余功能缺口）：任务书/包内容/只有本机 jar 的模组目前不进 `.mrpack` 的 `overrides/`——`.mrpack` 规范要求 `files[]` 带 downloads URL，本地 jar 无 URL。先做设计再动代码。
10. **O5 删包 9.6s**：改入队异步+任务进度 vs 补 FK 覆盖索引，需用户拍板（chain `[29]` 已把耗时写进 note）。
11. **给内核补专属单测**：O16 后半（url-only 库）、O17（macOS runtime 布局）、O18（执行位）、O19（竞速 partial）现在只由终局实跑覆盖，没有 Rust 单测钉住，回归风险最高。
12. **B5 拆 service.API + Prism 移 platform**：B 系唯一暂缓项，随时可开。
13. 兼容知识库 `knownIssues` 等用户整理真实兼容数据后按 `docs/compat-knowledge.md` 格式填入。
14. 客户端采集上报（`idea-client-telemetry-upload`）仅登记，用户未拍板前不动。
15. 模板预制开局（`idea-2bc71628`）待讨论。

## 阻塞与已知问题

- ~~**前端残余假数据**:工作台右栏"包健康"(86/142/3 写死)、概览页模组列表 4 条写死(PackPages.tsx `const mods = [`)、设置页"已连接"/缓存演戏~~ **2026-09-30 已核实为过期**：包健康改读 `packHealth`/`listLocks`/`listConflicts`（`PackPages.tsx:180-207`），设置页平台状态与缓存/剩余空间读启动器 `status`（`:688-694`），`const mods = [` 全仓 grep 零命中。无真实数据源的块改为空态或移除。
- **P7 三缺口未复核**:取消语义/重启恢复测试/HTTP 契约测试(08f916a 之后)。
- **视觉验收**:第三轮已由用户确认完成；后续若提出新的视觉问题，另立新任务，不回写本任务历史状态。
- vite 993 kB chunk 警告为既有现象,未处理。
- **curl 矩阵网络项偶发假失败**:CF key/双平台合并等需真实网络的断言在平台超时时会红,重跑即绿;纯本地断言(53 项里绝大多数)是确定性的。
- **O5 删包 9.6s** 与 **overrides 未落盘**（任务书/内容/仅本机 jar 的模组）是当前仅有的两块功能性缺口，见「下一步」。
- ~~**`go test ./...` 当前不为全绿**~~ **已修**（`6c0aa5d`）：`TestPackModCategoryAcrossReads` 夹具自犯 CHECK（2026-10-03 16:22 复现，10 包 ok / 1 包 FAIL）→ `project_id` 给真值后 16:45 复跑 **10 包全绿**、vet 干净。
- **验证证据日志不在仓库里**：`docs/tests/evidence/*.log`（chain-run15 173 例、terminal-run 43 断言等）被 `.gitignore:6` 的 `*.log` 排除，测试报告引用的证据别人 `git clone` 拿不到，只能重跑脚本复现。要不要把关键 run 日志改名（如 `.txt`）入库，是个待拍板项（`issue-evidence-logs-untracked`）。
- **共享开发数据丢过一次性真实数据**：`/tmp/mpack-data` 里 readd-test 的任务书/目录/魔改记录已不可恢复（成因未定，无日志）；链路基座可由 `chain-test-run.sh` 再生。口径已入 `AGENTS.md` 铁律，任何会话不得再动该目录除非用户当轮明示。
- ~~**README 口径过期**（schema 26 vs 实际 27；端口 5273/18871 vs 铁律 5271/18872），会误导下一个会话。~~ **已修于 `65cf1e5`**：`:22` → schema 27（补 `0027`），`:58`/`:70-72` → 5271/18872 + 作废端口清单。
- **⚠ `scripts/chain-test-run.sh` 与唯一开发实例撞口（`issue-chain-test-port-collision`，high）**：脚本 `:8` 默认端口就是 **18872**，它的 `pkill` 只匹配自己的 `mpack-chain-server` 二进制（杀不掉 `dev.sh` 的 `go run`），`wait_up` 只看 `/api/health` 返回 200 —— 开发后端会答这个 200。不停开发实例就跑它，173 个用例（含建包/删包）会打在 `/tmp/mpack-data` 上，与 10-03 日常库被洗空同型。**当前处置**：跑前先 `scripts/dev-stop.sh`；根治见「下一步」第 4 条。（机制由代码阅读确认，本轮**未实跑验证**——实跑一次就可能污染唯一开发库。）

## 关键决策

- **流水线终局通向启动 Minecraft**(`decision-9a0c8004`)
- **双平台配对名称优先、添加即钉版、一行镜像字段、身份表双库分离**(`decision-dual-source-naming`,用户 2026-09-01 拍板)
- **兼容知识自动修复=自动但可见;知识起步只人工核实**(`decision-compat-knowledge-autofix`)
- **模组搜索:单框模糊名称、双平台并发、不加锁、错误隔离**(`decision-da2110dc`,用户钦定)
- **否决嵌入 Prism 代码**;CLI 调用不传染 GPL,保留兜底(`decision-f3ee0d92`)
- **引入 superpowers 方法论**(proposed,`decision-7859b14d`)
- 技术栈:前端 React19+TS+Vite7+antd6+zod4,后端 Go+SQLite(`decision-3af1a2b3`)
- SQLite 纪律:单库分域、jar_index 按 sha1 跨包共享、原始 JSON 不落库、pack_mods 唯一权威清单(`decision-4ba2b3c4`)
- 流程:每页 docs 提示词 → mock 先行 → 截图验收 → 接后端;AI 效果图内部文字不可信(`decision-5cb3c4d5`)
- ~~端口:前端 5273 / 后端 18871;旧项目端口一律不碰(`decision-6dc4d5e6`)~~ **2026-10-03 被 `decision-dev-service-singleton` 取代**
- **唯一开发服务（2026-10-03 用户定稿，`decision-dev-service-singleton`）**：后端 `127.0.0.1:18872 -data /tmp/mpack-data` + 前端 `5271`(apps/web3) 是唯一口径；5173/5273/5274/5275/5276/18871/18880 一律作废；`scripts/dev.sh`/`dev-stop.sh` 是唯一起停方式；动共享数据目录须用户当轮明示。权威文本 = 仓库根 `AGENTS.md`「服务与环境铁律」。
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
- **按 `succeeded` 轮询任务终态**:契约 `docs/api/contract.md:50` 枚举是 `queued/running/paused/success/failed/cancelled`,任务用 **`success`**;`succeeded` 只属于 catalog status 与 mod_content run 两套独立词汇。e2e 脚本曾因此空转 200s。
- **以为 `/mod-search` 条目带 versionId**:字段只有 `provider/id/slug/name/downloads`,**不含版本**;添加模组必须先查 `/mod-versions` 再按 mcVersion+loader 过滤,否则必 404 `provider_not_found`。
- ~~**按契约省略 `files` 调 `POST /packs/{id}/build`**:契约 `:910` 写「files 可选」,但 `build.go:405` 要求非空,省略必 400(基线 D1)~~ **2026-09-30 已反转为正确用法**：省略 `files[]` 才是主路径，服务端按包内权威清单自行装配真 `.mrpack`；调用方自带 `files[]` 反而绕过构建闸门（O20 只装在装配路径上）。
- **由"仓库里没有 `target/`"推出"本机编译不了 Rust"**：`target/` 被 gitignore、且 SMB 上不放构建产物，都不代表没有工具链。任何"本机做不到"的结论前先跑 `which cargo` / `rustup which cargo`（本轮 O11 教训，已写进记忆口径）。
- **在仓库根跑 `go test ./...` / `go vet ./...`**：Go module 在 `apps/server`，根目录跑必报 `pattern ./...: directory prefix . does not contain main module`（2026-10-03 实测踩到）。固定 `cd apps/server && go test ./...`。
- **不经用户同意对 `/tmp/mpack-data` 重启/迁移/重建，或在双会话并行时把共享环境当私有环境**：2026-10-03 日常库就这么洗空了（真实数据丢失、成因未定）。另注意 `chain-test-run.sh` 每次运行**按设计**重置 `/tmp/mpack-chain`，那是正常行为，别当事故排查。
- **不停开发实例直接跑 `scripts/chain-test-run.sh`**：它默认端口 = 唯一开发后端的 18872，自己的后端 bind 不上而健康探活被开发后端答 200，于是 173 个用例（含建包/删包）打在 `/tmp/mpack-data` 上。先 `scripts/dev-stop.sh`，或 `CHAIN_PORT=` 换口（根治见「下一步」4）。

## 首先读取

- `AGENTS.md`（仓库根）—— **服务与环境铁律优先级最高**：只用 18872/5271 与 `/tmp/mpack-data`，禁历史端口、禁新开实例、动数据目录须当轮请示
- `docs/project-state/history/2026-10-03-session-summary.md` —— 本轮（Z Code 会话）详录：进度、验证快照、遗留缺陷、下一步
- `docs/tests/incident-2026-10-03-shared-env-wipe.md` —— 共享库清空事故的事实、恢复尝试与判定
- `docs/design/craft-chain-engine-plan-2026-10-03.md` + `docs/design/item-icon-renderer-plan-2026-10-03.md` —— 两份未消化完的工单（链路引擎 P0 零代码；图标 R4/R5/R6 未做）
- `docs/tests/defects-2026-09-30.md` —— **缺陷台账与验证边界**（D*/L*/O* 逐条带可复现出处；§4 说清两套验证各证明了什么）
- `docs/tests/chain-test-2026-09-30.md` —— 前端→后端调用链路 173 例（run15 PASS 172/FAIL 0/SKIP 1）与逐轮红绿叙事
- `docs/tests/evidence/terminal-run-2026-09-30.log` —— 真内核终局链路 43 条断言全绿，最后一条是游戏真起窗
- `docs/tests/e2e-baseline-2026-09-29.md` —— 上一轮能力基线（终局两处断点已被 09-30 闭合，其「本机从未编译」一句已就地更正）
- `docs/project-state/state.json` —— 机器可读状态(任务/问题/想法/决策全量；09-30 已补 O14-O25 与终局任务)
- `docs/api/contract.md` —— API 契约（启动器域已补齐；冲突 kind 枚举含 `provider_unavailable`）
- `docs/compat-knowledge.md` —— 兼容知识库整理指南(knownIssues 待用户数据)
- `scripts/contract/README.md` —— 四层验收体系用法

## 验证状态

- 2026-10-03（checkpoint 会话复跑，绑定已提交状态 HEAD `65cf1e5`）：`cd apps/server && go test ./... -count=1` = **10 包 ok / 0 FAIL**（红夹具已修）、`go vet ./...` 干净；`git ls-remote` 实测 `origin/DEV_2610-WK1` = `65cf1e5`（分支首推成功，upstream 已建立）；`lsof` 实测唯一服务在跑（18872 后端 + 5271 前端）。**未重跑**：`chain-test-run.sh`、`verify-terminal-chain.sh`、`npm run build`（web3 构建在提交前跑过，绑定指纹 `sha256:a795433e…b7ef53`）。
- 2026-10-03（本轮 checkpoint 实测，绑定工作区指纹 `sha256:a795433e…b7ef53`、HEAD `fe7c4ea`）：`cd apps/server && go test ./... -count=1` = **10 包 ok / 1 包 FAIL**（`TestPackModCategoryAcrossReads` 夹具自犯 CHECK）；`go vet ./...` 干净；`npm --prefix apps/web3 run build` 通过（883 kB chunk 警告为既有现象）；`ps` 实测唯一服务在跑（18872 + vite 5271）。会话内另有无日志留存的实测：catalog 重建 1754→1330/无名 0/evidence 全 lang、模组搜索三案例首命中、分类 PATCH 回读与浏览器分组验收。
- 2026-09-30（上一轮）：`go test ./... -count=1` 12 包全绿；`scripts/chain-test-run.sh` run15 173 例 PASS 172/FAIL 0/SKIP 1；`scripts/verify-terminal-chain.sh --launch` 真内核 43 断言全绿并拉起 Minecraft 1.21.1/Fabric 0.16.14；schema 24（**现已被 0025-0027 推进到 27，chain/terminal 两套脚本自那以后未重跑**）
- 已运行并通过(2026-09-01,HEAD 3df1ce4):`scripts/verify-contract.sh` 四层全绿——go build/vet/test 9 包 + gofmt 基线制;curl 矩阵 53/53;E2E 22/22×2(直连 18899 + vite 代理 5274);前端 tsc 零错误
- 实测:JEI 双平台合并卡;添加自动钉镜像版本(CF fileId 8769490 = 同版本号 jei-1.21.1-neoforge-19.51.0.417.jar);推荐接口按 loader 正确过滤;CF key 无效 400/真 key 保存清除恢复
- 注意:含真实网络的断言在平台抖动时偶发假失败,重跑即绿
- 用户验收:本阶段五项功能均经用户拍板/批准后实现,实测证据逐条展示
