# mPackLauncher 技术设计

> 关联文档：[launcher-core-spec.md](launcher-core-spec.md)（接口契约）、[launcher-core-architecture.md](launcher-core-architecture.md)（架构设计）、[launcher-core-domain.md](launcher-core-domain.md)（领域设计）
> 状态：设计基线

---

## 1. 项目配置

### 1.1 Cargo.toml

```toml
[package]
name = "mpack-launcher"
version = "0.1.0"
edition = "2021"

[dependencies]
# 核心库
mc-launcher-core = "0.1.2"

# 异步
tokio = { version = "1", features = ["full"] }
futures = "0.3"

# HTTP
reqwest = { version = "0.12", features = ["json", "rustls-tls", "stream"], default-features = false }

# 序列化
serde = { version = "1", features = ["derive"] }
serde_json = "1"

# CLI
clap = { version = "4", features = ["derive"] }

# 错误
thiserror = "1"
anyhow = "1"

# 哈希
sha1 = "0.10"
sha2 = "0.10"
md5 = "0.7"
hex = "0.4"

# 正则
regex = "1"

# 时间
chrono = "0.4"

# 压缩
zip = { version = "0.6", default-features = false, features = ["deflate"] }
flate2 = "1"

# 日志
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["env-filter"] }

# 系统信息
sysinfo = "0.30"

# 平台
dirs = "5"

# 加密存储（Windows DPAPI / macOS Keychain）
# 跨平台抽象：keyring crate
keyring = "2"

# 随机数
rand = "0.8"

[profile.release]
opt-level = "z"
lto = true
codegen-units = 1
panic = "abort"
strip = true
```

### 1.2 build.rs

```rust
use std::env;
use std::process::Command;

fn main() {
    // 版本信息注入
    let version = env::var("CARGO_PKG_VERSION").unwrap();
    let output = Command::new("git").args(["rev-parse", "--short", "HEAD"]).output();
    let commit = output.map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string()).unwrap_or_else(|_| "unknown".into());
    let build_time = chrono::Local::now().format("%Y-%m-%d %H:%M:%S").to_string();

    println!("cargo:rustc-env=MPACK_VERSION={}", version);
    println!("cargo:rustc-env=MPACK_COMMIT={}", commit);
    println!("cargo:rustc-env=MPACK_BUILD_TIME={}", build_time);
}
```

---

## 2. 下载子系统技术设计

### 2.1 并发下载器

**核心数据结构**：

```rust
pub struct Downloader {
    client: reqwest::Client,
    assets_semaphore: Semaphore,  // assets 32 并发
    libs_semaphore: Semaphore,    // libraries 16 并发
    mirror: Arc<MirrorManager>,
}

pub struct DownloadItem {
    pub url: String,
    pub dest: PathBuf,
    pub expected_sha1: Option<String>,
    pub size: Option<u64>,
}
```

**下载流程**：

```rust
pub async fn download_all(&self, items: Vec<DownloadItem>) -> Result<(), DownloadError> {
    let mut tasks = Vec::new();
    for item in items {
        // 跳过已存在且校验通过的文件
        if self.is_cached(&item).await? {
            continue;
        }
        // 根据文件类型选择对应 semaphore：assets 走 32 并发，libraries/client 走 16 并发
        let is_asset = item.url.contains("/assets/") || item.dest.starts_with("assets/");
        let permit = if is_asset {
            self.assets_semaphore.acquire().await?
        } else {
            self.libs_semaphore.acquire().await?
        };
        let client = self.client.clone();
        let mirror = self.mirror.clone();
        tasks.push(tokio::spawn(async move {
            let _permit = permit;
            Self::download_one(client, mirror, item).await
        }));
    }
    // 收集结果，任一失败不中断其他下载
    let results = futures::future::join_all(tasks).await;
    // 汇总错误
}
```

**单文件下载**：

```rust
async fn download_one(client, mirror, item) -> Result<()> {
    let url = mirror.rewrite(&item.url);
    // 检查断点续传
    let partial_path = item.dest.with_extension("partial");
    let offset = if partial_path.exists() {
        partial_path.metadata()?.len()
    } else { 0 };

    let mut request = client.get(&url);
    if offset > 0 {
        request = request.header("Range", format!("bytes={}-", offset));
    }
    let response = request.send().await?;

    // 流式下载 + 流式 SHA1
    let mut hasher = Sha1::new();
    let mut file = if offset > 0 {
        OpenOptions::new().append(true).open(&partial_path)?
    } else {
        File::create(&partial_path)?
    };

    let mut stream = response.bytes_stream();
    while let Some(chunk) = stream.next().await {
        let chunk = chunk?;
        hasher.update(&chunk);
        file.write_all(&chunk)?;
        // 进度上报（限流）
    }

    // 校验
    let actual = hex::encode(hasher.finalize());
    if let Some(expected) = item.expected_sha1 {
        if actual != expected {
            fs::remove_file(&partial_path)?;
            return Err(ChecksumMismatch);
        }
    }
    // 原子 rename
    fs::rename(&partial_path, &item.dest)?;
    Ok(())
}
```

### 2.2 性能优化

1. **连接池复用**：reqwest Client 内置连接池，clone 是廉价的（Arc 包装）
2. **流式哈希**：下载过程中计算 SHA1，避免下载完再读一遍文件
3. **并发分级**：assets 32 并发（小文件多）、libraries 16 并发（大文件）
4. **断点续传**：大文件支持 Range，中断后不从头开始
5. **跳过缓存**：下载前检查目标文件是否存在且 SHA1 匹配
6. **TCP_NODELAY**：reqwest 默认启用，减少小文件延迟
7. **DNS 缓存**：reqwest 内置 DNS 缓存

### 2.3 超时策略

| 超时类型 | 值 | 说明 |
|---|---|---|
| 连接超时 | 5s | TCP 连接 + TLS 握手 |
| 请求超时 | 30s | 整个请求（含下载） |
| 单文件最大时长 | 300s | 防止大文件卡死 |
| 重试间隔 | 1s/2s/4s | 指数退避 |
| 最大重试 | 3 次 | 超过则失败 |

---

## 3. Java 管理子系统技术设计

### 3.1 Java 版本检测

```rust
pub fn detect_java_version(path: &Path) -> Result<u32, JavaError> {
    let output = Command::new(path)
        .arg("-version")
        .output()?;
    // java -version 输出到 stderr
    let stderr = String::from_utf8_lossy(&output.stderr);
    parse_java_version(&stderr)
}

fn parse_java_version(s: &str) -> Result<u32, JavaError> {
    // 匹配: version "1.8.0_351" 或 version "17.0.9"
    let re = Regex::new(r#"version "(\d+)(?:\.(\d+))?"#)?;
    let caps = re.captures(s).ok_or(ParseError)?;
    let major: u32 = caps[1].parse()?;
    // Java 8 及以前格式为 1.8.x，主版本是 8
    if major == 1 {
        let minor: u32 = caps.get(2).ok_or(ParseError)?.as_str().parse()?;
        Ok(minor)
    } else {
        Ok(major)
    }
}
```

### 3.2 系统 Java 扫描

**扫描路径列表**（按平台）：

```rust
fn java_search_paths() -> Vec<PathBuf> {
    match OS {
        Windows => vec![
            r"C:\Program Files\Java",
            r"C:\Program Files\Eclipse Adoptium",
            r"C:\Program Files\Microsoft\jdk-*",
            r"C:\Program Files\Zulu",
            r"C:\Program Files\Amazon Corretto",
        ],
        Linux => vec![
            "/usr/lib/jvm",
            "/usr/java",
            "/opt/java",
            "/snap",
        ],
        MacOS => vec![
            "/Library/Java/JavaVirtualMachines",
            "/Library/Internet Plug-Ins/JavaAppletPlugin.plugin",
        ],
    }
}
```

扫描逻辑：遍历目录下的子目录，在 `bin/java`（Windows 是 `bin/java.exe`）处检测版本。

### 3.3 Java 自动下载

**Adoptium API**：

```
GET https://api.adoptium.net/v3/binary/latest/{version}/ga/{os}/{arch}/jdk/hotspot/normal/eclipse
```

参数：
- `version`: 8 / 17 / 21
- `os`: windows / linux / mac
- `arch`: x64 / aarch64

返回：302 重定向到下载链接。

**下载后处理**：
1. 下载到临时文件
2. 解压（zip 或 tar.gz）
3. 移动到 `<data_dir>/java/<version>/`
4. 执行 `java -version` 验证
5. 注册到 Java 注册表

---

## 4. 认证子系统技术设计

### 4.1 微软 OAuth Device Flow

```rust
pub async fn login_device_flow() -> Result<MicrosoftAccount, AuthError> {
    // 1. 请求 device code
    let device_resp = client.post("https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode")
        .form(&[("client_id", CLIENT_ID), ("scope", "XboxLive.signin offline_access")])
        .send().await?
        .json::<DeviceCodeResponse>().await?;

    // 输出 user_code 给用户
    output_device_code(&device_resp);

    // 2. 轮询 token
    let token = loop {
        sleep(Duration::from_secs(device_resp.interval)).await;
        let resp = client.post("https://login.microsoftonline.com/consumers/oauth2/v2.0/token")
            .form(&[
                ("grant_type", "urn:ietf:params:oauth:grant-type:device_code"),
                ("client_id", CLIENT_ID),
                ("device_code", &device_resp.device_code),
            ])
            .send().await?;
        if resp.status().is_success() {
            break resp.json::<TokenResponse>().await?;
        }
        let error = resp.json::<TokenError>().await?;
        match error.error.as_str() {
            "authorization_pending" => continue,
            "slow_down" => { sleep(Duration::from_secs(5)).await; continue; }
            _ => return Err(AuthFailed(error.error_description)),
        }
    };

    // 3. Xbox Live 认证
    let xbl = xbox_live_auth(&token.access_token).await?;
    // 4. XSTS 认证
    let xsts = xsts_auth(&xbl.token).await?;
    // 5. Minecraft 登录
    let mc = minecraft_login(&xsts.token, &xbl.user_hash).await?;
    // 6. 获取 Profile
    let profile = get_minecraft_profile(&mc.access_token).await?;

    Ok(MicrosoftAccount {
        username: profile.name,
        uuid: profile.id,
        access_token: mc.access_token,
        refresh_token: token.refresh_token,
        expires_at: now() + token.expires_in,
    })
}
```

### 4.2 Token 刷新

```rust
pub async fn refresh_token(refresh_token: &str) -> Result<TokenResponse, AuthError> {
    client.post("https://login.microsoftonline.com/consumers/oauth2/v2.0/token")
        .form(&[
            ("grant_type", "refresh_token"),
            ("client_id", CLIENT_ID),
            ("refresh_token", refresh_token),
            ("scope", "XboxLive.signin offline_access"),
        ])
        .send().await?
        .json().await
}
```

### 4.3 凭证加密存储

使用 `keyring` crate 跨平台抽象：

```rust
pub fn store_credentials(account: &MicrosoftAccount) -> Result<(), AuthError> {
    let entry = keyring::Entry::new("mpack-launcher", &account.uuid)?;
    let json = serde_json::to_string(account)?;
    entry.set_password(&json)?;
    Ok(())
}

pub fn load_credentials(uuid: &str) -> Result<MicrosoftAccount, AuthError> {
    let entry = keyring::Entry::new("mpack-launcher", uuid)?;
    let json = entry.get_password()?;
    Ok(serde_json::from_str(&json)?)
}
```

平台实现：
- Windows：DPAPI（密钥由用户账户保护）
- macOS：Keychain
- Linux：Secret Service（GNOME Keyring / KDE Wallet），无桌面环境时降级为加密文件

---

## 5. 启动命令构建技术设计

### 5.1 JVM 参数构建

```rust
pub fn build_jvm_args(
    java: &JavaRuntime,
    version: &VersionJson,
    options: &LaunchOptions,
    natives_dir: &Path,
) -> Vec<String> {
    let mut args = Vec::new();

    // 内存
    args.push(format!("-Xmx{}", options.xmx));
    args.push(format!("-Xms{}", options.xms));

    // 原生库路径
    args.push(format!("-Djava.library.path={}", natives_dir.display()));

    // 启动器标识
    args.push("-Dminecraft.launcher.brand=mPackLauncher".into());
    args.push(format!("-Dminecraft.launcher.version={}", MPACK_VERSION));

    // version JSON 中的 jvmArguments（1.17+ 有）
    if let Some(jvm_args) = &version.arguments.jvm {
        for arg in jvm_args {
            args.push(substitute(arg, &version, natives_dir));
        }
    }

    // 额外 JVM 参数
    if let Some(extra) = &options.jvm_args {
        args.extend(split_args(extra));
    }

    // classpath
    args.push("-cp".into());
    args.push(build_classpath(&version.libraries, &version.client_jar));

    // mainClass
    args.push(version.main_class.clone());

    args
}
```

### 5.2 游戏参数构建

```rust
pub fn build_game_args(
    version: &VersionJson,
    account: &Account,
    options: &LaunchOptions,
    game_dir: &Path,
    assets_dir: &Path,
) -> Vec<String> {
    let mut args = Vec::new();

    // version JSON 中的 game arguments（1.17+ 是数组，旧版是字符串模板）
    match &version.arguments.game {
        GameArgs::Array(list) => {
            for arg in list {
                args.push(substitute_game_arg(arg, account, options, game_dir, assets_dir));
            }
        }
        GameArgs::String(template) => {
            // 旧版：字符串模板，替换 ${auth_player_name} 等变量
            args.extend(substitute_legacy_template(template, account, options, game_dir, assets_dir));
        }
    }

    // 额外游戏参数
    if let Some(extra) = &options.game_args {
        args.extend(split_args(extra));
    }

    args
}
```

### 5.3 Classpath 构建

```rust
pub fn build_classpath(libraries: &[Library], client_jar: &Path) -> String {
    let mut paths: Vec<String> = libraries.iter()
        .filter(|lib| lib.rules_allow())  // 按 OS/arch rules 过滤
        .map(|lib| lib.path().to_string_lossy().to_string())
        .collect();
    paths.push(client_jar.to_string_lossy().to_string());
    // Windows 用分号分隔，Unix 用冒号
    paths.join(if cfg!(windows) { ";" } else { ":" })
}
```

### 5.4 内存自动分配

```rust
pub fn auto_xmx() -> String {
    let mut sys = sysinfo::System::new();
    sys.refresh_memory();
    let total_memory = sys.total_memory(); // KB
    let total_gb = total_memory / 1024 / 1024; // GB
    let xmx_gb = if total_gb < 4 {
        (total_gb * 3 / 4).max(1)  // <4GB: 75%
    } else {
        (total_gb / 2).min(4)       // >=4GB: 50%, 上限 4GB
    };
    format!("{}G", xmx_gb)
}
```

---

## 6. 镜像源技术设计

### 6.1 URL 重写（按文件类型分类）

```rust
pub fn rewrite_to_bmclapi(url: &str) -> String {
    if url.contains("piston-meta.mojang.com") {
        url.replace("https://piston-meta.mojang.com", "https://bmclapi.bangbang93.com")
    } else if url.contains("piston-data.mojang.com") {
        url.replace("https://piston-data.mojang.com", "https://bmclapi.bangbang93.com")
    } else if url.contains("libraries.minecraft.net") {
        url.replace("https://libraries.minecraft.net", "https://bmclapi.bangbang93.com/maven")
    } else if url.contains("resources.download.minecraft.net") {
        url.replace("https://resources.download.minecraft.net", "https://bmclapi.bangbang93.com/assets")
    } else {
        url.to_string()  // 第三方库不重写
    }
}
```

### 6.2 Auto 模式竞速下载

```rust
pub async fn download_with_race(&self, item: &DownloadItem) -> Result<()> {
    let mojang_url = item.url.clone();
    let bmclapi_url = rewrite_to_bmclapi(&item.url);

    match self.mirror {
        Mirror::Mojang => self.download_single(&mojang_url, item).await,
        Mirror::Bmclapi => self.download_single(&bmclapi_url, item).await,
        Mirror::Auto => {
            // 双源竞速：同时启动，谁先完成且校验通过用谁
            let mojang_task = self.download_single(&mojang_url, item);
            let bmclapi_task = self.download_single(&bmclapi_url, item);

            tokio::select! {
                result = mojang_task => {
                    if result.is_ok() {
                        tracing::debug!("Mojang source won the race");
                        result
                    } else {
                        // Mojang 失败，等 BMCLAPI
                        bmclapi_task.await
                    }
                }
                result = bmclapi_task => {
                    if result.is_ok() {
                        tracing::debug!("BMCLAPI source won the race");
                        result
                    } else {
                        // BMCLAPI 失败，等 Mojang
                        mojang_task.await
                    }
                }
            }
        }
    }
}
```

**超时配置**：
- 连接超时：5s
- 单文件下载超时：300s（大文件如 client.jar ~50MB）
- 竞速模式下，慢的源在快的源完成后被 abort

---

## 7. 输出协议技术设计

### 7.1 设计原则

启动器有两个独立的输出通道，严格分离：

| 通道 | 内容 | 格式 | 消费者 | 实现 |
|---|---|---|---|---|
| stderr | 调试日志 | 人类可读文本 | 开发者/日志文件 | tracing 框架 |
| stdout | 协议事件 | JSON Lines | Go 后端解析 | protocol.rs 模块 |

**不做的事**：不输出 per-file 进度、不输出百分比、不输出下载速度。只在流程进入新阶段和结束时输出事件。

### 7.2 事件类型

#### phase 事件（阶段变化）

流程进入新阶段时输出，告诉前端"当前在做什么"：

```json
{"type":"phase","phase":"downloading_assets","message":"正在下载资源文件"}
```

字段：
- `type`：固定为 `"phase"`
- `phase`：阶段标识（机器可读，如 `resolving_version`、`downloading_libraries`、`downloading_assets`、`installing_loader`、`launching`）
- `message`：人类可读的中文描述

#### result 事件（最终结果）

流程结束时输出，唯一一行 result 事件：

```json
{"type":"result","success":true,"data":{"version_id":"1.20.1","loader":"fabric"}}
```

失败时：

```json
{"type":"result","success":false,"error":"ChecksumMismatch","message":"文件校验失败：client.jar","suggestion":"网络不稳定，可尝试 --mirror bmclapi"}
```

### 7.3 Protocol 模块实现

```rust
// src/protocol.rs
use serde_json::{json, Value};
use std::io::{self, Write};

pub struct Protocol;

impl Protocol {
    /// 输出阶段变化事件
    pub fn phase(phase: &str, message: &str) {
        let event = json!({
            "type": "phase",
            "phase": phase,
            "message": message,
        });
        Self::emit(&event);
    }

    /// 输出成功结果
    pub fn success(data: Value) {
        let event = json!({
            "type": "result",
            "success": true,
            "data": data,
        });
        Self::emit(&event);
    }

    /// 输出失败结果
    pub fn failure(error: &str, message: &str, suggestion: Option<&str>) {
        let mut event = json!({
            "type": "result",
            "success": false,
            "error": error,
            "message": message,
        });
        if let Some(s) = suggestion {
            event["suggestion"] = json!(s);
        }
        Self::emit(&event);
    }

    fn emit(event: &Value) {
        let stdout = io::stdout();
        let mut lock = stdout.lock();
        // unwrap 安全：stdout 写入失败意味着进程已无法继续
        writeln!(lock, "{}", serde_json::to_string(event).unwrap()).unwrap();
    }
}
```

**线程安全**：`stdout.lock()` 是互斥锁，保证多行输出不交错。phase 事件只在流程进入新阶段时触发，不存在高并发竞争。

### 7.4 阶段标识定义

| phase | 触发时机 | message 示例 |
|---|---|---|
| `resolving_version` | 开始解析版本 manifest | "正在解析版本信息" |
| `downloading_libraries` | 开始下载支持库 | "正在下载支持库" |
| `downloading_assets` | 开始下载资源文件 | "正在下载资源文件" |
| `installing_loader` | 开始安装加载器（Forge/Fabric） | "正在安装 Fabric" |
| `verifying` | 开始校验文件 | "正在校验文件" |
| `preparing` | 启动前准备 | "正在准备启动" |
| `launching` | 正在 spawn 游戏进程 | "正在启动游戏" |

### 7.5 日志（tracing）配置

```rust
// main.rs 中初始化
tracing_subscriber::fmt()
    .with_writer(std::io::stderr)  // 日志走 stderr
    .with_env_filter("MPACK_LOG")  // 环境变量控制级别
    .init();
```

代码中使用：
```rust
tracing::info!("开始安装版本 {}", version);  // 日志 → stderr
Protocol::phase("downloading_assets", "正在下载资源文件");  // 事件 → stdout
```

---

## 8. 错误处理技术设计

### 8.1 错误类型层次

```rust
#[derive(thiserror::Error, Debug)]
pub enum LauncherError {
    // 参数错误
    #[error("参数错误: {0}")]
    InvalidArgument(String),

    // 网络错误
    #[error("网络错误: {0}")]
    Network(#[from] reqwest::Error),

    // 下载错误
    #[error("下载失败: {url}（已重试 {attempts} 次）")]
    DownloadFailed { url: String, attempts: u32 },

    // 校验错误
    #[error("校验失败: {file}（期望 {expected}，实际 {actual}）")]
    ChecksumMismatch { file: String, expected: String, actual: String },

    // 版本错误
    #[error("版本不存在: {0}")]
    VersionNotFound(String),
    #[error("加载器不兼容: {loader} 不支持 Minecraft {mc_version}")]
    LoaderIncompatible { loader: String, mc_version: String },

    // Java 错误
    #[error("未找到 Java 运行时（需要 Java {required}）")]
    JavaNotFound { required: String },
    #[error("Java 版本不匹配（需要 {required}，实际 {found}）")]
    JavaVersionMismatch { required: String, found: String },

    // 认证错误
    #[error("认证失败: {0}")]
    AuthFailed(String),

    // 游戏错误
    #[error("游戏崩溃（退出码 {code}）")]
    GameCrashed { code: i32, last_log: Vec<String> },

    // IO 错误
    #[error("IO 错误: {0}")]
    Io(#[from] std::io::Error),

    // JSON 错误
    #[error("JSON 错误: {0}")]
    Json(#[from] serde_json::Error),
}
```

### 8.2 退出码映射

```rust
impl LauncherError {
    pub fn exit_code(&self) -> i32 {
        match self {
            Self::InvalidArgument(_) => 1,
            Self::Network(_) | Self::DownloadFailed { .. } => 2,
            Self::ChecksumMismatch { .. } => 3,
            Self::VersionNotFound(_) | Self::LoaderIncompatible { .. } => 4,
            Self::JavaNotFound { .. } | Self::JavaVersionMismatch { .. } => 5,
            Self::AuthFailed(_) => 6,
            Self::GameCrashed { .. } => 10,
            _ => 1,
        }
    }

    pub fn suggestion(&self) -> Option<String> {
        match self {
            Self::DownloadFailed { .. } => Some("网络不稳定，可尝试 --mirror bmclapi 使用国内镜像".into()),
            Self::JavaNotFound { required } => Some(format!("可运行 `mpack-launcher java install --version {}` 自动安装", required)),
            Self::JavaVersionMismatch { .. } => Some("请安装对应版本的 Java，或使用 --java 指定正确的路径".into()),
            Self::AuthFailed(_) => Some("请重新运行 `mpack-launcher auth login` 登录".into()),
            Self::GameCrashed { code, .. } => {
                if *code == 1 { Some("可能是内存不足，尝试增大 --xmx 参数".into()) }
                else { None }
            }
            _ => None,
        }
    }
}
```

### 8.3 崩溃日志收集

`--wait` 模式下游戏崩溃时，收集最后 50 行日志：

```rust
fn tail_log(file: &Path, lines: usize) -> Vec<String> {
    let content = fs::read_to_string(file).unwrap_or_default();
    content.lines().rev().take(lines).collect::<Vec<_>>().into_iter().rev().map(String::from).collect()
}
```

---

## 9. 跨平台技术设计

### 9.1 路径处理

- 所有内部路径使用 `PathBuf`，输出时调用 `.display()`
- Windows 路径分隔符 `\`，Unix `/`，classpath 分隔符 Windows `;` / Unix `:`
- 路径规范化：`canonicalize()` 解析符号链接和 `..`

### 9.2 进程管理

- Windows：`std::process::Command` 自动处理 `.exe` 后缀
- 信号处理：Ctrl+C 时优雅终止（终止下载，清理临时文件）
- 游戏进程 detach：spawn 后不持有句柄，游戏独立运行

### 9.3 原生库解压

- natives 是 jar 格式（zip），包含 dll/so/dylib
- 使用 `zip` crate 解压到 `natives/<version_id>/`
- 按平台过滤：Windows 只解压 `.dll`，Linux `.so`，macOS `.dylib`

---

## 10. 性能优化技术清单

| 优化项 | 技术手段 | 预期收益 |
|---|---|---|
| 并发下载 | tokio + Semaphore | 下载时间减少 60-80% |
| 流式 SHA1 | 下载时边下边算 | 避免二次读取，大文件省 30% IO |
| 断点续传 | HTTP Range | 中断后零重复 |
| 连接池复用 | reqwest Client | 减少 TCP/TLS 握手 |
| 跳过缓存 | 下载前 SHA1 检查 | 重复 install 零下载 |
| 内存映射 | 大文件校验可用 mmap | 减少内存拷贝 |
| LTO | 链接时优化 | binary 体积减小 20% |
| panic=abort | 取消展开机制 | binary 体积减小 10% |
| strip | 去除符号表 | binary 体积减小 50% |
| opt-level=z | 最小体积优化 | binary 体积减小 15% |

---

## 11. 关键算法

### 11.1 离线 UUID 生成

```rust
pub fn offline_uuid(username: &str) -> String {
    let data = format!("OfflinePlayer:{}", username);
    let hash = md5::compute(data.as_bytes());
    // 设置 version 3 和 variant 位
    let mut bytes = hash.0;
    bytes[6] = (bytes[6] & 0x0f) | 0x30;  // version 3
    bytes[8] = (bytes[8] & 0x3f) | 0x80;  // variant RFC 4122
    format!("{:02x}{:02x}{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}{:02x}{:02x}{:02x}{:02x}",
        bytes[0],bytes[1],bytes[2],bytes[3],
        bytes[4],bytes[5],bytes[6],bytes[7],
        bytes[8],bytes[9],bytes[10],bytes[11],
        bytes[12],bytes[13],bytes[14],bytes[15])
}
```

### 11.2 版本号比较

用于判断 Minecraft 版本对应的 Java 要求：

```rust
pub fn mc_version_to_java(mc_version: &str) -> u32 {
    let parts: Vec<u32> = mc_version.split('.').filter_map(|s| s.parse().ok()).collect();
    match parts.as_slice() {
        [1, minor, ..] if *minor <= 16 => 8,
        // 1.20.5+ 必须在 minor<=20 之前判断，否则 1.20.5 会被误匹配到 17
        [1, 20, patch, ..] if *patch >= 5 => 21,
        [1, minor, ..] if *minor <= 20 => 17,  // 1.17 ~ 1.20.4
        [1, minor, ..] if *minor >= 21 => 21,
        _ => 17, // 默认
    }
}
```

---

## 12. 构建与发布技术

### 12.1 交叉编译

```bash
# Windows (宿主)
cargo build --release --target x86_64-pc-windows-msvc

# Linux (需安装 target)
rustup target add x86_64-unknown-linux-gnu
cargo build --release --target x86_64-unknown-linux-gnu

# macOS (需对应 SDK，仅在 macOS 宿主上可靠)
rustup target add aarch64-apple-darwin x86_64-apple-darwin
cargo build --release --target aarch64-apple-darwin
```

### 12.2 构建脚本

`scripts/build-launcher.ps1`：
1. 检查 `.tools/rust` 是否存在，不存在则下载便携 Rust
2. 设置 PATH 指向便携 Rust
3. 执行 `cargo build --release`
4.  strip（Linux/macOS）
5. 复制到 `dist/mpack-launcher/`
6. 输出 binary 大小和版本信息
