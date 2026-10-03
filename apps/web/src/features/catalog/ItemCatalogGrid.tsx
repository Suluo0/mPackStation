import type {CatalogItem} from '../../api/catalog';
import './item-catalog.css';

/* 物品网格：物品大一统的展示核心，浏览页与选择器共用（DRY）。
   纯展示组件——数据加载、过滤、详情由调用方负责。 */

export type ItemCatalogGridProps = {
  items: CatalogItem[];
  /** 返回物品图标 URL；仅对 iconStatus==='ready' 的物品调用。 */
  iconUrl: (itemId: string) => string;
  selectedId?: string | null;
  onSelect?: (itemId: string) => void;
  emptyText?: string;
  /** 单次最多渲染的格子数，避免上千物品一次性铺满 DOM。 */
  limit?: number;
};

export function ItemCatalogGrid({
  items, iconUrl, selectedId, onSelect,
  emptyText = '没有匹配的物品。', limit = 600,
}: ItemCatalogGridProps) {
  if (!items.length) return <div className="icg-empty">{emptyText}</div>;
  const shown = items.slice(0, limit);
  return (
    <div className="icg-wrap">
      <div className="icg-grid">
        {shown.map(it => {
          const icon = it.iconStatus === 'ready' ? iconUrl(it.id) : null;
          return (
            <button
              type="button"
              key={it.id}
              className={selectedId === it.id ? 'icg-tile is-active' : 'icg-tile'}
              onClick={() => onSelect?.(it.id)}
              title={it.id}
            >
              <span className="icg-icon">
                {icon
                  ? <img src={icon} alt="" loading="lazy"/>
                  : <span className="icg-icon-ph">{it.displayName.slice(0, 1)}</span>}
              </span>
              <span className="icg-name">{it.displayName}</span>
            </button>
          );
        })}
      </div>
      {items.length > limit && (
        <div className="icg-more">仅显示前 {limit} 个，共 {items.length} 个 —— 用搜索或命名空间筛选缩小范围。</div>
      )}
    </div>
  );
}
