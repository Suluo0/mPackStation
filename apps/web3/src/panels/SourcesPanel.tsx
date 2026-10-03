import {useCallback, useEffect, useMemo, useRef, useState, type ReactElement} from 'react';
import {
  addMod, listContentSources, listModVersions, searchAllMods, updateMod,
  type Mod, type ModVersion, type SearchAllItem,
} from '../api/mods';
import {
  getModContentRun, listModContent, parseModContent,
  type ModContentRun,
} from '../api/modContent';
import {getQuest} from '../api/content';
import {useFocus, useUrlPatch, useUrlState} from '../app/url';
import {usePackSummary} from '../app/PackSummaryContext';
import {useCatalog} from '../app/CatalogContext';
import {searchCatalogByTag, searchCatalogItems, suggestTags} from '../app/catalogSearch';
import {Icon} from '../app/Icon';
import './panels.css';

/* 来源面板 = 包的项目树（第三轮反馈修订）：
   - 原版 Minecraft 是第一个来源行（includeBuiltin），点击 = 索引态看它贡献的全部物品；
   - 模组行展开 = 解析状态 + 内容分组：「玩法内容」（会改游戏的）在前，
     「资源」（系统用来生成图标/翻译的模组自带文件）折叠在后；
   - 点任何内容类型 → 索引态 ?ns=&type= 浏览解析条目。
   用户自定义分类（"优化/科技"等）需要 pack_mods.category 迁移，待用户批准（文档 §8）。 */
export function SourcesPanel() {
  const {mode} = useUrlState();
  return mode === 'quest' ? <QuestChapters/> : <SourceTree/>;
}

const DOT: Record<string, string> = {installed: 'var(--mc-success)', pending: 'var(--mc-orange)', disabled: 'var(--mc-muted)'};

const KIND_LABELS: Record<string, string> = {
  recipe: '配方', item_model: '物品模型', item_icon: '物品图标', texture: '纹理',
  lang: '语言', ammo_definition: '机器配方', structure: '结构', terrain: '地形',
  loot: '战利品', advancement: '进度', tag: '标签', metadata: '元数据',
};
const kindLabel = (k: string) => KIND_LABELS[k] ?? k;

/* 玩法内容 vs 资源（第三轮反馈：资源类是模组自带文件，系统用它生成图标与翻译，
   不是作者要编辑的东西 → 折叠到第二组）。未知 kind 默认按玩法内容对待。 */
const RESOURCE_KINDS = new Set(['texture', 'lang', 'item_icon', 'item_model', 'metadata']);

function SourceTree() {
  const {packId, ns, q} = useUrlState();
  const patch = useUrlPatch();
  const {pack, refresh} = usePackSummary();
  const {status, refreshing} = useCatalog();
  const [mods, setMods] = useState<Mod[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  /* 顶栏输入框：同一个组件服务两个功能，mode 决定当前高亮与下拉内容（第五轮反馈） */
  const [mode, setMode] = useState<'search' | 'add' | null>(() => (q ? 'search' : null));
  const [input, setInput] = useState('');
  const searchSeq = useRef(0);
  const [hits, setHits] = useState<SearchAllItem[] | null>(null);
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

  const search = async () => {
    if (!input.trim() || input.trim().length < 2) return;
    const seq = ++searchSeq.current;
    setSearchError(null); setPicked(null); setHits(null);
    try {
      const r = (await searchAllMods(packId, {q: input.trim(), limit: 25})).items;
      if (seq === searchSeq.current) setHits(r);
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
        if (a.provider !== h.provider) setFallback(`主源 ${h.provider} 取不到版本，已改用镜像 ${a.provider}。`);
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
    setPicked(null); setVersions([]); setVersionsLoaded(false); setInput(''); setHits(null); setMode(null); setSrc(null); setFallback(null);
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

  return (
    <>
      <div className="tp-top">
        <div className="tp-head">
          <span>来源 <span className="count">{installed.length}</span></span>
          <span style={{flex: 1}}/>
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
            <button type="button" className={`tp-icon-btn${mode === 'search' ? ' on' : ''}`} aria-label="搜索物品"
              title="搜索包内物品（本地目录，支持中英文名 / ID / 来源缩写）"
              onClick={() => setMode(m => (m === 'search' ? null : 'search'))}><Icon name="search" size={14}/></button>
            <button type="button" className={`tp-icon-btn${mode === 'add' ? ' on' : ''}`} aria-label="添加模组"
              title="从 CurseForge / Modrinth 添加模组"
              onClick={() => setMode(m => (m === 'add' ? null : 'add'))}><Icon name="plus" size={14}/></button>
          </div>
        </div>
        {mode !== null && (
          <div className="head-dd">
            {mode === 'search' ? <SearchHits q={q}/> : (
              <div className="ss-head">
                <span>双平台搜索</span>
                <span style={{flex: 1}}/>
                <button type="button" className="p-btn" onClick={() => void search()}>搜</button>
              </div>
            )}
            {mode === 'add' && (
              <div className="ss-body">
                {searchError && <div className="p-empty">{searchError}</div>}
                {hits?.length === 0 && !picked && !searchError && <div className="p-empty">没有命中。设置页确认 CurseForge Key 后再试。</div>}
                {!picked && (hits ?? []).map(h => (
                  <div key={`${h.provider}-${h.id}`} className="p-row click" onClick={() => void pick(h)} title={h.summary ?? ''}>
                    <span className="grow">{h.name}</span>
                    <span className="sub">{h.provider}{h.mirror ? ' · 镜像' : ''}</span>
                  </div>
                ))}
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
                          <span className="sub">{others.length} {showAllVersions ? '▾' : '▸'}</span>
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
      <div className="tp-body">
        {error && <div className="p-empty">{error}</div>}
        {builtin.map(m => (
          <div key={m.id} className="p-row click" onClick={() => patch({ns: 'minecraft', mode: 'index', type: null})}
            title="Minecraft 原版 · 点击浏览它贡献的全部物品">
            <span className="dot" style={{background: 'var(--mc-blue)'}}/>
            <span className="grow">{m.displayName}</span>
            <span className="sub">原版</span>
          </div>
        ))}
        {renderGrouped(installed, expanded, setExpanded, toggleMod, patch)}
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

/* 单个模组行：名称行（点击=筛选它贡献的物品）+ 展开（解析状态 + 内容类型下钻）。 */
function ModRow({mod, expanded, onToggleExpand, onToggleStatus, onFilter}: {
  mod: Mod;
  expanded: boolean;
  onToggleExpand: () => void;
  onToggleStatus: () => void;
  onFilter: () => void;
}) {
  return (
    <>
      <div className="p-row click" onClick={onFilter} title={`${mod.displayName} · 点击筛选它贡献的物品`}>
        <button type="button" className="src-chevron" aria-label={expanded ? '收起' : '展开解析内容'}
          onClick={e => { e.stopPropagation(); onToggleExpand(); }}>
          {expanded ? '▾' : '▸'}
        </button>
        <span className="dot" style={{background: DOT[mod.status] ?? 'var(--mc-muted)'}}/>
        <span className="grow" style={{overflow: 'hidden', textOverflow: 'ellipsis'}}>{mod.displayName}</span>
        <button type="button" className="p-btn" disabled={mod.status === 'pending'}
          onClick={e => { e.stopPropagation(); onToggleStatus(); }}>
          {mod.status === 'disabled' ? '启用' : '停用'}
        </button>
      </div>
      {expanded && <CategoryEditor mod={mod}/>}
      {expanded && <ModContent modId={mod.id} ns={mod.canonicalModId}/>}
    </>
  );
}

/* 分类编辑：写 pack_mods.category（真接口），空 = 未分类。 */
function CategoryEditor({mod}: {mod: Mod}) {
  const {packId} = useUrlState();
  const {refresh} = usePackSummary();
  const [value, setValue] = useState(mod.category || '');
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const save = async () => {
    if (!packId) return;
    setBusy(true); setSaved(false);
    try {
      await updateMod(packId, mod.id, {category: value.trim()});
      setSaved(true);
      refresh();
      window.setTimeout(() => setSaved(false), 1500);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="p-row" style={{paddingLeft: 26}}>
      <span className="sub" style={{flex: 'none'}}>分类</span>
      <input className="p-input" style={{flex: 1, minWidth: 0}} value={value} placeholder="如：优化 / 科技（回车保存）"
        onChange={e => setValue(e.target.value)}
        onKeyDown={e => { if (e.key === 'Enter') void save(); }}/>
      <button type="button" className="p-btn" disabled={busy || value.trim() === (mod.category || '')}
        onClick={() => void save()}>{saved ? '✓' : '存'}</button>
    </div>
  );
}

/* 分组渲染（0027）：有多于一个类目时按类目分组出标题；否则平铺不噪声。 */
function renderGrouped(installed: Mod[], expanded: string | null,
  setExpanded: (fn: (x: string | null) => string | null) => void,
  toggleMod: (m: Mod) => Promise<void>,
  patch: (p: Record<string, string | null>, opts?: {push?: boolean}) => void) {
  const groups = new Map<string, Mod[]>();
  for (const m of installed) {
    const key = m.category?.trim() || '';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(m);
  }
  const categorized = [...groups.keys()].filter(k => k !== '');
  if (categorized.length === 0) {
    return installed.map(m => (
      <ModRow key={m.id} mod={m} expanded={expanded === m.id}
        onToggleExpand={() => setExpanded(x => x === m.id ? null : m.id)}
        onToggleStatus={() => void toggleMod(m).catch(() => undefined)}
        onFilter={() => patch({ns: m.canonicalModId, src: null, mode: 'index', type: null})}/>
    ));
  }
  const ordered = [['', ...categorized.sort((a, b) => a.localeCompare(b, 'zh'))]];
  const rows: ReactElement[] = [];
  for (const key of ordered[0]) {
    const list = groups.get(key) ?? [];
    if (list.length === 0) continue;
    if (key !== '') {
      rows.push(<div key={`g-${key}`} className="p-title" style={{padding: '4px 6px 0'}}>📁 {key} <span className="count">{list.length}</span></div>);
    }
    for (const m of list) {
      rows.push(<ModRow key={m.id} mod={m} expanded={expanded === m.id}
        onToggleExpand={() => setExpanded(x => x === m.id ? null : m.id)}
        onToggleStatus={() => void toggleMod(m).catch(() => undefined)}
        onFilter={() => patch({ns: m.canonicalModId, src: null, mode: 'index', type: null})}/>);
    }
  }
  return rows;
}

/* 展开区：解析运行状态 + 内容分组（玩法内容在前，资源折叠在后；取样上限 1000 条）。 */
function ModContent({modId, ns}: {modId: string; ns: string}) {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const [run, setRun] = useState<ModContentRun | null>(null);
  const [kinds, setKinds] = useState<{kind: string; count: number}[]>([]);
  const [sampled, setSampled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [resOpen, setResOpen] = useState(false);

  useEffect(() => {
    if (!packId || !modId) return;
    let alive = true;
    setLoading(true);
    setError(null);
    Promise.all([
      getModContentRun(packId, modId).catch(() => null),
      listModContent(packId, modId, {limit: 1000}).catch(() => null),
    ]).then(([r, list]) => {
      if (!alive) return;
      setRun(r);
      if (list) {
        const counter = new Map<string, number>();
        for (const it of list.items) counter.set(it.kind, (counter.get(it.kind) ?? 0) + 1);
        setKinds([...counter.entries()].map(([kind, count]) => ({kind, count})).sort((a, b) => b.count - a.count));
        setSampled(list.total > list.items.length);
      }
      setLoading(false);
    });
    return () => { alive = false; };
  }, [packId, modId]);

  const refreshCounts = async () => {
    if (!packId) return;
    const list = await listModContent(packId, modId, {limit: 1000}).catch(() => null);
    if (list) {
      const counter = new Map<string, number>();
      for (const it of list.items) counter.set(it.kind, (counter.get(it.kind) ?? 0) + 1);
      setKinds([...counter.entries()].map(([kind, count]) => ({kind, count})).sort((a, b) => b.count - a.count));
      setSampled(list.total > list.items.length);
    }
  };

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
            await refreshCounts();
          }
        } catch { /* 下一轮再试 */ }
      }, 3000);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (loading) return <div className="p-empty" style={{paddingLeft: 26}}>载入解析内容…</div>;

  const gameplay = kinds.filter(k => !RESOURCE_KINDS.has(k.kind));
  const resources = kinds.filter(k => RESOURCE_KINDS.has(k.kind));

  const kindRow = (k: {kind: string; count: number}) => (
    <div key={k.kind} className="p-row click"
      onClick={() => patch({ns, src: modId, mode: 'index', type: k.kind})}
      title={`在索引中浏览这个模组的${kindLabel(k.kind)}`}>
      <span className="grow">{kindLabel(k.kind)}</span>
      <span className="sub">{k.count}{sampled ? '+' : ''}</span>
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
            <span className="sub">{resources.reduce((s, k) => s + k.count, 0)} {resOpen ? '▾' : '▸'}</span>
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

/* 编排态：来源面板整体换成章节 rail（不是并存）。 */
function QuestChapters() {
  const {packId} = useUrlState();
  const [chapters, setChapters] = useState<{id: string; title: string; position: number}[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!packId) return;
    getQuest(packId)
      .then(b => setChapters([...b.revision.draft.chapters].sort((a, b) => a.position - b.position)))
      .catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  return (
    <>
      <div className="tp-head"><span>章节 <span className="count">{chapters.length}</span></span></div>
      <div className="tp-body">
        {error && <div className="p-empty">{error}</div>}
        {chapters.map(c => (
          <div key={c.id} className="p-row"><span className="grow">{c.title || '(未命名章节)'}</span></div>
        ))}
        {chapters.length === 0 && !error && <div className="p-empty">任务书还没有章节。画布（3D 落地）里可以建。</div>}
      </div>
    </>
  );
}
