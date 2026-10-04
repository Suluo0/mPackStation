# 模组内容提取引擎测试文档

## 1. 测试策略

| 层级 | 范围 | 工具 |
|---|---|---|
| 单元测试 | 解析器纯函数（ExtractModContent / parseRecipe / 元数据解析 / 动态配方识别） | Go testing + table-driven |
| 集成测试 | service 层（SubmitParseModContent / ListModContent / 任务 handler 落库）+ store 层（事务/幂等/分页） | Go testing + 内存 SQLite |
| API 契约测试 | HTTP 端点输入输出 / 错误码 / 状态码 | httptest + 参照 content_acceptance_test.go 模式 |
| 固定验证 | `go test ./...` / `go vet ./...` / `npm run build` | 命令行 |

## 2. 单元测试清单

### 2.1 `TestExtractModContent_MinimalJar`

- 输入：动态构造的 minimal jar（1 fabric.mod.json + 1 标准配方 + 1 special 动态配方 + 1 物品模型 + 1 worldgen structure）
- 断言：
  - `ModID == "testmod"`、`Version == "1.0.0"`、`Loader == "fabric"`
  - Recipes 长度 == 2
  - 标准配方：`IsDynamic == false`，payload 含 `type`/`ingredients`/`result`
  - special 配方：`IsDynamic == true`，payload 仅含 `{"type":"testmod:wildcard"}`
  - Items 长度 == 1，key == `testmod:magic_dirt`
  - Structures 长度 == 1
  - Stats.TotalFiles == 5（或实际条目数）

### 2.2 `TestExtractModContent_NeoforgeMetadata`

- 输入：含 `META-INF/neoforge.mods.toml` 的 jar
- 断言：`Loader == "neoforge"`，ModID/Version 从 toml 正确提取

### 2.3 `TestExtractModContent_ForgeMetadata`

- 输入：含 `META-INF/mods.toml` 的 jar
- 断言：`Loader == "forge"`

### 2.4 `TestParseRecipe_StandardTypes`（table-driven）

覆盖：
- `minecraft:crafting_shaped`（含 pattern/key）
- `minecraft:crafting_shapeless`
- `minecraft:smelting` / `blasting` / `stonecutting`
- `minecraft:smithing_transform`
- 自定义类型 `ae2:inscriber`（保留完整原始 JSON，IsDynamic=false）

断言：type 正确提取，ingredients/result 保留，IsDynamic=false。

### 2.5 `TestParseRecipe_DynamicDetection`

- 路径 `data/x/recipe/special/wildcard.json` + 内容 `{"type":"x:wildcard"}` → IsDynamic=true
- 路径 `data/x/recipe/normal.json` + 内容 `{"type":"x:custom","ingredients":[],"result":{}}` → IsDynamic=false（自定义但非 special）
- 路径含 special 但有完整 ingredients/result → IsDynamic=false（special 目录不强制 dynamic，以内容判据为准）

### 2.6 `TestExtractModContent_InvalidJsonSkipped`

- 输入：jar 含 1 个有效配方 + 1 个损坏 JSON 配方
- 断言：有效配方正常解析，损坏配方记录 `parse_error != ""`，整体不报错，Stats.ErrorCount == 1

### 2.7 `TestExtractModContent_ZipBombRejected`

- 输入：构造一个声明超大解压尺寸的 zip（或条目数 > 100000）
- 断言：返回错误 `jar_too_many_entries` 或 `jar_too_large`

### 2.8 `TestExtractModContent_PathTraversalRejected`

- 输入：zip 含 `../../etc/passwd` 条目名
- 断言：该条目被跳过（不读取内容），不 panic，记录 parse_error

### 2.9 `TestExtractModContent_EmptyJar`

- 输入：空 zip（无 data/ 无 fabric.mod.json）
- 断言：返回空 ExtractedContent，ModID=""，Loader=""，不报错

### 2.10 `TestExtractModContent_NotAZip`

- 输入：纯文本字节
- 断言：返回错误 `jar_invalid`

## 3. 集成测试清单

### 3.1 `TestSubmitParseModContent_CreatesTask`

- 前置：创建 pack + 已安装 mod（含 project_id/version_id，provider mock 返回 jar 字节）
- 调用：`SubmitParseModContent(ctx, packID, modID)`
- 断言：返回 task，kind == `parse_mod_content`，payload 含 packId/modId

### 3.2 `TestHandleParseModContentTask_FullFlow`

- 前置：pack + mod + provider mock（返回 minimal jar 字节）
- 直接调用：`HandleParseModContentTask(ctx, execution)`
- 断言：
  - task 状态 succeeded
  - `mod_content` 表有对应行（按 mod_id 查询）
  - `mod_content_runs` 表有 succeeded 记录，sha1 匹配
  - activity 表有 `mod_content.parsed` 记录

### 3.3 `TestHandleParseModContentTask_Idempotent`

- 前置：同 mod 同 sha1 已解析成功
- 再次调用 handler
- 断言：直接 succeed（不重新解析），`mod_content` 行数不变，run 不新增

### 3.4 `TestHandleParseModContentTask_VersionChangeReParses`

- 前置：mod 已解析（sha1=A）
- 更新 mod 的 sha1 为 B（模拟版本升级）
- 再次触发解析
- 断言：重新解析，旧 mod_content 行被替换，run 新增一条（sha1=B）

### 3.5 `TestReplaceModContent_Atomic`

- 前置：mod 有旧解析结果
- 调用 `ReplaceModContent` 传入新结果
- 断言：事务内旧行全部删除 + 新行全部插入；模拟中途失败时旧行不丢失（回滚）

### 3.6 `TestListModContent_Pagination`

- 前置：mod 有 150 条 recipe
- 调用 `ListModContent(limit=100, cursor="")`
- 断言：返回 100 条 + next_cursor 非空
- 用 next_cursor 再次调用 → 返回剩余 50 条 + next_cursor 为空
- total == 150

### 3.7 `TestListModContent_KindFilter`

- 前置：mod 有 recipe + item_model + structure
- 调用 `kind=recipe` → 仅返回 recipe
- 调用 `kind=item_model` → 仅返回 item_model
- 调用无效 kind → 返回错误 `invalid_kind`

### 3.8 `TestGetModContentRun_NotParsed`

- 前置：mod 从未解析
- 调用 `GetModContentRun` → 返回 NotFound（service 层转 `content_not_parsed`）

### 3.9 `TestHandleParseModContentTask_LocalModNoBytes`

- 前置：local 来源 mod，blobstore 无对应 sha1
- 调用 handler
- 断言：task 失败，error_code == `jar_bytes_unavailable`，run 状态 failed

## 4. API 契约测试清单

### 4.1 `TestPOST_Parse_202`

- 请求：`POST /api/packs/{packId}/mods/{modId}/content/parse`
- 断言：202，body 含 `taskId`/`status=queued`

### 4.2 `TestPOST_Parse_404_ModNotFound`

- 不存在的 modId → 404 `mod_not_found`

### 4.3 `TestPOST_Parse_409_Idempotent`

- 同 sha1 已解析成功 → 409 `parse_already_current`

### 4.4 `TestGET_Content_200`

- 前置：已解析
- 断言：200，body 含 `items`/`next_cursor`/`total`/`run`，run.status == succeeded

### 4.5 `TestGET_Content_404_NotParsed`

- 未解析 → 404 `content_not_parsed`

### 4.6 `TestGET_Content_KindFilter`

- `?kind=recipe` → 仅 recipe
- `?kind=invalid` → 400 `invalid_kind`

### 4.7 `TestGET_ContentRun_200`

- 已解析 → 200，含 run 详情
- 未解析 → 404

### 4.8 `TestGET_ContentById_200`

- 有效 contentId → 200，含 item 详情
- 无效 contentId → 404

### 4.9 `TestGET_Content_PaginationCursor`

- limit=1 → next_cursor 非空；用 cursor 翻页 → 下一条

## 5. fixture 说明

### 5.1 minimal jar（动态构造，不提交二进制）

在 `mod_content_extract_test.go` 的 `TestMain` 或辅助函数中用 `archive/zip` 动态构造：

| 条目 | 内容 | 预期 kind |
|---|---|---|
| `fabric.mod.json` | `{"id":"testmod","name":"Test Mod","version":"1.0.0"}` | metadata |
| `data/testmod/recipe/simple.json` | 标准 crafting_shapeless 配方 | recipe（非 dynamic） |
| `data/testmod/recipe/special/wildcard.json` | `{"type":"testmod:wildcard"}` | recipe（dynamic） |
| `assets/testmod/models/item/magic_dirt.json` | `{}` | item_model |
| `data/testmod/worldgen/structure/test_ruin.json` | `{"type":"minecraft:jigsaw"}` | structure |

### 5.2 provider mock

参照现有 `provider/http_adapter_test.go` 模式，用 `httptest.Server` mock Modrinth API：
- `/v2/project/{id}/version` → 返回版本列表
- 下载 URL → 返回 minimal jar 字节

### 5.3 测试数据库

所有集成测试使用内存 SQLite（参照 `v7_schema_acceptance_test.go` 的 `openTestDB` 模式），migration 自动应用。

## 6. 验收清单

- [ ] 所有单元测试通过（解析器覆盖标准配方/动态配方/元数据/异常）
- [ ] 所有集成测试通过（任务全流程/幂等/版本变更/事务/分页）
- [ ] 所有 API 契约测试通过（4 个端点 × 成功/失败路径）
- [ ] `go test ./...` 全量通过（无回归）
- [ ] `go vet ./...` 无警告
- [ ] `npm run build` 通过（前端未改，确认无破坏）
- [ ] migration 0008 可从 0007 干净升级（旧数据保留）
- [ ] migration 0008 可全新安装（空库直接到 0008）
- [ ] special 动态配方正确标记 is_dynamic
- [ ] zip bomb / 路径遍历 / 超大文件防护生效
- [ ] 同 sha1 重复触发幂等（不重复解析、不重复落库）
- [ ] activity/outbox/audit 记录完整（参照 addP6Evidence 模式）
- [ ] 代码不违反分层规则（httpapi 不写 SQL、store 唯一 SQL 层、service 承担业务编排）

## 7. 非目标（不测试）

- 不测试真实 Modrinth/CurseForge 网络调用（用 mock）
- 不测试前端集成（本能力不改前端）
- 不测试字节码层动态配方判据 B（未实现）
- 不测试自动触发（未实现）
- 不测试 jar 字节持久化到 blobstore（未实现）
