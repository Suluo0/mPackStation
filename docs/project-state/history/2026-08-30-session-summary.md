# 2026-08-30 会话存档

## 范围

本会话横跨:模组页真实链路修复与并行化、Prism 便携工具链、上手四步、流水线终局重定义、启动内核可行性调研、方法论 skill 选型、项目存档。

## 完成与证据

1. **模组页双平台真实链路**(`task-690ee8ef`,verified,committed)
   - 后端:main.go 装配 provider registry;`NewRouterWithProviders`;`GET /api/packs/{id}/mod-versions`;`ModSearchAll` WaitGroup 并发扇出 + 错误隔离;provider.go 补 json tag、Modrinth hashes 对象兼容、Registry.List。
   - CF 适配器三 bug(全部为"测试 fixture 写的想象形状"类):搜索方言缺 `gameId=432` → 按 CF 词汇表重写 + `cfLoaderType` 映射;logo 对象;gameVersions 混排拆分 loaders/gameVersions。
   - 前端:单框模糊名称搜索;清除假包 tech(main.tsx DefaultPackRedirect、AppShell 侧栏动态化、PackContext 真实数据)。
   - 证据:curl 实测 sodium/JEI;CF/MR 同版本 sha1 `1757289b…` 一致;go test/vet/gofmt 绿;tsc/build 绿。提交 `76fb818`、`05aefa7`、`6764228`、`a827e9b`(均未推送)。

2. **Prism 便携安装脚本**(`task-d0e1ded6`,verified,working_tree)
   - `scripts/prism-install.{sh,bat}`:代理前置探测 → 版本解析(API 直连→代理→钉住 11.0.3)→ 下载(代理→GitCode→gh-proxy→直连)→ bsdtar 解压到 `.tools/prism`。
   - GitCode 为唯一实测可靠的国内镜像(与官方 release 同路径,20,392,295 字节等大,16.3MB/s)。
   - 坑:原生 curl 不认 MSYS 路径;GNU tar 不解 zip;bat 中文丢引号(终版纯 ASCII+CRLF);`%CD%` 块内不更新;tasklist 参数被 MSYS 吃掉。

3. **上手四步 + tool_install 任务化**(`task-1bd4148d`,verified,working_tree)
   - 第 4 步 = 只登录:`POST /api/tools/prism/login` 唤起便携 Prism(`-d .tools/prism-data`),后端检测 `accounts.json` 自动打勾。
   - 安装 = 一等任务:`tool_install` kind;handler 流式把安装脚本输出写任务日志(1s 节流 + 10s 心跳);成败 = 任务终态 + exe 存在。
   - 迁移 `0003_task_kind_tool_install.sql` 重建 tasks 表扩展 CHECK;`CurrentSchemaVersion = 3`。
   - e2e:任务日志 11 条干净闭环(queued→…→succeeded);重复提交 `already_installed`;GUI 唤起便携目录生成。

4. **路线决策**(见 state.json decisions):终局 = 启动 Minecraft;搜索并行不加锁;否决嵌 Prism;superpowers 选型 proposed。

## 调研产出(会话级,不落项目代码)

- Prism 体量实测:launcher/ 795 文件 116,455 行 C++20,UI 占 38%;GPL-3.0-only;API key 不可冒用。
- mrpack-install:MIT、Go module、可 import;支持全 loader 服务端部署。
- minecraft-launcher-lib(Python,BSD-2)≈5k 行实现启动器能力全集 —— 自研体量参照系。
- superpowers 安装被 Hermes 安全扫描拦截(dangerous,227 条全为 docs/tests 误报);绕过方式 = 临时关 `plugins.scan_on_install`,**待用户批准**。
- superpowers 官方提示:Hermes 无 post-compaction hook,长会话压缩后引导失效 —— 对策 = 每阶段开新会话 + project-checkpoint。

## 遗留

- 未提交 12 文件 + 未推送 4 提交(见 HANDOFF 下一步 1)。
- 前端残余假数据:包健康右栏、概览页模组列表、设置页演戏。
- P7 三缺口(08f916a 后未复核);Codex 遗留 `docs/api/contract.md` 未跟踪。
- 模板预制开局、mrpack-install 冒烟、启动内核 Phase 0 —— 均待拍板。
- 会话级:Hermes 网关未重启(豆包搜索后端未生效,web_search 当前不可用);插件安全门禁调研因本次拦截获得实证。

## 下一会话第一步

读 `docs/project-state/HANDOFF.md` → 落地两个已提议 commit → 问用户 superpowers 安装批准 → 如拍板启动内核则开 superpowers brainstorming。

## 本次检查点 · 看板视觉问题已完成

- 用户明确要求将看板视觉问题标记为已完成。
- `task-f6b5c7d8` 已从 `implemented/acceptance=pending` 更新为 `delivered/acceptance=accepted`。
- 验证证据保留：前端构建通过，桌面与 390×844 移动视口无横向溢出。
- 本次未修改业务代码；仅更新项目状态、交接和历史记录。
