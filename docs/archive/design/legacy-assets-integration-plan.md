# mPackStation 内容与分发设计（Legacy Assets 接入 + 多格式导出）

> 方向（用户拍板，2026-09）：打包不是一种格式，而是**分发**——一个接口，用户选目标平台，出那个平台能用的包。
> 第一版支持 **Modrinth / CurseForge / 本地 zip** 三种；直链瘦身作为 Modrinth 的选项；mrpack 差分作为可选质检。
> 原则：**前端可"平移"，后端只可"重写逻辑"**；平台格式是固定规范，凡"按规范填字段"，不做算法；数据由 `provider`/`store` 已备。
> 现状基线：`docs/standards/development-priority.md` P0–P9，当前在 P7 前后端联调；`content.go` 已支持 `recipe/structure/ore`；`build.go` 已是成熟的"打包 DTO + 确定性 zip + artifact 登记"骨架，但只产一种自定格式本地 zip。

---

# 一、分发：多格式导出（核心）

## 1. 目标
单个 build 接口，调用方（前端/任务）传 `format`，按目标平台规范产出对应包；`artifact` 按格式登记，可供下载/上传。

## 2. 能力具备度核查（设计师前置）—— 三平台数据源齐备

> 前置核查（2026-09）。**更正：下载地址确实存在**——`provider.Metadata`/`Download` 返回 `DownloadURL/SHA1/SHA256/Size`（字段 `json:"downloadUrl"`，HTTP adapter 从平台 `files[].downloadUrl` 解析）；`ResolvePack`（mods.go）**已调用** `provider.Metadata`。只是它未落成独立列、service 也未读它。

**数据映射（全部具备）**

| 需映射 | 来源 |
|--------|------|
| MC / loader 版本 | `pack`：`mc_version`、`loader`、`loader_version` |
| 平台 + 引用（CF fileID/平台 version） | `pack_mods`：`source`、`project_id`、`version_id`、`file_name`、`required`（`uq_pack_mods_remote_project ON (pack_id, source, project_id)` 平台+ID 唯一）|
| 跨平台镜像 | `pack_mods`：`mirror_source`、`mirror_project_id`、`mirror_version_id` |
| **下载直链 / sha1 / sha256 / size** | `provider.Metadata`（或 `Download`）：`Version.DownloadURL/SHA1/SHA256/Size` |
| 哈希 / 大小（已落库） | `jar_index`（按 `sha1`）：`sha256`、`size_bytes` |
| 内嵌 jar 字节（回退） | `jar_index.file_path` |
| 依赖 / loader | `LockSnapshot`（`schemaVersion/packId/mods[]/dependencies/conflicts`）|
| overrides 字节 | `BuildInput.Files`（调用方传入）|

**所以分平台看**

- **CurseForge**：projectID / fileID / required / sha1 / sha256 / size / file_name / mc+loader 全部具备 → **可直接做**。
- **本地 zip**：等同现状 → 回归即可。
- **Modrinth**：path / sha1 / sha256 / size / env 具备；`downloads[].url` 由 `provider.Metadata`（用 `pack_mods.project_id/version_id` 调用，与 `ResolvePack` 相同）取 → **可做**。

**唯一的取数工程点（非缺口、非新增列）**

- Modrinth formatter 需要"按 mod 的 `project_id/version_id` 从 `provider.Metadata` 取 `DownloadURL/SHA1/SHA256/Size`"这一步——`ResolvePack` 已有相同调用，可直接复用/提取。
- 可选优化（非必须）：若希望 Modrinth 直链构建不依赖运行时 provider，可在解析依赖时顺手把 `downloadUrl` 落库（新增列 + 新 migration）。
- 无直链时：`downloads` 可省略 → jar 内嵌 `overrides/mods/*.jar`（从 `jar_index.file_path` 读字节，要求已下载）。

> 结论：**"分"三平台数据源齐备，无能力缺口；只需给 Modrinth formatter 加一个"从 provider.Metadata 取 downloadUrl"的读点（ResolvePack 已有同款调用）。**

## 3. 架构：一次内容模型 + 按格式序列化

```text
build 接口（传 packVersionId + format + buildConfig + files + snapshots）
   └─ 公共管线（已有）：校验 → 取版本/锁 → 交付检查 → 幂等 → 序列化 → 原子写 + 登记 artifact → 返回
         └─ 序列化阶段按 format 选 formatter：
              ├─ local-zip  formatter   （现状 writeDeterministicZip + 自定指纹 manifest）
              ├─ modrinth  formatter    （modrinth.index.json + overrides/）
              └─ curseforge formatter   （manifest.json + modlist.html + overrides/）
```

### 内容模型（跨格式共用）
```go
type PackCatalogFile struct {                     // 一个模组文件的实际来源引用
    Path       string   // 目标路径（mods/xxx.jar）
    DownloadURL string  // 直链（provider）
    SHA1       string
    SHA512     string
    Size       int64
    ProjectID  string   // CF
    VersionID  string   // CF fileID / 平台 version
    EnvClient  bool
    EnvServer  bool
    Required   bool
}
type BuildContent struct {
    Name, Version, Author, Summary, MCVersion, Loader, LoaderVersion string
    Files        []PackCatalogFile
    Overrides    []normalizedBuildFile   // 配置/资源/本地文件（B 0 字节）
    ServerOverrides []normalizedBuildFile
    Dependencies map[string]string       // minecraft/forge/neoforge/fabric-loader/quilt-loader
}
```

### Formatter 接口
```go
type BuildFormat string
const (
    FormatLocalZip    BuildFormat = "zip"
    FormatModrinth    BuildFormat = "modrinth"
    FormatCurseForge  BuildFormat = "curseforge"
)
type Formatter interface {
    Format() BuildFormat
    // 序列化 BuildContent 为目标 zip 字节（到 dst）；files 来自 catalog，overrides 字节已备
    Write(ctx context.Context, dst io.Writer, content BuildContent, opts BuildOptions) error
    // 幂等指纹：不同 format、不同直链开关必须不同指纹
    Fingerprint(content BuildContent, opts BuildOptions) string
}
```

## 4. 三种格式明细

| | Modrinth | CurseForge | 本地 zip |
|---|---|---|---|
| manifest | `modrinth.index.json` | `manifest.json` | 无 / 可选 `modlist` |
| files 引用 | `path + hashes{sha1,sha512} + downloads + fileSize + env` | `projectID + fileID + required` | 直接文件 |
| 依赖 | `dependencies`（minecraft/loader） | `dependencies`（mc/loader） | — |
| 目录 | `overrides/`、`server-overrides/` | `manifest.json` + `modlist.html` + `overrides/` | 原样 |
| 模组 jar | 依直链选项：`downloads` 直链 或 内嵌 `overrides/mods/*.jar` | `files[]` 引用 CF（不内嵌） | 原样文件 |

**直链瘦身** = Modrinth formatter 的一个选项（`opts.DirectLinks=true`）：files 用 `downloads=[url]`、jar 不进 `overrides`；缺 sha512 时在下载补计算。不再是独立任务/脚本。

## 5. 接口设计

- `BuildInput` 增加 `Format BuildFormat`（缺省 `zip` 兼容现状）。
- build 端点（`POST /api/packs/{id}/build` 与 `/versions/{versionId}/build`）的 body 在 `buildConfig` 或顶层增加 `format` 字段；同时 `Body.Files` 仍作 overrides 字节来源。
- 前端 `buildPack` body 增加 `format`；`PublishPage`/工作台提供"导出格式"选择（Modrinth / CurseForge / 本地 zip）。
- `artifact.Kind` 按 `format`（`"zip" / "modrinth" / "curseforge"`）；**幂等查找按 (format, fingerprint)**（当前仅按 `"zip"` + fingerprint，需扩展）。
- `BuildConfig`（json）承载 `format` 与 `directLinks` 等选项，作为 `opts` 传入 formatter。

## 6. `build.go` 重构路径（最小侵入）
1. 保留公共管线（校验/锁/交付检查/幂等/原子写/登记）—— 本来就是通用的。
2. 把 `buildManifest` + `writeDeterministicZip` 归入 `FormatLocalZip` formatter（现状行为，自定指纹 manifest 保留或降级为无 manifest）。
3. 新增 `FormatModrinth` / `FormatCurseForge` formatter：从 `LockSnapshot`/`provider` 组装 `PackCatalogFile`（catalog），把 `Files` 字节作为 overrides，序列化到 zip。
4. 引入 `formatters map[BuildFormat]Formatter`，`BuildPack` 按 `in.Format` 分发；指纹含 format+directLinks。

## 7. 验收（做成了才算）
- [ ] `POST /api/packs/{id}/build` 传 `format=modrinth` → 得含 `modrinth.index.json`（files 有正确 path/sha1/sha512/downloads/size/env + dependencies）→ zip；PCL2/HMCL 可读。
- [ ] `format=curseforge` → 含 `manifest.json`（files 用 projectID/fileID/required）+ `modlist.html` + `overrides/`，可上传 CF。
- [ ] `format=zip` 行为与现状一致（回归）。
- [ ] 同一内容不同 format 幂等指纹不同、各自独立登记；artifact.Kind 正确。
- [ ] 直链选项：`modrinth+directLinks` 时 files.downloads 填直链、jar 不进 overrides；缺 sha512 正确回退内嵌。

## 8. 废弃线 / 边界
- 任一平台要求的字段 mps 无来源（如 CF 必须的 `modlist.html`、或某平台特有头）→ 该 formatter 标记"不可用/待补字段"，不产出半成品。优先三格式中已能闭合的。
- 若产品后续砍掉某平台 → 删对应 formatter，不动主管线。
- **P3（mrpack 差分）** 不再是独立事项：作为发布质检，用于对比同一包的 modrinth 产物与本地 zip 是否内容一致，可选、低优先，仅在需要时用 `compare-mrpacks.ps1` 逻辑（可转 Go 包 `mrpackdiff`）。

---

# 二、P1 多方块可视化编辑器（概要，独立于分发）
- 后端 `content.go` 的 `structure` 是**文件引用模型**（`file/size/rotation/anchor/parameters`，白名单 + 必填 file/size），legacy `IsoPreview` 是**逐格 `layers` 模型** → 核心分歧点。
- 方案：不改后端既有字段；逐格数据放允许字段 `preview`（`{layers,sizeX,sizeY,sizeZ}`）。组件迁 `features/content/multiblocks/`（只取交互、不取旧 `/api/multiblocks` 依赖）；`useContentEditor` 加草稿回写（复用 `If-Match`）；`ContentEditorPage` 按 `doc.kind==='structure'` 挂载。
- **废弃线**：若后端/产品判断 structure 只该引用文件 → 只复用 `IsoPreview` 做"文件引用可视化预览"，删逐格编辑，不投入。

# 三、P2 搜索 URL 反解（概要）
- 提取 `parseUrl.ts` 纯逻辑（三平台 host→slug 正则 + `prettifySlug`）重写进 `mods.go|ModSearchAll`；PayloadCMS 的 find/create/事务/mod-links 全部作废。随 P5 搜索打磨。

---

# 汇总与推进
- **优先做分发**（本设计）：改造量集中，数据已齐备，能立刻兑现"一个接口选平台出包"的核心价值。建议顺序：`BuildInput.Format` + 公共管线解耦 → `modrinth` formatter（闭合 PCL2/HMCL）→ `curseforge` formatter → 前端 format 选择；直链作为 modrinth 选项。
- P1 编辑器/ P2 URL 搜索 与分发并行不冲突，按各自阶段推进（P6 / P5）。
- 共同底线（AGENTS.md）：不改已应用 migration；`service` 承载业务；`httpapi/store` 边界不破；provider 是唯一外部平台 HTTP 边界；前端旧样式不入全局。
