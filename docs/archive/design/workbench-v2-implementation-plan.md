# 工作台 v2.1 施工方案（执行方专用）

> ## ⛔ 本文已作废（2026-10-01 用户拍板改走「新开」路线）
>
> 本文是**就地改造 `apps/web`** 的路线。用户明确要求改为**新开 `apps/web2`**、按「先起框架 → 再放容器 → 再塞组件」三步渐进重做，理由是就地改造容易被旧结构牵着走（上一轮实现跑偏的机制）。
>
> **现行权威方案**（三份，按顺序执行）：
> 1. [`web2-step1-framework.md`](./web2-step1-framework.md) — 框架层：构建配置 / 路由骨架 / API 客户端 / 设计令牌
> 2. [`web2-step2-shell-and-pages.md`](./web2-step2-shell-and-pages.md) — 主体层：外壳 + 页面容器 + 状态层（全新写，禁参考旧实现）
> 3. [`web2-step3-domain-widgets.md`](./web2-step3-domain-widgets.md) — 叶子层：领域组件（新写为主，用 1733 行 headless 测试当规格）
>
> 设计权威仍是 [`workbench-interaction-design.md`](./workbench-interaction-design.md) v2.1，三份新方案与它一致。
> 本文保留仅作**已核实事实的索引**：里面的行号清单、后端已就绪能力表、零 props 签名表、假 affordance 清单在新方案里被引用为"出处"。不要按本文施工。

> 设计依据：[`workbench-interaction-design.md`](./workbench-interaction-design.md)（v2.1，IA 的唯一权威）。
> 本文只讲**怎么改代码**：改哪个文件、改成什么、怎么验收。设计取舍不在本文讨论，有疑问先读设计稿。
> 目标读者是执行方（可能是能力较弱的模型）。因此本文给出**可直接粘贴的代码骨架**、**精确的文件与行号**、**每阶段的验收命令**。

---

## 0. 铁律（违反任意一条即视为失败，必须回滚重做）

1. **不许新增任何导航项。** 顶栏可点导航最终 = 5（工作台 / 整合包 / 概览 / 内容 / 交付）+ 1 个 ⚙ 图标按钮。做完数一遍，多一个就是错。
2. **不许改后端。** 本轮全部改动只在 `apps/web/`。后端契约已就绪，缺的只是前端 wrapper。若你认为必须改后端，**停下来报告**，不要动手。
3. **不许碰这些端口与数据目录**：`5173`、`18765`、`18766`、`18871`、`/tmp/mpack-data`。验证 UI 一律用隔离链路环境（见 §1.2）。
4. **不许留假按钮。** 任何「点了弹 `message.info('...后续版本提供')`」的东西，要么实现，要么删除。不许保留。
5. **不许造同义参数。** URL 参数名固定为 §3.2 那张表，`?item=` 和 `?ns=` 已有实现，沿用，不要另起 `src=` / `itemId=` 之类。
6. **一次只做一个阶段。** 每阶段做完必须跑完该阶段的验收命令并全绿，才能进下一阶段。不许「先全改完再一起验」。
7. **不许提交 git。** 不要 `git add` / `git commit` / `git push`，除非用户明确要求。
8. **删代码就删干净。** 不要留 `// removed`、不要把导出改成 `_unused`、不要保留死 CSS。

---

## 1. 环境与命令

### 1.1 静态检查（每阶段必跑，全绿才算完）

```bash
cd /Volumes/Evo/code/mPackStation/apps/web && npx tsc --noEmit && npx vite build
cd /Volumes/Evo/code/mPackStation/apps/server && go test ./... -count=1
```

### 1.2 起隔离环境看界面（每阶段必跑，肉眼 + 截图验收）

```bash
cd /Volumes/Evo/code/mPackStation
bash scripts/chain-test-run.sh /tmp/chain-run-v21.log      # 后端 18872/18873 + 数据 /tmp/mpack-chain
```

它会在末尾提示你前端没起。按提示另开一个终端起 vite（**端口只能 5273**）：

```bash
cd /Volumes/Evo/code/mPackStation/apps/web
TOK=$(cat /tmp/mpack-chain/runtime-token)
VITE_API_TARGET=http://127.0.0.1:18872 VITE_MPACK_TOKEN=$TOK npx vite --port 5273
```

浏览器开 `http://127.0.0.1:5273`。用完停掉：`bash scripts/dev-stop.sh` 只停开发实例；隔离实例按 `chain-test-run.sh` 末尾的 pkill 提示停。

### 1.3 全链路回归（P4、P6 结束后各跑一次）

```bash
cd /Volumes/Evo/code/mPackStation && bash scripts/chain-test-run.sh /tmp/chain-run-v21.log
```

基线：**173 例，PASS 172 / FAIL 0 / SKIP 1**。不许退化。

---

## 2. 已核实的事实（直接用，不要再猜、不要再找）

以下每一条都由我 grep 复核过。执行时若发现与此不符，**停下来报告**，不要自行发挥。

### 2.1 现状导航与路由

- `apps/web/src/app/AppShell.tsx:22-25` = `primaryNav`（工作台 / 整合包）；`:27-38` = `packNav`（**10 项**：概览 / 模组 / 物品 / 合成 / 依赖与冲突 / 模组内容 / 魔改 / 任务书 / 打包与发布 / 启动器）。
- `AppShell.tsx:59-61`：`useEffect(..., [location.pathname])` 每次路由变化重拉 `fetchDashboard()`。
- `AppShell.tsx:132`：`<InspectorRail packId={activePackId}/>`；`:166`：`<CommandPalette packId={activePackId}/>`。
- `AppShell.tsx:140-165`：底边栏 `<footer className="app-statusbar">`，高 116px。
- `AppShell.tsx:174-181`：`ModulePlaceholder`，**全仓零引用**。
- `apps/web/src/main.tsx:51-74`：11 条包内路由 + 7 条 `DefaultPackRedirect`。`:61` 是孤儿路由 `/packs/:id/content-doc`。

### 2.2 页面组件签名（关键：全部零 props）

所有页面组件都**不接收 props**，自己 `useParams()` 取 `id`、`useSearchParams()` 取查询参数。这意味着把它们挂到新路由 `/packs/:id/content` 下**无需改动即可工作**（参数名仍是 `id`）。

| 组件 | 文件:行 | 取参 |
|---|---|---|
| `PackWorkbenchPage` | `pages/PackPages.tsx:95` | `useParams().id` |
| `PackModsPage` | `pages/PackPages.tsx:224` | 同上 |
| `DependenciesPage` | `pages/PackPages.tsx:313` | 同上 |
| `ContentEditorPage` | `pages/PackPages.tsx:351` | 同上（**要删**） |
| `QuestEditorPage` | `pages/PackPages.tsx:378` | 同上 |
| `PublishPage` | `pages/PackPages.tsx:439` | 同上 |
| `SettingsPage` | `pages/PackPages.tsx:623` | 无 |
| `ItemCatalogPage` | `pages/ItemCatalogPage.tsx:16` | `useSearchParams().get('ns')`（`:25`） |
| `RecipeBrowserPage` | `pages/RecipeBrowserPage.tsx:17` | `useParams().id` + `useSearchParams().get('item')`（`:32`） |
| `RecipeTweakPage` | `pages/RecipeTweakPage.tsx:39` | `useParams().id` |
| `ModContentPage` | `pages/ModContentPage.tsx:45` | `useParams().id` |
| `LauncherPage` | `pages/LauncherPage.tsx:23` | `useParams().id` |

### 2.3 外壳原语（已存在，是接通而不是新建）

- `features/focus/FocusContext.tsx`：`FocusProvider`（`:33`）、`useFocus()`（`:54`）。焦点类型 `'item'|'recipe'|'mod'|'quest'|'tag'`（`:7`）。**内部是纯 `useState`（`:34`），刷新即丢焦**。
  - 写端 6 处，**读端只有 1 处**：`RecipeBrowserPage.tsx:27-29`。
  - `focusRecipe`（`:42`）**全仓零调用**；`InspectorRecipePreview`（`InspectorRail.tsx:156`）**零消费方**。
- `features/focus/InspectorRail.tsx`：`:25` `go(suffix)`、`:27-30` `focusItemAndGo`。**三个传送门按钮全不带焦点参数**：`:96`（查看合成关系，只带 itemId 但目标是旧路由）、`:99`（在物品页打开 → `go('/items')`，不带 item）、`:102`（去魔改 → `go('/tweak')`，不带 item）。
- `features/focus/CommandPalette.tsx`：`:45-55` 8 个页面命令全指向旧路由；`:75` 物品命中硬跳 `/recipes`。
- `features/pack/PackCatalogContext.tsx`：`PackCatalogProvider({packId, locale='zh_cn', children})`（`:32`），提供 `phase/items/itemById/tagById/recipesByOutput/recipesByInput/displayName/iconUrl/reload/rebuild/rebuilding`（`:14-28`）。**这是唯一的目录数据源**。
  - ⚠ `pages/ModContentPage.tsx:58-59,152-168` 自己又拉了一份 catalog，是并行的第二套加载逻辑，P2 要合并掉。

### 2.4 后端已就绪、前端缺 wrapper（本轮只需加前端）

| 能力 | 后端 | 前端现状 |
|---|---|---|
| 冲突逐条解决 | `POST /api/packs/{packId}/conflicts/{conflictId}/resolve` → `{"status":"resolved"}`（`routes_mods.go:140-146`） | `api/mods.ts` **无 wrapper** |
| 冲突逐条忽略 | `POST .../conflicts/{conflictId}/ignore` → `{"status":"ignored"}`（`routes_mods.go:147-153`） | `api/mods.ts` **无 wrapper** |
| 产物下载 | `GET /api/packs/{packId}/artifacts/{artifactId}/download`（`routes_publish.go:156`）。GET 免令牌（`httpapi.go:399-403,431-433`，只有 `/api/fs/browse` 例外） | `api/releases.ts` **无 wrapper**，页面无按钮 → 直接用 `<a href>` 即可，不需要 fetch |
| 发布 / 轮询 / 重试 | `publishPack` / `pollRelease` / `retryRelease` 已在 `api/releases.ts:80-85` | **全仓零 UI 调用** |
| 任务书回滚 / 历史 | `rollbackQuest` / `questHistory` 已在 `api/content.ts:154,156` | **全仓零 UI 调用** |
| 任务节点引用模组 | `quest_nodes.mod_refs` 已落库（`content_repo.go:347`），validate 会报 `cross_pack_reference` / `missing_mod_reference`（`content.go:842-844`），前端 zod 有 `modRefs`（`api/content.ts:106`） | **无编辑 UI** |

### 2.5 要删的假 affordance（逐条已核实）

- `features/dashboard/PackList.tsx:77-89`：重命名 / 复制 / 一键打包三个 Dropdown，点了只弹 `message.info('该功能将在后续版本提供')`（`:88`）。
- `features/dashboard/TaskPanel.tsx:56`：「全部任务」按钮，弹 `message.info('任务中心将在后续版本提供')`。
- `pages/PackPages.tsx:468`：`onRunChecks` 在无检查记录时 POST 一个 `{kind:'content',status:'passed',detail:'{}'}` 的**占位假数据**。
- `AppShell.tsx:150`：`<b className="app-status-pack-saved">已保存</b>` 硬编码。
- `AppShell.tsx:161-164`：「查看日志」「输出目录」两个按钮只是 `navigate('/')` / `navigate('/settings')`。
- `pages/ModContentPage.tsx:60,276-277,495-511`：`relationKey` 死胡同 Modal（点物品→标签→成员，出不去）。

### 2.6 设计令牌

全部在 `features/dashboard/dashboard.css:4-60` 的 `:root`。颜色/圆角/间距**只准引用变量**，不许写死色值。布局变量 `--mc-sider-w: 220px`（`:18`）、`--mc-sider-w-fold: 72px`（`:19`），窄屏重定义在 `:240-241`。左栏删除后这两个变量与它们的所有引用一并删除。

---

## 3. 目标形态（一图 + 一表）

### 3.1 外壳

```
┌────────────────────────────────────────────────────────────────────────┐
│ ◆ mPack   工作台  整合包 │ 包名 ›  概览  内容  交付 │  ⏳2 任务  🔍  ⚙    │ 顶栏 48px
├──────────────────────────────────────────────────┬─────────────────────┤
│                                                  │ ▣ 包状况（可折叠）    │
│                  <Outlet/>                       │  健康 82 · 告警 2    │
│              各页面自己的内容                       │  MC 1.21.1 · Fabric │
│                                                  │ ─────────────────── │
│                                                  │ ▣ 当前焦点           │
│                                                  │  详情 + 传送门按钮    │
└──────────────────────────────────────────────────┴─────────────────────┘
        ↑ 没有左侧栏，没有底边栏
```

内容页（`/packs/:id/content`）内部再多一列页内左列「来源栏」（P2 才做）：

```
┌────────┬───────────────────────────────────────────┬─────────────────┐
│ 来源栏  │  [索引][关系][魔改][编排]                    │ 右栏（应用级）    │
│ 56/260 │  该态的画布                                 │                 │
└────────┴───────────────────────────────────────────┴─────────────────┘
```

### 3.2 路由与 URL 参数（唯一权威，不许改名）

| 新路由 | 渲染 | 说明 |
|---|---|---|
| `/` | `DashboardPage` | 不变 |
| `/welcome` | `DashboardPage forceEmpty` | 不变 |
| `/packs` | `PacksPage` | 不变 |
| `/packs/:id` | `OverviewPage`（= 现 `PackWorkbenchPage` 改造） | 吸收依赖与冲突 |
| `/packs/:id/content` | `ContentPage`（**新建**） | 四态外壳 |
| `/packs/:id/delivery` | `DeliveryPage`（**新建**） | 五段流水线 |
| `/settings` | `SettingsPage` | 不变 |

| 查询参数 | 含义 | 谁读 |
|---|---|---|
| `mode` | `index` \| `graph` \| `edit` \| `quest` | `ContentPage` |
| `item` | 焦点物品 id（**沿用现有**） | `FocusContext`、`RecipeBrowserPage:32` |
| `f` + `fid` | 非物品焦点：类型 + id | `FocusContext` |
| `ns` | 命名空间 / 模组来源筛选（**沿用现有**） | `ItemCatalogPage:25` |
| `type` | 内容 kind 筛选（recipe/structure/ore/…） | 索引态（P2） |
| `rail` | `mods` = 展开来源栏模组抽屉 | `ContentPage`（P2） |
| `doc` | 内容文档 id | 写态（P2） |
| `node` | 任务节点 id | 编排态（P5） |
| `panel` | `conflicts` = 概览滚到冲突面板 | `OverviewPage`（P4） |
| `step` | `install` = 交付页滚到安装段 | `DeliveryPage`（P6） |

**旧路由重定向表**（一律 `<Navigate replace>` 并**保留原有查询参数**）：

| 旧 | 新 |
|---|---|
| `/packs/:id/dependencies` | `/packs/:id?panel=conflicts` |
| `/packs/:id/mods` | `/packs/:id/content?mode=index&rail=mods` |
| `/packs/:id/items` | `/packs/:id/content?mode=index` |
| `/packs/:id/recipes` | `/packs/:id/content?mode=graph` |
| `/packs/:id/content`（原「模组内容」页） | `/packs/:id/content?mode=index`（同一路由，由 `ContentPage` 接管） |
| `/packs/:id/tweak` | `/packs/:id/content?mode=edit` |
| `/packs/:id/quests` | `/packs/:id/content?mode=quest` |
| `/packs/:id/content-doc` | `/packs/:id/content?mode=edit`（原页面删除） |
| `/packs/:id/publish` | `/packs/:id/delivery` |
| `/packs/:id/launcher` | `/packs/:id/delivery?step=install` |
| `/mods` `/items` `/recipes` `/content` `/tweak` `/quests` `/publish`（无包前缀） | `DefaultPackRedirect` 到上表对应的新路径 |

---

## 4. P1 · 换外壳 + 收口导航

**范围**：只做外壳与路由。**不动任何业务组件的内部逻辑**。四个态在 P1 就是把现成页面组件原样挂进 `ContentPage`（会有重复的页头，这是预期的，P2 收拾）。

### 4.1 新建 `apps/web/src/app/PackSummaryContext.tsx`

底边栏删除后，包状况要搬到右栏；而概览页（P4）与右栏需要**同一份**健康/冲突数据，且冲突处理后要能一起刷新。所以把它提成 context，顺手解决 `AppShell.tsx:59-61`「每次路由变化重拉 dashboard」。

```tsx
import {createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode} from 'react';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';
import {packHealth, listConflicts, listLocks, type PackHealth} from '../api/mods';

/* 包级摘要：顶栏、右栏「包状况」、概览页共用同一份数据。
   按 packId 缓存，不再随路由变化重拉；写操作成功后由调用方 refresh() 主动刷新。 */

export type PackSummaryValue = {
  pack: DashboardPack | null;
  health: PackHealth | null;
  locks: number;
  pendingConflicts: number;
  score: number | null;
  alertCount: number;
  refresh: () => void;
};

const Ctx = createContext<PackSummaryValue | null>(null);

export function PackSummaryProvider({packId, children}: {packId: string; children: ReactNode}) {
  const [pack, setPack] = useState<DashboardPack | null>(null);
  const [health, setHealth] = useState<PackHealth | null>(null);
  const [locks, setLocks] = useState(0);
  const [pending, setPending] = useState(0);

  const refresh = useCallback(() => {
    if (!packId) { setPack(null); setHealth(null); setLocks(0); setPending(0); return; }
    void fetchDashboard().then(d => setPack(d.packs.find(p => p.id === packId) ?? null)).catch(() => setPack(null));
    void packHealth(packId).then(setHealth).catch(() => setHealth(null));
    void listLocks(packId).then(v => setLocks(v.length)).catch(() => setLocks(0));
    void listConflicts(packId).then(v => setPending(v.filter(c => c.status === 'pending').length)).catch(() => setPending(0));
  }, [packId]);

  useEffect(refresh, [refresh]);

  /* 健康分公式原先只活在 PackPages.tsx:188-190 的组件里；提到这里成为唯一出处。 */
  const score = health ? (health.healthy ? 100 : Math.max(0, 100 - health.pendingErrors * 20 - health.pendingWarnings * 5)) : null;
  const alertCount = pending + (pack?.alerts.crashes ?? 0) + (pack?.alerts.updatable ?? 0);

  const value = useMemo<PackSummaryValue>(
    () => ({pack, health, locks, pendingConflicts: pending, score, alertCount, refresh}),
    [pack, health, locks, pending, score, alertCount, refresh]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function usePackSummary(): PackSummaryValue {
  const v = useContext(Ctx);
  if (!v) throw new Error('usePackSummary 必须在 <PackSummaryProvider> 内使用');
  return v;
}
```

> `DashboardPack` 的字段（`api/dashboard.ts:7-21`）已确认包含上面用到的全部：`id / name / mcVersion / loader / packVersion / modCount{total,installed,selected} / conflicts{resolved,pending} / edits{...} / alerts{crashes,updatable} / lastEditedAt / createdAt`。`fetchDashboard()` 返回 `{packs, lastEditedPackId, todayResolvedCount}`。

### 4.2 新建 `apps/web/src/app/TopBar.tsx`

```tsx
import {NavLink} from 'react-router-dom';
import {Button, Drawer, Tooltip} from 'antd';
import {CodeSandboxOutlined, SettingOutlined, SearchOutlined, SyncOutlined} from '@ant-design/icons';
import {useEffect, useState} from 'react';
import {fetchTasks, type Task} from '../api/tasks';
import {TaskPanel} from '../features/dashboard/TaskPanel';
import {usePackSummary} from './PackSummaryContext';

/* 顶栏：全局导航 2 项 + 包面包屑与包内 3 Tab + 全局动作。
   横向空间有限，这是「导航不再膨胀」的物理约束 —— 不许加第 6 个导航项。 */

const globalNav = [
  {to: '/', label: '工作台', end: true},
  {to: '/packs', label: '整合包', end: false},
];

const packTabs = [
  {suffix: '', label: '概览', end: true},
  {suffix: '/content', label: '内容', end: false},
  {suffix: '/delivery', label: '交付', end: false},
];

export function TopBar({packId, onOpenPalette}: {packId: string; onOpenPalette: () => void}) {
  const {pack, alertCount, refresh} = usePackSummary();
  const [tasks, setTasks] = useState<Task[]>([]);
  const [drawer, setDrawer] = useState(false);

  const loadTasks = () => { void fetchTasks().then(setTasks).catch(() => setTasks([])); };
  useEffect(loadTasks, []);
  useEffect(() => {
    const t = window.setInterval(loadTasks, 5000);
    return () => window.clearInterval(t);
  }, []);

  const running = tasks.filter(t => t.status === 'queued' || t.status === 'running');
  const top = running[0];

  return (
    <>
      <header className="app-topbar">
        <NavLink to="/" className="app-topbar-brand" title="mPackStation">
          <CodeSandboxOutlined/><span className="app-topbar-brand-name">mPack</span>
        </NavLink>

        <nav className="app-topbar-global">
          {globalNav.map(n => (
            <NavLink key={n.to} to={n.to} end={n.end}
                     className={({isActive}) => isActive ? 'app-topnav app-topnav-active' : 'app-topnav'}>
              {n.label}
            </NavLink>
          ))}
        </nav>

        {Boolean(packId) && (
          <nav className="app-topbar-pack">
            <span className="app-topbar-sep" aria-hidden/>
            <NavLink to={`/packs/${packId}`} end className="app-topbar-crumb" title={pack?.name}>
              {pack?.name ?? '整合包'}
            </NavLink>
            <span className="app-topbar-crumb-sep" aria-hidden>›</span>
            {packTabs.map(t => (
              <NavLink key={t.suffix} to={`/packs/${packId}${t.suffix}`} end={t.end}
                       className={({isActive}) => isActive ? 'app-toptab app-toptab-active' : 'app-toptab'}>
                {t.label}
              </NavLink>
            ))}
          </nav>
        )}

        <span className="app-topbar-spacer"/>

        {alertCount > 0 && (
          <Tooltip title={`${alertCount} 个待处理告警`}>
            <button type="button" className="app-topbar-alert" onClick={() => refresh()}>
              {alertCount} 告警
            </button>
          </Tooltip>
        )}
        {running.length > 0 && (
          <Tooltip title={running.map(t => `${t.title} · ${t.progress}%`).join('\n')}>
            <button type="button" className="app-topbar-task" onClick={() => setDrawer(true)}>
              <SyncOutlined spin/> {running.length} 个任务{top ? ` · ${top.progress}%` : ''}
            </button>
          </Tooltip>
        )}
        <Tooltip title="搜索（⌘K）">
          <Button type="text" size="small" className="app-topbar-icon" icon={<SearchOutlined/>} onClick={onOpenPalette}/>
        </Tooltip>
        <Tooltip title="工作台设置">
          <NavLink to="/settings" className="app-topbar-icon-link"><SettingOutlined/></NavLink>
        </Tooltip>
      </header>

      <Drawer title="后台任务" open={drawer} onClose={() => setDrawer(false)} width={520}>
        <TaskPanel tasks={tasks} onChanged={loadTasks}/>
      </Drawer>
    </>
  );
}
```

> `Task` 的字段（`api/tasks.ts:7-18`）：`id / type / title / packId / packName / status / progress / error / startedAt / finishedAt`。`status` 是枚举 `'queued' | 'running' | 'success' | 'failed' | 'cancelled' | 'paused'`——**没有 `'pending'`**，别写错。`TaskPanel` 的 props 是 `{tasks, onChanged}`（`TaskPanel.tsx:36`）。

### 4.3 新建 `apps/web/src/features/focus/RightRail.tsx`

右栏两段：包状况（可折叠）+ 焦点 Inspector（复用现有 `InspectorRail`）。

```tsx
import {useState} from 'react';
import {Button, Tag, Tooltip} from 'antd';
import {DownOutlined, UpOutlined, WarningFilled} from '@ant-design/icons';
import {usePackSummary} from '../../app/PackSummaryContext';
import {InspectorRail} from './InspectorRail';

/* 右栏 = 包状况 + 当前焦点。底边栏删除后，包上下文的常驻展示落在这里。
   焦点内容多时包状况可折叠成一行，让位给 Inspector。 */

export function RightRail({packId}: {packId: string}) {
  const {pack, health, locks, pendingConflicts, score, alertCount} = usePackSummary();
  const [folded, setFolded] = useState(false);

  return (
    <aside className="right-rail">
      <section className="psc">
        <div className="psc-head">
          <span className={`psc-score tabular ${score === null ? '' : score >= 80 ? 'ok' : 'warn'}`}>
            {score === null ? '—' : score}
          </span>
          <div className="psc-title">
            <strong title={pack?.name}>{pack?.name ?? '未选择整合包'}</strong>
            <span>
              MC {pack?.mcVersion ?? '—'} · {pack?.loader ?? '—'} · v{pack?.packVersion ?? '—'}
            </span>
          </div>
          <Tooltip title={folded ? '展开包状况' : '折叠包状况'}>
            <Button type="text" size="small"
                    icon={folded ? <DownOutlined/> : <UpOutlined/>}
                    onClick={() => setFolded(v => !v)}/>
          </Tooltip>
        </div>
        {!folded && (
          <div className="psc-grid">
            <div><span>健康</span><b className="tabular">{health ? (health.healthy ? '健康' : '需关注') : '—'}</b></div>
            <div>
              <span>告警</span>
              <b className={`tabular ${alertCount ? 'danger' : ''}`}>
                {alertCount ? <><WarningFilled/> {alertCount}</> : 0}
              </b>
            </div>
            <div><span>模组</span><b className="tabular">{pack?.modCount.installed ?? 0}/{pack?.modCount.total ?? 0}</b></div>
            <div><span>待处理冲突</span><b className={`tabular ${pendingConflicts ? 'danger' : ''}`}>{pendingConflicts}</b></div>
            <div><span>锁定快照</span><b className="tabular">{locks}</b></div>
          </div>
        )}
      </section>
      <InspectorRail packId={packId}/>
    </aside>
  );
}
```

### 4.4 重写 `apps/web/src/app/AppShell.tsx`

整文件替换为：

```tsx
import {useEffect, useState} from 'react';
import {Outlet, useLocation, useNavigate} from 'react-router-dom';
import './shell.css';
import '../features/focus/focus.css';
import {fetchOnboarding, type Onboarding} from '../api/onboarding';
import {listPacks} from '../api/packs';
import {OnboardingChecklist} from '../features/dashboard/OnboardingChecklist';
import {FocusProvider} from '../features/focus/FocusContext';
import {PackCatalogProvider} from '../features/pack/PackCatalogContext';
import {PackSummaryProvider} from './PackSummaryContext';
import {TopBar} from './TopBar';
import {RightRail} from '../features/focus/RightRail';
import {CommandPalette} from '../features/focus/CommandPalette';

/* 应用外壳：顶栏导航 + 内容区 + 右栏（包状况 / 焦点）。没有左侧栏，没有底边栏。 */

export function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const [onboarding, setOnboarding] = useState<Onboarding | null>(null);
  const refreshOnboarding = () => { void fetchOnboarding().then(setOnboarding).catch(() => setOnboarding(null)); };
  useEffect(refreshOnboarding, []);

  const packId = location.pathname.match(/^\/packs\/([^/]+)/)?.[1];
  /* 不在包页面时，顶栏包导航落到第一个真实整合包；没有包则不渲染包段。 */
  const [firstPackId, setFirstPackId] = useState<string | null>(null);
  useEffect(() => {
    if (packId) return;
    void listPacks().then(ps => setFirstPackId(ps[0]?.id ?? '')).catch(() => setFirstPackId(''));
  }, [packId]);
  const activePackId = packId ?? firstPackId ?? '';

  const [paletteOpen, setPaletteOpen] = useState(false);

  return (
    <FocusProvider>
      <PackSummaryProvider packId={activePackId}>
        <PackCatalogProvider key={activePackId} packId={activePackId}>
          <div className="app-shell">
            <TopBar packId={activePackId} onOpenPalette={() => setPaletteOpen(true)}/>
            <div className="app-body">
              <main className="app-content"><Outlet/></main>
              {Boolean(activePackId) && <RightRail packId={activePackId}/>}
            </div>
            <OnboardingChecklist
              onboarding={onboarding}
              onSettings={() => navigate('/settings')}
              onRefresh={refreshOnboarding}
              onLauncher={() => navigate(activePackId ? `/packs/${activePackId}/delivery?step=install` : '/packs')}
            />
            <CommandPalette packId={activePackId} open={paletteOpen} onOpenChange={setPaletteOpen}/>
          </div>
        </PackCatalogProvider>
      </PackSummaryProvider>
    </FocusProvider>
  );
}
```

**删掉的东西**（原文件里全部移除，不留注释）：`primaryNav`、`packNav`、`sectionLabel`、`collapsed`/`setCollapsed`、`dashPacks`/`fetchDashboard` 的本地 state、`modTotal`/`modInstalled`/`alertCount` 的本地计算、整个 `<aside className="app-sider">`、`<button className="app-sider-fold">`、整个 `<footer className="app-statusbar">`、`ModulePlaceholder`。同时清理不再使用的 import（`DatabaseOutlined`、`CheckCircleOutlined`、`WarningOutlined`、`DoubleLeftOutlined`、`FolderOpenOutlined`、`FileTextOutlined` 等）。

### 4.5 改 `apps/web/src/features/focus/CommandPalette.tsx`

1. 签名改为受控：`export function CommandPalette({packId, open, onOpenChange}: {packId: string; open: boolean; onOpenChange: (v: boolean) => void})`。删掉内部 `const [open, setOpen] = useState(false)`，所有 `setOpen(x)` 换成 `onOpenChange(x)`；keydown 监听保留（`:26-37`），`Escape` 与 `⌘K` 都调 `onOpenChange`。
2. `pageCmds`（`:45-55`）整体替换为 7 条，全部指向新路由：

```tsx
const pageCmds: Cmd[] = useMemo(() => [
  {key: 'nav-overview', label: '概览', icon: <HomeOutlined/>, run: () => go('')},
  {key: 'nav-index', label: '内容 · 索引（物品）', icon: <GoldOutlined/>, run: () => go('/content?mode=index')},
  {key: 'nav-graph', label: '内容 · 关系（合成）', icon: <ExperimentOutlined/>, run: () => go('/content?mode=graph')},
  {key: 'nav-edit', label: '内容 · 魔改', icon: <EditOutlined/>, run: () => go('/content?mode=edit')},
  {key: 'nav-quest', label: '内容 · 编排（任务书）', icon: <BookOutlined/>, run: () => go('/content?mode=quest')},
  {key: 'nav-delivery', label: '交付', icon: <RocketOutlined/>, run: () => go('/delivery')},
  {key: 'nav-settings', label: '工作台设置', icon: <SettingOutlined/>, run: () => { onOpenChange(false); navigate('/settings'); }},
  // eslint-disable-next-line react-hooks/exhaustive-deps
], [packId]);
```

（`go` 保持 `:43` 的实现；需补 `SettingOutlined` import，删掉不再用的 `AppstoreOutlined`。）

3. 物品命中（`:68-76` 的 `pick`）：不再硬跳 `/recipes`，改为**保留当前 mode**：

```tsx
const it = row.item;
onOpenChange(false);
focusItem(it, '命令面板');
const mode = new URLSearchParams(window.location.search).get('mode');
navigate(`/packs/${packId}/content?mode=${mode ?? 'graph'}&item=${encodeURIComponent(it)}`);
```

### 4.6 改 `apps/web/src/features/focus/FocusContext.tsx`：焦点进 URL

`FocusProvider` 内部改为以 URL 为事实源（`useState` 只保留 `open`）：

```tsx
import {createContext, useCallback, useContext, useMemo, useState, type ReactNode} from 'react';
import {useSearchParams} from 'react-router-dom';

/* 焦点的单一事实源是 URL：?item=<id>（物品）或 ?f=<type>&fid=<id>（其他类型）。
   刷新、分享链接、浏览器前进后退都能原样恢复焦点；context 只是它的内存视图。 */

export function FocusProvider({children}: {children: ReactNode}) {
  const [params, setParams] = useSearchParams();
  const [open, setOpen] = useState(true);

  const focus = useMemo<Focus | null>(() => {
    const item = params.get('item');
    if (item) return {type: 'item', id: item};
    const t = params.get('f');
    const id = params.get('fid');
    if (t && id && (['item', 'recipe', 'mod', 'quest', 'tag'] as string[]).includes(t)) {
      return {type: t as FocusType, id};
    }
    return null;
  }, [params]);

  const setFocus = useCallback((f: Focus | null) => {
    setParams(prev => {
      const next = new URLSearchParams(prev);
      next.delete('item'); next.delete('f'); next.delete('fid');
      if (f) {
        if (f.type === 'item') next.set('item', f.id);
        else { next.set('f', f.type); next.set('fid', f.id); }
      }
      return next;
    }, {replace: true});
    if (f) setOpen(true);
  }, [setParams]);

  const focusItem = useCallback((id: string) => setFocus({type: 'item', id}), [setFocus]);
  const focusRecipe = useCallback((id: string) => setFocus({type: 'recipe', id}), [setFocus]);
  const focusMod = useCallback((id: string) => setFocus({type: 'mod', id}), [setFocus]);
  const clear = useCallback(() => setFocus(null), [setFocus]);
  const toggle = useCallback(() => setOpen(v => !v), []);

  const value = useMemo<FocusValue>(
    () => ({focus, setFocus, focusItem, focusRecipe, focusMod, clear, open, setOpen, toggle}),
    [focus, setFocus, focusItem, focusRecipe, focusMod, clear, open, toggle]);
  return <FocusContext.Provider value={value}>{children}</FocusContext.Provider>;
}
```

配套改动：

- `Focus` 类型（`:9-16`）的 `label` / `from` 两个字段**删掉**（URL 里不存展示名，消费方一律从 `usePackCatalog().displayName(id)` 取；`from` 只是装饰，删）。
- `focusItem` / `focusRecipe` / `focusMod` 的第二个参数（`from`）随之删除。**全仓所有调用点都要改**：`grep -rn "focusItem(\|focusMod(\|focusRecipe(" apps/web/src`，把 `'物品页'`、`'合成器'`、`'Inspector'`、`'概览'`、`'模组页'`、`'命令面板'`、`'深链'` 这些第二参数删掉。
- `InspectorRail.tsx:138,145` 用 `focus.label ?? focus.id` 的地方改为从 catalog 取名（`displayName(focus.id)`）。
- `recipe` 焦点从此可用：`focusRecipe` 之前零调用，P3 会在关系态接上。

### 4.7 新建 `apps/web/src/pages/ContentPage.tsx`

```tsx
import {lazy, Suspense} from 'react';
import {useSearchParams} from 'react-router-dom';
import {Spin} from 'antd';
import {ItemCatalogPage} from './ItemCatalogPage';
import {RecipeBrowserPage} from './RecipeBrowserPage';
import {RecipeTweakPage} from './RecipeTweakPage';
import {QuestEditorPage} from './PackPages';

/* 内容页：四态一个焦点。态写在 URL 的 ?mode=，切态不换页、不丢焦点。
   P1 阶段四态直接复用现成页面组件（它们自带页头，看起来会有点重复），
   P2 才拆掉内层页头、加入页内左列「来源栏」。 */

const MODES = ['index', 'graph', 'edit', 'quest'] as const;
type Mode = typeof MODES[number];

const ModeTabs = ({mode, setMode}: {mode: Mode; setMode: (m: Mode) => void}) => (
  <nav className="content-modes">
    {([['index', '索引'], ['graph', '关系'], ['edit', '魔改'], ['quest', '编排']] as [Mode, string][]).map(([k, label]) => (
      <button key={k} type="button"
              className={k === mode ? 'content-mode content-mode-active' : 'content-mode'}
              onClick={() => setMode(k)}>
        {label}
      </button>
    ))}
  </nav>
);

export function ContentPage() {
  const [params, setParams] = useSearchParams();
  const raw = params.get('mode') ?? '';
  const mode: Mode = (MODES as readonly string[]).includes(raw) ? (raw as Mode) : 'index';
  const setMode = (m: Mode) => setParams(prev => {
    const next = new URLSearchParams(prev);
    next.set('mode', m);
    return next;
  }, {replace: true});

  return (
    <div className="content-page">
      <ModeTabs mode={mode} setMode={setMode}/>
      <Suspense fallback={<div className="content-loading"><Spin/></div>}>
        {mode === 'index' && <ItemCatalogPage/>}
        {mode === 'graph' && <RecipeBrowserPage/>}
        {mode === 'edit' && <RecipeTweakPage/>}
        {mode === 'quest' && <QuestEditorPage/>}
      </Suspense>
    </div>
  );
}
```

> `lazy` 在 P1 可以不真用（四个组件直接 import 即可，`Suspense` 留着）。P2 若单页体积成为问题，再把四个 import 换成 `lazy(() => import('./ItemCatalogPage'))` 形式——那时需要给这四个文件补 `export default`。**P1 不要做这件事**，避免无谓改动。

### 4.8 新建 `apps/web/src/pages/DeliveryPage.tsx`

P1 只做最小接管：按 `?step` 决定渲染哪一段。P6 才真正合并成五段流水线。

```tsx
import {useSearchParams} from 'react-router-dom';
import {PublishPage} from './PackPages';
import {LauncherPage} from './LauncherPage';

/* 交付页：检查 → 构建 → 下载 → 安装 → 起窗 → 发布，一屏走完。
   P1 先按 ?step 分别挂现有的发布页与启动器页；P6 把两者合并成五段流水线。 */

export function DeliveryPage() {
  const [params] = useSearchParams();
  return params.get('step') === 'install' ? <LauncherPage/> : <PublishPage/>;
}
```

### 4.9 重写 `apps/web/src/main.tsx` 的路由表

把 `<Routes>` 内部（`:51-75`）替换为：

```tsx
<Route element={<AppShell/>}>
  <Route path="/" element={<DashboardPage/>}/>
  <Route path="/welcome" element={<DashboardPage forceEmpty/>}/>
  <Route path="/packs" element={<PacksPage/>}/>
  <Route path="/packs/:id" element={<PackWorkbenchPage/>}/>
  <Route path="/packs/:id/content" element={<ContentPage/>}/>
  <Route path="/packs/:id/delivery" element={<DeliveryPage/>}/>
  <Route path="/settings" element={<SettingsPage/>}/>

  {/* 旧路由一律带参重定向，保留原有查询参数，不产生 404 */}
  <Route path="/packs/:id/dependencies" element={<LegacyRedirect to="" search={{panel: 'conflicts'}}/>}/>
  <Route path="/packs/:id/mods" element={<LegacyRedirect to="/content" search={{mode: 'index', rail: 'mods'}}/>}/>
  <Route path="/packs/:id/items" element={<LegacyRedirect to="/content" search={{mode: 'index'}}/>}/>
  <Route path="/packs/:id/recipes" element={<LegacyRedirect to="/content" search={{mode: 'graph'}}/>}/>
  <Route path="/packs/:id/tweak" element={<LegacyRedirect to="/content" search={{mode: 'edit'}}/>}/>
  <Route path="/packs/:id/quests" element={<LegacyRedirect to="/content" search={{mode: 'quest'}}/>}/>
  <Route path="/packs/:id/content-doc" element={<LegacyRedirect to="/content" search={{mode: 'edit'}}/>}/>
  <Route path="/packs/:id/publish" element={<LegacyRedirect to="/delivery"/>}/>
  <Route path="/packs/:id/launcher" element={<LegacyRedirect to="/delivery" search={{step: 'install'}}/>}/>

  {/* 无包前缀的旧入口：落到第一个真实包的新路径 */}
  <Route path="/mods" element={<DefaultPackRedirect suffix="/content" search={{mode: 'index', rail: 'mods'}}/>}/>
  <Route path="/items" element={<DefaultPackRedirect suffix="/content" search={{mode: 'index'}}/>}/>
  <Route path="/recipes" element={<DefaultPackRedirect suffix="/content" search={{mode: 'graph'}}/>}/>
  <Route path="/content" element={<DefaultPackRedirect suffix="/content" search={{mode: 'index'}}/>}/>
  <Route path="/tweak" element={<DefaultPackRedirect suffix="/content" search={{mode: 'edit'}}/>}/>
  <Route path="/quests" element={<DefaultPackRedirect suffix="/content" search={{mode: 'quest'}}/>}/>
  <Route path="/publish" element={<DefaultPackRedirect suffix="/delivery"/>}/>
  <Route path="/launcher" element={<DefaultPackRedirect suffix="/delivery" search={{step: 'install'}}/>}/>
  <Route path="*" element={<Navigate replace to="/"/>}/>
</Route>
```

并在同文件加两个组件（`DefaultPackRedirect` 是改造现有的 `:21-29`）：

```tsx
type Search = Record<string, string>;

const withSearch = (base: string, search: Search = {}, prev?: URLSearchParams) => {
  const q = new URLSearchParams(prev);
  for (const [k, v] of Object.entries(search)) q.set(k, v);
  const s = q.toString();
  return s ? `${base}?${s}` : base;
};

/* 旧路由 → 新路由，保留原有查询参数（?item= / ?ns= 等深链不能断）。 */
function LegacyRedirect({to, search = {}}: {to: string; search?: Search}) {
  const {id = ''} = useParams();
  const [params] = useSearchParams();
  return <Navigate replace to={withSearch(`/packs/${encodeURIComponent(id)}${to}`, search, params)}/>;
}

/* 无包上下文的旧入口：落到第一个真实整合包；一个包都没有就回列表页引导创建。 */
function DefaultPackRedirect({suffix, search = {}}: {suffix: string; search?: Search}) {
  const [params] = useSearchParams();
  const [to, setTo] = useState<string | null>(null);
  useEffect(() => {
    void listPacks()
      .then(ps => setTo(ps[0] ? withSearch(`/packs/${ps[0].id}${suffix}`, search, params) : '/packs'))
      .catch(() => setTo('/packs'));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [suffix]);
  return to ? <Navigate replace to={to}/> : null;
}
```

import 需补：`useParams`、`useSearchParams`（来自 `react-router-dom`）、`ContentPage`、`DeliveryPage`；**删掉** `DependenciesPage`、`PackModsPage`、`PublishPage`、`QuestEditorPage`、`ContentEditorPage`、`LauncherPage`、`ModContentPage`、`ItemCatalogPage`、`RecipeBrowserPage`、`RecipeTweakPage` 的直接 import（它们改由 `ContentPage` / `DeliveryPage` 内部引用）。保留 `PackWorkbenchPage`、`PacksPage`、`SettingsPage`、`DashboardPage`。

### 4.10 删孤儿：`ContentEditorPage`

- 删 `pages/PackPages.tsx:351-376` 整个 `ContentEditorPage` 函数。
- `pages/PackPages.tsx:20` 的 `import {useContentEditor, useQuestBook} from '../hooks/useEditors';` **改成只留 `useQuestBook`**（同行还有别的用途，不能整行删）。
- 删 `hooks/useEditors.ts:49-61` 的 `useContentEditor` 定义（删前先 `grep -rn "useContentEditor" apps/web/src` 确认只剩这两处）。
- 若 `useContentEditor` 用到的 api 函数因此变成零引用，一并删除。

### 4.11 删 `ModulePlaceholder`

`AppShell.tsx:174-181`（4.4 已随整文件重写删掉）。确认 `grep -rn "ModulePlaceholder" apps/web/src` = 0。

### 4.12 删假 affordance

- `features/dashboard/PackList.tsx:77-89`：删掉重命名 / 复制 / 一键打包三个 Dropdown 与 `message.info('该功能将在后续版本提供')`。若删完 Dropdown 的 import 不再使用，一并清掉。
- `features/dashboard/TaskPanel.tsx:56`：删「全部任务」按钮与其 `message.info`。
- `pages/PackPages.tsx:468`：`onRunChecks` 里的 `checks.length ? checks : [{kind:'content',status:'passed',detail:'{}'}]` 改为**只传真实 checks**；`checks.length === 0` 时不再造假数据，改为提示「暂无检查记录」并禁用按钮（保留 `:564` 的空态文案）。

### 4.13 CSS：`apps/web/src/app/shell.css`

**删除**：`.app-sider`（`:6-15`）、`.app-shell-collapsed *`（`:16`、`:59-67`、`:71`）、`.app-brand*`（`:18-28`，样式移进顶栏品牌）、`.app-workbench-settings`（`:29-31`）、`.app-nav*`（`:33-48`）、`.app-sider-fold`（`:50-57`）、`.inspector-rail` 的粘性与视口高度规则（`:76-78`，改由 `.right-rail` 承担）、`.app-statusbar` 与全部 `.app-status-*`（`:81-131`）、`:87-96` 那个针对侧栏的 `@media (max-width: 820px)` 块。

**改写** `.app-shell` / `.app-body` / `.app-content`：

```css
.app-shell { display: flex; flex-direction: column; min-height: 100vh; background: var(--mc-bg); }
.app-body { flex: 1; min-width: 0; display: flex; align-items: flex-start; }
.app-content {
  flex: 1; min-width: 0; position: relative; box-sizing: border-box;
  min-height: calc(100vh - var(--mc-topbar-h));
  padding: var(--mc-sp-lg);
  /* 背景图层保持原样，从原 .app-content 规则里照搬，只把 padding-bottom: 136px 去掉 */
}
```

**新增顶栏**：

```css
.app-topbar {
  position: sticky; top: 0; z-index: 120;
  display: flex; align-items: center; gap: var(--mc-sp-xs);
  height: var(--mc-topbar-h); padding: 0 var(--mc-sp);
  background: color-mix(in srgb, var(--mc-fill) 92%, transparent);
  border-bottom: 1px solid var(--mc-line);
  backdrop-filter: blur(8px);
}
.app-topbar-brand { display: inline-flex; align-items: center; gap: 8px; color: var(--mc-primary); font-size: 16px; text-decoration: none; }
.app-topbar-brand-name { font-weight: 800; letter-spacing: -.03em; color: var(--mc-text); font-size: 15px; }
.app-topbar-global, .app-topbar-pack { display: flex; align-items: center; gap: 2px; }
.app-topbar-sep { width: 1px; height: 20px; margin: 0 6px; background: var(--mc-line); }
.app-topnav { padding: 6px 12px; border-radius: var(--mc-radius-pill); color: var(--mc-text-2); font-size: 13px; font-weight: 600; text-decoration: none; }
.app-topnav:hover { background: var(--mc-hover); color: var(--mc-primary); }
.app-topnav-active, .app-topnav-active:hover { background: var(--mc-primary-bg); color: var(--mc-primary); }
.app-topbar-crumb { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 6px 10px; color: var(--mc-text); font-size: 13px; font-weight: 700; text-decoration: none; }
.app-topbar-crumb-sep { color: var(--mc-muted); font-size: 13px; }
.app-toptab { padding: 6px 12px; border-radius: var(--mc-radius-pill); color: var(--mc-text-2); font-size: 13px; font-weight: 600; text-decoration: none; }
.app-toptab:hover { background: var(--mc-hover); color: var(--mc-primary); }
.app-toptab-active, .app-toptab-active:hover { background: var(--mc-primary); color: #fffaf0; }
.app-topbar-spacer { flex: 1; }
.app-topbar-alert { padding: 4px 10px; border: 1px solid var(--mc-fail); border-radius: var(--mc-radius-pill); background: var(--mc-fail-bg); color: var(--mc-fail); font-size: 12px; font-weight: 700; cursor: pointer; }
.app-topbar-task { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border: 1px solid var(--mc-line); border-radius: var(--mc-radius-pill); background: var(--mc-fill); color: var(--mc-text-2); font-size: 12px; cursor: pointer; }
.app-topbar-icon, .app-topbar-icon-link { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 8px; color: var(--mc-text-2); text-decoration: none; }
.app-topbar-icon:hover, .app-topbar-icon-link:hover { background: var(--mc-hover); color: var(--mc-primary); }
@media (max-width: 1100px) { .app-topbar-crumb { max-width: 110px; } }
@media (max-width: 820px) { .app-topbar-brand-name, .app-topbar-crumb { display: none; } }
```

**新增内容页四态 Tab（P1 先用最简版，P2 再细化）**：

```css
.content-page { display: flex; flex-direction: column; gap: var(--mc-sp); }
.content-modes { display: inline-flex; gap: 2px; padding: 3px; border: 1px solid var(--mc-line); border-radius: var(--mc-radius-pill); background: var(--mc-fill); width: fit-content; }
.content-mode { padding: 5px 14px; border: 0; border-radius: var(--mc-radius-pill); background: transparent; color: var(--mc-text-2); font-size: 13px; font-weight: 600; cursor: pointer; }
.content-mode:hover { color: var(--mc-primary); }
.content-mode-active, .content-mode-active:hover { background: var(--mc-primary-bg); color: var(--mc-primary); }
.content-loading { display: grid; place-items: center; padding: 48px; }
```

**新增右栏容器**（并把 `focus.css` 里 `.inspector-rail` 的 sticky 移到这里）：

```css
.right-rail { position: sticky; top: var(--mc-topbar-h); height: calc(100vh - var(--mc-topbar-h)); box-sizing: border-box; display: flex; flex-direction: column; width: 320px; flex: none; overflow-y: auto; border-left: 1px solid var(--mc-line); background: var(--mc-fill); z-index: 50; }
.psc { padding: var(--mc-sp); border-bottom: 1px solid var(--mc-line); }
.psc-head { display: flex; align-items: center; gap: 10px; }
.psc-score { display: grid; place-items: center; width: 44px; height: 44px; flex: none; border-radius: 50%; font-size: 17px; font-weight: 800; background: var(--mc-primary-bg); color: var(--mc-primary); }
.psc-score.ok { background: var(--mc-success-bg); color: var(--mc-success); }
.psc-score.warn { background: var(--mc-gold-bg); color: var(--mc-gold); }
.psc-title { flex: 1; min-width: 0; }
.psc-title strong { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.psc-title span { display: block; margin-top: 2px; color: var(--mc-muted); font-size: 11px; }
.psc-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 12px; margin-top: 12px; }
.psc-grid > div { display: flex; flex-direction: column; gap: 2px; }
.psc-grid span { color: var(--mc-muted); font-size: 11px; }
.psc-grid b { font-size: 13px; }
.psc-grid b.danger { color: var(--mc-fail); }
@media (max-width: 1180px) { .right-rail { width: 280px; } }
@media (max-width: 980px) { .right-rail { display: none; } }
```

**改 `features/focus/focus.css`**：`.inspector-rail`（`:4-13`）现在是 `width:320px; flex:none; height:100%; overflow:hidden; border-left:1px solid`。宽度、边框、粘性定位都改由 `.right-rail` 承担，所以把它改成：

```css
.inspector-rail { flex: 1; min-height: 0; display: flex; flex-direction: column; overflow: hidden; }
```

（去掉 `width` / `flex:none` / `border-left` / `background`；`height:100%` 换成 `flex:1; min-height:0`。）
`.inspector-rail-collapsed`（`:87-101`）同样去掉宽度与高度约束，只保留外观。
注意：sticky / `height:100vh` / `padding-bottom:116px` 这三样在 **`shell.css:76`**（不在 focus.css），随 4.13 的删除清单一起删掉。

**改 `features/dashboard/dashboard.css`**：删 `--mc-sider-w`（`:18`）、`--mc-sider-w-fold`（`:19`）与 `:240-241` 的窄屏重定义，在 `:root` 里加 `--mc-topbar-h: 48px;`。删 `:397` 附近那段关于 `.app-statusbar` 的注释与任何依赖它的布局规则（先 `grep -n "app-statusbar\|mc-sider-w" apps/web/src` 确认所有引用点都清了）。

### 4.14 改页面内的旧路由跳转

`grep -rn "/mods\`\|/items\`\|/recipes\`\|/tweak\`\|/quests\`\|/publish\`\|/launcher\`\|/dependencies\`\|/content-doc" apps/web/src` 找出所有 `navigate()` 与 `<NavLink>`，逐条改成新路径。已知必改：

- `PackPages.tsx:117` → `` navigate(`/packs/${id}/delivery`) ``
- `PackPages.tsx:127,140,144` → `` navigate(`/packs/${id}/content?mode=index&rail=mods`) ``
- `PackPages.tsx:161-172`（`CatalogSignalCard` 四个 tile）→ `/content?mode=index`、`/content?mode=graph`、`/content?mode=edit`、`/content?mode=quest`
- `PackPages.tsx:208` → 改为**就地展开冲突面板**（P4 做），P1 先改成 `` navigate(`/packs/${id}?panel=conflicts`) ``
- `PackPages.tsx:245` → `` navigate(`/packs/${id}/content?mode=index&ns=${encodeURIComponent(modId)}`) ``
- `RecipeBrowserPage.tsx:140` → `` navigate(`/packs/${id}/content?mode=index&item=${encodeURIComponent(selected.id)}`) ``
- `InspectorRail.tsx:96,99,102,122,132,139,146` → 全部带焦点参数，见 §6（P3）。**P1 阶段至少把路径改对**：`/content?mode=graph&item=<id>`、`/content?mode=index&item=<id>`、`/content?mode=edit&item=<id>`、`/content?mode=quest`。
- `features/dashboard/OnboardingChecklist.tsx`：第 4 步指向 `/delivery`（`AppShell.tsx` 的 `onLauncher` 已在 4.4 改好）。
- `features/dashboard/ContinueCard.tsx`、`PackList.tsx`、`DashboardPage.tsx` 里所有跳包内页的地方同样改。

### 4.15 P1 验收（全绿才算完，把输出贴给用户）

```bash
cd /Volumes/Evo/code/mPackStation
# 1. 静态检查
cd apps/web && npx tsc --noEmit && npx vite build && cd ../..
cd apps/server && go test ./... -count=1 && cd ../..
# 2. 结构判据
grep -rn "app-statusbar\|app-sider\|ModulePlaceholder\|ContentEditorPage\|useContentEditor" apps/web/src | wc -l   # 必须 0
grep -rn "后续版本提供\|敬请期待" apps/web/src | wc -l                                                              # 必须 0
grep -c "label: '" apps/web/src/app/TopBar.tsx                                                                      # globalNav 2 条
# 3. 顶栏导航项数（人肉数 + 截图）：可点导航 = 5，包内 Tab = 3，无底边栏
```

**点击级验收（起 §1.2 的隔离环境，逐条截图）**：

1. 打开 `http://127.0.0.1:5273`：顶栏只有「工作台 / 整合包」，没有包段，没有底边栏，没有左侧栏。
2. 进一个包：顶栏出现「包名 › 概览 内容 交付」，右栏出现「包状况 + 当前焦点」两段。
3. 逐条访问 11 个旧路由，确认落到正确新位置**且原有 `?ns=` / `?item=` 参数保留**：
   `/packs/<id>/dependencies`、`/mods`、`/items`、`/recipes`、`/tweak`、`/quests`、`/content-doc`、`/publish`、`/launcher`，以及无包前缀的 `/items`、`/recipes`。
4. 在内容页点「关系」→ 点一个物品 → **刷新页面**：仍在关系态、焦点仍是那个物品、右栏仍显示它。（这是 P1 最重要的判据。）
5. 关系态点配方里的材料能继续下钻，右栏焦点跟着变，URL 的 `?item=` 跟着变。
6. `⌘K` 打开命令面板：7 条页面命令全部指向新路由；搜一个物品回车，落在内容页且焦点正确。
7. 底边栏的「查看日志」「输出目录」「已保存」在界面上彻底消失。
8. 全链路回归不退化：`bash scripts/chain-test-run.sh /tmp/chain-run-v21.log` → `PASS 172 / FAIL 0 / SKIP 1`（P1 不动后端，链路测试应完全不受影响；若退化说明你改错了）。

---

## 5. P2 · 内容页四态合一

**目标**：四态共用一个外壳、一个焦点、一份目录数据；来源栏吸收原「模组」+「模组内容」两页。

### 5.1 拆内层页头

四个态组件（`ItemCatalogPage` / `RecipeBrowserPage` / `RecipeTweakPage` / `QuestEditorPage`）各自的 `.page-heading`（含 `<h1>` 与「刷新 / 重建目录」按钮）**移出组件**，改由 `ContentPage` 统一渲染一行工具条：左侧是态标题，右侧是「刷新 / 重建目录」（数据来自 `usePackCatalog()` 的 `reload` / `rebuild` / `rebuilding`）。写态与编排态各自的「校验 / 应用 / 修订号」保留在组件内（它们是态专属动作）。

### 5.2 新建页内左列「来源栏」`features/content/SourceRail.tsx`

内容：包内模组清单 + 启停 + 钉版 + 依赖 + 解析状态 + 「搜模组」抽屉入口 + 「重建目录」+ 本地 jar。全部从两处搬：

- `PackPages.tsx:224-311`（`PackModsPage`）：搜索区（`:256-281`）、推荐区（`:282-290`）、已安装区（`:291-309`）。搜索与推荐**放进抽屉**（`rail=mods` 时打开），已安装清单常驻来源栏。
- `ModContentPage.tsx:216-245`（开始/重新解析 + 轮询 run）、`:257-274`（重建目录 + 轮询 status）、`:331-345`（解析状态与指标）。

宽度：默认折叠 56px 图标条，展开 260px；折叠状态存 `localStorage['mpack.sourceRail.folded']`。编排态时整个来源栏**换成章节 rail**（`QuestBookEditor.tsx:678-711` 已经自带章节 rail，此时来源栏不渲染，避免两套左列）。

### 5.3 合并重复的 catalog 加载

`ModContentPage.tsx:58-59,152-168` 那份自行 `getItemCatalog` 的逻辑删掉，改用 `usePackCatalog()`。`ModContentPage` 剩余部分（kind Tab `:354-368`、进度树 `:375-407`、payload JSON `:488-491`、标签 Modal `:495-511`）按 §4.3 的表拆进索引态与右栏：

- kind Tab → 索引态的 `?type=` 筛选维度。
- 进度树 / 表格视图切换 → 索引态保留。
- 标签 Modal → **删除**，改由右栏焦点的标签区承担（P3）。
- payload JSON 查看 → 移到写态的「源 JSON」折叠区。
- 拆完后 `ModContentPage.tsx` 应当**整个文件删除**（`grep -rn "ModContentPage" apps/web/src` = 0）。

### 5.4 长任务轮询上提

模组解析（`ModContentPage.tsx:216-245`）与目录重建（`PackCatalogContext.tsx:62-78`）的轮询，切态后必须继续。目录重建已在 context 里（不用动）；**模组解析的轮询要上提**到一个 context（可放进 `PackSummaryContext` 或新建 `features/content/ModIndexContext.tsx`），否则切态即丢进度。

### 5.5 P2 验收

- 四态来回切，右栏焦点不变、来源栏状态不变、解析进度仍在走。
- 浏览器网络面板：进入内容页后 `GET /api/packs/<id>/catalog` **只被调用一次**（切态不重复请求）。
- `grep -rn "ModContentPage" apps/web/src` = 0。
- `grep -rn "getItemCatalog" apps/web/src` 只剩 `api/catalog.ts` 的定义与 `PackCatalogContext.tsx` 一处调用。
- `npx tsc --noEmit && npx vite build` 全绿；截图四态各一张给用户。

---

## 6. P3 · 右栏成为真传送门

### 6.1 传送门必须带参数

`InspectorRail.tsx:95-105` 三个按钮改为：

```tsx
<div className="inspector-actions-list">
  <Button block icon={<ExperimentOutlined/>}
    onClick={() => navigate(`/packs/${packId}/content?mode=graph&item=${encodeURIComponent(item.id)}`)}>
    查看合成关系
  </Button>
  <Button block icon={<GoldOutlined/>}
    onClick={() => navigate(`/packs/${packId}/content?mode=index&item=${encodeURIComponent(item.id)}`)}>
    在索引中定位
  </Button>
  <Button block icon={<EditOutlined/>}
    onClick={() => navigate(`/packs/${packId}/content?mode=edit&item=${encodeURIComponent(item.id)}`)}>
    魔改它的配方
  </Button>
</div>
```

`:122`（标签成员）、`:132`（配方）、`:139`（模组）、`:146`（任务）同样带参数。因为焦点已在 URL 里，`focusItemAndGo` 这个辅助函数（`:27-30`）可以删掉——`navigate` 带 `?item=` 就等于设焦点。

### 6.2 四态全部读焦点

现状只有 `RecipeBrowserPage.tsx:27-29` 读焦点。补齐：

- **索引态**：读 `?item=` → 滚动定位并选中该物品（`ItemCatalogPage.tsx:22` 的 `selectedId` 初值取自焦点）。
- **写态**：读 `?item=` → 自动筛出「以该物品为产物」的内容文档；没有则给出「为它新建配方」按钮（预填产物）。
- **编排态**：读 `?item=` → 高亮所有以该物品为奖励/目标的节点，并在右栏列出它们（点节点 → `?node=<id>`）。

### 6.3 接上两个悬空实现

- `InspectorRecipePreview`（`InspectorRail.tsx:156-165`）：焦点为物品且 `recipesByOutput` 有命中时，在右栏直接预览第一条配方。
- `focusRecipe`：关系态点配方的标题/边框时调 `focusRecipe(r.id)`，右栏显示该配方（用 `InspectorRecipePreview` 渲染）+「魔改此配方」按钮（→ `?mode=edit&doc=<docId>`）。

### 6.4 多语言名称表搬进右栏

`ItemCatalogPage.tsx:177-191` 的表格移到 `InspectorRail` 的物品分支（数据就在 `item.names`）。原页面的详情 Drawer（`:141-194`）随之删除——右栏就是详情。

### 6.5 P3 验收

- 从任意态点右栏任一关联条目，都能到达对应态**并保持焦点**。
- `grep -rn "relationKey" apps/web/src` = 0。
- 下钻 5 层后：顶栏「内容」Tab 高亮、右栏焦点、面包屑三者一致。
- 截图：右栏展开物品详情（含多语言名称表 + 配方预览）。

---

## 7. P4 · 概览就地处理冲突

### 7.1 补 wrapper（`apps/web/src/api/mods.ts` 末尾追加）

```ts
/* 冲突逐条处理：后端 routes_mods.go:140/147 已实现，返回 {"status": "resolved"|"ignored"}。 */
const conflictActionSchema = z.object({status: z.string()});
export const resolveConflict = (packId: string, conflictId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/conflicts/${encodeURIComponent(conflictId)}/resolve`, {}, conflictActionSchema);
export const ignoreConflict = (packId: string, conflictId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/conflicts/${encodeURIComponent(conflictId)}/ignore`, {}, conflictActionSchema);
```

### 7.2 概览页吸收依赖与冲突

`PackWorkbenchPage`（`PackPages.tsx:95-150`）改造为 `OverviewPage`：

- 把 `DependenciesPage`（`:313-349`）的冲突清单**整块搬进概览**，每行加两个按钮：`解决` → `resolveConflict`，`忽略` → `ignoreConflict`。成功后调 `usePackSummary().refresh()` + 重新 `listConflicts`。
- 「重新解析依赖」按钮（`:327`）保留，`resolve` 来自 `useDependencies(id)`。
- `PackHealthRail`（`:177-210`）**删除**——它的数据已经在右栏「包状况」段（§4.3）。概览不再自带健康栏，避免同一信息两处展示、两处口径。
- `?panel=conflicts` → 页面加载后滚动到冲突卡片并高亮一次。
- `CatalogSignalCard`（`:154-175`）四个 tile 改为「设焦点 + 切态」，路径见 §4.14。
- 新增：概览显示本包相关的后台任务（复用 `TaskPanel`，用 `fetchTasks()` 过滤本包）。

### 7.3 删 `DependenciesPage`

搬完后 `grep -rn "DependenciesPage" apps/web/src` = 0，函数体删除。

### 7.4 链路守护用例

在 `scripts/chain-test.py` 里加一条：`POST /conflicts/{cid}/ignore` 后 `GET /health` 的 `pendingErrors`/`pendingWarnings` 相应下降，且 `GET /conflicts` 里该条 `status === 'ignored'`。**照抄该文件里既有用例的写法**（正例 + 反例：对不存在的 conflictId 调用应返回 4xx 而不是 500）。

### 7.5 P4 验收

- 概览上忽略一条 `error` 级冲突后：右栏「包状况」的健康分与告警数**立刻**变化（不需要刷新页面），交付页构建按钮的禁用状态随之变化（P6 做闸门前，先确认 `usePackSummary().refresh()` 生效）。
- `grep -rn "PackHealthRail\|DependenciesPage" apps/web/src` = 0。
- `bash scripts/chain-test-run.sh /tmp/chain-run-v21.log` → PASS 数 = 基线 + 新增用例数，FAIL 0。

---

## 8. P5 · 魔改 → 任务顺路生成

### 8.1 apply 成功后的一次点击

`RecipeTweakPage.tsx:147-158` 的 apply 成功分支里，加一个「生成任务节点」按钮。点击后**预填**以下全部字段（作者一个都不用重填），然后跳到 `?mode=quest&node=<新节点id>`：

| 字段 | 来源 |
|---|---|
| 标题 | 产物物品的 `displayName`（`usePackCatalog().displayName`） |
| 图标 | 产物物品的 catalog 图标（`iconUrl(itemId)`），**不是 emoji** |
| 奖励 | `[{type:'item', itemId:<产物>, count:<配方产物数量>}]` |
| 任务 | `[{type:'item', itemId:<产物>, count:<同上>}]` |
| `modRefs` | 该配方来源模组的 id（内容文档里已有） |
| 前置 | 配方**材料**的物品 id 去匹配现有任务节点的奖励/目标，命中即连边 |
| 章节 | 当前编排态选中的章节；没有则第一个章节 |
| 位置 | 画布空白处（避开已有节点） |

节点类型与字段名以 `features/quest/questGraph.ts:93-108` 为准，**不要自己发明字段名**。

### 8.2 编排态补 `modRefs` 编辑 UI

`QuestBookEditor.tsx:956-1301` 的 Inspector 分组里加一节「引用的模组内容」：显示该节点 `modRefs` 的条目，每条可点（→ `?mode=graph&item=<id>` 或 `?mode=edit&doc=<docId>`），可增删。增的时候复用 `ItemPickerModal`（`features/catalog/ItemPickerModal.tsx:11`）。

后端 validate 会报 `missing_mod_reference` / `cross_pack_reference`（`content.go:842-844`），前端要把这两个 code 就地展示在对应字段旁，不要只弹一个 toast。

### 8.3 接上任务书的回滚与历史

`api/content.ts:154,156` 的 `rollbackQuest` / `questHistory` 在编排态加 UI（与写态 `RecipeTweakPage.tsx:160-177,298-308` 的历史/回滚**同一套交互**：一个「历史」抽屉，列出修订，每条可回滚）。

### 8.4 节点图标改取 catalog

`QuestBookEditor.tsx:177-204` 的 `IconPicker` + `features/quest/questIcons.ts:8-31` 的 emoji 目录：**默认取物品图标**，emoji 降级为兜底（物品没有图标时才用）。

### 8.5 P5 验收

- 魔改一条配方 → apply → 点「生成任务节点」→ 编排态出现一个新节点，标题/图标/奖励/任务/前置/`modRefs` 全部已填好，作者没有输入任何字符。
- 故意把 `modRefs` 指向一个不在包内的模组 → validate 报 `missing_mod_reference` 或 `cross_pack_reference`，错误就地显示在字段旁。
- 任务书历史抽屉能列出修订并回滚成功。

---

## 9. P6 · 交付流水线合一

### 9.1 新建 `pages/DeliveryPage.tsx`（替换 §4.8 的临时版）

一屏五段，纵向排布，每段显示自己的状态与动作：

| 段 | 搬自 | 内容 |
|---|---|---|
| ① 检查 | `PackPages.tsx:549-565` | 检查列表 + 「重新检查」（**不许再造占位 check**，见 §4.12） |
| ② 构建 | `PackPages.tsx:566-586,590-613` | 导出目录注册 + 版本登记 + 开始构建 + 产物清单 + **下载** |
| ③ 安装 | `LauncherPage.tsx:165-221,74-128` | 选游戏目录 → 查已装版本 → 安装（`artifact_id`） |
| ④ 起窗 | `LauncherPage.tsx:130-153,237-240` | 离线用户名 + 内存 + Java 路径 → 启动 → NDJSON 日志流 |
| ⑤ 发布 | 新增 | 选 provider → `publishPack` → `pollRelease` 轮询 → 失败可 `retryRelease` |

搬完后 `LauncherPage.tsx` 与 `PublishPage`（`PackPages.tsx:439-615`）**整体删除**，`grep -rn "LauncherPage\|PublishPage" apps/web/src` = 0。

### 9.2 产物下载

后端 `GET /api/packs/{packId}/artifacts/{artifactId}/download`（`routes_publish.go:156`）是 GET 且免令牌，所以**直接用链接**，不需要 fetch、不需要新 wrapper：

```tsx
<a className="artifact-download"
   href={`/api/packs/${encodeURIComponent(id)}/artifacts/${encodeURIComponent(a.id)}/download`}
   download={a.fileName}>下载</a>
```

放在 `PackPages.tsx:581-586` 的产物行里（搬到交付页后）。

### 9.3 构建闸门语义前端化

后端有 `assertPackBuildable`（缺陷 O20）：还有 `error` 级未解决冲突就不许构建，否则 409。前端要**提前反映**：`usePackSummary().health?.pendingErrors > 0` 时，「开始构建」禁用并显示「有 N 个未解决的 error 级冲突 · 去概览处理」（链接 `/packs/:id?panel=conflicts`）。不许让作者点了构建才吃 409。

### 9.4 发布段

- provider 两个：`curseforge` / `modrinth`。
- 无凭证时后端会拒绝，这是 **by-design 不是缺陷**：界面要把后端返回的 `errorCode`/`errorMessage` 原样显示，并给出「去设置配置 Key」的链接（`/settings`），**不要**写成「功能即将上线」。
- `pollRelease` 每 3s 轮询直到终态；失败给「重试」按钮调 `retryRelease`。

### 9.5 链路守护用例

`scripts/chain-test.py` 加一条：有 error 级未解决冲突时 `POST /build` 返回 409（对照后端已有测试 `TestBuildBlockedByUnresolvedFatalConflict`），冲突 ignore 后同一请求成功。

### 9.6 P6 验收

- 一屏内走完：检查 → 登记版本 → 注册导出目录 → 构建 → **下载产物**（真的下到文件，大小 > 0）→ 安装 → 起窗。
- 有 error 冲突时构建按钮禁用，处理后立刻可用。
- 发布段在无凭证时显示后端返回的真实错误码与「去设置」链接，没有假成功。
- `grep -rn "LauncherPage\|PublishPage" apps/web/src` = 0。
- `bash scripts/chain-test-run.sh /tmp/chain-run-v21.log` 全绿。
- 终局（**必须真跑一次**，这是本项目的硬要求）：
  ```bash
  cd /Volumes/Evo/code/mPackStation && CARGO_TARGET_DIR=/tmp/mpack-launcher-target bash scripts/verify-terminal-chain.sh --launch
  ```
  43 条断言全绿，Minecraft 真起窗。`CARGO_TARGET_DIR` **必须**是本地盘 `/tmp/mpack-launcher-target`，不许指向 Evo（SMB 挂载）。

---

## 10. 禁止清单（碰了就是错）

| 禁止 | 原因 |
|---|---|
| 新增任何顶栏导航项 | §0-1。这是本轮的核心目标 |
| 改 `apps/server/**` 任何文件 | §0-2。后端契约已就绪 |
| 改后端契约文档去迎合前端 | 同上 |
| 碰 `5173` / `18765` / `18766` / `18871` / `/tmp/mpack-data` | §0-3，那是用户的开发实例 |
| 把令牌写进任何文件 | 令牌只从 `/tmp/mpack-chain/runtime-token` 读进环境变量 |
| `CARGO_TARGET_DIR` 指向 `/Volumes/Evo/**` | SMB 挂载不放构建产物，会极慢且可能损坏 |
| 保留「后续版本提供 / 敬请期待」类假按钮 | §0-4 |
| 新造 URL 参数名（`src=` / `itemId=` / `tab=` 等） | §0-5，参数表见 §3.2 |
| 给四个态组件加 props | 它们现在是零 props 的，加了就要改调用点，徒增风险；态间通信一律走 URL + context |
| `git commit` / `git push` | §0-7 |
| 跳过阶段验收直接进下一阶段 | §0-6 |

---

## 11. 最终验收（P6 之后逐条跑，全部满足才算交付）

设计稿 §11 的 12 条是权威，这里给出可执行形式：

```bash
cd /Volumes/Evo/code/mPackStation
cd apps/web && npx tsc --noEmit && npx vite build && cd ../..
cd apps/server && go test ./... -count=1 && cd ../..
bash scripts/chain-test-run.sh /tmp/chain-run-v21.log

# 结构判据（每条都必须输出 0）
for p in "app-statusbar" "app-sider" "ModulePlaceholder" "ContentEditorPage" "useContentEditor" \
         "ModContentPage" "DependenciesPage" "PackHealthRail" "LauncherPage" "PublishPage" \
         "relationKey" "后续版本提供" "敬请期待" "mc-sider-w"; do
  printf '%-22s %s\n' "$p" "$(grep -rn "$p" apps/web/src | wc -l | tr -d ' ')"
done
```

点击级判据（隔离环境逐条走 + 截图）：

1. 顶栏可点导航 = 5（工作台 / 整合包 / 概览 / 内容 / 交付）+ ⚙；包内 Tab = 3；**无左侧栏、无底边栏**；右栏 = 包状况 + 当前焦点两段。
2. 11 条旧路由逐条访问，全部落到正确新位置，`?ns=` / `?item=` 等原有参数不丢。
3. 从任意物品出发，不刷新不丢焦点即可到达：它的配方、来源模组、引用它的任务节点、它的魔改写态。
4. 关系态点材料持续下钻到原料；下钻 5 层后顶栏 Tab / 右栏焦点 / 面包屑三者一致。
5. 魔改一条配方 → apply → 关系态立刻显示「已魔改」，无需手动刷新。
6. apply 后一次点击生成任务节点，产物 / 数量 / 图标 / 奖励 / 前置 / `modRefs` 全部预填。
7. 概览逐条 resolve/ignore 冲突，处理后健康分、右栏告警数、交付构建闸门三处同步更新。
8. 交付页一屏走完 检查 → 构建 → 下载 → 安装 → 起窗，五段状态各自独立可见。
9. 界面上没有点了没反应的按钮、没有写死的数字、没有拿不到数据却显示成功的块。
10. 任意状态刷新页面，所见完全一致（URL 是单一事实源）。
11. 上面那段 for 循环全部输出 0。
12. `tsc` / `vite build` / `go test` / `chain-test`（≥ PASS 172 / FAIL 0）/ `verify-terminal-chain.sh --launch`（43 断言全绿，Minecraft 真起窗）全部通过。

**做完把 §11 的 12 条结果逐条贴给用户，附截图。不许只说"完成了"。**
