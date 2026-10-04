# 前端 → 后端调用链路测试报告（2026-09-30）

用户口径（原文要求）：只做前端请求，以后端正确响应并处理为通过标准，以数据库入库为准；每个功能至少一条正向 + 一条反向用例；按此流程把所有功能测一遍。

## 1. 结论

| 项 | 结果 | 出处 |
|---|---|---|
| 链路用例总数 | **173**（29 个功能组，每组≥1 正 ≥1 反） | `docs/active/tests/evidence/chain-run15-2026-09-30.log` |
| 终局结果 | **PASS 172 / FAIL 0 / SKIP 1** | 同上，汇总行 |
| 唯一 SKIP | `[28-边界安全] R1b 超大 body 经前端代理` —— 观察项（实得 413），不参与判定 | `scripts/chain-test.py:769` |
| Go 单测/构建 | `go build ./...` OK，`go vet` OK，`go test ./...` 全绿（含新增 launcher 包 8 例） | 本次会话执行 |
| 前端 | `npx tsc --noEmit` 零错误；`npx vite build` 通过（1 212 kB chunk 警告为既有现象） | 本次会话执行 |

同一套件在本轮跑了 15 次（run1…run15）。run1→run6 逐步暴露缺陷，run7 起转绿，run8/9/10 是启动器 argv 契约修复后的复验；run11/run12/run13 是修 O1/O2/O4/O8/O9/O10/O13 之后的复验（run12 剩 1 条失败是我自己的用例目标写错——把"改构建输入"打在会被装配先挡住的包上，run13 修正后全绿）。**run14 是把 O15-O24 修完、服务端接入真产物安装链路之后的复验**，它红了 8 条，全部指向同一处新引入的相互作用：构建闸门（O20）排在装配之前，而 `conflict()` 又把"平台不可用"悄悄写成 error 级依赖冲突（新登记为 **O25**），于是链路测试里那些"有本机 jar 模组/缺平台依赖"的包一律构建不出去，下游 `[22]` 的反向用例跟着 cascading 红。run15 修完 O25 并按新语义调整守护口径后全绿（173 例）。归档 `chain-run8`（修复后首轮）、`chain-run10`（argv 契约）、`chain-run12`（暴露装配 INNER JOIN 静默丢件与裸 409 的那一轮）、`chain-run13`（O13 为止的最终）、`chain-run15`（含 O15-O25 的最终）。

用例数 153 → 166 → 173 的增量全部是"修复的守护断言"：装配真 `.mrpack`（P5/P5a/P5b/P5c/P5d/P6/R1/R7 共 8 条）、启动器安装记录与按包启动（P2/P3/P4/R5 共 4 条）、目录错误码分流（R4/R5/R6 共 3 条）、任务书空态（R0 收紧 1 条）、真产物安装（P6/R6/R7/R8/R9 共 5 条）、网络态冲突分级（P3b 1 条）、导出目录换名重登记（1 条），减去被改写覆盖的旧观察项。

## 2. 方法

请求侧只走前端会走的那条路：`vite` dev server（`:5273`，代理 `/api` → 后端，`changeOrigin:false` 以保留 Host 守卫的真实行为），请求体形状取自 `apps/web/src/api/*.ts` 的真实调用，不手写「更适合后端」的 payload。判定三段：

1. HTTP 状态码与错误码（`error.code`）符合预期；
2. 写请求落库——直接查 `/tmp/mpack-chain/mpackstation.db`（`sqlite3` 断言行数、状态、计数口径）；
3. 异步任务类额外等到终态（契约 `docs/active/api/contract.md:50` 枚举是 `success`，不是 `succeeded`），终态不对就按失败计。

隔离环境（绝不碰用户开发库 `/tmp/mpack-data`，绝不碰 5173/18765/18766）：

| 实例 | 地址 | 数据目录 | 用途 |
|---|---|---|---|
| A | `127.0.0.1:18872` | `/tmp/mpack-chain` | 主链路，带启动器协议桩 |
| B | `127.0.0.1:18873` | `/tmp/mpack-chain-nobin` | 故意不给启动器二进制，验同步 503 |
| 前端 | `127.0.0.1:5273` | — | 真实代理路径 |

重放：`bash scripts/chain-test-run.sh /tmp/chain-runN.log`（自建桩、起 A/B 两实例、跑 `scripts/chain-test.py`、只回收 B 实例）。

## 3. 功能组覆盖

`1-建包 / 2-查询 / 3-改包 / 4-归档 / 5-工作台 / 6-引导 / 7-文件系统 / 8-模组搜索 / 9-模组版本 / 10-添加模组 / 11-本地模组 / 12-模组清单 / 13-依赖解析 / 14-内容目录 / 15-模组解析 / 16-内容文档 / 17-任务书 / 18-版本 / 19-构建产物 / 20-交付检查 / 21-发布 / 22-启动器 / 23-任务域 / 24-系统 / 25-凭证 / 26-外部工具 / 27-导入 / 28-边界安全 / 29-删除`，每组用例数与正/反配比见日志汇总段（最大 22-启动器 15 例，其次 19-构建产物 14 例；最小 9-模组版本 2 例 = 1 正 1 反）。

## 4. 链路测试挖出并修掉的缺陷

每条都有对应断言，删除修复会立刻变红。

| # | 缺陷 | 症状 | 修复 | 锁住它的用例 |
|---|---|---|---|---|
| L1 | `Conflict` 结构体 `detailPath`/`resolvedAt` 带 `omitempty`，时间字段没打 camelCase tag | 前端 zod 收到 `undefined` 判「接口数据结构不符合约定」，**依赖与冲突页整页打不开** | `apps/server/internal/service/mods.go:91-100` 显式 tag + 注释说明不能省 | `[13-依赖解析] P3 conflicts 列表`（逐 key 存在性断言） |
| L2 | 内建 `minecraft` 成员行被算进模组数 | 健康度/工作台/引导三处口径不一致（截图 3 vs 2），且「添加第一个模组」在新包上自动完成 | `mod_repo.go:51,347,351`、`pack_repo.go:229-231,450-452` 统一 `origin<>'builtin'`（与 `ListPackMods` 对齐） | `[13] P4 包健康`（SQL 同口径复核） |
| L3 | 删除整合包必 500 | `minecraft_pack_mod_delete_guard` 无条件拦截级联删；catalog 五张子表留孤儿行，COMMIT 外键校验再报错 | 迁移 `0021_pack_delete_cascade.sql`（守卫加「父包仍存在」条件 + 补 pack_id 索引）+ `pack_repo.go:295-336` 显式清理 | `[29-删除] P2 删除包` 204 + 行数复核 |
| L4 | `GET /api/fs/browse` 读请求免鉴权 | 进程绑 `0.0.0.0` 时同网段任意主机可枚举服务器任意目录（实测不带令牌列出 `/private/etc`） | `httpapi.go` `tokenRequiredForRead()` 让目录浏览需令牌；前端 `api/fs.ts` 改 `tokenHeaders()` | `[7-文件系统]` 无令牌反向用例 |
| L5 | 删除包的拒绝原因被压成 `resource conflict` | 前端只能显示「冲突」 | 新增 `ErrPackHasActiveTasks`/`ErrBuiltinMemberProtected` 包装 `ErrConflict`，HTTP 层出专码 `pack_has_active_tasks` | `[29-删除] R2` 断专码 |
| L6 | 启动器接口零契约（基线 D6） | 前后端字段名/终态全靠猜 | `docs/active/api/contract.md` 新增 §8（请求/响应状态码/argv/JSON Lines 协议） | 文档 |
| L7 | **install argv 与 Rust CLI 契约不符（新，P0）** | Go 发 `install --version 1.21.1`，`launcherCore/src/cli.rs:48-72` 的 `InstallArgs` 必填项是 `--mc` 且**没有** `--version` → 真实二进制 clap 退出码 2，安装永远失败；链路测试此前全绿是因为协议桩不校验旗标 | `launcher.go:66-89` 改 `--mc`，并支持 `--loader-version`/`--java`；载荷与前端补 `loader_version`/`java_path`（`launcher_task.go:15-21`、`api/launcher.ts`、`LauncherPage.tsx:88-95`）；桩改成 clap 语义（`scripts/chain-test-run.sh`） | `TestInstall_ForwardsMcFlag`（断 argv）+ `[22-启动器] P1`（桩会像 clap 一样退出 2） |
| L8 | 取消启动任务时读循环不可中断，且把 `fork/exec` 原文当错误抛给调用方（基线 D4/D5 同域） | 任务卡 `running`，错误信息不可读 | `launcher.go:92-152` 扫描放进协程 + `select` on `ctx.Done()`，终态返回 `mpack-launcher 已取消: <ctx.Err>`；`WaitDelay` 作为子进程持有管道时的兜底 | `TestRun_CanceledContext` |

L7 的红绿验证方式值得记下：桩改为 clap 语义后，直接对手工调用旧 argv 立刻 `exit=2`（`bad_args: missing required argument --mc`），而新 argv 正常回 result。这一步是必要的——**协议桩只校验协议时，会把跨语言 CLI 契约的错误完全掩盖掉**。

## 4b. 本轮之后新登记的缺陷

见 `docs/active/tests/defects-2026-09-30.md`：`O1/O1b`（构建装配与"不许静默丢模组"）、`O2`（启动版本 ID 与安装结果挂钩）、
`O4`（任务书空态专码）、`O8`（冲突摘要点名）、`O9`（目录读取三码分流）、`O10`（登记版本入口）、
`O13`（构建输入冲突专码）为已修；`O5`（删包同步阻塞约 10 s）、`O12`（复杂模型图标）为未修，均带实测出处。

启动器域接真内核之后又挖出一批，全部已修（缺陷文档 §1 有逐条出处）：
`O14`（内核读 `.mrpack`）、`O15`（导出目录换名重登记 500）、`O16`（加载器 url-only 库漏装）、
`O17`（macOS runtime 布局探测）、`O18`（下载 runtime 缺执行位）、`O19`（竞速下载残留 partial）、
`O20`（致命冲突未拦构建）、`O21`（重新 resolve 后过期冲突不结案）、`O22`（原样回传锁快照仍被判不一致）、
`O23`（`kind='known_issue'` 撞 CHECK）、`O24`（依赖类型不分）、`O25`（平台不可用被写成 error 级冲突，
run14 的 8 条红全指向它）。

## 5. 已知不足与限制（不要当成已通过）

1. ~~**启动器「成功」只证明到协议桩**~~ **误判，已纠正**：本机 cargo 一直可用（brew rustup，工具链在
   `~/.rustup/toolchains/stable-aarch64-apple-darwin/bin`，只是默认不在 PATH），`launcherCore` 已真编译
   真运行。本轮 `scripts/verify-terminal-chain.sh --launch` 用 cargo 出的二进制一路走到
   **Minecraft 1.21.1 / Fabric 0.16.14 起窗**（43 条断言全绿，`docs/active/tests/evidence/terminal-run-2026-09-30.log`）。
   本报告这 173 例**仍然是桩驱动的**——它证明的是「前端请求 → 202 → 任务入库 → worker → fork/exec →
   终态收敛」这条契约链，快、每轮可重跑；真下载真起窗那份归终局验证，两套别混着读（缺陷文档 §4）。
2. ~~构建域未闭环（基线 D1）~~ **已闭环**：省略 `files[]` 时服务端按包内权威清单装配真 `.mrpack`（`[19] P6` 开 zip 校验 manifest 与 `files[].downloads/hashes`）。**再下一环也已闭环（缺陷 O14）**：`[22] P6` 只给 `artifact_id` 就能把这份 `.mrpack` 交给安装链路，真内核侧 `launcherCore/src/mrpack.rs` 读得懂它，终局验证里装完 sha1 全对、游戏起窗。剩余边界见缺陷文档 §4「验证边界」。
3. ~~launch 版本 ID 与安装结果脱节~~ **已闭环（缺陷 O2）**：install 终态把内核回的真实 `version_id` 写进 `launcher_installs`（迁移 0022），`GET /api/launcher/installs` 可查，launch 省略 `version` 时服务端入队前解析，未安装则 409 `launcher_not_installed`。桩已按 `install.rs:74-77` 回 `version_id`，所以 `[22] P2` 能断到 `fabric-loader-` 前缀。
4. 边界组只覆盖 8 条（超大 body、伪造 Host、跨站 Origin、未知路由、SQL 注入式 id、路径穿越 itemId）；并发压测、断连恢复、崩溃后任务重领不在本轮范围。
5. 反向用例接受「一组合理码」（如 `[400,404,409,422]`）而不是单一码——这是刻意的：语义本身还待拍板（见缺陷 O9），断言把「不得退化为 500」作为硬底线。
6. 链路测试用的写令牌 `MPACK_TOKEN=chain-token-20260930` 只在隔离环境注入，未写入任何提交文件；用户开发库 `/tmp/mpack-data` 与 5173/18765/18766/18871 全程未被触碰。
7. 终局验证（真内核）另有第四套隔离环境：后端 18874、数据 `/tmp/mpack-terminal`、`MPACK_LAUNCHER_BIN` 指向 cargo 编译产物，`CARGO_TARGET_DIR=/tmp/mpack-launcher-target`（本地盘，不放 SMB 挂载上）。重放：`bash scripts/verify-terminal-chain.sh --launch`。
