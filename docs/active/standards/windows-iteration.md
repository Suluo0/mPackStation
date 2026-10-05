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
| **PowerShell 7（`pwsh`）** | **全部脚本入口** | 5.1 不行，必须 7 | **全部** |
| Go | 后端 | `go.mod` = 1.27 | 全部 |
| Node.js + npm | 前端（web3） | Vite 7 要求 Node ≥ 20.19 或 ≥ 22.12 | 全部（不开前端界面可跳过） |
| Rust + cargo | 启动器内核 `mpack-launcher` | edition 2021 | 启动器 |
| MSVC C++ 工具链 | Rust 的 `*-pc-windows-msvc` 目标需要链接器 | VS 2019+，需含 `VC.Tools.x86.x64` | 启动器 |
| JDK 21 | 启动 MC 1.21.1 | 内核按 MC 版本推算要求 | 启动器 |

**`pwsh` 是硬前置**：`build.sh` / `package.sh` / `test.sh` / `verify.sh` 四个入口的
第一件事就是 `exec pwsh -NoProfile -File ...`，缺了它直接报错退出。Windows 自带的是
Windows PowerShell 5.1（`powershell.exe`），**不叫 pwsh、也不满足要求**：

```powershell
winget install --id Microsoft.PowerShell --source winget
```

Go 侧无 cgo（`modernc.org/sqlite`），无需 C 工具链。仓库内若存在
`.tools/go/bin/go.exe` 会优先使用，无需额外配置 PATH。

MSVC 是否够用可以直接问 vswhere（有输出即带 C++ 组件）：

```powershell
& "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe" `
  -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property displayName
```

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
# 0) 前置：PowerShell 7（见第 1 节，只有 5.1 会直接跑不动脚本）
winget install --id Microsoft.PowerShell --source winget

# 1) 依赖（dev.ps1 不会替你装 npm 依赖，必须先来这一步）
cd apps/web3 ; npm ci ; cd ..\..

# 2) 起后端 + 前端（后端 18872 / 前端 5271）
pwsh scripts/dev.ps1

# 3) 浏览器打开 http://127.0.0.1:5271/
```

> `npm ci` 结尾可能报 `esbuild postinstall 被 allowScripts 拦下`。这是 npm 12 的脚本
> 白名单机制，**可以忽略**：esbuild 的平台二进制走 optionalDependencies 分发，
> `node_modules\@esbuild\win32-x64\esbuild.exe` 已经在位，vite 能正常起。

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

## 8. 实测记录（2026-10-06，两轮真机执行）

机器：Windows 11（build 26200）/ Go 1.27.0 / Node 24.19.0 / rustc 1.98.0 / VS2019 Community。

**已在真机跑通：**

| 入口 | 结果 |
| --- | --- |
| `build-launcher.ps1` | 通过，release 编译 33s，产物落到 `.tools\launcher\mpack-launcher.exe` |
| `dev.ps1` | 通过，`127.0.0.1:18872` 与 `127.0.0.1:5271` 均 200，数据目录 = `%TEMP%\mpack-data` |
| `launcherCore` 在 Windows 编译 | 通过（先修了 `install.rs` 的 `cfg!` 平台门，见下） |

**真机执行暴露的两个 bug（已修）：**

1. `build-launcher.ps1` 声明了 `[switch]$Debug`。带 `[CmdletBinding()]` 的脚本不能再
   声明 cmdlet 公共参数同名（Debug / Verbose / ErrorAction 等），否则解析期就
   `MetadataError`。已改 `-DebugBuild`。
2. `launcherCore/src/java/install.rs` 用 `if cfg!(windows) { return Ok(()); }` 做平台门。
   `cfg!` 是**运行期宏**，被门住的 `use std::os::unix::fs::PermissionsExt;` 仍参与编译，
   Windows 上直接 E0433/E0599。已改成 `#[cfg(unix)]` / `#[cfg(windows)]` 双函数。
   引入时点是 `6c0aa5d`，此后内核再没被编译过（Mac 无 cargo，Windows 没跑过 cargo build），
   即**内核自 10-03 起在 Windows 上一直编不过**。

**第二轮实测（2026-10-06，把开发迁到 PC 的会话）：**

| 入口 | 结果 |
| --- | --- |
| `dev-stop.ps1 -ByPort` | 通过。`server : pid=2676 stopped (tree)` / `web : pid=13596 stopped (tree)`，端口确认释放 |
| `verify.ps1 -SkipInstall` | 大部分通过，见下 |

`verify.ps1 -SkipInstall` 逐项结果：Go tests、Go vet、前端 type check、前端
production build（3632 模块 / 4.37s）、新增的 PowerShell 脚本风格检查 —— **全绿**；
唯一红的是 `Go formatting`。

**但 `Go formatting` 的红是假红，属基础设施问题，不是代码缺陷。**

根因：仓库本地 `core.autocrlf=true`，而 `.gitattributes` 只覆盖了 `*.sql`（lf）与
`*.bat` / `*.cmd`（crlf），**没有 `*.go` 的规则**。于是 Windows 检出的 `.go` 全是 CRLF
（实测 `internal/config/config.go`：CRLF=152、独立 LF=0；`cmd/server/main.go`：184/0），
而 `gofmt` 输出 LF，`gofmt -l` 就把**每一个** `.go` 文件都报成 unformatted。
`git status` 反而是干净的（git 按 autocrlf 归一化后认为无改动），Go 代码本身也没问题
（`go test ./...`、`go vet ./...` 全绿；macOS 上 gofmt 干净）。

也就是说：Windows 上 `verify.ps1` 会**恒红在这一项**。修法见 §10 —— 在该动
`.gitattributes` 之前，别把这条当代码问题去追。

**仍未实测（下次上 Windows 请重点确认）：**

1. `dev-reset.ps1` / `build.ps1` / `package.ps1` / `test.ps1` 还没真机跑过
   （`dev-stop.ps1`、`verify.ps1` 已在本轮跑过）。
2. `verify.ps1` 不带 `-SkipInstall` 的路径没试过 —— 它会跑 `npm ci`，
   而 `npm ci` 会重建 `node_modules`，**与正在运行的 vite（node 持有文件）在 Windows
   上很可能冲突**。要跑完整 verify，先 `dev-stop.ps1` 停服务。
3. 内核只是**编译通过**，没在 Windows 上跑过实际功能（安装整合包、启动游戏）。
   `cfg(windows)` 分支（`winreg` 注册表探测、`CREATE_NEW_PROCESS_GROUP` detach、
   `tasklist` 存活检查、`%APPDATA%` 数据目录）仍是纯静态存在。
4. `build.ps1` / `package.ps1` 不产出内核 exe，分发包里也没有它——完整分发验证前需先补。
5. `build-and-deploy.md` 里描述的 Windows 迭代尚未形成回归证据。

**坑：`Invoke-WebRequest` 会走系统代理。** 机器上开着本地代理（如 Clash 的
`127.0.0.1:7897`）时，对 `127.0.0.1` 的探测请求可能被代理接管而长时间挂住，
看起来像"服务没起来"。用 curl 并显式绕开代理：

```powershell
curl.exe -s --noproxy "*" http://127.0.0.1:18872/api/health
```

## 9. 远端推 → 本机自动跟上

本机作为**消费端**时，用 `scripts/autosync.ps1` 常驻轮询远端，省掉"每次都要记得
pull + 重启"。

```powershell
pwsh scripts/autosync.ps1                     # 常驻（启动时服务没跑会先拉起）
pwsh scripts/autosync.ps1 -Once               # 只跑一轮，用于验证
pwsh scripts/autosync.ps1 -IntervalSec 15     # 调轮询间隔，默认 30s
pwsh scripts/autosync.ps1 -Once -ForceRestart # 无新提交也强制重启（验证重启路径用）
```

它只做四件事：`git fetch` → 比对 SHA → 按改动路径决定重建什么 → 需要时重启服务。

| 改动路径 | 动作 |
| --- | --- |
| `apps/web3/src/**` | 什么都不做（vite HMR 自己生效） |
| `apps/server/**` | 重启 dev 服务 |
| `launcherCore/**` | `cargo build` 后再重启 |
| `apps/web3/package*.json` | `npm ci` 后再重启 |

两条安全规则：**工作区有未提交改动时拒绝拉取**（记 WARN 到 `.tmp/autosync/autosync.log`），
只用 `--ff-only` 不做 merge。也就是说本机默认是只读消费端；确实要在本机改代码时，
改完先提交，否则同步会停住并提示。

日志：`.tmp/autosync/autosync.log`（已 gitignore）。

### 三条容易踩的实现约束（2026-10-06 实测得出）

**1. 不能用 `& pwsh -File scripts/dev.ps1` 调 dev.ps1 —— 调用方永远等不到 EOF。**
`dev.ps1` 用 `Start-Process` 拉起的 go/node 常驻服务会继承调用方的输出句柄，
于是「dev.ps1 进程早已退出」但调用方一直等不到流关闭，实测 120s 不返回。
`autosync.ps1` 因此统一走 `Start-Process` + `WaitForExit(超时)`，输出落
`.tmp/autosync/<脚本名>.out.log`，再自己轮询端口判断成败 —— **不看退出码**。

**2. 停服务必须 `dev-stop.ps1 -ByPort`，只按 PID 文件停是不够的。**
PID 文件可能缺失或过期：服务在别的会话里手工起过，或 `go run` 父进程先退出、
真正持有端口的 `server.exe` 变成孤儿。那种情况下旧进程停不掉，随后 `dev.ps1`
的 `Assert-PortFree` 会直接抛错，而端口上仍挂着旧进程 —— 于是「端口在监听」
被误判成重启成功。`Restart-Services` 现在带 `-ByPort`，并在重启后比对监听
PID：**端口在监听 ≠ 服务重启过**，未变化就明确记 WARN。

**3. `autosync.ps1` 自身的改动不会自举。**
远端更新了 `scripts/autosync.ps1` 后，磁盘文件会被正常 pull 下来，但**内存里
跑着的仍是任务启动时加载的那份代码**，本次不会生效。改完 autosync 自己的逻辑，
必须重启计划任务：

```powershell
Stop-ScheduledTask -TaskName 'mPackStation-autosync'
Start-ScheduledTask -TaskName 'mPackStation-autosync'
```

（没有 `Restart-ScheduledTask` 这个 cmdlet，只有 Stop/Start 两步。）
这条是实测踩出来的：修完 145feb7 后任务仍按旧代码打出
「只有前端源码变化，vite HMR 自行生效」，而那次实际改的是 `scripts/autosync.ps1`。

### 实测记录（2026-10-06）

| 场景 | 结果 |
| --- | --- |
| 任务启动、服务没跑 → 自动拉起 | 通过。`01:16:10 启动` → `01:16:13 服务已就绪（pid=8612,10324）`，全程 3s |
| `-Once -ForceRestart` 重启路径 | 通过。`01:20:29 无新提交 → 重启` → `01:20:34 服务已就绪（pid=9056,13344）`，监听 PID 确认换新 |
| 远端提交 → 自动拉取 | 通过。`01:23:24 发现新提交 e26c4a22 → 145feb71（1 个文件）` |
| `dev-stop.ps1 -ByPort` 清孤儿 | 通过。`server : pid=2676 stopped (tree)` / `web : pid=13596 stopped (tree)`，端口确认释放 |

**别在 SSH 前台等 autosync。** 它内部拉起的常驻服务会牵连会话，实测前台调用
收到 SIGTERM（exit 137）而远端动作其实已完成 —— 看起来像失败，实际是好的。
验证请用 `-Once` 并把输出落盘，或直接读 `.tmp/autosync/autosync.log`
（日志是 UTF-8；经 SSH 回显中文会乱码，取回本地看或用 `scp`）。

### 从别的机器用浏览器访问

两端按铁律**只绑回环**，所以别的机器访问不了，也不该为此改绑 `0.0.0.0`。
用 SSH 隧道把回环端口带过来即可，安全边界不变：

```bash
ssh -N -L 5271:127.0.0.1:5271 -L 18872:127.0.0.1:18872 suluo@10.144.144.2
```

然后在本机浏览器开 `http://127.0.0.1:5271/`。

### 让 autosync 常驻

SSH 会话里起的进程会随会话结束而受影响，用计划任务让它独立于任何终端：

```powershell
$repo = 'D:\workIn\mPackStation'
$action = New-ScheduledTaskAction -Execute 'pwsh.exe' `
  -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$repo\scripts\autosync.ps1`"" `
  -WorkingDirectory $repo
$trigger = New-ScheduledTaskTrigger -AtLogOn
# 这一段不能省。按默认值建出来的任务：ExecutionTimeLimit 是 3 天，会把常驻轮询
# 掐死；DisallowStartIfOnBatteries / StopIfGoingOnBatteries 默认 True，
# 笔记本一旦用电池就不启动、或中途被停掉。
$settings = New-ScheduledTaskSettingsSet `
  -ExecutionTimeLimit ([TimeSpan]::Zero) `
  -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
  -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) `
  -MultipleInstances IgnoreNew -StartWhenAvailable
Register-ScheduledTask -TaskName 'mPackStation-autosync' -Action $action -Trigger $trigger `
  -Settings $settings -Description 'mPackStation: 轮询远端分支并保持 dev 服务可用' -Force
```

注册后核对一遍（期望：`PT0S`、重启 3 次、电池不停）：

```powershell
(Get-ScheduledTask -TaskName 'mPackStation-autosync').Settings |
  Select-Object ExecutionTimeLimit, RestartCount, RestartInterval,
                DisallowStartIfOnBatteries, StopIfGoingOnBatteries, MultipleInstances
```

停掉/卸载：

```powershell
Stop-ScheduledTask -TaskName 'mPackStation-autosync'
Unregister-ScheduledTask -TaskName 'mPackStation-autosync' -Confirm:$false
```

## 10. 换行符策略（待定项，2026-10-06 提出）

第 8 节记录的 `Go formatting` 假红，根因是 `.gitattributes` 里没有 `*.go` 的规则，
叠加仓库本地 `core.autocrlf=true`，导致 Windows 检出的 Go 源码全是 CRLF。

两个修法，**尚未决定**（会改变跨平台行为，且本仓库历史上被换行符坑过 ——
见 `.gitattributes` 里 `*.sql` 那条注释提到的 sha256 校验）：

**方案 A：仓库级，补 `.gitattributes`（推荐，一次修好所有平台）**

```gitattributes
# 源码一律 LF：gofmt / prettier / 各类 lint 都按 LF 比对，CRLF 会造成恒定的假红。
*.go   text eol=lf
*.ts   text eol=lf
*.tsx  text eol=lf
*.js   text eol=lf
*.json text eol=lf
*.md   text eol=lf
*.ps1  text eol=lf
*.sh   text eol=lf
```

配套需要在**每台已有工作区**上重新检出（属性只影响后续 checkout）：
先 `git status` 必须是干净的，然后

```bash
git config core.autocrlf false
git rm --cached -r .
git reset --hard
```

先跑 `git status` 是为了确认没有未提交改动 —— `git reset --hard` 会丢掉它们。
Windows 上 `.bat` / `.cmd` 仍由现有属性保持 CRLF，不受影响。

**方案 B：仅本机，改 `core.autocrlf`**

只解决这一台机器，新克隆/新机器会再次踩到。而且因为 `*.ps1` 等没有属性覆盖，
单改 `core.autocrlf=false` 会让工作区里所有 CRLF 文件被 git 视为已修改，
仍然需要上面同样的重新检出步骤。**不推荐单独使用。**

在决定之前，Windows 上跑 `verify.ps1` 请用 `-SkipInstall`，
并忽略 `Go formatting` 那一项。

