# 2026-10-03 会话详录 · Z Code 会话收尾 checkpoint

- 项目：mPackStation（`/Volumes/Evo/code/mPackStation`）
- checkpoint：`cp-2026-10-03-zcode-session`（state.json / HANDOFF.md 同步更新）
- 会话来源：Z Code `sess_0c49f511-f692-49a8-8792-76a4f6e54ddb`，2026-10-02 11:16 → 2026-10-03 16:08，任务标题「查看 Evo 中的 m pack station 项目」
- 上下文完整度：**partial** —— 导出了全部 52 条真实用户消息与全部助手文本（10-03 段逐条读，10-02 段蒸馏后交叉核对），但 590 条工具调用结果未逐条复核；仓库现状一律以本轮实测为准
- Git 现场（**分析时**，收尾时已变，见下「Git 交付」）：分支 `DEV_2610-WK1`，HEAD `fe7c4ea`（2026-10-02 10:54，未前进），工作区 **193 个文件路径未提交**（75 改 + 118 新；`git status --porcelain` 折叠后 126 条），**该分支当时还没有远端对应物**
- 工作区指纹：`sha256:a795433e455031778428ac5df350fe0c30be54861ebbb21a80b1aa01d9b7ef53`

## 本轮（2026-10-03）用户目标

1. 消化另外会话留下的两份方案（图标渲染器补齐 + 逆向链路引擎），按依赖顺序推进；
2. 收口目录构建器修复（物品权威来源从「模型文件」改「语言文件键」）；
3. 修用户 10-02 报的模组搜索痛点（搜 `ae2`/`精致存储` 出不到正主）；
4. 落用户旧需求：包内模组自定义分类；
5. 事故之后的环境定稿：**所有服务全停 → 后端 18872 + 前端 5271 → 写进项目必读约束文件，「这个定下来不要再改了」**。

## 已完成并留下证据

| 事项 | 状态 | 证据 |
|---|---|---|
| 唯一开发服务口径落规 | 已交付、用户当轮明示接受 | `AGENTS.md` 顶部「服务与环境铁律」；`scripts/dev.sh:13-29` 硬绑 18872/5271 + 端口占用检查；`ps` 实测 18872 后端与 web3 vite 5271 在跑（16:22） |
| 图标渲染器 R1+R2+R0 | 已实现，Go 侧测试绿 | `item_model_resources.go`/`mod_content_icons.go` 放行 `item_layers`/`separate_transforms`/`composite`、`#` 变量解引用；`0026_catalog_item_icon_reason.sql`；`go test ./internal/service -count=1` ok（本轮 checkpoint 复跑） |
| 模组搜索别名 + 复合排序 | 已实现，单测绿 | `modsearch_alias.go`+`_test.go`；本轮 `-run "Alias|Search"` 三个测试 PASS；真机三案例首命中为会话内实测（无日志留存） |
| 包内模组自定义分类 | 已验证（自带回归测试已修绿） | `0027_pack_mod_category.sql`、`service/mods.go:818` PATCH、两条读取路径（`mod_repo.go:52`、`pack_scoped_repo.go:77` + `scanPackMod`）带出 `category`；会话内 API 回读 + 浏览器分组验收通过 |
| catalog-v4 清空后复跑闭环 | 已验证（会话内真接口） | 1754 → **1330** 条物品、无名 0、`evidence` 全 `lang`；18 个疑似残渣核对为官方纹饰锻造模板 |

## 缺陷与验证缺口（下轮必修）

1. ~~**`go test ./...` 不为全绿**~~ **已修**：`apps/server/internal/store/category_test.go:23` 夹具 `source='modrinth'` + `project_id=NULL` 违反 `pack_mods` 建表 CHECK（16:22 以 `-count=1` 确定性复现，10 包 ok / 1 包 FAIL）→ `project_id` 改真值 `'proj-jei'`，16:45 复跑 **10 包 ok / 0 FAIL**，随 `6c0aa5d` 入库。
2. **日常库真实数据丢失，成因未定**：`/tmp/mpack-data` 的 `packs`/`pack_mods`/`mod_content` 全 0 行，readd-test 的任务书/目录/魔改记录不可恢复；恢复手段已穷尽（`.recover` 只有 9 月孤儿数据、APFS 快照不覆盖 /tmp、唯一幸存副本是仓库 `data/` 下 09-19 的 schema 16 快照，未回灌）。链路库 `/tmp/mpack-chain` 的清空已判定为 `chain-test-run.sh` **设计内重置**，不算事故。全文 `docs/tests/incident-2026-10-03-shared-env-wipe.md`。
3. ~~README 口径双过期~~ **已修于 `65cf1e5`**：`:22` schema 26 → 27、`:58`/`:70-72` 端口口径改 5271/18872 并列作废端口。注意这条过期文本曾被 `6c0aa5d` 一起入库，README 的端口段现与 `AGENTS.md` 逐字一致。
4. **两套终局脚本自 09-30 后未重跑**：`chain-test-run.sh` run15（173 例）与 `verify-terminal-chain.sh`（43 断言）的绿灯绑定的是 schema 24；当前 schema 27 且目录/搜索/分类三处行为都变了，旧结论对现状只算历史证据。
5. **图标覆盖缺口收窄但未清零**：R4 OBJ(+22)、R5 tint(+15)、R6 化学桶(+18) 未做；R3 `builtin/entity` 按计划「不做，只说清」。

## 未实现的目标（本轮明确挂账）

- **逆向链路引擎 P0**：`docs/design/craft-chain-engine-plan-2026-10-03.md` §1 规格齐全（`apps/web3/src/chain/` 下 `interpret.ts`/`ticks.ts`/`expand.ts`/`chainRules.json`/`ChainView.tsx` + `url.ts`、`EditorArea.tsx` 挂载，≈350 行，后端零改动）。实测 `grep -rli chain apps/web3/src` 零命中 —— **代码零落地**，会话在 14:55 宣布进入该项后被打断。
- 会话 todo 的最后两项：「链路引擎 P0（5 文件 + 3 挂载）」pending、「全量验收 + 文档 + 汇报」pending。
- 历史遗留（非本轮）：`.mrpack` overrides 落盘、O5 删包 9.6s、内核专属 Rust 单测、B5 拆 `service.API`。

## 关键决策（本轮新增/变更）

- `decision-dev-service-singleton`（active）：唯一后端 18872 + `-data /tmp/mpack-data`、唯一前端 5271；5173/5273/5274/5275/5276/18871/18880 作废；`dev.sh`/`dev-stop.sh` 为唯一起停方式；动共享数据目录须用户当轮明示；迁移编号先 `ls migrations/` 再取号。
- 取代 `decision-6dc4d5e6`（端口 5273/18871）→ 标记 `superseded`。

## state.json 变更摘要

- 新增任务 5：`task-catalog-v4-20261002`（补登记，此前 HANDOFF 已成文但 state.json 停在 09-30）、`task-icon-renderer-r0r2-20261003`、`task-mod-search-alias-20261003`、`task-pack-mod-category-20261003`、`task-dev-service-lockdown-20261003`、`task-chain-engine-p0-20261003`（planned）
- 新增问题 3：`issue-category-testfixture-check`、`issue-daily-db-wipe-cause-unknown`、`issue-readme-schema-port-stale`；`issue-item-icon-coverage-gap` 补本轮部分推进证据（状态仍 open）
- 新增决策 1、取代 1；`repository` 同步 branch/head/脏区；`checkpoint` 换为本轮
- 第二遍（`6c0aa5d`/`65cf1e5` 之后）：`repository.head` = `65cf1e5`，五个任务的 `delivery` 转 `remote_verified`（附 `pushed_at`），`issue-readme-schema-port-stale` → `resolved`，新增 `issue-chain-test-port-collision`（high），`next_session` 首位换成撞口安全闸
- 归一 3 条历史 evidence 的 `level: verified` → `observed`（`verified` 不在 state-model 的证据等级枚举里，内容未改，逐条附 `note` 说明）
- `checkpoint.py validate` 通过（`valid: true`）

## 排除为 session-only（不入项目状态）

- checkpoint 由 Qoder 侧执行而非 Z Code 侧：Z Code 会话 15:43 被要求跑该 skill，只读到 SKILL.md 与跑完 `inspect` 就中断（16:08），未落任何状态文件 —— 属会话执行链问题，不改变项目事实。
- 在仓库根跑 `go test ./...` 报「does not contain main module」：属操作方式，已作为「不要重复尝试」条目落 HANDOFF，不作为项目缺陷。

## Git 交付（checkpoint 会话执行，用户当轮指示「重新做一次提交，并记录提交的哈希」）

| 提交 | 内容 | 远端 |
|---|---|---|
| `6c0aa5de0608b09e91c91aed7a6667839c842f5c` | 10-02/10-03 两轮改造积压：**190 路径 = 117 新增 + 73 修改，19742 插入 / 940 删除**（`apps/web3/` 整目录、迁移 0021-0027、`launcherCore`、catalog-v4 判据、图标 R0-R2、搜索别名排序、包内分类，含提交前修好的红夹具） | 已推送 |
| `65cf1e589891d366897fba6579c57e857ff2b758` | README 口径同步（schema 27 + 5271/18872 + 撞口警告） | 已推送 |
| 本次提交（`git log -1 --format=%H -- docs/project-state`） | checkpoint 三件套：`state.json` + `HANDOFF.md` + 本文件 | 随后推送 |

- 分支 `DEV_2610-WK1` **此前从未推送**：`git ls-remote` 实测远端无该分支、本地无 `origin/DEV_2610-WK1` 引用。首推用 `git push --set-upstream origin DEV_2610-WK1`，结果是 `[new branch]`，远端 tip 实测 `65cf1e5`。
- 提交前实测（`-count=1`，16:45）：`go test ./...` = **10 包 ok / 0 FAIL**、`go vet ./...` 干净。
- 暂存方式：从 `git status --porcelain -uall` 生成清单后 `xargs -0 git add --`，**未用 `git add .`/`-A`**；排除 `docs/.DS_Store`（本机 Finder 产物，且 `.gitignore` 没有忽略它，会一直脏）。

## 提交之后新查出的两件事（已回写状态文件）

1. **README 口径确实过期**：`6c0aa5d` 把过期文本一起入库了（`:22` schema 26 而 `store.go:41` = 27；`:70-72` 仍写 5273/18871）→ 单独提交 `65cf1e5` 修正，并把作废端口清单与 `AGENTS.md` 逐字对齐（我一度多写了 18873/18874，那是验证脚本的合法端口）。
2. **`issue-chain-test-port-collision`（high）**：`scripts/chain-test-run.sh:8` 默认端口就是唯一开发后端的 **18872**；`:29-30` 的 `pkill` 只匹配自己的 `mpack-chain-server` 二进制，杀不掉 `dev.sh` 起的 `go run`；`:20-26` 的 `wait_up` 只看 `/api/health` 是否 200，而开发后端会答这个 200 → 不停开发实例就跑它，173 个用例（含建包/删包）会打在 `/tmp/mpack-data` 上。**机制由代码阅读确认（observed），后果未实跑验证（inferred）——实跑一次就可能污染唯一开发库。** 已写进 README 警告、HANDOFF「下一步」4 与「不要重复尝试」。

## 下一会话首个动作

**先做撞口修复（「下一步」4）再考虑跑任何验证脚本**：跑 `scripts/chain-test-run.sh` 之前必须先 `scripts/dev-stop.sh`。修完把两套终局脚本（`chain-test-run.sh` 173 例、`verify-terminal-chain.sh --launch` 43 断言）在 schema 27 上重跑一遍，替掉 09-30 的旧结论；随后开链路引擎 P0。全程只用 18872/5271 与 `/tmp/mpack-data`，新开实例或重建该目录前必须先问用户。
