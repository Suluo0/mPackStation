import {useEffect, useMemo, useState} from 'react';
import {listModContent, type ModContentItem} from '../api/modContent';
import {createContent, getQuest, saveQuestDraft} from '../api/content';
import {ApiError} from '../api/http';
import {useCatalog} from '../app/CatalogContext';
import {applyFilter, matchNamespace, parseFilterSpec, textMatchItem, tagMatchItem, typeBucket} from '../app/catalogSearch';
import {FilterBuilder} from './FilterBuilder';
import {makeChapter, rid} from './questDraft';
import {useFocus, useUrlPatch, useUrlState} from '../app/url';
import {ItemGrid, ItemQuickView} from './ItemGrid';
import {iconReasonText} from './iconReason';
import {hoverProps} from '../app/hoverTarget';
import {Icon} from '../ui/Icon';
import {useContextMenu, type MenuItem} from '../ui/ContextMenu';

/* 索引态（3A 升级）：网格（图标 + 方向键走格，Enter 进关系）/ 表格两视图（D-3B 决策）。
   搜索框在来源面板的放大镜里（?q= 仍是单一事实源，这里只显示与清除）。 */
const CAP_STEP = 200;

export function ModeIndex() {
  const {q, ns, type, src, view} = useUrlState();
  if (type && src) return <ModContentTable modId={src} kind={type} ns={ns}/>;
  return <CatalogTable q={q} ns={ns} view={view}/>;
}

function CatalogTable({q, ns, view}: {q: string; ns: string | null; view: 'grid' | 'table'}) {
  const patch = useUrlPatch();
  const {packId, f, fmode, sort, dir, fs, chap} = useUrlState();
  const {catalog, refreshing, error} = useCatalog();
  const [focus, setFocus] = useFocus();
  const [cap, setCap] = useState(CAP_STEP);
  const [quick, setQuick] = useState<{id: string; x: number; y: number} | null>(null);
  /* 右键菜单：坐标/关闭/Esc 都在 ui/ContextMenu，这里只给条目。 */
  const menu = useContextMenu();

  /* 物品右键：网格格子和表格行共用同一套动作 —— 同一个对象不该因为换了视图
     就换一套菜单。动作都是页面级的（要写 URL / 清瞬态），所以菜单挂在这里。 */
  const openItemMenu = (e: React.MouseEvent, id: string) =>
    menu.open(e, itemItems(id, e.clientX, e.clientY));
  const gotoLens = (id: string, opts: {mode: 'graph' | 'chain'; rd?: 'in' | null}) => {
    setQuick(null);
    setFocus({kind: 'item', id}, {
      patch: {mode: opts.mode, item: id, ...(opts.mode === 'graph' ? {rd: opts.rd ?? null, r: null, rpath: null} : {})},
    });
  };

  /* 速览浮层要落在右键的位置上，所以坐标从事件带进来，不从菜单取。 */
  const itemItems = (id: string, x: number, y: number): MenuItem[] => {
    const it = catalog?.items.find(v => v.id === id);
    const name = it?.displayName ?? id.split(':').pop() ?? id;
    const copy = (text: string) => { void navigator.clipboard?.writeText(text).catch(() => undefined); };
    return [
      {label: '看合成（R）', action: () => gotoLens(id, {mode: 'graph', rd: null})},
      {label: '看用途（U）', action: () => gotoLens(id, {mode: 'graph', rd: 'in'})},
      {label: '看逆向链路（C）', action: () => gotoLens(id, {mode: 'chain'})},
      {separator: true},
      /* 内容编辑入口（2026-10-05）：R/U 只解决「看」，改配方/进任务书从这里走。 */
      {label: `魔改它的配方`, icon: 'wrench', action: () => { void openRecipeEditor(id, name).catch(() => undefined); }},
      {label: '引入任务书', icon: 'quest', action: () => { void intoQuestBook(id, name).catch(() => undefined); }},
      {separator: true},
      {label: '设为焦点', icon: 'focus', action: () => setFocus({kind: 'item', id})},
      {label: '看速览配方', action: () => setQuick({id, x, y})},
      {separator: true},
      {label: '复制物品 ID', icon: 'copy', action: () => copy(id)},
      {label: '复制名称', icon: 'copy', action: () => copy(name)},
    ];
  };

  /* 浮层开着时，焦点换人（点别的格子 / 方向键）就跟着换内容 ——
     否则浮层会停在旧物品上，和旁边的高亮对不上。滚轮不在此列：网格不拦滚轮，
     滚轮只负责翻页，不该让高亮框跟着跑。 */
  useEffect(() => {
    setQuick(q => (q && focus?.kind === 'item' && focus.id !== q.id ? {...q, id: focus.id} : q));
  }, [focus]);

  const iconUrl = (itemId: string) =>
    `/api/packs/${encodeURIComponent(packId ?? '')}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;

  /* ── 物品 → 内容编辑的两个入口（2026-10-05 用户反馈：R/U 只能看，改要有着落）──
     魔改：为物品建一份预填了 output 的配方草稿，跳到魔改态继续编辑；
     引入任务书：往当前章节（无任务书则连书一起建）追加一个以物品命名的任务节点，
     跳到编排态落点。失败时仍导航过去，由那边的错误条说明原因。 */
  const openRecipeEditor = async (id: string, name: string) => {
    if (!packId) return;
    const d = await createContent(packId, {
      kind: 'recipe', slug: `recipe-${Date.now().toString(36)}`, title: `魔改 ${name}`,
      payload: {schema_version: 1, type: 'crafting', input: [], output: {id, count: 1}},
    });
    patch({mode: 'edit', doc: d.id, item: null});
  };

  const intoQuestBook = async (id: string, name: string) => {
    if (!packId) return;
    const makeNode = (chapterId: string, peers: {x: number; y: number}[]) => ({
      id: rid('node'), chapterId, title: name, description: `引入自目录：${id}`, icon: '',
      x: peers.length ? Math.max(...peers.map(n => n.x)) + 3 : 0,
      y: peers.length ? Math.min(...peers.map(n => n.y)) : 0,
      prerequisites: [], rewards: [], modRefs: [], position: peers.length,
    });
    const b = await getQuest(packId).catch((e: unknown) => (
      e instanceof ApiError && e.status === 404 ? null : Promise.reject(e)));
    if (!b) {
      const ch = makeChapter(0, '第一章');
      const node = makeNode(ch.id, []);
      await saveQuestDraft(packId, 0, {book: {title: '任务书'}, chapters: [ch], nodes: [node], edges: []});
      patch({mode: 'quest', chap: ch.id, node: node.id, qscope: null});
      return;
    }
    const d = b.revision.draft;
    const chapters = [...d.chapters].sort((a, b2) => a.position - b2.position);
    let ch = chapters.find(c => c.id === chap) ?? chapters[0];
    if (!ch) { ch = makeChapter(0, '第一章'); d.chapters.push(ch); }
    const node = makeNode(ch.id, d.nodes.filter(n => n.chapterId === ch.id));
    await saveQuestDraft(packId, b.revision.revision, {...d, nodes: [...d.nodes, node], book: d.book});
    patch({mode: 'quest', chap: ch.id, node: node.id, qscope: null});
  };

  /* 过滤管线：复杂条件（?f=）→ ?ns= 快捷来源 → ?q= 文本/标签词，全部 AND；
     ?q= 以 # 开头时按标签语义（与来源下拉的 # 搜索同一口径——搜索驱动页面）。 */
  const items = useMemo(() => {
    let all = catalog?.items ?? [];
    const {conds, mode} = parseFilterSpec(f, fmode);
    all = applyFilter(all, conds, mode);
    if (ns) all = all.filter(it => it.id.startsWith(`${ns}:`));
    if (q.trim()) {
      const raw = q.trim();
      if (raw.startsWith('#')) all = all.filter(it => tagMatchItem(it, raw.slice(1)));
      else if (raw.startsWith('@')) all = all.filter(it => (it.id.split(':')[0] ?? '').toLowerCase().startsWith(raw.slice(1).toLowerCase()));
      else all = all.filter(it => textMatchItem(it, raw));
    }
    /* 排序：默认排序 = 目录原序（正序）；按名称/按 ID 可再切升序/降序；
       按类型 = 刷怪蛋（*_spawn_egg）聚在最前、其余按 ID —— 目录没有创造栏
       分组数据（0025/0027 都不含），ID 形态推导是第一版最稳的一类。 */
    if (sort === 'name') all = [...all].sort((a, b) => a.displayName.localeCompare(b.displayName, 'zh'));
    if (sort === 'id') all = [...all].sort((a, b) => a.id.localeCompare(b.id));
    if (sort === 'type') all = [...all].sort((a, b) => typeBucket(a) - typeBucket(b) || a.id.localeCompare(b.id));
    if (dir === 'desc') all = [...all].reverse();
    return all;
  }, [catalog, f, fmode, ns, q, sort, dir]);
  /* 当前来源下无结果时，看看全目录里有没有别的命名空间命中（交叉来源提示） */
  const crossHit = useMemo(
    () => (items.length === 0 && ns && q && !q.startsWith('#') ? matchNamespace(catalog?.items ?? [], q) : null),
    [items.length, ns, q, catalog],
  );
  /* 可合成的物品种数 = 有 output 引用的去重物品数。
     这个数才是和「配方数」同量纲的比较对象（1290 > 837），
     目录里的全部物品数（1330）拿它比会看成「配方比物品少」。 */
  const craftable = useMemo(() => {
    const s = new Set<string>();
    for (const r of catalog?.recipes ?? []) {
      for (const ref of r.refs) {
        if (ref.role === 'output' && ref.kind === 'item') s.add(ref.id);
      }
    }
    return s.size;
  }, [catalog]);

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">索引</span>
        {/* 简单搜索：JEI 式前缀（#标签 @来源 /正则 裸词），始终可见 */}
        <input className="p-input ed-search" value={q} placeholder="搜：#标签 @来源 /正则 文本…"
          onChange={e => patch({q: e.target.value || null})}/>
        {q && (
          <button type="button" className="ctx-clear" style={{color: 'var(--mc-muted)'}}
            aria-label="清除搜索" onClick={() => patch({q: null})}>✕</button>
        )}
        {ns && (
          <span className="q-chip" title="来源面板里点模组名设置的快捷筛选">
            来源 {ns}
            <button type="button" className="ctx-clear" style={{color: 'var(--mc-text-2)'}}
              aria-label="清除来源" onClick={() => patch({ns: null, type: null})}>✕</button>
          </span>
        )}
        {/* 高级搜索默认收起来：普通搜索只留一个漏斗图标，点开才展开复杂过滤器。
            以前那个常驻的「高级」开关白占地方，而且一进索引就摊着多条件面板的入口。 */}
        <button type="button" className={`p-btn${fs === '1' ? ' on-view' : ''}`}
          title={fs === '1' ? '收起高级搜索' : '高级搜索（多条件组合）'}
          aria-pressed={fs === '1'}
          onClick={() => patch({fs: fs === '1' ? null : '1'})}>
          <Icon name="filter" size={12}/>
        </button>
        {fs === '1' && (
          <span className="q-chip" title="高级搜索已开启">
            高级<button type="button" className="ctx-clear" style={{color: 'var(--mc-text-2)'}}
              aria-label="关闭高级搜索" onClick={() => patch({fs: null})}>✕</button>
          </span>
        )}
        {fs === '1' && <FilterBuilder/>}
        <select className="p-input" value={sort} onChange={e => patch({sort: e.target.value === 'def' ? null : e.target.value})} style={{width: 92}} title="排序">
          <option value="def">默认排序</option>
          <option value="name">按名称</option>
          <option value="id">按 ID</option>
          <option value="type">按类型</option>
        </select>
        {/* 方向切换：只显示一个箭头，升/降序切换时旧箭头滑出、新箭头滑入（.dir-arrow 动效），
            当前显示的箭头本身就是排序状态。两个 icon 都保留在 DOM 里，靠 data-dir 决定谁在位。 */}
        <button type="button" className="dir-btn"
          title={dir === 'asc' ? '当前：升序，点击切到降序' : '当前：降序，点击切到升序'}
          aria-label={dir === 'asc' ? '当前升序，点击切到降序' : '当前降序，点击切到升序'}
          onClick={() => patch({dir: dir === 'asc' ? 'desc' : null})}>
          <span className="dir-arrow" data-dir={dir}>
            <span className="dir-a dir-down"><Icon name="sortdown" size={13}/></span>
            <span className="dir-a dir-up"><Icon name="sortup" size={13}/></span>
          </span>
        </button>
        <span className="grow"/>
        {/* 计数口径写全：1330 是目录里的**全部**物品（原矿、方块、掉落这些不可合成的都在里面），
            所以它必然大于配方数；可合成的输出只有 837 个，配方 1290 条 ——
            1290 > 837 才对。只写「物品 / 配方」两个数会看成「配方比物品少，是不是漏了」。
            末尾的 r14751 是目录版本号（只增计数器），和数据量无关，已去掉。 */}
        <span className="ed-count" title="物品 = 目录全部物品（含不可合成）；可合成 = 有配方输出的物品种数；配方 = 配方条目数（同一物品可能有多条）">
          物品 {items.length}{catalog ? ` / ${catalog.items.length}` : ''}
          {catalog && ` · 可合成 ${craftable} · 配方 ${catalog.recipes.length}`}
        </span>
        <span className="view-switch">
          <button type="button" className={`p-btn${view === 'grid' ? ' on-view' : ''}`} onClick={() => patch({view: null})}>网格</button>
          <button type="button" className={`p-btn${view === 'table' ? ' on-view' : ''}`} onClick={() => patch({view: 'table'})}>表格</button>
        </span>
        {refreshing && <span className="sub">目录载入中…</span>}
      </div>
      <div className="ed-scroll">
        {error && (
          <div className="ed-placeholder">
            <b>{error}</b>
            <div>目录过期（catalog_stale）的唯一出路是重建 —— 入口已挪到左侧「构建」。</div>
            <div style={{marginTop: 8}}>
              <button type="button" className="p-btn primary" onClick={() => patch({tool: 'build'})}>去构建面板重建</button>
            </div>
          </div>
        )}
        {!error && view === 'grid' && (
          <ItemGrid packId={packId ?? ''} items={items} selectedId={focus?.kind === 'item' ? focus.id : null}
            onSelect={id => setFocus({kind: 'item', id})}
            onQuick={(id, anchor) => setQuick({id, x: anchor.x, y: anchor.y})}
            onContextMenu={openItemMenu}
            onOpen={id => { setFocus({kind: 'item', id}); patch({mode: 'graph'}); }}/>
        )}
        {quick && (
          <ItemQuickView packId={packId ?? ''} itemId={quick.id} anchor={quick}
            onClose={() => setQuick(null)}
            onOpenGraph={(id, rd) => {
              /* 焦点 + 透镜 + 方向一次写完，别分两次导航。 */
              setFocus({kind: 'item', id}, {patch: {mode: 'graph', rd: rd === 'in' ? 'in' : null, r: null, rpath: null}});
              setQuick(null);
            }}/>
        )}
        {!error && view === 'table' && (
          <>
            <table className="ed-table" {...hoverProps()}>
              <thead>
                <tr>
                  <th className="ed-th-icon"/>
                  <th style={{width: '36%'}}>名称</th><th>ID</th><th style={{width: 120}}>来源</th>
                </tr>
              </thead>
              <tbody>
                {items.slice(0, cap).map(it => (
                  <tr key={it.id} data-hover-item={it.id}
                    className={`click${focus?.kind === 'item' && focus.id === it.id ? ' on' : ''}`}
                    onContextMenu={e => openItemMenu(e, it.id)}
                    onClick={e => {
                      setFocus({kind: 'item', id: it.id});
                      /* 表格里也单击即出速览 —— 和网格同一套手势，不按视图分裂交互。 */
                      const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
                      setQuick({id: it.id, x: r.left, y: r.bottom});
                    }}>
                    <td className="ed-td-icon">
                      {it.iconStatus === 'ready'
                        ? <img src={iconUrl(it.id)} alt="" loading="lazy"/>
                        : <span className="ed-td-ph" title={iconReasonText(it.iconReason)}>{it.displayName.slice(0, 1)}</span>}
                    </td>
                    <td>{it.displayName}</td>
                    <td className="mono">{it.id}</td>
                    <td className="mono">{it.id.split(':')[0]}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {items.length > cap && (
              <div style={{padding: '10px 4px', display: 'flex', gap: 8, alignItems: 'center'}}>
                <span className="sub" style={{color: 'var(--mc-muted)'}}>已显示前 {cap} / 共 {items.length}</span>
                <button type="button" className="p-btn" onClick={() => setCap(c => c + CAP_STEP)}>显示更多</button>
              </div>
            )}
          </>
        )}
        {!error && items.length === 0 && (
          <div className="ed-placeholder">
            当前来源{ns ? `（${ns}）` : ''}没有匹配「{q}」的物品。
            {crossHit && (
              <div style={{marginTop: 8}}>
                来源 <b>{crossHit.ns}</b> 里有 {crossHit.count} 个命中。
                <button type="button" className="p-btn primary" style={{marginLeft: 8}}
                  onClick={() => patch({ns: null, type: null})}>在全部来源中搜「{q}」</button>
              </div>
            )}
            {!crossHit && <div>换个关键词试试：支持 ID、中文名、英文名、来源缩写（如 ae2）。</div>}
          </div>
        )}
      </div>

      {menu.menu}
    </>
  );
}

/* 模组解析内容面（来源树下钻进入）：某模组某类 kind 的解析条目。
   ?src= 是内部模组 id（内容 API 路径参数），?ns= 是目录命名空间（显示用）。 */
function ModContentTable({modId, kind, ns}: {modId: string; kind: string; ns: string | null}) {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const [items, setItems] = useState<ModContentItem[]>([]);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [cap, setCap] = useState(CAP_STEP);

  useEffect(() => {
    if (!packId) return;
    listModContent(packId, modId, {kind, limit: 1000})
      .then(r => { setItems(r.items); setTotal(r.total); setError(null); })
      .catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId, modId, kind]);

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">索引</span>
        <span className="sub">{ns ?? modId} · {kind} · {total} 条{total > items.length ? `（取前 ${items.length}）` : ''}</span>
        <span className="grow"/>
        <button type="button" className="p-btn" onClick={() => patch({type: null})}>看物品目录</button>
        <button type="button" className="p-btn" onClick={() => patch({type: null, ns: null, src: null})}>清除来源</button>
      </div>
      <div className="ed-scroll">
        {error && <div className="ed-placeholder"><b>{error}</b></div>}
        {!error && items.length === 0 && (
          <div className="ed-placeholder">这个模组没有 {kind} 类解析内容。回来源树重新解析试试。</div>
        )}
        {!error && items.length > 0 && (
          <table className="ed-table">
            <thead>
              <tr><th style={{width: '34%'}}>key</th><th>路径</th><th style={{width: 90}}>动态</th></tr>
            </thead>
            <tbody>
              {items.slice(0, cap).map(it => (
                <tr key={it.id} title={it.parseError ?? ''}>
                  <td className="mono">{it.key || it.id}</td>
                  <td className="mono">{it.path}</td>
                  <td>{it.isDynamic ? <span className="sub">动态/特殊</span> : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {!error && items.length > cap && (
          <div style={{padding: '10px 4px'}}>
            <button type="button" className="p-btn" onClick={() => setCap(c => c + CAP_STEP)}>显示更多（{items.length - cap}）</button>
          </div>
        )}
      </div>
    </>
  );
}
