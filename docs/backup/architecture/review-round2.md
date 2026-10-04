# mPackLauncher 启动器内核设计文档 — 第二轮盲审报告

> 评审日期：2026-09-03
> 评审对象：docs/architecture/ 下 launcher-core-*.md 共 6 份
> 评审轮次：第二轮（第一轮高严重度问题修复验证 + 新增问题排查）

---

## 总体评分：4.5 / 10

**一句话总评**：第一轮 8 个问题中仅 3 个真正修复，plan.md 时间单位零修改、mc_version_to_java 核心 bug 因 match 臂顺序错误依然存在，6 份文档间存在多处未同步的残留矛盾，尚不具备进入实现阶段的条件。

---

## 一、第一轮问题修复状态逐项验证

### 1. panic=abort 与 catch_unwind 冲突 → ✅ 已修复

- spec.md §7.1：`panic = "abort"`
- architecture.md §2.1：明确写"不做 catch_unwind；panic 视为不可恢复的内部错误"
- technical.md §1.1：`panic = "abort"`
- 6 份文档全文无 catch_unwind 残留。

### 2. mc_version_to_java 函数 bug（1.20.5 误匹配 Java 17）→ ❌ 未修复

**technical.md §11.2 代码依然有 bug，且引入了新的矛盾：**

```rust
match parts.as_slice() {
    [1, minor, ..] if *minor <= 16 => 8,
    [1, 17, ..] => 16,                          // ← Bug A：1.17 返回 16，但 spec/domain 规定 1.17→Java 17
    [1, minor, ..] if *minor <= 20 => 17,      // ← Bug B：1.20.5 在此被捕获（minor=20 ≤ 20），返回 17
    [1, 20, patch, ..] if *patch >= 5 => 21,   // ← 永远不可达！被上一条臂拦截
    [1, minor, ..] if *minor >= 21 => 21,
    _ => 17,
}
```

**逐条验证**：
- 输入 `"1.20.5"` → parts=`[1,20,5]` → 第三条臂 `minor=20 ≤ 20` 命中 → 返回 **17**（应为 21）。作者声称修复的 1.20.5 bug **完全没有生效**，因为 match 臂顺序错误。
- 输入 `"1.17"` → parts=`[1,17]` → 第二条臂命中 → 返回 **16**（应为 17，与问题 #4 的修复直接矛盾）。

正确写法必须将 `[1, 20, patch, ..] if *patch >= 5` 放在 `[1, minor, ..] if *minor <= 20` 之前，并删除 `[1, 17, ..] => 16` 或改为 17。

### 3. --access-token 命令行传参与安全声明矛盾 → ✅ 已修复

- spec.md §4.3.2 launch 参数表无 `--access-token`
- spec.md §4.3.2："microsoft 时从缓存读取，不通过命令行传递凭证"
- spec.md §9："不通过命令行参数传递 token（从凭证存储读取）"
- architecture.md §7.2："进程命令行参数中不传递 token"
- 6 份文档无 `--access-token` 残留。

**遗留小缺口**：launch 命令仅有 `--account-type microsoft`，无 `--account-id` 或类似参数。当缓存中存在多个微软账号时，启动器选择哪个账号？文档未定义。建议补充默认账号选择策略。

### 4. Java 16 无下载来源 → 1.17 映射改为 Java 17 → ⚠️ 部分修复

**已修复的部分**：
- spec.md §4.3.4："1.17 ~ 1.20.4：Java 17"
- domain.md §2.3：同上
- architecture.md §2.2.4：同上

**未修复的残留**：
- technical.md §11.2：`[1, 17, ..] => 16` — 代码仍返回 Java 16
- technical.md §3.3：Adoptium API 参数列表仍写 `version: 8 / 16 / 17 / 21`，包含 16
- test-plan.md JAVA-07：`mc_version "1.17" → 需要 Java 16` — 测试用例仍期望 Java 16

三份文档与 spec/domain/architecture 的 Java 17 映射直接冲突。

### 5. LiteLoader 声明支持但零实现 → ✅ 已修复

- 6 份文档全文检索无 LiteLoader 残留。
- 加载器列表统一为 Forge/NeoForge/Fabric/Quilt + Vanilla。

### 6. 代理支持声明但完全缺失 → ✅ 已修复

- 6 份 launcher 文档全文检索无 proxy/代理 残留。
- （grep 命中的 3 条均来自 backend-architecture 等其他文档，与启动器内核无关。）

### 7. 进度计划使用时间单位 → ❌ 未修复

**launcher-core-plan.md 完全未修改，全文有 44 处时间单位**，包括但不限于：

| 位置 | 内容 |
|---|---|
| §1 总览 | "总工期 ~21 个工作日（约 4 周）" |
| §2 M0 | "工期：3 天"，任务表 7 项均有 0.5d/1d 预估 |
| §2 M1 | "工期：3 天"，4 项任务有预估 |
| §2 M2 | "工期：5 天"，5 项任务有预估，"预留 1 天缓冲" |
| §2 M3 | "工期：4 天"，4 项任务有预估 |
| §2 M4 | "工期：3 天"，5 项任务有预估 |
| §2 M5 | "工期：3 天"，4 项任务有预估 |
| §3 | "关键路径总工期：~18 天…总工期 ~21 天" |
| §4 | 风险表 4 处"预留 X 天缓冲"，"总缓冲：约 3 天" |
| §6.1 | "每个工作日结束时更新" |

这直接违反用户偏好（项目文档不得出现时间单位用于进度评估），也与 spec.md §12 末句"不设固定工期，以验收门槛为完成标准"自相矛盾。plan.md 需要按验收门槛制全面重写。

### 8. 新增 M-1 技术预研里程碑 → ⚠️ 部分修复

- spec.md §12 已新增 M-1 行，内容合理。
- **plan.md 完全没有 M-1**：§1 写"里程碑 M0 ~ M5，共 6 个"，§2 从 M0 开始拆解，§3 依赖图无 M-1 节点。
- M-1 作为所有实现的前置依赖（验证 mc-launcher-core 真实能力），却未进入计划文档的任务拆解和依赖图，这是关键遗漏。

---

## 二、第一轮中低严重度问题处理状态

| 问题 | 状态 | 说明 |
|---|---|---|
| 缓存策略矛盾 | ✅ 已处理 | architecture §2.4.2 明确"不设独立缓存层，文件直接写入目标位置"，*.partial 临时文件机制一致 |
| 下载并发单 Semaphore | ❌ 未处理 | architecture §2.2.1 说"两个独立 Semaphore"，但 technical §2.1 的 `Downloader` 结构体只有一个 `semaphore: Semaphore`，`download_all` 也只用一个 |
| sysinfo API 错误 | ❌ 未处理 | technical §5.4 `sysinfo::System::total_memory()` 以静态方法调用，sysinfo 0.30 的 `total_memory()` 是实例方法（需 `System::new()` + `refresh_memory()`），此代码无法编译 |
| Cargo.toml 缺依赖 | ❌ 未处理 | 见下方新问题 #5 |
| version JSON 非原子写入 | ✅ 已处理 | spec §5.1 步骤 9 明确"先写临时文件再 rename" |
| Forge processor 无超时 | ✅ 已处理 | spec §9 明确"默认 300s，超时则终止并报错" |
| 并发安装无文件锁 | ✅ 已处理 | spec §9 明确使用 `<dir>/.mpack-install.lock` |
| 路径安全校验缺失 | ✅ 已处理 | spec §9、domain §10、architecture §7.1 均声明路径规范化+禁止目录穿越 |

---

## 三、新发现的问题

### 【高】H-1：mc_version_to_java match 臂顺序错误，1.20.5 仍返回 Java 17

详见第一轮问题 #2。这是核心功能 bug，直接导致 1.20.5+ 版本使用错误的 Java 版本启动，游戏将崩溃。

### 【高】H-2：plan.md 时间单位零修改，违反硬性约束

详见第一轮问题 #7。44 处时间单位，且与 spec.md 的"不设固定工期"声明矛盾。

### 【中】M-1：下载职责划分模糊——mc-launcher-core 还是自研 Downloader？

- architecture.md §1.3 数据流："mc-launcher-core: 并发下载 client.jar/libraries/assets"
- technical.md §2：完整实现了自研 `Downloader`（含 Semaphore、断点续传、流式 SHA1、mirror 重写）
- spec.md §5.1 流程步骤 7 写"并发下载"但未明确由谁执行

两份文档对"谁负责下载"没有一致答案。如果 mc-launcher-core 自带下载，则自研 Downloader 是重复造轮子且无法复用库的校验逻辑；如果自研 Downloader 替代库的下载，则需要说明如何绕过库的下载接口、如何与库的安装流程衔接。这是架构级歧义，必须在实现前澄清。

### 【中】M-2：Downloader 单 Semaphore 与架构文档的双 Semaphore 矛盾

- architecture §2.2.1："两个独立 Semaphore 控制并发数——assets 32、libraries 16"
- architecture §3.2 图：分别画出 assets (Semaphore=32) 和 libraries (Semaphore=16)
- technical §2.1 `Downloader`：仅一个 `semaphore: Semaphore` 字段，`download_all` 统一 acquire

若按 technical 实现，assets 和 libraries 共享同一个并发上限，无法实现 spec 承诺的"assets 32 并发、libraries 16 并发"同时进行。

### 【中】M-3：Cargo.toml 缺失至少 3 个直接依赖

technical.md §1.1 的 Cargo.toml 中缺少以下被代码引用的 crate：

| 缺失依赖 | 引用位置 | 说明 |
|---|---|---|
| `md-5` | §11.1 `md5::compute(data.as_bytes())` | 离线 UUID 生成 |
| `regex` | §3.1 `Regex::new(r#"version "(\d+)...` | Java 版本解析 |
| `chrono` | §1.2 build.rs `chrono::Local::now()` | 构建时间注入 |

此外 test-plan §6.1 提到 `wiremock`、`tempfile` 作为测试依赖，但 Cargo.toml 无 `[dev-dependencies]` 段。按此 Cargo.toml 执行 `cargo build` 将编译失败。

### 【中】M-4：test-plan JAVA-07 与 spec 冲突——1.17 期望 Java 16

test-plan.md §2.3 JAVA-07：`mc_version "1.17" → 需要 Java 16`，但 spec/domain/architecture 均规定 1.17→Java 17。若按此测试用例验收，将与规格矛盾。

### 【中】M-5：M-1 里程碑未进入 plan.md 的任务拆解和依赖图

spec §12 有 M-1，但 plan.md 从 M0 开始，M-1 的交付物、验收门槛、与 M0 的依赖关系均未定义。M-1 是验证 mc-launcher-core 能力的前置关卡，缺失它意味着 M0 可能基于错误假设开工。

### 【中】M-6：进度上报并发模型矛盾——有界 channel vs 直接 stdout 锁

- architecture §3.3："进度事件通过有界 channel（容量 1000），防止生产过快导致内存膨胀"
- architecture §2.4.3："进度事件直接从下载 task 调用 emit_progress，内部获取 stdout 锁后写入"
- technical §7.1：`emit_progress` 直接 `stdout.lock()` + `writeln!`，无 channel

两种模型二选一即可，但文档同时描述了两种。若用直接 stdout 锁，则高并发下载时 32 个 task 竞争同一把锁，可能成为瓶颈；若用有界 channel，则需要一个独立的消费 task，technical 中缺失。

### 【低】L-1：sysinfo API 调用方式错误

technical §5.4：`sysinfo::System::total_memory()` 以关联函数方式调用。sysinfo 0.30 中 `total_memory()` 是 `&self` 方法，必须先创建 `System` 实例并调用 `refresh_memory()`。正确写法类似：

```rust
let mut sys = System::new();
sys.refresh_memory();
let total_memory = sys.total_memory();
```

### 【低】L-2：进度事件字段命名不一致

- spec §4.3.1 示例 JSON：`speed_bps`
- spec §5.7 结构体：`speed_bps`
- spec §5.1 文字描述："含实时速度（bytes_per_second）"
- domain §9.2：`speed_bps` = 最近 5 秒平均

字段名统一为 `speed_bps`，但 spec §5.1 的文字描述用了 `bytes_per_second`，容易误导实现者。建议统一。

### 【低】L-3：spec §5.1 "全部下载完成后统一校验" 与流式校验矛盾

- spec §5.1："全部下载完成后统一校验，校验失败的文件重新下载"
- domain §6.5："下载过程中流式计算 SHA1…下载完成后比对期望 SHA1"（逐文件）
- technical §2.1 `download_one`：每个文件下载完立即校验

实际设计是逐文件校验，spec 的"统一校验"描述不准确。

### 【低】L-4：总体进度计算公式未考虑文件大小差异

spec §5.1 和 domain §9.2 均定义：
```
总进度 = (已完成文件数 + 进行中文件的字节完成比例) / 总文件数
```
client.jar（~30MB）与一个 1KB 的 asset 权重相同，导致下载大文件时进度条长时间不动、最后瞬间跳变。建议按字节加权。

### 【低】L-5：plan.md 文档状态表错误

plan.md §5.2 写 `launcher-core-test-plan.md | 进行中`，但 test-plan.md 头部状态为"方案基线"且内容完整。应更新为"已完成"。

### 【低】L-6：多微软账号启动时账号选择策略未定义

launch 命令仅有 `--account-type`，无账号标识参数。auth status 可显示多个缓存账号，但 launch 时如何选择？需补充"默认使用最近登录的账号"或增加 `--account-uuid` 参数。

---

## 四、文档间残留矛盾汇总

| # | 矛盾点 | 涉及文档 | 严重度 |
|---|---|---|---|
| C-1 | 1.17 Java 映射：spec/domain/architecture=Java 17，technical 代码=16，test-plan=16 | spec / domain / architecture vs technical / test-plan | 高 |
| C-2 | 1.20.5 Java 映射：所有文档=Java 21，technical 代码实际返回 17 | 全部 vs technical | 高 |
| C-3 | 工期：spec="不设固定工期"，plan=44 处时间单位 | spec vs plan | 高 |
| C-4 | 里程碑：spec 有 M-1，plan 无 M-1 | spec vs plan | 中 |
| C-5 | 下载并发：architecture=双 Semaphore，technical=单 Semaphore | architecture vs technical | 中 |
| C-6 | 下载执行者：architecture=mc-launcher-core，technical=自研 Downloader | architecture vs technical | 中 |
| C-7 | 进度上报：architecture=有界 channel，technical=直接 stdout 锁 | architecture vs technical | 中 |
| C-8 | Adoptium 版本参数：technical 仍列 16，spec 只下载 8/17/21 | spec vs technical | 中 |
| C-9 | 校验时机：spec="统一校验"，domain/technical=逐文件校验 | spec vs domain/technical | 低 |
| C-10 | 进度速度字段名：spec 文字=bytes_per_second，结构体=speed_bps | spec 内部 | 低 |
| C-11 | test-plan 文档状态：plan 写"进行中"，实际已完成 | plan vs test-plan | 低 |

---

## 五、技术可行性评估

### mc-launcher-core 0.1.2 能力描述

spec 附录 A 列出的能力清单（Vanilla/Fabric/Quilt/Forge/NeoForge 安装、Java 启动命令构建、离线账号、微软认证 helper、进度事件等）**均为声明性描述，未经 M-1 验证**。文档自身也在风险表中承认"mc-launcher-core 的 Forge processor 实现有 bug"为中概率高影响风险。

**不切实际的假设**：
1. architecture §2.3 列出的 API（`Launcher::new(dir)`、`launcher.install(InstallRequest)`、`launcher.load_version(id)`、`launcher.build_launch_command_from_version`）是推测性命名，未与 0.1.2 实际 API 核对。
2. spec 假设库能处理"全加载器支持"，但 Forge processor 是已知高风险点，M-1 之前不应视为确定能力。
3. spec §1.1 声称"1.6+ 保证"，但 mc-launcher-core 对旧版本（尤其是 1.6-1.7 的 legacy 格式）的支持程度未验证。

**建议**：M-1 必须在 M0 之前完成，输出一份库 API 实测报告，确认上述能力和 API 签名后再冻结设计。

---

## 六、结论

**不可进入实现阶段。**

必须先修复以下阻断性问题：

1. **[高]** 修正 technical.md `mc_version_to_java` 的 match 臂顺序，删除 `=> 16`，确保 1.17→17、1.20.5→21
2. **[高]** 按验收门槛制全面重写 plan.md，删除所有时间单位（天/周/小时/d），新增 M-1 里程碑拆解
3. **[高]** 同步 test-plan JAVA-07 为 Java 17，同步 technical Adoptium 参数移除 16
4. **[中]** 澄清下载职责划分（mc-launcher-core vs 自研 Downloader），统一 architecture 和 technical
5. **[中]** 统一 Downloader 并发模型（双 Semaphore 还是单 Semaphore），与 spec 的 32/16 并发承诺对齐
6. **[中]** 补全 Cargo.toml 缺失依赖（md-5、regex、chrono、dev-dependencies）
7. **[中]** 完成 M-1 技术预研，输出 mc-launcher-core 0.1.2 实测能力报告，再决定是否冻结设计

以上 7 项修复并通过第三轮评审后，方可进入 M0 实现。
