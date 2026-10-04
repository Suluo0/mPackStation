# 项目目录结构（每包一个 folder）—— 事实与方案

> 2026-10-04。来源：用户反馈「包管理应该像 Maven/Gradle 项目一样，每个项目有自己独立的目录」。
> 本轮先落**事实基线与方案**；存储重构涉及数据迁移与打包链路，需要专门一轮实施。

## 现状（2026-10-04 排查确认，详见 docs/active/user_check/排查确认记录.md #3）

- 全部业务数据在**单一 SQLite 文件** `{dataDir}/mpackstation.db`（`apps/server/cmd/server/main.go`，`store.Open`），包边界只存在于数据库键里（按 `pack_id` 分域，0021 起包删除级联）。
- 磁盘上**没有任何 per-pack 目录**：数据目录里只有 db、`server.lock`、`cache/minecraft-assets/`（按 MC 版本跨包共享）。
- 其余落盘点：`.mrpack` 产物写用户批准的导出目录（`allowed_export_dirs` 表）；导入暂存 `{dataDir}/tmp`；模组 jar 字节不持久化（解析时现拉）；图标是库内 BLOB。
- 应用级配置：进程配置走 flag/`MPACK_*` 环境变量/配置文件（`config.DefaultPath()`，OS 惯例目录）；CurseForge key 在 DB `secrets` 表。

## 归属评估（重构的事实基础）

天然属于**单包边界**（适合搬进项目目录）：`pack_mods`/selections、`mod_content`、`pack_catalog_*`、`catalog_generations`、conflicts/locks/versions/artifacts/releases、任务书 `content_*`、各类包内证据表。

天然**跨包共享**（应留全局）：`mod_identity` / `platform_projects` / `platform_releases`（平台元数据缓存）、`jar_index`（按 sha1 跨包去重）、`file_objects`（按哈希）、`secrets` / `settings` / `allowed_export_dirs` / `onboarding_state`、磁盘上的原版资源缓存。

## 目标形态（草案）

```
<用户选择的项目根目录>/            # 默认 Documents/mPackStation Projects，允许自选
  <包名>-<短id>/                   # 每个整合包一个项目目录（Maven/Gradle 式）
    pack.toml                     # 包元数据（人可读、可进 Git）
    overrides/                    # 随包分发的配置/脚本（与 .mrpack overrides 同构）
    exports/                      # 这个包的构建产物
    cache/                        # 这个包自己的临时物（可删）
<系统数据目录>/                    # dataDir（保持现状：/tmp/mpack-data 或 OS 惯例）
  mpackstation.db                 # 只留跨包共享的表（平台缓存/jar 索引/secrets/settings）
  cache/minecraft-assets/
```

要点：

1. **项目目录 = 用户可理解、可备份、可手动携带的单位**；`.mrpack` 构建时 `overrides/` 直接来自项目目录，二者同构，用户改了立即生效。
2. **SQLite 仍是运行时引擎**：包数据是否整体搬出库（改成目录内文件）是第二层决策。第一版可以只把「人可读的包描述 + overrides + 产物」落到项目目录，库内数据保留（目录是库的**投影 + 产物容器**，不是替代）——这样迁移成本可控，也符合「构建不需要外网、同一输入产出同一 zip」的确定性原则。
3. **pack.toml 先做投影不做权威**：权威仍是 `pack_mods` 链路；pack.toml 由服务端在每次 selection/lock 变更后落盘。等导入/导出闭环验证过，再评估让 pack.toml 升级为权威。
4. **迁移**：新增包时创建目录；存量包提供一次性「建立项目目录」动作（按 pack_id 生成）；删除包不删用户目录（用户的文件）。

## 未决问题

- 项目根目录的发现与切换 UI（项目管理面板给「在访达/资源管理器中显示」）。
- 多开/移动硬盘场景：项目目录在移动盘上时 dataDir 共享缓存是否可用（应可用，缓存按 MC 版本共享）。
- `allowed_export_dirs` 与项目目录内 `exports/` 的关系（项目目录内的 exports 应默认豁免批准流程，因为它本来就是用户自己的地盘）。
