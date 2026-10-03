import {useRef} from 'react';
import type {CatalogItem} from '../api/catalog';
import {useCatalog} from '../app/CatalogContext';

/* 物品网格（3A）：图标 + 名称，方向键走格（焦点即选中，URL 同步），
   Enter 进关系态。图标仅对 iconStatus==='ready' 的物品请求，缺失显示首字占位。 */
export function ItemGrid({packId, items, selectedId, onSelect, onQuick, onOpen}: {
  packId: string;
  items: CatalogItem[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onQuick: (id: string, anchor: {x: number; y: number}) => void;
  onOpen: (id: string) => void;
  limit?: number;
}) {
  const gridRef = useRef<HTMLDivElement>(null);

  if (items.length === 0) {
    return <div className="ed-placeholder">没有匹配的物品。</div>;
  }

  const iconUrl = (itemId: string) =>
    `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;

  const move = (delta: number) => {
    const next = Math.min(items.length - 1, Math.max(0, delta));
    onSelect(items[next].id);
    // 走格后把目标滚进视野
    requestAnimationFrame(() => {
      gridRef.current?.querySelector<HTMLElement>(`[data-item-id="${CSS.escape(items[next].id)}"]`)
        ?.scrollIntoView({block: 'nearest'});
    });
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const grid = gridRef.current;
    if (!grid) return;
    const tile = grid.querySelector<HTMLElement>('.ig-tile');
    const tileW = tile ? tile.offsetWidth + 8 : 72;
    const cols = Math.max(1, Math.floor(grid.clientWidth / tileW));
    const cur = Math.max(0, items.findIndex(it => it.id === selectedId));
    switch (e.key) {
      case 'ArrowRight': e.preventDefault(); move(cur + 1); break;
      case 'ArrowLeft': e.preventDefault(); move(cur - 1); break;
      case 'ArrowDown': e.preventDefault(); move(cur + cols); break;
      case 'ArrowUp': e.preventDefault(); move(cur - cols); break;
      case 'Home': e.preventDefault(); move(0); break;
      case 'End': e.preventDefault(); move(items.length - 1); break;
      case 'Enter':
        if (selectedId) { e.preventDefault(); onOpen(selectedId); }
        break;
      default:
        break;
    }
  };

  return (
    <div className="ig-wrap">
      <div className="ig-grid" ref={gridRef} tabIndex={0} role="grid" aria-label="物品网格"
        onKeyDown={onKeyDown}>
        {items.map(it => {
          const ready = it.iconStatus === 'ready';
          const on = selectedId === it.id;
          return (
            <button type="button" key={it.id} data-item-id={it.id}
              className={`ig-tile${on ? ' on' : ''}`}
              title={`${it.displayName}\n${it.id}（双击速览配方）${it.iconReason ? `\n${it.iconReason}` : ""}`}
              onClick={() => onSelect(it.id)}
              onDoubleClick={e => {
                const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
                onQuick(it.id, {x: r.left, y: r.bottom});
              }}>
              <span className="ig-icon">
                {ready
                  ? <img src={iconUrl(it.id)} alt="" loading="lazy"/>
                  : <span className="ig-ph">{it.displayName.slice(0, 1)}</span>}
              </span>
              <span className="ig-name">{it.displayName}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** 速览浮层（第八轮反馈）：双击物品从格子向下弹出，不顶掉页面。
    15 行内滚动（.qv-body max-height）；行点击进关系态，✕/Esc 关闭。 */
export function ItemQuickView({packId, itemId, anchor, onClose, onOpenGraph}: {
  packId: string;
  itemId: string;
  anchor: {x: number; y: number};
  onClose: () => void;
  onOpenGraph: (id: string) => void;
}) {
  const {itemById, recipesByOutput, recipesByInput} = useCatalog();
  const item = itemById.get(itemId);
  const outs = recipesByOutput.get(itemId) ?? [];
  const ins = recipesByInput.get(itemId) ?? [];
  const top = Math.min(anchor.y + 8, Math.max(8, window.innerHeight - 470));
  const left = Math.min(Math.max(anchor.x - 200, 8), Math.max(8, window.innerWidth - 450));
  const iconUrl = `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;

  return (
    <div className="qv" style={{top, left}} onKeyDown={e => { if (e.key === 'Escape') onClose(); }}>
      <div className="qv-head">
        {item && item.iconStatus === 'ready' && <img className="ss-icon" alt="" src={iconUrl}/>}
        <span style={{fontWeight: 600}}>{item?.displayName ?? itemId}</span>
        <span className="mono" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 11.5, color: 'var(--mc-muted)'}}>{itemId}</span>
        <span style={{flex: 1}}/>
        <button type="button" className="p-btn" onClick={() => onOpenGraph(itemId)}>看关系</button>
        <button type="button" className="ctx-clear" style={{color: 'var(--mc-muted)'}} aria-label="关闭" onClick={onClose}>✕</button>
      </div>
      <div className="qv-body">
        <div className="p-title">作为产物 <span className="count">{outs.length}</span></div>
        {outs.map(r => (
          <div key={`o-${r.id}`} className="p-row click" onClick={() => onOpenGraph(itemId)} title="进关系态看这条配方">
            <span className="grow">{r.type}</span>
            <span className="mono" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 11.5, color: 'var(--mc-muted)'}}>{r.id}</span>
          </div>
        ))}
        {outs.length === 0 && <div className="p-empty">没有以它为产物的配方。</div>}
        <div className="p-title" style={{marginTop: 6}}>作为原料 <span className="count">{ins.length}</span></div>
        {ins.map(r => {
          const other = r.refs.find(x => x.id !== itemId && x.role === 'output');
          return (
            <div key={`i-${r.id}`} className="p-row click" title={other ? `进关系态居中 ${other.id}` : '进关系态'}
              onClick={() => onOpenGraph(itemId)}>
              <span className="grow">{r.type}</span>
              <span className="mono" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 11.5, color: 'var(--mc-muted)'}}>{other?.id ?? r.id}</span>
            </div>
          );
        })}
        {ins.length === 0 && <div className="p-empty">没有以它为原料的配方。</div>}
      </div>
    </div>
  );
}
