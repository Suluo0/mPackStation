# mPackLauncher 进度计划

> 关联文档：[launcher-core-spec.md](launcher-core-spec.md)（接口契约）、[launcher-core-architecture.md](launcher-core-architecture.md)（架构设计）
> 状态：计划基线
> 说明：本计划不设工期预估，以里程碑验收门槛为完成标准，按依赖顺序推进。

---

## 1. 总览

| 项 | 值 |
|---|---|
| 里程碑 | M-1 ~ M5，共 7 个 |
| 关键路径 | M-1 → M0 → M1 → M2 → M3 → M4 → M5 |
| 推进方式 | 前一里程碑验收通过后才进入下一里程碑 |
| 人力 | 1 人 |

---

## 2. 里程碑拆解

### M-1：技术预研（mc-launcher-core 能力验证）

**目标**：验证 mc-launcher-core 0.1.2 的真实能力，确认核心假设成立。

**任务**：
- 引入 mc-launcher-core，编译通过
- Vanilla 安装跑通（下载 version manifest、client.jar、libraries、assets）
- Vanilla 离线启动跑通（构建命令、spawn 进程、游戏窗口出现）
- Fabric 安装跑通（Meta API、JSON 合并）
- Forge 安装跑通（installer 下载、processor 执行）—— 验证最大风险点
- 记录库的 API 与文档假设的差异

**验收门槛**：
- 三种安装（Vanilla/Fabric/Forge）均能成功，无阻塞性缺陷
- 离线启动游戏到主菜单
- 库的 API 与设计文档假设一致，或差异已记录并评估

**交付物**：
- `apps/launcher/` 下的 PoC 代码
- 技术预研结论（库能力清单、已知缺陷、需自行实现的部分）

---

### M0：项目骨架 + CLI 框架 + 协议层 + 错误层

**目标**：项目结构完整，CLI 框架可用，协议事件和错误体系就绪。

**任务清单**：

| # | 任务 | 依赖 | 交付物 |
|---|---|---|---|
| M0-1 | 初始化 Cargo 项目，配置依赖和 release profile | M-1 | `apps/launcher/Cargo.toml` |
| M0-2 | 实现 cli.rs：clap 命令树定义（install/launch/auth/java/list/version） | M0-1 | `src/cli.rs` |
| M0-3 | 实现 error.rs：统一错误类型 + 退出码映射 + suggestion | M0-1 | `src/error.rs` |
| M0-4 | 实现 protocol.rs：phase/result 事件输出到 stdout | M0-3 | `src/protocol.rs` |
| M0-5 | 实现 platform.rs：OS/架构检测、内存检测、数据目录 | M0-1 | `src/platform.rs` |
| M0-6 | 实现 install 编排层：调用 download/ + loader/ 完成安装流程 | M0-2,M0-4,M0-5 | `src/install.rs`（编排） |
| M0-7 | 实现文件锁：防止多进程并发安装同一目录 | M0-1 | `src/lock.rs` |
| M0-8 | 实现磁盘空间预检：安装前检查目标目录可用空间 | M0-5 | `src/platform.rs` 扩展 |
| M0-9 | 单元测试：CLI 解析、错误映射、协议序列化、文件锁 | M0-2~M0-8 | `tests/` |

**验收门槛**：
- `cargo build --release` 通过，binary < 10MB
- `mpack-launcher version` 输出版本信息
- `mpack-launcher install --help` 输出完整帮助
- 协议事件输出格式正确（phase + result）
- 错误退出码映射正确
- 文件锁能阻止并发安装

---

### M1：下载层 + Vanilla 安装 + Java 检测 + Vanilla 启动

**目标**：能安装并启动 Vanilla 游戏到主菜单，Java 自动检测可用。

**任务清单**：

| # | 任务 | 依赖 | 交付物 |
|---|---|---|---|
| M1-1 | 实现 download/mirror.rs：URL 重写（按文件类型分类） | M0-6 | `src/download/mirror.rs` |
| M1-2 | 实现 download/item.rs：DownloadItem + FileChecker 预校验 | M1-1 | `src/download/item.rs` |
| M1-3 | 实现 download/cache.rs：断点续传 + 流式 SHA1 + 原子 rename | M1-2 | `src/download/cache.rs` |
| M1-4 | 实现 download/concurrent.rs：双 Semaphore 并发调度 | M1-3 | `src/download/concurrent.rs` |
| M1-5 | 实现 download/mod.rs：Downloader 公开 API + download_all 编排 | M1-4 | `src/download/mod.rs` |
| M1-6 | 实现 java/detect.rs：系统 Java 扫描 + 版本解析 | M0-5 | `src/java/detect.rs` |
| M1-7 | 实现 java/registry.rs：多版本管理 + 版本匹配策略 | M1-6 | `src/java/registry.rs` |
| M1-8 | 实现 java/mod.rs：公开 API detect + select | M1-7 | `src/java/mod.rs` |
| M1-9 | 实现 launch/command.rs：启动命令构建（classpath、JVM 参数） | M1-8 | `src/launch/command.rs` |
| M1-10 | 实现 launch/process.rs：进程管理（spawn/detach/wait/kill） | M1-9 | `src/launch/process.rs` |
| M1-11 | 实现 launch/mod.rs：公开 API build + spawn + wait | M1-10 | `src/launch/mod.rs` |
| M1-12 | install 编排集成：Vanilla 安装流程（调用 download/） | M1-5 | `src/install.rs` 更新 |
| M1-13 | 集成测试：Vanilla 安装 + 启动冒烟 | M1-11,M1-12 | `tests/install_vanilla.rs`, `tests/launch_smoke.rs` |

**验收门槛**：
- `mpack-launcher install --mc 1.20.1 --dir <path>` 能完成 Vanilla 下载
- `mpack-launcher launch --version 1.20.1 --username Steve` 能启动游戏到主菜单
- Java 自动检测能找到系统已安装的 Java
- `--wait` 模式能返回游戏退出码
- 离线账号 UUID 生成正确
- 重复 install 不重复下载（幂等）

---

### M2：加载器安装（Fabric/Forge/NeoForge/Quilt）

**目标**：全加载器支持，processor 流程跑通。

**任务清单**：

| # | 任务 | 依赖 | 交付物 |
|---|---|---|---|
| M2-1 | 实现 loader/fabric.rs：Fabric Meta API + JSON 合并 | M1-12 | `src/loader/fabric.rs` |
| M2-2 | 实现 loader/forge.rs：installer 下载 + processor 执行 + 超时控制 | M2-1 | `src/loader/forge.rs` |
| M2-3 | 实现 loader/neoforge.rs：NeoForge 版本解析 + installer 执行 | M2-2 | `src/loader/neoforge.rs` |
| M2-4 | 实现 loader/quilt.rs：Quilt Meta API + JSON 合并 | M2-1 | `src/loader/quilt.rs` |
| M2-5 | 实现 loader/mod.rs：公开 API install(loader_type, mc_version) | M2-4 | `src/loader/mod.rs` |
| M2-6 | install 编排集成：加载器安装流程 | M2-5 | `src/install.rs` 更新 |
| M2-7 | 集成测试：Fabric/Forge/NeoForge/Quilt 1.20.1 | M2-6 | `tests/install_*.rs` |

**风险**：
- Forge processor 是最大风险点，若 mc-launcher-core 实现有缺陷需自行修补或 workaround
- 旧版 Forge（1.16 及以前）标记为最佳努力，不阻塞 M2

**验收门槛**：
- 四种加载器 1.20.1 均能安装成功并启动到主菜单
- processor 执行失败时有清晰的错误日志和超时控制
- 加载器版本 `latest` 能正确解析

---

### M3：Java 自动下载 + 镜像竞速 + 微软 OAuth

**目标**：零配置一键启动（自动下载 Java），国内网络可用（BMCLAPI 竞速），微软登录可用。

**任务清单**：

| # | 任务 | 依赖 | 交付物 |
|---|---|---|---|
| M3-1 | download/mirror.rs 扩展：auto 竞速模式（双源同时下载） | M1-1 | `src/download/mirror.rs` 更新 |
| M3-2 | java/install.rs：Java 自动下载（Adoptium API，8/17/21）、解压验证 | M1-8 | `src/java/install.rs` |
| M3-3 | java/mod.rs 扩展：install 公开 API | M3-2 | `src/java/mod.rs` 更新 |
| M3-4 | auth/offline.rs：离线账号 + UUID v3 生成 | M0-3 | `src/auth/offline.rs` |
| M3-5 | auth/microsoft.rs：微软 OAuth device flow + token 刷新 | M3-4 | `src/auth/microsoft.rs` |
| M3-6 | auth/store.rs：keyring 加密存储 | M3-5 | `src/auth/store.rs` |
| M3-7 | auth/mod.rs：公开 API login + status + logout | M3-6 | `src/auth/mod.rs` |

**验收门槛**：
- 全新环境（无系统 Java）能自动下载匹配的 Java 并启动游戏
- `--mirror bmclapi` 能从国内镜像下载所有资源
- `--mirror auto` 双源竞速模式可用
- `mpack-launcher auth login --provider microsoft` 能完成 device flow 登录
- 登录后 token 通过 keyring 存储，launch 时自动使用
- token 过期时自动刷新

---

### M4：错误处理完善 + 性能优化 + 跨平台验证

**目标**：友好错误提示，性能指标达标，三平台可用。

**任务清单**：

| # | 任务 | 依赖 | 交付物 |
|---|---|---|---|
| M4-1 | 完善所有错误场景的 suggestion | M2-7,M3-7 | `src/error.rs` 更新 |
| M4-2 | 游戏崩溃日志收集（最后 50 行） | M1-11 | `src/launch/process.rs` 更新 |
| M4-3 | 性能优化：并发调优、流式校验、断点续传完善 | M1-5 | `src/download/` 优化 |
| M4-4 | 非 JSON 文本输出模式（--text）：人类可读的阶段提示 | M0-4 | `src/protocol.rs` 扩展 |
| M4-5 | 性能测试：冷启动时间、内存、下载速度、binary 大小 | M4-3 | 性能测试报告 |
| M4-6 | 跨平台编译验证（Linux/macOS） | M2-7 | 三平台 binary |

**验收门槛**：
- 每个错误类型都有用户可操作的建议
- 游戏崩溃时返回最后 50 行日志
- 冷启动 < 50ms，空闲内存 < 20MB
- 100Mbps 网络下安装 1.20.1 Vanilla < 60s
- binary < 10MB
- `--text` 模式输出人类可读的阶段提示
- Linux/macOS 编译通过，功能测试通过

---

### M5：mPackStation 集成 + 任务化 + 终局闭环

**目标**：从 mPackStation 一键启动 Minecraft，纳入任务系统，终局达成。

**任务清单**：

| # | 任务 | 依赖 | 交付物 |
|---|---|---|---|
| M5-1 | Go 侧封装：exec 调用 mPackLauncher，解析 JSON Lines | M4-5 | `apps/server/internal/service/launcher.go` |
| M5-2 | 任务化：install_game / launch_game 任务类型 | M5-1 | `apps/server/internal/service/` |
| M5-3 | 前端集成：启动按钮、进度展示、日志查看 | M5-2 | `apps/web/src/features/` |
| M5-4 | 端到端测试：构建 mrpack → 安装 → 启动游戏 | M5-3 | E2E 测试用例 |

**验收门槛**：
- mPackStation 中点击"启动游戏"能自动完成安装+启动
- 安装进度实时显示在前端
- 游戏日志可在前端查看
- 启动任务纳入任务系统，可取消、可查看状态
- 终局闭环：从整合包到 Minecraft 游戏窗口，全程无需手动操作

---

## 3. 任务依赖图

```
M-1 ──→ M0-1 ──→ M0-2 ──→ M0-6 ──→ M1-3 ──→ M2-3
                   │         │         │
                   │         └──→ M0-4 ─┘         └──→ M2-1 ──→ M2-2
                   │                   │                   │
                   ├──→ M0-3 ──→ M0-7  │                   └──→ M2-4
                   │         │         │                         │
                   └──→ M0-5 ─┘         └──→ M1-1 ──→ M1-2 ──→ M1-4
                                         │         │
                                         │         └──→ M4-2
                                         │
                                         └──→ M3-2

M0-6 ──→ M3-1
M0-3 ──→ M3-3 ──→ M3-4

M2-5 ──→ M4-5
M4-3 ──→ M4-4
M4-5 ──→ M5-1 ──→ M5-2 ──→ M5-3 ──→ M5-4
```

**关键路径**：M-1 → M0-1 → M0-2 → M0-6 → M1-3 → M2-1 → M2-2 → M2-4 → M2-5 → M4-5 → M5-1 → M5-2 → M5-3 → M5-4

---

## 4. 风险与应对

| 风险 | 概率 | 影响 | 应对 |
|---|---|---|---|
| Forge processor 实现有 bug | 中 | 高 | M-1 预研重点验证；严重时降级为"Forge 最佳努力"，先发布 Vanilla+Fabric+NeoForge |
| mc-launcher-core API 不完整 | 低 | 中 | 封装层可绕过库直接调用 reqwest 实现缺失功能 |
| mc-launcher-core 项目停止维护 | 低 | 高 | M-1 评估社区活跃度；预留 fork 能力，MIT 许可证允许 |
| 微软 OAuth 流程复杂 | 低 | 中 | 参考成熟开源实现，device flow 是标准协议 |
| 跨平台编译问题 | 中 | 低 | 主平台保证 Windows，Linux/macOS 用 CI 或对应平台构建 |
| 国内网络访问 Mojang/Files 慢 | 高 | 中 | BMCLAPI 镜像 + auto 降级，M3 实现 |
| BMCLAPI 单点故障 | 中 | 中 | auto 模式官方+BMCLAPI 双源；后续可扩展第三镜像 |
| 性能不达标 | 低 | 中 | M4 专项优化；性能指标可分阶段达标 |

---

## 5. 交付物清单

### 代码交付物

| 里程碑 | 文件 |
|---|---|
| M-1 | `apps/launcher/` PoC + 预研结论 |
| M0 | `Cargo.toml`, `src/main.rs`, `src/cli.rs`, `src/error.rs`, `src/protocol.rs`, `src/platform.rs`, `src/install.rs`, `src/lock.rs` |
| M1 | `src/download/`, `src/java/`, `src/launch/` |
| M2 | `src/loader/` |
| M3 | `src/download/mirror.rs`（竞速）, `src/java/install.rs`, `src/auth/` |
| M4 | 各模块优化, `src/protocol.rs`（--text 模式） |
| M5 | `apps/server/internal/service/launcher.go`, 前端组件 |

### 文档交付物

| 文档 | 状态 |
|---|---|
| launcher-core-spec.md | 已完成 |
| launcher-core-architecture.md | 已完成 |
| launcher-core-domain.md | 已完成 |
| launcher-core-technical.md | 已完成 |
| launcher-core-plan.md（本文档） | 已完成 |
| launcher-core-test-plan.md | 已完成 |

### 测试交付物

| 测试 | 里程碑 |
|---|---|
| 单元测试 | M0 起持续 |
| 集成测试（Vanilla/Fabric） | M1 |
| 集成测试（Forge/NeoForge/Quilt） | M2 |
| Java 自动下载测试 | M3 |
| 镜像降级测试 | M3 |
| 性能测试 | M4 |
| 端到端测试 | M5 |

---

## 6. 进度跟踪

### 6.1 里程碑验收

每个里程碑结束时：
- 运行该里程碑的验收门槛检查清单
- 记录通过/失败项
- 未通过项记入 issue，决定是修复还是降级
- 验收通过后才进入下一里程碑

### 6.2 状态记录

进度状态写入 `docs/project-state/` 下的会话摘要，与项目现有规范一致。

---

## 7. 假设与约束

### 假设

1. mc-launcher-core 0.1.2 的 Forge/NeoForge 实现基本可用（M-1 验证）
2. 开发环境为 Windows，可安装 Rust 工具链
3. 测试网络可访问 Mojang 和 BMCLAPI
4. 测试机器有足够磁盘空间（>5GB）用于下载游戏文件

### 约束

1. 不修改 mc-launcher-core 源码（通过封装层适配）
2. 保持 mPackStation 纯 Go 无 cgo
3. binary 体积 < 10MB
4. 不引入 GPL 传染性依赖
