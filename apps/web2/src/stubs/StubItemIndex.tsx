import {useSearchParams} from 'react-router-dom';
import {useCatalog} from '../app/CatalogContext';
import {useFocus} from '../app/useFocus';

/* 素面叶子：索引态。第三步换成 ItemIndexPanel（网格 + 图标 + 类型筛选条 + 方向键走格）。
   ns 来自来源栏「贡献 N 物品」，item 焦点由 useFocus 读写。 */
export function StubItemIndex() {
  const [params, setParams] = useSearchParams();
  const ns = params.get('ns');
  const [focus, setFocus] = useFocus();
  const {catalog, refreshing, error, rebuild} = useCatalog();
  if (refreshing) return <div className="rr-line">目录载入中…</div>;
  /* 出错时也要给动作：目录过期（catalog_stale）的唯一出路就是重建，
     只回一行错误文案等于把用户堵死在这里（§5-9 空态必须指向下一个动作）。 */
  if (error) return (
    <div className="rr-line">
      {error}
      <button type="button" className="tb-btn" onClick={() => void rebuild()}>重建目录</button>
    </div>
  );

  const all = catalog?.items ?? [];
  const items = (ns ? all.filter(it => it.id.startsWith(`${ns}:`)) : all).slice(0, 120);

  return (
    <div className="cp-panel">
      <div className="rr-line">
        物品 {all.length} · 配方 {catalog?.recipes.length ?? 0} · 来源 {ns ?? '全部'}
        {ns && <button type="button" className="tb-btn" onClick={() => setParams(p => { const n = new URLSearchParams(p); n.delete('ns'); return n; }, {replace: true})}>清除来源筛选</button>}
      </div>
      <div className="rr-line">
        目录状态 {catalog ? `r${catalog.revision}` : '—'}
        <button type="button" className="tb-btn" onClick={() => void rebuild()}>重建目录</button>
      </div>
      {items.map(it => (
        <div key={it.id} className="rr-line" style={{cursor: 'pointer', color: focus?.id === it.id ? 'var(--mc-primary-deep)' : undefined}}
          onClick={() => setFocus({kind: 'item', id: it.id})}>
          {it.displayName} · {it.id}
        </div>
      ))}
    </div>
  );
}
