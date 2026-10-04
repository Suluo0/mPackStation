import type {CatalogItem} from '../api/catalog';
import {iconUrl} from '../editor/recipeUtils';
import {iconReasonText} from '../editor/iconReason';

/* 列表行首的物品图标（网格/表格/搜索命中/命令面板共用一套外观）。
   只有 catalog 里 iconStatus==='ready' 的才有图；其余给首字占位并把
   后端渲染器给的原因挂进 title —— 「没有」和「为什么没有」是两件事。 */
export function ItemIcon({packId, item, size = 20, className = 'row-icon'}: {
  packId: string | null;
  item: CatalogItem;
  size?: number;
  className?: string;
}) {
  if (item.iconStatus === 'ready') {
    return <img className={className} src={iconUrl(packId ?? '', item.id)} alt=""
      loading="lazy" width={size} height={size}/>;
  }
  const why = iconReasonText(item.iconReason);
  return (
    <span className={className} title={why || '目录里没有这一项'}
      style={{display: 'inline-flex', alignItems: 'center', justifyContent: 'center', flex: 'none'}}>
      {item.displayName.slice(0, 1)}
    </span>
  );
}
