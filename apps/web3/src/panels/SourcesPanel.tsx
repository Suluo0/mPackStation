import {useCallback, useEffect, useMemo, useRef, useState, type ReactElement, type ReactNode} from 'react';
import {
  addMod, listContentSources, listModVersions, removeMod, resolvePack, searchAllMods, updateMod,
  otherLoadersText, providerLabel,
  type Mod, type ModVersion, type SearchAllItem,
} from '../api/mods';
import {
  getModContentRun, parseModContent,
  type ModContentRun,
} from '../api/modContent';
import {useFocus, useUrlPatch, useUrlState} from '../app/url';
import {useNavRootSlot} from '../app/navRootSlot';
import {createPortal} from 'react-dom';
import {loaderLabel} from '../api/packs';
import {usePackSummary} from '../app/PackSummaryContext';
import {useCatalog} from '../app/CatalogContext';
import {searchCatalogByTag, searchCatalogItems, suggestTags} from '../app/catalogSearch';
import {Icon} from '../ui/Icon';
import {useContextMenu, type MenuItem} from '../ui/ContextMenu';
import {Prompt} from '../ui/Prompt';
import './panels.css';

/* 菜单分隔线。抽成常量是因为三元表达式里写 `{separator: true}` 会被推断成
   `{separator: boolean}`，对不上判别联合的 MenuSeparator。 */
const SEP: MenuItem = {separator: true};

/* 来源面板 = 包的项目树（第三轮反馈修订）：
   - 原版 Minecraft 是第一个来源行（includeBuiltin），点击 = 索引态看它贡献的全部物品；
   - 模组行展开 = 解析状态 + 内容分组：「玩法内容」（会改游戏的）在前，
     「资源」（系统用来生成图标/翻译的模组自带文件）折叠在后；
   - 点任何内容类型 → 索引态 ?ns=&type= 浏览解析条目。
   用户自定义分类（"优化/科技"等）需要 pack_mods.category 迁移，待用户批准（文档 §8）。 */
export function SourcesPanel() {
  return <SourceTree/>;
}

/* 章节面板已挪到 QuestPanel.tsx（第九轮反馈：它要能增删改，不该再是只读列表，
   也不该继续寄居在 600 行的来源面板里）。这里不再导出 QuestChaptersPanel。 */

const DOT: Record<string, string> = {installed: 'var(--mc-success)', pending: 'var(--mc-orange)', disabled: 'var(--mc-muted)'};

const KIND_LABELS: Record<string, string> = {
  recipe: '配方', item_model: '物品模型', item_icon: '物品图标', texture: '纹理',
  lang: '语言', ammo_definition: '机器配方', structure: '结构', terrain: '地形',
  loot: '战利品', loot_table: '战利品', worldgen: '群系与地形', advancement: '进度', tag: '标签', metadata: '元数据',
};
const kindLabel = (k: string) => KIND_LABELS[k] ?? k;

/* 玩法内容 vs 资源（第三轮反馈：资源类是模组自带文件，系统用它生成图标与翻译，
   不是作者要编辑的东西 → 折叠到第二组）。未知 kind 默认按玩法内容对待。 */
const RESOURCE_KINDS = new Set(['texture', 'lang', 'item_icon', 'metadata']);

function SourceTree() {
  const {packId, ns, q} = useUrlState();
  const patch = useUrlPatch();
  const {pack, refresh} = usePackSummary();
  const {status, refreshing} = useCatalog();
  /* 包名那一行的动作插槽（app/navRootSlot.ts）。面板不在树里渲染时为 null。 */
  const slot = useNavRootSlot();
  const [mods, setMods] = useState<Mod[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  /* 顶栏输入框：同一个组件服务两个功能，mode 决定当前高亮与下拉内容（第五轮反馈） */
  const [mode, setMode] = useState<'search' | 'add' | null>(() => (q ? 'search' : null));
  const [input, setInput] = useState('');
  const searchSeq = useRef(0);
  const [hits, setHits] = useState<SearchAllItem[] | null>(null);
  /* 降级搜索结果（后端 ModSearchAllResult.fallback）：按本包加载器一个都没搜到
     时，后端摘掉加载器限制重搜回来的「只支持其他加载器」的模组。
     注意与上面的 fallback(镜像回退提示，字符串) 是两回事，故另起名 altHits。 */
  const [altHits, setAltHits] = useState<SearchAllItem[]>([]);
  const [picked, setPicked] = useState<SearchAllItem | null>(null);
  const [versions, setVersions] = useState<ModVersion[]>([]);
  /* 版本拉取是否已落定（成功或失败都算）：只有「还在飞」才显示载入中，
     失败时上面已有错误行，这里不能再骗用户等一个永不到来的列表。 */
  const [versionsLoaded, setVersionsLoaded] = useState(false);
  const [showAllVersions, setShowAllVersions] = useState(false);
  /* 实际用来取版本/加装的那一路：主源坏了就切到镜像，加装必须按同一源的 versionId 提交 */
  const [src, setSrc] = useState<{ provider: string; projectId: string } | null>(null);
  const [fallback, setFallback] = useState<string | null>(null);
  /* 搜索/版本拉取的错误只留在下拉里：和模组的树共用一个 error 时，
     一次 CF 拉版本失败会一直挂在树顶部，用户分不清是树坏了还是搜索坏了。 */
  const [searchError, setSearchError] = useState<string | null>(null);
  /* 右键菜单：这层只按「光标下是什么」给出条目，坐标/关闭/Esc/出视口翻转
     全在 ui/ContextMenu 里，本文件不再存 x/y。 */
  const modMenu = useContextMenu();
  const groupMenu = useContextMenu();
  const blankMenu = useContextMenu();
  const builtinMenu = useContextMenu();

  /* 本体（builtin）的右键：它不能被停用也不能被移除，所以只给「看什么」的动作。
     展开能力仍走行首的箭头 —— 原版也解析出了配方/结构/物品，和模组一样能下钻。 */
  const builtinItems = (m: Mod): MenuItem[] => [
    {label: '浏览它贡献的全部物品', icon: 'search',
      action: () => patch({ns: m.canonicalModId || 'minecraft', mode: 'index', type: null})},
    {separator: true},
    {label: expanded === m.id ? '收起解析内容' : '展开解析内容', icon: 'layers',
      action: () => setExpanded(x => (x === m.id ? null : m.id))},
  ];

  const modItems = (m: Mod): MenuItem[] => [
    /* 停启用在右键里：行上挂「停用 / ✕」两个按钮，每多一个模组就多两个
       可误点的靶子（移除尤其不可撤销），而这两个动作的使用频率远低于浏览。 */
    {
      label: `${m.status === 'disabled' ? '启用' : '停用'} ${m.displayName}`,
      icon: m.status === 'disabled' ? 'play' : 'pause',
      disabled: m.status === 'pending',
      action: () => { void toggleMod(m).catch(() => undefined); },
    },
    {label: `重新解析 ${m.displayName}`, icon: 'refresh', action: () => { void parseModContent(packId!, m.id).catch(() => undefined); }},
    {separator: true},
    /* 归类：现有分类 → 新建 → 取消归类。子菜单而不是平铺，是因为分类会长；
       顺序按「最常用的动作在最近的地方」排。 */
    {label: '移动到分类', icon: 'folder', submenu: [
      ...categories.map(([name]) => ({
        label: name,
        disabled: (m.category ?? '').trim() === name,
        action: () => { void moveToCategory([m], name); },
      })),
      ...(categories.length ? [SEP] : []),
      {label: '新建分类…', icon: 'folderPlus', action: () => askNewCategory([m], `把「${m.displayName}」放进新分类`)},
      {
        label: '取消归类（回到未分类）',
        disabled: !(m.category ?? '').trim(),
        action: () => { void moveToCategory([m], ''); },
      },
    ]},
    {separator: true},
    {label: '从包中移除', icon: 'trash', danger: true, action: () => { void remove(m).catch(() => undefined); }},
  ];

  const groupItems = (name: string): MenuItem[] => [
    {label: '新建…', icon: 'plus', submenu: newItems(name)},
    {separator: true},
    {label: `重命名分类「${name}」…`, icon: 'wrench', action: () => {
      setRenameCatValue(name); setRenamingCat(name); setCatOpen(false);
    }},
    {
      label: `解散分类（${modsInCategory(name).length} 个模组回到未分类）`,
      icon: 'trash', danger: true,
      action: () => { void moveToCategory(modsInCategory(name), ''); },
    },
  ];

  /* 「新建…」的内容：目录内部与空白处共用一套，只是新建目录的落点不同 ——
     在目录上建 = 建成子分类名「父/新」，空白处建 = 建成顶层分类。 */
  const newItems = (parent?: string): MenuItem[] => [
    {
      label: '新建目录…', icon: 'folderPlus',
      action: () => setPrompt({
        title: parent ? `在「${parent}」下新建目录` : '新建目录',
        ok: v => { const n = v.trim(); if (n) setPendingCats(cs => cs.includes(n) ? cs : [...cs, n]); },
      }),
    },
    {
      label: '添加模组…', icon: 'store',
      action: () => patch({tool: 'store'}),
    },
  ];

  const [catOpen, setCatOpen] = useState(false);
  const [renamingCat, setRenamingCat] = useState<string | null>(null);
  const [renameCatValue, setRenameCatValue] = useState('');
  const [prompt, setPrompt] = useState<{title: string; ok: (v: string) => void} | null>(null);

  const load = useCallback(() => {
    if (!packId) return;
    listContentSources(packId).then(setMods).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);
  useEffect(load, [load]);

  if (!packId) return null;
  const busyCatalog = refreshing || status?.status === 'running' || status?.status === 'pending';
  const builtin = mods.filter(m => m.origin === 'builtin');
  const installed = mods.filter(m => m.origin !== 'builtin');

  const toggleMod = async (m: Mod) => {
    await updateMod(packId, m.id, {status: m.status === 'disabled' ? 'installed' : 'disabled'});
    load();
    refresh();
  };

  const remove = async (m: Mod) => {
    await removeMod(packId, m.id);
    load();
    refresh();
  };

  const resolveDeps = async () => {
    await resolvePack(packId);
    load();
    refresh();
  };

  /* ── 分类操作 ─────────────────────────────────────────────────── */
  /* 分类清单（名字 → 成员数）。空 category 不算一个分类，它只是「还没归类」。
     pendingCats 是「刚建、还没移入模组」的空分类：后端只有「模组.category」
     一个字段，空分类没有落库点，先留在前端 —— 模组移入后自然生效，
     一直为空的话刷新就消失（行上会注明）。 */
  const [pendingCats, setPendingCats] = useState<string[]>([]);
  const categories = useMemo(() => {
    const m = new Map<string, number>();
    for (const mod of mods) {
      if (mod.origin === 'builtin') continue;
      const k = (mod.category ?? '').trim();
      if (k) m.set(k, (m.get(k) ?? 0) + 1);
    }
    for (const k of pendingCats) if (!m.has(k)) m.set(k, 0);
    return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0], 'zh'));
  }, [mods, pendingCats]);

  /* 把一组模组整体改到某个分类（空字符串 = 未分类）。
     后端没有批量接口，只能一个个 PATCH；分类操作是低频人工动作，串行足够。
     一条失败就停并报错 —— 半途失败留下半个分类比整体失败更难懂。 */
  const moveToCategory = async (targets: Mod[], category: string) => {
    if (!packId || targets.length === 0) return;
    setError(null);
    try {
      for (const m of targets) {
        if ((m.category ?? '').trim() === category) continue;
        await updateMod(packId, m.id, {category});
      }
      /* 有成员落库后，这个分类不再需要前端暂存。 */
      setPendingCats(prev => prev.filter(k => k !== category));
      await refresh();
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };
  const modsInCategory = (name: string) =>
    mods.filter(m => m.origin !== 'builtin' && (m.category ?? '').trim() === name);

  /* 新建分类：先要名字（后端只有「模组.category」这一个字段，没有独立的分类表），
     拿到名字就把这些模组搬进去。 */
  const askNewCategory = (targets: Mod[], title: string) => {
    setPrompt({title, ok: v => { const name = v.trim(); if (name) void moveToCategory(targets, name); }});
  };

  /* 分类管理菜单里的「新建分类」：不带宿主模组，先建一个空分类（前端暂存），
     再右键模组移进来。 */
  const newCategoryFromMenu = () => {
    setPrompt({
      title: '新分类名称（建好后右键模组 → 移动到分类）',
      ok: v => {
        const name = v.trim();
        if (!name) return;
        setPendingCats(prev => (prev.includes(name) || categories.some(([k]) => k === name) ? prev : [...prev, name]));
        setCatOpen(false);
      },
    });
  };

  const commitRenameCat = (from: string) => {
    const to = renameCatValue.trim();
    setRenamingCat(null);
    if (!to || to === from) return;
    void moveToCategory(modsInCategory(from), to);
  };

  const search = async () => {
    if (!input.trim() || input.trim().length < 2) return;
    const seq = ++searchSeq.current;
    setSearchError(null); setPicked(null); setHits(null); setAltHits([]);
    try {
      const r = await searchAllMods(packId, {q: input.trim(), limit: 25});
      if (seq === searchSeq.current) { setHits(r.items); setAltHits(r.fallback ?? []); }
    }
    catch (e) {
      if (seq === searchSeq.current) { setHits([]); setSearchError(e instanceof Error ? e.message : String(e)); }
    }
  };
  // 输入防抖 300ms 自动搜索（≥2 字符）；过期响应丢弃
  useEffect(() => {
    if (mode !== 'add' || input.trim().length < 2) return;
    const t = window.setTimeout(() => { void search(); }, 300);
    return () => window.clearTimeout(t);
  }, [input, mode, packId]);

  const pick = async (h: SearchAllItem) => {
    setPicked(h); setVersions([]); setVersionsLoaded(false); setShowAllVersions(false);
    setSearchError(null); setFallback(null); setSrc(null);
    /* 主源（多为 curseforge）取版本挂掉时，搜索结果里的镜像源往往是好的：
       按主源→镜像依次试，成功的那一路同时决定 add() 提交用的 provider/projectId。 */
    const attempts = [{provider: h.provider, projectId: h.id}];
    if (h.mirror?.projectId) attempts.push({provider: h.mirror.provider, projectId: h.mirror.projectId});
    let lastErr = '没有可用版本';
    for (const a of attempts) {
      try {
        const v = await listModVersions(packId, a.provider, a.projectId);
        if (v.length === 0) { lastErr = `${a.provider} 没有返回版本`; continue; }
        setVersions(v); setSrc(a);
        if (a.provider !== h.provider) setFallback(`主源 ${providerLabel(h.provider)} 取不到版本，已改用镜像 ${providerLabel(a.provider)}。`);
        setVersionsLoaded(true);
        return;
      } catch (e) { lastErr = e instanceof Error ? e.message : String(e); }
    }
    setSearchError(lastErr);
    setVersionsLoaded(true);
  };

  const add = async (v: ModVersion) => {
    if (!packId || !picked) return;
    const source = src ?? {provider: picked.provider, projectId: picked.id};
    await addMod(packId, {provider: source.provider, projectId: source.projectId, versionId: v.id, required: true});
    setPicked(null); setVersions([]); setVersionsLoaded(false); setInput(''); setHits(null); setMode(null); setSrc(null); setFallback(null); setAltHits([]);
    load();
    refresh();
  };

  /* 面板标题承诺的是「兼容版本」，就只默认给兼容的：按本包 MC 版本 + 加载器筛。
     loaders 缺失（部分 CF 条目不回报加载器）时不按加载器卡死，只按 MC 版本判。 */
  const isCompatible = (v: ModVersion) => {
    if (!pack) return true;
    const gv = v.gameVersions ?? [];
    if (gv.length > 0 && !gv.includes(pack.mcVersion)) return false;
    const ld = v.loaders ?? [];
    return ld.length === 0 || ld.some(l => l.toLowerCase() === pack.loader.toLowerCase());
  };
  const compat = versions.filter(isCompatible);
  const others = versions.filter(v => !isCompatible(v));
  const versionRow = (v: ModVersion) => (
    <div key={v.id} className="p-row click" onClick={() => void add(v).catch(e => setSearchError(String(e)))}>
      <span className="grow">{v.versionNumber || v.name || v.id}</span>
      {(v.gameVersions ?? []).slice(0, 1).map(g => <span key={g} className="sub">{g}</span>)}
    </div>
  );

  /* 这三个按钮**不在本面板里渲染**：它们属于「包名」那一行，由包根目录树的根行
     提供插槽（.nav-acts），这里用 portal 送上去。删掉面板自己的标题行之后，
     按钮留在原地就成了无主的孤儿 —— 动作归谁，就该摆在那一行上。
     状态仍留在本面板（catOpen / mode），树不需要知道这些。 */
  const actions = (
    <>
      {/* 分类管理：没法在这里凭空建空分类（分类是挂在模组上的字段），
          所以这里只做「看有哪些分类 + 重命名 + 解散」，新建走右键模组。 */}
      <button type="button" className={`tp-icon-btn${catOpen ? ' on' : ''}`}
        aria-label="分类" title={`分类管理（${categories.length} 个）`}
        onClick={() => setCatOpen(v => !v)}><Icon name="folderPlus" size={14}/></button>
      <div className={`head-field${mode !== null ? ' open' : ''}`}>
        {mode !== null && (
          <>
            <input className="head-input" autoFocus
              value={mode === 'search' ? q : input}
              placeholder={mode === 'search' ? '搜物品：中英文名 / ID / 缩写…' : '模组名（Enter 搜索）…'}
              onChange={e => mode === 'search' ? patch({q: e.target.value || null}) : setInput(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Escape') setMode(null);
                if (e.key === 'Enter' && mode === 'add') void search();
              }}/>
            {(mode === 'search' ? q : input) && (
              <button type="button" className="head-clear" aria-label="清空输入"
                onClick={() => { if (mode === 'search') patch({q: null}); else setInput(''); }}>✕</button>
            )}
          </>
        )}
        <button type="button" className="tp-icon-btn" aria-label="解析依赖"
          title="解析全部模组依赖，锁定版本并检测冲突"
          onClick={() => void resolveDeps().catch(e => setError(String(e)))}><Icon name="refresh" size={14}/></button>
        <button type="button" className={`tp-icon-btn${mode === 'search' ? ' on' : ''}`} aria-label="搜索物品"
          title="搜索包内物品（本地目录，支持中英文名 / ID / 来源缩写）"
          onClick={() => setMode(m => (m === 'search' ? null : 'search'))}><Icon name="search" size={14}/></button>
        {/* 「添加模组」的加号按钮已移除：商店面板（tool=store）有完整的
            模组搜索 + 兼容版本选择 + 降级说明，这里的入口只会重复。 */}
      </div>
    </>
  );

  return (
    <>
      {slot && createPortal(actions, slot)}
      {/* .tp-top 现在只承载两个下拉（分类菜单 / 搜索命中）：按钮已经上移到根行，
          下拉留在内容区顶端，不会被 .nav-tree 的滚动容器裁掉。 */}
      <div className="tp-top">
        {catOpen && (
              <>
                <div style={{position: 'fixed', inset: 0, zIndex: 40}} onClick={() => setCatOpen(false)}/>
                <div className="cat-menu">
                  <div className="p-title" style={{padding: '4px 8px 2px'}}>分类 <span className="count">{categories.length}</span></div>
                  {categories.length === 0 && (
                    <div className="p-empty" style={{padding: '4px 8px'}}>还没有分类。</div>
                  )}
                  {categories.map(([name, n]) => (
                    <div key={name} className="cat-row">
                      <Icon name="folder" size={12}/>
                      <span className="grow" style={{overflow: 'hidden', textOverflow: 'ellipsis'}}>{name}</span>
                      <span className="sub">{n}</span>
                      <button type="button" className="p-btn" title="重命名（该分类下全部模组一起改）"
                        onClick={() => { setRenameCatValue(name); setRenamingCat(name); setCatOpen(false); }}>
                        改名
                      </button>
                      <button type="button" className="p-btn" title="解散分类：成员回到未分类"
                        onClick={() => { setCatOpen(false); setPendingCats(prev => prev.filter(k => k !== name)); void moveToCategory(modsInCategory(name), ''); }}>
                        解散
                      </button>
                    </div>
                  ))}
                  <div className="cat-hint">
                    <button type="button" className="p-btn" onClick={newCategoryFromMenu}>
                      <Icon name="plus" size={12}/> 新建分类…
                    </button>
                    建好后右键模组 → 移动到分类。空分类暂存本页，移入模组后才会保存。
                  </div>
                </div>
              </>
            )}
        {mode !== null && (
          <div className="head-dd">
            {mode === 'search' ? <SearchHits q={q}/> : (
              <div className="ss-head">
                <span>模组搜索</span>
                <span style={{flex: 1}}/>
                {/* 提交键与商店面板同款放大镜（.ui-search-btn），不用「搜」字 ——
                    中文挤得下、英文 search 放不下。输入框在顶栏、按钮在下拉里，
                    布局上不是一个整体，所以复用的是 ui 层的样式而不是 SearchInput 组件。 */}
                <button type="button" className="ui-search-btn" aria-label="搜索" title="搜索"
                  onClick={() => void search()}>
                  <Icon name="search" size={14}/>
                </button>
              </div>
            )}
            {mode === 'add' && (
              <div className="ss-body">
                {searchError && <div className="p-empty">{searchError}</div>}
                {hits?.length === 0 && !picked && !searchError && altHits.length === 0 && (
                  <div className="p-empty">没有命中。设置页确认 CurseForge Key 后再试。</div>
                )}
                {!picked && (hits ?? []).map(h => (
                  <div key={`${h.provider}-${h.id}`} className="p-row click" onClick={() => void pick(h)} title={h.summary ?? ''}>
                    <span className="grow">{h.name}</span>
                    <span className="sub">{providerLabel(h.provider)}{h.mirror ? ' · 镜像' : ''}</span>
                  </div>
                ))}

                {/* 降级结果：同 MC 版本、但只支持其他加载器。这些**装不进**本包，
                    所以刻意不可点击 —— 让用户点到一半才失败，比一开始就说清楚更糟。
                    文案不提「没有命中」：主结果里可能有几条平台的模糊命中。 */}
                {!picked && altHits.length > 0 && (
                  <>
                    <div className="p-empty">
                      本包（MC {pack?.mcVersion} · {loaderLabel(pack?.loader)}）没有真正匹配的模组。以下同 MC 版本、但只支持其他加载器的，装不进本包：
                    </div>
                    {altHits.map(h => (
                      <div key={`alt-${h.provider}-${h.id}`} className="p-row na" title={h.summary ?? ''}>
                        <span className="grow">{h.name}</span>
                        <span className="sub">仅支持 {otherLoadersText(h.loaders, pack?.loader ?? '') || '其他加载器'}</span>
                      </div>
                    ))}
                  </>
                )}
                {picked && (
                  <>
                    <div className="p-row">
                      <span className="grow" style={{fontWeight: 600}}>{picked.name}</span>
                      <button type="button" className="p-btn" onClick={() => setPicked(null)}>返回</button>
                    </div>
                    <div className="p-empty">
                      {pack ? `选一个兼容版本（MC ${pack.mcVersion} · ${pack.loader}，添加即钉版）：` : '选一个兼容版本（添加即钉版）：'}
                    </div>
                    {!versionsLoaded && <div className="p-empty">版本载入中…</div>}
                    {fallback && <div className="p-empty">{fallback}</div>}
                    {versionsLoaded && compat.map(versionRow)}
                    {versionsLoaded && versions.length > 0 && compat.length === 0 && (
                      <div className="p-empty">
                        这个模组的 {versions.length} 个版本里没有匹配本包（MC {pack?.mcVersion} · {pack?.loader}）的，展开下面的其他版本可以强行加装。
                      </div>
                    )}
                    {versionsLoaded && others.length > 0 && (
                      <>
                        <div className="p-row click" style={{opacity: .75}} onClick={() => setShowAllVersions(v => !v)}
                          title="这些版本不属于本包的 MC 版本或加载器，装了大概率崩">
                          <span className="grow sub">其他版本（不匹配本包）</span>
                          <span className="sub" style={{display: 'inline-flex', alignItems: 'center', gap: 3}}>
                            {others.length} <Icon name={showAllVersions ? 'caretDown' : 'caretRight'} size={12}/>
                          </span>
                        </div>
                        {showAllVersions && others.map(versionRow)}
                      </>
                    )}
                  </>
                )}
                {!hits && !picked && !searchError && <div className="p-empty">输入模组名后按 Enter；命中后在下方选版本。</div>}
              </div>
            )}
          </div>
        )}
      </div>
      {/* 空白区右键：只给「新建…」这类跟光标下对象无关的动作。
          与目录上右键同一模型 —— 先「新建…」，再选建什么，不把动作平铺在顶层。 */}
      <div className="tp-body" onContextMenu={e => blankMenu.open(e, [
        {label: '新建…', icon: 'plus', submenu: newItems()},
      ])}>
        {error && <div className="p-empty">{error}</div>}
        {/* 本体也走 ModRow：它也解析出了配方 / 结构 / 物品，展开后和模组一样
            能按内容类型下钻。差别只有两处 —— 圆点是蓝色（不是装/停的状态灯），
            右键菜单里没有「停用 / 移除 / 归类」（本体不能也不需要）。 */}
        {builtin.map(m => (
          <ModRow key={m.id} mod={m} expanded={expanded === m.id}
            onToggleExpand={() => setExpanded(x => x === m.id ? null : m.id)}
            onFilter={() => patch({ns: m.canonicalModId || 'minecraft', mode: 'index', type: null})}
            onContextMenu={e => builtinMenu.open(e, builtinItems(m))}
            onParsed={load}
            badge={<span className="sub">原版</span>}/>
        ))}
        <ModGroups installed={installed} expanded={expanded} setExpanded={setExpanded}
          extraCats={pendingCats}
          patch={patch}
          onParsed={load}
          onContextMenu={(e, m) => modMenu.open(e, modItems(m))}
          onGroupContextMenu={(e, name) => groupMenu.open(e, groupItems(name))}
          renamingCat={renamingCat} renameCatValue={renameCatValue}
          setRenameCatValue={setRenameCatValue}
          onCommitRenameCat={commitRenameCat} onCancelRenameCat={() => setRenamingCat(null)}/>
        {installed.length === 0 && !error && <div className="p-empty">还没有模组。点顶栏右上「+」搜一个。</div>}
        {ns && !expanded && (
          <div className="p-row">
            <span className="sub grow">只看 {ns} 贡献的物品</span>
            <button type="button" className="p-btn" onClick={() => patch({ns: null, type: null})}>清除</button>
          </div>
        )}

      <div className="p-section">
        <div className="p-title">内容目录</div>
        <div className="p-row">
          <span className="dot" style={{background: busyCatalog ? 'var(--mc-orange)' : status?.status === 'failed' ? 'var(--mc-fail)' : 'var(--mc-success)'}}/>
          <span className="grow sub">{busyCatalog ? '目录构建中…' : status ? `revision ${status.builtRevision}` : '未构建'}</span>
        </div>
      </div>
      {modMenu.menu}
      {groupMenu.menu}
      {blankMenu.menu}
      {builtinMenu.menu}

      {/* 输入弹窗走 ui/Prompt（Esc / 空值 / 聚焦三件事统一处理）。 */}
      {prompt && (
        <Prompt title={prompt.title} okLabel="创建并移入"
          placeholder="分类名，如：科技 / 主线 / 优化"
          onOk={prompt.ok} onClose={() => setPrompt(null)}/>
      )}
    </div>
    </>
  );
}

/* 搜索命中（下拉内容）：与索引态同一套相关度排序；# 前缀走标签搜索
   （概念词如 #tools 展开为镐/斧/锄/剑/锹），点击行 = 设焦点，不清空搜索。 */
function SearchHits({q}: {q: string}) {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const {catalog} = useCatalog();
  const [focus, setFocus] = useFocus();
  const isTag = q.trim().startsWith('#');
  const tagQ = isTag ? q.trim().slice(1) : '';
  const tagMatches = useMemo(
    () => (isTag && tagQ ? searchCatalogByTag(catalog?.items ?? [], tagQ) : []),
    [catalog, isTag, tagQ],
  );
  const tagSuggestions = useMemo(
    () => (isTag && tagQ && tagMatches.length === 0 ? suggestTags(catalog?.tags ?? [], tagQ) : []),
    [catalog, isTag, tagQ, tagMatches.length],
  );
  const hits = useMemo(
    () => (isTag ? [] : q.trim() ? searchCatalogItems(catalog?.items ?? [], q, null).slice(0, 30) : []),
    [catalog, isTag, q],
  );
  const iconUrl = (itemId: string) =>
    `/api/packs/${encodeURIComponent(packId ?? '')}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;

  if (isTag) {
    return (
      <>
        <div className="ss-head">
          <span>标签命中 {tagMatches.length}</span>
        </div>
        <div className="ss-body">
          {tagMatches.map(({item, matchedTag}) => (
            <div key={item.id} className={`p-row click${focus?.kind === 'item' && focus.id === item.id ? ' on' : ''}`}
              onClick={() => setFocus({kind: 'item', id: item.id})} title={`${item.id} · ${matchedTag}`}>
              {item.iconStatus === 'ready'
                ? <img className="ss-icon" alt="" loading="lazy" src={iconUrl(item.id)}/>
                : <span className="ss-icon ss-ph">{item.displayName.slice(0, 1)}</span>}
              <span className="grow">{item.displayName}</span>
              <span className="mono">{item.id.split(':').pop()}</span>
            </div>
          ))}
          {tagMatches.length === 0 && tagQ && (
            <>
              <div className="p-empty">没有物品命中标签「{tagQ}」。目录里相关的标签：</div>
              {tagSuggestions.map(t => (
                <div key={t.id} className="p-row click" onClick={() => patch({q: `#${t.id}`})} title={`精确到这个标签（${t.count} 个物品）`}>
                  <span className="grow mono" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 11.5}}>{t.id}</span>
                  <span className="sub">{t.count} 物品</span>
                </div>
              ))}
              {tagSuggestions.length === 0 && <div className="p-empty">目录里也没有含「{tagQ}」的标签。试试 #tools（工具类概念词）。</div>}
            </>
          )}
          {!tagQ && <div className="p-empty"># 后跟标签或概念词，如 #tools、#pickaxes。</div>}
        </div>
      </>
    );
  }

  return (
    <>
      <div className="ss-head">
        <span>命中 {hits.length}</span>
      </div>
      <div className="ss-body">
        {hits.map(({item}) => (
          <div key={item.id} className={`p-row click${focus?.kind === 'item' && focus.id === item.id ? ' on' : ''}`}
            onClick={() => setFocus({kind: 'item', id: item.id})} title={item.id}>
            {item.iconStatus === 'ready'
              ? <img className="ss-icon" alt="" loading="lazy" src={iconUrl(item.id)}/>
              : <span className="ss-icon ss-ph">{item.displayName.slice(0, 1)}</span>}
            <span className="grow">{item.displayName}</span>
            <span className="mono">{item.id}</span>
          </div>
        ))}
        {hits.length === 0 && (
          <div className="p-empty">{q.trim() ? <>没有匹配「{q}」的物品。标签试试 <b>#tools</b>。</> : '输入关键词；支持英文（applied）、缩写（ae2）、#标签。'}</div>
        )}
      </div>
    </>
  );
}

/* 单个模组行：名称行（点击=筛选它贡献的物品）+ 展开（解析状态 + 内容类型下钻）。

   行上刻意**不挂**「停用 / 移除」按钮：那是低频且不可逆的动作，
   挤在每个模组右侧等于给误点造靶子，现在一律走右键菜单。
   本体（builtin）也用这一行，只是 badge 不同（「原版」而非状态相关的东西）。 */
/* 向游戏实际贡献内容的种类（配方/物品模型/结构/群系/战利品/进度/标签）。
   只有 lang/metadata/texture/item_icon 的模组对游戏没有任何变化 —— 折叠成一行。 */
const GAMEPLAY_KINDS = new Set(['recipe', 'item_model', 'structure', 'worldgen', 'loot_table', 'advancement', 'tag']);

function ModRow({mod, expanded, onToggleExpand, onFilter, onContextMenu, onParsed, badge}: {
  mod: Mod;
  expanded: boolean;
  onToggleExpand: () => void;
  onFilter: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  onParsed: () => void;
  badge?: ReactNode;
}) {
  /* 空模组判定用后端给的精确计数（contentKinds），不再靠展开后取样数。
     原版（minecraft）的 mod_content 里 metadata 只有一行内部口径，其余种类
     齐全，不会被误判。 */
  const gameplayTotal = Object.entries(mod.contentKinds ?? {})
    .filter(([k]) => GAMEPLAY_KINDS.has(k))
    .reduce((s, [, n]) => s + n, 0);
  const isEmpty = gameplayTotal === 0;
  const nameEl = mod.nameZh
    ? <span className="grow" style={{overflow: 'hidden', textOverflow: 'ellipsis'}}>{mod.nameZh} <span className="sub">{mod.displayName}</span></span>
    : <span className="grow" style={{overflow: 'hidden', textOverflow: 'ellipsis'}}>{mod.displayName}</span>;
  return (
    <>
      <div className="p-row click" onClick={onFilter} onContextMenu={onContextMenu}
        title={`${mod.nameZh ?? mod.displayName} · 点击筛选它贡献的物品 · 右键更多操作`}>
        {/* 空内容模组没有展开区可给，行首不放箭头 —— 展开箭头的有无本身就是
            「这个模组有没有可下钻的内容」的信号。 */}
        {!isEmpty && (
          <button type="button" className="src-chevron" aria-label={expanded ? '收起' : '展开解析内容'}
            aria-expanded={expanded}
            onClick={e => { e.stopPropagation(); onToggleExpand(); }}>
            <Icon name={expanded ? 'caretDown' : 'caretRight'} size={13}/>
          </button>
        )}
        {/* 本体的圆点是蓝色：它不是「装了 / 停了」的状态，是「这就是游戏本身」。 */}
        <span className="dot" style={{background: mod.origin === 'builtin' ? 'var(--mc-blue)' : DOT[mod.status] ?? 'var(--mc-muted)'}}/>
        {nameEl}
        {badge}
      </div>
      {isEmpty && (
        <div className="p-row" style={{paddingLeft: 26}}>
          <span className="sub">{mod.description ?? '这个模组不向游戏提供物品、配方、结构等内容（例如纯翻译/纯库模组）。'}</span>
        </div>
      )}
      {expanded && !isEmpty && (
        <>
          {/* 展开第一行 = 模组的一句话描述（平台元数据；原版行由后端给定）。 */}
          {mod.description && (
            <div className="p-row" style={{paddingLeft: 26}} title="模组的一句话描述">
              <span className="sub">{mod.description}</span>
            </div>
          )}
          <ModContent mod={mod} ns={mod.canonicalModId} onParsed={onParsed}/>
        </>
      )}
    </>
  );
}

/* 分组渲染（0027 / 本轮加分类管理）：按 mod.category 分组的模组树。
   未分类的模组不套标题（平铺在最前），有分类的每组一个标题行，
   标题行可右键重命名 / 解散，也可以就地改名（双击标题走的是同一个状态机）。

   为什么是组件而不是函数：分组标题要能就地变成输入框（重命名），
   这需要它自己持有展开/编辑态；原来那个函数式渲染没法挂。 */
function ModGroups({installed, expanded, setExpanded, extraCats, patch, onParsed,
  onContextMenu, onGroupContextMenu, renamingCat, renameCatValue, setRenameCatValue,
  onCommitRenameCat, onCancelRenameCat}: {
  installed: Mod[];
  expanded: string | null;
  setExpanded: (fn: (x: string | null) => string | null) => void;
  /* 前端暂存的空分类（新建后还没移入模组）：也要渲染成可折叠的分组。 */
  extraCats: string[];
  patch: (p: Record<string, string | null>, opts?: {push?: boolean}) => void;
  onParsed: () => void;
  onContextMenu: (e: React.MouseEvent, m: Mod) => void;
  onGroupContextMenu: (e: React.MouseEvent, name: string) => void;
  renamingCat: string | null;
  renameCatValue: string;
  setRenameCatValue: (v: string) => void;
  onCommitRenameCat: (from: string) => void;
  onCancelRenameCat: () => void;
}) {
  /* 折叠的分类集合。与模组展开（单选 expanded）不同：分类折叠是多选开关，
     默认全展开 —— 目录的意义就是看见里面的东西，折叠只是收起干扰。 */
  const [collapsedCats, setCollapsedCats] = useState<Set<string>>(new Set());
  const groups = new Map<string, Mod[]>();
  for (const m of installed) {
    const key = m.category?.trim() || '';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(m);
  }
  for (const k of extraCats) if (!groups.has(k)) groups.set(k, groups.get(k) ?? []);
  const categorized = [...groups.keys()].filter(k => k !== '').sort((a, b) => a.localeCompare(b, 'zh'));

  const row = (m: Mod) => (
    <ModRow key={m.id} mod={m} expanded={expanded === m.id}
      onToggleExpand={() => setExpanded(x => x === m.id ? null : m.id)}
      onFilter={() => patch({ns: m.canonicalModId, src: null, mode: 'index', type: null})}
      onContextMenu={e => onContextMenu(e, m)}
      onParsed={onParsed}/>
  );

  /* 一个分类都没有时不套标题，避免给「还没归类」凭空造一个组。 */
  if (categorized.length === 0) return <>{installed.map(row)}</>;

  const rows: ReactElement[] = [];
  for (const key of ['', ...categorized]) {
    const list = groups.get(key) ?? [];
    const isPendingEmpty = list.length === 0 && extraCats.includes(key);
    if (list.length === 0 && !isPendingEmpty) continue;
    if (key !== '') {
      const collapsed = collapsedCats.has(key);
      rows.push(
        <div key={`g-${key}`} className="p-title cat-head"
          title={`${key} · ${list.length} 个模组\n点击折叠 / 展开；右键：重命名 / 解散这个分类`}
          onClick={() => setCollapsedCats(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key); else next.add(key);
            return next;
          })}
          onContextMenu={e => onGroupContextMenu(e, key)}>
          <button type="button" className="src-chevron" aria-label={collapsed ? '展开分类' : '折叠分类'}
            aria-expanded={!collapsed}
            onClick={e => {
              e.stopPropagation();
              setCollapsedCats(prev => {
                const next = new Set(prev);
                if (next.has(key)) next.delete(key); else next.add(key);
                return next;
              });
            }}>
            <Icon name={collapsed ? 'caretRight' : 'caretDown'} size={13}/>
          </button>
          {renamingCat === key
            ? <input className="p-input" autoFocus value={renameCatValue}
                style={{flex: 1, minWidth: 0, marginLeft: 4}}
                onClick={e => e.stopPropagation()}
                onChange={e => setRenameCatValue(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter') onCommitRenameCat(key);
                  if (e.key === 'Escape') onCancelRenameCat();
                }}
                onBlur={() => onCommitRenameCat(key)}/>
            : <>{' '}{key} <span className="count">{list.length}</span></>}
        </div>);
    }
    if (key === '' || !collapsedCats.has(key)) {
      if (isPendingEmpty) {
        rows.push(
          <div key={`g-empty-${key}`} className="p-empty" style={{paddingLeft: 26}}>
            空分类（暂存本页，移入模组后保存）：右键任意模组 → 移动到分类 → {key}
          </div>);
      }
      for (const m of list) rows.push(row(m));
    }
  }
  return <>{rows}</>;
}

/* 展开区：解析运行状态 + 内容分组（玩法内容在前，资源折叠在后）。
   种类计数直接用清单接口带的精确聚合（mod.contentKinds，后端 GROUP BY 出的），
   **不再拉 1000 条完整 payload 自己数** —— 那个响应好几 MB，8s 超时在外面走
   mesh 时打不住，请求被静默吞掉后就只剩「解析数在、内容为空」的假象
   （2026-10-04 用户在外实测踩中）。 */
function ModContent({mod, ns, onParsed}: {mod: Mod; ns: string; onParsed: () => void}) {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const modId = mod.id;
  const [run, setRun] = useState<ModContentRun | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [resOpen, setResOpen] = useState(false);

  const kinds = useMemo(() => Object.entries(mod.contentKinds ?? {})
    .map(([kind, count]) => ({kind, count}))
    .sort((a, b) => b.count - a.count), [mod.contentKinds]);

  useEffect(() => {
    if (!packId || !modId) return;
    let alive = true;
    setLoading(true);
    setError(null);
    getModContentRun(packId, modId).catch(() => null).then(r => {
      if (!alive) return;
      setRun(r);
      setLoading(false);
    });
    return () => { alive = false; };
  }, [packId, modId]);

  const parse = async () => {
    if (!packId) return;
    setError(null);
    try {
      await parseModContent(packId, modId);
      // 解析是后台任务；进度看底部停靠，这里轮询 run 直到落终态
      const t = window.setInterval(async () => {
        try {
          const r = await getModContentRun(packId, modId);
          setRun(r);
          if (r.status !== 'running' && r.status !== 'pending') {
            window.clearInterval(t);
            onParsed(); // 新计数在清单接口的 contentKinds 里，重拉清单即可
          }
        } catch { /* 下一轮再试 */ }
      }, 3000);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (loading) return <div className="p-empty" style={{paddingLeft: 26}}>载入解析内容…</div>;

  /* metadata（模组自述元数据）是系统内部口径，只送后端消费，不再作为内容种类
     展示（2026-10-04 用户反馈：它不该出现在前端）。 */
  const gameplay = kinds.filter(k => !RESOURCE_KINDS.has(k.kind) && k.kind !== 'metadata');
  const resources = kinds.filter(k => RESOURCE_KINDS.has(k.kind) && k.kind !== 'metadata');

  const kindRow = (k: {kind: string; count: number}) => (
    <div key={k.kind} className="p-row click"
      onClick={() => patch({ns, src: modId, mode: 'index', type: k.kind})}
      title={`在索引中浏览这个模组的${kindLabel(k.kind)}`}>
      <span className="grow">{kindLabel(k.kind)}</span>
      <span className="sub">{k.count}</span>
    </div>
  );

  return (
    <div className="mod-kinds">
      {!run && (
        <div className="p-row" style={{paddingLeft: 26}}>
          <span className="sub grow">还没有解析过这个模组。</span>
          <button type="button" className="p-btn" onClick={() => void parse()}>解析</button>
        </div>
      )}
      {run && (
        <div className="run-stats">
          <span>已解析 {run.parsedCount}</span>
          {run.dynamicCount > 0 && <span>动态 {run.dynamicCount}</span>}
          {run.errorCount > 0 && <span style={{color: 'var(--mc-fail)'}}>错误 {run.errorCount}</span>}
          <span>文件 {run.totalFiles}</span>
        </div>
      )}
      {error && <div className="p-empty" style={{paddingLeft: 26}}>{error}</div>}
      {gameplay.map(kindRow)}
      {resources.length > 0 && (
        <>
          <div className="p-row click" style={{paddingLeft: 26}} onClick={() => setResOpen(v => !v)}
            title="模组自带的贴图/翻译/清单，系统用它们生成目录图标与中文名；一般不用编辑">
            <span className="grow sub">资源（图标/翻译等，{resources.length} 类）</span>
            <span className="sub" style={{display: 'inline-flex', alignItems: 'center', gap: 3}}>
              {resources.reduce((s, k) => s + k.count, 0)} <Icon name={resOpen ? 'caretDown' : 'caretRight'} size={12}/>
            </span>
          </div>
          {resOpen && resources.map(kindRow)}
        </>
      )}
      {run && gameplay.length === 0 && resources.length === 0 && (
        <div className="p-empty" style={{paddingLeft: 26}}>这个模组没有解析出内容条目（功能全靠运行时代码的模组就是这样）。</div>
      )}
    </div>
  );
}
