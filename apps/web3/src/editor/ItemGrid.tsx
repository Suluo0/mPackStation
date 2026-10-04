import {useEffect, useMemo, useRef} from 'react';
import type {CatalogItem} from '../api/catalog';
import {useCatalog} from '../app/CatalogContext';
import {collectRecipes, makeRecipeViewCache, tagLabel, type IngredientSlot, type RecipeHit} from './recipeView';
import {shownCandidate} from './recipeUtils';
import {iconReasonText} from './iconReason';
import {hoverProps, setHoverTarget} from '../app/hoverTarget';

/* 物品网格（3A）：图标 + 名称，方向键走格（焦点即选中，URL 同步），
   单击立刻出速览浮层，Enter 进关系态。图标仅对 iconStatus==='ready' 的物品请求，
   缺失的用首字占位并挂上渲染器给的原因。 */
export function ItemGrid({packId, items, selectedId, onSelect, onQuick, onOpen, onContextMenu}: {
  packId: string;
  items: CatalogItem[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onQuick: (id: string, anchor: {x: number; y: number}) => void;
  onOpen: (id: string) => void;
  /* 右键格子 = 对这个物品的动作（看合成/看用途/复制 id…）。由索引态提供，
     因为菜单里那些动作要改 URL，属于页面级的事，不是网格自己的。 */
  onContextMenu?: (e: React.MouseEvent, id: string) => void;
  limit?: number;
}) {
  const gridRef = useRef<HTMLDivElement>(null);

  const reveal = (id: string) => {
    requestAnimationFrame(() => {
      gridRef.current?.querySelector<HTMLElement>(`[data-item-id="${CSS.escape(id)}"]`)
        ?.scrollIntoView({block: 'nearest'});
    });
  };

  /* 网格被卸载（切态、切来源）时撤掉悬停登记，别让 R/U 打在一个看不见的物品上。 */
  useEffect(() => () => setHoverTarget(null), []);

  /* 网格上刻意不挂滚轮。曾经把滚轮映射成「在物品间前后移动」（模仿 JEI 的物品列表），
     代价是首屏一滚鼠标、高亮框就自己跳 —— 用户没点任何东西，滚轮本该用来翻页浏览。
     焦点框（.ig-tile.on）只由显式操作产生：点格子，或按方向键走格。
     关系态槽位的滚轮轮换是另一回事：那个槽确实有多个并列候选可换，拦它是有明确
     对象的，见 RecipeCard.SlotCell —— 不要因为这里删了就顺手把那里也删掉。 */
  if (items.length === 0) {
    return <div className="ed-placeholder">没有匹配的物品。</div>;
  }

  const iconUrl = (itemId: string) =>
    `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;

  const move = (delta: number) => {
    const next = Math.min(items.length - 1, Math.max(0, delta));
    onSelect(items[next].id);
    reveal(items[next].id);
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
        onKeyDown={onKeyDown} {...hoverProps()}>
        {items.map(it => {
          const ready = it.iconStatus === 'ready';
          const on = selectedId === it.id;
          const why = iconReasonText(it.iconReason);
          return (
            <button type="button" key={it.id} data-item-id={it.id}
              data-hover-item={it.id}
              className={`ig-tile${on ? ' on' : ''}`}
              title={`${it.displayName}\n${it.id}`
                + `\n单击速览配方 · R 看合成 · U 看用途`
                + (ready ? '' : `\n没有图标：${why || '目录里没有这一项'}`)}
              onClick={e => {
                /* 第一下单击就出速览 —— 先选中、再双击是多余的一步。 */
                onSelect(it.id);
                const r = e.currentTarget.getBoundingClientRect();
                onQuick(it.id, {x: r.left, y: r.bottom});
              }}
              onContextMenu={e => onContextMenu?.(e, it.id)}>
              <span className="ig-icon">
                {ready
                  ? <img src={iconUrl(it.id)} alt="" loading="lazy"/>
                  : <span className="ig-ph" title={why}>{it.displayName.slice(0, 1)}</span>}
              </span>
              <span className="ig-name">{it.displayName}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** 速览浮层：单击物品即弹出（不顶掉页面），列「怎么合成 / 能干什么」两条路。
    每行是一条**配方**，显示产物图标 + 产物名称（按当前语言解析）+ 机器 + 原料摘要 ——
    以前这里直接把配方 type 和配方 id 甩出来，等于让用户读 ID。
    R/U 进关系态对应方向；✕ / Esc / 点外面关闭。 */
export function ItemQuickView({packId, itemId, anchor, onClose, onOpenGraph}: {
  packId: string;
  itemId: string;
  anchor: {x: number; y: number};
  onClose: () => void;
  onOpenGraph: (id: string, rd: 'out' | 'in') => void;
}) {
  const {itemById, tagById, recipesByOutput, recipesByInput} = useCatalog();
  const boxRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  /* 点浮层外面 / 按 Esc 都关掉。监听挂在 document 上，
     mousedown 先于网格的 click，所以「点另一个物品」会先关旧的再开新的。 */
  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      const el = boxRef.current;
      if (el && !el.contains(e.target as Node)) closeRef.current();
    };
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') closeRef.current(); };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [itemId]);

  const viewOf = useMemo(() => makeRecipeViewCache({itemById, tagById}), [itemById, tagById]);
  const item = itemById.get(itemId);
  const tagKeys = useMemo(() => item?.tags ?? [], [item]);
  const outs = useMemo(() => collectRecipes(recipesByOutput.get(itemId), recipesByOutput, tagKeys),
    [itemId, recipesByOutput, tagKeys]);
  const ins = useMemo(() => collectRecipes(recipesByInput.get(itemId), recipesByInput, tagKeys),
    [itemId, recipesByInput, tagKeys]);

  const top = Math.min(anchor.y + 8, Math.max(8, window.innerHeight - 470));
  const left = Math.min(Math.max(anchor.x - 240, 8), Math.max(8, window.innerWidth - 560));
  const icon = (id: string) => `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(id)}`;

  /* 一行配方：产物（图标 + 名称）×N · 机器 · 原料代表图标。 */
  const row = (hit: RecipeHit) => {
    const v = viewOf(hit.rec);
    const outId = v.output ? shownCandidate(v.output, {}) : null;
    const outItem = outId ? itemById.get(outId) : undefined;
    const outName = outId ? (outItem?.displayName ?? outId.split(':').pop() ?? outId) : null;
    const slots = v.kind === 'grid'
      ? v.grid.filter((s): s is IngredientSlot => s !== null)
      : v.inputs;
    /* 摘要按「槽位接受什么」合并，不按具体物品：木桶是「任意木板 ×6 + 任意木台阶 ×2」，
       把 acacia_planks 单独拎出来说反而在暗示「必须金合欢木板」。 */
    const counts = new Map<string, {n: number; tagId: string | null; rep: string}>();
    for (const s of slots) {
      const rep = shownCandidate(s, {});
      if (!rep) continue;
      const key = s.tagId ?? rep;
      const hit = counts.get(key);
      if (hit) hit.n += Math.max(1, s.count);
      else counts.set(key, {n: Math.max(1, s.count), tagId: s.tagId, rep});
    }
    const reps = [...counts.values()];
    const tip = [
      v.machine,
      hit.rec.id,
      hit.viaTag ? `经标签 ${hit.viaTag} 命中` : '',
      '点击进关系态看这条配方',
    ].filter(Boolean).join('\n');
    return (
      <div className="qv-row" key={hit.rec.id} title={tip} onClick={() => onOpenGraph(itemId, 'out')}>
        <span className="qv-out">
          {outItem?.iconStatus === 'ready'
            ? <img src={icon(outId!)} alt="" loading="lazy"/>
            : <span className="qv-ph">{(outName ?? '?').slice(0, 1)}</span>}
        </span>
        <span className="qv-text">
          <span className="qv-name">
            {outName ?? v.outputNote ?? '产物由运行时决定'}
            {v.output && v.output.count > 1 ? ` ×${v.output.count}` : ''}
          </span>
          <span className="qv-sub">
            {v.machine}
            {v.kind === 'rule' ? ' · 规则类' : ''}
            {hit.viaTag ? ` · 经 #${hit.viaTag.split(':').pop()}` : ''}
          </span>
        </span>
        <span className="qv-ing">
          {reps.slice(0, 4).map(({n, tagId, rep}) => {
            const ri = itemById.get(rep);
            const nm = ri?.displayName ?? rep.split(':').pop() ?? rep;
            const label = tagId ? tagLabel(tagById.get(tagId), tagId, itemById) : nm;
            const kinds = tagId ? (tagById.get(tagId)?.members.length ?? 0) : 0;
            const title = tagId
              ? `任意「${label}」：${kinds} 种同类物品任选一种${n > 1 ? ` · 共 ${n} 个` : ''}`
              : `${nm}${n > 1 ? ` ×${n}` : ''}`;
            return (
              <span className="qv-ing-cell" key={tagId ?? rep} title={title} data-hover-item={rep}>
                {ri?.iconStatus === 'ready'
                  ? <img src={icon(rep)} alt="" loading="lazy"/>
                  : <span className="qv-ing-ph">{nm.slice(0, 1)}</span>}
                {n > 1 && <span className="qv-n">{n}</span>}
              </span>
            );
          })}
          {reps.length > 4 && <span className="qv-more">+{reps.length - 4}</span>}
          {reps.length === 0 && v.kind === 'rule' && <span className="qv-more">规则</span>}
        </span>
      </div>
    );
  };

  return (
    <div className="qv" ref={boxRef} style={{top, left}} {...hoverProps()}>
      <div className="qv-head">
        {item && item.iconStatus === 'ready' && <img className="ss-icon" alt="" src={icon(itemId)}/>}
        <span className="qv-title">{item?.displayName ?? itemId.split(':').pop()}</span>
        <span className="qv-id" title={itemId}>{itemId}</span>
        <span style={{flex: 1}}/>
        <button type="button" className="p-btn" title="看它怎么合成（R）"
          onClick={() => onOpenGraph(itemId, 'out')}>看合成 <kbd>R</kbd></button>
        <button type="button" className="p-btn" title="看它能干什么（U）"
          onClick={() => onOpenGraph(itemId, 'in')}>看用途 <kbd>U</kbd></button>
        <button type="button" className="ctx-clear" style={{color: 'var(--mc-muted)'}} aria-label="关闭" onClick={onClose}>✕</button>
      </div>
      <div className="qv-body">
        <div className="p-title">怎么合成 · 作为产物 <span className="count">{outs.length}</span></div>
        {outs.map(row)}
        {outs.length === 0 && <div className="p-empty">没有以它为产物的配方（采集类物品就是这样）。</div>}
        <div className="p-title" style={{marginTop: 6}}>能干什么 · 作为原料 <span className="count">{ins.length}</span></div>
        {ins.map(row)}
        {ins.length === 0 && <div className="p-empty">没有以它为原料的配方。</div>}
      </div>
    </div>
  );
}
