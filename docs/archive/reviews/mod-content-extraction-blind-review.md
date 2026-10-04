# 模组内容提取引擎 — 盲审盲测报告

> 审查日期：2026-09-06
> 审查范围：`parse_mod_content` 任务 kind、migration 0008、store/service/httpapi 三层实现及全部测试
> 审查方式：独立盲审（不了解开发过程，仅基于文档与代码）

---

## 一、总体结论

**评级：需修改后合并（Medium-Low）**

核心功能闭环完整，分层合规，migration 安全，全部测试通过。但存在 **1 个已确认的数据正确性缺陷**（run.error_count 始终为 0）、若干文档-代码不一致和错误码映射偏差，以及测试覆盖缺口。修复成本低，建议修复后合并。

---

## 二、盲测结果

### 2.1 固定验证命令

| 命令 | 结果 | 摘要 |
|---|---|---|
| `go test ./... -count=1` | ✅ 通过 | 全部 10 个包通过，httpapi 2.478s、service 2.073s、store 0.802s、task 0.417s |
| `go vet ./...` | ✅ 通过 | 无任何警告 |
| `npm run build` | ✅ 通过 | tsc + vite build 成功，4945 modules，6.29s |

### 2.2 解析器边界测试

| 用例 | 结果 | 说明 |
|---|---|---|
| 路径遍历（`../../etc/passwd`） | ✅ 条目被跳过 | 仅解析合法 recipe，evil 条目不产生 ContentItem |
| 超大单条目（51MB stored） | ✅ 条目被跳过 | 但**未记录 parse_error**（见问题 M-02） |
| 超多条目（100001 条） | ✅ 返回 `ErrJarTooManyEntries` | 在遍历前检查 `len(zr.File)` |
| 非 zip 输入 | ✅ 返回错误 | `zip.NewReader` 失败 → `ErrJarInvalid` |
| 空 jar | ✅ 返回空 ExtractedContent | ModID=""、ParsedCount=0，不报错 |

### 2.3 API 契约测试

| 端点 | 场景 | 结果 |
|---|---|---|
| POST `/content/parse` | 正常触发 | ✅ 202，body 含 `taskId` + `status=queued` |
| POST `/content/parse` | mod 不存在 | ✅ 404（但 code=`pack_not_found`，见问题 M-04） |
| POST `/content/parse` | 同 sha1 已解析 | ✅ 409，code=`parse_already_current` |
| GET `/content` | 未解析 | ✅ 404，code=`content_not_parsed` |
| GET `/content` | 已解析 | ✅ 200，含 items/next_cursor/total/run |
| GET `/content?kind=recipe` | kind 过滤 | ✅ 200，total=2 |
| GET `/content?kind=bogus` | 无效 kind | ✅ 400 |
| GET `/content/run` | 未解析/已解析 | ✅ 404 / 200 |
| GET `/content/{id}` | 存在/不存在 | ✅ 200 / 404 |
| GET `/content?limit=1&cursor=...` | 分页翻页 | ✅ cursor 有效，翻页不重复 |

### 2.4 幂等性验证

通过临时数据库级测试确认：
- 连续两次 `ParseModContentSync`（同 sha1）后，`mod_content` 表行数不变（5 行）
- `mod_content_runs` 表仅 1 条记录，status=succeeded
- 第二次解析在 `GetModContentRunBySHA` 检查处短路，不重新下载/解析/落库

---

## 三、文档一致性问题

### S-01【中等】`error_count` 语义与实现不符 — 已确认为 Bug

- **设计文档** §3.2：`mod_content_runs.error_count` 为 `INTEGER NOT NULL DEFAULT 0`，用于记录解析失败条目数。
- **测试文档** §2.6：损坏 JSON 配方应记录 `parse_error`，`Stats.ErrorCount == 1`。
- **代码** `mod_content_task.go:281-293` `finishModContentRun`：函数签名无 `errorCount` 入参，内部仅根据 `errorMessage != ""` 设为 0 或 1。成功路径（line 185）传入 `errorMessage=""`，导致 `error_count` 始终为 0，**完全忽略 `extracted.Stats.ErrorCount`**。
- **验证**：构造含 1 个无效 JSON 配方的 jar，解析成功后 `run.error_count=0`（期望 1）。
- **影响**：API 返回的 `run.errorCount` 永远为 0，下游无法感知部分解析失败。
- **修复**：`finishModContentRun` 增加 `errorCount int` 入参，成功调用时传入 `extracted.Stats.ErrorCount`；失败路径仍传 0 或 1。

### S-02【轻微】`mod_content.key` / `modid` 列默认值不一致

- 设计文档 §3.1：`key TEXT NOT NULL`、`modid TEXT NOT NULL`（无 DEFAULT）。
- migration 0008：`key TEXT NOT NULL DEFAULT ''`、`modid TEXT NOT NULL DEFAULT ''`。
- 影响极小（service 层总是赋值），但文档应同步。

### S-03【轻微】Quilt loader 支持未在设计文档列出

- 设计文档 §2.1 元数据行仅列 fabric/neoforge/forge。
- 代码 `mod_content_extract.go:159` 额外支持 `quilt.mod.json`。
- 属于正向扩展，建议补入设计文档。

### S-04【轻微】开发文档标注 `mod_repo.go 新增 GetPackMod` 不准确

- `GetPackMod` 已存在于 `mod_repo.go:79`，非本次新增。
- 开发文档文件清单应更正为"无修改"或删除该条目。

---

## 四、分层合规问题

**结论：全部合规，无违规。**

| 约束 | 检查结果 |
|---|---|
| httpapi 不写 SQL / 不调 Provider | ✅ `routes_mod_content.go` 仅调用 `service.API` 方法；`httpapi.go` 仅注册路由和 handler |
| store 是唯一 SQL 层 | ✅ `mod_content_repo.go` 全部参数化 SQL；service 层无直接 SQL |
| service 承担业务规则/事务编排 | ✅ `parseAndPersistModContent` 编排校验→幂等→下载→解析→落库→证据 |
| task 无领域逻辑 | ✅ `task.go` 仅新增 `KindParseModContent` 常量和白名单条目 |
| provider 唯一外部 HTTP | ✅ `fetchModJarBytes` 通过 `a.p5Adapter(mod.Source).Download()` 获取字节，未自行创建 http.Client |

### L-01【中等】落库与 run 状态更新不在同一事务

- 设计文档 §4.3 步骤 5："事务：删除旧 mod_content 行 → 批量插入新行 → upsert mod_content_runs"。
- 代码：`ReplaceModContent`（内部 `WithTx` 事务）与 `finishModContentRun`（独立 DB 调用）分离。
- 风险：若 `ReplaceModContent` 成功但 `finishModContentRun` 失败，内容已替换但 run 停留在 `running`，后续幂等检查（`GetModContentRunBySHA` status=succeeded）会失败，导致重复解析。
- 修复：将 run 状态更新纳入 `ReplaceModContent` 的同一事务，或在 store 层新增 `ReplaceModContentAndFinishRun` 原子方法。

---

## 五、安全与资源约束问题

### SEC-01【中等】任务错误信息泄露内部细节

- `mod_content_task.go` 中多处 `TaskError.Message = err.Error()`：
  - line 112：`invalid_payload` 暴露 JSON 原始错误
  - line 135：`mod_not_found` 暴露 store 层错误
  - line 155：`db_error` 暴露 SQL 错误
  - line 164：`jar_fetch_failed` 暴露 provider 错误（可能含 URL/token）
  - line 173：`extract_failed` 暴露内部错误
- 架构基线 v7 §8："错误信息不泄漏 SQL、绝对路径、上游 token 或内部栈；详细栈只进受控日志。"
- 这些 message 写入 `tasks.error_message`，可通过任务 API 暴露。
- 修复：`TaskError.Message` 使用稳定用户文案，原始 `err` 仅写入结构化日志（`slog.Error`），通过 `request_id`/`task_id` 关联。

### SEC-02【中等】超大条目被静默跳过，不记录 parse_error

- 设计文档 §8："单条目解压上限：50MB"。
- 开发文档 §步骤 4："单条目 > 50MB 跳过并记录 parse_error"。
- 代码 `mod_content_extract.go:98-100`：`if f.UncompressedSize64 > modContentMaxEntryBytes { continue }` —— 直接跳过，不产生 ContentItem，不计入 Stats，不记录任何错误。
- 影响：用户无法得知有内容因过大被丢弃；`total_files` 和 `error_count` 均不反映该条目。
- 修复：跳过前构造一个 `ContentItem{Kind: 从路径推导, Path: f.Name, ParseError: "entry exceeds 50MB limit"}` 并追加到对应切片，或至少递增 `Stats.ErrorCount`。

### SEC-03【轻微】`modContentMaxJSONDepth` 常量定义但未使用

- `mod_content_extract.go:24`：`modContentMaxJSONDepth = 32`。
- 全文无引用。JSON 解析使用 `json.Unmarshal`，无深度限制。
- 修复：要么实现深度限制（自定义 `json.Decoder` + 递归计数），要么删除该常量并从设计文档 §8 移除"JSON 解析深度上限：32 层"。

### SEC-04【轻微】总解压上限基于声明值而非实际值

- `mod_content_extract.go:101`：`totalDecompressed + int64(f.UncompressedSize64) > modContentMaxTotalBytes` 使用 zip 头声明的解压大小。
- 恶意 zip 可声明极小解压尺寸但实际高压缩比，绕过总上限。
- 缓解：单条目 `readModContentZipEntry` 使用 `io.LimitReader(rc, 50MB+1)` 限制实际读取，内存峰值受单条目上限约束。
- 建议：在 `readModContentZipEntry` 中累加实际读取字节数，用于总上限判断。

### SEC-05【轻微】路径遍历检查误判合法名称

- `mod_content_extract.go:95`：`strings.Contains(name, "..")` 会跳过 `data/my..mod/recipe/x.json` 等合法名称。
- 因代码不写磁盘（仅读入内存），无安全风险，但可能漏解析合法模组内容。
- 修复：改用 `path.Clean(name)` 后检查是否以 `..` 开头，或逐段检查路径组件。

---

## 六、幂等性问题

### ID-01【轻微】stale "running" run 无恢复机制

- 若进程在创建 running run（line 151）之后、finishModContentRun（line 185）之前崩溃，该 run 永远停留在 `running`。
- `GetModContentRunBySHA` 检查 `status == "succeeded"`，因此不会误判幂等，但 `LatestModContentRun` 会返回 running 状态，`GET /content/run` 显示 running 而非实际失败。
- 建议：启动时扫描 running 超过 N 分钟的 mod_content_runs，标记为 failed；或在任务恢复时联动更新。

---

## 七、动态配方识别

**判据 A 实现正确，风险可控。**

- 代码 `mod_content_extract.go:243-256`：路径含 `/recipe/special/` 且无 `ingredients`/`ingredient`/`result`/`output` → `IsDynamic=true`。
- 测试 `TestParseRecipe_DynamicDetection` 覆盖 4 种场景，包括 special 目录但有完整内容（不误判）。
- **漏判风险**：非 special 目录下的动态配方（如部分 mod 将动态配方放在 `recipe/` 根目录）不会被标记。设计文档已明确"判据 A 足以覆盖绝大多数"，判据 B（字节码）为后续增强，符合预期。
- **误判风险**：special 目录下仅有 type 字段的合法静态配方会被误标为 dynamic。但 Minecraft 约定中 special 目录确实用于动态配方，风险可接受。

---

## 八、测试覆盖缺口

| 测试文档要求 | 实现状态 | 缺口说明 |
|---|---|---|
| `TestReplaceModContent_Atomic`（事务回滚） | ❌ 缺失 | 测试文档 §3.5 明确要求，无对应测试 |
| 超大条目记录 parse_error | ⚠️ 部分 | `TestExtractModContent_LargeEntrySkipped` 仅验证跳过，未验证 parse_error（与代码缺陷 SEC-02 相关） |
| `jar_too_large`（jar >100MB） | ❌ 缺失 | 测试文档 §2.7 提到，仅测试了 too_many_entries |
| `mod_not_installed`（422）API 测试 | ❌ 缺失 | 错误码表列出，无 API 层测试 |
| `jar_bytes_unavailable` API 测试 | ❌ 缺失 | 仅 service 层 `TestHandleParseModContentTask_LocalModNoBytes`，无 HTTP 层测试 |
| `TestPOST_Parse_409` 幂等 | ✅ 已实现 | — |
| 分页 cursor 格式 | ✅ 已实现 | cursor 为 content id，`id > ?` 分页 |
| fixture 含所有 kind | ⚠️ 部分 | minimal jar 含 metadata/recipe/item_model/structure，缺 worldgen(非structure)/loot_table/advancement/tag |

**fixture 真实性**：minimal jar 由测试代码动态构造（`buildMinimalJar`），含 fabric.mod.json + 标准配方 + special 动态配方 + 物品模型 + worldgen structure，符合开发文档 §5 要求。但未覆盖 loot_table/advancement/tag 等 kind 的解析路径。

---

## 九、错误码映射问题

### E-01【中等】mod 不存在返回 `pack_not_found` 而非 `mod_not_found`

- `SubmitParseModContent` line 76-81：`GetPackMod` 失败或 `mod.PackID != packID` 均返回 `store.ErrNotFound`。
- `httpapi.go:253` `writeError`：`service.IsNotFound(err)` → 404 `pack_not_found`。
- 设计文档错误码表：`mod_not_found` → 404。
- 修复：在 service 层将 `store.ErrNotFound` 包装为带 code 的 `DomainError{Status:404, Code:"mod_not_found"}`，或在 `writeError` 中对 mod content 端点的 NotFound 做区分。

### E-02【中等】mod 未安装返回 400 `invalid_argument` 而非 422 `mod_not_installed`

- `SubmitParseModContent` line 82-84：`mod.Status != "installed"` 返回 `ErrInvalidArgument`。
- `writeError` line 257：→ 400 `invalid_argument`。
- 设计文档错误码表：`mod_not_installed` → 422。
- 修复：返回 `DomainError{Status:422, Code:"mod_not_installed", Message:"mod is not installed"}`。

---

## 十、migration 安全

| 检查项 | 结果 |
|---|---|
| 新建 migration 0008 而非修改已有 | ✅ |
| tasks 表重建为 tasks_v10，INSERT...SELECT 保留旧数据 | ✅ |
| tasks.kind CHECK 包含 `parse_mod_content` | ✅ |
| mod_content / mod_content_runs 外键 ON DELETE CASCADE | ✅ |
| UNIQUE(mod_id, kind, path) 支持幂等重解析 | ✅ |
| UNIQUE(mod_id, sha1) 作为幂等去重依据 | ✅ |
| `CurrentSchemaVersion = 8` | ✅ |
| 全新安装可直接到 0008 | ✅（migration 顺序无间隙，checksum 校验通过） |

---

## 十一、具体修改建议（按优先级）

### P0 — 必须修复

1. **【S-01】修复 `finishModContentRun` error_count**
   - 文件：`service/mod_content_task.go`
   - 操作：函数签名增加 `errorCount int` 参数；line 185 成功调用传入 `extracted.Stats.ErrorCount`；失败路径传入 0。
   - 测试：新增含无效 JSON 的 jar，断言 `run.ErrorCount > 0`。

2. **【L-01】落库与 run 状态更新原子化**
   - 文件：`store/mod_content_repo.go` + `service/mod_content_task.go`
   - 操作：新增 `ReplaceModContentAndFinishRun(ctx, packID, modID, items, run)` 在同一 `WithTx` 内执行删旧+插新+upsert run；service 层调用该方法替代分离的两步。

### P1 — 应该修复

3. **【SEC-01】TaskError.Message 脱敏**
   - 文件：`service/mod_content_task.go`
   - 操作：所有 `TaskError{Message: err.Error()}` 改为稳定文案；原始错误用 `slog.Error("parse failed", "error", err, "task_id", ...)` 记录。

4. **【SEC-02】超大条目记录 parse_error**
   - 文件：`service/mod_content_extract.go`
   - 操作：`continue` 前根据路径推导 kind，构造 `ContentItem{ParseError: "entry exceeds 50MB limit"}` 并追加到输出；递增 `Stats.ErrorCount`。

5. **【E-01/E-02】修正错误码映射**
   - 文件：`service/mod_content_task.go` + `httpapi/httpapi.go`
   - 操作：mod 不存在 → `DomainError{404, "mod_not_found"}`；mod 未安装 → `DomainError{422, "mod_not_installed"}`。

### P2 — 建议修复

6. **【SEC-03】删除或实现 JSON 深度限制**
7. **【SEC-04】总解压上限基于实际读取字节**
8. **【SEC-05】路径遍历改用 path.Clean 逐段检查**
9. **【ID-01】启动时恢复 stale running run**
10. **【S-02/S-03/S-04】同步文档（key/modid 默认值、quilt、GetPackMod 标注）**
11. 补充缺失测试：`TestReplaceModContent_Atomic`、`jar_too_large`、`mod_not_installed` API、`jar_bytes_unavailable` API、fixture 增加 loot_table/advancement/tag 条目。

---

## 十二、审查总结

| 维度 | 评级 | 说明 |
|---|---|---|
| 文档一致性 | 中等 | error_count Bug + 3 处轻微不一致 |
| 分层合规 | 良好 | 无违规，仅事务原子性需加强 |
| 安全与资源约束 | 中等 | 错误信息泄露 + 超大条目静默丢弃 + 未使用的深度常量 |
| 幂等性 | 良好 | 核心幂等有效，仅 stale running 无恢复 |
| 动态配方识别 | 良好 | 判据 A 正确，符合设计预期 |
| 测试覆盖 | 中等 | 主体覆盖完整，缺事务回滚/错误码/部分 kind fixture |
| migration 安全 | 优秀 | 完全合规 |
| 契约完整性 | 中等 | 2 个错误码映射偏差 |

**最终结论：需修改后合并。** P0 两项（error_count 修复 + 事务原子化）必须在合并前完成；P1 建议同批修复；P2 可作为后续改进项。
