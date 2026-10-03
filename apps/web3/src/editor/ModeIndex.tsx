import {useEffect, useMemo, useState} from 'react';
import {listModContent, type ModContentItem} from '../api/modContent';
import {useCatalog} from '../app/CatalogContext';
import {applyFilter, matchNamespace, parseFilterSpec, textMatchItem, tagMatchItem} from '../app/catalogSearch';
import {FilterBuilder} from './FilterBuilder';
import {useFocus, useUrlPatch, useUrlState} from '../app/url';
import {ItemGrid, ItemQuickView} from './ItemGrid';
import {Icon} from '../app/Icon';

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
  const {packId, f, fmode, sort, dir} = useUrlState();
  const {catalog, refreshing, error, rebuild} = useCatalog();
  const [focus, setFocus] = useFocus();
  const [cap, setCap] = useState(CAP_STEP);
  const [quick, setQuick] = useState<{id: string; x: number; y: number} | null>(null);

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
    /* 排序：默认排序 = 目录原序（正序）；按名称/按 ID 可再切升序/降序 */
    if (sort === 'name') all = [...all].sort((a, b) => a.displayName.localeCompare(b.displayName, 'zh'));
    if (sort === 'id') all = [...all].sort((a, b) => a.id.localeCompare(b.id));
    if (dir === 'desc') all = [...all].reverse();
    return all;
  }, [catalog, f, fmode, ns, q, sort, dir]);
  /* 当前来源下无结果时，看看全目录里有没有别的命名空间命中（交叉来源提示） */
  const crossHit = useMemo(
    () => (items.length === 0 && ns && q && !q.startsWith('#') ? matchNamespace(catalog?.items ?? [], q) : null),
    [items.length, ns, q, catalog],
  );

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">索引</span>
        {q && (
          <span className="q-chip" title="搜索框在来源面板的放大镜里">
            搜索 “{q}”
            <button type="button" className="ctx-clear" style={{color: 'var(--mc-text-2)'}}
              aria-label="清除搜索" onClick={() => patch({q: null})}>✕</button>
          </span>
        )}
        {ns && (
          <span className="q-chip" title="来源面板里点模组名设置的快捷筛选">
            来源 {ns}
            <button type="button" className="ctx-clear" style={{color: 'var(--mc-text-2)'}}
              aria-label="清除来源" onClick={() => patch({ns: null, type: null})}>✕</button>
          </span>
        )}
        <FilterBuilder/>
        <select className="p-input" value={sort} onChange={e => patch({sort: e.target.value === 'def' ? null : e.target.value})} style={{width: 92}} title="排序">
          <option value="def">默认排序</option>
          <option value="name">按名称</option>
          <option value="id">按 ID</option>
        </select>
        {/* 方向切换：一个标准按钮，左半↓=降序、右半↑=升序，哪个方向亮哪一半 */}
        <button type="button" className="dir-btn"
          title={dir === 'asc' ? '当前：升序，点击切到降序' : '当前：降序，点击切到升序'}
          onClick={() => patch({dir: dir === 'asc' ? 'desc' : null})}>
          <span className={dir === 'desc' ? 'on' : ''}><Icon name="sortdown" size={13}/></span>
          <span className={dir === 'asc' ? 'on' : ''}><Icon name="sortup" size={13}/></span>
        </button>
        <span className="grow"/>
        <span className="ed-count">
          物品 {items.length}{catalog ? ` / ${catalog.items.length}` : ''} · 配方 {catalog?.recipes.length ?? 0}{catalog ? ` · r${catalog.revision}` : ''}
        </span>
        <span className="view-switch">
          <button type="button" className={`p-btn${view === 'grid' ? ' on-view' : ''}`} onClick={() => patch({view: null})}>网格</button>
          <button type="button" className={`p-btn${view === 'table' ? ' on-view' : ''}`} onClick={() => patch({view: 'table'})}>表格</button>
        </span>
        {refreshing && <span className="sub">目录载入中…</span>}
        <button type="button" className="p-btn" onClick={() => void rebuild()}>重建目录</button>
      </div>
      <div className="ed-scroll">
        {error && (
          <div className="ed-placeholder">
            <b>{error}</b>
            <div>目录过期（catalog_stale）的唯一出路是重建。</div>
            <div style={{marginTop: 8}}><button type="button" className="p-btn primary" onClick={() => void rebuild()}>重建目录</button></div>
          </div>
        )}
        {!error && view === 'grid' && (
          <ItemGrid packId={packId ?? ''} items={items} selectedId={focus?.kind === 'item' ? focus.id : null}
            onSelect={id => setFocus({kind: 'item', id})}
            onQuick={(id, anchor) => setQuick({id, x: anchor.x, y: anchor.y})}
            onOpen={id => { setFocus({kind: 'item', id}); patch({mode: 'graph'}); }}/>
        )}
        {quick && (
          <ItemQuickView packId={packId ?? ''} itemId={quick.id} anchor={quick}
            onClose={() => setQuick(null)}
            onOpenGraph={id => { setFocus({kind: 'item', id}); setQuick(null); patch({mode: 'graph'}); }}/>
        )}
        {!error && view === 'table' && (
          <>
            <table className="ed-table">
              <thead>
                <tr><th style={{width: '38%'}}>名称</th><th>ID</th><th style={{width: 120}}>来源</th></tr>
              </thead>
              <tbody>
                {items.slice(0, cap).map(it => (
                  <tr key={it.id} className={`click${focus?.kind === 'item' && focus.id === it.id ? ' on' : ''}`}
                    onClick={() => setFocus({kind: 'item', id: it.id})}>
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
