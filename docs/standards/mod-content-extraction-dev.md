# 模组内容提取引擎开发文档

## 1. 文件清单

### 1.1 新增文件

| 文件 | 职责 |
|---|---|
| `apps/server/internal/store/migrations/0008_mod_content.sql` | 建表 mod_content / mod_content_runs + tasks kind 扩展 |
| `apps/server/internal/store/mod_content_repo.go` | mod_content / mod_content_runs 的参数化 SQL + 事务 |
| `apps/server/internal/service/mod_content_extract.go` | 纯解析器（zip 遍历 + 分类 + 配方/元数据解析 + 动态配方识别） |
| `apps/server/internal/service/mod_content_task.go` | 任务 handler + API 方法（触发/查询/落库编排） |
| `apps/server/internal/httpapi/routes_mod_content.go` | HTTP 路由注册 |
| `apps/server/internal/service/mod_content_extract_test.go` | 解析器单元测试 |
| `apps/server/internal/service/mod_content_task_test.go` | 服务层测试 |
| `apps/server/internal/httpapi/mod_content_acceptance_test.go` | API 契约测试 |
| `apps/server/internal/testdata/mod-content-minimal.jar` | 测试 fixture（最小可解析 jar） |

### 1.2 修改文件

| 文件 | 改动 |
|---|---|
| `apps/server/internal/task/task.go` | 新增 `KindParseModContent Kind = "parse_mod_content"` |
| `apps/server/internal/httpapi/httpapi.go` | NewRouterWithProviders 中注册 task handler；newRouter 中注册路由 |
| `apps/server/internal/store/legacy_upgrade.go` | 若需要，补充 0008 的 legacy 列迁移（通常不需要，全新表） |

## 2. 实现步骤（按依赖顺序）

### 步骤 1：task kind 定义

`task/task.go` 常量区新增：

```go
// KindParseModContent extracts data-driven game content from a mod jar.
KindParseModContent Kind = "parse_mod_content"
```

同时在 `task.go` 的 kind 白名单（约 1068 行 `case KindResolve, ...`）中加入 `KindParseModContent`。

### 步骤 2：migration 0008

参照 0007 的表重建模式扩展 tasks.kind，同时新建两张表。完整 SQL 见 `migrations/0008_mod_content.sql`。

关键点：
- tasks 表重建为 `tasks_v10`，kind CHECK 加入 `'parse_mod_content'`
- `mod_content` 表 `UNIQUE(mod_id, kind, path)` 支持幂等重解析
- `mod_content_runs` 表 `UNIQUE(mod_id, sha1)` 作为幂等去重依据
- 所有外键 `ON DELETE CASCADE`（pack 删除时级联清理）

### 步骤 3：store 层

`store/mod_content_repo.go` 实现：

```go
type ModContentRecord struct {
    ID, PackID, ModID, Modid, Version, Kind, Path, Key, Payload, ParseError string
    IsDynamic, ParsedAt int64
}

type ModContentRunRecord struct {
    ID, PackID, ModID, SHA1, Status, ErrorMessage string
    TotalFiles, ParsedCount, DynamicCount, ErrorCount int
    StartedAt, FinishedAt int64
}

func (r *Repository) UpsertModContentRun(ctx, run ModContentRunRecord) error
func (r *Repository) GetModContentRun(ctx, modID, sha1 string) (ModContentRunRecord, error)
func (r *Repository) ReplaceModContent(ctx, packID, modID string, items []ModContentRecord) error  // 事务：删旧+插新
func (r *Repository) ListModContent(ctx, packID, modID, kind string, limit int, cursor string) ([]ModContentRecord, string, int, error)
func (r *Repository) GetModContent(ctx, packID, modID, id string) (ModContentRecord, error)
func (r *Repository) LatestModContentRun(ctx, modID string) (ModContentRunRecord, error)
```

`ReplaceModContent` 必须在事务内执行（删旧行 + 批量插新行 + 更新 run 状态），参照 `content_repo.go` 的 `WithTx` 模式。

### 步骤 4：解析器（纯函数层）

`service/mod_content_extract.go` 核心：

```go
func ExtractModContent(r io.Reader) (*ExtractedContent, error)
```

实现要点：
1. `zip.NewReader(io.SectionReader)` —— 注意 `zip.NewReader` 需要 `io.ReaderAt`，调用方先把字节读入 `bytes.Reader`
2. 遍历 `z.File`，按路径前缀分类：
   - `fabric.mod.json` / `META-INF/neoforge.mods.toml` / `META-INF/mods.toml` → 元数据
   - `data/*/recipe/**/*.json` → 配方
   - `assets/*/models/item/**/*.json` → 物品模型
   - `data/*/worldgen/structure/**/*.json` / `data/*/structures/**/*.nbt` → 结构
   - `data/*/worldgen/**/*.json`（非 structure）→ 地形
   - `data/*/loot_table/**/*.json` → 战利品
   - `data/*/advancement/**/*.json` → 进度
   - `data/*/tags/**/*.json` → 标签
3. 每条目调用对应 parser，单条失败不中断整体（记录 `parse_error`，继续）
4. 资源约束：条目数 > 100000 直接返回错误；单条目 > 50MB 跳过并记录 parse_error；总解压 > 500MB 停止后续条目

配方 parser：
```go
func parseRecipe(path string, data []byte) ContentItem
```
- JSON 解码为 `map[string]any`
- 提取 `type` 字段
- 判据 A：路径含 `/recipe/special/` 且无 `ingredients`/`result`/`output` → `IsDynamic=true`
- 标准配方：提取 ingredients（保留原始结构）、result/output（id + count）
- 自定义类型：保留完整原始 JSON
- key = 从路径推导（`data/ae2/recipe/misc/facade.json` → `ae2:misc/facade`）

元数据 parser：
- fabric.mod.json：JSON 解码，取 `id`/`name`/`version`
- neoforge.mods.toml / mods.toml：简易 TOML 解析（正则提取 `modId`/`version`，不引入第三方 toml 库）
- loader 类型由文件来源推断

### 步骤 5：任务 handler + API 方法

`service/mod_content_task.go`：

```go
func (a *API) HandleParseModContentTask(ctx context.Context, ex *task.Execution) error
func (a *API) SubmitParseModContent(ctx, packID, modID string) (*task.Task, bool, error)
func (a *API) ListModContent(ctx, packID, modID, kind string, limit int, cursor string) ([]ModContentItem, string, int, *ModContentRun, error)
func (a *API) GetModContent(ctx, packID, modID, contentID string) (ModContentItem, error)
func (a *API) GetModContentRun(ctx, packID, modID string) (*ModContentRun, error)
```

任务 handler 流程（详见设计文档 §4.3）：
1. 解码 payload → packID/modID
2. 查 pack_mods 拿 sha1/project_id/version_id/source
3. 幂等检查：`LatestModContentRun(modID)` 若同 sha1 且 succeeded → 直接 succeed
4. 获取 jar 字节：`a.fetchModJarBytes(ctx, mod)`（内部方法，先 blobstore 后 provider）
5. 调用 `ExtractModContent(bytes.NewReader(jarBytes))`
6. 事务落库：`ReplaceModContent` + 更新 run 状态
7. 写 activity/outbox/audit（kind=`mod_content`，action=`mod_content.parsed`）
8. `ex.Succeed(ctx, "extracted N items")`

### 步骤 6：HTTP 路由

`httpapi/routes_mod_content.go`：

```go
func registerModContentRoutes(mux *http.ServeMux, app *service.API)
```

四个端点（详见设计文档 §5）。在 `httpapi.go` 的 `newRouter` 中调用 `registerModContentRoutes(mux, app)`。

任务 handler 注册在 `NewRouterWithProviders` 中：
```go
_ = q.RegisterHandler(task.KindParseModContent, task.HandlerFunc(app.HandleParseModContentTask))
```

## 3. API 契约

### 3.1 POST /api/packs/{packId}/mods/{modId}/content/parse

请求体：空（或 `{}`）

成功响应 `202`：
```json
{ "taskId": "task-xxx", "status": "queued" }
```

幂等响应 `409`：
```json
{ "code": "parse_already_current", "message": "content already parsed for this jar version" }
```

### 3.2 GET /api/packs/{packId}/mods/{modId}/content

Query：`kind`（可选，recipe/item_model/structure/worldgen/loot_table/advancement/tag）、`limit`（默认 100，最大 500）、`cursor`（分页）

响应 `200`：
```json
{
  "items": [
    {
      "id": "mc-xxx",
      "kind": "recipe",
      "path": "data/ae2/recipe/misc/facade.json",
      "key": "ae2:misc/facade",
      "payload": { "type": "ae2:facade" },
      "isDynamic": true,
      "parseError": ""
    }
  ],
  "next_cursor": null,
  "total": 556,
  "run": {
    "status": "succeeded",
    "parsedCount": 556,
    "dynamicCount": 1,
    "errorCount": 0,
    "parsedAt": "2026-09-06T..."
  }
}
```

未解析响应 `404`：`{ "code": "content_not_parsed", "message": "mod content has not been parsed yet" }`

### 3.3 GET /api/packs/{packId}/mods/{modId}/content/run

响应 `200`：`{ "run": {...} }`，字段同上。

### 3.4 GET /api/packs/{packId}/mods/{modId}/content/{contentId}

响应 `200`：`{ "item": {...} }`

## 4. 错误码

| code | HTTP | 含义 |
|---|---|---|
| `mod_not_found` | 404 | pack_mod 不存在 |
| `mod_not_installed` | 422 | mod 状态不是 installed |
| `content_not_parsed` | 404 | 尚未执行过解析 |
| `parse_already_current` | 409 | 同 sha1 已解析成功 |
| `jar_bytes_unavailable` | 422 | local 来源 mod 无 jar 字节且无法重新下载 |
| `jar_too_large` | 422 | jar 超过 100MB |
| `jar_too_many_entries` | 422 | zip 条目超过 100000 |
| `jar_invalid` | 422 | 不是有效的 zip/jar |
| `invalid_kind` | 400 | kind 参数不在白名单 |
| `task_queue_unavailable` | 503 | queue 未配置 |

## 5. fixture 构造

`testdata/mod-content-minimal.jar` 用 Go 测试代码在 `TestMain` 中动态构造（不提交二进制文件）：

```go
func buildMinimalJar(t *testing.T) []byte {
    var buf bytes.Buffer
    zw := zip.NewWriter(&buf)
    // fabric.mod.json
    addFile(zw, "fabric.mod.json", `{"id":"testmod","name":"Test Mod","version":"1.0.0"}`)
    // 标准配方
    addFile(zw, "data/testmod/recipe/simple.json", `{"type":"minecraft:crafting_shapeless","ingredients":[{"item":"minecraft:dirt"}],"result":{"id":"testmod:magic_dirt","count":1}}`)
    // special 动态配方
    addFile(zw, "data/testmod/recipe/special/wildcard.json", `{"type":"testmod:wildcard"}`)
    // 物品模型
    addFile(zw, "assets/testmod/models/item/magic_dirt.json", `{}`)
    // worldgen structure
    addFile(zw, "data/testmod/worldgen/structure/test_ruin.json", `{"type":"minecraft:jigsaw"}`)
    zw.Close()
    return buf.Bytes()
}
```

预期解析结果：1 metadata + 2 recipe（1 标准 + 1 dynamic）+ 1 item_model + 1 structure。

## 6. 验证命令

```bash
cd apps/server
go test ./...
go vet ./...
cd ../web
npm run build
```

## 7. 不做的事（边界）

- 不实现自动触发（模组进包后自动解析）——仅手动 API 触发
- 不实现字节码层动态配方判据 B（extends CustomRecipe 检查）——仅判据 A（special 目录）
- 不持久化 jar 字节到 blobstore
- 不改动前端
- 不实现"从 mod_content 导入为 content 草稿"
- 不引入第三方 toml 解析库（用正则简易提取 neoforge 元数据）
