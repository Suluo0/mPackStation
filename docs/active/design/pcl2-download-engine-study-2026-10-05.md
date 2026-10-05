# PCL2 多线程下载引擎拆解

> 目的：mPackStation 的整合包导入需要"把清单里几十上百个 jar 拉下来"这件事。
> 2026-10-05 的教训是先自造了一版（8 路 worker + 重试 + 续传 + sha1 去重）又全部撤掉。
> 本文把 PCL2 的实现读透，作为"要么照抄设计，要么确认已有实现够用"的判断依据。

## 0. 来源

| 项 | 值 |
|---|---|
| 仓库 | `Meloong-Git/PCL`（Hex-Dragon/PCL2 的官方延续，PCL 2 源码） |
| 分支 / 快照 | `main` @ `0e0d12f`（2026-09-28） |
| 语言 | VB.NET（.NET Framework/WPF） |
| 许可 | 分发优先许可（见仓库 `LICENCE`），非标准 OSI，商用需看原文 |
| 代码同步方式 | 仓库 README 明说：**随每次 PCL 发布版本同步一次**，不实时 |
| 另有 | `MeloongCore`（C#，Apache-2.0）—— PCL 的新版通用库，本次未展开 |

本次精读的文件：

| 文件 | 行数 | 职责 |
|---|---|---|
| `Plain Craft Launcher 2/Modules/Base/ModNet.vb` | 1933 | **下载引擎全部核心**：请求、多线程引擎、下载管理器 |
| `Plain Craft Launcher 2/Modules/Minecraft/ModModpack.vb` | 860 | 整合包安装（Modrinth / CF / HMCL / MMC / MCBBS / Compress） |
| `Plain Craft Launcher 2/Modules/Minecraft/ModDownload.vb` | 1391 | 原版/依赖/资源文件清单构造 |
| `Plain Craft Launcher 2/Pages/PageSetup/Settings.vb` | — | 设置项默认值 |

---

## 1. 分层结构

```
LoaderDownload          任务单元（"下载 Mod"这一个任务）
  └─ NetFile[N]         单个文件（一个目标路径 = 一个 NetFile）
       ├─ NetSource[M]  该文件的多个下载源（URL + 失败计数 + 是否被禁用）
       └─ NetThread[K]  把文件切成 K 段，每段一个线程 —— 单向链表
NetManagerClass         全局：线程总额度、限速令牌、跨任务文件复用、存在性检查调度
```

关键点：**并发的最小单位是"段"（NetThread），不是"文件"。** 一个 100MB 的 jar 可以吃掉好几个并发额度；一个 2MB 的小 jar 只吃一个。

---

## 2. 线程模型：段链（这是精髓）

段不用"起始+长度"表示，而是**单向链表**（`NetThread.NextThread`），段的结束位置由链表里的下一个段推导：

```vb
Public ReadOnly Property DownloadEnd As Long
    Get
        If NextThread Is Nothing Then
            If Task.IsUnknownSize Then Return 1000L * 1000 * 1000 * 1000L  '约 1T
            Else Return Task.FileSize - 1
        Else
            Return NextThread.DownloadStart - 1        ' ← 边界是动态的
        End If
    End Get
End Property
```

好处：**开新段不需要改老段的数据**，只要插进链表，前一个段的 `DownloadEnd` 自动收缩，两边各下各的，互不重叠。`Task.LockChain` 串行化链表修改。

### 开新段怎么切（`TryBeginThread`，ModNet.vb:808）

1. 有段则找 `DownloadUndone` 最大的那个段（`FilePieceMax`）；
2. **门槛**：最大段剩余 `< FilePieceLimit`（256KB）就放弃，不切；
3. 切点 = `FilePieceMax.DownloadEnd - FilePieceMax.DownloadUndone * 0.4`
   —— 即从最大段的**尾部**切走 40%，不是均分；
4. 只对 `非 IsNoSplit` 的文件做这事。

### 什么文件不切（`IsNoSplit`，ModNet.vb:674）

```vb
Return IsUnknownSize OrElse FileSize < 1024 * 1024   '小于 1MB 的文件不分割
```

小文件走**单线程 + MemoryStream 内存缓冲**（`IsNoSplit` 时 `Th.Temp = Nothing`，直接写内存），连临时文件都不落盘。

### 首段与源的选择

`FirstThreadSource` 记录"首段用的是第几个源"，下一个文件的首段从 `Id+1` 开始 —— **按文件轮转下载源**，天然做负载均衡（ModNet.vb:837-838）。

---

## 3. 能力是怎么控制的（逐项）

| 能力 | 机制 | 位置 |
|---|---|---|
| **全局并发上限** | `NetTaskThreadLimit = ToolDownloadThread + 1`，默认设置值 63 → **64 个并发连接**；`NetTaskThreadCount` 计数，超了就 `Continue While` 不再开线程 | ModNet.vb:311、1742 |
| **动态加线程** | 不是无脑开满。管理器每 20ms 扫一遍：**当前全局速度 ≥ 速度下限（`NetTaskSpeedLimitLow`）就完全不追加线程**；低于才追加 | ModNet.vb:1747 |
| **速度下限自适应** | 初值 256KB/s；每 0.1s 用"近 1 秒平均速度 × 0.85"作为新下限，**只升不降**（`If Limit > NetTaskSpeedLimitLow Then`） | ModNet.vb:1698-1703 |
| **限速（用户可设）** | 令牌桶：后台线程每 100ms 补 `high/10`；每个读循环前 `While NetTaskSpeedLimitLeft <= 0 : Sleep(16)` | ModNet.vb:1779、996 |
| **限速档位** | 设置 0–14 → 0.1–1.5 MB/s；15–31 → 2–10 MB/s；32–41 → 11–20 MB/s；≥42 → **不限速**（默认 42） | ModNet.vb:326-335、Settings.vb:118 |
| **准备中线程序列控制** | 追加线程前先数：`PreparingCount > DownloadingCount` 就不加 —— 防止一堆线程都卡在连接阶段把额度占满 | ModNet.vb:1762 |
| **慢速掐断** | 数据包间隔 > 5s **且** 这段时间拿到的字节数 < 间隔毫秒数（≈ 低于 1KB/s）**且** 不是单线程 → 抛 `TimeoutException`，弃掉这一段 | ModNet.vb:1025 |
| **超时自适应** | `Timeout = clamp(平均连接耗时, 15s, 30s) × (1 + 该源失败次数)` —— 源越不靠谱给越久，但不超过 30s | ModNet.vb:909 |
| **多下载源** | `NetSource` 列表；失败即 `IsFailed`，多线程下该源被禁用，换下一个 | ModNet.vb:1063-1143 |
| **源降级单线程** | 有些源多线程会抽风。`SourcesOnce` = 只允许单线程的源；`IsRangeNotSupported` / 404 / 502 / 无返回数据 / 403 / 429 都会把源从多线程池踢到单线程池 | ModNet.vb:1079-1099 |
| **Range 不支持识别** | 校验响应 `ContentLength`：单段请求必须等于 `FileSize - DownloadStart`，不符 → `RangeNotSupportedException`。首段用 `ContentLength < 0` 判断"大小未知" | ModNet.vb:925-971 |
| **核弹级兜底** | 合并失败 / 首次下载失败 → `Retried` 标记，**清空源状态、所有源标记为"不允许断点续传"，逐个从头重试**。注释点明用途："兼容多个下载源中的一部分返回错误的文件，以及部分在多线程下载时会抽风的源" | ModNet.vb:1105-1120 |
| **任务级熔断** | `LoaderDownload.FailCount` 连续失败 ≥ `max(文件数×8, 线程上限×8+3)`（上限 1 万）→ 整个任务强制失败并汇总错误 | ModNet.vb:1350 |
| **文件校验** | `FileChecker`：Hash(sha1/md5) + ActualSize + MinSize + IsJson。**在合并之后校验**，失败则 `Retried` 走全量重试 | ModNet.vb:1205-1219 |
| **段级断点续传** | 每段写自己的临时文件 `Download\{fileUuid}_{threadUuid}_{rand}.tmp`，最后按链表顺序拼回目标路径 | ModNet.vb:984、1191-1202 |
| **已存在即复用（跨实例）** | `CheckExistingFiles`：扫 **所有 MC 文件夹的 versions/**，用 hash 找到同文件就**直接复制过去**（`IsCopy = True`），不下载。最多 8 个线程并行查，每线程至少分 40 个文件 | ModNet.vb:1455、1474-1508 |
| **跨任务去重** | `NetManager.AllFiles` 以 `LocalPath` 为键；已在下载中的文件，新任务直接复用同一个 `NetFile` 对象（`File = OngoingFile`）；任务内再 `DistinctBy(LocalPath)` 一次 | ModNet.vb:1822、1399 |
| **磁盘空间预检** | 首段拿到 `ContentLength` 后，若 > 50MB 则检查：临时目录盘要 1.1×、目标盘要 1× + 5MB | ModNet.vb:951-961 |
| **重定向 / UA 伪装** | `SecretHeadersSign(url, req, SimulateBrowserHeaders)` 按域名加 Referer/UA（绕过某些 CDN 的防盗链） | ModNet.vb:915 |
| **HTTP 层缓存协商** | 元数据请求走 CacheCow 文件缓存（`PathTemp & "Cache/Http/"`）；**文件下载不走缓存** | ModNet.vb:226 |
| **管理器分片避锁** | 两个 `ThreadStarter` 线程，按 `File.Uuid Mod 2` 各管一半文件，减少 `AllFiles` 的锁争用 | ModNet.vb:1733 |
| **对特定源的礼貌限频** | 新增线程若命中 `bmclapi`，`Sleep(100)` 降低请求频率 | ModNet.vb:1744 |

### 请求层的重试策略（与下载层分开，`NetRequestByClientRetry`）

- 第 1 次：超时 10s
- 第 2 次：等 500ms，超时放宽到 30s
- 第 3 次：**仅当**前两次累计耗时 > 5.5s 才重试，超时缩到 4s（快速试一把）
- 403 / 404 直接不重试；**429 睡 10s 再重试**
- `RequireJson` 时会校验首尾 `{}`/`[]` —— 注释写明"被 GFW 截断时可能不完整"，截断当异常抛出去触发重试

另有 `NetRequestByClientMultiple`：同 URL 起 3 个线程、每个错开 250ms 起跑，**谁先回来用谁，其余 `Interrupt()`**。用于"接口本身不稳"的场景。

---

## 4. 整合包安装怎么用这套引擎（`ModModpack.vb`）

以 Modrinth 装法为例（`InstallPackModrinth`，368 行起）：

1. 读 `modrinth.index.json` → 取 `dependencies` 里的 minecraft / neoforge / fabric-loader
2. 解压包到临时目录 → `CopyOverrideDirectory("overrides/" → 实例目录)`、再 `client-overrides/`（**后者覆盖前者**）
3. 遍历 `files[]` 构造 `NetFile`：
   - `env.client == "unsupported"` → 跳过；`optional` → **弹窗问用户**要不要下
   - 一个文件可以有多个 URL：清单自带 + `DlSourceModGet` 补国内源 → `Distinct`
   - **路径安全校验**：目标路径必须是实例文件夹的子路径，否则弹窗报错并中止（防"整合包作者写 `../../` 逃逸"）
   - 校验器**只给 sha1，不给大小** —— 注释写明"Modrinth 整合包不应校验文件大小（他们官方自己说的）"
4. 这一步作为 `LoaderDownload` 挂在总 `LoaderCombo` 上，`ProgressWeight = 文件数 × 1.5`（每个 Mod 估 1.5s）
5. 加载器安装是**并行挂载的另一个 LoaderCombo**，进度按权重合并

CF 装法（`InstallPackCurseForge`）不同之处：清单只有 `projectID/fileID`，需要先批量调 CF API 换下载地址（`LoaderTask(Of Integer, JArray)`），且**要处理"文件已被删除导致 API 返回列表比请求的短"**这个坑（注释在 232 行）。

`LoaderDownload.RefreshStat` 里还有个细节：进度按文件**加权求和**，`IsCopy`（本地复制得来的）文件只按 0.2 权重计 —— 因为复制比下载快得多，不这么算进度条会跳。

---

## 5. 与 mPackStation 现状对照

### 5.1 我们内核 `launcherCore/src/download/` 已有什么

| 能力 | 内核实现 | 与 PCL2 的差距 |
|---|---|---|
| 文件级并发 | `ConcurrentDownloader`：`assets_sem = 8`、`libraries/other_sem = 4`（tokio `Semaphore`） | ✅ 有；但**固定档位**（mods 归 `Other` → 4 并发），PCL2 是**按实测速度动态增减** |
| 单文件多段 | **没有** | ❌ 缺。内核是一个文件一个 task，PCL2 是段级切分 |
| 重试 | 5 次 + 指数退避 1/2/4/8/16s | ✅ 有；PCL2 是"分层重试 + 源降级 + 全量兜底" |
| 多源 | `get_download_urls(url, mirror)` 只对 **Mojang 系 URL** 加 bmclapi 镜像；Modrinth/CF 的 CDN 直链**没有备用源** | ⚠️ 弱。PCL2 每个文件都可挂多个源并逐个降级 |
| 竞速 | `download_with_race`：多源同时下，先成功者胜，其余 abort + 清 `.partial` | ✅ 有（比 PCL2 的"依次尝试"更激进，但对 CDN 直链无效因为只有一个源） |
| 断点续传 | `do_download_to` 按 `.partial` 已有字节数续（`Range: bytes=<n>-`） | ✅ 有；PCL2 是**段级**（每段独立 `.tmp`） |
| 校验 | `FileChecker::should_skip`（sha1/size 存在即跳过）+ 下载后 sha1 校验，失败删文件重试 | ✅ 有；PCL2 多一个"合并后校验" 
| 已存在即跳过 | ✅ 有（同路径） | ⚠️ PCL2 还能**跨实例全盘搜索同 hash 文件并复制**，我们只查同路径 |
| 限速 | 无 | ❌ 缺（对本地工具非必需） |
| 慢速掐断 | 只有整体 `DOWNLOAD_TIMEOUT = 300s` | ⚠️ 弱。PCL2 是"5s 内 <1KB/s 就掐段重来" |
| 磁盘空间预检 | 无 | ❌ 缺（我们刚在测试里踩过盘满风险） |

### 5.2 Go 侧（provider）现状

`provider.HTTPAdapter.Download` 是**单文件、单次、内存态**（`Metadata` → 选 primary file → 一次 GET → `[]byte`）。我在 2026-10-05 的收口里：

- 删掉了 Go 侧自造的 `DownloadToFile` / 8 路 worker 池 / `.partial` 续传 / sha1 去重 / CF 专用端 → **这些确实是重复造轮子**
- 但同时也把导入退回了**串行逐条 `adapter.Download`**，且下载内容进内存（132 个 jar / 975MB 逐个来）

也就是说：**删对了（不该在 Go 侧重造），但没接上（该用内核的下载层，或至少把 provider 改成流式）**。

---

## 6. 结论与可选路线

### 事实层面的结论

1. PCL2 这套引擎的**设计密度**很高，值得学的不是"8 路并发"这种数字，而是**控制逻辑**：
   - 并发额度是**全局共享的钱包**（默认 64），文件只是消费者
   - 线程数由**实测速度**驱动，速度够了就完全不加
   - 每个失败都分**源级 / 段级 / 文件级 / 任务级**四层处理，逐层降级
   - 小文件走内存、不切段；大文件才切
2. 我们内核的下载层**已经有简化版的 80%**（并发、重试、竞速、续传、校验、跳过）。缺的只有：单文件多段、动态调速、慢速掐断、Modrinth 直链的多源、磁盘预检。
3. 2026-10-05 那版 Go 实现的问题不是"想做多线程"，而是**没认识到数据面已经在内核里**。

### 可选路线（需要你定，我再动手）

| 路线 | 做法 | 代价 |
|---|---|---|
| **A. Go 侧只做控制面，数据面全交内核** | 导入 handler 解析清单 → 写临时 `.mrpack` → 调 `runner.InstallFromMrpack` → 读内核返回的计数入库。一行下载代码都不写 | 内核的 `install` 语义是"装到实例目录"，与工作台的项目目录模型需要对齐；内核二进制要先 `cargo build --release` |
| **B. 给内核补齐 PCL2 的缺失项** | 在 `download/` 里加：动态并发（按实测速度）、段级切分、慢速掐断、磁盘预检、Modrinth 直链多源 | 改 Rust 内核，需重新真机验证 |
| **C. Go 侧照 PCL2 设计重写一遍** | 全盘照抄段链模型 | 就是 2026-10-05 之前那版的错误方向，只是抄得好看些 |

倾向 **A 为主、B 按需补**。C 不推荐。

---

## 附：核查方式

本文所有行号引用来自上文 `0e0d12f` 快照的本地副本（`/tmp/pcl2/all/`，112 个 `.vb` 全量下载后 grep 定位）。仓库未克隆、未修改。
