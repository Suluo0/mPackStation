import {useMemo, useRef, useState} from 'react';
import {useCatalog} from './CatalogContext';
import {searchCatalogItems} from './catalogSearch';
import {ItemIcon} from './ItemIcon';

/* 物品 ID 输入框（共享组件）：输入即搜的下拉选择。

   「远程搜索」在这里是伪需求 —— 全量目录已经一次性下发并缓存在
   useCatalog 里（CatalogContext 的口径），客户端检索零延迟、零流量，
   所以候选直接从内存出，走 searchCatalogItems 的六档相关度。

   行首带图标：光看名字分不出「橡木木板」和「云杉木板」（StubItemSearch
   同一条结论），图标是最快的区分手段。

   键盘：↑↓ 移动高亮，Enter 采纳高亮项（无高亮则保留手输值），
   Esc 关闭下拉。点选行用 onMouseDown（抢在 blur 关闭之前）。

   手输了一个目录里不存在的 ID 时，下拉给一条提示，别让用户以为搜不到 = 输对了。 */
export function ItemIdInput({value, onChange, placeholder}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const {catalog, packId, refreshing} = useCatalog();
  const [open, setOpen] = useState(false);
  const [hi, setHi] = useState(-1);
  const blurTimer = useRef<number | null>(null);

  const hits = useMemo(() => {
    const q = value.trim();
    if (!q) return [];
    return searchCatalogItems(catalog?.items ?? [], q, null).slice(0, 12);
  }, [catalog, value]);

  const unknown = value.trim() !== '' && hits.length === 0 && !refreshing;

  const adopt = (id: string) => {
    onChange(id);
    setOpen(false);
    setHi(-1);
  };

  return (
    <span className="si-wrap">
      <input
        className="p-input"
        value={value}
        placeholder={placeholder}
        role="combobox" aria-expanded={open && (hits.length > 0 || unknown)}
        onChange={e => { onChange(e.target.value.trim()); setOpen(true); setHi(-1); }}
        onFocus={() => { if (blurTimer.current !== null) { window.clearTimeout(blurTimer.current); blurTimer.current = null; } setOpen(true); }}
        onBlur={() => { blurTimer.current = window.setTimeout(() => setOpen(false), 150); }}
        onKeyDown={e => {
          if (!open || hits.length === 0) {
            if (e.key === 'Escape') setOpen(false);
            return;
          }
          if (e.key === 'ArrowDown') { e.preventDefault(); setHi(h => Math.min(h + 1, hits.length - 1)); }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(h - 1, -1)); }
          else if (e.key === 'Enter') { e.preventDefault(); adopt((hi >= 0 ? hits[hi] : hits[0]).item.id); }
          else if (e.key === 'Escape') { setOpen(false); setHi(-1); }
        }}/>
      {open && (hits.length > 0 || unknown) && (
        <div className="si-dd head-dd" role="listbox">
          {hits.map(({item}, i) => (
            <div key={item.id}
              role="option" aria-selected={i === hi}
              className={`p-row click${i === hi ? ' on' : ''}`}
              style={i === hi ? {background: 'var(--mc-hover)'} : undefined}
              onMouseDown={e => { e.preventDefault(); adopt(item.id); }}
              onMouseEnter={() => setHi(i)}>
              <ItemIcon packId={packId} item={item}/>
              <span className="grow">{item.displayName}</span>
              <span className="sub">{item.id}</span>
            </div>
          ))}
          {unknown && (
            <div className="p-row na">
              <span className="sub">目录里没有「{value.trim()}」—— 检查拼写，或先在索引态确认该物品已解析。</span>
            </div>
          )}
        </div>
      )}
    </span>
  );
}
