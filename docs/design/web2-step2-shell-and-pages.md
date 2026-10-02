# web2 第二步 · 主体层实施方案（外壳 + 页面容器 + 状态层）

> 三步「新开」路线的第 2 步。前置：第一步 `web2-step1-framework.md` 已验收通过。
> 设计权威 = `docs/design/workbench-interaction-design.md` v2.1（下称「设计文档」）。
>
> **本步的性质：全新写。** 用户明确要求「从这一步开始，没有任何参考已有内容的必要」。
> 所以本步产出的是外壳、页面容器、状态层、URL 契约——**全部新写**，旧前端的对应实现一律不看、不抄、不迁。
> 领域组件（物品网格、配方渲染、任务书画布、编辑器）不在本步范围内，本步只给它们留**插槽**，第三步填。

---

## 0. 铁律

1. **禁读旧实现作为参考**。`apps/web/src/` 下这些目录**不许打开**：`pages/`、`app/`、`features/dashboard/`、`features/focus/`、`features/pack/`、`features/catalog/`、`features/recipe/`、`features/content/`、`features/quest/`、`ui/`。
   本步唯一可读的现有代码是 **`apps/web2/src/api/`**（契约层，第一步已迁入）和 **`apps/web2/src/styles/tokens.css`**。
   规格只从设计文档来。看不懂规格就停下来问，**不要靠翻旧代码反推意图**——那正是上一轮实现跑偏的机制。
2. 不改后端；不碰端口 5173/18765/18766/18871；不碰 `/tmp/mpack-data`；不改 `apps/web/`。
3. **不加顶层路由**。第一步定的 6 条就是全部（`/`、`/packs`、`/packs/:id`、`/packs/:id/content`、`/packs/:id/delivery`、`/settings`）。页内状态一律走查询参数。
4. **不造同义参数**。URL 参数名以 §3 的表为准，一个概念一个名字。
5. **不做假 affordance**。没有真实数据来源的按钮不许渲染（设计文档 §5-8）。占位就用第一步的 `Placeholder`，不要写"敬请期待"。
6. 令牌只走环境变量；不 `git add` / `git commit`；不装新依赖。
7. **每节验收不绿不进下一节**，输出原样贴回。
8. 允许写「素面叶子」：本步为了验收交互链路，可以在插槽位置放一个**不超过 40 行、无独立 CSS、直接用 `api/` 拉真实数据渲染成最朴素列表/数字**的临时组件。它们第三步会被整体替换，命名一律以 `Stub` 开头（如 `StubItemIndex`），方便第三步 grep 清零。

---

## 1. 本步要实现的规格（全部来自设计文档，带出处）

| 规格条目 | 出处 | 本步落点 |
|---|---|---|
| 外壳 = 48px 顶栏 + 右栏两段，**无左侧导航栏、无底边栏** | §3 图 + §4.5 | `app/AppShell.tsx` |
| 顶栏左段：品牌 + 「工作台」「整合包」2 项，**2 项封顶** | §3 顶栏三段表 | `app/TopBar.tsx` |
| 顶栏中段：面包屑 `包名 ›` + 包内三 Tab「概览/内容/交付」，**仅在有包上下文时出现** | 同上 | 同上 |
| 顶栏右段：长任务胶囊（点开=任务抽屉）· ⌘K · ⚙ | 同上 | 同上 + `app/TaskPill.tsx` |
| 右栏上段「包状况」：健康分 + 告警数 + MC/loader/版本 + 模组数，**可折叠成一行** | §3 右栏两段表、§4.5 | `app/RightRail.tsx` |
| 右栏下段「当前焦点」：焦点详情 + **带参数**传送门 | 同上 | 同上（叶子在第三步） |
| 导航可点项 5 + ⚙（原 12） | §4.5 | `TopBar` |
| 内容页四态：索引 / 关系 / 魔改 / 编排，`?mode=` 切换，**切态不丢焦点** | §4.3、§5-1 | `pages/ContentPage.tsx` |
| 内容页页内左列「来源栏」：默认折叠 56px，展开 260px，折叠态记 `localStorage`；编排态整体换成章节 rail | §3 末段、§4.3 | `pages/content/SourceRail.tsx` |
| 交付页五段纵向：①检查 ②构建 ③安装 ④起窗 ⑤发布，`?step=` 可锚定 | §4.4 | `pages/DeliveryPage.tsx` |
| 概览是唯一「做判断」的页面，冲突**就地** resolve/ignore | §4.2、§5-4 | `pages/OverviewPage.tsx` |
| URL 是焦点的单一事实源；任意状态刷新原样恢复 | §3.1 末、§5-5 | `app/useFocus.ts` |
| 长任务不打断当前工作：轮询在 context 层，离开页面进度不丢 | §5-6 | `app/PackSummaryContext.tsx` + `app/TaskPill.tsx` |
| 按 `packId` 缓存 dashboard，**不许每次路由变化重拉** | §4.5 末行 ⚠ | `app/PackSummaryContext.tsx` |
| ⌘K 命令面板：切态 + 设焦点；物品命中保留当前 `mode` | §4.5 | `app/CommandPalette.tsx` |
| 键盘：⌘K 设焦点、方向键走网格、Enter 进关系态、`E` 进写态、`Q` 进编排态、`Esc` 回上一态 | §5-10 | `app/useHotkeys.ts` |
| 上手清单胶囊 4 步，第 4 步指向交付页；完成态读后端迎新域 | §4.5 + `api/onboarding.ts` | `app/OnboardingChecklist.tsx` |
| 空态必须指向下一个动作（文案里含一个可点主按钮） | §5-9 | 所有页面 |

### 1.1 本步**不**实现（第三步做）

物品网格与详情、配方 JEI 渲染、内容文档 JSON 编辑器、任务书画布与 Inspector 表单、模组搜索抽屉、启动器面板、发布表单。本步只留插槽 + 素面叶子。

（进度树不在列表里：第三步 D-3B 已拍板砍掉，本步也不要为它留插槽。）

---

## 2. 目标结构

```
apps/web2/src/
├── app/
│   ├── AppShell.tsx            外壳骨架：TopBar + 主体 + RightRail
│   ├── TopBar.tsx              顶栏三段
│   ├── TaskPill.tsx            长任务胶囊 + 任务抽屉
│   ├── RightRail.tsx           右栏两段（包状况 + 焦点插槽）
│   ├── CommandPalette.tsx      ⌘K
│   ├── OnboardingChecklist.tsx 上手清单胶囊
│   ├── PackSummaryContext.tsx  包级汇总（健康/告警/任务）+ refresh()
│   ├── CatalogContext.tsx      包级内容目录缓存 + 派生索引 + 重建轮询
│   ├── OnboardingContext.tsx   全局迎新域（上手清单与空态四步条的唯一数据源）
│   ├── useFocus.ts             焦点 ↔ URL 的双向绑定
│   ├── useHotkeys.ts           全局键盘
│   ├── nav.ts                  导航项定义（唯一事实源，TopBar 与 ⌘K 共用）
│   ├── router.tsx              第一步已有，本步替换 Placeholder
│   ├── shell.css
│   └── Placeholder.tsx         本步结束时删除
├── pages/
│   ├── WorkbenchPage.tsx       /
│   ├── PacksPage.tsx           /packs
│   ├── OverviewPage.tsx        /packs/:id
│   ├── ContentPage.tsx         /packs/:id/content（四态壳）
│   ├── DeliveryPage.tsx        /packs/:id/delivery（五段壳）
│   ├── SettingsPage.tsx        /settings
│   └── content/
│       ├── ModeTabs.tsx        四态切换条
│       └── SourceRail.tsx      页内左列（折叠/展开/编排态换章节 rail）
├── stubs/                      素面叶子，第三步整体删除（铁律 8）
│   ├── StubItemIndex.tsx
│   ├── StubRecipeGraph.tsx
│   ├── StubContentEdit.tsx
│   ├── StubQuest.tsx
│   ├── StubModRail.tsx
│   ├── StubConflicts.tsx
│   ├── StubHealth.tsx
│   ├── StubFocusInspector.tsx
│   ├── StubTasks.tsx
│   └── StubDeliverySections.tsx
├── api/                        第一步迁入，零改动
└── styles/                     第一步产出
```

组件树：

```
RouterProvider
└── AppShell                        ← 全局 Provider 都挂这里
    ├── OnboardingContext.Provider  ← 与包无关，挂最外层
    ├── PackSummaryContext.Provider
    ├── TopBar
    │   ├── 左：品牌 · 工作台 · 整合包
    │   ├── 中：包名 › · 概览 · 内容 · 交付     （无包上下文时整段不渲染）
    │   └── 右：TaskPill · ⌘K · ⚙
    ├── main（Outlet）              ← 页面容器
    ├── RightRail
    │   ├── 包状况段（可折叠）
    │   └── 焦点段（插槽 → StubFocusInspector）
    ├── OnboardingChecklist
    ├── CommandPalette
    └── TaskDrawer（TaskPill 点开）
```

---

## 3. URL 契约（本步的脊柱，先定死再写组件）

### 3.1 参数总表

| 参数 | 取值 | 生效路由 | 含义 | 谁写 | 谁读 |
|---|---|---|---|---|---|
| `mode` | `index` \| `graph` \| `edit` \| `quest` | `/packs/:id/content` | 内容页四态，缺省 `index` | ModeTabs、⌘K、传送门、概览 tile | ContentPage |
| `item` | 物品 id | 全部包内路由 | **焦点：物品**（沿用已实现参数名，禁改） | 索引态点选、⌘K、传送门 | useFocus → 各态 |
| `f` | `recipe` \| `mod` \| `node` \| `doc` | 全部包内路由 | 焦点类型（非物品时） | 同上 | useFocus |
| `fid` | 对应 id | 全部包内路由 | 焦点 id（非物品时） | 同上 | useFocus |
| `ns` | 模组 id 或命名空间 | `/content` | 来源筛选（沿用已实现参数名，禁改） | 来源栏、概览「贡献 N 物品」 | 索引态 |
| `type` | 内容 kind | `/content` | 类型筛选（配方/物品模型/结构/地形/战利品/进度/标签/元数据/语言） | 索引态筛选条 | 索引态 |
| `rail` | `mods` | `/content` | 来源栏展开模组抽屉 | 顶栏、概览「查看全部模组」 | SourceRail |
| `doc` | 内容文档 id | `/content?mode=edit` | 写态选中文档 | 写态列表、传送门 | 写态 |
| `node` | 任务节点 id | `/content?mode=quest` | 编排态选中节点 | 画布点选、传送门、「生成任务节点」 | 编排态 |
| `panel` | `conflicts` | `/packs/:id` | 概览冲突面板锚点 | 交付页闸门提示、告警点击 | OverviewPage |
| `step` | `install` | `/packs/:id/delivery` | 交付页滚动/高亮到某段 | 上手清单第 4 步、构建完成提示 | DeliveryPage |

**规则**：
- 焦点只有一种表达：物品用 `?item=`，其余用 `?f=`+`?fid=`。**不许**再出现 `?focus=`、`?selected=` 之类同义参数。
- 切 `mode` 时**保留**焦点参数；设焦点时**保留** `mode`。这是设计文档 §5-1「切态不丢焦点」的实现约束。
- 任何参数变化都用 `setSearchParams(next, {replace: true})`，除切 `mode` 用 `replace: false`（让浏览器后退能在四态间走）。

### 3.2 六个页面各自读什么

| 路由 | 读 | 写 |
|---|---|---|
| `/` | — | — |
| `/packs` | — | — |
| `/packs/:id` | `panel` | `panel`、`item`/`f`+`fid`（点 tile/模组摘要时）、`mode`（跳转内容页时） |
| `/packs/:id/content` | `mode`、`item`/`f`+`fid`、`ns`、`type`、`rail`、`doc`、`node` | 同左全部 |
| `/packs/:id/delivery` | `step` | `step`、`panel`（跳概览处理冲突时） |
| `/settings` | — | — |

---

## 4. 状态层（五个文件，全新写）

### 4.1 `app/useFocus.ts`

职责：把焦点从「内存 useState」变成「URL 的视图」。设计文档 §5-5 的判据是**刷新后焦点原样恢复**，纯内存实现不可能满足。

```tsx
import {useCallback, useMemo} from 'react';
import {useSearchParams} from 'react-router-dom';

export type FocusKind = 'item' | 'recipe' | 'mod' | 'node' | 'doc';
export type Focus = {kind: FocusKind; id: string} | null;

/* 焦点在 URL 里只有一种表达：物品用 ?item=（沿用既有参数名），其余用 ?f= + ?fid=。
   这个 hook 是它的唯一读写入口 —— 组件不许自己碰这三个参数，否则会出现两处写、互相覆盖。 */
export function useFocus(): [Focus, (next: Focus, opts?: {mode?: string}) => void] {
  const [params, setParams] = useSearchParams();

  const focus = useMemo<Focus>(() => {
    const item = params.get('item');
    if (item) return {kind: 'item', id: item};
    const kind = params.get('f') as FocusKind | null;
    const id = params.get('fid');
    return kind && id ? {kind, id} : null;
  }, [params]);

  const setFocus = useCallback((next: Focus, opts?: {mode?: string}) => {
    setParams(prev => {
      const p = new URLSearchParams(prev);
      p.delete('item'); p.delete('f'); p.delete('fid');
      if (next) {
        if (next.kind === 'item') p.set('item', next.id);
        else { p.set('f', next.kind); p.set('fid', next.id); }
      }
      if (opts?.mode) p.set('mode', opts.mode);
      return p;
    }, {replace: true});
  }, [setParams]);

  return [focus, setFocus];
}
```

**验收**：设焦点 → 刷新 → 焦点还在；切 `mode` → 焦点还在；清焦点 → URL 里三个参数都不剩。

### 4.2 `app/PackSummaryContext.tsx`

职责：包级汇总数据的**唯一**来源 + 长任务轮询的**唯一**所在。解决设计文档 §4.5 末行的 ⚠（现状每次路由变化重拉 dashboard）和 §5-6（长任务离开页面即丢进度）。

```tsx
import {createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode} from 'react';
import {useParams} from 'react-router-dom';
import {packHealth, listLocks, listConflicts, type PackHealth, type Lock, type Conflict} from '../api/mods';
import {fetchTasks, type Task} from '../api/tasks';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';

/* ↑ 这些导出名已逐个核对过真实文件（api/mods.ts、api/tasks.ts、api/dashboard.ts）。
   注意坑：dashboard 域没有「按 packId 取单包」的端点，只有全局的 fetchDashboard()，
   返回 {packs[], lastEditedPackId, todayResolvedCount}，本包要自己 find。
   任务域的函数名是 fetchTasks 不是 listTasks。 */

export type PackSummary = {
  packId: string | null;
  pack: DashboardPack | null;
  health: PackHealth | null;
  locks: Lock[];
  pendingConflicts: Conflict[];
  tasks: Task[];
  score: number;          // 健康分，公式见下
  alertCount: number;
  loading: boolean;
  error: string | null;
  refresh: () => void;    // 任何写操作成功后调用；这是「改完立刻看得见」的唯一机制
};

const Ctx = createContext<PackSummary | null>(null);

export function usePackSummary(): PackSummary {
  const v = useContext(Ctx);
  if (!v) throw new Error('usePackSummary 必须在 PackSummaryProvider 内使用');
  return v;
}

/* 健康分公式必须只此一处。设计文档 §4.2 的 ⚠ 指出旧实现把公式埋在组件里，
   导致右栏和概览各算一份、可能不一致。
   扣分口径全部取自后端已返回的计数字段（PackHealth.pendingErrors / pendingWarnings、
   DashboardPack.alerts.crashes / alerts.updatable），不自己数数组：
   error 级待解决 -8，warning 级 -3，崩溃告警 -6，可更新模组 -1；下限 0 上限 100。 */
function computeScore(h: PackHealth | null, p: DashboardPack | null): number {
  return Math.max(0, Math.min(100,
    100
    - (h?.pendingErrors ?? 0) * 8
    - (h?.pendingWarnings ?? 0) * 3
    - (p?.alerts.crashes ?? 0) * 6
    - (p?.alerts.updatable ?? 0),
  ));
}

/* Conflict.status 在契约里是自由字符串（api/mods.ts 的 conflictSchema），
   后端 resolve/ignore 会写成 'resolved' / 'ignored'，其余一律算待解决。
   不要写成 status === 'pending' —— 那个值后端未必用。 */
const SETTLED = new Set(['resolved', 'ignored']);

export function PackSummaryProvider({children}: {children: ReactNode}) {
  const {id} = useParams();
  const packId = id ?? null;
  const [pack, setPack] = useState<DashboardPack | null>(null);
  const [health, setHealth] = useState<PackHealth | null>(null);
  const [locks, setLocks] = useState<Lock[]>([]);
  const [pending, setPending] = useState<Conflict[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!packId) { setPack(null); setHealth(null); setLocks([]); setPending([]); return; }
    setLoading(true);
    try {
      const [dash, h, l, c] = await Promise.all([
        fetchDashboard(), packHealth(packId), listLocks(packId), listConflicts(packId),
      ]);
      setPack(dash.packs.find(p => p.id === packId) ?? null);
      setHealth(h); setLocks(l);
      setPending(c.filter(x => !SETTLED.has(x.status)));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [packId]);

  useEffect(() => { void load(); }, [load]);

  /* 任务轮询固定 5s。放在 context 而不是页面里，这样切态/切页都不会丢进度
     （设计文档 §5-6）。fetchTasks 是全局的，按 packId 过滤在消费方做。 */
  useEffect(() => {
    const tick = async () => {
      try { setTasks(await fetchTasks()); } catch { /* 轮询失败不打扰用户，下一轮再试 */ }
    };
    void tick();
    const t = window.setInterval(tick, 5000);
    return () => window.clearInterval(t);
  }, []);

  const value = useMemo<PackSummary>(() => ({
    packId, pack, health, locks, pendingConflicts: pending, tasks,
    score: computeScore(health, pack),
    alertCount: (health?.pendingErrors ?? 0) + (pack?.alerts.crashes ?? 0),
    loading, error, refresh: () => void load(),
  }), [packId, pack, health, locks, pending, tasks, loading, error, load]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
```

> **上面用到的导出名已逐个核对过真实文件**（2026-10-01），照抄即可，不用再猜：
>
> | 文件 | 真实导出 | 坑 |
> |---|---|---|
> | `api/dashboard.ts` | `fetchDashboard(): Promise<DashboardData>`、`fetchActivities(): Promise<DashboardActivity[]>`、`type DashboardPack`、`type DashboardData`、`type DashboardActivity` | **没有按 packId 取单包的端点**。`DashboardData = {packs[], lastEditedPackId, todayResolvedCount}`，本包要 `packs.find(p => p.id === packId)` |
> | `api/tasks.ts` | `fetchTasks(): Promise<Task[]>`、`pauseTask` / `resumeTask` / `cancelTask` / `retryTask`、`type Task` | 函数名是 `fetchTasks`，**不是** `listTasks`。`Task.status` = `queued\|running\|success\|failed\|cancelled\|paused`，**没有 `pending`** |
> | `api/mods.ts` | `packHealth(packId): Promise<PackHealth>`、`listLocks(packId): Promise<Lock[]>`、`listConflicts(packId): Promise<Conflict[]>`、`resolvePack(packId)` | `PackHealth = {packId, mods, installed, pendingErrors, pendingWarnings, healthy}`；`Conflict.status` 是自由字符串，**不能假定它等于 `'pending'`** |
> | `DashboardPack` 字段 | `id` `name` `iconUrl`(nullable) `mcVersion` `loader` `packVersion` `modCount{total,installed,selected}` `conflicts{resolved,pending}` `edits{recipes,structures,ores,quests}` `alerts{crashes,updatable}` `lastEditedAt` `createdAt` | `alerts` / `modCount` 都是**必有**字段，不是可选，所以用 `pack?.alerts.crashes` 而不是 `pack?.alerts?.crashes` |
>
> 这是铁律 1 允许的唯一"读现有代码"情形：api 层是契约不是交互实现。

**验收**：在内容页四态之间来回切 8 次，devtools Network 里 `dashboard`/`health`/`locks`/`conflicts` 各只有 1 次请求（不是 8 次）；`tasks` 每 5s 一次。

### 4.3 `app/CatalogContext.tsx`

职责：包级**内容目录**（物品/方块/标签/配方）的唯一缓存 + 派生索引 + 重建轮询。

为什么必须有这一层：索引态、关系态、焦点 Inspector 三处都要用同一份目录数据。旧实现的问题是**有两套并行加载逻辑**（设计文档 §4.3 页内左列的 ⚠：`ModContentPage.tsx:58-59,152-168` 自己拉了一份 catalog，与包级 context 重复）。web2 只准有一套。

契约出处（`apps/web2/src/api/catalog.ts`，第一步已迁入，**以文件为准**）：

| 导出 | 形状 |
|---|---|
| `getItemCatalog(packId, locale='zh_cn')` | `ItemCatalog` = `{revision, builtAt, locale, availableLocales[], warnings[], items[], blocks[], tags[], recipes[]}` |
| `getCatalogStatus(packId)` | `{sourceRevision, builtRevision, status: 'pending'\|'running'\|'succeeded'\|'failed', builtAt, lastError, warnings[], stale}` |
| `rebuildItemCatalog(packId, locale='zh_cn')` | `{taskId, status}` |
| `CatalogItem` | `{id, displayName, resolvedLocale, evidence, modelPath, iconStatus, names[{locale,name,key,source}], tags[]}` |
| `CatalogRecipe` | `{id, type, status, payload, diagnostics[], refs[{role:'input'\|'output', kind:'item'\|'item_tag', id, slot, alternative, count}]}` |
| `CatalogTag` | `{registry:'item'\|'block', id, displayName, resolvedLocale, status, labelSource, diagnostics[], members[]}` |

```tsx
import {createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode} from 'react';
import {useParams} from 'react-router-dom';
import {
  getItemCatalog, getCatalogStatus, rebuildItemCatalog,
  type ItemCatalog, type CatalogItem, type CatalogRecipe, type CatalogTag,
} from '../api/catalog';

export type CatalogState = {
  packId: string | null;
  catalog: ItemCatalog | null;
  status: Awaited<ReturnType<typeof getCatalogStatus>> | null;
  locale: string;
  setLocale: (v: string) => void;
  /* 派生索引：一律在这里算一次，组件不许各自 reduce 全量数组（几千个物品 × 每次渲染 = 卡） */
  itemById: Map<string, CatalogItem>;
  tagById: Map<string, CatalogTag>;
  recipesByOutput: Map<string, CatalogRecipe[]>;   // 物品 id → 它作为产物的配方
  recipesByInput: Map<string, CatalogRecipe[]>;    // 物品 id → 它作为原料的配方
  namespaces: {ns: string; count: number}[];       // 来源栏/索引态的命名空间计数
  rebuild: () => Promise<void>;                    // 触发重建；轮询由本 context 负责
  refreshing: boolean;
  error: string | null;
};

const Ctx = createContext<CatalogState | null>(null);

export function useCatalog(): CatalogState {
  const v = useContext(Ctx);
  if (!v) throw new Error('useCatalog 必须在 CatalogProvider 内使用');
  return v;
}

const NS_RE = /^([^:]+):/;

export function CatalogProvider({children}: {children: ReactNode}) {
  const {id} = useParams();
  const packId = id ?? null;
  const [locale, setLocale] = useState('zh_cn');
  const [catalog, setCatalog] = useState<ItemCatalog | null>(null);
  const [status, setStatus] = useState<CatalogState['status']>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!packId) { setCatalog(null); setStatus(null); return; }
    setRefreshing(true);
    try {
      const [c, s] = await Promise.all([getItemCatalog(packId, locale), getCatalogStatus(packId)]);
      setCatalog(c); setStatus(s); setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRefreshing(false);
    }
  }, [packId, locale]);

  useEffect(() => { void load(); }, [load]);

  /* 重建/解析进行中时轮询 status，落到终态后自动重载目录。
     放在 context 而不是页面：切态切页都不能丢进度（设计文档 §5-6）。 */
  useEffect(() => {
    if (!packId) return;
    const busy = status?.status === 'pending' || status?.status === 'running';
    if (!busy) return;
    const t = window.setInterval(async () => {
      try {
        const s = await getCatalogStatus(packId);
        setStatus(s);
        if (s.status === 'succeeded' || s.status === 'failed') await load();
      } catch { /* 下一轮再试 */ }
    }, 3000);
    return () => window.clearInterval(t);
  }, [packId, status?.status, load]);

  const derived = useMemo(() => {
    const itemById = new Map<string, CatalogItem>();
    const tagById = new Map<string, CatalogTag>();
    const recipesByOutput = new Map<string, CatalogRecipe[]>();
    const recipesByInput = new Map<string, CatalogRecipe[]>();
    const nsCount = new Map<string, number>();
    for (const it of catalog?.items ?? []) {
      itemById.set(it.id, it);
      const ns = NS_RE.exec(it.id)?.[1] ?? '(无命名空间)';
      nsCount.set(ns, (nsCount.get(ns) ?? 0) + 1);
    }
    for (const tg of catalog?.tags ?? []) tagById.set(tg.id, tg);
    for (const rc of catalog?.recipes ?? []) {
      for (const ref of rc.refs) {
        const bucket = ref.role === 'output' ? recipesByOutput : recipesByInput;
        const arr = bucket.get(ref.id);
        if (arr) arr.push(rc); else bucket.set(ref.id, [rc]);
      }
    }
    const namespaces = [...nsCount.entries()].map(([ns, count]) => ({ns, count})).sort((a, b) => b.count - a.count);
    return {itemById, tagById, recipesByOutput, recipesByInput, namespaces};
  }, [catalog]);

  const value = useMemo<CatalogState>(() => ({
    packId, catalog, status, locale, setLocale, ...derived,
    rebuild: async () => {
      if (!packId) return;
      await rebuildItemCatalog(packId, locale);
      await load();   // 触发一次 status 拉取，让轮询接管
    },
    refreshing, error,
  }), [packId, catalog, status, locale, derived, load, refreshing, error]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
```

挂在 `AppShell` 里，**嵌在 `PackSummaryProvider` 内层**（两者都依赖 `useParams().id`，互不依赖对方）：

```tsx
<PackSummaryProvider>
  <CatalogProvider>
    … 顶栏 / 主体 / 右栏 …
  </CatalogProvider>
</PackSummaryProvider>
```

**验收**：
- 索引态与关系态显示同一份物品数据，Network 里 `catalog?locale=` 只请求 1 次（不是每态一次）。
- 点「重建目录」→ 顶栏胶囊出现任务 → 切到别的态再切回来，进度还在 → 落终态后目录自动刷新。
- 全仓只有一处 `getItemCatalog(` 调用：`grep -rn 'getItemCatalog(' apps/web2/src | grep -v 'api/catalog.ts'` 只应命中 `CatalogContext.tsx`。

### 4.4 `app/nav.ts`

```ts
/* 导航项的唯一事实源。TopBar 渲染它，CommandPalette 也用它生成页面命令 ——
   两处各写一份是上一轮"导航膨胀"的根因之一。 */
export type NavItem = {to: string; label: string; scope: 'global' | 'pack'; keys?: string};

export const GLOBAL_NAV: NavItem[] = [
  {to: '/', label: '工作台', scope: 'global'},
  {to: '/packs', label: '整合包', scope: 'global'},
];

/* 包内三项。v2.1 把旧的 10 项收敛成这 3 项，多一项就是违反设计文档 §2 的减法判据。 */
export const PACK_NAV: NavItem[] = [
  {to: '', label: '概览', scope: 'pack'},
  {to: 'content', label: '内容', scope: 'pack'},
  {to: 'delivery', label: '交付', scope: 'pack'},
];

/* 内容页四态。它们是查询参数不是路由（铁律 3）。 */
export const CONTENT_MODES = [
  {mode: 'index', label: '索引', hotkey: ''},
  {mode: 'graph', label: '关系', hotkey: 'Enter'},
  {mode: 'edit', label: '魔改', hotkey: 'E'},
  {mode: 'quest', label: '编排', hotkey: 'Q'},
] as const;

export type ContentMode = typeof CONTENT_MODES[number]['mode'];
```

### 4.5 `app/OnboardingContext.tsx`

上手清单胶囊（§5.8）和工作台空态的四步流程条（§6.1）**是同一份数据的两个视图**。放在一个 context 里，`GET /api/onboarding` 全站只发一次。挂在 `PackSummaryProvider` 之外（它跟包无关，是全局域）。

```tsx
import {createContext, useCallback, useContext, useEffect, useState, type ReactNode} from 'react';
import {acknowledgeOnboarding, fetchOnboarding, type Onboarding} from '../api/onboarding';

type OnboardingState = {
  steps: Onboarding['steps'] | null;   // null = 还没拉到，UI 一律显示未完成，禁默认打勾
  loading: boolean;
  error: string | null;
  ack: (key: keyof Onboarding['steps']) => Promise<void>;
  reload: () => Promise<void>;
};

const Ctx = createContext<OnboardingState | null>(null);

export function OnboardingProvider({children}: {children: ReactNode}) {
  const [steps, setSteps] = useState<Onboarding['steps'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const v = await fetchOnboarding();
      setSteps(v.steps);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void reload(); }, [reload]);

  /* 用后端回填的全量 steps 覆盖本地，不做乐观更新：
     后端可能同时推进别的键（例如建包时自动勾 firstPack），乐观写会把它盖掉。 */
  const ack = useCallback(async (key: keyof Onboarding['steps']) => {
    try {
      const v = await acknowledgeOnboarding({[key]: true});
      setSteps(v.steps);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  return <Ctx.Provider value={{steps, loading, error, ack, reload}}>{children}</Ctx.Provider>;
}

export function useOnboarding(): OnboardingState {
  const v = useContext(Ctx);
  if (!v) throw new Error('useOnboarding 必须在 OnboardingProvider 之内');
  return v;
}

/* UI 四步 ↔ 后端 steps 键。prismAccount 已废弃（恒 true），不在清单里。 */
export const CHECKLIST = [
  {key: 'firstPack', label: '新建 / 导入整合包'},
  {key: 'firstMod', label: '添加第一个模组'},
  {key: 'curseforgeKey', label: '配置 CurseForge Key'},
  {key: 'launcherReady', label: '启动器就绪并起窗'},
] as const satisfies readonly {key: keyof Onboarding['steps']; label: string}[];
```

`AppShell` 里包一层：`<OnboardingProvider><PackSummaryProvider>…</PackSummaryProvider></OnboardingProvider>`。

**禁止**：`prismAccount` 出现在 UI（§7.2 检查 K）；从 `usePackSummary()` 的模组数/冲突数反推完成态；在 `OnboardingChecklist` 或 `WorkbenchPage` 里直接调 `fetchOnboarding()`（检查 K 要求调用点唯一）。

---

## 5. 外壳组件

### 5.1 `app/AppShell.tsx`

```tsx
import {Outlet} from 'react-router-dom';
import {PackSummaryProvider} from './PackSummaryContext';
import {OnboardingProvider} from './OnboardingContext';
import {TopBar} from './TopBar';
import {RightRail} from './RightRail';
import {CommandPalette} from './CommandPalette';
import {OnboardingChecklist} from './OnboardingChecklist';
import {useHotkeys} from './useHotkeys';
import './shell.css';

export function AppShell() {
  const [paletteOpen, openPalette] = useHotkeys();
  return (
    <OnboardingProvider>
      <PackSummaryProvider>
        <div className="shell">
          <TopBar onOpenPalette={() => openPalette(true)}/>
          <div className="shell-body">
            <main className="shell-main"><Outlet/></main>
            <RightRail/>
          </div>
          <OnboardingChecklist/>
          <CommandPalette open={paletteOpen} onOpenChange={openPalette}/>
        </div>
      </PackSummaryProvider>
    </OnboardingProvider>
  );
}
```

`router.tsx` 改为把 6 条路由挂在 `AppShell` 之下（`createBrowserRouter([{element: <AppShell/>, children: [...6 条...]}])`）。第一步的 `Placeholder.tsx` 在 6 个页面都写出来后**删除**。

### 5.2 `shell.css` 布局骨架

```css
.shell {
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.shell-body {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: var(--mc-sp-sm);
  padding: var(--mc-sp-sm);
  overflow: hidden;
}
.shell-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: auto;
}
```

**没有底边栏，所以没有 `padding-bottom` 补偿**（设计文档 §4.5 明确删除）。右栏宽度用 `--mc-rail-w` / `--mc-rail-w-fold`。

### 5.3 `app/TopBar.tsx`

结构（三段，用 `nav.ts` 的常量渲染，禁硬编码标签）：

```
┌ .app-topbar (height: var(--mc-topbar-h)) ─────────────────────────────────┐
│ [.tb-left  ◆ mPack | 工作台  整合包]                                       │
│ [.tb-mid   包名 ›  概览  内容  交付]     ← 仅 packId 存在时渲染            │
│ [.tb-right <TaskPill/>  [🔍 ⌘K]  [⚙]]                                    │
└───────────────────────────────────────────────────────────────────────────┘
```

实现要点：
- `.tb-mid` 的面包屑是**包名**（来自 `usePackSummary().pack?.name`），不是"创作工具/包工作台"这类分组标题——设计文档 §3 明确删掉假分组。
- 包内 Tab 的高亮判定：`概览` = 路径正好 `/packs/:id`；`内容` = 以 `/content` 结尾；`交付` = 以 `/delivery` 结尾。用 `NavLink` 的 `end` 属性处理概览，避免它把子路由也吃成高亮。
- `.tb-left` 两项 + `.tb-mid` 三项 = **可点导航 5 项**，加 ⚙ 图标按钮。验收要数得出来。
- ⌘K 按钮点击 = 打开命令面板（与快捷键同一入口）。
- 无包上下文（`/`、`/packs`、`/settings`）时 `.tb-mid` **整段不渲染**，不是渲染成灰色。

### 5.4 `app/TaskPill.tsx`

- 数据源 `usePackSummary().tasks`，只取未终态：`status === 'queued' || status === 'running'`。
  **注意**：`Task.status` 的枚举是 `queued|running|success|failed|cancelled|paused`，**没有 `pending`**（以 `api/tasks.ts` 为准）。
- 无未终态任务 → **组件返回 null**（不是渲染一个空胶囊）。
- 有任务 → 胶囊显示 `⏳ <最近一条的 kind 中文> <progress>%`；`paused` 显示暂停图标；点击打开 `Drawer`。
- Drawer 内容 = 插槽，本步放 `StubTasks`（列出全部任务 + status + progress），第三步换真实任务面板（暂停/继续/取消/重试/错误详情）。
- 概览页要能看到**本包**任务：`tasks.filter(t => t.packId === packId)`，本步在概览的素面叶子里用上。

### 5.5 `app/RightRail.tsx`

```
┌ .right-rail (width: var(--mc-rail-w)) ─┐
│ [.psc  包状况            [折叠 ▸]]     │  ← 可折叠成一行（--mc-rail-w-fold? 不，高度折叠）
│   健康 82 · 告警 2                      │
│   MC 1.21.1 · Fabric · v0.3.0 · 24 模组 │
│ ──────────────────────────────────────  │
│ [.rr-focus  当前焦点]                   │
│   <插槽：StubFocusInspector>            │
└─────────────────────────────────────────┘
```

- 包状况段数据全部来自 `usePackSummary()`，**组件内不许自己发请求**。
- 折叠：点标题栏右侧箭头，折叠后只剩一行（健康分 + 告警数），状态记 `localStorage['web2.rail.collapsed']`。
- 焦点段：无焦点时显示空态，文案必须含一个可点主按钮（设计文档 §5-9），本步用「按 ⌘K 找一个物品」并真的打开命令面板。
- 有焦点时渲染插槽，本步 = `StubFocusInspector`：显示 `kind` + `id` + **三个带参数传送门按钮**：
  - 「在索引态打开」→ `setFocus(focus, {mode:'index'})` 并导航到 `/packs/:id/content`
  - 「去魔改」→ `setFocus(focus, {mode:'edit'})`
  - 「看关系」→ `setFocus(focus, {mode:'graph'})`
  这三个按钮**必须带焦点参数**——设计文档 §4.5 的 ⚠ 指出旧实现三个按钮全不带参数，跳过去等于从头找。这是本步最重要的一条验收。
- 右栏 `overflow: hidden` + 内部 `display:flex; flex-direction:column`，焦点段 `flex:1; min-height:0; overflow:auto`。**不许用 `position: sticky` + `height: 100vh`**（那是旧外壳为底边栏做的补偿）。

### 5.6 `app/CommandPalette.tsx`

受控组件：`{open: boolean; onOpenChange: (v: boolean) => void}`（快捷键在 `useHotkeys` 里，组件自己不监听全局键盘，避免两处监听）。

命令来源两部分：
1. **页面/态命令**，由 `nav.ts` 生成：工作台、整合包、概览、内容·索引、内容·关系、内容·魔改、内容·编排、交付、设置。选「内容·关系」= 导航到 `/packs/:id/content?mode=graph`，**保留当前焦点参数**。
2. **物品命令**（第三步接真实目录搜索）：本步留一个 `StubItemSearch`，输入非空时调 `api/catalog.ts` 的搜索接口取前 8 条。选中后 `setFocus({kind:'item', id}, {mode: 当前 mode})` —— **保留当前 mode，不硬跳某个页面**。设计文档 §4.5 的 ⚠ 明确指出旧实现物品命中会硬跳 `/recipes`。

关闭时机：选中、Esc、点击遮罩。

### 5.7 `app/useHotkeys.ts`

```tsx
import {useEffect, useState} from 'react';
import {useLocation, useNavigate, useSearchParams} from 'react-router-dom';
import {CONTENT_MODES, type ContentMode} from './nav';

/* 全局键盘（设计文档 §5-10）。返回 [面板开关, setter] 给 AppShell。
   输入框聚焦时一律不响应单键快捷键，否则在 JSON 编辑器里敲 e/q 会切态。 */
export function useHotkeys(): [boolean, (v: boolean) => void] {
  const [open, setOpen] = useState(false);
  const nav = useNavigate();
  const loc = useLocation();
  const [params, setParams] = useSearchParams();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const typing = !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault(); setOpen(v => !v); return;
      }
      if (e.key === 'Escape') {
        if (open) { setOpen(false); return; }
        // Esc 回上一态：按 CONTENT_MODES 顺序退一格，已在 index 则不动
        if (!loc.pathname.endsWith('/content')) return;
        const cur = (params.get('mode') ?? 'index') as ContentMode;
        const i = CONTENT_MODES.findIndex(m => m.mode === cur);
        if (i > 0) setParams(p => { const n = new URLSearchParams(p); n.set('mode', CONTENT_MODES[i - 1].mode); return n; }, {replace: false});
        return;
      }
      if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
      if (!loc.pathname.endsWith('/content')) return;

      const k = e.key.toLowerCase();
      if (k === 'e' || k === 'q') {
        e.preventDefault();
        setParams(p => { const n = new URLSearchParams(p); n.set('mode', k === 'e' ? 'edit' : 'quest'); return n; }, {replace: false});
      }
      if (e.key === 'Enter') {
        const cur = params.get('mode');
        if (cur === 'index') {
          e.preventDefault();
          setParams(p => { const n = new URLSearchParams(p); n.set('mode', 'graph'); return n; }, {replace: false});
        }
      }
      if (e.key.startsWith('Arrow')) {
        // 方向键走物品网格：交给索引态叶子处理，这里只负责不抢事件
        void nav; // 保留 nav 引用以便后续跳转命令使用
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, loc.pathname, params, setParams, nav]);

  return [open, setOpen];
}
```

> `void nav` 那一段是**故意留白**：方向键走网格需要知道网格里哪个格子聚焦，那是第三步叶子的内部状态。本步只保证不抢事件。第三步实现时把这段替换成真实逻辑，并删掉 `void nav`。
> 若 `noUnusedLocals` 报错，用上面的 `void nav` 方式，**不许关掉 tsconfig 的开关**。

### 5.8 `app/OnboardingChecklist.tsx`

4 步胶囊浮层（不占栏）。**数据只从 `useOnboarding()` 拿，步骤定义只用 §4.5 的 `CHECKLIST`**，本文件不许自己发请求、不许自己写一份步骤数组：

```tsx
import {NavLink} from 'react-router-dom';
import {CHECKLIST, useOnboarding} from './OnboardingContext';

/* 跳转目标：键 → 路径。带 :id 的项在没有包上下文时禁用（不许跳到 /packs/undefined）。 */
const TARGET: Record<typeof CHECKLIST[number]['key'], string> = {
  firstPack: '/packs',
  firstMod: 'content?rail=mods',       // 相对当前包
  curseforgeKey: '/settings',
  launcherReady: 'delivery?step=install',
};

export function OnboardingChecklist() {
  const {steps, loading, ack} = useOnboarding();
  if (loading || !steps) return null;          // 未拉到就不渲染，禁默认打勾
  const done = CHECKLIST.filter(s => steps[s.key]).length;
  if (done === CHECKLIST.length) return null;  // 全部完成后整个胶囊消失
  // …渲染 4 个胶囊 + 进度 done/4；点击未完成的当前步 → NavLink 到 TARGET[key]
  //    并在落到目标页且动作真实发生后调 ack(key)
}
```

| UI 步 | steps 键 | 点击后跳 |
|---|---|---|
| 1 新建/导入整合包 | `firstPack` | `/packs` |
| 2 添加模组 | `firstMod` | 当前包 `/content?rail=mods`（无包上下文 → `/packs`） |
| 3 配 CurseForge Key | `curseforgeKey` | `/settings` |
| 4 启动器就绪并起窗 | `launcherReady` | 当前包 `/delivery?step=install`（无包上下文 → `/packs`） |

**完成判定不许自己从包数据推导**——后端有独立的迎新域，`api/onboarding.ts` 已存在且是唯一事实源：

| 事实 | 出处（已核对） |
|---|---|
| `fetchOnboarding(): Promise<Onboarding>` · `acknowledgeOnboarding(steps: Record<string,boolean>): Promise<Onboarding>` | `api/onboarding.ts:19,24`（`GET/PUT /api/onboarding`） |
| `Onboarding = {steps: {curseforgeKey, firstPack, firstMod, prismAccount, launcherReady}}`，全 boolean | `api/onboarding.ts:8-17` |
| `prismAccount` 有 `.default(true)`，**已废弃且恒为 true**（自研 mPackLauncher 不要求正版）→ 不进清单 | `api/onboarding.ts:6,13` |
| `launcherReady` 有 `.default(false)`，**这才是真实的第 4 步待办** | `api/onboarding.ts:14` |

实现约束：
- `ack(key)` 由 §4.5 的 context 发 `PUT`，用后端回填的全量 steps 覆盖本地；组件里不要出现 `fetchOnboarding`/`acknowledgeOnboarding`（§7.2 检查 K）。
- 设计文档 §4.5 写的是"4 步，第 4 步指向交付页"——与此表一致，只是第 3 步的语义以 `steps` 契约为准（原稿写的"处理冲突"在迎新域里没有对应键，冲突待办已经在概览页与右栏告警里表达，不重复占一步）。
- **判定不出来的步骤不许显示成已完成**（设计文档 §5-8 禁假 affordance）。`steps` 里 false 就是未完成，不要拿 `usePackSummary()` 的模组数/冲突数去反推。
- `ack` 的触发点是**动作真实完成之后**（包建出来了 / 模组加进去了 / Key 存下来了 / 起窗成功），不是"点了跳转就算完成"。跳转只负责带路。

---

## 6. 页面容器

每个页面本步只写**布局 + 真实数据的素面呈现 + 插槽**。第三节列的 URL 参数读写必须落实。

### 6.1 `pages/WorkbenchPage.tsx`（`/`）

规格 = 设计文档 §4.1。布局四块：

```
[环境自检横幅]                     ← 有告警才渲染
[最近编辑包大卡 | 后台任务面板]
[包列表（含"只看待处理"开关）]
[最近动态 + 今日已解决计数]
```

- 空态（一个包都没有）→ 迎新视图：Hero + 「新建整合包」+「导入包」两个主按钮 + 四步流程条。设计文档 §4.1 的 ⚠：starter 灵感卡如果不可点就**删掉**，不要留纯展示卡。
  - 四步流程条与 §5.8 的上手清单**同源**：都读 `useOnboarding().steps`、都用 §4.5 的 `CHECKLIST`，禁两处各算一套、禁在页面里自己发 `GET /api/onboarding`（§7.2 检查 K：该调用点全站唯一）。
- 本步的素面叶子：`StubPackList`（列出 `api/packs.ts` 的真实包，每行一个「打开」按钮）、`StubTasks`、`StubActivity`。
- **禁做**：重命名/复制/一键打包这类没有后端支撑的 Dropdown（设计文档 §4.1 ⚠ 指出旧实现只弹"后续版本提供"）。要么本步就不渲染，要么第三步接真实端点后再加。

### 6.2 `pages/PacksPage.tsx`（`/packs`）

全部包的表格 + 新建/导入入口。素面叶子 `StubPackList` 复用。

### 6.3 `pages/OverviewPage.tsx`（`/packs/:id`）

规格 = 设计文档 §4.2。**这是唯一用来做判断的页面**。布局：

```
[出包 CTA 条]                                  → /packs/:id/delivery
[健康区（插槽）| 目录规模信号 4 tile]
[冲突面板（?panel=conflicts 时展开并滚动到位）]
[已选模组摘要 前 5 + 「查看全部」→ /content?rail=mods]
[本包任务]
```

- 健康区数据来自 `usePackSummary()`，本步 `StubHealth` 直接显示分数 + 三个计数。
- **目录规模 4 个 tile（物品/配方/魔改/任务）点击行为**：设焦点（若有对应对象）+ 切到内容页对应态。设计文档 §4.2 ⚠ 指出旧实现只是 `navigate`，本步必须用 §3 的参数写法：
  - 物品 tile → `/packs/:id/content?mode=index`
  - 配方 tile → `/packs/:id/content?mode=graph`
  - 魔改 tile → `/packs/:id/content?mode=edit`
  - 任务 tile → `/packs/:id/content?mode=quest`
- **冲突面板必须支持行级 resolve / ignore**（设计文档 §4.2 ⚠ + §5-4）。后端端点已存在：
  `POST /api/packs/{packId}/conflicts/{conflictId}/resolve` → `{"status":"resolved"}`
  `POST /api/packs/{packId}/conflicts/{conflictId}/ignore` → `{"status":"ignored"}`
  （出处 `apps/server/internal/httpapi/routes_mods.go` 的 conflicts 路由组）
  **先查 `apps/web2/src/api/mods.ts` 有没有对应 wrapper**：若没有，就在 `api/mods.ts` **追加**两个函数（这是本步唯一允许改 api 文件的情形，且只准追加、不准改已有导出），形状照现有 `post(...)` 的写法：
  ```ts
  export const resolveConflict = (packId: string, conflictId: string) =>
    post(`/api/packs/${encodeURIComponent(packId)}/conflicts/${encodeURIComponent(conflictId)}/resolve`, {}, z.object({status: z.string()}));
  export const ignoreConflict = (packId: string, conflictId: string) =>
    post(`/api/packs/${encodeURIComponent(packId)}/conflicts/${encodeURIComponent(conflictId)}/ignore`, {}, z.object({status: z.string()}));
  ```
  （`z` 与 `post` 的 import 按该文件既有写法补。）
  成功后必须调 `usePackSummary().refresh()`，让健康分、顶栏告警、交付闸门三处同时更新——这就是设计文档 §5-4 的判据。
- 「重新解析依赖」按钮调 `api/mods.ts` 的 `resolvePack`，成功后 `refresh()`。

### 6.4 `pages/ContentPage.tsx`（`/packs/:id/content`）

规格 = 设计文档 §4.3。**本步的核心**。

```tsx
import {lazy, Suspense} from 'react';
import {useSearchParams} from 'react-router-dom';
import {SourceRail} from './content/SourceRail';
import {ModeTabs} from './content/ModeTabs';
import {CONTENT_MODES, type ContentMode} from '../app/nav';
import {StubItemIndex, StubRecipeGraph, StubContentEdit, StubQuest} from '../stubs/...';

/* 四态一个焦点：mode 只从这里读一次，四个态组件都是零 props 或只接 packId/mode，
   焦点由它们自己用 useFocus() 读 —— 这样第三步替换叶子时容器一行都不用改。 */
export function ContentPage() {
  const [params] = useSearchParams();
  const mode = (params.get('mode') ?? 'index') as ContentMode;
  const known = CONTENT_MODES.some(m => m.mode === mode) ? mode : 'index';
  return (
    <div className="content-page">
      <SourceRail mode={known}/>
      <section className="cp-canvas">
        <ModeTabs value={known}/>
        <Suspense fallback={<div className="cp-loading">载入中…</div>}>
          {known === 'index' && <StubItemIndex/>}
          {known === 'graph' && <StubRecipeGraph/>}
          {known === 'edit' && <StubContentEdit/>}
          {known === 'quest' && <StubQuest/>}
        </Suspense>
      </section>
    </div>
  );
}
```

CSS：`.content-page {display:flex; gap:var(--mc-sp-sm); height:100%; min-height:0}`，`.cp-canvas {flex:1; min-width:0; display:flex; flex-direction:column}`。

`ModeTabs`：4 个按钮，点击 `setSearchParams(p => {p.set('mode', m); return p}, {replace:false})`（用 `replace:false` 让后退能在四态间走）。当前态高亮，右侧显示该态的快捷键提示。

**`SourceRail`**（设计文档 §3 末段 + §4.3 页内左列）：
- 默认折叠 56px 图标条，展开 260px；折叠态记 `localStorage['web2.srcrail.folded']`。
- `mode === 'quest'` 时**整体换成章节 rail 插槽**（不是与章节 rail 并存），本步放 `StubQuestChapters`（列出真实章节名）。
- 其余三态显示模组来源插槽：本步 `StubModRail`（列出 `api/mods.ts` 的 `listMods` 真实结果，每行：名称 + 启停开关 + 解析状态点 + 「贡献 N 物品」按钮）。
  「贡献 N 物品」点击 = `setSearchParams(p => {p.set('ns', modId); p.set('mode','index'); return p})` —— 这是设计文档 §3.1 里 `/mods` 与 `/items` 两条旧路由合并后的行为。
- `?rail=mods` 时自动展开并打开「添加模组」抽屉插槽（本步 `StubModSearch`：一个输入框 + 空结果提示；第三步接双平台并发搜索）。

### 6.5 `pages/DeliveryPage.tsx`（`/packs/:id/delivery`）

规格 = 设计文档 §4.4。**五段纵向，一屏走完**，不是两个页面拼起来。

```
① 检查     delivery-checks 列表 + 「重新检查」；有 error 级未解决冲突 → 构建按钮禁用 + 「去概览处理」链接（/packs/:id?panel=conflicts）
② 构建     导出目录（DirectoryPicker 插槽）· 版本号 + 渠道 · 「开始构建」· 产物清单（含下载）
③ 安装     选游戏目录 → 查已装版本 → 安装
④ 起窗     离线用户名 + 内存 + Java 路径 → 启动 → 实时日志
⑤ 发布     CurseForge / Modrinth + 轮询 + 重试
```

- `?step=install` → 挂载后滚动到第③段并高亮 2s。
- 本步每段都是插槽 + 素面呈现：`StubChecks`（列出 `api/releases.ts` 的 checks）、`StubBuild`（版本号输入 + 构建按钮 + 产物列表）、`StubInstall`、`StubLaunch`、`StubPublish`。
- **①的闸门联动必须在本步做**（设计文档 §4.4 ⚠：前端未反映后端 `assertPackBuildable` 闸门，作者点了构建才吃 409）。判定数据来自 `usePackSummary().pendingConflicts.length > 0`。
- **②的产物下载**：后端 `GET /api/packs/:id/artifacts/{aid}/download` 免令牌，所以直接渲染 `<a href={...} download>`，**不要**走 `api/http.ts` 的 fetch 封装（那会把二进制当 JSON 解析）。
- **禁做**：旧实现里"无检查记录时 POST 一个占位 check"这种假数据行为（设计文档 §4.4 ⚠）。没有记录就显示空态 + 「重新检查」按钮。

### 6.6 `pages/SettingsPage.tsx`（`/settings`）

系统信息 + 目录配置 + 令牌状态（**只显示"已配置/未配置"，永不显示令牌值**）。素面叶子 `StubSettings`。

---

## 7. 验收（分节跑，不绿不进下一节）

### 7.1 静态

```bash
cd /Volumes/Evo/code/mPackStation/apps/web2
npm run test          # tsc --noEmit，0 错误
npm run build         # 构建成功
```

### 7.2 结构（机器可查）

```bash
cd /Volumes/Evo/code/mPackStation
# A. 顶层路由仍是 6 条，没膨胀
grep -c "path: '" apps/web2/src/app/router.tsx                        # 期望 6

# B. 导航项数量：全局 2 + 包内 3
grep -c "scope: 'global'" apps/web2/src/app/nav.ts                    # 期望 2
grep -c "scope: 'pack'" apps/web2/src/app/nav.ts                      # 期望 3

# C. 没有同义焦点参数
grep -rnE "\?(focus|selected|sel|current)=" apps/web2/src || echo "无同义参数"
grep -rno "'item'\|'fid'\|'f'" apps/web2/src/app/useFocus.ts | head   # 焦点只在这一处读写

# D. 组件不许自己碰焦点参数（只有 useFocus 可以）
grep -rln "params.get('item')\|params.get('fid')" apps/web2/src | grep -v 'app/useFocus.ts' || echo "焦点读写已收口"

# E. 没有假 affordance 文案
grep -rniE "后续版本提供|敬请期待|coming soon|暂未实现" apps/web2/src || echo "无假 affordance 文案"

# F. 没有底边栏 / 左侧导航栏残留
grep -rniE "statusbar|status-bar|app-sider|sider-w" apps/web2/src || echo "无底栏/左栏"

# G. 素面叶子全部在 stubs/ 下且以 Stub 命名（第三步要能一次 grep 清零）
ls apps/web2/src/stubs/ | grep -vc '^Stub'                            # 期望 0

# H. 没有引用旧前端
grep -rn "apps/web/\|\.\./\.\./\.\./web/" apps/web2/src || echo "未引用旧前端"

# I. 令牌没被写进任何文件
grep -rniE "chain-token|X-MPack-Token:\s*['\"][A-Za-z0-9]" apps/web2 || echo "无硬编码令牌"

# J. 目录加载只有一套（旧实现的病根是有两套并行）
grep -rln 'getItemCatalog(' apps/web2/src | grep -v 'api/catalog.ts'   # 期望只输出 app/CatalogContext.tsx

# K. 迎新状态只有一套，且不是从包数据反推的
grep -rln 'fetchOnboarding(' apps/web2/src | grep -v 'api/onboarding.ts'  # 期望只输出 1 个文件（onboarding 的 context/提供者）
grep -rn 'prismAccount' apps/web2/src || echo "未使用已废弃的 prismAccount"
```

### 7.3 交互（浏览器里做，逐条截图或录屏留证）

环境：§6 的隔离环境（web2 在 5274，后端 18872）。

| # | 操作 | 判据 |
|---|---|---|
| 1 | 打开 `/` | 顶栏只有左段 + 右段，**中段不渲染**；无左栏、无底栏；页面高度撑满视口无空白条 |
| 2 | 数顶栏可点导航项 | 正好 5 项（工作台/整合包/概览/内容/交付）+ 1 个 ⚙ 图标 |
| 3 | 进 `/packs` → 打开一个包 | 顶栏中段出现，面包屑显示**包名**（不是"创作工具"这类分组标题），概览 Tab 高亮 |
| 4 | 点内容 Tab | URL 变 `/packs/:id/content`，默认索引态；点四态 Tab，URL 的 `mode` 跟着变，浏览器后退回上一态 |
| 5 | 索引态选一个物品 → 切关系 → 切魔改 → 切编排 → 切回索引 | 右栏焦点**始终是同一个物品**；URL 里 `item=` 全程不丢 |
| 6 | 在任意态刷新页面 | 态和焦点原样恢复（设计文档 §5-5 的判据） |
| 7 | 右栏焦点段点「去魔改」 | URL 变成 `?mode=edit&item=<同一个 id>`，**不是**跳到不带参数的整页 |
| 8 | 按 `E` / `Q` / `Enter`（索引态）/ `Esc` | 分别切到魔改/编排/关系/上一态；在输入框里敲这些键**不切态** |
| 9 | 按 ⌘K → 输入 → 选一个物品命令 | 面板关闭，焦点设成该物品，**当前 mode 不变**（不是硬跳某个页面） |
| 10 | 概览点「配方」tile | 落到 `/packs/:id/content?mode=graph` |
| 11 | 概览冲突面板点某条的「忽略」 | 该条消失或标记 ignored；健康分变化；顶栏告警数变化（三处同时更新，不需手动刷新） |
| 12 | 概览点「查看全部模组」 | 落到 `/content?rail=mods`，来源栏自动展开 |
| 13 | 来源栏点某模组的「贡献 N 物品」 | 落到 `?mode=index&ns=<modId>`，索引态只显示该模组的物品 |
| 14 | 折叠来源栏 → 刷新 | 仍是折叠态（localStorage 生效） |
| 15 | 切到编排态 | 来源栏**整体变成章节 rail**，不是两条并存 |
| 16 | 交付页 | 五段从上到下一屏；有待解决冲突时构建按钮禁用且给出「去概览处理」链接 |
| 17 | 点上手清单第 4 步 | 落到 `/delivery?step=install`，页面滚动到安装段并高亮 |
| 18 | 触发一个长任务后立刻切态切页再切回来 | 顶栏胶囊进度一直在（轮询在 context，不在页面） |
| 19 | 内容页四态来回切 8 次，看 Network | `dashboard`/`health`/`locks`/`conflicts` 各 1 次请求，不是 8 次 |
| 20 | 右栏包状况段点折叠 | 缩成一行（健康分 + 告警数），焦点段获得更多高度 |
| 21 | 上手清单：看 4 步的完成态，再对照 `GET /api/onboarding` 的 `steps` | 完全一致；`launcherReady=false` 时第 4 步必须显示未完成，**不许因为有包/有模组就打勾** |
| 22 | 上手清单点第 3 步 → 去 `/settings` 填 CurseForge Key 并保存 → 回来 | 第 3 步变已完成（走的是 `acknowledgeOnboarding({curseforgeKey:true})`，Network 里有 `PUT /api/onboarding`）；刷新后仍是已完成 |

### 7.4 完成标志

7.1 + 7.2 全绿，7.3 的 22 条逐条通过并留证。**任何一条不通过都不算第二步完成**，不要"先往下走回头再修"。

---

## 8. 禁止清单

- 禁打开 `apps/web/src/` 下铁律 1 列出的目录（规格看不懂就问，不要翻旧代码反推）
- 禁增加第 7 条顶层路由；禁把四态做成路由
- 禁新造 URL 参数名（`?focus=`、`?selected=`、`?src=`、`?tab=` 一律禁止）
- 禁在 `RightRail` / `TopBar` / `OverviewPage` 里自己发请求（一律走 `usePackSummary()`）
- 禁用 `position: sticky` + `height: 100vh` 做右栏（那是为已删除的底边栏做的补偿）
- 禁渲染没有真实数据支撑的按钮、卡片、Dropdown
- 禁把健康分公式复制到第二个文件
- 禁在 `stubs/` 之外写素面叶子（第三步要能一次删干净）
- 禁改 `api/` 已有导出（只准按 §6.3 追加 conflict 的两个 wrapper）
- 禁关 tsconfig 的 strict 系列开关
- 禁 `git add` / `git commit`

---

## 9. 交给第三步的插槽契约

第三步替换 `stubs/` 时，**容器一行都不该改**。为此本步结束时插槽接口必须固定如下（第三步按这个签名实现）：

| 插槽文件（本步） | 第三步替换为 | 必须遵守的签名 |
|---|---|---|
| `StubItemIndex` | `ItemIndexPanel` | `() => JSX`；自己用 `useFocus()` + `useSearchParams()` 读 `ns`/`type`/`item` |
| `StubRecipeGraph` | `RecipeGraphPanel` | `() => JSX`；自己读 `item` |
| `StubContentEdit` | `ContentEditPanel` | `() => JSX`；自己读 `doc`；apply 成功后必须调 `usePackSummary().refresh()` |
| `StubQuest` | `QuestPanel` | `() => JSX`；自己读 `node` |
| `StubQuestChapters` | `QuestChapterRail` | `() => JSX` |
| `StubModRail` | `ModSourceRail` | `() => JSX`；自己读 `ns`/`rail` |
| `StubModSearch` | `ModSearchDrawer` | `{open, onClose}` |
| `StubConflicts` | `ConflictList` | `() => JSX`；行级 resolve/ignore 后调 `refresh()` |
| `StubHealth` | `HealthPanel` | `() => JSX`；数据只从 `usePackSummary()` 取 |
| `StubFocusInspector` | `FocusInspector` | `() => JSX`；自己用 `useFocus()`；传送门必须带焦点参数 |
| `StubTasks` | `TaskPanel` | `{tasks: Task[]}` |
| `StubPackList` | `PackList` | `() => JSX` |
| `StubActivity` | `ActivityFeed` | `() => JSX` |
| `StubChecks`/`StubBuild`/`StubInstall`/`StubLaunch`/`StubPublish` | 交付五段组件 | `{packId: string}` |
| `StubSettings` | `SettingsPanel` | `() => JSX` |

**零 props 是刻意设计**：所有页面级状态都在 URL、`PackSummaryContext`、`CatalogContext` 里，叶子组件自己读（`useFocus()` / `useSearchParams()` / `usePackSummary()` / `useCatalog()`）。这样第三步可以逐个替换、逐个验收，不需要动容器，也不会出现"改一个叶子碰坏另一个"。

**第二步完成的标志**：§7 全绿 + §9 的插槽签名与实现一致（用 `grep -n 'export function Stub' apps/web2/src/stubs/*.tsx` 逐个核对）。
