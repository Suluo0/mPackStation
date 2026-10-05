# Windows 快速迭代指南

> 目标：在一台 Windows 机器上跑起 mPackStation，并高频迭代四个产品能力
> （启动器 / 优化 / 配方编辑 / 查阅）。
> 环境铁律见仓库根 `AGENTS.md`；构建入口见 `build-and-deploy.md`。
>
> 状态：脚本已按现行口径对齐（2026-10-05），但**尚未在 Windows 上实跑验证**——
> 见文末「未验证事项」。

## 0. 先看这条：四个能力的依赖是不均匀的

| 能力 | 需要什么 | 平台包袱 |
| --- | --- | --- |
| 查阅（目录/图标/关系透镜） | Go + 一个包里有 jar | **无**。纯 Go `archive/zip` 解析 |
| 配方编辑（自建 content 文档） | Go + SQLite | **无**。不依赖 jar、不依赖解析 |
| 优化（依赖锁定 / 冲突 / 兼容推荐） | Go + 能出网（Modrinth / CurseForge） | **无**。纯 HTTP + 规则 |
| 启动器（安装整合包 / 启动游戏） | Rust 内核 exe + JDK 21 + 一个 Minecraft 目录 | **有**。三样都得在 Windows 本机 |

也就是说：**只有启动器这一个能力需要额外的运行时**。其余三个把 Go 跑起来就能调，
不需要 Java、不需要 Python、不需要 Minecraft 实例。迭代策略应该据此分层。

## 1. 工具链

| 组件 | 用途 | 版本参考 | 只有谁需要 |
| --- | --- | --- | --- |
| Go | 后端 | `go.mod` = 1.27 | 全部 |
| Node.js + npm | 前端（web3） | Vite 7 要求 Node ≥ 20.19 或 ≥ 22.12 | 全部（不开前端界面可跳过） |
| Rust + cargo | 启动器内核 `mpack-launcher` | edition 2021 | 启动器 |
| JDK 21 | 启动 MC 1.21.1 | 内核按 MC 版本推算要求 | 启动器 |

Go 侧无 cgo（`modernc.org/sqlite`），无需 C 工具链。仓库内若存在
`.tools/go/bin/go.exe` 会优先使用，无需额外配置 PATH。

内核与 JDK 都可以后补：缺失时启动器任务会在**提交阶段**同步返回 503，不会留下
半死的任务，其余能力完全不受影响。

## 2. 代码放哪：两条路线

| | A. 直接在工作副本上跑 | B. 本地盘再 clone 一份 |
| --- | --- | --- |
| 做法 | 在外置/网络卷上的那份代码里直接开发 | 在本地 SSD 上 clone，独立分支 |
| 优点 | 零同步成本，只有一份代码 | 构建快；不碰原工作副本 |
| 缺点 | **构建慢**，尤其 cargo 增量编译与外置卷 I/O | 需要同步代码（push/pull 或分支） |
| 适合 | 改前端/Go 后端为主 | 要反复重编 Rust 内核 |

**推荐 B**，并把 cargo target 指到本地盘：

```powershell
pwsh scripts/build-launcher.ps1 -TargetDir D:\cargo-target\mpack
```

如果是走 A，至少把 `CARGO_TARGET_DIR` 挂到本地 SSD——否则每次改一行 Rust 都要等
一次全量编译。

## 3. 首次跑通

```powershell
# 1) 依赖
cd apps/web3 ; npm ci ; cd ..\..

# 2) 起后端 + 前端（后端 18872 / 前端 5271）
pwsh scripts/dev.ps1

# 3) 浏览器打开 http://127.0.0.1:5271/
```

首次进入是空库，迎新流程会引导建包或导入整合包。

要调试**启动器能力**时再多两步：

```powershell
# 4) 编译内核（首次约几分钟）
pwsh scripts/build-launcher.ps1 -TargetDir D:\cargo-target\mpack

# 5) 重启 dev（脚本会把内核注入 MPACK_LAUNCHER_BIN）
pwsh scripts/dev-stop.ps1
pwsh scripts/dev.ps1
```

JDK 21 的探测顺序（内核自带）：`JAVA_HOME` → `PATH` → **Windows 注册表** →
常见厂商安装目录 → 启动器 runtime 目录 → 深度扫描。所以「装好 JDK 21 并让它进
PATH 或 JAVA_HOME」就够了，也可以在启动命令里显式 `--java` 指定。

## 4. 四个能力的迭代路径

### 4.1 启动器能力

| 项 | 位置 |
| --- | --- |
| 前端 | `apps/web3/src/panels/RunPanel.tsx`（游戏目录、安装记录、启动配置） |
| 路由 | `POST /api/launcher/install`、`POST /api/launcher/launch`、`GET /api/launcher/installs` |
| 服务 | `apps/server/internal/service/launcher_task.go`、`.../launcher/launcher.go` |
| 内核 | `launcherCore/src/`（`install.rs` 编排、`java/detect.rs` 探测、`launch/` 组命令） |
| 落库 | `launcher_installs` |

迭代闭环：改 Rust → `build-launcher.ps1` → 停/起 dev → 在 RunPanel 里重跑安装。
内核与 Go 之间是 JSON Lines 协议（`launcher.go` 的 `run()` / `scanStdout`），
排查问题直接看内核 stdout 事件最有效。

### 4.2 查阅能力（最适合当第一个调的）

| 项 | 位置 |
| --- | --- |
| 前端 | `editor/ModeIndex.tsx`（索引/搜索）、`ModeGraph.tsx` + `recipeView.ts`（JEI 式关系） |
| 解析入口 | `POST /api/packs/{packId}/mods/{modId}/content/parse` → 任务 `parse_mod_content` |
| 目录重建 | `POST /api/packs/{packId}/catalog/rebuild` → 任务 `catalog_init` |
| 读取 | `GET /api/packs/{packId}/catalog`、`/catalog/items/{id}`、`/catalog/tags/{tagId}` |
| 落库 | `pack_catalog_items / blocks / tags / recipes`、`mod_content` |

链路是**两段异步任务**：先解析单个 jar 落 `mod_content`，解析完会自动重投目录
重建，最后 `pack_catalog_*` 才有数据。看进度去任务面板，不要只看前端有没有刷新。

不依赖任何外部运行时，所以这是验证「Go 后端在 Windows 上到底跑不跑得动」的
最短路径：导入一个包 → 解析一个模组 → 看索引页出不出物品。

### 4.3 配方编辑能力

| 项 | 位置 |
| --- | --- |
| 前端 | `editor/ModeEdit.tsx`（魔改态）、`panels/ContentPanel.tsx`（自建 recipe/structure/ore） |
| 路由 | `POST /api/packs/{packId}/content`、`PUT .../draft`、`POST .../validate|apply|rollback`、`GET .../history` |
| 服务 | `apps/server/internal/service/content.go`（kind 白名单 + 字段校验） |
| 落库 | `content_documents` / `content_revisions` |

要点：**自建配方与解析出来的配方是两套数据源**。编辑写的是 content 文档；
查阅/关系透镜读的是 jar 解析出的 `pack_catalog_recipes`。只有「应用 + 重建目录」
之后两边才对齐。调试编辑态时不要拿目录里的配方当真。

这一块零外部依赖，适合做后端逻辑的快速回归（改完 `go test ./internal/service/`）。

### 4.4 优化能力

| 项 | 位置 |
| --- | --- |
| 前端 | `dock/ProblemsPanel.tsx`（处置 / 忽略 / 一键装前置） |
| 路由 | `GET .../conflicts`、`POST .../conflicts/{id}/resolve`、`.../ignore`、`GET .../health`、`.../mod-recommendations` |
| 服务 | `apps/server/internal/service/mods.go`（`ResolvePack` / `ListConflicts` / `PackHealth`）、`compat_knowledge.go`（`go:embed` 只读知识库） |
| 落库 | `conflicts`、`pack_locks`、`mod_dependencies` |

依赖**出网**才能拉到平台元数据。Windows 上代理是这里最常见的失败原因：后端进程
必须能访问 `api.modrinth.com` / `api.curseforge.com`（CurseForge 还要 API Key）。
`dev.ps1` 会先探测代理存活再透传，死代理会被剥离——如果你看到「搜索永远 0 结果」，
先查这里。

error 级未解决冲突会拦构建（`build_conflict_gate_test.go`），调试处置流程时注意这点。

## 5. 迭代节奏：改什么，怎么生效

| 改动 | 生效方式 | 等待 |
| --- | --- | --- |
| web3 的 tsx/ts/css | Vite HMR，保存即生效 | 秒级 |
| Go 后端 | **没有热重载**：`dev-stop.ps1` → `dev.ps1` | 数秒 |
| Rust 内核 | 重编 + 重启 dev | 首次几分钟，增量较快 |
| `catalog`/`content` 数据 | 走应用内重建或重新导入 | 视包大小 |

`go run` 起的是编译后的临时进程，**改完 `.go` 必须重启**，否则你会以为改了没生效。

## 6. 数据重置

测试数据全部可再生，随时可以回到干净起点：

```powershell
pwsh scripts/dev-reset.ps1                 # 归档并清空数据库 + 项目目录
pwsh scripts/dev-reset.ps1 -KeepData       # 只清项目目录（想留包记录时）
pwsh scripts/dev-reset.ps1 -KeepProjects   # 只重置数据库
```

它是**归档而不是直接删**：数据库相关文件进 `<数据目录>\backup-<时间戳>\`，项目
实例目录进 `%TEMP%\mpack-projects-<时间戳>\`。确认无误后手工删掉归档即可释放空间。

为什么要脚本化：一个 132 模组的包落盘约 1.2GB，而导入在「同包复用」接上之前每次
都会新建包 + 新目录。手工清理很容易只删一半（目录删了、库里还留着包记录），下次
导入就分不清哪条是脏数据。

## 7. 与 macOS 的差异 / 已知坑

| 坑 | 说明 | 处理 |
| --- | --- | --- |
| `.bat` 换行符 | LF-only 的 `.bat` 里 `:label` / `goto` 行为不可靠 | 已加 `.gitattributes`（`*.bat`/`*.cmd` → CRLF），检出自带 |
| 迁移 SQL 换行符 | 迁移以字节级 sha256 校验，CRLF 会被判为篡改导致库起不来 | 已有 `*.sql eol=lf` |
| 内核查找路径 | 后端口径是 `<数据目录的父目录>/.tools/launcher/`，数据目录在 `%TEMP%` 时会指向 `%TEMP%\.tools` | `dev.ps1` 显式注入 `MPACK_LAUNCHER_BIN` |
| 文件权限位 | 代码里大量 `0o755/0o600`，Windows 上是 no-op | 无害；锁与存储已有平台分支 |
| 代理 | 后端不出网会表现为「平台不可达、搜索 0 结果」 | `dev.ps1` 探测存活后透传 |
| Prism 工具安装 | 那条路强依赖 `cmd.exe` + `.bat`，仅 Windows 可用 | 与内核链路互不影响 |
| cmd/bash 混合 | `verify-contract.bat` 会调 git-bash 跑 `.sh` | 需要 git-bash 在 PATH（或用 PowerShell 入口） |

## 8. 未验证事项

以下是本次改动**没有实测**的部分，第一次在 Windows 上跑时请重点确认：

1. `scripts/dev.ps1` / `dev-reset.ps1` / `build-launcher.ps1` 只做过静态检查
   （括号平衡、路径拼接、变量名），**没有在 Windows 上实跑**。
2. `launcherCore` 未在 Windows 上 `cargo build` 过。代码里有 `cfg(windows)`
   分支（`winreg` 注册表探测、`CREATE_NEW_PROCESS_GROUP` detach、`tasklist` 存活
   检查、`%APPDATA%` 数据目录），但未实编译验证。
3. `build.ps1` / `package.ps1` 不产出内核 exe，分发包里也没有它——如果要在
   Windows 上做完整的分发验证，需要先补这一步。
4. `build-and-deploy.md` 里描述的 Windows 迭代尚未形成回归证据。
