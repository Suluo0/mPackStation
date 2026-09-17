# 模组内容提取引擎设计文档

## 1. 背景与目标

当前 mPackStation 后端能解析整合包的 `manifest.json`（包名/MC 版本/loader），能从 Modrinth/CurseForge 拉取模组元数据（版本/哈希/依赖），但**无法读取模组 jar 内部的游戏内容**——配方、结构、地形生成、物品、战利品表等全部不可见。

本能力的目标：给定一个已安装的模组（pack_mod），异步解析其 jar 文件，提取其中**数据驱动（data-driven）的游戏内容**，落库为只读目录，供内容编辑页、冲突检测、发布质检等下游消费。

## 2. 范围

### 2.1 解析范围（数据驱动内容）

| 内容类型 | 数据路径 | 提取字段 |
|---|---|---|
| 元数据 | `fabric.mod.json` / `META-INF/neoforge.mods.toml` / `META-INF/mods.toml` / `quilt.mod.json` | modid、name、version、loader 类型 |
| 配方 | `data/<modid>/recipe/**/*.json` | type、ingredients（输入）、result/output（输出）、是否 special 动态配方 |
| 物品模型 | `assets/<modid>/models/item/**/*.json` | 物品 id 清单（可渲染物品） |
| 结构 | `data/<modid>/worldgen/structure/**/*.json`、`data/<modid>/structures/**/*.nbt` | 结构 id、类型、引用模板 |
| 地形生成 | `data/<modid>/worldgen/**/*.json`（configured_feature / placed_feature / biome / structure_set） | 特征 id、类型、维度/群系约束 |
| 战利品表 | `data/<modid>/loot_table/**/*.json` | 战利品 id、类型 |
| 进度 | `data/<modid>/advancement/**/*.json` | 进度 id |
| 标签 | `data/<modid>/tags/**/*.json` | 标签 id、包含的物品/方块 |

### 2.2 不解析范围（代码注册内容）

以下内容由 Java 代码在运行时动态注册，静态 jar 解析无法获取完整展开：

- 动态配方（special recipe，如 AE2 facade 伪装版）——仅识别其**存在与类型**，不展开具体实例
- 代码注册的方块/物品本体（部分 mod 的物品/方块完全由代码注册，无对应 data JSON）
- 运行时矿脉生成（部分 mod 的 worldgen 由代码动态构造）
- 行为逻辑、事件监听、Mixin

这些内容标记为 `is_dynamic=true`，仅记录类型声明，不记录展开实例。完整展开需运行时转储（独立增强，不在本能力范围）。

## 3. 数据模型

### 3.1 新表 `mod_content`

```sql
CREATE TABLE mod_content (
  id            TEXT PRIMARY KEY,
  pack_id       TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  mod_id        TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
  modid         TEXT NOT NULL DEFAULT '', -- 模组命名空间（如 ae2）
  version       TEXT NOT NULL DEFAULT '', -- 模组版本
  kind          TEXT NOT NULL,            -- recipe / item_model / structure / worldgen / loot_table / advancement / tag / metadata
  path          TEXT NOT NULL,            -- jar 内相对路径（如 data/ae2/recipe/misc/facade.json）
  key           TEXT NOT NULL DEFAULT '', -- 内容 id（如 ae2:facade）
  payload       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)), -- 解析出的结构化 JSON
  is_dynamic    INTEGER NOT NULL DEFAULT 0, -- 1=动态/代码注册，仅记录类型声明
  parse_error   TEXT NOT NULL DEFAULT '',  -- 单条解析失败原因（空=成功）
  parsed_at     INTEGER NOT NULL,
  UNIQUE(mod_id, kind, path)
);
CREATE INDEX idx_mod_content_pack ON mod_content(pack_id);
CREATE INDEX idx_mod_content_mod ON mod_content(mod_id);
CREATE INDEX idx_mod_content_kind ON mod_content(kind);
```

### 3.2 新表 `mod_content_runs`

记录每次解析任务的运行摘要（幂等去重依据）：

```sql
CREATE TABLE mod_content_runs (
  id            TEXT PRIMARY KEY,
  pack_id       TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  mod_id        TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
  sha1          TEXT NOT NULL,            -- 解析时的 jar sha1（版本变更后重解析）
  status        TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','succeeded','failed')),
  total_files   INTEGER NOT NULL DEFAULT 0,
  parsed_count  INTEGER NOT NULL DEFAULT 0,
  dynamic_count INTEGER NOT NULL DEFAULT 0,
  error_count   INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  started_at    INTEGER NOT NULL,
  finished_at   INTEGER,
  UNIQUE(mod_id, sha1)
);
```

### 3.3 与现有 `content_documents` 的关系

- `mod_content`：**只读解析目录**，来自模组 jar，不可编辑
- `content_documents`：**用户创作的内容文档**（配方/结构/矿脉），可编辑、有修订历史
- 两者独立存储，不混用。内容编辑页可"从 mod_content 导入为草稿"（后续增强，不在本能力范围）

## 4. 任务设计

### 4.1 新任务 kind

`task.KindParseModContent = "parse_mod_content"`

需 migration 0008 扩展 `tasks.kind` CHECK 约束（参照 0007 的表重建模式）。

### 4.2 任务 payload

```json
{
  "packId": "pack-xxx",
  "modId": "mod-xxx"
}
```

### 4.3 任务执行流程

1. 校验 pack_id / mod_id 存在且 mod 已安装
2. 查 `mod_content_runs`：若同 mod_id + 同 sha1 已有 succeeded 记录，直接返回（幂等）
3. 获取 jar 字节：
   - 优先查 blobstore（若该 sha1 已落盘）
   - 否则从 provider（Modrinth/CF）按 project_id + version_id 重新下载
   - local 来源的 mod：从上传暂存区读（若存在）
4. 调用解析器 `ExtractModContent(io.Reader)` → `ExtractedContent`
5. 事务：删除该 mod_id 旧的 `mod_content` 行 → 批量插入新行 → upsert `mod_content_runs`
6. 写 activity / outbox / audit（参照 addP6Evidence 模式）
7. 进度上报：下载 20% → 元数据 30% → 配方 50% → 结构/地形 70% → 物品/其他 90% → 落库 100%

### 4.4 触发时机

- **手动触发**：`POST /api/packs/{packId}/mods/{modId}/content/parse`
- **自动触发（后续增强）**：模组进包成功后自动排解析任务（本能力先做手动，自动触发作为独立后续项，避免与 addPackMod 事务耦合）

## 5. API 设计

### 5.1 触发解析

```
POST /api/packs/{packId}/mods/{modId}/content/parse
→ 202 { taskId, status }
→ 409 { code: "parse_already_current", message: "content already parsed for this jar version" }
```

### 5.2 查询解析结果

```
GET /api/packs/{packId}/mods/{modId}/content?kind=recipe&limit=100&cursor=xxx
→ 200 { items: [...], next_cursor, total, run: {status, parsedCount, dynamicCount, errorCount, parsedAt} }
```

### 5.3 查询解析运行状态

```
GET /api/packs/{packId}/mods/{modId}/content/run
→ 200 { run } 或 404 content_not_parsed
```

### 5.4 单条内容详情

```
GET /api/packs/{packId}/mods/{modId}/content/{contentId}
→ 200 { item }
```

## 6. 解析器架构

### 6.1 分层结构

```
service/mod_content_extract.go
├── ExtractModContent(r io.Reader) (*ExtractedContent, error)   // 入口
├── extractMetadata(z *zip.Reader) ModMetadata                    // 元数据
├── classifyAndExtract(z *zip.Reader) []RawContentItem           // 遍历+分类
├── parseRecipe(path string, data []byte) RecipeItem              // 配方解析
├── parseWorldgen(path string, data []byte) WorldgenItem          // 地形/结构
├── detectDynamicRecipe(path string, data []byte, z *zip.Reader) bool  // 动态配方识别
└── classfile.HasSuperclass(data []byte, superName string) bool   // 字节码辅助（内部包）
```

### 6.2 核心数据结构

```go
type ExtractedContent struct {
    ModID      string
    Version    string
    Loader     string // fabric / neoforge / forge / quilt
    Items      []ContentItem // kind=item_model
    Recipes    []ContentItem // kind=recipe
    Structures []ContentItem // kind=structure
    Worldgen   []ContentItem // kind=worldgen
    LootTables []ContentItem // kind=loot_table
    Advancements []ContentItem // kind=advancement
    Tags       []ContentItem // kind=tag
    Metadata   ContentItem   // kind=metadata
    Stats      ExtractStats
}

type ContentItem struct {
    Kind       string
    Path       string
    Key        string
    Payload    json.RawMessage
    IsDynamic  bool
    ParseError string
}
```

### 6.3 配方解析细节

- 标准配方（`minecraft:crafting_shaped/shapeless/smelting/blasting/stonecutting/smithing_transform`）：提取 type、ingredients（含 tag/item）、result/output（id + count）
- 自定义配方类型（`ae2:inscriber` 等）：保留完整原始 JSON，标记 `is_dynamic=false`（类型是数据驱动的，实例是静态的）
- special 动态配方（`recipe/special/` 目录，或 serializer 类继承 `CustomRecipe`）：标记 `is_dynamic=true`，payload 只存 `{"type": "..."}`

### 6.4 动态配方识别（双判据）

判据 A（JSON 层，低成本）：配方路径包含 `/recipe/special/` 且 JSON 只有 `type` 字段无 ingredients/result → 标记 dynamic

判据 B（字节码层，精确）：解析配方 JSON 的 `type` 字段 → 映射到 serializer 类名（如 `ae2:facade` → `appeng.recipes.game.FacadeRecipe`）→ 检查该类是否继承 `net/minecraft/world/item/crafting/CustomRecipe` → 是则标记 dynamic

判据 A 足以覆盖绝大多数动态配方；判据 B 作为增强（需要解析 class 常量池，已在 POC 中验证 Go 原生可行）。本能力先实现判据 A，判据 B 作为后续增强项（字节码解析器独立成包，不阻塞主流程）。

## 7. jar 字节获取策略

当前系统模组进包时**不持久化 jar 字节**（`dl.Content` 仅用于算 hash 后丢弃，`jar_index.FilePath` 是虚拟路径 `jar://<sha1>`）。解析引擎采用：

1. **优先 blobstore**：若该 sha1 已在 blobstore（如 build 流程缓存过），直接 `blobstore.Open(sha1)`
2. **回退 provider 下载**：按 `pack_mods.project_id + version_id` 调用 `provider.Download` 重新获取字节
3. **local 来源**：按 sha1 查 blobstore；若不存在，返回错误 `jar_bytes_unavailable`（local mod 需用户重新上传或提供文件）

下载的字节**不持久化**到 blobstore（避免存储膨胀），解析完成即丢弃。若后续 build 流程也需要，可独立决策是否缓存。

## 8. 安全与资源约束

- jar 大小上限：100MB（超过返回 `jar_too_large`）
- zip 条目数上限：100000（超过返回 `jar_too_many_entries`，防 zip bomb）
- 单条目解压上限：50MB
- 总解压上限：500MB
- 路径遍历防护：拒绝包含 `..`、绝对路径、`:` 的 zip 条目名（参照 import_service.validateArchiveName）
- 任务超时：沿用 task 系统默认 2 小时 deadline，解析本身通常亚秒级

## 9. 与现有系统的集成点

| 集成点 | 方式 |
|---|---|
| task kind 扩展 | migration 0008 重建 tasks 表（参照 0007） |
| task handler 注册 | `httpapi.go` NewRouterWithProviders 中 `q.RegisterHandler(task.KindParseModContent, ...)` |
| 路由注册 | `httpapi.go` newRouter 中 `registerModContentRoutes(mux, app)` |
| store 层 | 新增 `store/mod_content_repo.go`（参数化 SQL + 事务） |
| service 层 | 新增 `service/mod_content_extract.go`（纯解析）+ `service/mod_content_task.go`（任务 handler + API 方法） |
| activity/outbox/audit | 参照 `addP6Evidence` 模式，kind=`mod_content` |
| 前端 | 本能力不改动前端（API 就绪后前端独立迭代） |

## 10. 验收标准

- [ ] 任意已安装模组可通过 API 触发解析，任务状态可追踪
- [ ] 解析结果落库 `mod_content`，按 kind/path 唯一去重
- [ ] 同 sha1 重复触发幂等（不重复解析）
- [ ] 配方解析覆盖标准类型 + 自定义类型，special 动态配方正确标记 `is_dynamic`
- [ ] 元数据识别 fabric/neoforge/forge 三种 loader 格式
- [ ] zip bomb / 路径遍历 / 超大文件防护生效
- [ ] `go test ./...`、`go vet ./...` 通过
- [ ] 契约测试覆盖 API 输入输出
- [ ] fixture 包含最小可解析 jar（含 1 配方 + 1 物品模型 + 1 special 动态配方）
