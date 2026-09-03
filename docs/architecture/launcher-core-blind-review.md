# mPackLauncher 启动器内核 — 独立盲审报告

> 评审日期：2026-09-03
> 评审身份：独立技术文档盲审员（无项目背景、无作者意图假设）
> 评审范围：launcher-core-spec / architecture / domain / technical / plan / test-plan 共 6 份文档
> 评审方法：逐份完整阅读后交叉比对，仅基于文档内信息判断

---

## 总体评分：5.5 / 10

文档覆盖面广、结构清晰，核心思路（独立 Rust binary + CLI + JSON Lines + mc-launcher-core 封装）是合理的。但存在**多处文档间直接矛盾**（尤其是镜像 auto 模式、进度协议、模块结构）、**若干代码示例无法编译**（build.rs 依赖、断点续传 SHA1 逻辑）、以及**声明了但未设计的功能**（文件锁、磁盘预检、list 实现、非 JSON 输出模式）。当前状态不建议直接进入实现，需先消解高优先级矛盾并补齐关键设计。

---

## 一、按维度列出的问题

### 1. 完整性

| # | 严重程度 | 问题 |
|---|---|---|
| C-1 | **高** | `install` 编排模块缺失。spec §3 目录树和 architecture §1.1 目录树中均无顶层 `install.rs`，但 architecture §2.2.1 定义了 `install.rs` 的 `execute()` 函数和状态机，plan M0-6/M1-3/M2-1~3 也以 `src/install.rs` 为交付物。该模块的职责边界（与 `download/`、`loader/` 的关系）未在目录结构中体现。 |
| C-2 | **高** | 非 JSON 输出模式未设计。spec §4.1 声明"默认人类可读文本"，但 protocol.rs（technical §7.3）只输出 JSON，全文档无任何人类可读输出模块的设计。plan M0-4 提到 `output.rs` 但未说明其职责。`--json` 标志的开/关两种路径只有一种被设计。 |
| C-3 | **高** | 并发安装文件锁（`<dir>/.mpack-install.lock`）在 spec §9 声明，但无任何模块设计、无 API、无测试用例。 |
| C-4 | **高** | 磁盘空间预检在 spec §10 声明（"安装前预检可用空间"），test-plan INT-E02 要求测试，但无任何模块设计、无算法、无阈值定义。 |
| C-5 | **中** | `list` 命令（spec §4.3.5）输出 `installed_at`（毫秒时间戳）和 `size_bytes`，但版本 JSON 中不存储安装时间，目录结构中无元数据文件，这两个字段的来源未设计。 |
| C-6 | 中 | `uninstall` 未实现。domain §2.2 版本状态机包含 `uninstall` 转移，但 CLI 契约（spec §4.3）无此子命令，无任何模块设计。状态机与接口契约不匹配。 |
| C-7 | 中 | 微软 OAuth `CLIENT_ID` 未定义来源。technical §4.1/4.2 代码引用 `CLIENT_ID` 常量，但全文档未说明该值从何而来（硬编码？环境变量？配置文件？），也未说明微软应用注册情况。这是功能能否运行的前提。 |
| C-8 | 中 | 多微软账号场景未设计。spec §4.3.2 launch 仅支持 `--account-type microsoft`，无 `--account-id` 或选择机制；当缓存中有多个微软账号时行为未定义。 |
| C-9 | 中 | `--json` 模式下未指定 `--log-file` 的行为未定义。spec §5.2 说"`--json` 模式下必须指定 `--log-file`"，但未说明不指定时是报错、警告还是自动生成路径。 |
| C-10 | 低 | `--width`/`--height`/`--demo`/`--server` 在 spec §4.3.2 声明为 launch 参数，但 technical §5.2 `build_game_args` 代码示例中均未体现。 |
| C-11 | 低 | `--concurrency` 参数（spec §4.3.1）声明可配置 assets 并发数，但 technical §2.1 `Downloader` 的两个 Semaphore 是硬编码的，无参数传入路径。 |
| C-12 | 低 | mc-launcher-core 版本信息注入。spec §7.4 要求 `version` 命令输出 mc-launcher-core 版本，但 technical §1.2 build.rs 未注入该值。 |

### 2. 一致性

| # | 严重程度 | 矛盾点 | 涉及文档 |
|---|---|---|---|
| X-1 | **高** | **镜像 auto 模式策略根本矛盾**。spec §5.5 描述为"每个文件同时启动官方源和 BMCLAPI 两个下载任务（竞速），哪个先完成且校验通过就用哪个"；architecture §2.4.1、domain §7.3、technical §6.1 均描述为"失败计数阈值（连续 3 次失败后全局切换 BMCLAPI）"。这是两种完全不同的实现策略，无法同时成立。 | spec vs architecture/domain/technical |
| X-2 | **高** | **进度协议矛盾**。spec §5.1 明确"仅在流程进入新阶段时输出 phase 事件，不做 per-file 进度上报，不输出百分比、下载速度、字节数"；technical §7.1 重申"不做 per-file 进度"。但 domain §9 定义了完整的 Progress 领域模型，包含 `bytes_downloaded`/`bytes_total`/`speed_bps`、"每个文件开始/完成时必发"、"大文件每 10% 发一次"。两者直接冲突。 | spec/technical vs domain |
| X-3 | **高** | **模块结构矛盾**。spec §3 和 architecture §1.1 定义为子目录结构（`download/`、`loader/`、`launch/`、`java/`、`auth/`）；plan M0~M4 交付物全部为扁平文件（`src/install.rs`、`src/java.rs`、`src/launch.rs`、`src/auth.rs`、`src/mirror.rs`、`src/cache.rs`）；test-plan §1.3 覆盖率表也按扁平文件命名。三套结构互不兼容。 | spec/architecture vs plan/test-plan |
| X-4 | **高** | **协议模块命名矛盾**。spec/architecture/technical/test-plan 均使用 `protocol.rs`；plan M0-4 交付物为 `src/output.rs` + `src/progress.rs`。 | plan vs 其余全部 |
| X-5 | **高** | **凭证存储方式矛盾**。spec §5.3 和 architecture §4.1 描述为文件存储（`<data_dir>/auth/credentials.json`，加密）；technical §4.3 使用 `keyring` crate（Windows DPAPI / macOS Keychain / Linux Secret Service），在 Windows/macOS 上根本不写文件。domain §5.3 又描述为"Windows DPAPI 加密后写入文件"（DPAPI 加密后写文件与 keyring 的 DPAPI 凭证存储是两种实现）。 | spec/architecture vs technical vs domain |
| X-6 | **高** | **Linux 认证文件路径矛盾**。spec §5.3 写 `~/.config/mpack-launcher/auth.json`；spec §6.2 和 architecture §4.1 写 `<data_dir>/auth/credentials.json`（Linux data_dir 为 `~/.local/share/mpack-launcher/`）。同一文档内两个不同路径。 | spec §5.3 vs spec §6.2 |
| X-7 | **中** | **校验时机矛盾**。spec §5.1 说"全部下载完成后统一校验，校验失败的文件重新下载"；technical §2.1 `download_one` 在下载过程中流式计算 SHA1 并在完成时立即校验；domain §6.5 说"下载过程中流式计算 + 安装时再次校验"。spec 的"统一校验"与其他文档的"逐文件即时校验"不一致。 | spec vs technical/domain |
| X-8 | **中** | **错误结构体字段矛盾**。spec §5.6 `LauncherError::GameCrashed { code: i32 }`；technical §8.1 为 `GameCrashed { code: i32, last_log: Vec<String> }`。 | spec vs technical |
| X-9 | **中** | **失败结果字段矛盾**。spec §5.7 `ResultEvent` 失败时字段为 `error`/`message`/`suggestion`；spec §4.3.2 游戏崩溃示例使用 `error_code`（而非 `error`）且多出 `details` 对象（含 exit_code/pid/duration_ms/last_log_lines），`ResultEvent` 定义中无 `details` 字段。 | spec §5.7 vs spec §4.3.2 |
| X-10 | **中** | **Quilt 是否为阻塞验收矛盾**。spec §11.2 将"Quilt 1.20.1"列为"兼容性测试（记录结果，不阻塞发布）"；spec §12 M2 验收门槛要求"三种加载器均能安装并启动"（含 Quilt）；plan M2 验收门槛明确要求"Quilt 1.20.1 安装成功并能启动到主菜单"。 | spec §11.2 vs spec §12/plan M2 |
| X-11 | **中** | **Phase 枚举与阶段标识不一致**。domain §9.1 定义 `Phase` 枚举为 `Resolve/JavaSetup/Download/Verify/InstallLoader/ExtractNatives`（PascalCase，6 个值）；spec §5.1 和 technical §7.4 定义阶段标识为 snake_case 字符串 `resolving_version/downloading_libraries/downloading_assets/installing_loader/verifying/preparing/launching`（7 个值）。名称、数量、命名风格均不同。 | domain vs spec/technical |
| X-12 | **中** | **install 状态机含 JavaSetup 状态但无对应 phase 事件**。architecture §2.2.1 状态机有 `JavaSetup`，但 phase 事件列表（spec §5.1、technical §7.4）中无 `java_setup` 阶段。 | architecture vs spec/technical |
| X-13 | 低 | **离线 UUID 算法描述不一致**。architecture §1.1 写"UUID v3 生成"；spec §5.3 写"MD5 哈希生成"；technical §11.1 实现为 MD5 + 设置 version 3 位（即 UUID v3）。三者本质一致但表述不统一，易造成误解。 | architecture vs spec |
| X-14 | 低 | **退出码 130 未实现**。spec §4.2 定义退出码 130（用户中断），但 technical §8.2 `exit_code()` 无 130 映射，architecture §2.1 信号处理未提及退出码。 | spec vs technical/architecture |

### 3. 技术可行性

| # | 严重程度 | 问题 |
|---|---|---|
| F-1 | **高** | **build.rs 无法编译**。technical §1.2 build.rs 使用 `chrono::Local::now()`，但 technical §1.1 Cargo.toml 中 `chrono` 仅在 `[dependencies]` 中，未在 `[build-dependencies]` 中声明。build.rs 编译时无法访问 `[dependencies]`，会报 `unresolved import`。 |
| F-2 | **高** | **断点续传 SHA1 计算错误**。technical §2.1 `download_one` 中，当 offset > 0（断点续传）时，`hasher` 从 0 开始只对新下载的 chunk 计算 SHA1，未对 `.partial` 文件中已有的字节进行哈希。最终 `actual` 仅覆盖续传部分，与期望的全文件 SHA1 必然不匹配，导致所有断点续传文件校验失败。 |
| F-3 | **高** | **`.partial` 文件名生成错误**。technical §2.1 使用 `item.dest.with_extension("partial")`，对于 `1.20.1.jar` 会生成 `1.20.1.partial`（丢失 `.jar`），而非 `1.20.1.jar.partial`。这会导致同一目录下 `1.20.1.jar` 和 `1.20.1.json` 的临时文件均名为 `1.20.1.partial`，互相覆盖。 |
| F-4 | **高** | **请求超时 30s 与单文件最大时长 300s 矛盾**。technical §2.3 表中"请求超时 30s（整个请求含下载）"和"单文件最大时长 300s"同时存在。若 reqwest 的总超时设为 30s，则任何下载超过 30s 的文件（如 client.jar 在慢网络下）都会被中断，300s 上限形同虚设。 |
| F-5 | **中** | **Windows 宿主交叉编译 Linux 不可行**。spec §7.3 声称"Windows 宿主可构建 Linux 版本"，目标为 `x86_64-unknown-linux-gnu`。该 target 需要 GNU 链接器（ld）和 glibc 头文件，Windows 原生不具备，需 WSL 或交叉工具链。technical §12.1 仅说"需安装 target"，未提及链接器依赖。 |
| F-6 | **中** | **binary < 10MB 目标存疑**。Cargo.toml 依赖包含 tokio(full)、reqwest(rustls)、clap(derive)、zip、keyring、sysinfo、chrono、regex 等。tokio full + reqwest + rustls 通常已占 5-8MB，加上 clap derive、zip、keyring（拖入 secret-service 等），strip 后接近或超过 10MB。且 `tokio = { features = ["full"] }` 引入了大量不需要的模块（fs、io-util、sync 全量），与最小体积目标矛盾。 |
| F-7 | **中** | **mc-launcher-core 0.1.2 API 假设未验证**。architecture §2.3 列出 `Launcher::new(dir)`、`launcher.install(InstallRequest)`、`launcher.load_version(id)`、`launcher.build_launch_command_from_version()` 等 API 名称，均为假设。plan M-1 虽安排验证，但当前设计文档大量依赖这些 API，若名称/签名不符将导致大面积返工。 |
| F-8 | **中** | **launch 编排为 sync 但依赖 async 操作**。architecture §2.2.2 `launch::execute` 签名为 `pub fn execute(...)`（同步），但启动流程需要 auth token 刷新（异步 HTTP）和 Java 检测/下载（可能异步）。同步函数中无法直接 await，需在内部创建运行时或改为 async，设计未说明。 |
| F-9 | 低 | **`md5 = "0.7"` 版本过旧**。technical §1.1 使用 md5 0.7（2020 年发布），虽可编译但存在已知维护问题，建议评估是否使用 `md-5`（RustCrypto 维护）。 |
| F-10 | 低 | **`is_cached` 对无 SHA1 的文件无法判断**。technical §2.1 `DownloadItem.expected_sha1` 为 `Option<String>`，`is_cached` 若遇到 None 则无法校验文件完整性，可能跳过损坏文件。 |

### 4. 架构合理性

| # | 严重程度 | 问题 |
|---|---|---|
| A-1 | **高** | 编排层与能力层的边界模糊。architecture §2.2 引入 `install.rs`/`launch.rs`/`auth.rs`/`java.rs` 四个编排文件，但 §1.1 目录树中已有 `launch/`、`java/`、`auth/` 三个子目录（内含 mod.rs 作为"公开 API"）。`launch.rs` 与 `launch/mod.rs`、`java.rs` 与 `java/mod.rs`、`auth.rs` 与 `auth/mod.rs` 的职责如何划分未说明，极易导致重复实现或循环依赖。 |
| A-2 | 中 | `download/` 模块同时承担下载编排和镜像管理、缓存、并发调度，职责过重。mirror.rs 被放在 download/ 下，但镜像管理也被 java/install 模块使用（Java 下载、Forge installer 下载），跨模块依赖方向不清晰。 |
| A-3 | 中 | mc-launcher-core 封装层缺失。architecture §2.3 声明"不直接暴露库类型到 CLI 层，在编排层做适配"，但无独立的 adapter/facade 模块设计。若库 API 变更，将波及所有编排模块。 |
| A-4 | 低 | `platform.rs` 承担 OS 检测、内存检测、路径规范化、数据目录定位、Java 路径枚举五项职责，建议拆分或至少明确子函数边界。 |

### 5. 安全与健壮性

| # | 严重程度 | 问题 |
|---|---|---|
| S-1 | **高** | **Forge processor 执行的安全边界不足**。spec §9 承认"不执行下载的任意代码（Forge processors 除外）"，但仅靠"校验 installer SHA1 + 超时控制"不足以防御 processor 中的恶意/缺陷代码。processor 以当前用户权限执行任意 Java 代码，可读写文件系统、访问网络。文档未设计沙箱、工作目录隔离、环境变量清理、网络限制等措施。 |
| S-2 | **高** | **路径穿越防护未设计**。spec §9 声明"`--dir` 和 `--log-file` 参数经过路径规范化和安全校验，禁止目录穿越"，但全文档无具体校验算法（如 `canonicalize` 后检查前缀、拒绝符号链接出界等），无测试用例。 |
| S-3 | 中 | **错误信息脱敏规则不完整**。spec §9 要求"不泄漏完整绝对路径"，但 `LauncherError::ChecksumMismatch { file: String }` 和 `DownloadFailed { url: String }` 直接包含路径/URL，`suggestion()` 中也包含路径。technical §8.1 未做任何脱敏处理。 |
| S-4 | 中 | **微软 token 在 `auth login` 成功结果中输出**。spec §4.3.3 示例输出 `"access_token":"***"`（已脱敏），但 technical §4.1 `login_device_flow` 返回含明文 `access_token` 的 `MicrosoftAccount`，输出层是否脱敏未设计。test-plan AUTH-05 仅测试存储往返，未测试输出脱敏。 |
| S-5 | 中 | **`--log-file` 路径可指向系统关键文件**。若用户传入 `--log-file /etc/passwd`（Linux）或 `C:\Windows\System32\...`，启动器会以当前权限覆盖该文件。文档未限制 log-file 必须在实例目录或数据目录内。 |
| S-6 | 低 | **`panic = "abort"` 下 stdout 写入失败直接终止**。technical §7.3 `Protocol::emit` 中 `writeln!(...).unwrap()`，若 Go 侧提前关闭管道（如用户取消任务），stdout 写入失败触发 panic → abort，可能留下 `.partial` 临时文件和未释放的文件锁。 |
| S-7 | 低 | **Linux 无桌面环境下 keyring 降级方案未设计**。technical §4.3 提到"无桌面环境时降级为加密文件"，但加密算法、密钥派生、文件格式均未定义。 |

### 6. 测试覆盖

| # | 严重程度 | 问题 |
|---|---|---|
| T-1 | **高** | **断点续传 SHA1 正确性无测试**。technical §2.1 的续传哈希 bug（F-2）在测试方案中无对应用例。test-plan §3.2 INT-V06 仅验证"kill 后恢复，不重复已下载"，未校验恢复后文件的 SHA1 正确性。 |
| T-2 | **高** | **路径穿越安全测试缺失**。test-plan 无任何用例测试 `--dir`/`--log-file` 的目录穿越防护。 |
| T-3 | **高** | **文件锁并发测试缺失**。spec §9 声明并发安装文件锁，但 test-plan 无"两个进程同时 install 同一目录"的测试用例。 |
| T-4 | 中 | **磁盘空间预检测试（INT-E02）依赖未设计的功能**。见 C-4。 |
| T-5 | 中 | **非 JSON 输出模式无测试**。test-plan 所有协议测试（PROT-01~06）均针对 JSON 模式，人类可读输出模式零覆盖。 |
| T-6 | 中 | **退出码 130（Ctrl+C）无测试**。spec §4.2 定义该退出码，但 test-plan 无信号处理测试。 |
| T-7 | 中 | **`list` 命令功能测试缺失**。CLI-12 仅测试参数解析，无"扫描 versions/ 目录并输出版本列表"的功能测试。 |
| T-8 | 中 | **性能指标"空闲内存 < 20MB"测量方法错误**。test-plan §4.1 定义"空闲内存"的测试方法为"安装过程中峰值 RSS"，这测量的是峰值而非空闲。 |
| T-9 | 中 | **启动测试自动化方式不明确**。test-plan §3.4 注说"使用 `--game-args '--demo'` 或启动后 5 秒自动 kill"，但 `--demo` 不会自动退出，仍需人工关闭窗口；5 秒 kill 无法验证游戏是否真正启动到主菜单。 |
| T-10 | 低 | **`java remove` 无单元测试**，仅 INT-J06 集成测试覆盖。 |
| T-11 | 低 | **镜像竞速模式（若按 spec 实现）无测试**。test-plan MIR-06~09 均基于失败计数阈值模型，与 spec 的竞速模型不匹配（见 X-1）。 |

---

## 二、文档间矛盾点汇总（按严重程度排序）

| 优先级 | 矛盾 | 文档 A | 文档 B | 建议裁决 |
|---|---|---|---|---|
| P0 | auto 镜像模式：竞速 vs 失败计数切换 | spec §5.5 | architecture §2.4.1 / domain §7.3 / technical §6.1 | 二选一。竞速模式实现复杂（双任务取消）且浪费带宽，建议采用失败计数阈值模型，修正 spec。 |
| P0 | 进度协议：phase-only vs per-file 字节级进度 | spec §5.1 / technical §7.1 | domain §9 | 建议采用 phase-only（与 spec 一致），删除 domain §9 的 Progress 领域模型或降级为内部统计不输出。 |
| P0 | 模块结构：子目录 vs 扁平文件 | spec §3 / architecture §1.1 | plan M0~M4 / test-plan §1.3 | 统一为一种。建议保留子目录结构（扩展性好），修正 plan 和 test-plan 的文件路径。 |
| P0 | 凭证存储：文件 vs keyring | spec §5.3/§6.2 / architecture §4.1 | technical §4.3 | 建议统一为 keyring（技术更安全），修正 spec/architecture/domain 的存储描述，并补充 Linux 无桌面降级方案。 |
| P1 | 协议模块名：protocol.rs vs output.rs+progress.rs | spec/architecture/technical/test-plan | plan M0-4 | 统一为 protocol.rs，删除 plan 中的 output.rs/progress.rs。 |
| P1 | 校验时机：统一后校验 vs 逐文件即时校验 | spec §5.1 | technical §2.1 / domain §6.5 | 建议逐文件即时校验（性能更优），修正 spec 描述。 |
| P1 | GameCrashed 结构体字段 | spec §5.6 | technical §8.1 | 统一为含 last_log 的版本（technical 更完整），修正 spec。 |
| P1 | 失败结果字段：error vs error_code + details | spec §5.7 | spec §4.3.2 | 统一 ResultEvent 定义，建议增加 `details` 字段，统一使用 `error`。 |
| P1 | Quilt 是否阻塞验收 | spec §11.2 | spec §12 / plan M2 | 明确 Quilt 为 M2 阻塞项（与 plan 一致），从兼容性测试列表移除或标注。 |
| P2 | Phase 枚举命名/数量 | domain §9.1 | spec §5.1 / technical §7.4 | 统一为 snake_case 字符串，删除 domain 中的 PascalCase 枚举或映射说明。 |
| P2 | Linux 认证路径：~/.config vs ~/.local/share | spec §5.3 | spec §6.2 / architecture §4.1 | 统一为 data_dir 下的路径。 |
| P2 | 退出码 130 | spec §4.2 | technical §8.2 | 在 exit_code() 中补充信号处理映射。 |

---

## 三、最值得改进的 3 件事

### 1. 消解 P0 级矛盾，统一设计基线（X-1 ~ X-4）

当前 6 份文档在最核心的四个设计点上互相矛盾：镜像策略、进度协议、模块结构、凭证存储。这些矛盾不解决，实现阶段将频繁返工，且不同开发者可能按不同文档实现出不兼容的代码。建议召开一次设计裁决会议，逐项确定唯一方案后同步更新所有文档。

### 2. 修复技术设计中的编译/逻辑错误（F-1 ~ F-4）

build.rs 的 chrono 依赖问题、断点续传的 SHA1 计算错误、`.partial` 文件命名错误、超时配置矛盾，这四个问题如果直接按文档实现，代码要么编译不过，要么核心功能（断点续传）完全失效。建议在进入实现前修正 technical.md 的代码示例，并补充对应的单元测试设计。

### 3. 补齐声明但未设计的功能模块（C-1 ~ C-4）

install 编排模块、非 JSON 输出模式、文件锁、磁盘空间预检——这四项在 spec 中被声明为产品能力，但没有任何模块设计、API 定义或测试用例。尤其是 install 编排模块是整个安装流程的中枢，缺失它意味着 M0 里程碑的交付物定义不完整。建议在 spec 和 architecture 中补充这四个模块的最小设计。

---

## 四、结论

**暂不建议进入实现阶段。**

理由：
1. 存在 4 个 P0 级文档间矛盾（镜像策略、进度协议、模块结构、凭证存储），实现者无法确定应遵循哪份文档；
2. 存在 4 个高严重度技术可行性问题（build.rs 编译失败、断点续传 SHA1 错误、临时文件命名冲突、超时配置矛盾），按现有代码示例实现会导致核心功能失效；
3. 存在 4 个高严重度完整性缺失（install 编排模块、非 JSON 输出、文件锁、磁盘预检），M0 里程碑交付物定义不完整。

**建议行动**：先完成上述"最值得改进的 3 件事"，更新全部 6 份文档至一致状态，然后进行第二轮盲审。第二轮通过后可进入 M-1（技术预研）阶段，M-1 的核心目标正是验证 mc-launcher-core 的真实 API，这也能消解 F-7 的不确定性。

预计修复工作量：文档修订 1-2 天，无需代码变更。
