# web1 → web3 功能回归清单与测试矩阵（2026-10-02）

口径：
- **web1** = `apps/web/src`（用户确认「后端能力在 web1 是可以正常使用的」，因此把它当作能力基线）。
- **web3** = `apps/web3/src`（本轮唯一测试入口，http://127.0.0.1:5275）。
- 判定「web3 消失」的机械标准：web1 有真实调用点的接口，在 web3 里 **api 层有函数但 src 内（排除 `src/api/`）零调用点**，或 **有 UI 元素但无 onClick/无接口**。
- 抽验方式：对 19 个导出函数逐个 `grep -rn "\b<函数名>\b"` 排除 api 目录，命中数全为 0（2026-10-02 实测）。
- 修复原则（用户定）：优先改前端；前端满足不了的记缺陷不修，不改后端。

---

## A. web1 有完整交互、web3 完全消失的功能

| # | 功能 | 接口 | web1 证据 | web3 现状证据 |
|---|---|---|---|---|
| A1 | 删除整合包 | `DELETE /api/packs/{id}` | `apps/web/src/pages/DashboardPage.tsx:34`、`features/dashboard/PackList.tsx:24-34` | `apps/web3/src/api/packs.ts:53` 零调用点 |
| A2 | 改包元数据（重命名） | `PATCH /api/packs/{id}` | `apps/web/src/api/packs.ts:40`（web1 也是占位按钮） | `apps/web3/src/api/packs.ts:40` 零调用点 |
| A3 | 本地 zip 导入（拖拽） | `POST /api/packs/import*` | `apps/web/src/features/dashboard/PackModals.tsx:205-215` | web3 只有链接导入：`apps/web3/src/editor/Welcome.tsx:110,116` |
| A4 | 兼容知识库推荐 + 一键加装 | `GET /api/packs/{p}/mod-recommendations` → `POST .../mods` | `apps/web/src/hooks/useModSearch.ts:27,74-81` | `apps/web3/src/api/mods.ts:142` 零调用点 |
| A5 | 移除模组 | `DELETE /api/packs/{p}/mods/{m}` | `apps/web/src/hooks/useModSearch.ts:92` | `apps/web3/src/api/mods.ts:149` 零调用点；`panels/SourcesPanel.tsx:303-306` 只有启用/停用 |
| A6 | 依赖解析（产锁 + 冲突） | `POST /api/packs/{p}/resolve` | `apps/web/src/hooks/useDependencies.ts:21`、`pages/PackPages.tsx:327,339` | `apps/web3/src/api/mods.ts:153` 零调用点；lock 取了不消费（`app/PackSummaryContext.tsx:51`） |
| A7 | 冲突行处置（web1 也没有，web3 反而更强） | `POST .../conflicts/{c}/resolve\|ignore` | 无按钮（`apps/web/src/pages/PackPages.tsx:340-343`） | **可用**：`apps/web3/src/app/ProblemsPopover.tsx:28,29` |
| A8 | 模组内容图标补齐（下载/缓存原版资源） | `POST .../mods/{m}/content/icons/resolve` | `apps/web/src/pages/ModContentPage.tsx:177` | `apps/web3/src/api/modContent.ts:81` 零调用点 |
| A9 | 单条解析结果详情 | `GET .../content/{contentId}` | `apps/web/src/api/modContent.ts:70`（web1 未接 UI） | `apps/web3/src/api/modContent.ts:70` 零调用点 |
| A10 | 魔改：内容文档 CRUD 全链 | `POST /content`、`PUT .../draft`(If-Match)、`POST .../validate`、`.../apply`、`.../rollback`、`GET .../history` | `apps/web/src/pages/RecipeTweakPage.tsx:108,123,138,151,162,170` | web3 六个函数全零调用点（`apps/web3/src/api/content.ts:62,66,68,70,73,75`）；`editor/ModeEdit.tsx:74` 是只读 textarea |
| A11 | 任务书：草稿保存 / 校验 / 应用 / 回滚 | `PUT /quests/draft`、`POST /quests/validate`、`/apply`、`/rollback`、`GET /quests/history` | `apps/web/src/hooks/useEditors.ts:140,186,203`、`features/quest/QuestBookEditor.tsx:651,652,653` | web3 五个函数全零调用点（`apps/web3/src/api/content.ts:148,150,152,154,156`） |
| A12 | 任务书画布编辑（节点/连线/章节/属性面板/ItemPicker） | 本地态 + A11 落库 | `apps/web/src/features/quest/QuestBookEditor.tsx:643-1297` | `apps/web3/src/editor/ModeQuest.tsx:15,47` 只读节点密表格 |
| A13 | 交付检查「跑一次」 | `POST /api/packs/{p}/delivery-checks/run` | `apps/web/src/pages/PackPages.tsx:468` | `apps/web3/src/api/releases.ts:65` 零调用点；`panels/BuildPanel.tsx:88` 空态无入口 |
| A14 | 开始构建产物 | `POST /api/packs/{p}/build` | `apps/web/src/pages/PackPages.tsx:525` | `apps/web3/src/api/releases.ts:78` 零调用点；`panels/BuildPanel.tsx:75` 空态无入口 |
| A15 | 发布 / 发布记录轮询 / 重试 | `POST .../publish`、`GET /releases`、`POST /releases/{id}/retry` | web1 也只有 api 无 UI（`apps/web/src/api/releases.ts:71-85`） | `apps/web3/src/api/releases.ts:80,82,84` 零调用点 |
| A16 | 导出目录白名单 + 目录选择器 | `POST /api/export-dirs`、`GET /api/fs/browse` | `apps/web/src/pages/PackPages.tsx:481,518,598,659`、`features/common/DirectoryPicker.tsx:39` | `apps/web3/src/api/fs.ts:21,24` 整文件零调用点；`panels/RunPanel.tsx:61` 手输路径 |
| A17 | 任务控制（暂停/继续/取消/重试） | `POST /api/tasks/{id}/{action}` | `apps/web/src/features/dashboard/TaskPanel.tsx:82-94` | `apps/web3/src/api/tasks.ts:35-38` 零调用点；`dock/BottomDock.tsx:59` 行只能选中 |
| A18 | 任务错误详情抽屉 | `GET /api/tasks/{id}` | `apps/web/src/features/dashboard/TaskPanel.tsx:95,105` | web3 只有日志流 `dock/BottomDock.tsx:35,89` |
| A19 | 清除 CurseForge Key | `DELETE /api/system/providers/curseforge/key` | `apps/web/src/pages/PackPages.tsx:650,686` | `apps/web3/src/api/system.ts:57` 零调用点；`app/SettingsModal.tsx:42` 只能覆盖 |
| A20 | 系统状态卡（缓存/平台详情） | `GET /api/system/status` | `apps/web/src/pages/PackPages.tsx:636-637` | `apps/web3/src/api/system.ts:33` 零调用点，设置弹窗只显示 health |
| A21 | 语言切换（目录 locale） | `GET /catalog?locale=` | `apps/web/src/features/pack/PackCatalogContext.tsx`（同 web3） | `apps/web3/src/app/CatalogContext.tsx:37` 固定 `zh_cn`，`setLocale` 零消费 |

## B. web3 有 UI 但只是占位/只读（能看不能动）

| # | 位置 | 现象 | 证据 |
|---|---|---|---|
| B1 | `editor/ModeEdit.tsx:18,23,71,74` | 魔改态只有文档列表 + 只读 textarea，注释自认「探针」 | 见 A10 |
| B2 | `editor/ModeQuest.tsx:15,34-38,47` | 编排态只读表格，空态无建章节入口 | 见 A11/A12 |
| B3 | `panels/SourcesPanel.tsx:435,448` | 章节 rail 只读，空态文案「画布里可以建」但画布不存在 | B2 |
| B4 | `panels/BuildPanel.tsx:20-23,71,75,101` | 交付/构建/发布面板只读，无跑检查、无构建、无发布按钮 | A13/A14/A15 |
| B5 | `editor/ModeGraph.tsx:4-5,22,55` | 关系态是双向清单（注释自认 v0，非图形化） | 可用但降级 |
| B6 | `app/CommandPalette.tsx:18-20`、`app/StubItemSearch.tsx:11` | ⌘K 物品检索走本地内存缓存，非接口 | 可用但降级 |
| B7 | `panels/SourcesPanel.tsx:151-158` | 文案「选一个兼容版本」但代码不做兼容过滤，`next_cursor` 丢弃 | 版本列表原样渲染 |

## C. web3 已可用（本轮测试基线，无需改动）

建包 `editor/Welcome.tsx:70` · MC 版本候选 `Welcome.tsx:65` · 链接导入 `Welcome.tsx:110,116` · 包菜单 `app/PackMenu.tsx:19` · 冲突处置 `app/ProblemsPopover.tsx:28,29` · CF Key 填写 `app/SettingsModal.tsx:19` · 来源树/启停 `panels/SourcesPanel.tsx:59,69` · 双平台搜模组 `SourcesPanel.tsx:77` · 列版本+添加钉版 `SourcesPanel.tsx:83,89` · 触发解析+轮询 `SourcesPanel.tsx:330,361,366` · kind 下钻 `SourcesPanel.tsx:331,385` · 索引态目录/状态/重建/图标 `editor/ModeIndex.tsx:93,100`、`api/catalog.ts:19-21` · 过滤器 `editor/FilterBuilder.tsx:24-32` · 网格速览 `editor/ItemGrid.tsx:61,69` · 登记版本 `panels/BuildPanel.tsx:33` · 启动器装/起 `panels/RunPanel.tsx:33,47` · 任务列表与日志 `app/PackSummaryContext.tsx:62`、`dock/BottomDock.tsx:35` · 迎新清单 `app/OnboardingChecklist.tsx:17-20` · 最近动态 `panels/ActivityFooter.tsx:9`

## D. 测试矩阵（接口 → web3 入口 → 结果）

结果列在本轮端到端走查中逐条填写：`PASS` / `FAIL(前端可修)` / `DEFECT(记为待办)`。

| 接口 | web3 入口 | 结果 |
|---|---|---|
| `GET /api/health`、`/api/system/health` | 状态条 / 设置弹窗 | PASS（弹窗回读：CF ✓可达 · Modrinth ✓可达 · 存储可写 · 写令牌 ✓） |
| `GET /api/meta/mc-versions` | Welcome MC 版本下拉 | PASS（选中 1.21.1） |
| `POST /api/packs` | Welcome 新建整合包 | PASS（`pack-40c638744e041f185dede8f8`，建包后自动起 catalog.rebuild） |
| `GET /api/packs`、`GET /api/dashboard` | TopBar 包菜单 | PASS |
| `POST /api/packs/import/inspect`、`POST /api/packs/import` | Welcome 从链接导入 | 未验证（需真实整合包链接，本机无样本）→ 见 E7 |
| `PUT /api/system/providers/curseforge/key` | 设置弹窗 CF Key | PASS（只用 UI 填，未落任何配置文件；填前「未配置/不可达」，填后「已配置/可达」） |
| `GET /api/packs/{p}/mod-search` | 来源面板「+」搜索 | PASS（双平台 13 命中，curseforge 与 modrinth 都有 → CF Key 真实生效） |
| `GET /api/packs/{p}/mod-versions` | 命中后版本列表 | FAIL → **后端缺陷 E1**：modrinth 正常（614 条），curseforge 一律 500 |
| `POST /api/packs/{p}/mods` | 点版本添加（钉版） | PASS（走 modrinth：REI 16.0.799+fabric 钉版成功，来源计数 0→1） |
| `PATCH /api/packs/{p}/mods/{m}` | 模组行 启用/停用 | PASS（停用→启用→停用 三态回读一致） |
| `POST /api/packs/{p}/mods/{m}/content/parse` | 模组行「解析」 | PASS（已解析 73 / 文件 73，无错误计数） |
| `GET .../content/run`、`GET .../content?kind=` | 解析统计 + kind 下钻 | PASS（run 统计渲染；kind 行写 `?ns=&src=&type=` 进下钻） |
| `GET /api/packs/{p}/catalog`、`/catalog/status`、`/catalog/icon` | 索引态 | PASS（物品 1330 · 配方 1290 · r14826；图标 `<img>` 全部 naturalWidth=32） |
| `POST /api/packs/{p}/catalog/rebuild` | 索引态「重建目录」 | PASS（revision 从 r14826 递增、目录计数与空态提示同步更新） |
| `GET /api/tasks?recent=20`、`GET /api/tasks/{id}/log` | 底部停靠 | PASS（任务列表 + 6 行日志「已索引 1330 个物品、1062 个方块、331 个标签和 1290 个配方」）；但**只有看，没有控制** → E5 |
| `GET /api/activities?limit=10` | 侧栏页脚 | PASS |
| `GET/PUT /api/onboarding` | 迎新清单 | PASS（存 Key 后清单「配置 CurseForge Key」自动变 ✓，说明 ack 写入生效） |
| `GET /locks`、`GET /conflicts`、`GET /health` | 顶栏问题徽标 | PASS（空态 `problems-chip clean`，健康 100） |
| `POST /conflicts/{c}/resolve\|ignore` | 顶栏问题徽标弹层「处置/忽略」 | 入口存在（`ProblemsPopover.tsx:52-53`），干净包 0 条冲突，未触发实调用 |
| `POST /api/packs/{p}/resolve` | 无入口 | DEFECT → A6（`resolvePack` 零调用，见 G） |
| `GET /api/packs/{p}/versions\|artifacts\|releases\|delivery-checks` | 构建面板四段 | PASS（版本 2 行 `0.2.0 release`/`0.1.0 draft`；产物/交付检查/发布记录各 0，空态文案渲染；闸门状态「健康，可以构建」） |
| `POST /api/packs/{p}/versions` | 构建面板「登记」 | PASS（UI 输入 0.2.0 → 登记 → 列表即时 1→2，`channel` 落 `release`）。原记「面板纯只读」是误判，按钮在 `BuildPanel.tsx:50` |
| `POST /api/packs/{p}/delivery-checks/run`、`/build`、`/publish/{provider}`、`/api/releases/{id}/poll\|retry` | 无入口 | DEFECT → A14（web1 有：`apps/web/src/pages/PackPages.tsx:468/525`；web3 的 `api/releases.ts` 导出了这五个函数但零调用，见 G） |
| `GET /api/packs/{id}/artifacts/{a}/download` | 产物行「下载」 | 未触发（按钮只在 `status==='ready'` 渲染，而产物需先能构建 → 卡在上一行） |
| `POST /api/packs/import/inspect` | Welcome「解析」 | 接口 200 但语义空：`https://modrinth.com/modpack/fabulously-optimized` → 界面显示 `(未命名) · 0 个条目` → 见 **E7** |
| `POST /api/packs/import` | Welcome「确认导入」 | 202 起任务，任务 **failed**：`pack archive could not be parsed` → 见 **E7** |
| `GET /api/launcher/installs` | 运行面板 游戏目录 | PASS（填目录后回报「该目录还没有本包的安装记录」，dot 变灰） |
| `POST /api/launcher/install`、`POST /api/launcher/launch` | 运行面板 重装/启动 | FAIL → **环境+后端缺陷 E2**：`重装版本` 恒禁用（无安装记录），`启动` 报「mpack-launcher 未安装:设置 MPACK_LAUNCHER_BIN 或把二进制放到 `<workbench>`/.tools/launcher/」 |
| `GET /api/export-dirs`、`/api/fs/browse`、`/api/system/status`、`POST /api/tools/prism/install\|login` | 无入口 | DEFECT → A16/A20（`api/fs.ts`、`api/system.ts` 整个模块零引用，见 G） |

四透镜与工具面另测：关系（graph）PASS（苹果「作为产物 0 / 作为原料 1 → minecraft:golden_apple」）；焦点详情 PASS（多语言名称 / 所属标签 / 来源依据 / 传送门三个跳转）；命令面板 PASS（8 条命令）；复杂搜索 PASS（`#tools` 加条件后 `?fs=1&f=%23tools%3A1`，命中 30）；表格视图 PASS（名称/ID/来源三列）；索引态搜索、排序、网格/表格切换、来源筛选清除均 PASS。

## E. 缺陷记录（前端无法满足、交用户统一处理）

### E1 添加 CurseForge 模组：`GET /api/packs/{p}/mod-versions` 一律 500（阻塞级）

- 现象：双平台搜索能出 CF 命中，但点任意 CF 项目取版本 → `{"error":{"code":"internal_error","message":"internal server error"}}`，HTTP 500，`duration_ms≈390`（说明请求已经打到 CF 并拿到数据）。
- 复现：`curl -H "X-MPack-Token: $TOK" "http://127.0.0.1:18880/api/packs/$P/mod-versions?provider=curseforge&projectId=263420"`（263420=Xaero's Minimap，328085=Create 同样 500）；同 Key 直连 `https://api.curseforge.com/v1/mods/263420/files` 返回 **200**。
- 根因定位（未改，等你定）：`apps/server/internal/provider/http_adapter.go:309` 把 CF 的 `dependencies[].relationType` 声明成 `string`，而 CF 实际回的是**数字**（`{"modId":636608,"relationType":2}`）→ `json.Unmarshal` 整个数组失败 → `decode versions` → 500。
- 为什么前端修不了：响应体在进前端前就没生成；web1/web2/web3 三个前端调的是同一个 URL 同一套参数（`apps/web/src/api/mods.ts:132` 与 web3 逐字相同），所以这不是 web3 的退化，是后端一直坏着——只是 web1 也没能用 CF 加模组。
- 影响：CF 这条路整个不可用，只能用 Modrinth 加模组。

### E2 运行面板无法端到端：缺 `mpack-launcher` 二进制

- 现象：`▶ 启动 Minecraft` 报「mpack-launcher 未安装:设置 MPACK_LAUNCHER_BIN 或把二进制放到 `<workbench>`/.tools/launcher/」。
- 本机事实：这台 Mac 没有任何 `.minecraft` 目录（`~/Library/Application Support/minecraft`、`~/.minecraft` 都不存在），所以 `重装版本` 按钮永远禁用（`RunPanel.tsx` 要求先有安装记录）。
- 附带小毛病：错误文案里的 `<workbench>` 是占位符没展开，用户看不出该往哪儿放。
- 结论：需要你把 launcher 二进制装上（或告诉我该从哪构建），装好后这一条才能验。

### E3 配方「看得见数、点不开面」

- 现象：索引工具条显示「配方 1290」，但 web3 没有任何入口能列出这 1290 条配方；`?mode=index&type=recipe` 被忽略——`ModeIndex.tsx:16` 只在 `type && src` 同时存在时才渲染内容面，`type` 单独存在直接落到 `CatalogTable`。
- 结果：配方只能靠「选中某个物品 → 关系透镜」反查，无法按配方浏览/检索。
- 修法选择权在你：要么索引态加「物品/配方」两个对象域切换，要么把这个数字从工具条撤掉（现在的显示方式是在承诺一个点不到的能力）。

### E4 三个态是只读占位，且空态文案指向不存在的入口

- 魔改：`ModeEdit.tsx` 明写「Code · payload JSON（只读探针，可写编辑器由 3C 落地）」，0 份文档时提示「索引态选一个配方后，「去魔改」会建第一份」——但 E3 里配方根本选不到，这句话是死路。
- 编排：`ModeQuest.tsx` 空态「这个整合包还没有任务书，在任务编辑器里保存一次草稿即可创建」——web3 没有任务编辑器；来源面板 `QuestChapters` 也写「任务书还没有章节。画布（3D 落地）里可以建」，同样没有画布。
- 构建：`BuildPanel` 的「登记版本」是真能用的（实测 0.2.0 登记成功），但产物/交付检查/发布记录三段恒 0 且无按钮，文案自己标了「3F 落地」；交付检查空态写「登记一个版本后，交付闸门会自动跑检查」——登记 0.2.0 之后 `delivery_checks` 仍是 0 行，这句承诺没兑现（跑检查的 `runDeliveryChecks` 前端零调用，见 G）。
- 结论：这三条对应 A10/A11/A12/A13/A14，是能力没迁过来，不是交互坏了；等你排期。

### E5 有接口、无入口的写操作（web3 缺按钮）

完整清单与逐条函数名见 **G**（机械核查，可复现命令在那节）。要点：这些后端接口都在，前端一行没调；补齐是纯前端工作量——我没动，因为要占 UI 位置，落点得你定。本轮实测新确认两条同类：构建/发布全链（A13/A14）与图标补齐（A8）。

### E6 焦点详情把内部 ID 当人话显示

「多语言名称」表的来源列直接显示 `minecraft-pack-eb6251017c9b09daa2bae3ab`（`EditorHeader.tsx` 里 `{n.source}` 原样输出）。前端可修（改成「原版内置」/包名），但显示口径要你定，我先记不改。

### E7 链接导入：后端根本没下载，URL 来源必失败（非 web3 退化）

- 实测（干净实例 `18880`，全走 UI）：Welcome →「从链接导入」→ 选 Modrinth → 填 `https://modrinth.com/modpack/fabulously-optimized` →「解析」HTTP 200，但界面显示 `(未命名) · 0 个条目`；点「确认导入」HTTP 202 起任务，任务落库即 **failed**：
  `t-1790946205345770000-30` 与 `t-1790946314794798000-36` 两条都是 `status=failed, progress=10, error="pack archive could not be parsed"`，`packs` 表仍只有原包（没有新包）。
- 根因（代码可复核）：`apps/server/internal/service/import_service.go:91-96` 对 URL 来源把**URL 字符串本身**当文件内容写进暂存盘（`data = []byte(in.URL)`），`inspectArchive(..., required=false)` 因此直接返回 `(0, "")`——不下载、不解析，所以条目数恒 0、包名恒空；后台任务 `handleImportTask`（同文件 `:224`）再拿这个"假归档"去 `parsePackMetadata` → 必然 `import_parse_failed`。全程没有任何出网下载代码。
- 为什么前端修不了：前端只能提交 URL，下载与装配是服务端职责；把 URL 换成 CDN 直链也一样（校验只认 host，`validateImportURL:264`）。
- 不是 web3 退化：web1 全仓 `grep -w inspectImport|confirmImport` **零命中**——链接导入从来没在 web1 有入口，web3 的 Welcome 表单是从 web2 迁来的新增面。所以这条不是"迁丢了"，是后端能力没实现完。
- 顺带：`importSourceEnum` 含 `local`（`api/imports.ts:6`，后端 `local_zip` 走的是真内容 base64，理论上是唯一能成功的一路），但 Welcome 的下拉只列 `curseforge/modrinth` 两项（`Welcome.tsx:12-15`）。要不要把"上传本地 zip"补进迎新页，口径你定。
- 前端我改了的部分：确认导入后原来**毫无反馈**（`packId` 为 null 时界面原地不动，用户以为按钮没生效）——现在给一句落点提示，见 F.4。

### G. 「有接口、无入口」机械核查（web3 api 层零调用清单）

核查命令（可复现，输出即下表）：

```bash
cd /Volumes/Evo/code/mPackStation/apps/web3/src
for f in api/*.ts; do grep -oE '^export (async )?function [A-Za-z0-9_]+|^export const [A-Za-z0-9_]+ =' "$f" \
  | sed -E 's/^export (async )?function //; s/^export const //; s/ =$//' | grep -v 'Schema$' \
  | while read -r fn; do
      grep -rlw "$fn" . | grep -v "^./$f\$" >/dev/null || echo "$f  $fn"
    done
done | sort
```

2026-10-02 实测输出 38 行（除 `imports.ts importSourceEnum` 是类型枚举外全部下表所列）。

真正缺入口的写操作（点了没处点，需你排期）：

| 能力 | 零调用函数 | 对应 A 项 |
| --- | --- | --- |
| 删包 / 改包 / 单包详情 | `packs.ts deletePack, updatePack, getPack` | A1 |
| 移除模组 | `mods.ts removeMod` | A5 |
| 依赖解析 | `mods.ts resolvePack` | A6 |
| 兼容推荐 | `mods.ts listModRecommendations` | A4 |
| 图标补齐 | `modContent.ts resolveModContentIcons, getModContent` | A8 |
| 任务 暂停/继续/取消/重试 | `tasks.ts pauseTask, resumeTask, cancelTask, retryTask` | A17 |
| 清除 CF Key | `system.ts clearCurseForgeKey` | A19 |
| 导出目录 / 文件浏览 | `fs.ts registerExportDir, browseDirectories`（整个 `api/fs.ts` 无人引用） | A16 |
| 系统状态 | `system.ts fetchStatus` | A20 |
| Prism 便携安装/登录 | `system.ts installPrism, launchPrismLogin` | A18 |
| 魔改 草稿/校验/应用/回滚/历史 | `content.ts createContent, saveContentDraft, validateContent, applyContent, rollbackContent, contentHistory` | A10 |
| 编排 草稿/校验/应用/回滚/历史 | `content.ts saveQuestDraft, validateQuest, applyQuest, rollbackQuest, questHistory` | A11/A12 |
| 交付检查跑批 / 构建 / 发布 / 轮询 / 重试 | `releases.ts runDeliveryChecks, buildPack, publishPack, pollRelease, retryRelease` | A13/A14 |

同一能力的另一条读路径（**没有用户可见损失**，只是 api 层留了重复出口）：`packs.ts listPacks`、`mods.ts listMods, searchMods` —— 包列表与模组树实际走 `dashboard.ts fetchDashboard` 与 `modContent.ts listModContent`。这一类不用补 UI，属清理项。

## F. 本轮已修的前端缺陷（`apps/web3/src/panels/SourcesPanel.tsx`）

1. **版本列表号称「兼容版本」却全量倒灌**：REI 一次给 614 条、无任何过滤，用户要在 614 行里自己找 1.21.1+fabric。现在按本包 `mcVersion` + `loader` 筛，标题写明口径（`选一个兼容版本（MC 1.21.1 · fabric，添加即钉版）`），不匹配的折叠进「其他版本（不匹配本包）608 ▸」仍可强行加装。实测 614 → 6 + 608。
2. **拉版本失败后永远卡在「版本载入中…」**：原来用 `versions.length === 0` 判载入中，E1 那种失败会让界面一直等一个不会来的列表。改成显式 `versionsLoaded` 落定标志（成功/失败都算落定），失败只显示错误行。
3. **搜索/版本错误串到模组树顶部**：`error` 一个 state 同时服务树和下拉，E1 报错后它挂在来源树顶上，看起来像「树坏了」。拆成 `searchError`（只在下拉内显示）+ `error`（树级），并且「没有命中。设置页确认 CurseForge Key 后再试」这句现在只在真无错误时出现——不再把后端 500 诬告成你没填 Key。

验证：`npx tsc --noEmit` 干净；`vite build` 通过；三条都在 `127.0.0.1:5275` ↔ `127.0.0.1:18880` 的干净实例上按 UI 实操复核（现象见 D 表与 E1）。

4. **「确认导入」点了像没反应**（`apps/web3/src/editor/Welcome.tsx`）：后端 `POST /api/packs/import` 只回任务、`packId` 为 null，原代码 `if (done.packId) onImported(...)` 之后什么都不做——界面原地不动、无提示，用户只能重复点。现在补一句落点：`导入任务已提交（t-…，复用此前同一任务时另标），进度见底部「任务」，完成后包自动出现在列表。` 实测：解析 → 确认 → 提示出现并带真实 task id；再次解析同一 URL 会新建 preview 并重新起任务（复用逻辑在后端按 preview 判定）。这条只补反馈，导入本身仍因 E7 的后端未实现而失败。
