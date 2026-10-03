# mPackStation 能力清单（四张列表，2026-10-02 实测）

口径：入口一律指 web3（`http://127.0.0.1:5275`，一对一绑后端 `127.0.0.1:18880`，数据目录 `/tmp/mpack-clean`）。
详细证据与复现命令在同目录 `web3-regression-from-web1-2026-10-02.md`（D 测试矩阵 / E 缺陷 / G 机械核查）。

图例：✅ 已具备且本轮实测通过 · ❌ 缺失（后端有，web3 无入口） · ➕ 新增（web1 没有） · ⚠️ 有缺陷（入口在，实测出问题）

---

## 1. ✅ 已具备、测试通过（28 项）

| # | 能力 | web3 入口 | 接口 | 实测结果 |
|---|---|---|---|---|
| 1 | 平台健康自检 | 状态条 / 设置弹窗 | `GET /api/system/health` | CF ✓可达 · Modrinth ✓可达 · 存储可写 · 写令牌 ✓ |
| 2 | MC 版本候选 | 新建包「MC 版本」下拉 | `GET /api/meta/mc-versions` | 列出并选中 1.21.1 |
| 3 | 新建整合包 | 迎新页 | `POST /api/packs` | `pack-40c638744e041f185dede8f8`，建包自动起目录重建 |
| 4 | 包切换菜单 | 顶栏包名 | `GET /api/dashboard` | 包列表 + 计数渲染 |
| 5 | 填 CurseForge Key | 设置弹窗（**只经 UI，未落配置文件**） | `PUT /api/system/providers/curseforge/key` | 未配置/✖不可达 → ✓已配置/✓可达 |
| 6 | 双平台搜模组 | 来源面板「+」 | `GET /api/packs/{p}/mod-search` | 13 命中含 `provider=curseforge` → Key 真生效 |
| 7 | 列模组版本 | 命中后版本列表 | `GET /api/packs/{p}/mod-versions` | Modrinth 614 条正常（CF 见 ⚠️1） |
| 8 | 兼容版本过滤 | 版本列表 | 同上（前端过滤） | 614 → 6 兼容 + 608 折叠（本轮新修） |
| 9 | 添加模组并钉版 | 点版本行 | `POST /api/packs/{p}/mods` | REI `16.0.799+fabric` 成功，来源计数 0→1 |
| 10 | 模组启用/停用 | 模组行开关 | `PATCH /api/packs/{p}/mods/{m}` | 停用→启用→停用 三态回读一致 |
| 11 | 触发模组解析 | 模组行「解析」 | `POST .../content/parse` | 202 起任务，已解析 73 / 文件 73，无错误 |
| 12 | 解析进度轮询 | 来源面板 | `GET .../content/run` | 轮询到 succeeded 后行计数落定 |
| 13 | 模组内容下钻 | kind 行写 `?ns=&src=&type=` | `GET .../content?kind=` | 物品/方块/标签清单渲染 |
| 14 | 物品目录读取 | 索引态 | `GET /api/packs/{p}/catalog` | 1330 物品 |
| 15 | 目录状态 | 索引态工具条 | `GET .../catalog/status` | 物品 1330 · 配方 1290 · r14826 |
| 16 | 物品图标 | 网格 `<img>` | `GET .../catalog/icon` | 全部 naturalWidth=32 |
| 17 | 目录重建 | 索引态「重建目录」 | `POST .../catalog/rebuild` | revision 递增、计数与空态同步 |
| 18 | 高级过滤器 | 索引态漏斗 | 本地 + `?f=&fmode=&fs=1` | `#tools` 加条件后命中 30 |
| 19 | 网格/表格切换 | 工具条 | `?view=table` | 名称/ID/来源三列 |
| 20 | 索引态搜索/排序/清除筛选 | 工具条 | 本地目录缓存 | 均生效 |
| 21 | 命令面板 | ⌘K | 本地缓存检索 | 8 条命令 |
| 22 | 关系透镜 | 四透镜「关系」 | 目录 + 内容接口 | 苹果「作为产物 0 / 作为原料 1 → minecraft:golden_apple」 |
| 23 | 焦点详情 | 四透镜「索引」选中项 | 同上 | 多语言名称 / 所属标签 / 来源依据 / 传送门跳转（显示口径见 ⚠️5） |
| 24 | 任务列表 | 底部停靠 | `GET /api/tasks?recent=20` | 5 条任务、状态与进度实时 |
| 25 | 任务日志 | 底部停靠选中行 | `GET /api/tasks/{id}/log` | 6 行「已索引 1330 个物品、1062 个方块、331 个标签和 1290 个配方」 |
| 26 | 最近动态 | 侧栏页脚 | `GET /api/activities?limit=10` | 渲染 |
| 27 | 迎新清单打勾 | 迎新页四步 | `GET/PUT /api/onboarding` | 存 Key 后「配置 CurseForge Key」自动变 ✓ |
| 28 | 包健康与冲突计数 | 顶栏问题徽标 | `GET /locks`、`/conflicts`、`/health` | 空态 `problems-chip clean`，健康 100 |

另：`GET /api/launcher/installs`（填目录后回报「该目录还没有本包的安装记录」）与「登记包版本」（`POST .../versions`，UI 输 0.2.0 点登记 → 列表 1→2，channel 落 release）也实测通过。

---

## 2. ❌ 缺失：后端能力在，web3 没有入口（13 类 / 38 个函数）

机械核查：`grep -rlw <函数名>` 在 `apps/web3/src` 排除 `src/api/` 后零命中（命令见 G 节，2026-10-02 输出 38 行）。

| # | 缺失能力 | 接口 | 零调用函数 | 对应 |
|---|---|---|---|---|
| 1 | 删除整合包 | `DELETE /api/packs/{id}` | `packs.ts deletePack` | A1 |
| 2 | 改包元数据（重命名） | `PATCH /api/packs/{id}` | `packs.ts updatePack` | A2 |
| 3 | 单包详情 | `GET /api/packs/{id}` | `packs.ts getPack` | A2 |
| 4 | 移除模组 | `DELETE /api/packs/{p}/mods/{m}` | `mods.ts removeMod` | A5 |
| 5 | 依赖解析（产锁 + 冲突） | `POST /api/packs/{p}/resolve` | `mods.ts resolvePack` | A6 |
| 6 | 兼容知识库推荐 + 一键加装 | `GET .../mod-recommendations` | `mods.ts listModRecommendations` | A4 |
| 7 | 模组内容图标补齐 | `POST .../content/icons/resolve` | `modContent.ts resolveModContentIcons, getModContent` | A8 |
| 8 | 任务 暂停/继续/取消/重试 | `POST /api/tasks/{id}/{action}` | `tasks.ts pauseTask, resumeTask, cancelTask, retryTask` | A17 |
| 9 | 清除 CurseForge Key | `DELETE /api/system/providers/curseforge/key` | `system.ts clearCurseForgeKey` | A19 |
| 10 | 导出目录白名单 + 目录选择器 | `POST /api/export-dirs`、`GET /api/fs/browse` | `fs.ts registerExportDir, browseDirectories`（整模块无人引用） | A16 |
| 11 | 系统状态卡 | `GET /api/system/status` | `system.ts fetchStatus` | A20 |
| 12 | Prism 便携安装 / 微软登录 | `POST /api/tools/prism/install`、`/login` | `system.ts installPrism, launchPrismLogin` | A18 |
| 13 | 魔改 + 编排 + 构建发布 全部写操作 | `POST /content`、`.../draft\|validate\|apply\|rollback\|history`、`/quests/*`、`/delivery-checks/run`、`/build`、`/publish/{provider}`、`/releases/{id}/poll\|retry` | `content.ts` 11 个 + `releases.ts` 5 个 | A10-A15 |

同族但**无用户可见损失**（只是 api 层留了重复出口，不用补 UI）：`packs.ts listPacks`、`mods.ts listMods, searchMods` —— 包列表走 `dashboard.ts fetchDashboard`，模组树走 `modContent.ts listModContent`。

另有两项非接口类缺失：本地 zip 导入的「上传/拖拽」入口（web1 有 `PackModals.tsx:205-215`，web3 下拉只列 curseforge/modrinth）、目录语言切换（`?locale=` 固定 `zh_cn`，A21）。

---

## 3. ➕ 新增：web1 没有、web3 才有的

标记法核查：同一关键字在 `apps/web/src` 命中 0 个文件、在 `apps/web3/src` 命中 ≥1。

| # | 新增能力 | web1 | web3 证据 |
|---|---|---|---|
| 1 | URL 作状态单一事实源（可分享/前进后退） | 0 处 `useUrlState` | `apps/web3/src/app/url.ts`，21 个文件引用 |
| 2 | 四透镜工作区（索引/关系/魔改/编排 同屏切换） | 0 处 `lens` | `editor/EditorHeader.tsx`、`EditorArea.tsx` |
| 3 | 关系透镜（物品的产物/原料双向清单） | 0 处 `ModeGraph` | `editor/ModeGraph.tsx`（注释自认 v0 非图形化） |
| 4 | 底部停靠任务面板 + 日志流 | 0 处 `BottomDock` | `dock/BottomDock.tsx` |
| 5 | 状态条（health 常驻） | 0 处 `StatusBar` | `app/StatusBar.tsx` |
| 6 | 侧栏「最近动态」页脚 | 0 处 `ActivityFooter` | `panels/ActivityFooter.tsx` |
| 7 | 顶栏问题徽标弹层里就地处置冲突 | 0 处（web1 连按钮都没有） | `app/ProblemsPopover.tsx:52-53` |
| 8 | 设置弹窗 | 0 处 `SettingsModal` | `app/SettingsModal.tsx`（CF Key 只经此填入） |
| 9 | 高级过滤器构造器（kind:value + and/or） | 0 处 `catalogSearch/fmode` | `editor/FilterBuilder.tsx`、`app/catalogSearch.ts` |
| 10 | 从链接导入整合包（表单） | 0 处调用 `inspectImport/confirmImport` | `editor/Welcome.tsx`（表单迁自 web2；能力本身见 ⚠️4） |
| 11 | 版本列表兼容过滤（本轮新增） | 无此逻辑 | `panels/SourcesPanel.tsx` `isCompatible` + 「其他版本 608 ▸」折叠 |
| 12 | 确认导入后的任务落点提示（本轮新增） | 无 | `editor/Welcome.tsx` notice 行 |

---

## 4. ⚠️ 测出缺陷（入口在 web3，实测不通过）

| # | 缺陷 | 级别 | 层级 | 现象与根因 | 状态 |
|---|---|---|---|---|---|
| 1 | CurseForge 取模组版本一律 500 | **阻塞** | 后端 | `GET /api/packs/{p}/mod-versions?provider=curseforge` → `internal_error`；同 Key 直连 CF 返回 200。根因 `apps/server/internal/provider/http_adapter.go:309` 把 `relationType` 声明成 `string`，CF 实际回数字 `{"relationType":2}` → 解码失败。三前端调用逐字相同，非 web3 退化 | 未修（按你的规则只记录） |
| 2 | 运行面板无法端到端 | 环境+后端 | 前端无法修 | 缺 `mpack-launcher` 二进制（`MPACK_LAUNCHER_BIN`）；本机无任何 `.minecraft`，「重装版本」恒禁用；错误文案里 `<workbench>` 占位符没展开 | 等你装二进制 |
| 3 | 配方「有数无入口」 | 前端可修（口径要你定） | 前端 | 工具条显示「配方 1290」，但 `ModeIndex.tsx:16` 要求 `type && src` 同时存在才渲染内容面，单独 `?type=recipe` 被忽略 | 待你选：加对象域切换 或 撤掉数字 |
| 4 | 链接导入是空壳 | 后端未实现 | 后端 | 解析 200 却显示 `(未命名) · 0 个条目`；确认导入起任务后两条（`t-1790946205345770000-30`、`t-1790946314794798000-36`）均 `failed: pack archive could not be parsed`。根因 `service/import_service.go:91-96` 把 URL 字符串当归档内容落盘，全程不下载 | 未修 |
| 5 | 焦点详情把内部 ID 当人话 | 体验 | 前端可修（口径要你定） | 「多语言名称」来源列直出 `minecraft-pack-<packId>`（`EditorHeader.tsx` `{n.source}`） | 待你定显示口径 |
| 6 | 魔改/编排是只读占位，空态文案指向不存在的入口 | 能力未迁 | 前端+排期 | `ModeEdit.tsx:74` 只读 textarea，0 文档时提示「索引态选配方去魔改」（撞上 ⚠️3 死路）；`ModeQuest.tsx` 提示「在任务编辑器里保存草稿」——web3 没有任务编辑器 | 对应 A10-A12 |
| 7 | 交付检查空态是假承诺 | 文案+入口 | 前端 | `BuildPanel.tsx:88` 写「登记一个版本后，交付闸门会自动跑检查」；实测登记 0.2.0 后 `delivery_checks` 仍 0 行，跑检查的 `runDeliveryChecks` 前端零调用 | 已记入 E4 |
| 8 | 「确认导入」点击后原毫无反馈（已修） | 前端 | 前端 | `packId` 为 null 时界面原地不动，用户以为按钮没生效 → 现给带 taskId 的落点提示 | **本轮已修** |
| 9 | 版本列表三条交互缺陷（已修） | 前端 | 前端 | 兼容过滤缺失（614 全灌）、失败后永卡「版本载入中…」、错误串到模组树顶 | **本轮已修** |

未触发（不是缺陷，是缺样本）：`POST /conflicts/{c}/resolve|ignore` 按钮存在但干净包 0 冲突；产物 `.../artifacts/{id}/download` 只在 `status=ready` 出现，而构建本身缺入口（❌13）。
