import {useCatalog} from '../app/CatalogContext';

/* 素面叶子：⌘K 的物品命中。api 的目录域没有服务端搜索端点（只有整份目录），
   所以命中走 CatalogContext 已缓存的那一份在本地过滤 —— 这样检查 J
   「目录加载全站唯一调用点」才守得住。第三步接真实目录搜索后替换。 */
export function StubItemSearch({query, onPick}: {query: string; onPick: (id: string) => void}) {
  const {catalog, refreshing, error} = useCatalog();
  const q = query.trim().toLowerCase();
  if (refreshing) return <div className="palette-empty">目录载入中…</div>;
  if (error) return <div className="palette-empty">{error}</div>;

  const hits = (catalog?.items ?? [])
    .filter(it => it.id.toLowerCase().includes(q) || it.displayName.toLowerCase().includes(q))
    .slice(0, 8);

  if (hits.length === 0) return <div className="palette-empty">没有匹配的物品。</div>;
  return (
    <div>
      {hits.map(it => (
        <div key={it.id} className="palette-item" onClick={() => onPick(it.id)}>
          <span>{it.displayName}</span>
          <span className="pi-sub">{it.id}</span>
        </div>
      ))}
    </div>
  );
}
