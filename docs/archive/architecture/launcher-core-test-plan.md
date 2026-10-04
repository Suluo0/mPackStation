# mPackLauncher 测试方案

> 关联文档：[launcher-core-spec.md](launcher-core-spec.md)（接口契约）、[launcher-core-technical.md](launcher-core-technical.md)（技术设计）
> 状态：方案基线

---

## 1. 测试策略总览

### 1.1 测试金字塔

```
        ┌─────────────┐
        │  E2E 测试   │  少而精，验证终局闭环
        │  (mPackStation 集成) │
        ├─────────────┤
        │  集成测试    │  跨模块，需要网络/文件系统
        │  (安装/启动/认证)  │
        ├─────────────┤
        │  单元测试    │  多而全，纯函数，无外部依赖
        │  (解析/构建/校验)  │
        └─────────────┘
```

### 1.2 测试层次定义

| 层次 | 目标 | 数量级 | 运行速度 | 外部依赖 |
|---|---|---|---|---|
| 单元测试 | 验证单个函数/模块的逻辑正确性 | ~100+ | < 1s | 无 |
| 集成测试 | 验证多模块协作和外部 API 交互 | ~30 | 10s~5min | 网络、文件系统 |
| 性能测试 | 验证性能指标达标 | ~10 | 1~10min | 网络、系统资源 |
| E2E 测试 | 验证 mPackStation 到游戏的完整链路 | ~5 | 1~10min | 全栈 |

### 1.3 覆盖率目标

| 模块 | 行覆盖率目标 | 说明 |
|---|---|---|
| error.rs | 90% | 错误映射必须全覆盖 |
| cli.rs | 85% | 参数解析覆盖主要组合 |
| java/ | 80% | 版本解析、匹配规则全覆盖 |
| download/mirror.rs | 90% | URL 重写、竞速模式全覆盖 |
| protocol.rs | 90% | phase/result 事件序列化、输出格式验证 |
| platform.rs | 70% | 平台相关逻辑，跨平台测试受限 |
| download/ | 70% | 核心流程靠集成测试覆盖 |
| launch/ | 60% | 命令构建靠集成测试覆盖 |
| auth/ | 50% | 网络流程靠 mock 覆盖部分 |
| **整体** | **> 70%** | |

---

## 2. 单元测试设计

### 2.1 CLI 解析测试（cli.rs）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| CLI-01 | `install --mc 1.20.1` | 最小参数解析正确 |
| CLI-02 | `install --mc 1.20.1 --loader fabric --loader-version 0.16.5` | 加载器参数解析 |
| CLI-03 | `install --mc 1.20.1 --mirror bmclapi --concurrency 16 --force` | 可选参数解析 |
| CLI-04 | `launch --version 1.20.1 --username Steve --xmx 4G` | 启动参数解析 |
| CLI-05 | `launch --version 1.20.1 --account-type microsoft` | 账号类型解析 |
| CLI-06 | `auth login --provider microsoft` | 认证子命令解析 |
| CLI-07 | `java install --version 17` | Java 子命令解析 |
| CLI-08 | 缺少必填参数 `install`（无 --mc） | 返回错误，退出码 1 |
| CLI-09 | 无效枚举值 `--loader invalid` | 返回错误，退出码 1 |
| CLI-10 | `--json` 标志 | 正确设置输出模式 |
| CLI-11 | `version` 子命令 | 输出版本信息 |
| CLI-12 | `list --dir /tmp/test` | 列表子命令解析 |

### 2.2 错误处理测试（error.rs）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| ERR-01 | 每种错误变体的 exit_code 映射正确 | 退出码与 spec 一致 |
| ERR-02 | DownloadFailed 的 suggestion 包含镜像建议 | 用户可操作 |
| ERR-03 | JavaNotFound 的 suggestion 包含安装命令 | 用户可操作 |
| ERR-04 | GameCrashed(exit=1) 的 suggestion 包含内存建议 | 用户可操作 |
| ERR-05 | 错误的 Display 输出不包含敏感信息 | 安全 |
| ERR-06 | Io 错误自动转换 | thiserror 转换 |
| ERR-07 | Json 错误自动转换 | thiserror 转换 |

### 2.3 Java 管理测试（java/）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| JAVA-01 | 解析 `version "17.0.9"` → 主版本 17 | 新版本格式 |
| JAVA-02 | 解析 `version "1.8.0_351"` → 主版本 8 | 旧版本格式 |
| JAVA-03 | 解析 `version "21.0.1"` → 主版本 21 | Java 21 |
| JAVA-04 | 解析无效输出 → 返回 ParseError | 错误处理 |
| JAVA-05 | mc_version "1.20.1" → 需要 Java 17 | 版本映射 |
| JAVA-06 | mc_version "1.16.5" → 需要 Java 8 | 旧版本映射 |
| JAVA-07 | mc_version "1.17" → 需要 Java 17 | 分界版本（1.17 统一用 Java 17） |
| JAVA-08 | mc_version "1.21.1" → 需要 Java 21 | 最新版本 |
| JAVA-09 | mc_version "1.20.5" → 需要 Java 21 | 分界版本 |
| JAVA-10 | 系统内存 8GB → auto_xmx = "4G" | 内存分配 |
| JAVA-11 | 系统内存 16GB → auto_xmx = "4G"（上限） | 内存上限 |
| JAVA-12 | 系统内存 2GB → auto_xmx = "1G"（75%） | 小内存 |

### 2.4 镜像源测试（download/mirror.rs）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| MIR-01 | Mojang 模式 URL 不变 | 不重写 |
| MIR-02 | BMCLAPI 模式 piston-meta 重写正确 | 域名替换 |
| MIR-03 | BMCLAPI 模式 piston-data 重写正确 | 域名替换 |
| MIR-04 | BMCLAPI 模式 libraries 重写正确（含 /maven） | 路径替换 |
| MIR-05 | BMCLAPI 模式 resources 重写正确（含 /assets） | 路径替换 |
| MIR-06 | Auto 模式失败 0 次 → 使用 Mojang | 初始状态 |
| MIR-07 | Auto 模式失败 2 次 → 仍使用 Mojang | 未达阈值 |
| MIR-08 | Auto 模式失败 3 次 → 切换 BMCLAPI | 阈值触发 |
| MIR-09 | Auto 模式切换后不回退 | 稳定性 |
| MIR-10 | 非白名单 URL 不重写 | 安全性 |

### 2.5 协议输出测试（protocol.rs）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| PROT-01 | phase 事件输出格式 | type="phase"，含 phase 和 message |
| PROT-02 | result 成功事件格式 | type="result"，success=true，含 data |
| PROT-03 | result 失败事件格式 | type="result"，success=false，含 error/message/suggestion |
| PROT-04 | JSON Lines 格式 | 每行一个 JSON，无多余空行 |
| PROT-05 | stdout/stderr 分离 | 协议只在 stdout，日志只在 stderr |
| PROT-06 | 多线程输出不交错 | 线程安全 |

### 2.6 启动命令构建测试（launch/command.rs）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| LAUNCH-01 | classpath 包含所有 libraries + client.jar | 完整性 |
| LAUNCH-02 | classpath 顺序正确（libraries 在前，client.jar 在后） | 顺序 |
| LAUNCH-03 | Windows classpath 用分号分隔 | 平台 |
| LAUNCH-04 | Linux classpath 用冒号分隔 | 平台 |
| LAUNCH-05 | JVM 参数包含 -Xmx/-Xms | 内存 |
| LAUNCH-06 | JVM 参数包含 -Djava.library.path | natives |
| LAUNCH-07 | 游戏参数包含 --username/--version/--gameDir | 基础参数 |
| LAUNCH-08 | 离线账号 UUID 生成正确 | 算法 |
| LAUNCH-09 | 额外 --jvm-args 被追加 | 扩展 |
| LAUNCH-10 | 额外 --game-args 被追加 | 扩展 |
| LAUNCH-11 | --server 参数被加入游戏参数 | 服务器 |
| LAUNCH-12 | 1.17+ version JSON 的 jvm arguments 被正确替换 | 变量替换 |

### 2.7 认证测试（auth/）

**测试用例**：

| # | 用例 | 验证点 |
|---|---|---|
| AUTH-01 | 离线账号 UUID 与标准算法一致 | 算法正确性 |
| AUTH-02 | 不同用户名生成不同 UUID | 唯一性 |
| AUTH-03 | token 过期判断正确（<5min 过期） | 过期逻辑 |
| AUTH-04 | token 未过期时不刷新 | 正常路径 |
| AUTH-05 | 凭证存储/读取往返一致 | 持久化 |

---

## 3. 集成测试设计

### 3.1 测试环境

| 项 | 要求 |
|---|---|
| 网络 | 可访问 Mojang 官方 + BMCLAPI |
| 磁盘空间 | > 10GB 可用 |
| 内存 | > 4GB |
| Java | 系统安装 Java 8/17/21（或允许自动下载） |
| 测试目录 | 临时目录，测试后清理 |

### 3.2 Vanilla 安装测试

| # | 用例 | 验证点 | 网络 |
|---|---|---|---|
| INT-V01 | 安装 1.20.1 Vanilla | version_id 正确，所有文件存在 | 需要 |
| INT-V02 | 安装 1.21.1 Vanilla | 最新版本兼容 | 需要 |
| INT-V03 | 安装 1.16.5 Vanilla（Java 8） | 旧版本兼容 | 需要 |
| INT-V04 | 重复安装 1.20.1（幂等） | 不重复下载，速度快 | 需要 |
| INT-V05 | --force 强制重新安装 | 重新下载所有文件 | 需要 |
| INT-V06 | 安装过程中 kill 后恢复 | 断点续传，不重复已下载 | 需要 |
| INT-V07 | 安装后校验所有文件 SHA1 | 完整性 | 不需要 |
| INT-V08 | 不存在的版本 `--mc 99.99.99` | 返回 VersionNotFound，退出码 4 | 需要 |

### 3.3 加载器安装测试

| # | 用例 | 验证点 | 网络 |
|---|---|---|---|
| INT-L01 | 安装 Fabric 1.20.1（latest） | version_id 包含 fabric，loader 版本正确 | 需要 |
| INT-L02 | 安装 Fabric 1.20.1（指定版本 0.16.5） | 指定版本生效 | 需要 |
| INT-L03 | 安装 Forge 1.20.1 | processor 执行成功，version JSON 正确 | 需要 |
| INT-L04 | 安装 NeoForge 1.20.1 | processor 执行成功 | 需要 |
| INT-L05 | 安装 Quilt 1.20.1 | 安装成功 | 需要 |
| INT-L06 | Forge 1.16.5（旧版 installer） | 最佳努力，记录结果 | 需要 |
| INT-L07 | 加载器与 MC 版本不兼容 | 返回 LoaderIncompatible，退出码 4 | 需要 |
| INT-L08 | 同版本重复安装加载器 | 幂等，不重复执行 processor | 需要 |

### 3.4 启动测试

| # | 用例 | 验证点 | 网络 |
|---|---|---|---|
| INT-S01 | 启动 1.20.1 Vanilla（离线） | 游戏进程启动，PID 正确，5 秒后退出 | 不需要 |
| INT-S02 | 启动 Fabric 1.20.1（离线） | 游戏进程启动，日志包含 Fabric 标识 | 不需要 |
| INT-S03 | 启动 Forge 1.20.1（离线） | 游戏进程启动，日志包含 Forge 标识 | 不需要 |
| INT-S04 | --wait 模式等待游戏退出 | 返回退出码 0，duration_ms > 0 | 不需要 |
| INT-S05 | 无效 version_id 启动 | 返回错误，退出码 4 | 不需要 |
| INT-S06 | Java 路径不存在 | 返回 JavaNotFound，退出码 5 | 不需要 |
| INT-S07 | Java 版本不匹配 | 返回 JavaVersionMismatch，退出码 5 | 不需要 |
| INT-S08 | --log-file 重定向日志 | 日志文件存在且非空 | 不需要 |

> 启动测试使用 `--game-args "--demo"` 或启动后 5 秒自动 kill，避免需要人工关闭游戏窗口。

### 3.5 Java 管理测试

| # | 用例 | 验证点 | 网络 |
|---|---|---|---|
| INT-J01 | 检测系统已安装的 Java | 列表非空，版本正确 | 不需要 |
| INT-J02 | 自动下载 Java 17（全新环境） | 下载成功，java -version 正确 | 需要 |
| INT-J03 | 自动下载 Java 8 | 下载成功，版本正确 | 需要 |
| INT-J04 | 自动下载 Java 21 | 下载成功，版本正确 | 需要 |
| INT-J05 | `java list` 列出所有已检测/已下载的 Java | 列表完整 | 不需要 |
| INT-J06 | `java remove --version 17` 删除已下载的 Java | 目录被删除 | 不需要 |
| INT-J07 | 安装时自动匹配 Java 版本 | 1.20.1 用 Java 17，1.16.5 用 Java 8 | 需要 |

### 3.6 镜像源测试

| # | 用例 | 验证点 | 网络 |
|---|---|---|---|
| INT-M01 | --mirror mojang 安装 | 从官方源下载 | 需要 |
| INT-M02 | --mirror bmclapi 安装 | 从 BMCLAPI 下载，URL 被重写 | 需要 |
| INT-M03 | --mirror auto 正常网络 | 走官方源 | 需要 |
| INT-M04 | --mirror auto 模拟官方超时 | 自动降级到 BMCLAPI | 需要（模拟） |
| INT-M05 | BMCLAPI 下载的文件 SHA1 正确 | 镜像数据完整性 | 需要 |

### 3.7 认证测试

| # | 用例 | 验证点 | 网络 |
|---|---|---|---|
| INT-A01 | 离线登录 | 账号信息正确，UUID 标准 | 不需要 |
| INT-A02 | 微软 device flow 登录 | 完整流程，token 存储 | 需要 |
| INT-A03 | auth status 显示已登录账号 | 信息正确，token 脱敏 | 不需要 |
| INT-A04 | auth logout 清除凭证 | 凭证被删除 | 不需要 |
| INT-A05 | token 过期自动刷新 | 刷新成功，launch 使用新 token | 需要 |

> 微软登录测试需要人工在浏览器操作，标记为手动测试。

### 3.8 错误场景测试

| # | 用例 | 验证点 |
|---|---|---|
| INT-E01 | 网络断开时安装 | 返回网络错误，退出码 2，建议切换镜像 |
| INT-E02 | 磁盘空间不足时安装 | 提前检测并报错，不留下半成品 |
| INT-E03 | 下载文件 SHA1 不匹配 | 自动重试，3 次后报错退出码 3 |
| INT-E04 | 游戏启动后立即崩溃 | 返回 GameCrashed，退出码 10，含最后 50 行日志 |
| INT-E05 | 权限不足无法写入目录 | 返回 IO 错误，退出码 1 |
| INT-E06 | --json 模式下 stdout 每行合法 JSON | 协议完整性 |

---

## 4. 性能测试设计

### 4.1 性能指标与验收标准

| 指标 | 目标 | 测试方法 |
|---|---|---|
| 内核冷启动时间 | < 50ms | `time mpack-launcher version` 取 10 次平均 |
| 空闲内存 | < 20MB | 安装过程中峰值 RSS |
| 安装峰值内存 | < 100MB | 安装 1.20.1 过程中 RSS 峰值 |
| 1.20.1 Vanilla 安装耗时 | < 60s（100Mbps） | 干净环境，冷缓存 |
| 1.20.1 Vanilla 安装耗时（热缓存） | < 5s | 已下载所有文件，幂等跳过 |
| binary 大小 | < 10MB | `ls -la` release binary |
| 游戏启动延迟（已安装） | < 3s | 从 launch 命令到游戏窗口出现 |

### 4.2 性能测试方法

**冷启动时间**：
```bash
for i in {1..10}; do
  time ./mpack-launcher version
done
# 取平均值，排除第一次（文件系统缓存）
```

**内存测量**：
```bash
# 安装过程中采样
./mpack-launcher install --mc 1.20.1 --dir /tmp/test &
PID=$!
while kill -0 $PID 2>/dev/null; do
  ps -o rss= -p $PID
  sleep 0.5
done | sort -n | tail -1
```

**安装耗时**：
```bash
# 冷缓存（清理后）
rm -rf /tmp/test
time ./mpack-launcher install --mc 1.20.1 --dir /tmp/test

# 热缓存（重复安装）
time ./mpack-launcher install --mc 1.20.1 --dir /tmp/test
```

### 4.3 并发下载性能验证

- 测试不同并发数（8/16/32/64）的下载耗时
- 确认 32 并发为最优（收益递减点）
- 记录 CPU/网络利用率

---

## 5. 端到端测试设计

### 5.1 测试场景

| # | 场景 | 步骤 | 验证点 |
|---|---|---|---|
| E2E-01 | 一键启动 Vanilla | mPackStation 创建空包 → 点击启动 → 游戏窗口出现 | 全程无人工干预，游戏到主菜单 |
| E2E-02 | 一键启动 Fabric 包 | 创建包 → 加 Fabric 模组 → 启动 → 游戏窗口出现 | Fabric 加载成功，模组生效 |
| E2E-03 | 一键启动 Forge 包 | 创建包 → 加 Forge 模组 → 启动 → 游戏窗口出现 | Forge 加载成功 |
| E2E-04 | 安装阶段展示 | 启动安装 → 前端实时显示当前阶段 | 阶段文字随流程切换，无长时间卡顿 |
| E2E-05 | 游戏日志查看 | 启动游戏 → 前端查看日志 | 日志实时更新，可滚动 |
| E2E-06 | 取消安装 | 安装中点击取消 → 任务取消 → 可重新安装 | 取消后无残留进程，文件可清理 |

### 5.2 E2E 测试环境

- 使用独立测试数据目录（不污染开发数据）
- 使用测试包（小体积，快速安装）
- 游戏启动后 10 秒自动关闭（脚本 kill）
- 截图记录每个步骤

---

## 6. 测试工具与框架

### 6.1 Rust 测试

- **单元测试**：Rust 内置 `#[test]` + `assert!`
- **集成测试**：`tests/` 目录，独立 crate
- **异步测试**：`tokio::test`
- **Mock**：`wiremock`（HTTP mock）、`tempfile`（临时目录）
- **覆盖率**：`cargo tarpaulin`（或 `cargo llvm-cov`）

### 6.2 Go 侧测试（mPackStation 集成）

- 沿用项目现有测试框架
- `exec` 调用测试使用真实 binary
- 协议事件解析测试使用 fixture 数据

### 6.3 前端测试

- 沿用项目现有测试框架（tsc --noEmit）
- 组件测试使用现有 mock 机制

---

## 7. 测试执行流程

### 7.1 开发阶段

- 每个任务完成后运行相关单元测试
- 提交前运行 `cargo test` 全量单元测试
- 提交前运行 `cargo clippy` 静态检查
- 提交前运行 `cargo fmt --check` 格式检查

### 7.2 里程碑验收

每个里程碑结束时运行：
1. 全量单元测试（必须通过）
2. 该里程碑相关的集成测试（必须通过）
3. 性能测试（必须达标）
4. `cargo build --release`（必须通过）
5. 二进制大小检查（必须 < 10MB）

### 7.3 发布前

1. 全量单元测试 + 集成测试
2. 全量性能测试
3. 三平台编译验证
4. E2E 测试
5. 覆盖率报告（>70%）

---

## 8. 测试数据与 Fixture

### 8.1 测试用版本

| 版本 | 用途 | 说明 |
|---|---|---|
| 1.20.1 | 主力测试版本 | 加载器生态最完善 |
| 1.21.1 | 最新版本测试 | 验证最新兼容 |
| 1.16.5 | 旧版本测试 | Java 8 + 旧 Forge |
| 1.8.9 | 古版本测试 | 最佳努力 |

### 8.2 临时目录管理

- 所有测试使用 `tempfile::tempdir()` 创建临时目录
- 测试结束后自动清理（RAII）
- 集成测试可配置保留目录（调试用）

### 8.3 网络依赖处理

- 需要网络的集成测试标记 `#[ignore]`，默认不运行
- CI 中使用 `cargo test -- --ignored` 运行网络测试
- 离线环境下跳过网络测试，不报错

---

## 9. 质量门禁

### 9.1 提交门禁

- `cargo fmt --check` 通过
- `cargo clippy -- -D warnings` 通过
- `cargo test`（单元测试）通过
- 新增代码有对应单元测试

### 9.2 里程碑门禁

- 该里程碑所有验收用例通过
- 单元测试覆盖率不下降
- 无 clippy warning
- 性能指标达标

### 9.3 发布门禁

- 全量测试通过（单元+集成+E2E）
- 三平台编译通过
- 覆盖率 > 70%
- 性能指标全部达标
- 无已知 P0/P1 bug

---

## 10. 已知测试限制

| 限制 | 说明 | 应对 |
|---|---|---|
| 微软登录需人工操作 | device flow 需要浏览器 | 标记为手动测试，CI 中跳过 |
| 游戏启动需 GUI | 无头环境无法测试 | 开发机手动验证，CI 只测到 spawn 成功 |
| 网络不稳定 | Mojang/BMCLAPI 偶发超时 | 测试允许重试 3 次，记录网络相关失败 |
| 跨平台测试 | 开发机为 Windows | Linux/macOS 用交叉编译+手动验证 |
| Forge processor 耗时 | 每次安装 1-3 分钟 | 集成测试默认忽略，按需运行 |
