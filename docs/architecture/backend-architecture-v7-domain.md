# mPackStation 后端架构设计 v7（产品领域补全版，非权威草案）

> 本文件已被 `docs/architecture/backend-architecture-v7.md` 最终合并版取代；如与最终版冲突，以最终版为准。

> 本文是 v6 的领域补全版，不修改、不覆盖 `backend-architecture.md`。v6 中已经冻结的基础设施、任务 lease/fencing、provider 限流、blobstore、迁移校验、错误信封和浏览器防护继续有效；本文补齐 v6 明确留待后续设计的产品领域，并把前端页面契约落成可实现的模型。
>
> v7 的部署模型是“单机、单用户、单实例”。**没有 workspace、tenant、owner、member、role 或未来多租户兼容层**。鉴权与安全在本文中作为一个统一边界设计：它保护本机服务免受恶意网页和意外暴露，但不伪装成多用户权限系统。

## 0. v7 目标与不可变取舍

### 0.1 产品边界

mPackStation 的唯一业务对象是整合包。所有模组、版本、锁快照、内容编辑、任务书、构建产物和发布记录必须能追溯到一个 `pack_id`，只有系统级设置、provider 缓存和全局任务可以没有包。

产品闭环为：

```text
创建/导入整合包
  → 搜索并加入模组
  → 解析依赖与冲突
  → 下载并索引 JAR
  → 编辑配方/结构/矿脉
  → 编排任务书
  → 交付检查
  → 可复现构建
  → 本地导出或发布
```

### 0.2 v7 做出的明确决定

1. **彻底删除多租户概念。** `Principal` 只有本机主体；不保留 `owner_id`、`workspace_id`、成员矩阵或“未来接入后不改签名”的伪抽象。历史 schema 中如有这些列，0001 重建时不创建，后续迁移删除。
2. **鉴权与安全合并。** 所有写操作仍由启动 token + Host/Origin 防护覆盖；LAN 模式下所有端点都必须带 token。这个维度同时描述“谁能调用”与“调用会造成什么本机风险”。
3. **前端最终字段是外部 API 的产品契约。** 内部可使用 `kind`、`status` 等领域名，但 HTTP adapter 输出前端已经使用的 `type`、`cancelled`、ISO 时间和计数字段，避免组件读取数据库语义。
4. **草稿和已应用内容分离。** 内容编辑器和任务书都支持保存、校验、历史和回滚；只有 applied revision 进入交付检查与构建。
5. **包的发行版本与锁快照版本分离。** `pack_versions.version` 是用户可见的整合包版本；`pack_locks.version` 是依赖解析快照序号，二者不可互换。
6. **读模型允许聚合，不允许成为第二事实源。** dashboard 由权威业务表确定性聚合；如为性能建立快照，必须可由权威表重建并在同一事务中更新。

### 0.3 不在 v7 领域文档中解决的事情

- 不实现多用户登录、JWT、session、workspace 或远程协作。
- 不把前端视觉细节、图标和布局复制进后端架构；页面规范仍以 `docs/specs/*` 和 `dashboard-page-prompt.md` 为准。
- 不用一个“大 JSON”替代需要约束、排序、引用和历史的核心表；JSON 只用于领域 payload、扩展字段和 provider 原始响应。

## 1. 架构边界与单用户主体

### 1.1 模块边界

v6 的依赖方向保持不变：

```text
cmd/server → httpapi → service → {store, task, provider, blobstore}
                                 ↘ obs / platform
```

v7 新增或明确的领域服务为：

```text
dashboardsvc   看板聚合与信号
packsvc        包、包版本和包内模组
contentsvc     配方、结构、矿脉草稿与应用
questsvc       任务书章节、节点、边和奖励
importsvc      URL/文件导入编排
buildsvc       交付检查、构建、产物
publishsvc     provider 发布状态
settingssvc    设置、密钥状态、onboarding
activitysvc    outbox 到用户动态的投影
```

`httpapi` 不得直接触 SQL、provider、文件或任务队列。`store` 仍是唯一 SQL 入口；所有跨表写在 service 的单个短事务内完成，网络和磁盘 I/O 不持有数据库事务。

### 1.2 Principal 定义

```go
type Principal struct {
    ID   string // 固定为 "local"
    Kind string // 固定为 "local-user"
}
```

它不是租户主体，也不参与业务表的过滤。service 只用它写审计事件和判断“本机服务调用”，不提供角色矩阵。`authorize(principal, resource)` 的 resource domain 只有 `pack`、`settings`、`secrets`、`task`、`system`，且当前本机主体拥有这些资源的合法操作；安全拒绝来自 token、Host、Origin、路径、大小、速率或任务状态检查。

## 2. 统一产品数据模型

### 2.1 包与包版本

`packs` 保存包的稳定身份和当前编辑上下文：

```sql
CREATE TABLE packs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    icon_path TEXT NOT NULL DEFAULT '',
    mc_version TEXT NOT NULL,
    loader TEXT NOT NULL CHECK (loader IN ('forge','neoforge','fabric','quilt')),
    loader_version TEXT NOT NULL,
    current_version_id TEXT,
    current_lock_id TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

用户可见版本单独建模，支持历史、构建和发布关联：

```sql
CREATE TABLE pack_versions (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    version TEXT NOT NULL CHECK (version GLOB '[0-9]*.[0-9]*.[0-9]*'),
    channel TEXT NOT NULL DEFAULT 'draft' CHECK (channel IN ('draft','release')),
    changelog TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','imported','build')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(pack_id, version)
);
ALTER TABLE packs ADD CONSTRAINT fk_packs_current_version
    REFERENCES pack_versions(id) ON DELETE SET NULL;
```

SQLite 不支持以 `ALTER TABLE ADD CONSTRAINT` 的形式增加所有 FK；实际 `0001_init.sql` 必须在建表时把 `current_version_id` 的 FK 写入 `packs`，以上片段表达语义而非可直接执行顺序。创建包时同一事务创建初始 `pack_versions`（例如 `0.1.0`）并回填 current；版本号修改只允许 service 执行，必须是有效 SemVer 且不可覆盖已有版本。

锁快照仍使用 v6 `pack_locks`：

- `pack_locks.version` 是同一包内单调递增的解析快照序号。
- `pack_locks.pack_version_id` 指向本次锁定对应的产品版本（建议在 0002 迁移增加）。
- `snapshot_path` 指向不可变 JSON；构建必须固定读取该快照及其 SHA-256。
- 修改 MC 版本或 loader 会创建新的草稿上下文、清空 current lock 引用并生成待解析信号，不删除历史锁。

### 2.2 模组和健康来源

v6 `pack_mods`、`jar_index`、`conflicts` 保持为权威来源。v7 明确 dashboard 字段的计算规则：

| 前端字段 | 权威来源与规则 |
|---|---|
| `modCount.total` | `pack_mods` 中该包全部条目 |
| `modCount.installed` | `sha1 IS NOT NULL` 且 `jar_index` 文件存在 |
| `modCount.selected` | `sha1 IS NULL` 的已启用条目 |
| `conflicts.resolved` | `status='resolved'` 的冲突数 |
| `conflicts.pending` | `status='pending'` 的冲突数（error/warning 均可返回，阻塞只看 error） |
| `edits.recipes/structures/ores` | `content_documents.kind` 对应包的 applied revision 数 |
| `edits.quests` | 已应用任务书中的节点数 |
| `alerts.crashes` | `pack_alerts.kind='crash' AND status='open'` |
| `alerts.updatable` | `pack_mod_updates.status='pending'` |

新增表：

```sql
CREATE TABLE pack_alerts (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('crash','update')),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved','ignored')),
    source_ref TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(pack_id, kind, source_ref)
);
CREATE TABLE pack_mod_updates (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    pack_mod_id TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
    candidate_version_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','ignored')),
    checked_at INTEGER NOT NULL,
    UNIQUE(pack_id, pack_mod_id)
);
```

更新检查是 provider 的 `update-preflight` 任务输出，不能在 dashboard GET 中隐式访问上游；看板只读最近一次结果。

## 3. Dashboard 读模型

### 3.1 聚合原则

`GET /api/dashboard` 是看板唯一主查询。它返回所有包、最近编辑包和今日已解决数；不返回全局模组库等虚荣指标。`dashboardsvc` 使用一次只读事务读取包和聚合计数，按 `updated_at DESC, id DESC` 排序，`lastEditedPackId` 等于同一排序的第一项，避免前端和后端分别猜“最近”。

为便于验证，可实现 SQL view `dashboard_pack_view`，但 view 不作为可写表。若日后引入 `pack_dashboard_stats` 快照，必须：

1. 记录 `source_updated_at` 和 `computed_at`；
2. 每次影响健康信号的 service 事务同步更新；
3. 启动时可全量重建；
4. dashboard 响应标注 `statsFresh`，不能用旧快照冒充零值。

### 3.2 HTTP 契约

```json
{
  "packs": [{
    "id": "pack_01",
    "name": "科技魔法",
    "description": "",
    "iconUrl": null,
    "mcVersion": "1.20.1",
    "loader": "neoforge",
    "loaderVersion": "20.4.230",
    "packVersion": "1.2.0",
    "modCount": {"total": 142, "installed": 130, "selected": 12},
    "conflicts": {"resolved": 12, "pending": 3},
    "edits": {"recipes": 8, "structures": 2, "ores": 1, "quests": 15},
    "alerts": {"crashes": 0, "updatable": 5},
    "lastEditedAt": "2026-08-28T10:32:00+08:00",
    "createdAt": "2026-08-20T09:00:00+08:00"
  }],
  "lastEditedPackId": "pack_01",
  "todayResolvedCount": 12
}
```

时间在 HTTP 使用 ISO 8601，落库仍为 unix ms。`iconUrl` 只能是本地受控资源 URL 或 null，不允许 provider 外链。

### 3.3 看板分区接口

- `GET /api/tasks?recent=20` 返回前端所需的最近任务数组；`GET /api/tasks?cursor=&limit=` 返回 v6 标准 `{items,next_cursor,total}`。
- `GET /api/activities?limit=10` 返回最近动态数组；游标形式可返回标准 envelope。
- 看板区块分别容错：dashboard、tasks、activities、health/status 失败时由 adapter 显示该区块错误，不把未知当作 0。
- dashboard 读不创建任务、不刷新 provider 缓存、不写业务状态。

## 4. Settings 与 Onboarding

### 4.1 Settings 分类和类型

`settings` 仍是 key-value 表，但 key 集合由 service 固定，不接受任意 key：

| key | 类型 | 默认值 | 作用 |
|---|---|---|---|
| `default_mc_version` | string | `""` | 新建包默认 MC 版本 |
| `default_loader` | enum | `"neoforge"` | 新建包默认加载器 |
| `storage_data_dir` | path | 当前 data | 仅显示/迁移入口，不允许任意写入 |
| `cache_policy` | enum | `normal` | `normal`/`offline`，影响 provider stale 回退 |
| `cache_retention_days` | integer | `30` | 1–365，不能绕过安全宽限 |
| `default_pack_description` | string | `""` | 新建包表单默认描述 |
| `ui_density` | enum | `comfortable` | 前端偏好，不影响业务 |

provider API key 只存 `secrets`：`provider.curseforge.api_key`、`provider.modrinth.api_key`（Modrinth 可为空）。`GET /api/settings` 只返回配置状态和 masked 值，不返回密文；PATCH 采用完整键白名单并返回规范化后的设置。

```json
{
  "settings": {
    "defaultMcVersion": "1.20.1",
    "defaultLoader": "neoforge",
    "storageDataDir": "D:/mPackStation/data",
    "cachePolicy": "normal",
    "cacheRetentionDays": 30,
    "defaultPackDescription": "",
    "uiDensity": "comfortable"
  },
  "providers": {
    "curseforge": {"configured": true, "masked": "tp-cb…", "lastCheckedAt": "..."},
    "modrinth": {"configured": false, "masked": null, "lastCheckedAt": null}
  }
}
```

`POST /api/settings/providers/{provider}/test` 是显式动作，返回 `{provider, reachable, checkedAt, errorCode}`；不能通过 GET 隐式触发 provider 请求。设置修改、密钥更新、导出目录变更、缓存清理全部写审计和 outbox（敏感值只写 key 和 configured 状态）。

### 4.2 Onboarding 语义

前端只需要三个布尔值，但后端必须定义其来源。`GET /api/onboarding` 返回：

```json
{"steps":{"curseforgeKey":true,"firstPack":true,"firstMod":false}}
```

规则是“派生状态 OR 用户确认状态”：

- `curseforgeKey` = key 已配置，或 onboarding_state 已确认；
- `firstPack` = 至少存在一个包，或已确认；
- `firstMod` = 至少存在一个 `pack_mods`，或已确认。

`onboarding_state(step PRIMARY KEY, acknowledged INTEGER, acknowledged_at INTEGER)` 只存用户确认，不重复存业务事实。`PUT /api/onboarding` 接受同形状 steps，只允许把 false 设为 true；撤销由清理 onboarding 的内部动作完成，不提供前端 DELETE。所有 step 更新幂等。

空态清单全部完成后，服务仍保留状态，前端负责不渲染；不能依赖 localStorage。

## 5. 内容编辑领域（配方、结构、矿脉）

### 5.1 内容模型

三种内容共享文档/修订生命周期，payload 按 kind 使用严格 schema：

```sql
CREATE TABLE content_documents (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('recipe','structure','ore')),
    name TEXT NOT NULL,
    current_revision_id TEXT,
    applied_revision_id TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','applied','invalid')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(pack_id, kind, name)
);
CREATE TABLE content_revisions (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES content_documents(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL CHECK (revision > 0),
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    payload_sha256 TEXT NOT NULL,
    validation TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(validation)),
    validation_status TEXT NOT NULL DEFAULT 'unknown' CHECK (validation_status IN ('unknown','valid','warning','invalid')),
    created_at INTEGER NOT NULL,
    applied_at INTEGER,
    UNIQUE(document_id, revision),
    UNIQUE(document_id, payload_sha256)
);
```

实际建表时 current/applied revision FK 需要在完整 DDL 中一次声明，和 v6 的循环 FK 规则相同。revision 不可原地更新；“保存草稿”是追加 revision 并更新 current，“应用到包”是在一次事务中校验 revision、更新 applied/current 和 outbox。只有 `validation_status IN ('valid','warning')` 的 revision 可应用；阻塞错误不能应用。

### 5.2 Payload 最低契约

- `recipe`：`type`、`inputs[]`、`outputs[]`、`conditions[]`、`resultCount`；槽位、物品 ID、数量和条件必须非空且有界。
- `structure`：`structureId`、`dimensions`、`palette[]`、`placements[]`、`spawnRules`；尺寸、方块数量和坐标必须受上限约束。
- `ore`：`blockId`、`dimension`、`minY`、`maxY`、`veinsPerChunk`、`size`、`distribution`；Y 范围和分布枚举必须合法。

未知字段默认拒绝，允许 `metadata` 扩展对象。服务端 canonical JSON 后计算 hash，避免相同内容重复产生 revision。

### 5.3 内容 API

```text
GET    /api/packs/{packId}/content?kind=&cursor=&limit=
POST   /api/packs/{packId}/content
GET    /api/packs/{packId}/content/{contentId}
PATCH  /api/packs/{packId}/content/{contentId}          保存草稿（If-Match revision 必填）
POST   /api/packs/{packId}/content/{contentId}/validate
POST   /api/packs/{packId}/content/{contentId}/apply
GET    /api/packs/{packId}/content/{contentId}/history
POST   /api/packs/{packId}/content/{contentId}/rollback  从历史 revision 生成新草稿
DELETE /api/packs/{packId}/content/{contentId}
```

详情响应包含 `draftRevision`、`appliedRevision`、`validation`、`affectedMods[]`、`conflicts[]`、`undoAvailable`。保存失败不会改变已应用内容；并发 revision 不匹配返回 `content_revision_conflict`。应用成功会刷新 dashboard 编辑计数、影响的冲突并写 `kind='content'` outbox 与 `edit` activity。

## 6. 任务书领域（章节、节点、边、奖励）

### 6.1 模型与图约束

```sql
CREATE TABLE quest_books (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL UNIQUE REFERENCES packs(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '任务书',
    description TEXT NOT NULL DEFAULT '',
    current_revision INTEGER NOT NULL DEFAULT 0,
    applied_revision INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','applied','invalid')),
    updated_at INTEGER NOT NULL
);
CREATE TABLE quest_chapters (
    id TEXT PRIMARY KEY,
    book_id TEXT NOT NULL REFERENCES quest_books(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cover_color TEXT NOT NULL DEFAULT 'ember',
    position INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE(book_id, position)
);
CREATE TABLE quest_nodes (
    id TEXT PRIMARY KEY,
    chapter_id TEXT NOT NULL REFERENCES quest_chapters(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    icon_ref TEXT NOT NULL DEFAULT '',
    position_x REAL NOT NULL DEFAULT 0,
    position_y REAL NOT NULL DEFAULT 0,
    conditions TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(conditions)),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','applied'))
);
CREATE TABLE quest_edges (
    id TEXT PRIMARY KEY,
    book_id TEXT NOT NULL REFERENCES quest_books(id) ON DELETE CASCADE,
    from_node_id TEXT NOT NULL REFERENCES quest_nodes(id) ON DELETE CASCADE,
    to_node_id TEXT NOT NULL REFERENCES quest_nodes(id) ON DELETE CASCADE,
    UNIQUE(book_id, from_node_id, to_node_id),
    CHECK(from_node_id <> to_node_id)
);
CREATE TABLE quest_rewards (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES quest_nodes(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('item','experience','command','unlock')),
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    position INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE quest_node_mod_refs (
    node_id TEXT NOT NULL REFERENCES quest_nodes(id) ON DELETE CASCADE,
    pack_mod_id TEXT NOT NULL REFERENCES pack_mods(id) ON DELETE CASCADE,
    PRIMARY KEY(node_id, pack_mod_id)
);
```

章节 position 在同一 book 内唯一；节点和边必须属于同一 book；边提交时在 service 内执行有界 DFS/Kahn 拓扑检查，禁止环；引用已删除的模组、奖励 payload 缺字段、孤立非入口节点和没有奖励的非终点节点产生 warning 或 error（规则可配置但必须进入 validation 结果）。章节颜色仅分组，不承担成功/失败语义。

任务书编辑采用增量事务写规范化表，保存时生成 revision 快照 hash；预览读取 draft，构建/导出只读取 applied revision。删除章节级联节点、边和奖励，必须写审计和 activity。

### 6.2 任务书 API

```text
GET    /api/packs/{packId}/quests
PUT    /api/packs/{packId}/quests                 保存书籍元数据/生成修订
POST   /api/packs/{packId}/quests/validate
POST   /api/packs/{packId}/quests/apply
GET    /api/packs/{packId}/quests/history
POST   /api/packs/{packId}/quests/rollback
POST   /api/packs/{packId}/quests/chapters
PATCH  /api/packs/{packId}/quests/chapters/{chapterId}
DELETE /api/packs/{packId}/quests/chapters/{chapterId}
POST   /api/packs/{packId}/quests/nodes
PATCH  /api/packs/{packId}/quests/nodes/{nodeId}
DELETE /api/packs/{packId}/quests/nodes/{nodeId}
POST   /api/packs/{packId}/quests/edges
DELETE /api/packs/{packId}/quests/edges/{edgeId}
POST   /api/packs/{packId}/quests/preview
```

`GET` 返回 `chapters[]`、每章 `nodes[]`、全局 `edges[]`、校验结果、`draftRevision`/`appliedRevision` 和保存状态。所有 ID 均由服务端生成；拖拽排序使用显式 position，不让前端以数组下标隐式重排。

## 7. Import 领域与安全流程

### 7.1 支持的输入

```ts
type ImportRequest =
  | { source: 'curseforge_url' | 'modrinth_url'; url: string; name?: string }
  | { source: 'local_zip'; uploadId: string; name?: string }
```

URL 只允许 CurseForge/Modrinth 的公开项目或整合包 URL，解析前先做 scheme、host、长度和 SSRF 校验；不接受任意内网 URL。文件上传使用 v6 512 MiB 流式限制、zip 条目/压缩炸弹/symlink/路径穿越检查，`uploadId` 指向受 blobstore 管理的临时对象。

### 7.2 两阶段 API

```text
POST /api/packs/import/preview
POST /api/packs/import
GET  /api/imports/{importId}
POST /api/imports/{importId}/cancel
```

`preview` 不创建包、不写 `packs`；URL preview 触发受 v6 全局限速的 provider 读取，local preview 只读取临时 zip。响应：

```json
{
  "source":"modrinth_url",
  "name":"科技魔法",
  "mcVersion":"1.20.1",
  "loader":"fabric",
  "packVersion":"1.0.0",
  "modCount":142,
  "warnings":[],
  "token":"preview_01"
}
```

确认导入时提交 `{previewToken, name?, idempotencyKey}`，返回 `202`：

```json
{"importId":"imp_01","taskId":"task_01","packId":null,"status":"queued"}
```

任务内部阶段为 `received → inspected → extracted → parsed → creating_pack → staging_mods → indexing → succeeded/failed/canceled`。创建包与初始 pack_version 必须在一个短事务中完成；模组文件只先进入 staging，下载/索引交由任务 handler。任一阶段失败，事务回滚新包元数据，临时文件和任务 payload 由 GC 清理；若已经产生 pack，则写明确的 failed import 状态，不允许半成品静默出现在 dashboard。

manifest 解析器覆盖 CurseForge `manifest.json`、Modrinth `modrinth.index.json` 和本地受支持 zip；远端文件需 provider 声明的域名和版本校验，受限分发文件登记 distribution conflict，不绕过平台限制。

## 8. Task API 与 Activity API 契约

### 8.1 内部/外部任务名映射

内部任务类型沿用 v6 的稳定集合：`resolve`、`download`、`index`、`build`、`publish`、`import`、`cache_gc`。HTTP 为兼容前端使用产品名：

| 内部 kind | 外部 `type` |
|---|---|
| `index` | `index-mod` |
| `build` | `build-pack` |
| `import` | `import-pack` |
| `resolve` | `update-preflight`（由 payload.operation 区分依赖解析/更新预演） |
| `download` | `download-mod`（任务区可展示但前端可隐藏） |
| `publish` | `publish-pack` |

内部状态 `canceled` 统一由 HTTP adapter 输出为前端的 `cancelled`；数据库和 service 只保留一种拼写，禁止两套状态机。

### 8.2 Task API

```text
GET  /api/tasks?recent=20
GET  /api/tasks?packId=&status=&kind=&cursor=&limit=
GET  /api/tasks/{taskId}
POST /api/tasks/{taskId}/pause
POST /api/tasks/{taskId}/resume
POST /api/tasks/{taskId}/cancel
POST /api/tasks/{taskId}/retry
GET  /api/tasks/{taskId}/log
```

最近任务响应与 dashboard prompt 对齐：

```json
{
  "id":"task_01",
  "type":"index-mod",
  "title":"索引 Create 模组数据层",
  "packId":"pack_01",
  "packName":"科技魔法",
  "status":"running",
  "progress":60,
  "error":null,
  "startedAt":"2026-08-28T10:30:00+08:00",
  "finishedAt":null
}
```

`recent` 结果按 `updated_at DESC, id DESC`，最多 20；运行中任务由前端轮询，服务端不承诺 websocket。pause/resume/cancel/retry 仍遵守 v6 状态机、lease epoch 和幂等；包删除会调用 `CancelByPack`，任务行保留到 GC。

### 8.3 Activity API

数据库 outbox 仍使用领域 kind：`pack/mod/conflict/task/build/content/quest/system`；投影到 `activities` 时使用前端展示 kind：

| 领域事件 | UI kind | 示例 |
|---|---|---|
| pack create/import | `import` | 导入了「科技魔法」 |
| mod add | `add-mod` | 向「科技魔法」添加了 JEI |
| conflict resolved | `resolve` | 自动解决了 2 个配方冲突 |
| build/publish success | `build` | 打包「科技魔法」v1.2.0 成功 |
| content/quest apply | `edit` | 更新了「科技魔法」任务书 |
| warning/error/crash | `alert` | 「科技魔法」有 2 个待处理告警 |

`activities` 表的 `kind` 应改为上述六个 UI kind，`origin_event_id UNIQUE` 负责幂等；领域细节放 `detail`，展示句子由 activitysvc 统一生成，前端不拼接原始数据库字段。`GET /api/activities?limit=10` 按时间倒序返回 `{id,kind,text,packId,at}` 数组。outbox 投递失败不丢业务提交，重试不产生重复 activity。

## 9. 构建、交付与发布领域补齐

为支持 publish 页面及 dashboard 的 `packVersion`，v7 建议在 M4 直接冻结以下模型：

```sql
CREATE TABLE delivery_checks (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    pack_version_id TEXT NOT NULL REFERENCES pack_versions(id) ON DELETE CASCADE,
    check_key TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pass','warning','block')),
    detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
    checked_at INTEGER NOT NULL,
    UNIQUE(pack_id, pack_version_id, check_key)
);
CREATE TABLE artifacts (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    pack_version_id TEXT NOT NULL REFERENCES pack_versions(id) ON DELETE CASCADE,
    task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
    file_path TEXT NOT NULL,
    file_name TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
    sha256 TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('building','ready','failed','deleted')),
    created_at INTEGER NOT NULL
);
CREATE TABLE releases (
    id TEXT PRIMARY KEY,
    pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    pack_version_id TEXT NOT NULL REFERENCES pack_versions(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK(provider IN ('curseforge','modrinth','local')),
    artifact_id TEXT REFERENCES artifacts(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK(status IN ('queued','publishing','published','failed','canceled')),
    remote_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

端点：

```text
GET  /api/packs/{packId}/delivery-checks
POST /api/packs/{packId}/builds
GET  /api/packs/{packId}/artifacts
POST /api/packs/{packId}/publish/{provider}
GET  /api/packs/{packId}/releases
```

交付检查至少包含依赖、阻塞冲突、缺失 JAR、内容校验、任务书图校验、版本号和磁盘空间。build 任务只读取 applied 内容、current lock 和指定 pack_version，成功后原子登记 artifact；发布任务不自动重试非幂等 provider 写入，支持查询和人工重试。

## 10. 统一错误、分页和不变量

沿用 v6 错误信封；v7 新增领域码：

```text
content_revision_conflict 409
content_invalid             422
quest_cycle                 422
quest_invalid_reference     422
quest_revision_conflict     409
pack_version_conflict       409
import_preview_expired      410
import_invalid_source       422
import_in_progress          409
build_blocked               409
artifact_not_found          404
release_not_found           404
```

不变量补充：

1. 一个包只能有一个 current product version 和一个 current lock；current lock 的 `pack_id`、`pack_version_id` 必须属于同一包。
2. pack_version、content revision、quest revision 追加式保存，不原地改写历史；回滚是创建新草稿。
3. 构建只能读取 applied revision；任何 pending error conflict、invalid content 或 quest cycle 都阻塞构建。
4. dashboard 计数只来自权威表；健康信号未知返回 null/`-` 语义，不返回伪造的 0。
5. `pack_alerts` 和 `pack_mod_updates` 的 open/pending 状态都必须由任务或明确用户操作改变，dashboard GET 不改变它们。
6. import preview token 一次性、短 TTL、绑定输入 hash；确认时 idempotency key 与 payload hash 必须匹配。
7. 所有内容、任务书、版本、导入、构建、发布写入同事务 outbox；outbox/activity 投影至少一次但幂等。
8. 删除包级联业务行，但 blobs、artifacts、payload 和外部发布记录按 v6 清理/保留策略处理，不直接删除共享 JAR。

## 11. 鉴权与安全（统一维度）

这是单用户本地应用的唯一安全边界，不是多租户授权系统：

- 监听默认仅 `127.0.0.1`；`allow_lan=true` 时所有 GET、metrics、audit 也必须 token，并启动大字风险警告。
- 写请求必须同时通过 `X-MPack-Token`、Host 白名单和 FrontendOrigin 校验；常数时间比较，错误统一映射为 `unauthorized`/`forbidden_origin`。
- provider API key 走 v6 KeyProtector，日志、task payload、activity 和错误详情都不得出现原值。
- 文件路径、导出目录、导入 zip、URL preview 分别通过 v6 pathpolicy、标记文件、归档防护和 SSRF 校验。
- 资源授权不按 user/tenant 过滤；服务只校验资源存在、资源状态、主体为 local 和动作是否是合法 endpoint。跨包 ID 只返回 `pack_not_found`，防止错误信息泄漏。
- 所有高影响动作（删除包、清理缓存、修改密钥、导出、发布）写 audit；审计写失败则整体回滚。
- 内容 payload、任务书 JSON、manifest、provider 返回都受大小、字段、深度和枚举限制；不能把 JSON1 当作完整 schema 校验。

接受的风险：同一操作系统用户进程可读取本机 data 目录或伪造导出目录标记；v7 不防同机同用户恶意进程，也不承诺 TLS。这个取舍替代了 v6“为未来多租户预留”的设计复杂度。

## 12. 迁移、契约测试与验收

v7 不修改 v6 文件；落地时新增迁移（示例 `0002_domain_v7.sql`）并遵守 v6 checksum/不可改规则。迁移必须覆盖：

- 删除 `owner_id`/workspace 相关对象（若实际数据库已存在）；
- `packs.description/icon_path/current_version_id`、`pack_versions`；
- content、quest、alert/update、import/build/release 表；
- task/outbox/activities 的 kind CHECK 联动；
- current/applied revision 的循环 FK 和索引。

契约测试最少包括：

- dashboard 计数与单独 SQL fixture 完全一致；包排序和 todayResolved 的本地时区边界；无包空态；未知 health 不冒充零。
- pack version 与 lock version 互不混淆；版本冲突、解归档和删除行为。
- content 三种 kind 的 schema、If-Match、validate/apply/rollback、影响模组和阻塞构建。
- quest 跨章节边、环、奖励、模组删除级联、排序和预览/应用隔离。
- import URL SSRF、预览过期/重放、三种 manifest、半失败回滚、任务恢复。
- task 内部 kind → 外部 type、canceled → cancelled 的稳定映射；暂停/取消/重试的 fencing。
- outbox 重投不重复 activity，activity 文案和 UI kind 六种映射完整。
- provider update-preflight 只读最近结果，不由 dashboard 隐式调用上游。
- token、Host、Origin、LAN、导入、密钥脱敏和审计失败回滚。

建议验收顺序：

```text
0002 migration + fixture
 → pack/version + dashboard
 → settings/onboarding + activity
 → content/quest draft/apply
 → import preview/confirm
 → delivery/build/release
 → 前端 USE_MOCK=false 契约验收
```

## 13. v7 自评（按 10 个维度）

评分沿用用户给出的 10 分制，但将原“鉴权/多租户就绪度”和“安全边界”合并为一个“鉴权与安全”；为保持十个维度，新增“工程化交付”。分数针对**本文设计质量**，不是当前代码完成度。

| 维度 | 评分 | 判断 |
|---|---:|---|
| 耦合性 | 8.5/10 | dashboard、content、quest、import、build 均通过 service/store/task/provider 边界落地；保留 HTTP adapter 负责前端字段映射，避免组件绑内部模型。 |
| 可维护性 | 8.5/10 | 版本、修订、图约束、导入阶段和错误码都有单一语义来源；表较多但每张表都有职责、FK、保留策略和测试入口。 |
| 可扩展性 | 8.5/10 | content kind、task kind、activity 映射、provider 和 release target 都有扩展点；未知 kind 需要迁移和契约测试，刻意不支持无约束插件化。 |
| 健壮性 | 8/10 | 导入、草稿、图校验、构建门禁、幂等和恢复路径完整；外部 provider 仍受其可用性影响，URL preview 的失败只能清晰失败不能消除。 |
| 数据一致性 | 9/10 | current/applied revision、pack version/lock、outbox、FK、唯一键和构建读取边界明确；SQLite 循环 FK 的实际建表顺序仍需在迁移 fixture 中验证。 |
| 异步任务能力 | 8.5/10 | v6 lease/fencing/retry/recovery 与 v7 import/build/publish 阶段接上，外部状态映射固定；不增加 websocket，采用轮询是单机取舍。 |
| Provider 扩展能力 | 8/10 | 搜索、版本、下载、指纹、发布和 update-preflight 都有清晰调用边界；不同平台 manifest/受限分发差异仍需 fixture 逐一冻结。 |
| 可观测性 | 8/10 | task、outbox、activity、audit、health/status、metrics 可互相追踪，request/task/pack ID 贯穿；领域查询慢查询和 content/quest 细粒度指标可在实现阶段补充。 |
| 鉴权与安全 | 8.5/10 | 删除多租户复杂度，统一单用户 token + Host/Origin + 路径/导入/密钥/审计边界；接受同机同用户进程和无 TLS LAN 风险，不提供虚假的角色安全。 |
| 工程化交付 | 7.5/10 | 本文给出迁移、契约测试和验收序列，可直接指导构建/发布落地；工程构建脚本、安装包和跨平台部署文件仍应在独立 build/deploy 文档冻结。 |

### 13.1 评分取舍说明

- 没有为“未来多租户可迁移”加分；删除 owner/workspace/member 后，单用户模型更简单、更可验证。
- 没有把所有页面状态复制成数据库字段；只有会影响构建、恢复或跨请求一致性的状态落库，纯展示状态由 API 聚合。
- 没有要求 websocket、分布式锁或远程对象存储；单机任务轮询、SQLite WAL 和本地 blobstore 足以覆盖当前产品范围。
- 工程化交付没有给满分，因为它需要与 `backend-architecture-v7-domain.md` 分开的构建/部署规范共同完成；这不是产品领域缺口的回避，而是保持文档职责单一。
