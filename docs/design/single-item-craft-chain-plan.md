# 单物品完整链路：从零做出一个「反物质」——方案（2026-10-02）

需求原话：「通过解析通用机械这个模组的公式，拿到制作出一个反物质这个物品的完整合成链路。」

即：**给定一个目标物品，递归展开它所需的全部前置配方，算出每一层要几个、用什么机器做、哪一环根本做不出来**，并且这条链要能在界面上看、能改、能落到任务书里引导玩家。

---

## 0. 结论先说

今天这条路有 **4 个断点，从前往后依次是**：

| # | 断点 | 层级 | 证据 |
|---|---|---|---|
| B1 | **通用机械本体装不进包**：Modrinth 上没有 1.21.1·neoforge 的 Mekanism，CurseForge 版本接口一律 500 | 后端（已记 E1） | 本轮实测：新建包 `pack-06afb18714826bf9c213416d`（链测-Mekanism，1.21.1 · neoforge）→ 搜 mekanism → 9 条 CF 命中全部点不开版本，界面直出「双平台搜索搜 internal server error」；Modrinth 侧只有 `Mekanism Community Edition CORE`（36 个版本全是 1.7.10/1.12.2）与 `Mekanism Extra`，兼容过滤器正确显示「0 兼容 / 36 其他版本」 |
| B2 | **就算装上，Mekanism 的机器配方一条都展不开**：解析器只结构化 5 组配方类型，其余全标 `unsupported` 且不产生任何边 | 后端 | `apps/server/internal/service/item_catalog.go:298-329` 的 `switch recipe.Type` 白名单只有 `minecraft:crafting_shaped / crafting_shapeless / ae2:transform / smelting / blasting / smoking / campfire_cooking / stonecutting / smithing_transform / smithing_trim / crafting_transmute`；`default:` → `recipe.Status="unsupported"` + 「保留了原始定义，但尚未结构化此配方类型」，**refs 为空**。`mekanism:metallurgic_infusing`、`mekanism:chemical_infusing`、`mekanism:electrolysis`、`mekanism:activation` 等全落 default。另外 `:330` 输出只读 `root["result"]`，而 Mekanism/NeoForge 系普遍写 `output` → 即使补了类型分支也取不到产物 |
| B3 | **前端没有"链"这个视图**：关系态只算一层「作为产物/作为原料」，不递归、不算数量 | 前端（本轮可修） | `apps/web3/src/editor/ModeGraph.tsx:7,22-23` 直接读 `useCatalog().recipesByOutput/Input`，`:34-64` 渲染成 flex 双栏清单（`:5` 注释自认 v0 非图形化）；无深度、无累乘、无叶子分类 |
| B4 | **自己补的配方进不了目录，也进不了游戏**：`apply` 只写数据库；目录重建只读 jar 解析结果 | 后端（缺失能力） | `content_repo.go:216-247`（apply 全文 5 条 SQL，无一处 `os.*`）；`outbox_events` 全仓只有 INSERT、无消费者；`catalog_repo.go` grep `content_documents` = 0 匹配 → 自定义配方不进目录；`build_mrpack.go:119-120` 原文「overrides/ 暂不装配」→ 不进产物；`content.go:293` 的 kind 枚举只有 `recipe|structure|ore`，**没有"新建物品本体"这一类**（物品只能来自已解析的模组 jar，`0008_mod_content.sql:52-53`、`item_catalog.go:720`） |

**可行的最大子集**：B3 是纯前端 + 现有目录数据就能交付的——原版与已解析模组之间，链是**已经存在的图**（现役包实测 `10706` 条 input 边、`2516` 条 output 边、2580 条配方行，其中 parsed 2552 / unsupported 28）。把"一层关系"升级成"递归整链 + 数量累乘 + 断点标注"，就是本方案的 L0。

反物质这条链今天**必然停在 B2**：Mekanism 装不上（B1），装上也解不出电解/化学注入（B2）。所以 L0 的界面与算法要**按"B2 修好后即可复用"来设计**，并把展不开的那一环显式显示成断点，而不是假装没有。

---

## 1. 目标 / 非目标

**目标**
1. 任一物品 → 一棵可展开的合成树（含数量、机器、深度），一眼看出"从零到它总共要备哪些原料"。
2. 断点可见：哪一环没有配方、哪一环的配方类型解析器不认识、哪一环需要别的模组/世界采集。
3. 一条链能预填成任务书章节/节点（复用既有口径，不新造模型）。

**非目标（本轮明确不做）**
- 不做"凭空造一个原版与模组都不存在的新物品"——后端模型里没有落点（B4），要单独立项。
- 不改后端（用户规则）。B1/B2/B4 只出**可执行的后端工单**，不动手。
- 不做图形化无限画布（那是 3D 批次的既有范围，`docs/design/web2-step3-domain-widgets.md:122`）。

---

## 2. 什么叫"完整链路"（形式定义，验收就按这个判）

给定目标物品 `T` 与数量 `n`（默认 1）：

```
Chain(T, n):
  候选 = recipesProducing(T)                     # refs.role=output 且 id==T
  若无候选 → 节点类型 = LEAF(T)
      子判据：
        inCatalog(T)   → LEAF_WORLD（原版/世界可得，或需采集）
        inTagsOnly     → LEAF_TAG（只被标签提及，本体未证实存在）
        否则            → LEAF_UNKNOWN（目录里没有这个物品 → 上游配方/模组缺失）
  若候选 > 1 → 分支（MULTI），默认取「原料层数最浅」的一条，可手切
  对每条候选配方 R：
      机器 = R.type（minecraft:crafting_shaped → 工作台；minecraft:smelting → 熔炉；
                     mekanism:* → 对应机器；未识别 → UNKNOWN_MACHINE 并标断点）
      产出率 out(R) = Σ refs.output(R) 中 id==T 的 count
      需求系数 k = ceil(n / out(R))            # 做 n 个 T 要跑 k 次 R
      对 R 的每个 input 边 e（按 slot 归并 alternative 组，同一 slot 的 alternative 视为「或」）：
          子节点 = Chain(e.id, e.count * k)     # 数量在这里累乘
  环：路径上出现已在栈内的 id → CYCLE，就地截断并标注（不无限递归）
```

**这条定义里最容易被忽略、必须实现的 5 件事**（都对应真实数据形态）：

| 项 | 为什么必须 | 数据依据 |
|---|---|---|
| 数量累乘 `count * k` | 1 个信标要 5 个玻璃 × 1，但 1 个黄金苹果要 8 个金锭 → 每层要 `8*1`，金锭又 ×9 个金粒 = 72 金粒 | `refs.count` 已由 `item_catalog.go:267-271` 填 |
| 同 slot 的 `alternative` 归并 | 配方里"任一木板都行"是多条 refs、同 slot、`alternative=0..n`，不归并会把树撑成假分支 | `catalogIngredient` 对数组 alternatives 逐条 `Alternative: index`（`item_catalog.go:238-248`），前端 schema 已暴露 `alternative`（`api/catalog.ts:9`） |
| `item_tag` 输入要落地成候选物品集 | 标签输入（如 `#c:ingots`）不是具体物品；不展开就只能显示"某标签"，用户没法备料 | refs.kind = `item|item_tag`；标签成员在 `catalog.tags[].members` |
| 产出率 >1 的配方要除 | 1 次烧炼可能出 1 个，但 `crafting_shapeless` 常见出 4 个（如 1 铁锭→? / 1 南瓜→4 南瓜片），不除会算少 | `refs` 里 role=output 的 count |
| 断点分级，不是一句"做不了" | B2 那种"配方存在但类型不认识"（`status=unsupported`、refs 只剩 output 或全空）和"根本没有配方"是完全不同的下一步动作 | `pack_catalog_recipes.parse_status IN ('parsed','partial','unsupported','invalid')`，`diagnostics` 已带中文原因 |

---

## 3. 分层方案

### L0 · 合成链视图（纯前端，本轮就能做完并验收）

**做什么**：新增一个"链路"透镜态，输入=当前焦点物品，输出=递归展开的合成树 + 原料总账 + 断点表。数据源就用现有 `useCatalog()` 的本地全量目录（无新接口、无后端改动）。

**改动点（机械可核查，全部是行级定位）**

| 改动 | 文件:行 | 内容 |
|---|---|---|
| 1 | `apps/web3/src/app/url.ts:7-12` | `MODES` 加一项 `{mode:'chain', label:'链路', hotkey:'C'}` |
| 2 | `apps/web3/src/editor/EditorArea.tsx:20-24` | 加 `{mode==='chain' && <ModeChain/>}` |
| 3 | 新文件 `apps/web3/src/editor/ModeChain.tsx` | 视图本体（下段） |
| 4 | 新文件 `apps/web3/src/app/craftChain.ts` | 纯函数求解器（可单测，不碰 React） |
| 5 | `apps/web3/src/editor/editor.css` | 复用 `.qv`(`:431`)、`.ed-table`(`:173`)、`.p-row/.p-btn/.p-empty`(`app/frame.css:226,280,313`)，只加缩进与断点色 2 个类 |

**求解器契约**（`craftChain.ts`，纯函数，输入输出都可断言）：

```ts
export type ChainNode = {
  itemId: string; need: number;                 // 这一层要几个
  depth: number;
  kind: 'recipe' | 'multi' | 'leaf_world' | 'leaf_tag' | 'leaf_unknown' | 'cycle';
  machine?: string;                             // 配方 type 原文，界面负责转人话
  recipeId?: string;
  alternatives?: string[][];                    // 同 slot 的「或」组
  out: ChainNode[];
  breakReason?: string;                         // 例：「配方类型 mekanism:electrolysis 未结构化」
};
export function buildChain(target: string, need: number, cat: ItemCatalog, opts?: {maxDepth?: number; maxNodes?: number}): ChainResult;
export function summarize(result: ChainResult): {rawMaterials: {itemId: string; count: number}[]; machines: {machine: string; runs: number}[]; breaks: ChainNode[]};
```

- `maxDepth` 默认 12、`maxNodes` 默认 2000，超了**截断并显式提示**（不做静默丢弃；这条来自"超限要收敛不要报错"的既有约定）。
- 必须**带 memo 的自环检测**（栈内 id 判重），原版有 `furnace↔? ` 类环，模组链里"循环产物"更常见。
- 必须可单测：`npx vitest run src/app/craftChain.test.ts`，用例至少覆盖 ①黄金苹果 72 金粒 ②同 slot alternative 归并 ③产出率 4 的除法 ④`#c:ingots` 标签展开 ⑤环截断 ⑥`unsupported` 配方出断点而非空树。

**界面（`ModeChain.tsx`）**：三段，从上到下

1. **头部**：目标物品（图标 + 名字 + 数量输入框，默认 1）+ 深度上限 + 「重算」。焦点走现有契约：物品焦点 `?item=`、非物品 `?f=&fid=`（`app/url.ts:44-46,86-87`），从索引态/关系态右键「看整条链」跳进来。
2. **合成树**：缩进树（不是画布），每行 = `图标 名称 ×数量 · 机器 · [断点标记]`；同 slot 的多选一行显示「任一：A / B / C」；点击任意节点 → 焦点切到它（复用 `useFocus`）；`unsupported` 节点标橙、`leaf_*` 标灰、`cycle` 标红，鼠标悬停显示 `diagnostics` 原文。
3. **原料总账**：把所有叶子按物品合并求和（"最终要备：粗金 ×36、玻璃 ×5、下界之星 ×1…"）+ 机器清单（"电解机 ×2 次、冶金注入机 ×14 次"）。这两块是**用户真正拿去备料的东西**，也是导出给别人的抓手（复制为文本）。

**为什么先做缩进树而不是 DAG/画布**：链的诉求先是"读得通、算得对"，缩进树零依赖、可键盘导航、和现有 `.qv`/表格样式同源；同一物品在多处出现是正常的（先不合并，总账里才合并），DAG 化留到 3D 画布批次一起做，不跟既有排期撞。

**验收（全在 5275 UI 上点，一对一绑 18880）**

1. 索引态点「金苹果/黄金苹果」→ 透镜切「链路」→ 树里出现 `金粒 ×72`（经 8 金锭 ×9），深度 2，机器=工作台。
2. 点一个 `#c:ingots` 类输入 → 展开为成员物品清单（不是原始标签串）。
3. 造一个环（或取现网带环的模组）→ 显示 `cycle` 且总账不炸。
4. 焦点物品无任何配方 → 显示"这一环没有已解析的配方"，并列出它**在别的配方里作为产物但类型未结构化**的候选，附 `parse_status=unsupported` 的原因原文。
5. 原料总账复制出的文本行数 = 叶子物品数；数量与手算一致（用金苹果校验）。
6. `npx tsc --noEmit` + `vite build` 绿。

---

### L1 · 让"断点"能一键变成"下一步动作"（前端，依赖既有接口）

- 断点节点上给「为它新建配方」→ 走既有 `POST /api/packs/{p}/content`（kind=recipe，`routes_content.go:18-29`）预填 `output`，这条正是设计文档里已有的设想（`docs/design/workbench-v2-implementation-plan.md:960`）。
- 前提是把魔改态从只读探针改成可写（`ModeEdit.tsx:74` 的 `readOnly` textarea + 工具栏挂 `api/content.ts:68-76` 已就绪的 `saveContentDraft/validateContent/applyContent/rollbackContent`），即 3C 批次的最小版。
- 注意此时**新配方只在数据库里**：目录看不见它（B4），所以链视图必须显式把"自建配方"当作另一条数据源读（`listContent` + `getContent` 的 `payload`），并打上"未装配"的水印，等后端通道打通再切到目录。这是 L1 里唯一需要新代码逻辑的地方。

### L2 · 后端解锁（只出工单，不改）

按依赖顺序，每条给判据与验收：

1. **E1**：`provider/http_adapter.go:309` 的 `Relation string json:"relationType"` 改成数字（CF 实回 `{"relationType":2}`）。验收：`GET /api/packs/{p}/mod-versions?provider=curseforge&projectId=191315`（Mekanism）200 且 CF 版本可见 → **B1 解除，通用机械能装**。
2. **解析器配方类型扩展**（`item_catalog.go:298-334`）：
   - 输出侧同时认 `result` 与 `output`；
   - 把 NeoForge/Forge 常见容器字段（`ingredients`/`ingredient`/`inputs`/`output`/`outputs`）做**类型无关的通用抽取**，遇到不认识的结构至少产出 `partial` 而不是 `unsupported` 且零边；
   - 非 item 输入（Mekanism 的 `chemical`/气体、流体、能量）要有新 kind（如 `chemical`），否则化学类链条在图上会凭空少一层。
   验收：装 Mekanism 后 `pack_catalog_recipes` 里 `type LIKE 'mekanism:%'` 的行 `refs` 非空、`parse_status` 不再是 unsupported；反物质的直接前置在链视图里能看到。**B2 解除**。
3. **自定义内容进目录/进产物**：`catalog` 读取 `content_documents` 的 applied 修订（现在 0 引用）；`build_mrpack` 装配 overrides（`build_mrpack.go:119-120` 的"暂不装配"）。**B4 解除**。
4. **validate 加引用校验**（`content.go:374-398` 现在只有 4 条规则、从不读 `input/output/block`，假 id 也能 passed 并 apply）：引用不存在物品时报 error，`AffectedMods` 不再是硬编码 `"[]"`。
5.（立项级）**新物品本体**：kind 加 `item`/`block` 或走 addon jar —— 现有模型没有这一类，属新功能不属于修 bug。

### L3 · 链 → 任务书（复用既有口径，不新造模型）

既有口径直接引用，不另发明：「任务节点的目标与奖励**就是物品**，前置**就是配方链**」（`docs/design/workbench-interaction-design.md:63`），预填判据在 `:357-373`（链路 E）；图的权威是 `draft.edges`，`A→B` 读作"B 依赖 A"，`prerequisites` 只是派生只读（`docs/design/quest-book-ftb-experience.md:63-68`、`docs/design/mp-infinite-canvas.md:199-207`）。

做法：链视图加「导出为任务章节」→ 按 L0 的树生成 `chapters[]/nodes[]/edges[]`（每环一个节点、`edges` 按依赖方向、节点 `rewards` 填该环产物），走 `saveQuestDraft → validateQuest → applyQuest`。后端已备 422 `quest_cycle`/`quest_orphan_node`/`quest_invalid_reference`（`docs/api/contract.md:858-906`），正好当 L0 环检测的第二道闸。

---

## 4. 排期与判据

| 期 | 内容 | 依赖 | 可交付判据 |
|---|---|---|---|
| L0 | 链路透镜 + 求解器 + 总账（纯前端） | 无 | 上面验收 6 条全绿，用现役包 1290 条已解析配方的原版链跑通 |
| L1 | 魔改写入口 + 自建配方进链视图 | L0 | 能建一条自建配方并在链里显示（带"未装配"水印） |
| L2 | 后端 5 张工单 | 你排期 | 反物质那条链在链视图里从叶子到目标**无 unsupported 断点** |
| L3 | 链 → 任务书预填 | L0/L2 | 一键生成章节 + validate 无 422 |

**关于「反物质」本身留一个必须你来定的口径**：反物质的确切物品 id 与它属于通用机械的哪个模块（本体/Additions/Extreme Reactors），今天装不上包所以无法从目录里核对；L2 工单第 1 条通了之后，第一步就是在链视图里按 id 搜出来确认，而不是照记忆猜。

---

## 5. 风险

1. **树爆炸**：深度 12、节点 2000 只是护栏，真实模组链（含矿物字典 `#c:ingots` 上百成员）展开后总账可能对上百物品求和。对策：标签输入默认**只列成员不逐个递归**，用户点开哪个再展开哪个。
2. **多解选择是产品决策不是算法决策**：同一物品常有 3 条配方（熔炼/ Blast / 手工），"取最浅"只是默认值，必须让用户能切换并影响总账。
3. **数量语义不统一**：Forge 系部分配方 count 缺省、部分按 `count` 有的写 `quantity`（如 1.21 原版 `result:{id,count}` 与新写法并存），L2 第 2 条要一次性定口径，否则累乘会静默错。
4. **B2 修完前的链视图会被误读成"链断了是前端 bug"**：断点必须把原因原文（`diagnostics`）显示出来，这句话是用户唯一能看懂的求助信息。
