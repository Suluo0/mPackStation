# 逆向链路引擎（Craft Chain Engine）实现方案 — 2026-10-03

一句话：**入参 = 一个终极物品 id + 数量，出参 = 递归到原版采集的完整链路**，每个节点带确切数量，每条边带「机器 · 时长 · 时长出处」，取不到的地方显式标断点。

- 落点：**纯前端**（`apps/web3/src/chain/`），后端一行不改（用户口径：后端已有能力，优先改前端；改不了才记录缺陷）。
- 依据：本仓库 `docs/active/design/antimatter-chain-2026-10-02.md` §9/§10/§11（数字与命名以那里为准）+ 2026-10-03 四 jar 通用性实测。
- 现有数据基础（全部实测，不是假设）：
  - `GET /api/packs/{p}/catalog?locale=zh_cn` 返回 `recipes[] = {id,type,status,payload,refs[]}`，**payload 3505/3505 全量透出** → 前端可自己重算边。
  - 现役包 `pack-06afb18714826bf9c213416d`：`parsed 1516 / unsupported 1989`；已结构化边 input/item 5611、input/item_tag 1215、output/item 1583。
  - 名称：`1711` 个目录物品里 `1680` 个有简中名 = **98%**，`itemById.get(id).displayName` 直接可用。
  - 通用机械的 402 条机器配方**零边**（`mekanism:enriching/oxidizing/dissolution/chemical_infusing/centrifuging/crystallizing` 全落 unsupported）→ 这是本引擎要自己解决的核心缺口，**不改后端解析器**。

---

## 1. 模块划分（5 个新文件 + 3 处挂载）

```
apps/web3/src/chain/
  interpret.ts     配方 payload → 边（原料/产物/非物品输入），键名并集 + 可追加词表
  ticks.ts         机器时长：L1 配方字段 → L2/L3 规则表 → unknown，带出处与置信度
  expand.ts        逆向递归引擎（本方案的"方法"本体）
  chainRules.json  数据表：type→产物键/节拍/速率/采集时长，每行带 source 出处
  ChainView.tsx    维度一渲染：节点=数量，箭头=机器·时长，断点成列
```

挂载（改 3 个文件，各 1 行）：

| 文件 | 锚点 | 改动 |
|---|---|---|
| `apps/web3/src/app/url.ts` | `:7-12` `export const MODES = [` | 在 `{mode:'graph', label:'关系', hotkey:'Enter'},` 之后插一行 `{mode:'chain', label:'链路', hotkey:'C'},` |
| `apps/web3/src/editor/EditorArea.tsx` | `:21` `{mode === 'graph' && <ModeGraph/>}` | 下一行加 `{mode === 'chain' && <ModeChain/>}`，并在文件头 `:4` 后加 `import {ModeChain} from './ModeChain';` |
| `apps/web3/src/editor/ModeChain.tsx` | 新文件 | 薄壳：读 `useFocus()`/`useUrlState()`，调 `expandChain()`，把结果交给 `ChainView` |

`EditorHeader.tsx:37` 的透镜导航是 `MODES.map()`，加了数组项自动出现，无需改该文件。

---

## 2. interpret.ts —— 把 payload 变成边

规则（2026-10-03 实测标定）：**按键名并集分类，不按 type 白名单**。并集缺一个键就掉一批配方：Mekanism 不加 `main_output` 时只有 76%，加上后 89%（2126/2215）。Thermal 98%、IF 97% 用原版风格键即中。

```ts
export type Ref = {kind:'item'|'item_tag'; id:string; qty:number; unit:'item'|'mB'};
export type Interpreted = {
  inputs:Ref[]; outputs:Ref[];
  nonItem:{energy?:number; mana?:number; heat?:number; fluid?:Ref[]};  // §4 建模要求
  ticks?:{value:number; source:'recipe-field'; field:string};
  ok:boolean; misses:string[];                                         // ok=false → 断点
};

const IN_KEYS   = ['input','ingredient','ingredients','item_input','item_inputs','chemical_input',
                   'fluid_input','inputs','top_input','bottom_input','left_input','right_input',
                   'stack','catalyst','key'];                    // key = crafting_shaped
const OUT_KEYS  = ['output','outputs','result','results','item_output','item_outputs',
                   'chemical_output','fluid_output','main_output','secondary_output'];
const TICK_KEYS = ['duration','cookingtime','time','ticks'];
```

取值约定（必须照抄，否则数量会错）：物品 `count`（缺省 1）；化学品/流体 `amount`（**单位 mB，不许 ceil 成整数**）；`tag` 与 `item` 互斥出现 → `kind:'item_tag'`，展开时取 `tagById.get(id).members[0]`，无成员则记断点 `tag-unresolvable`。

`misses` 判定：`inputs.length===0 || outputs.length===0`。Mekanism 里注定进 `misses` 的是 89 条无物品边类型（`energy_conversion`/`mek_data`/`bin_insert`/`bin_extract`）+ 158 条 `sawing` 特例（`secondary_output` 为空对象）。

---

## 3. ticks.ts —— 时长四层降级

| 层 | 取法 | 实测可用度 | 本期是否实现 |
|---|---|---|---|
| L1 | 配方 JSON 的 `duration`/`cookingtime` | Mekanism 3.2%、Thermal 0%、IF 0.3% | ✅ 本期做（前端读 payload 即可） |
| L2 | 机器类 `BASE_TICKS_REQUIRED` 常量 | 只有 Mekanism 有（全 jar 22 个常量） | ⏸ 需后端解析 class → 工单 BK-1 |
| L3 | 沿继承链上溯 | 已验证链能建（2975 类全解出），但 `TileEntityChemicalInfuser` 链上 6 层无任何 tick 常量 → **静态不可判定** | ⛔ 到此为死线，不许再猜 |
| L4 | 规则表（人工/实机/config 源码） | 覆盖关键速率型机器 | ✅ 本期做，落成 `chainRules.json` |

```ts
export type Tick = {
  mode:'per-cycle'|'rate'|'unknown';
  ticksPerCycle?:number;              // L1/L4
  rate?:{inQty:number; inUnit:'mB'|'item'; outQty:number; outUnit:'mB'|'item'; perTicks:number};
  parallel?:number;                   // 2 ** 速度升级数，默认 1
  source:string;                      // 必填：'recipe.duration' | 'javap BASE_TICKS_REQUIRED' | 'GeneralConfig.java:140' | 'JEI 实测 2026-10-03'
  confidence:'measured'|'derived'|'assumed';
};
```

耗时公式（两种模型，写死在 `expand.ts`，不允许第三种）：
- 节拍型：`cycles = ceil(need / outQty)`；`ticks = cycles * ticksPerCycle / (parallel ?? 1)`；`现实秒 = ticks / 20`。
- 速率型：`ticks = 总量 / rate`（例：SNA `≤64 mB/t` ⇒ 1e7 mB ÷ 64 = 156,250 t = 2.2 h）。
- 能量驱动（SPS）：不产 ticks，产出「需 1 TJ ÷ 进能速率」，显示为待输入功率而不是时长。

---

## 4. expand.ts —— 逆向递归本体

```ts
export type ChainNode = {
  id:string; need:number; unit:'item'|'mB'; depth:number;
  via?:{recipeId:string; type:string; machine:string; tick:Tick; cycles:number};
  leaf?:'gather'|'no-recipe'|'cycle'|'unresolved'|'cap';
  children:ChainNode[];
};
export type Chain = {root:ChainNode; nodes:number; broken:ChainNode[]; totals:{energy?:number; items:Map<string,number>}};

export function expandChain(targetId:string, qty=1,
  opts:{maxNodes?:number; maxDepth?:number; speedUpgrades?:number} = {}): Chain
```

算法（BFS + 路径栈，参数默认 `maxNodes=600 / maxDepth=24 / speedUpgrades=0`）：

1. 出边来源三级降级：`recipesByOutput`（后端已结构化，`status==='parsed'`）→ 没有则 `interpret(payload)`（前端自己算）→ 再没有则 `chainRules.json` 的 `producedBy` 表。
2. 多配方选一（**默认排序键，不询问用户**）：`status==='parsed'` 优先 → 产出量 `outQty` 大者优先 → 输入里 `item_tag` 少者优先 → `recipeId` 字典序（保证可复现）。UI 上保留「换一条配方」按钮，点了在同层重选。
3. 数量传递：`childNeed = ceil(need / outQty * inQty)`（item 单位）；mB 单位不 ceil。`totals.items` 只对 `leaf==='gather'` 的节点累加，避免中间物被重复计入。
4. 环检测：递归栈里出现同 id → 该节点 `leaf='cycle'`，停止下钻（反物质链里 `antimatter ↔ pellet` 就是互逆闭环，§8 错2 的成因，必须有这一条）。
5. 无配方兜底：`leaf='gather'`，从 `chainRules.json.gather` 取采集时长（原版公式 `刻数 = ceil(destroyTime × 系数 × 20 / 工具速度)`，系数：正确工具 1、徒手 1.5、错工具 5 且不掉落）。
6. 非物品输入（**用户 2026-10-03 定的硬要求**）：`interpret` 拿到的 `energy/mana/heat` 不进 `children`，进 `totals.energy` 并在边上标注；反物质链的 1 TJ、裂变堆的 10 TJ  heat 就是这么显示的。理由已实测：Mekanism 378/2215 条配方（17%）带 `per_tick_usage`/`secondary_probability` 这类字段，89 条完全无物品边，纯物品图会丢掉真正的瓶颈变量。

---

## 5. chainRules.json —— 语义层是数据，不是代码

```json
{
  "byType": {
    "mekanism:enriching":         {"tick":200, "source":"javap TileEntity*.BASE_TICKS_REQUIRED", "confidence":"derived"},
    "mekanism:oxidizing":         {"tick":100, "source":"javap TileEntityChemicalOxidizer",     "confidence":"derived"},
    "mekanism:dissolution":       {"tick":100, "source":"javap TileEntityChemicalDissolutionChamber","confidence":"derived"},
    "mekanism:crystallizing":     {"tick":200, "source":"javap TileEntityChemicalCrystallizer",  "confidence":"derived"},
    "mekanism:chemical_infuser":  {"tick":null,"source":"继承链 6 层无常量，静态不可判定",        "confidence":"assumed"},
    "mekanism:centrifuging":      {"tick":null,"source":"同上",                                   "confidence":"assumed"}
  },
  "byMachineRate": {
    "mekanism:solar_neutron_activator":{"rate":{"inQty":10,"inUnit":"mB","outQty":1,"outUnit":"mB","perTicks":1},"cap":"64 mB/t","source":"GeneralConfig.java:140","confidence":"derived"},
    "mekanism:sps_port":               {"energy":{"joulesPerMg":1000000,"inputPerMg":1000},"source":"GeneralConfig.java:316/320","confidence":"derived"},
    "fission_reactor":                 {"rate":{"inQty":1,"inUnit":"mB","outQty":1,"outUnit":"mB","perTicks":1},"note":"需 Mekanism: Generators jar","confidence":"derived"}
  },
  "gather": {
    "minecraft:log":   {"destroyTime":0.5,"tool":"axe","wood":true},
    "minecraft:stone": {"destroyTime":1.5,"tool":"pickaxe","stone":true},
    "mekanism:ore_block_uranium":{"destroyTime":3.0,"tool":"pickaxe","confidence":"assumed"}
  }
}
```

约束：`tick:null` 一律渲染成「节拍未核」，**任何组件不得用 200 兜底**（§8 那批作废数字就是这么来的）。

---

## 6. ChainView.tsx —— 维度一渲染（箭头 = 机器 · 时长）

- 结构：沿用 `editor.css` 的 `.graph-cols > .graph-col` + `.g-head`（选择器作用域在 `.graph-col` 内，`editor.css:211`，所以必须用这套类名才带样式）。
- 每层一个 `.graph-col`：列头 = `深度 N · 需要 <官方中文名> ×数量`；行 = `p-row`，左侧物品名（`itemById.get(id).displayName`，**不许自己翻译**），右侧 `<mono>机器 · ticks 来源</mono>`。
- 边上文案模板：`{机器} · {ticksPerCycle} t/次 × {cycles} = {现实时长}`；速率型：`{机器} · {rate} ⇒ {时长}`；未核：`{机器} · 节拍未核`（灰色 `--mc-muted`）。
- 断点列固定放最后，按 `leaf` 分四类给四种文案：`no-recipe`（目录里没有配方）、`unresolved`（有配方但读不出边，附 type 与条数）、`cycle`（闭环，附配方 id）、`cap`（超 600 节点/24 层）。
- 顶部一行汇总：`原料合计`（`totals.items`）、`需能量`、`断点 N 处`。

---

## 7. 验收（全走 web3 UI，唯一入口 5275）

前置：服务在跑（`/tmp/mpack-clean-server -addr 127.0.0.1:18880 -data /tmp/mpack-clean`，前端 5275），包内已装通用机械（`mod-da87f7f7ac2daa462b4a0f1a`，catalog revision 19730）。

```
npx tsc --noEmit                                    # 期望 rc=0
```

金样例 1 —— 原版多级链：`http://127.0.0.1:5275/?pack=pack-06afb18714826bf9c213416d&mode=chain&item=minecraft%3Agolden_apple`
- 期望：`金苹果 ×1` → 8 金锭 → **72 金粒** → 8 金矿 → 采集层；节点 ≥ 4 层；无 `unresolved` 断点；每行机器名用官方中文。

金样例 2 —— 模组链 + 已知断点：`...&item=mekanism%3Apellet_antimatter`
- 期望：**反物质球 ×1**；`铀矿石 5,000`；`氢氟酸 5,000,000 mB`；边上出现 `富集仓 · 200 t/次`、`高炉 · 100 t/次`、`化学灌注器 · 节拍未核`、`同位素离心机 · 节拍未核`；断点列出现 2 个 `unresolved` + 1 个 `cycle`（antimatter↔pellet）；汇总行显示 `需能量 1 TJ`。
- 数字必须与 `antimatter-chain-2026-10-02.md` §9 表逐行一致（11 行、单机耗时 13.9 h/13.9 h/27.8 h/27.8 h/6.9 h/—/—/5.8 d/2.2 h/—/10 s）。

回归：`mode=graph` 三列视图与 `unstructuredByMention` 断点列不受影响（本次不改它）。

---

## 8. 分期与工时

| 期 | 内容 | 触碰 | 估算 |
|---|---|---|---|
| **P0（本期，纯前端）** | `interpret.ts` + `ticks.ts` + `expand.ts` + `chainRules.json`（通用机械 + 原版已实测条目）+ `ChainView`/`ModeChain` + 3 处挂载 | 只加文件，改 3 行 | 4 个模块 ≈ 350 行 TS/TSX |
| P1（后端工单 **BK-1**，等授权） | class 常量池解析（L2/L3）产出机器节拍表；顺带依赖 E8（jar 落盘，现在 `jar_index` 只有 `jar://<sha1>` 占位，blob 未持久化） | `apps/server` | 未估，不动手 |
| P2（工具） | 实机开客户端读 JEI 面板值，回填 `chainRules.json`（把 `confidence:'assumed'` 升成 `measured'`） | 无代码改动 | 需要 `mpack-launcher`（E2 阻塞） |
| P3（扩测） | 用 `/tmp/mkn/generic_scan.py` 跑魔法类模组，验「魔力/Mana 作为非物品输入」是否够用 | 数据 | 待用户点名模组 |

## 9. 明确不做（可 grep 核查）

- 不改后端解析器白名单（E9 归用户）、不改 `item_catalog.go`、不动 web2、不动 `apps/server` 任何文件。
- 不引入 i18n 框架、不做界面文案抽取（本轮已量化：173 条待抽，另案）。
- 不自造中文机器/物品名：一律 `itemById.get(id).displayName`；核查 `grep -rn "富集炉\|注气反应器\|溶解炉\|反物质丸\|萤石" apps/web3/src` **必须为空**。
- 不用默认 200 t 兜底未知节拍：核查 `grep -rn "200" apps/web3/src/chain` 只允许出现在 `chainRules.json` 的带出处条目里。
