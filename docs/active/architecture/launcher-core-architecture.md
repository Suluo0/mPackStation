# mPackLauncher 架构设计

> 关联文档：[launcher-core-spec.md](launcher-core-spec.md)（接口契约）
> 状态：设计基线

---

## 1. 架构总览

### 1.1 模块架构

mPackLauncher 采用"CLI 入口 + 四个能力模块"的结构，每个能力模块内部按子职责拆分为多文件，避免上帝代码：

```
src/
├── main.rs            程序入口
├── cli.rs             命令行解析、子命令分发、错误→退出码映射
├── error.rs           统一错误类型（被所有模块引用）
├── protocol.rs        stdout JSON 事件协议（phase 阶段变化 + result 最终结果）
├── platform.rs        平台差异（OS/内存/数据目录/路径规范化）
│
├── download/          下载能力模块
│   ├── mod.rs         Downloader 公开 API + download_all 编排
│   ├── mirror.rs      URL 重写 + 多源竞速降级（按文件类型分类）
│   ├── item.rs        DownloadItem + FileChecker（下载前预校验跳过）
│   ├── concurrent.rs  双 Semaphore 并发调度（assets 32 / libraries 16）
│   └── cache.rs       断点续传 + 流式 SHA1 校验 + 原子 rename
│
├── loader/            加载器安装模块
│   ├── mod.rs         公开 API：install(loader_type, mc_version)
│   ├── fabric.rs      Fabric Meta API + JSON 合并
│   ├── forge.rs       installer 下载 + processor 执行 + 超时控制
│   ├── quilt.rs       Quilt Meta API + JSON 合并
│   └── neoforge.rs    NeoForge 版本解析 + installer 执行
│
├── launch/            启动能力模块
│   ├── mod.rs         公开 API：build + spawn + wait + kill
│   ├── command.rs     启动命令构建（classpath、JVM 参数、游戏参数）
│   └── process.rs     进程管理（spawn/detach/wait/kill、崩溃日志收集）
│
├── java/              Java 运行时模块
│   ├── mod.rs         公开 API：detect + install + select
│   ├── detect.rs      系统 Java 扫描 + 版本解析
│   ├── install.rs     Adoptium 下载 + 解压 + 验证
│   └── registry.rs    多版本管理 + 版本匹配策略
│
└── auth/              认证模块
    ├── mod.rs         公开 API：login + status + logout
    ├── offline.rs     离线账号 + UUID v3 生成
    ├── microsoft.rs   微软 OAuth device flow + token 刷新
    └── store.rs       keyring 加密存储
```

**输出通道分离**：
- **stderr**：tracing 日志（人类可读，调试用），级别由 `MPACK_LOG` 环境变量控制
- **stdout**：`protocol.rs` 输出的 JSON Lines（机器可读，Go 后端解析），只有两种事件：`phase`（阶段变化）和 `result`（最终结果）
- 两者完全分离，日志不混入 stdout，协议事件不混入 stderr

### 1.2 进程模型

```
┌──────────────────────┐
│  mPackStation (Go)   │
│  后端进程             │
│  ├─ task worker      │
│  └─ os/exec          │
└──────────┬───────────┘
           │ CLI 调用 + stdin/stdout JSON Lines
           ▼
┌──────────────────────┐
│  mPackLauncher (Rust)│
│  子进程，短期/中期存活 │
│  ├─ install 模式     │──→ 完成后退出（秒~分钟级）
│  ├─ launch 模式      │──→ 默认 spawn 游戏后退出
│  │                   │    --wait 模式随游戏存活
│  └─ auth 模式        │──→ 完成后退出
└──────────┬───────────┘
           │ spawn
           ▼
┌──────────────────────┐
│  Minecraft (Java)    │
│  独立进程             │
│  与 mPackLauncher 无关│
└──────────────────────┘
```

**关键设计决策**：
- mPackLauncher 是**无状态短期进程**，每次调用独立，不维护常驻状态
- 状态通过文件系统持久化（已安装版本、Java 缓存、认证 token）
- 游戏进程与 mPackLauncher 解耦：launch 默认模式 spawn 后立即退出，游戏继续运行
- 这种设计保证：内核崩溃不影响游戏，游戏崩溃不影响内核

### 1.3 数据流

#### 安装流程数据流

```
CLI 参数
  │
  ▼
cli.rs → download::Downloader::install()
  ├─→ protocol::phase("resolving_version", "正在解析版本信息")
  ├─→ mc-launcher-core: 解析 version manifest + 加载器元数据
  ├─→ download/mirror.rs: 按文件类型重写 URL
  ├─→ download/item.rs: FileChecker 预校验，跳过已存在文件
  ├─→ protocol::phase("downloading_libraries", "正在下载支持库")
  ├─→ download/concurrent.rs: 双 Semaphore 调度下载
  ├─→ download/cache.rs: 断点续传 + 流式 SHA1 + 原子 rename
  ├─→ protocol::phase("downloading_assets", "正在下载资源文件")
  ├─→ (Forge) loader/forge.rs: 下载 installer + 执行 processor
  ├─→ java/: Forge processor 执行时需要匹配 Java
  └─→ protocol::result(success=true, version_id=...)
```

#### 启动流程数据流

```
CLI 参数
  │
  ▼
cli.rs → launch::Launcher::launch()
  ├─→ protocol::phase("preparing", "正在准备启动")
  ├─→ java/: 匹配对应版本的 Java 路径
  ├─→ auth/: 获取账号信息（离线 UUID / 微软 token）
  ├─→ mc-launcher-core: 加载 version JSON + 构建 LaunchCommand
  ├─→ launch/command.rs: 拼接 classpath、JVM 参数、游戏参数
  ├─→ platform.rs: 平台路径分隔符、natives 过滤
  ├─→ protocol::phase("launching", "正在启动游戏")
  ├─→ launch/process.rs: spawn 游戏进程（detach 或 wait）
  └─→ protocol::result(success=true, pid=...)
```

---

## 2. 模块详细设计

### 2.1 CLI 层（main.rs / cli.rs）

**职责**：
- 定义 clap 命令树（子命令、参数、帮助文本）
- 解析命令行参数
- 初始化运行时（tracing、错误钩子、信号处理）
- 分发到对应编排模块
- 统一捕获错误，映射到退出码

**clap 命令树**：

```
mpack-launcher
├── install    --mc <VER> [--loader ...] [--dir ...] [--java ...] [--mirror ...] ...
├── launch     --version <ID> [--dir ...] [--java ...] [--account-type ...] ...
├── auth
│   ├── login   [--provider microsoft|offline] [--username ...]
│   ├── status
│   └── logout
├── java
│   ├── list
│   ├── install  --version <VER>
│   ├── remove   --version <VER>
│   └── detect
├── list       [--dir ...]
└── version
```

**错误处理**：
- 所有编排函数返回 `Result<(), LauncherError>`
- main 中统一捕获，调用 `error.exit_code()` 和 `error.user_message()`
- `--json` 模式下输出 JSON 错误对象，非 JSON 模式输出人类可读文本
- release 构建使用 `panic = "abort"`，不做 catch_unwind；panic 视为不可恢复的内部错误，进程直接终止并返回非零退出码

### 2.2 编排层

#### 2.2.1 install.rs

**职责**：安装流程的总编排。

**核心函数**：
```rust
pub async fn execute(args: InstallArgs) -> Result<InstallResult, LauncherError>
```

**流程状态机**：

```
Init → ResolveVersion → JavaSetup → Download → Verify → InstallLoader → ExtractNatives → Done
  │         │              │           │          │            │                │
  │         └─版本不存在→ Fail         │          │            │                │
  │                        └─Java失败→ Fail       │            │                │
  │                                      └─下载失败→ Fail(可重试) │                │
  │                                                  └─校验失败→ ReDownload       │
  │                                                                └─Loader失败→ Fail
  │                                                                                  │
  └──────────────────────────────────────────────────────────────────────────────────┘
```

**并发设计**：
- 下载阶段使用 tokio 并发，两个独立 Semaphore 控制并发数
- assets 组：32 并发（文件小但数量多）
- libraries 组：16 并发（文件较大）
- 每个文件下载是一个 tokio task，完成后发送 progress 事件
- 下载使用 `FuturesUnordered` 收集结果，任意失败不中断其他下载

**幂等性**：
- 每个文件下载前检查本地是否存在且 SHA1 匹配
- 匹配则跳过，不匹配则重新下载
- 支持 Range 的大文件（>10MB）支持断点续传

#### 2.2.2 launch.rs

**职责**：启动流程编排。

**核心函数**：
```rust
pub fn execute(args: LaunchArgs) -> Result<LaunchResult, LauncherError>
```

**两种模式**：
- **detach 模式（默认）**：spawn 游戏进程，输出 PID，立即退出
- **wait 模式**：spawn 后等待进程退出，收集退出码和最后 50 行日志

**游戏进程管理**：
- 使用 `std::process::Command`，不使用第三方库
- 工作目录设为 Minecraft 根目录
- stdout/stderr 根据 `--log-file` 参数决定继承或重定向
- 环境变量继承父进程，可通过 `--jvm-args` 传递 JVM 属性

#### 2.2.3 auth.rs

**职责**：认证管理。

**子模块**：
- `offline`：离线账号生成
- `microsoft`：微软 OAuth device flow 完整实现
- `store`：token 加密存储与读取

**微软 OAuth 状态机**：

```
Start → RequestDeviceCode → WaitForUser → PollToken → XboxAuth → XstsAuth → MinecraftAuth → GetProfile → Save → Done
  │            │                  │             │           │           │              │
  │            └─网络错误→ Fail   └─超时→ Fail   └─拒绝→ Fail └─失败→ Fail └─失败→ Fail   └─无配置→ Fail
```

**token 生命周期**：
- access_token 有效期约 24 小时
- refresh_token 有效期约 90 天
- launch 时检查 access_token 过期时间，< 5 分钟则自动刷新
- 刷新失败则清除缓存，提示重新登录

#### 2.2.4 java.rs

**职责**：Java 运行时管理。

**核心抽象**：
```rust
pub struct JavaRuntime {
    pub path: PathBuf,
    pub version: JavaVersion,  // 主版本号：8/17/21
    pub source: JavaSource,    // SystemSpecified / Detected / Downloaded
}

pub enum JavaVersion {
    Java8, Java17, Java21, Other(u32),
}
```

**版本匹配规则**（查表法）：
```rust
pub fn required_java_version(mc_version: &str) -> JavaVersion {
    // 1.16.5 及更早 → Java8
    // 1.17 ~ 1.20.4 → Java17（1.17 官方推荐 16，但 17 兼容且为 LTS）
    // 1.20.5+ → Java21
}
```

**检测优先级**：
1. `--java` 指定的路径
2. 内核数据目录中已下载的 Java
3. 系统常见路径扫描
4. JAVA_HOME 环境变量
5. PATH 中的 java
6. 自动下载

### 2.3 核心层（mc-launcher-core）

这是外部依赖，我们不修改其源码，通过封装层调用。

**使用的 API**：
- `Launcher::new(dir)`：创建启动器实例
- `launcher.install(InstallRequest)`：安装版本
- `launcher.load_version(id)`：加载已安装版本的元数据
- `launcher.build_launch_command_from_version(version, options)`：构建启动命令

**封装原则**：
- 不直接暴露 mc-launcher-core 的类型到 CLI 层
- 在编排层做适配，将库的错误转换为 `LauncherError`
- 将库的进度事件转换为我们的 `ProgressEvent` 格式
- Cargo.lock 钉死库版本，升级需评估

### 2.4 基础设施层

#### 2.4.1 mirror.rs

**职责**：下载源管理、URL 重写、多源竞速降级。

**核心抽象**：
```rust
pub enum Mirror {
    Mojang,
    Bmclapi,
    Auto,  // 竞速模式：双源同时下载，谁先完成且校验通过用谁
}
```

**URL 重写规则（按文件类型分类）**：
- `piston-meta.mojang.com` → `bmclapi.bangbang93.com`（版本元数据）
- `piston-data.mojang.com` → `bmclapi.bangbang93.com`（client.jar）
- `libraries.minecraft.net` → `bmclapi.bangbang93.com/maven`（支持库）
- `resources.download.minecraft.net` → `bmclapi.bangbang93.com/assets`（资源文件）
- 第三方库（非 Mojang 域名）不加重写，直接用原 URL

**auto 模式（竞速+超时降级）**：
- 每个文件同时启动官方源和 BMCLAPI 两个下载任务
- 官方源连接超时 5s、下载超时 30s，超时后视为失败
- 哪个源先完成且 SHA1 校验通过就用哪个，取消另一个
- 两个源都失败则报错
- 无跨进程状态，单次调用内独立决策

#### 2.4.2 cache.rs

**职责**：下载缓存和断点续传。

**缓存策略**：
- 下载中的临时文件：`*.partial`，与目标文件同目录
- 下载完成且 SHA1 校验通过后，原子 rename 为目标文件
- 启动时扫描 `.partial` 文件，支持 Range 的大文件可断点续传
- 幂等检查：下载前检查目标文件是否存在且 SHA1 匹配，匹配则跳过
- 不设独立缓存层，文件直接写入目标位置（libraries/assets 按标准路径组织，天然可复用）

#### 2.4.3 protocol.rs

**职责**：stdout JSON 事件协议输出。这是给 Go 后端解析的机器可读协议，不是日志。

**两种事件**：

| 事件类型 | 触发时机 | 用途 |
|---|---|---|
| `phase` | 流程进入新阶段时 | 告诉前端"当前在做什么"（如"正在下载资源文件"） |
| `result` | 流程结束时 | 成功/失败 + 结果数据 |

**输出格式**：JSON Lines over stdout，每行一个 JSON 对象。

**与日志的分离**：
- 日志（tracing）→ stderr，人类可读，调试用
- 协议事件（protocol.rs）→ stdout，JSON 格式，Go 后端解析
- 两者完全独立，互不干扰

**线程安全**：使用 `std::io::StdoutLock` 保证多行输出不交错。

#### 2.4.4 error.rs

**职责**：统一错误类型、退出码映射、用户友好消息。

**设计原则**：
- 每个错误变体包含足够的上下文信息
- 每个错误变体映射到唯一退出码
- 每个错误变体提供 `suggestion()` 方法，返回用户可操作建议
- 错误链保留 source，便于调试（stderr 输出完整链，stdout 只输出用户消息）

#### 2.4.5 platform.rs

**职责**：平台相关的检测和路径处理。

**功能**：
- OS 检测（Windows/Linux/macOS）
- 架构检测（x86_64/aarch64）
- 系统内存总量检测（用于自动 Xmx 分配）
- 常见 Java 安装路径枚举（按平台不同）
- 路径规范化（绝对路径、符号链接解析）
- 数据目录定位（按平台不同）

---

## 3. 并发模型

### 3.1 异步运行时

- 使用 `tokio` 多线程运行时
- 工作线程数 = CPU 核心数（默认）
- 安装流程是 async，启动流程是 sync（进程 spawn 本身是阻塞的）

### 3.2 下载并发

```
┌─────────────────────────────────────────┐
│          install (async fn)              │
│                                         │
│  assets (Semaphore=32)                  │
│  ┌─ task1 ─┐  ┌─ task2 ─┐  ...  ┌─ taskN─┐│
│  │ 下载+校验 │  │ 下载+校验 │       │ 下载+校验 ││
│  └────┬────┘  └────┬────┘       └────┬────┘│
│       │            │                  │     │
│       └────────────┼──────────────────┘     │
│                    ▼                        │
│            protocol::phase() 阶段事件        │
│                                         │
│  libraries (Semaphore=16)                │
│  （同上结构）                              │
└─────────────────────────────────────────┘
```

### 3.3 事件输出

- 使用 `tokio::sync::Semaphore` 限制并发数
- 每个下载 task 开始时 acquire，结束时 release
- 阶段事件（phase）通过 `protocol::phase()` 输出到 stdout，仅在流程进入新阶段时触发，不做 per-file 上报
- `protocol.rs` 内部使用 `stdout.lock()` 保证多行输出不交错
- 日志（tracing）走 stderr，与 stdout 协议完全分离

---

## 4. 状态持久化

### 4.1 持久化数据

| 数据 | 位置 | 格式 | 加密 |
|---|---|---|---|
| 已安装版本 | `<dir>/versions/` | 目录 + JSON | 否 |
| 游戏文件 | `<dir>/libraries/`, `assets/`, `natives/` | 二进制 | 否 |
| Java 运行时 | `<data_dir>/java/<version>/` | 目录 | 否 |
| 认证 token | 系统 keyring（Windows DPAPI / macOS Keychain / Linux Secret Service） | 加密 | 是 |
| 下载缓存 | `<data_dir>/cache/downloads/` | 二进制 | 否 |
| Java 注册表 | `<data_dir>/java/registry.json` | JSON | 否 |

### 4.2 无状态设计

mPackLauncher 进程本身是无状态的：
- 每次调用读取文件系统获取当前状态
- 不在内存中维护跨调用的状态
- 这保证了进程可以随时被 kill，不会丢失状态

---

## 5. 与外部系统的交互

### 5.1 外部 API 依赖

| 服务 | 用途 | 端点 |
|---|---|---|
| Mojang Piston Meta | 版本清单 | `piston-meta.mojang.com` |
| Mojang Piston Data | client.jar 下载 | `piston-data.mojang.com` |
| Minecraft Libraries | 库文件下载 | `libraries.minecraft.net` |
| Minecraft Resources | assets 下载 | `resources.download.minecraft.net` |
| Fabric Meta | Fabric 版本元数据 | `meta.fabricmc.net` |
| Forge Files | Forge installer 下载 | `files.minecraftforge.net` |
| NeoForge Maven | NeoForge 下载 | `maven.neoforged.net` |
| Quilt Meta | Quilt 版本元数据 | `meta.quiltmc.org` |
| Microsoft OAuth | 认证 | `login.microsoftonline.com` |
| Xbox Live | XBL 认证 | `user.auth.xboxlive.com` |
| XSTS | XSTS 认证 | `xsts.auth.xboxlive.com` |
| Minecraft Services | MC 登录/Profile | `api.minecraftservices.com` |
| Adoptium | Java 下载 | `api.adoptium.net` |
| BMCLAPI | 国内镜像 | `bmclapi.bangbang93.com` |

### 5.2 与 mPackStation 的交互

- 协议：CLI + stdin/stdout JSON Lines
- 调用方式：`os/exec`
- 生命周期：mPackStation 管理 mPackLauncher 进程的创建和等待
- 进度透传：mPackStation 读取 mPackLauncher 的 stdout，转发到 task events
- 错误处理：mPackStation 根据退出码和 JSON 结果判断成功/失败

---

## 6. 可观测性

### 6.1 日志

- 使用 `tracing` 结构化日志
- 输出到 stderr，不影响 stdout 的 JSON 协议
- 级别由 `MPACK_LOG` 环境变量控制（error/warn/info/debug/trace）
- 日志字段：timestamp、level、module、operation、duration_ms、error
- 不记录敏感信息（token、完整路径）

### 6.2 协议事件

- 通过 stdout JSON Lines 输出，供 Go 后端解析
- 只有两种事件：
  - `phase`：流程进入新阶段时输出，包含阶段标识和人类可读消息
  - `result`：流程结束时输出，包含成功/失败和结果数据
- 不做 per-file 进度上报，不输出百分比/速度/字节数
- 调用方根据 phase 切换前端显示文字，根据 result 判断完成

### 6.3 退出码

- 每个错误类型映射到唯一退出码
- 调用方根据退出码快速判断错误类别
- 详细信息在最后一行 JSON 中

---

## 7. 安全架构

### 7.1 攻击面

- 出站 HTTP 请求：只访问可信域名，校验证书（rustls 默认验证）
- 文件写入：只写入 Minecraft 目录和数据目录，路径规范化防止目录穿越
- 进程执行：只执行 Java 和 Forge processors（校验来源）
- 本地存储：认证 token 加密存储

### 7.2 凭证保护

- 微软 token 通过 `keyring` crate 存储到系统凭证库（Windows DPAPI / macOS Keychain / Linux Secret Service）
- Linux 无桌面环境时降级为加密文件（机器指纹派生密钥 + 0600 权限）
- 日志中 token 脱敏（只显示前后 4 位）
- 错误信息中不包含 token
- 进程命令行参数中不传递 token（启动时从 keyring 读取）

### 7.3 下载安全

- 所有下载文件校验 SHA1（与官方哈希比对）
- Forge installer 校验 SHA1（与官方发布的哈希比对）
- 不执行未校验的代码
- URL 重写只在白名单域名间进行

---

## 8. 架构决策记录（ADR）

### ADR-001：独立进程而非 cgo 库

**决策**：mPackLauncher 作为独立 binary，通过 CLI 被 Go 调用，而非编译为 cgo 静态库。

**理由**：
- 保持 mPackStation 纯 Go 无 cgo 的架构原则
- 独立进程崩溃不影响主进程
- 可独立分发、独立升级
- 未来可直接作为独立启动器后端

**代价**：进程间通信开销（可忽略，CLI 调用频率低）。

### ADR-002：基于 mc-launcher-core 而非完全自研

**决策**：使用 `mc-launcher-core` (MIT) 作为核心库，封装一层编排逻辑。

**理由**：
- 全加载器支持，避免重复造轮子（尤其是 Forge processor）
- MIT 许可证无传染
- 专注于编排、体验、性能优化，而非底层协议实现

**代价**：依赖第三方库，API 变更需适配（Cargo.lock 钉死版本）。

### ADR-003：无状态短期进程

**决策**：mPackLauncher 每次调用是独立进程，不维护常驻状态。

**理由**：
- 简单可靠，崩溃无副作用
- 状态通过文件系统持久化，可观测可调试
- 与 mPackStation 的任务系统天然契合（每个任务一个进程）

**代价**：每次调用有进程启动开销（~50ms，可接受）。

### ADR-004：JSON Lines over stdout 作为进度协议

**决策**：进度通过 stdout 的 JSON Lines 实时输出，而非 HTTP/WebSocket。

**理由**：
- 简单，无需额外端口和服务
- 与 `os/exec` 天然配合
- 行分隔易于解析
- 单向数据流，无并发问题

**代价**：不支持双向通信（当前不需要）。

### ADR-005：auto 镜像模式默认开启

**决策**：默认 `--mirror auto`，双源竞速模式（官方 + BMCLAPI 同时下载，谁快用谁）。

**理由**：
- 国内用户访问 Mojang 不稳定，竞速模式同时利用两个源，最快可用
- 用户无需了解镜像概念
- 竞速模式比"先官方失败再降级"延迟更低（不需要等超时才切换）

**代价**：竞速模式在网络好时会浪费一半带宽（两个源同时下载），但下载完成后立即取消慢的那个，实际浪费有限。
