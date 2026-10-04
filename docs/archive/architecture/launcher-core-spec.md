# mPackLauncher 启动器内核规格说明书

> 状态：设计基线
> 日期：2026-09-03
> 关联决策：启动内核（终局=启动 Minecraft）
> 依赖：`mc-launcher-core` (Rust, MIT)
> 设计目标：最高兼容性 / 最好性能 / 最无缝体验

---

## 0. 文档定位

本文是 mPackLauncher 启动器内核的**唯一设计基线**，按最终完成态编写，不设版本范围限制。实现分里程碑推进，但设计覆盖全部目标能力。后续实现、测试、验收均以本文为准。

---

## 1. 目标与边界

### 1.1 三大设计目标

#### 最高兼容性

- **Minecraft 版本**：支持 Mojang version manifest 中的全部正式版本（1.6+ 保证，1.5 及更早/Alpha/Beta 最佳努力）
- **模组加载器**：Forge、NeoForge、Fabric、Quilt
- **平台**：Windows x64 / Linux x64 / macOS (Intel + Apple Silicon)
- **Java 运行时**：自动检测、自动下载、多版本并存，按版本需求自动匹配（Java 8/17/21）
- **认证**：离线账号 + 微软 OAuth（device flow）
- **网络环境**：直连 + BMCLAPI 镜像，自动降级

#### 最好性能

- **并发下载**：assets 32 并发、libraries 16 并发，HTTP 连接池复用
- **内存占用**：内核进程空闲 < 20MB，安装峰值 < 100MB
- **启动速度**：内核进程冷启动 < 50ms，已安装版本启动到游戏窗口 < 3s（不含游戏自身加载）
- **分发体积**：单 binary < 10MB（Windows），strip + LTO + panic=abort
- **幂等安装**：已下载文件 SHA1 校验跳过，中断恢复零重复下载
- **校验效率**：下载流式计算 SHA1，不做二次读取

#### 最无缝体验

- **一键启动**：给定 mc 版本 + 加载器 + 目录，一条命令完成安装→启动
- **自动 Java 管理**：用户不需要知道什么是 Java，内核自动下载匹配版本
- **阶段事件**：JSON Lines 流式上报，仅在流程进入新阶段时输出（不做 per-file 进度）
- **友好错误**：每个失败场景有稳定 error_code + 用户可理解的 message + 可操作建议
- **零配置**：合理默认值，高级参数可覆盖但不需要
- **与 mPackStation 深度集成**：纳入任务系统，进度透传到前端，游戏日志可查看

### 1.2 非目标

- 不做 GUI（无头内核，通过 CLI 被上层调用）
- 不做模组下载、管理、排序（由 mPackStation 负责）
- 不做整合包构建、发布（由 mPackStation 负责）
- 不做跨实例游戏文件共享（每个实例独立目录）
- 不做游戏内 mod 配置编辑（启动后由游戏内菜单处理）
- 不保证 Classic / Pre-classic / Infdev 等超古版本（无标准 version JSON）

### 1.3 产品边界

```
mPackStation (Go 后端)
    │  CLI 调用 + JSON Lines 通信
    ▼
mPackLauncher (Rust 单 binary)
    │  基于 mc-launcher-core
    ▼
Minecraft (Java 进程)
```

mPackLauncher 是**独立进程**，不是 cgo 库。保证：
- mPackStation 保持纯 Go 无 cgo
- 启动器内核可独立分发、独立升级
- 未来可直接作为独立启动器的后端

---

## 2. 技术选型

### 2.1 语言与核心依赖

| 项 | 选择 | 理由 |
|---|---|---|
| 语言 | Rust | 单 binary、低内存、跨平台、启动器生态库成熟、无 GC 停顿 |
| 核心库 | `mc-launcher-core` (MIT) | 全加载器支持、API 干净、MIT 无传染、兼容层处理旧版本 |
| 异步运行时 | tokio | 并发下载、进度上报、任务编排 |
| HTTP | reqwest (rustls) | 纯 Rust TLS，无 OpenSSL 依赖，支持连接池 |
| 序列化 | serde + serde_json | 标准选择 |
| 哈希 | sha1 + sha2 | 资源文件校验 |
| 压缩 | zip + flate2 | natives 解压、mrpack 处理 |
| 错误处理 | thiserror + anyhow | 统一错误类型 + 便捷传播 |
| 日志 | tracing + tracing-subscriber | 结构化日志，可过滤级别 |
| CLI 解析 | clap (derive) | 标准选择，自动生成 help |
| 协议输出 | protocol.rs | stdout JSON Lines（phase 阶段事件 + result 最终结果） |

### 2.2 为什么不选其他方案

| 方案 | 排除原因 |
|---|---|
| Go 自研 | Go 生态无全加载器库，Forge/NeoForge processor 需自行实现，工作量 2.5-3 月 |
| Python (minecraft-launcher-lib) | 需打包 Python 运行时，体积大(~50MB)，启动慢，GIL 限制并发 |
| Java (HMCLCore) | GPLv3 传染，需 JRE，技术栈不匹配 |
| PCL CE 改造 | C#/WPF Windows only，自定义许可证限制，无干净 CLI 接口 |
| Prism CLI 嵌入 | 体积大(~100MB+)，黑盒控制力弱，作为短期兜底保留但不作为长期内核 |

### 2.3 Rust 工具链

- 工具链通过 `.tools/rust` 便携分发，不要求用户系统安装 Rust
- 构建使用稳定版 Rust（stable channel）
- 交叉编译目标：`x86_64-pc-windows-msvc`、`x86_64-unknown-linux-gnu`、`aarch64-apple-darwin`、`x86_64-apple-darwin`
- 所有平台均做功能验收，不只是编译验证

---

## 3. 目录结构

```
apps/launcher/
├── Cargo.toml
├── Cargo.lock
├── build.rs                    # 版本信息注入（git commit、构建时间）
├── src/
│   ├── main.rs                 # 入口，运行时初始化，子命令分发
│   ├── cli.rs                  # clap 命令定义、参数解析、错误→退出码映射
│   ├── error.rs                # 统一错误类型 + 退出码映射 + 用户友好 message
│   ├── protocol.rs             # stdout JSON 事件协议（phase 阶段变化 + result 最终结果）
│   ├── platform.rs             # 平台检测、路径规范化、OS 特定处理
│   ├── download/               # 下载能力模块
│   │   ├── mod.rs              # Downloader 公开 API + download_all 编排
│   │   ├── mirror.rs           # URL 重写 + 多源竞速降级（按文件类型分类）
│   │   ├── item.rs             # DownloadItem + FileChecker（预校验跳过）
│   │   ├── concurrent.rs       # 双 Semaphore 并发调度
│   │   └── cache.rs            # 断点续传 + 流式 SHA1 + 原子 rename
│   ├── loader/                 # 加载器安装模块
│   │   ├── mod.rs              # 公开 API：install(loader_type, mc_version)
│   │   ├── fabric.rs           # Fabric Meta API + JSON 合并
│   │   ├── forge.rs            # installer 下载 + processor 执行 + 超时控制
│   │   ├── quilt.rs            # Quilt Meta API + JSON 合并
│   │   └── neoforge.rs         # NeoForge 版本解析 + installer 执行
│   ├── launch/                 # 启动能力模块
│   │   ├── mod.rs              # 公开 API：build + spawn + wait + kill
│   │   ├── command.rs          # 启动命令构建（classpath、JVM 参数、游戏参数）
│   │   └── process.rs          # 进程管理（spawn/detach/wait/kill、崩溃日志）
│   ├── java/                   # Java 运行时模块
│   │   ├── mod.rs              # 公开 API：detect + install + select
│   │   ├── detect.rs           # 系统 Java 扫描 + 版本解析
│   │   ├── install.rs          # Adoptium 下载 + 解压 + 验证
│   │   └── registry.rs         # 多版本管理 + 版本匹配策略
│   └── auth/                   # 认证模块
│       ├── mod.rs              # 公开 API：login + status + logout
│       ├── offline.rs          # 离线账号 + UUID v3 生成
│       ├── microsoft.rs        # 微软 OAuth device flow + token 刷新
│       └── store.rs            # keyring 加密存储
├── tests/
│   ├── install_vanilla.rs      # Vanilla 多版本安装测试
│   ├── install_fabric.rs       # Fabric 安装测试
│   ├── install_forge.rs        # Forge 安装测试（含旧版）
│   ├── install_neoforge.rs     # NeoForge 安装测试
│   ├── launch_smoke.rs         # 启动冒烟测试（离线账号）
│   ├── java_auto.rs            # Java 自动检测/下载测试
│   ├── mirror.rs               # 镜像源降级测试
│   ├── protocol.rs             # 协议事件输出测试
│   └── error_cases.rs          # 错误场景测试
└── README.md
```

---

## 4. CLI 接口契约

### 4.1 通用约定

- 所有命令接受 `--dir <PATH>` 指定 Minecraft 根目录（等价于 `.minecraft`）
- 所有命令接受 `--json` 标志，启用结构化 JSON 输出（默认人类可读文本）
- 进度信息通过 **stdout 的 JSON Lines** 输出，每行一个 JSON 对象
- 最终结果通过 **退出码 + 最后一行 JSON** 表达
- 日志通过 **stderr** 输出，级别由 `MPACK_LOG` 环境变量控制（error/warn/info/debug/trace）
- 所有路径参数接受绝对路径和相对路径，内部规范化为绝对路径

### 4.2 退出码

| 退出码 | 含义 | 用户可操作建议 |
|---|---|---|
| 0 | 成功 | - |
| 1 | 通用错误（参数错误、IO 错误等） | 检查参数和文件权限 |
| 2 | 网络错误（下载失败、API 不可达） | 检查网络或配置镜像源 |
| 3 | 校验错误（SHA1 不匹配、文件损坏） | 重新运行 install（会自动重下损坏文件） |
| 4 | 版本解析错误（版本不存在、加载器不兼容） | 检查版本号和加载器组合 |
| 5 | Java 运行时错误（路径不存在、版本不匹配） | 安装对应 Java 版本或使用自动下载 |
| 6 | 认证错误（微软登录失败、token 过期） | 重新登录 |
| 10 | Minecraft 进程异常退出 | 查看游戏日志 |
| 130 | 用户中断（Ctrl+C） | - |

### 4.3 子命令

#### 4.3.1 `install` — 安装游戏版本

```
mpack-launcher install [OPTIONS] --mc <VERSION>
```

**参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `--mc` | string | 是 | - | Minecraft 版本，如 `1.20.1`、`1.21.1`、`1.8.9` |
| `--loader` | enum | 否 | `vanilla` | `vanilla` / `fabric` / `forge` / `neoforge` / `quilt` |
| `--loader-version` | string | 否 | `latest` | 加载器版本，`latest` 表示最新稳定版 |
| `--dir` | path | 否 | `./.minecraft` | Minecraft 根目录 |
| `--java` | path | 否 | 自动匹配 | Java 可执行文件路径；不指定则自动检测/下载匹配版本 |
| `--mirror` | enum | 否 | `auto` | `auto` / `mojang` / `bmclapi`；auto 模式先试官方，失败自动降级 BMCLAPI |
| `--concurrency` | int | 否 | 32 | 并发下载数（assets），libraries 为其一半 |
| `--force` | flag | 否 | false | 强制重新下载已存在的文件 |
| `--json` | flag | 否 | false | JSON 输出模式 |

**输出（JSON 模式）**：

阶段事件（JSON Lines，每行一个，仅在进入新阶段时输出）：
```json
{"type":"phase","phase":"resolving_version","message":"正在解析版本信息"}
{"type":"phase","phase":"downloading_libraries","message":"正在下载支持库"}
{"type":"phase","phase":"downloading_assets","message":"正在下载资源文件"}
{"type":"phase","phase":"installing_loader","message":"正在安装 Fabric"}
```

最终结果：
```json
{"type":"result","success":true,"data":{"version_id":"1.20.1-fabric-0.16.5","mc_version":"1.20.1","loader":"fabric","loader_version":"0.16.5"}}
```

错误结果：
```json
{"type":"result","success":false,"error":"DownloadFailed","message":"下载 client.jar 失败：连接重置，已重试 3 次","suggestion":"网络不稳定，可尝试 --mirror bmclapi 使用国内镜像"}
```

#### 4.3.2 `launch` — 启动游戏

```
mpack-launcher launch [OPTIONS] --version <VERSION_ID>
```

**参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `--version` | string | 是 | - | 版本 ID，由 install 输出的 `version_id` |
| `--dir` | path | 否 | `./.minecraft` | Minecraft 根目录 |
| `--java` | path | 否 | 自动匹配 | Java 可执行文件路径 |
| `--account-type` | enum | 否 | `offline` | `offline` / `microsoft` |
| `--username` | string | 条件 | - | 离线账号用户名（offline 时必填）；microsoft 时从缓存读取，不通过命令行传递凭证 |
| `--xmx` | string | 否 | 自动 | 最大堆内存，如 `2G`、`4096M`；自动按系统内存 50% 分配 |
| `--xms` | string | 否 | 同 Xmx | 初始堆内存 |
| `--jvm-args` | string | 否 | - | 额外 JVM 参数，空格分隔 |
| `--game-args` | string | 否 | - | 额外游戏参数，空格分隔 |
| `--width` | int | 否 | 854 | 窗口宽度 |
| `--height` | int | 否 | 480 | 窗口高度 |
| `--server` | string | 否 | - | 启动后自动连接的服务器地址 |
| `--demo` | flag | 否 | false | 演示模式 |
| `--json` | flag | 否 | false | JSON 输出模式 |
| `--wait` | flag | 否 | false | 等待游戏进程退出后再返回 |
| `--log-file` | path | 否 | - | 将游戏 stdout/stderr 重定向到此文件 |

**输出（JSON 模式）**：

启动成功（非 wait 模式）：
```json
{"type":"result","success":true,"pid":12345,"version_id":"1.20.1-fabric-0.16.5","java":{"path":"/path/to/java","version":"17.0.9"},"command":"java -Xmx2G ...","log_file":"/path/to/logs/latest.log"}
```

启动成功（wait 模式，正常退出）：
```json
{"type":"result","success":true,"pid":12345,"exit_code":0,"duration_ms":125430}
```

游戏崩溃（wait 模式）：
```json
{"type":"result","success":false,"error_code":"game_crashed","message":"Minecraft 异常退出（退出码 1）","details":{"exit_code":1,"pid":12345,"duration_ms":3420,"last_log_lines":["...","java.lang.OutOfMemoryError: Java heap space"]},"suggestion":"可能是内存不足，尝试增大 --xmx"}
```

#### 4.3.3 `auth` — 认证管理

```
mpack-launcher auth login [--provider microsoft|offline] [--username NAME]
mpack-launcher auth status
mpack-launcher auth logout
```

**微软登录流程（device flow）**：
1. 执行 `mpack-launcher auth login --provider microsoft`
2. 输出：
```json
{"type":"auth_device_code","user_code":"XXXX-XXXX","verification_uri":"https://www.microsoft.com/link","message":"请在浏览器打开 https://www.microsoft.com/link 并输入代码 XXXX-XXXX"}
```
3. 内核轮询 token 端点，用户在浏览器完成登录后自动获取 token
4. 成功后输出：
```json
{"type":"result","success":true,"username":"Steve","uuid":"...","access_token":"***","expires_at":1710000000}
```
5. token 加密缓存到本地，后续 `launch --account-type microsoft` 自动使用

**离线登录**：
```
mpack-launcher auth login --provider offline --username Steve
```
直接生成离线账号（UUID 由用户名哈希生成），缓存到本地。

**auth status**：输出当前缓存的账号信息（token 脱敏）。

**auth logout**：清除本地缓存的 token。

#### 4.3.4 `java` — Java 运行时管理

```
mpack-launcher java list                    # 列出已检测/已下载的 Java
mpack-launcher java install --version 17    # 下载指定 Java 版本
mpack-launcher java remove --version 17     # 删除指定 Java 版本
mpack-launcher java detect                   # 检测系统已安装的 Java
```

**Java 版本匹配规则**：
- Minecraft 1.16.5 及更早：Java 8
- Minecraft 1.17 ~ 1.20.4：Java 17（1.17 官方推荐 Java 16，但 Java 17 兼容运行且为 LTS，统一使用 Java 17）
- Minecraft 1.20.5 及更新：Java 21
- 加载器有特殊要求时以加载器要求为准

**下载源**：Adoptium Temurin（官方，提供 Java 8/17/21 LTS），BMCLAPI 镜像兜底。

#### 4.3.5 `list` — 列出已安装版本

```
mpack-launcher list --dir <PATH> [--json]
```

输出：
```json
{"type":"result","success":true,"versions":[{"id":"1.20.1-fabric-0.16.5","mc_version":"1.20.1","loader":"fabric","loader_version":"0.16.5","installed_at":1710000000000,"size_bytes":524288000}]}
```

#### 4.3.6 `version` — 查看内核版本

```
mpack-launcher version
```

输出：`mPackLauncher <version> (mc-launcher-core <version>, built <time>, commit <hash>)`

---

## 5. 核心模块设计

### 5.1 install 模块

**职责**：解析版本清单、并发下载、校验完整性、安装加载器。

**流程**：

```
1. 解析 --mc 和 --loader 参数
2. 选择镜像源（auto 模式先官方，失败降级 BMCLAPI）
3. 请求 version_manifest，获取指定版本的 version JSON URL
4. 下载并解析 version JSON
5. Java 运行时准备：
   - 如果指定了 --java，校验版本是否匹配
   - 否则检测系统 Java，匹配则使用
   - 否则自动下载匹配版本的 Java
6. 如果有加载器：
   - Fabric/Quilt：请求 Meta API，合并 loader version JSON
   - Forge/NeoForge：下载 installer，解析 install_profile.json，执行 processors（含超时控制）
7. 并发下载：
   - client.jar（SHA1 校验）
   - libraries（按 rules 过滤 OS/arch，SHA1 校验，16 并发）
   - assets（解析 asset index，32 并发下载 objects，SHA1 校验）
8. 解压 native 库到 natives/ 目录
9. 原子写入 version JSON 到 versions/<id>/<id>.json（先写临时文件再 rename）
10. 输出 version_id 和安装统计
```

**并发策略**：
- assets 下载：默认 32 并发（`--concurrency` 可调）
- libraries 下载：默认 16 并发（为 assets 的一半）
- HTTP 连接池复用，每个 host 最大连接数 = 并发数
- 单文件下载失败：重试 3 次，指数退避（1s/2s/4s），切换镜像源
- 下载流式计算 SHA1，不做二次读取
- 全部下载完成后统一校验，校验失败的文件重新下载
- 断点续传：支持 Range 请求的大文件断点续传

**协议事件上报**：
- phase 枚举：`resolving_version` / `downloading_libraries` / `downloading_assets` / `installing_loader` / `verifying` / `preparing` / `launching`
- 仅在流程进入新阶段时输出 phase 事件，不做 per-file 进度上报
- 不输出百分比、下载速度、字节数等细粒度进度
- 最终输出一行 result 事件表示成功/失败

### 5.2 launch 模块

**职责**：加载版本 JSON，构建 Java 启动命令，spawn 进程。

**构建命令的要素**：
1. Java 可执行文件路径（`--java` 或自动匹配）
2. JVM 参数：
   - `-Xmx` / `-Xms`（自动按系统内存 50% 分配，可覆盖）
   - `-Djava.library.path=<natives>`
   - `-Dminecraft.client.jar=<client.jar>`
   - `-Dminecraft.launcher.brand=mPackLauncher`
   - `-Dminecraft.launcher.version=<内核版本>`
   - 额外 `--jvm-args`
3. Classpath：按 version JSON 的 libraries 顺序拼接，加上 client.jar
4. MainClass：从 version JSON 读取
5. 游戏参数：`--username`、`--version`、`--gameDir`、`--assetsDir`、`--assetIndex`、`--uuid`、`--accessToken`、`--userType`、`--versionType`、`--width`、`--height`、`--server`、`--demo`
6. 工作目录：Minecraft 根目录
7. 环境变量：继承父进程，可通过 `--jvm-args` 传递 `-D` 属性

**内存自动分配**：
- 检测系统总内存
- Xmx = min(系统内存 * 50%, 4GB)（上限 4GB 避免占用过多）
- 系统内存 < 4GB 时 Xmx = 系统内存 * 75%
- 可通过 `--xmx` 完全覆盖

**进程管理**：
- 默认模式：spawn 后立即返回 PID，游戏进程独立于内核进程
- `--wait` 模式：等待进程退出，返回退出码、运行时长、最后 50 行日志
- `--log-file`：将游戏 stdout/stderr 重定向到指定文件
- 游戏进程的 stdout/stderr 默认继承父进程（方便用户看日志），`--json` 模式下必须指定 `--log-file`

### 5.3 auth 模块

**离线账号**：
- 用户名直接作为游戏内显示名
- UUID 由用户名通过 MD5 哈希生成（离线模式标准算法：`UUID.nameUUIDFromBytes(("OfflinePlayer:" + name).getBytes())`）
- access token 为空字符串

**微软 OAuth（device flow）**：
1. POST `https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode`，获取 user_code + verification_uri + device_code
2. 输出 user_code 和 verification_uri，等待用户操作
3. 轮询 POST `https://login.microsoftonline.com/consumers/oauth2/v2.0/token`（grant_type=device_code），间隔 5 秒
4. 获取 access_token 后，POST `https://user.auth.xboxlive.com/user/authenticate` 获取 XBL token
5. POST `https://xsts.auth.xboxlive.com/xsts/authorize` 获取 XSTS token
6. POST `https://api.minecraftservices.com/authentication/login_with_xbox` 获取 Minecraft access_token + UUID
7. GET `https://api.minecraftservices.com/minecraft/profile` 获取用户名
8. 缓存：access_token + refresh_token + UUID + 用户名 + 过期时间，加密存储到本地

**token 刷新**：
- launch 时检查 token 是否过期，过期则用 refresh_token 自动刷新
- 刷新失败则提示重新登录

**本地存储**：
- Windows：DPAPI 加密
- macOS：Keychain
- Linux：`~/.config/mpack-launcher/auth.json`，权限 0600，文件内容加密（主密钥派生自机器指纹）

### 5.4 java 模块

**职责**：Java 运行时的检测、下载、版本匹配、多版本管理。

**检测逻辑**：
1. 检查 `--java` 指定的路径
2. 检查内核数据目录中已下载的 Java（`<data_dir>/java/<version>/`）
3. 扫描常见安装路径：
   - Windows：`C:\Program Files\Java\`、`C:\Program Files\Eclipse Adoptium\`、`%LOCALAPPDATA%\Packages\Microsoft.4297127D64EC6_8wekyb3d8bbwe\LocalCache\Local\`
   - Linux：`/usr/lib/jvm/`、`/usr/java/`
   - macOS：`/Library/Java/JavaVirtualMachines/`、`~/Library/Java/JavaVirtualMachines/`
4. 检查 `JAVA_HOME` 环境变量
5. 检查 PATH 中的 `java`
6. 对每个候选执行 `java -version`，解析版本号

**下载逻辑**：
- 从 Adoptium Temurin API 获取下载链接
- BMCLAPI 镜像兜底
- 下载到 `<data_dir>/java/<version>/`
- 解压后校验
- 记录到本地 Java 注册表

**版本匹配**：
- 按 Minecraft 版本映射到 Java 大版本（见 4.3.4）
- 优先使用已检测到的匹配版本
- 无匹配则自动下载
- 下载失败则报错并提示手动安装

### 5.5 mirror 模块

**职责**：下载源管理、自动降级、故障转移。

**镜像源**：

| 镜像 | 资源覆盖 | 说明 |
|---|---|---|
| `mojang` | 官方全部资源 | piston-meta / piston-data / libraries.minecraft.net / resources.download.minecraft.net |
| `bmclapi` | 全部资源镜像 | bangbang93 维护的国内镜像，覆盖 version manifest / client.jar / libraries / assets |
| `auto` | 智能切换 | 先试官方，超时/失败自动降级 BMCLAPI |

**auto 模式策略（竞速+超时降级）**：
1. 每个文件同时启动官方源和 BMCLAPI 两个下载任务（竞速）
2. 官方源超时阈值 5s（连接）/ 30s（下载），超时后视为失败
3. 哪个源先完成且校验通过就用哪个，取消另一个
4. 两个源都失败则报错
5. 单次调用内的失败计数不跨进程持久化（无状态设计）

### 5.6 error 模块

统一错误枚举（部分）：

```rust
#[derive(thiserror::Error, Debug)]
pub enum LauncherError {
    #[error("参数错误: {0}")]
    InvalidArgument(String),
    #[error("网络错误: {0}")]
    Network(#[from] reqwest::Error),
    #[error("下载失败: {url}（已重试 {attempts} 次）")]
    DownloadFailed { url: String, attempts: u32 },
    #[error("校验失败: {file}（期望 {expected}，实际 {actual}）")]
    ChecksumMismatch { file: String, expected: String, actual: String },
    #[error("版本不存在: {0}")]
    VersionNotFound(String),
    #[error("加载器不兼容: {loader} 不支持 Minecraft {mc_version}")]
    LoaderIncompatible { loader: String, mc_version: String },
    #[error("未找到 Java 运行时（需要 Java {required}）")]
    JavaNotFound { required: String },
    #[error("Java 版本不匹配（需要 {required}，实际 {found}）")]
    JavaVersionMismatch { required: String, found: String },
    #[error("微软登录失败: {0}")]
    AuthFailed(String),
    #[error("游戏崩溃（退出码 {code}）")]
    GameCrashed { code: i32 },
    #[error("IO 错误: {0}")]
    Io(#[from] std::io::Error),
    #[error("JSON 错误: {0}")]
    Json(#[from] serde_json::Error),
}
```

每个错误变体映射到退出码（见 4.2），并提供 `suggestion()` 方法返回用户可操作建议。

### 5.7 protocol 模块

stdout JSON 事件协议，只有两种事件类型：

```rust
// phase 事件：流程进入新阶段
pub struct PhaseEvent {
    pub r#type: &'static str,  // "phase"
    pub phase: String,         // 阶段标识
    pub message: String,       // 人类可读中文描述
}

// result 事件：流程结束
pub struct ResultEvent {
    pub r#type: &'static str,  // "result"
    pub success: bool,
    pub data: Option<Value>,   // 成功时的结果数据
    pub error: Option<String>, // 失败时的错误类型
    pub message: Option<String>, // 失败时的用户友好消息
    pub suggestion: Option<String>, // 失败时的可操作建议
}
```

通过 stdout 以 JSON Lines 格式输出，每行一个事件。调用方（Go 侧）按行读取解析。日志（tracing）走 stderr，与 stdout 协议完全分离。

---

## 6. 目录约定

### 6.1 Minecraft 根目录结构

标准 Minecraft 目录结构，与官方启动器、Prism、HMCL 兼容：

```
<dir>/
├── versions/
│   └── <version_id>/
│       ├── <version_id>.json
│       └── <version_id>.jar
├── libraries/
│   └── ... (按 Maven 路径组织)
├── assets/
│   ├── indexes/
│   │   └── <asset_index>.json
│   └── objects/
│       └── <ab>/<hash>
├── natives/
│   └── <version_id>/
│       └── *.dll / *.so / *.dylib
└── logs/
    └── latest.log
```

### 6.2 内核数据目录

```
<data_dir>/
├── java/
│   └── <version>/           # 自动下载的 Java 运行时
├── auth/
│   └── credentials.json     # 加密的认证信息
└── cache/
    └── downloads/           # 下载缓存（断点续传临时文件）
```

- Windows：`%APPDATA%\mPackLauncher\`
- Linux：`~/.local/share/mpack-launcher/`
- macOS：`~/Library/Application Support/mPackLauncher/`

### 6.3 与 mPackStation 的数据隔离

mPackStation 的每个整合包实例使用**独立的 Minecraft 目录**：

```
data/instances/<pack_id>/minecraft/
```

不同包之间的游戏文件、配置、存档完全隔离，符合 v7 架构的 `pack_id` 分域原则。

---

## 7. 构建与分发

### 7.1 构建配置

`Cargo.toml` release profile：

```toml
[profile.release]
opt-level = "z"        # 最小体积优化
lto = true             # 链接时优化
codegen-units = 1      # 单代码生成单元（更好的优化）
panic = "abort"        # 终止而非展开（减小体积）
strip = true           # 自动 strip 符号
```

### 7.2 目标 binary 大小

| 平台 | 预估大小 | 说明 |
|---|---|---|
| Windows x64 | ~6-8 MB | 静态链接，含 reqwest+rustls+tokio |
| Linux x64 | ~5-7 MB | 静态链接 |
| macOS arm64 | ~6-8 MB | - |
| macOS x64 | ~6-8 MB | - |

### 7.3 分发方式

- 构建产物放在 `dist/mpack-launcher/` 下，随 mPackStation 一起分发
- 便携 Rust 工具链放在 `.tools/rust/`，构建脚本自动使用
- 构建脚本：`scripts/build-launcher.ps1`（Windows）、`scripts/build-launcher.sh`（POSIX）
- 支持交叉编译：Windows 宿主可构建 Linux 版本；macOS 版本需在 macOS 宿主上构建（需 Apple SDK）

### 7.4 版本信息注入

通过 `build.rs` 注入：
- 版本号（从 Cargo.toml 读取）
- mc-launcher-core 版本
- Git commit hash
- 构建时间
- 构建平台

`mpack-launcher version` 输出以上信息。

---

## 8. 与 mPackStation 的集成

### 8.1 Go 侧调用方式

mPackStation 后端通过 `os/exec` 调用 mPackLauncher binary：

```go
// 伪代码
cmd := exec.Command("mpack-launcher", "install",
    "--mc", "1.20.1",
    "--loader", "fabric",
    "--dir", instanceDir,
    "--mirror", "auto",
    "--json",
)
stdout, _ := cmd.StdoutPipe()
// 逐行读取 stdout 解析进度事件 → 写入 task events
// 最后一行解析结果
// 退出码判断成功/失败
```

### 8.2 任务化

安装和启动操作纳入 mPackStation 的任务系统：
- 安装任务：`kind=install_game`，异步执行，进度通过 task events 实时上报到前端
- 启动任务：`kind=launch_game`，启动后返回 PID，游戏进程独立于后端进程
- 任务状态机复用现有 lease/fencing/幂等机制
- 游戏日志通过 `--log-file` 写入实例目录，前端可查看

### 8.3 配置

mPackStation 设置页新增：
- `launcher.java_path`：Java 可执行文件路径（留空则自动检测/下载）
- `launcher.default_xmx`：默认最大内存（留空则自动分配）
- `launcher.mirror`：下载镜像源（auto/mojang/bmclapi）
- `launcher.concurrent_downloads`：并发下载数（默认 32）
- `launcher.data_dir`：内核数据目录（Java、认证缓存）

### 8.4 终局闭环

```
用户在 mPackStation 构建 mrpack
    → 后端调用 mpack-launcher install（自动安装对应版本+加载器+Java）
    → 后端将 mrpack 解压到实例目录
    → 后端调用 mpack-launcher launch（自动匹配 Java，启动游戏）
    → 游戏窗口出现，终局达成
```

用户全程不需要手动安装 Java、不需要知道加载器版本、不需要配置环境变量。

---

## 9. 安全边界

- mPackLauncher 不监听任何网络端口，只做出站 HTTP/HTTPS 请求
- 只请求可信域名：Mojang 官方、BMCLAPI、Adoptium、Fabric Meta、Forge Files、NeoForge Maven、Quilt Meta
- 下载的文件严格校验 SHA1（官方提供的哈希）
- Forge processors 执行前校验 installer 的 SHA1（与官方发布的哈希比对），执行时设置超时（默认 300s），超时则终止并报错
- 不写入 Minecraft 目录和内核数据目录以外的路径；`--dir` 和 `--log-file` 参数经过路径规范化和安全校验，禁止目录穿越
- 认证 token 加密存储，不落盘明文；不通过命令行参数传递 token（从凭证存储读取）
- 错误信息脱敏：不泄漏完整绝对路径、不泄漏 token
- 并发安装同一目录时使用文件锁（`<dir>/.mpack-install.lock`），防止多进程数据竞争
- 不执行下载的任意代码（Forge processors 除外，这是 Forge 安装机制的必要部分，且校验来源+超时控制）

---

## 10. 错误恢复

| 场景 | 恢复策略 |
|---|---|
| 下载中断 | 已下载文件保留 SHA1 校验，重新运行 install 时跳过已校验文件；支持 Range 的大文件断点续传 |
| 安装过程崩溃 | version JSON 未写入则视为未安装；已写入但文件不全则下次 install 自动补全缺失文件 |
| Forge processor 失败 | 输出完整 processor 日志，建议重试；连续失败提示检查 Java 版本；自动切换镜像源重试 |
| 游戏启动崩溃 | 返回退出码 + 最后 50 行日志 + 常见错误匹配建议（如 OOM→增大内存、OpenGL 错误→检查显卡驱动） |
| Java 版本不匹配 | 明确提示需要的版本，提供自动下载选项 |
| 微软 token 过期 | 自动用 refresh_token 刷新；刷新失败提示重新登录 |
| 镜像源不可达 | auto 模式自动降级；手动模式报错并建议切换 |
| 磁盘空间不足 | 安装前预检可用空间，不足则提前报错并提示需要的空间 |

---

## 11. 测试策略

### 11.1 单元测试

- CLI 参数解析测试（所有子命令、所有参数组合）
- 错误类型到退出码的映射测试
- 错误 suggestion 生成测试
- 版本 JSON 合并逻辑测试（Fabric/Forge 继承）
- JVM 命令构建测试（classpath 顺序、参数替换、内存自动分配）
- Java 版本匹配规则测试
- 协议事件（phase/result）序列化/反序列化测试
- 镜像源 URL 重写测试

### 11.2 集成测试（需要网络）

**必测（验收门槛）**：
- Vanilla 1.20.1 安装 + 启动冒烟
- Vanilla 1.21.1 安装 + 启动冒烟
- Fabric 1.20.1 安装 + 启动冒烟
- Forge 1.20.1 安装 + 启动冒烟
- NeoForge 1.20.1 安装 + 启动冒烟
- Java 自动下载（全新环境，无系统 Java）
- 下载中断恢复（kill 后重跑，验证零重复下载）
- 镜像源降级（模拟官方超时，验证自动切 BMCLAPI）

**兼容性测试（记录结果，不阻塞发布）**：
- Vanilla 1.16.5 + Java 8
- Vanilla 1.8.9 + Java 8
- Forge 1.16.5（旧版 installer 格式）
- Quilt 1.20.1
- macOS Apple Silicon 启动

### 11.3 性能测试

- 冷启动时间 < 50ms
- 空闲内存 < 20MB
- 安装 1.20.1 Vanilla 总耗时（100Mbps 网络）< 60s
- 安装峰值内存 < 100MB
- binary 大小 < 10MB

### 11.4 验收标准

- 必测场景全部通过
- 性能指标全部达标
- 重复 install 不重复下载（幂等）
- `--json` 模式下 stdout 每一行都是合法 JSON
- 退出码与错误类型一一对应
- 错误信息包含用户可操作的建议
- Windows/Linux/macOS 三平台编译通过且功能测试通过

---

## 12. 里程碑

| 阶段 | 内容 | 交付物 | 验收门槛 |
|---|---|---|---|
| **M-1** | 技术预研：验证 mc-launcher-core 真实能力 | Vanilla + Fabric + Forge 1.20.1 安装跑通的 PoC，确认库 API 与假设一致 | 库能独立完成三种安装，无阻塞性缺陷 |
| **M0** | 项目骨架 + CLI 框架 + Vanilla 安装 | 能 install Vanilla 并输出 version_id，含阶段事件上报 | 幂等安装、JSON Lines 协议完整、退出码正确 |
| **M1** | Vanilla 启动 + Fabric 安装/启动 + Java 自动检测 | 能启动 Vanilla 和 Fabric 到主菜单 | 离线启动成功、Java 自动检测、--wait 模式返回退出码 |
| **M2** | Forge/NeoForge/Quilt 安装/启动 | 全加载器支持，processor 流程跑通 | 三种加载器均能安装并启动、processor 失败有清晰日志 |
| **M3** | Java 自动下载 + 镜像源 + 微软 OAuth | 零配置一键启动，国内网络可用 | 全新环境自动下载 Java、auto 镜像降级、微软登录+token 刷新 |
| **M4** | 错误处理完善 + 性能优化 + 跨平台验证 | 友好错误、性能达标、三平台可用 | 所有错误有 suggestion、性能指标达标、Linux/macOS 编译通过 |
| **M5** | mPackStation 集成 + 任务化 + 终局闭环 | 从 mPackStation 一键启动 Minecraft | 前端进度实时展示、游戏日志可查看、终局闭环无人工干预 |

里程碑按依赖顺序推进，前一里程碑验收通过后才进入下一里程碑。不设固定工期，以验收门槛为完成标准。

---

## 13. 风险与应对

| 风险 | 概率 | 影响 | 应对 |
|---|---|---|---|
| mc-launcher-core 的 Forge processor 实现有 bug | 中 | 高 | M2 重点验证；不行则自己写 Forge 安装逻辑，或用 portablemc 兜底 |
| mc-launcher-core 版本升级破坏 API | 低 | 中 | Cargo.lock 钉死版本；封装层做适配，不直接暴露库类型 |
| 旧版 Forge（1.16 及以前）installer 格式不兼容 | 中 | 中 | 标记为最佳努力，不阻塞发布；后续按需适配 |
| Windows 上 Rust 交叉编译 Linux/macOS 复杂 | 中 | 低 | 主平台保证 Windows，Linux/macOS 用 CI 或对应平台构建 |
| 国内网络访问 Mojang/Files 慢 | 高 | 中 | BMCLAPI 镜像 + auto 降级，M3 实现 |
| 微软 OAuth 流程复杂 | 低 | 中 | 参考成熟实现，device flow 是标准协议，文档齐全 |
| Java 自动下载在受限网络失败 | 中 | 低 | 报错提示手动安装，保留 `--java` 参数覆盖 |

---

## 14. 未来独立路径

mPackLauncher 从设计上就是可独立的：

1. **代码独立**：放在 `apps/launcher/`，不 import mPackStation 的任何包
2. **接口独立**：CLI 是唯一接口，不依赖 mPackStation 的内部类型
3. **配置独立**：通过命令行参数和环境变量配置，不读 mPackStation 的 config.toml
4. **构建独立**：有自己的 Cargo.toml 和构建脚本
5. **数据独立**：有自己的数据目录，不与 mPackStation 混用

未来独立时：
- 将 `apps/launcher/` 整体迁出为新仓库
- CLI 接口保持不变（语义化版本保证兼容）
- mPackStation 继续通过 CLI 调用，无需修改
- 可独立发布 Release，其他项目也能使用
- 可基于此内核开发独立 GUI 启动器（Tauri/Egui）

---

## 15. 受保护文件

```text
docs/architecture/launcher-core-spec.md       # 本规格，变更需 ADR/评审记录
apps/launcher/Cargo.toml, Cargo.lock          # 依赖变更需说明许可证、安全、体积影响
apps/launcher/src/cli.rs                      # CLI 接口变更需同步契约文档和 Go 侧适配器
```

---

## 附录 A：mc-launcher-core 能力清单

- [x] Vanilla 版本解析与安装（全版本，基于 Mojang manifest）
- [x] Fabric 安装（Meta API）
- [x] Quilt 安装
- [x] Forge 安装（install_profile + processors）
- [x] NeoForge 安装
- [x] client.jar / libraries / assets / natives 下载与校验
- [x] 继承版本元数据合并
- [x] Java 启动命令构建
- [x] 离线账号
- [x] 微软认证 helper（auth 模块）
- [x] macOS Apple Silicon 兼容性调整（旧版本 LWJGL 补丁）
- [x] 协议事件（protocol 模块，phase + result）
- [ ] Java 运行时自动下载（新 facade 不包含，我们自己实现）
- [ ] mrpack 安装（不包含，由 mPackStation 负责）
- [ ] BMCLAPI 镜像（不包含，我们自己实现）

---

## 附录 B：术语表

| 术语 | 含义 |
|---|---|
| Vanilla | 原版 Minecraft，无模组加载器 |
| Loader / 加载器 | Forge/Fabric/NeoForge/Quilt，让 mod 能加载进游戏的框架 |
| Processor | Forge 安装器中的安装处理器，Java 程序，执行 patch/生成等操作 |
| Asset Index | Minecraft 资源文件索引，声明每个资源的哈希和路径 |
| Natives | 平台相关的原生库（dll/so/dylib），LWJGL 等需要 |
| Version JSON | Minecraft 版本元数据，声明 libraries、assets、mainClass、参数等 |
| mrpack | Modrinth 整合包格式，zip 压缩，含 mod 列表和 overrides |
| BMCLAPI | bangbang93 维护的国内 Minecraft 资源镜像 |
| Device Flow | 微软 OAuth 2.0 设备授权流程，适用于无浏览器/输入受限的设备 |
