import {searchCatalogItems} from './catalogSearch';
import {useCatalog} from '../app/CatalogContext';

/* ⌘K 的物品命中：与索引态同一套搜索纯函数（多语言 + 命名空间命中 + 相关度排序），
   数据用 CatalogContext 已缓存的一份（目录加载全站唯一调用点）。 */
export function StubItemSearch({query, onPick}: {query: string; onPick: (id: string) => void}) {
  const {catalog, refreshing, error} = useCatalog();
  if (refreshing) return <div className="palette-empty">目录载入中…</div>;
  if (error) return <div className="palette-empty">{error}</div>;

  const hits = searchCatalogItems(catalog?.items ?? [], query, null).slice(0, 8);

  if (hits.length === 0) return <div className="palette-empty">没有匹配的物品。</div>;
  return (
    <div>
      {hits.map(({item}) => (
        <div key={item.id} className="palette-item" onClick={() => onPick(item.id)}>
          <span>{item.displayName}</span>
          <span className="pi-sub">{item.id}</span>
        </div>
      ))}
    </div>
  );
}
