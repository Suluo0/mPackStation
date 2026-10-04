# 端到端能力基线 2026-09-29

脚本：`scripts/e2e-baseline.sh`（可重放，只依赖 `BASE`/`TOKEN`/`OUT`/`DATA`/`MCVER`/`LOADER`）
证据：`docs/archive/project-state-history/e2e-baseline-20260929.log`（终轮）＋ `-run1.log`/`-run4.log`（演化过程）＋ 逐步响应体 `/tmp/e2e-out-5/*.log`
环境：HEAD `89195a7`，`go run` 二进制，`-addr 127.0.0.1:18899 -data /tmp/mpack-e2e`，MC `1.21.1` + fabric，macOS arm64

## 结论

**`PASS=27 FAIL=3 SKIP=0`。设计-锁定-内容-任务书四段真实可用；「打包」和「启动」两段是假的。**

终局断在两处，且都不是脚本问题：

1. **构建产物不是 .mrpack，也不含任何模组**。`POST /packs/{id}/build` 只要调用方自带 `files[]`，把它原样压成 zip。实测产物 `0.1.222046-bfedf896b1f765a4.zip` **283 字节**，`unzip -l` 里只有一个 160 字节的 `modrinth.index.json`（内容就是调用方传进去的那份占位 manifest）。全仓 Go 服务端 `mrpack` / `manifestVersion` 零命中（`build.go:332` 文件名硬写成 `<version>-<fp16>.zip`，`Kind:"zip"`）。
2. **启动器在本机结构性不可用**。带合法 payload 调用 `POST /api/launcher/install` → 任务 2 秒即 `failed`，error 原文：`start mpack-launcher: fork/exec /private/tmp/.tools/launcher/mpack-launcher.exe: no such file or directory`。根因见 D5。~~`launcherCore/` 在本机从未编译（无 `target/`，只有 Cargo.toml/src/tests）~~ —— 2026-09-30 更正：这句里"本机没有 Rust 工具链"的推断是**误判**，brew rustup 一直在装，只是 `~/.rustup/toolchains/stable-aarch64-apple-darwin/bin` 不在默认 PATH；真编译真运行已于 2026-09-30 跑通（`docs/tests/evidence/terminal-run-2026-09-30.log`）。当时"仓库里没有 `target/`"这个观察本身没错，错的是由它推出"本机做不到"。

所以 HANDOFF 那句「流水线终局必须通向启动一个 Minecraft」目前**在 API 层是断链**，不是差一点。

## 真实可用（逐条有响应体佐证）

| 环节 | 证据 |
|---|---|
| 建包 | 201 `pack-22fe30cd9493c34b94684724` |
| 面向包搜索 | jei 20 条 / rei 3 条 / cloth-config 5 条，`provider=modrinth`，真实下载量与 icon URL |
| 版本解析 | `/mod-versions` 候选 3720/614/161 条，按 `1.21.1+fabric` 取首个兼容版本（`Dd2GGBzd` 等） |
| 添加模组 | 3 × 201，落真实 jar 名 `jei-1.21.1-fabric-19.57.0.450.jar`；`mirrorSource=null`（CF key 未配） |
| 依赖与冲突 | 锁快照 1 份 + 4 条待处理冲突（JEI/REI 功能重叠，判定合理） |
| 包内容目录 | rebuild → `succeeded`（缓存命中时 10s），`locale=zh_cn`，物品页 2.7 MB |
| 物品详情 | `minecraft:acacia_boat` → 「金合欢木船」、2 语言、`iconStatus=ready`、`tags=[minecraft:boats]` |
| 模组内容解析 | 202 → 15s 到终态 `success`，`parsedCount=83`，条目 kind=metadata/lang |
| 解析后目录联动 | 手动二次 rebuild 后 **JEI 的「未纳入目录」warning 消失**（另两个未解析模组仍在）；代码路径确认存在：`mod_content_task.go:129` 解析成功后 `SubmitCatalogInit` |
| 任务书闭环 | draft PUT 200（If-Match 0）→ validate `passed` → apply `applied` → preview/history 200 |
| 导出目录注册 | `POST /api/export-dirs` 201 `ready`；构建确实落盘到该目录 |

## 缺陷

### D1 阻塞终局 · 构建不装配包，只归档调用方给的文件
- `service/build.go:405-406` `validateBuildInput` 要求 `len(Files) > 0`；契约 `docs/api/contract.md:910` 写的是 **`files`（可选）**。按契约调用（`{packVersionId, exportDirName}`）→ 400 `invalid_argument`，本轮 `build mrpack` FAIL 就是这一条。基线脚本同时保留了「契约形」和「前端形」两种调用，把矛盾钉成可重放断言。
- 前端已按实现绕开：`PackPages.tsx:440-452` 自己拼一个 `{formatVersion:1, dependencies:{}}` 的 manifest 传上去，**不含 files 数组、不含 jar**，注释 `generatedBy:'mPackStation'`。产物 283 B 即由此而来。
- 结果：`.mrpack` 语义（模组文件清单 + overrides + 依赖）在系统里不存在；契约里的 422 `build_blocked`（有未解决冲突时）和 403 `export_dir_not_allowed` 两条分支从未被执行过——本轮包里正好有 4 条未解决冲突，仍直接进了构建。

### D2 阻塞级 · `POST /packs/{id}/versions` 唯一约束冲突裸 500
新建包时服务端已自动写入 `0.1.0 / draft` 行，于是**对刚建好的包正常发起的第一个动作就撞约束**，返回 500 `internal_error` 而非 409/422。基线脚本加了显式探针 `duplicate version (期望 409)` → 稳定 500。
次生不一致：自动行 ID 前缀 `packver-`，手动创建前缀 `version-`。
同类复发：已 resolved 的 `issue-670c4eeb`（tool_install 首次提交 500）是同一个病——约束错误没走错误映射。

### D3 契约语义 · `GET /packs/{id}/quests` 把「包存在但没任务书」报成 `pack_not_found`
同一包 `catalog/status` = 200，可排除包真不存在。链路：`store/content_repo.go:371-375` 无 `quest_books` 行抛裸 `ErrNotFound` → `httpapi.go` 统一映射 `pack_not_found`。契约 `:838` 写的就是这个码，所以是**契约把两种语义挤进一个码**；调用方只能靠猜 `If-Match: 0` 建档（写侧正常）。

### D4 参数校验缺失 · `launcher/install` 受理残缺 payload
`LauncherInstallPayload` 字段是 `version`/`minecraft_dir`（`launcher_task.go:14-19`），**没有 packId**。传 `{"packId":...}` 得 202 + taskId，异步立刻 `failed: version and minecraft_dir are required`。同步可判的必填项放到异步才报错，契约又完全没写这个接口（见 D6）。

### D5 结构性 · 启动内核二进制路径在 macOS 上永远解析不到
`service/launcher/launcher.go:37-39`：
- 文件名硬编码 `mpack-launcher.exe`，非 Windows 平台必然 no such file
- 拼的是 `workbenchRoot/.tools/launcher/`，而实参是 **`-data` 数据根目录**（报错路径 `/private/tmp/.tools/launcher/...` 即 `/tmp/mpack-e2e` + 后缀），与注释「next to the server executable」不符
- 注释声称「or on PATH」，但代码没有 `exec.LookPath` 兜底
- 失败把 Go `fork/exec` 原文当 error 透出给 API 消费者，没有归一错误码

### D6 文档缺口 · 启动器接口零契约
`docs/api/contract.md` 全文 `launcher` 出现 0 次，而 `POST /api/launcher/install`（`routes_system.go:92`）已实现且 202 语义正确。

### D7 覆盖缺口 · 包目录域没有「按物品反查配方」端点
`GET /catalog/items/{id}` 返回字段实测为 `displayName/evidence/iconStatus/id/modelPath/names/resolvedLocale/tags`，无 recipes/usages；catalog 路由全集只有 `icon/status/rebuild/(list)/items/{id}/tags/{id}`。配方只能从 `mods/{modId}/content` 按模组取。契约与 ADR 均未定义该端点，属设计留白，但「点物品看配方」这条 JEI 核心体验在包目录域无 API。

## 复核后撤销的怀疑（避免下轮重复误判）

1. ~~目录重建与模组解析时序耦合~~ → **自动重投存在**（`mod_content_task.go:129`）。真实行为是最终一致：首建必然带「未解析」warning，解析完成后自动重建，实测 revision 14754→14842。
2. ~~任务终态字段用 `succeeded`~~ → 契约 `:50` 明确枚举 `queued/running/paused/success/failed/cancelled`，**任务用 `success`**；`succeeded` 只属于 mod_content run 与 catalog status 另一套词汇。脚本此前按 `succeeded` 轮询任务，白等 200s。
3. ~~跨包 catalog revision 泄漏~~ → `sourceRevision` 是全局批次号（jar 按 sha1 跨包共享，符合 `decision-4ba2b3c4`），`builtAt` 才按包区分，实测三包各自独立。
4. ~~`POST /packs/{id}/build` 路由不存在~~ → 存在（`routes_publish.go:110`），此前 400 是 D1 与空 packVersionId 的连锁。

## 基线脚本自身的坑（已修）

1. `/mod-search` 条目字段是 `provider/id/slug/name/downloads`，**不含 versionId**；旧版按 `projectId/versionId` 取空后被 `read` 按空格切碎，把模名的词当 projectId 发出 → 三连 404。现显式查 `/mod-versions` 并按 mcVersion+loader 过滤。
2. 固定包名 → 第二轮起 422 `pack_name_duplicate` 直接 exit 1。现加时间戳；版本号同理。
3. `If-Match` 兜底 1 → 新包必 409 `revision_conflict`。现用 0。
4. `resolve` 期望 200，实际 202（`routes_mods.go:122` 返回**完整最终锁快照**却用 202，语义自相矛盾，契约未写码——保留为待议，不算本轮缺陷）。
5. 解析后固定 `sleep 8` 读列表，拿到 `running/parsedCount=0` 会误判解析失败；现轮询到终态。
6. catalog 轮询用 `ready` 匹配，实际是 `succeeded` → 每轮空转满 300s。

## 重放

```bash
cd apps/server && MPACK_TOKEN=e2e-token-123 go run ./cmd/server -addr 127.0.0.1:18899 -data /tmp/mpack-e2e
OUT=/tmp/e2e-out-6 bash scripts/e2e-baseline.sh          # 另开终端，约 1-3 分钟（目录缓存命中）
```

## 未覆盖（别当成已通）

- 发布链路：`publish/{provider}`、delivery-checks、release 轮询与重试，本轮未打
- 冲突解决动作 `ResolveConflict`（accept/ignore）与 422 `build_blocked` 分支
- CurseForge 侧与双平台镜像钉版（key 未配，`mirrorSource` 恒 null）
- 启动器任何真实执行（安装/启动/Java 下载/离线账号）
- SNBT/FTB 导出、KubeJS 导出、IconExporter、`allowModDistribution`：全仓 grep 0 实现，与 HANDOFF 的「非目标」一致
- 前端点击级回归沿用上轮 33/33 PASS，本轮未重跑
