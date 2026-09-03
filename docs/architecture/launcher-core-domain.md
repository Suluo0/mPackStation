# mPackLauncher 领域设计

> 关联文档：[launcher-core-spec.md](launcher-core-spec.md)（接口契约）、[launcher-core-architecture.md](launcher-core-architecture.md)（架构设计）
> 状态：设计基线

---

## 1. 领域模型总览

mPackLauncher 的领域围绕五个核心概念展开：

```
                    ┌──────────────┐
                    │  Instance    │  (Minecraft 根目录)
                    │  (实例)       │
                    └──────┬───────┘
                           │ 1:N
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ Version  │ │  Account │ │ JavaRuntime│
        │ (版本)    │ │ (账号)    │ │ (Java运行时)│
        └────┬─────┘ └──────────┘ └──────────┘
             │ 1:1
             ▼
        ┌──────────┐
        │  Loader  │  (加载器: Forge/Fabric/...)
        │ (加载器)  │
        └──────────┘
```

### 1.1 核心概念定义

| 概念 | 定义 | 唯一标识 |
|---|---|---|
| Instance | 一个独立的 Minecraft 运行环境（目录） | 目录绝对路径 |
| Version | 一个可启动的 Minecraft 版本（含加载器） | `version_id`（如 `1.20.1-fabric-0.16.5`） |
| Loader | 模组加载器类型及版本 | `loader` + `loader_version` |
| JavaRuntime | 一个 Java 运行时实例 | 路径 + 版本号 |
| Account | 一个游戏账号 | UUID |
| Mirror | 一个下载镜像源 | 枚举值 |

---

## 2. Version（版本）领域

### 2.1 版本 ID 规范

`version_id` 是版本的唯一标识，命名规则：

```
<mc_version>[-<loader>-<loader_version>]
```

示例：
- `1.20.1`（Vanilla）
- `1.20.1-fabric-0.16.5`
- `1.20.1-forge-47.2.0`
- `1.20.1-neoforge-4.10.10`

**规则**：
- Vanilla 版本 ID = mc_version
- 带加载器的版本 ID = `mc_version-loader-loader_version`
- 版本 ID 同时作为 `versions/` 下的目录名和 JSON 文件名
- 版本 ID 全局唯一（在一个 Instance 内）

### 2.2 版本状态机

```
                install
  (不存在) ──────────────→ Installing
                              │
                   ┌──────────┼──────────┐
                   ▼          ▼          ▼
              Downloading  Verifying  InstallingLoader
                   │          │          │
                   └──────────┼──────────┘
                              ▼
                          ExtractingNatives
                              │
                              ▼
                          Installed ──────→ launch ──→ Running
                              │
                         uninstall
                              ▼
                           (不存在)
```

**状态说明**：
- `(不存在)`：version JSON 不存在，或目录不完整
- `Installing`：安装流程进行中（由 progress.phase 细分）
- `Installed`：version JSON 存在且所有文件校验通过
- `Running`：游戏进程正在运行（仅 launch --wait 模式可感知）

**状态持久化**：
- 状态不单独存储，通过文件系统状态推导
- `Installed` = version JSON 存在 + client.jar 存在 + SHA1 校验通过
- 安装中断后重新 install，自动从断点恢复

### 2.3 版本兼容性规则

**Minecraft 版本与 Java 版本映射**：

| Minecraft 版本 | 最低 Java | 推荐 Java | 说明 |
|---|---|---|---|
| 1.16.5 及更早 | Java 8 | Java 8 | 旧版生态基于 Java 8 |
| 1.17 ~ 1.20.4 | Java 17 | Java 17 | 1.17 官方推荐 Java 16，但 Java 17 兼容运行且为 LTS，统一使用 Java 17 |
| 1.20.5 及更新 | Java 21 | Java 21 | 当前 LTS |

**加载器与 Minecraft 版本兼容性**：
- 每个加载器有自己的版本支持矩阵
- 安装时请求加载器 Meta API，若指定版本不支持则返回 `LoaderIncompatible` 错误
- `latest` 版本号自动解析为该 MC 版本下的最新稳定版

---

## 3. Loader（加载器）领域

### 3.1 加载器枚举

```rust
pub enum LoaderType {
    Vanilla,    // 无加载器
    Fabric,
    Forge,
    NeoForge,
    Quilt,
}
```

### 3.2 加载器安装机制分类

| 加载器 | 安装机制 | 复杂度 | 说明 |
|---|---|---|---|
| Vanilla | 无 | - | 不需要安装 |
| Fabric | Meta API → JSON 合并 | 低 | 官方 Meta API 直接返回完整 version JSON |
| Quilt | Meta API → JSON 合并 | 低 | 与 Fabric 类似，Quilt 兼容 Fabric |
| Forge | Installer → processors | 高 | 下载 installer.jar，解析 install_profile.json，执行 Java processors（含超时控制） |
| NeoForge | Installer → processors | 高 | 从 Forge 分裂，机制类似但 Maven 仓库不同 |

### 3.3 Forge/NeoForge Processor 领域

Forge 安装器的核心是 `install_profile.json`，其中声明了一系列 processors：

```
install_profile.json
  ├── version: 目标 version JSON（合并到 Minecraft version JSON）
  ├── processors: [
  │     {
  │         jar: processor 的 jar 包
  │         className: 主类名
  │         args: [参数列表，含变量替换]
  │         outputs: {输出文件: 期望哈希}
  │     }
  │ ]
  └── data: 变量定义（用于 args 中的 {} 替换）
```

**Processor 执行流程**：
1. 下载 installer.jar
2. 解压，提取 install_profile.json 和相关 jar
3. 按顺序执行每个 processor：
   - 替换 args 中的变量（`{MINECRAFT_JAR}`、`{SIDE}` 等）
   - 用 Java 执行 processor jar
   - 校验 outputs 中的文件哈希
4. 合并 version JSON 到 Minecraft version JSON
5. 写入最终的 version JSON

**Processor 执行的领域约束**：
- 必须按声明顺序执行（有依赖关系）
- 每个 processor 的 outputs 必须校验通过
- 任一 processor 失败则整个安装失败
- processor 执行需要 Java（版本通常与目标 MC 版本一致）

---

## 4. JavaRuntime（Java 运行时）领域

### 4.1 领域模型

```rust
pub struct JavaRuntime {
    pub path: PathBuf,           // java 可执行文件绝对路径
    pub version: u32,            // 主版本号（8/17/21）
    pub full_version: String,    // 完整版本（如 17.0.9+9）
    pub source: JavaSource,      // 来源
    pub architecture: Arch,      // x86_64 / aarch64
}

pub enum JavaSource {
    UserSpecified,   // --java 参数指定
    Downloaded,      // 内核自动下载
    SystemDetected,  // 系统扫描发现
}
```

### 4.2 Java 版本解析

执行 `java -version`，解析 stderr 输出：

```
openjdk version "17.0.9" 2023-10-17
OpenJDK Runtime Environment Temurin-17.0.9+9 (build 17.0.9+9)
OpenJDK 64-Bit Server VM Temurin-17.0.9+9 (build 17.0.9+9, mixed mode, sharing)
```

解析规则：
- 从 `version "XX.Y.Z"` 中提取主版本号
- Java 8 及更早格式为 `1.8.0_xxx`，主版本号为 8
- Java 9+ 格式为 `XX.Y.Z`，主版本号为 XX

### 4.3 Java 选择策略

```
输入: mc_version, user_java_path
  │
  ├─ user_java_path 存在?
  │    ├─ 是 → 校验版本匹配 → 匹配则使用，不匹配则报错
  │    └─ 否 ↓
  │
  ├─ 扫描已下载的 Java（<data_dir>/java/）
  │    └─ 有匹配版本 → 使用
  │
  ├─ 扫描系统 Java
  │    ├─ 常见路径
  │    ├─ JAVA_HOME
  │    └─ PATH
  │    └─ 有匹配版本 → 使用
  │
  └─ 自动下载匹配版本
       ├─ 下载成功 → 注册并使用
       └─ 下载失败 → 报错，提示手动安装
```

### 4.4 Java 下载源

- **主源**：Adoptium Temurin API（`api.adoptium.net`）
- **兜底**：BMCLAPI Java 镜像
- 下载格式：tar.gz / zip，解压到 `<data_dir>/java/<version>/`
- 下载后校验：执行 `java -version` 确认版本正确

---

## 5. Account（账号）领域

### 5.1 账号类型

```rust
pub enum AccountType {
    Offline,     // 离线账号
    Microsoft,   // 微软账号
}
```

### 5.2 离线账号

- 用户名：用户指定的任意字符串
- UUID：由用户名通过标准算法生成
  ```
  UUID = UUID.nameUUIDFromBytes(("OfflinePlayer:" + username).getBytes("UTF-8"))
  ```
- access_token：空字符串
- 不需要网络，不需要认证

### 5.3 微软账号

**认证流程（device flow）**：

```
1. 请求 device code
   POST login.microsoftonline.com/consumers/oauth2/v2.0/devicecode
   → user_code, verification_uri, device_code, interval, expires_in

2. 输出 user_code + verification_uri，等待用户在浏览器操作

3. 轮询 token（间隔 interval 秒）
   POST login.microsoftonline.com/consumers/oauth2/v2.0/token
   grant_type=device_code
   → 等待中: authorization_pending
   → 成功: access_token, refresh_token, expires_in

4. Xbox Live 认证
   POST user.auth.xboxlive.com/user/authenticate
   → xbl_token, user_hash

5. XSTS 认证
   POST xsts.auth.xboxlive.com/xsts/authorize
   → xsts_token

6. Minecraft 登录
   POST api.minecraftservices.com/authentication/login_with_xbox
   → mc_access_token, uuid

7. 获取 Profile
   GET api.minecraftservices.com/minecraft/profile
   → username

8. 缓存所有凭证
```

**Token 生命周期管理**：

```
              登录
               │
               ▼
        ┌───────────────┐
        │   Active      │  access_token 有效
        └───────┬───────┘
                │ access_token 接近过期(<5min)
                ▼
        ┌───────────────┐
        │  Refreshing   │  用 refresh_token 换新 token
        └───────┬───────┘
                │
           ┌────┴────┐
           ▼         ▼
       Active    Expired  (refresh_token 也过期)
                     │
                     ▼
                RequireLogin
```

**凭证存储**：
- 存储后端：系统 keyring（Windows DPAPI / macOS Keychain / Linux Secret Service）
- Linux 无桌面环境时降级为加密文件（机器指纹派生密钥 + 0600 权限）
- 存储内容：access_token、refresh_token、uuid、username、expires_at

---

## 6. Download（下载）领域

### 6.1 下载任务模型

```rust
pub struct DownloadTask {
    pub url: String,
    pub destination: PathBuf,
    pub expected_sha1: Option<String>,
    pub size: Option<u64>,
    pub kind: DownloadKind,  // ClientJar / Library / Asset / Native / Installer / Java
}
```

### 6.2 下载状态机

```
Pending → Downloading → Verifying → Completed
  │          │             │
  │          ├─失败→ Retry ─┤
  │          │    (最多3次)  │
  │          │              │
  │          └─失败→ Failed  │
  │                        │
  └─校验不匹配→ Redownload ─┘
```

### 6.3 断点续传

- 支持 Range 请求的文件（HTTP 206）：
  - 检查 `.partial` 文件的已下载字节数
  - 发送 `Range: bytes=<offset>-` 请求
  - 追加写入 `.partial` 文件
  - 完成后 rename 为目标文件
- 不支持 Range 的文件：从头下载

### 6.4 并发下载策略

- **assets**：32 并发（文件小但数量多，通常数百到数千个）
- **libraries**：16 并发（文件较大，数量较少）
- **client.jar**：单独下载（单文件，通常 20-50MB）
- **natives**：随 libraries 一起下载
- 全局 HTTP 连接池复用，每个 host 最大连接数 = 并发数

### 6.5 校验策略

- 下载过程中流式计算 SHA1（边下边算，不二次读取）
- 下载完成后比对期望 SHA1
- 不匹配则删除文件，标记为需重新下载
- 安装时再次校验（防止缓存损坏或磁盘错误）

---

## 7. Mirror（镜像源）领域

### 7.1 镜像源枚举

```rust
pub enum Mirror {
    Mojang,    // 官方源
    Bmclapi,   // BMCLAPI 国内镜像
    Auto,      // 自动模式
}
```

### 7.2 URL 重写映射

| 官方域名 | 路径 | BMCLAPI 重写 |
|---|---|---|
| `piston-meta.mojang.com` | `/mc/game/version_manifest.json` | `bmclapi.bangbang93.com/mc/game/version_manifest.json` |
| `piston-data.mojang.com` | `/v1/objects/...` | `bmclapi.bangbang93.com/v1/objects/...` |
| `libraries.minecraft.net` | `/...` | `bmclapi.bangbang93.com/maven/...` |
| `resources.download.minecraft.net` | `/<ab>/<hash>` | `bmclapi.bangbang93.com/assets/<ab>/<hash>` |

### 7.3 Auto 模式（竞速降级）

```
┌─────────────────────────────────────────────┐
│              单个文件下载                      │
│                                             │
│  ┌──────────┐        ┌──────────────┐       │
│  │  Mojang  │        │   BMCLAPI    │       │
│  │  (竞速)  │        │   (竞速)     │       │
│  └────┬─────┘        └──────┬───────┘       │
│       │                     │               │
│       └─────────┬───────────┘               │
│                 ▼                           │
│         谁先完成且校验通过 → 用谁，取消另一个   │
│         两个都失败 → 报错                     │
└─────────────────────────────────────────────┘
```

**规则**：
- 每个文件同时启动 Mojang 和 BMCLAPI 两个下载任务
- Mojang 连接超时 5s、下载超时 30s，超时后视为失败
- 哪个源先完成且 SHA1 校验通过就采用哪个，取消另一个
- 两个源都失败则该文件下载失败
- 无全局状态，每个文件独立决策（无状态短期进程设计）

---

## 8. Error（错误）领域

### 8.1 错误分类

| 类别 | 退出码 | 可恢复性 | 说明 |
|---|---|---|---|
| 参数错误 | 1 | 不可（用户修正参数） | 参数缺失、格式错误、非法值 |
| 网络错误 | 2 | 可重试 | 连接失败、超时、DNS 错误 |
| 校验错误 | 3 | 可重试 | SHA1 不匹配、文件损坏 |
| 版本错误 | 4 | 不可（版本不存在） | 版本号无效、加载器不兼容 |
| Java 错误 | 5 | 可（安装 Java） | Java 未找到、版本不匹配 |
| 认证错误 | 6 | 可（重新登录） | 登录失败、token 过期 |
| 游戏崩溃 | 10 | 视情况 | 游戏进程非零退出 |

### 8.2 错误领域模型

```rust
pub struct ErrorContext {
    pub code: ErrorCode,
    pub message: String,           // 用户可读消息
    pub suggestion: Option<String>, // 可操作建议
    pub details: serde_json::Value, // 结构化详情
    pub source: Option<Box<dyn Error>>, // 原始错误链（仅 stderr）
}
```

### 8.3 常见错误与建议映射

| 错误 | 建议 |
|---|---|
| DownloadFailed | "网络不稳定，可尝试 --mirror bmclapi 使用国内镜像" |
| JavaNotFound | "未找到 Java {required}，可运行 `mpack-launcher java install --version {required}` 自动安装" |
| JavaVersionMismatch | "需要 Java {required}，当前是 {found}，请安装对应版本或使用 --java 指定路径" |
| GameCrashed (OOM) | "游戏内存不足，尝试增大 --xmx 参数" |
| GameCrashed (OpenGL) | "显卡驱动可能过旧，尝试更新显卡驱动" |
| AuthFailed | "登录失败，请重新运行 `mpack-launcher auth login`" |
| LoaderIncompatible | "{loader} 不支持 Minecraft {mc_version}，请检查版本组合" |

---

## 9. Protocol（协议事件）领域

### 9.1 事件类型

启动器只输出两种事件到 stdout，不做字节级进度上报：

```rust
pub enum ProtocolEvent {
    Phase {
        phase: String,    // 阶段标识
        message: String,  // 人类可读中文描述
    },
    Result {
        success: bool,
        data: Option<Value>,      // 成功时的结果数据
        error: Option<String>,    // 失败时的错误类型
        message: Option<String>,  // 失败时的用户友好消息
        suggestion: Option<String>, // 失败时的可操作建议
    },
}
```

### 9.2 阶段标识

| phase | 触发时机 |
|---|---|
| `resolving_version` | 开始解析版本 manifest |
| `downloading_libraries` | 开始下载支持库 |
| `downloading_assets` | 开始下载资源文件 |
| `installing_loader` | 开始安装加载器 |
| `verifying` | 开始校验文件 |
| `preparing` | 启动前准备 |
| `launching` | 正在 spawn 游戏进程 |

### 9.3 输出约束

- 每次调用输出 0~N 个 phase 事件 + 恰好 1 个 result 事件
- phase 事件仅在流程进入新阶段时输出，不做 per-file 上报
- 不输出百分比、下载速度、字节数等细粒度进度
- 日志（tracing）走 stderr，与 stdout 协议完全分离

---

## 10. 领域不变量

以下规则在任何情况下都必须成立：

1. **version_id 唯一性**：一个 Instance 内不能有两个相同 version_id 的版本
2. **Java 版本匹配**：启动时 Java 主版本必须满足该 MC 版本的最低要求
3. **文件完整性**：Installed 状态的版本，其所有文件必须通过 SHA1 校验
4. **凭证安全**：认证 token 不得以明文形式出现在日志、错误信息、stdout 中
5. **路径安全**：所有写入操作必须在 Instance 目录或数据目录内，禁止目录穿越
6. **幂等性**：对同一版本重复执行 install，结果一致，不重复下载已校验文件
7. **退出码确定性**：同一错误类型总是返回相同退出码
8. **JSON 协议完整性**：`--json` 模式下 stdout 每行必须是合法 JSON，最后一行必须是 result 类型
