import {useEffect, useRef, type ReactNode} from 'react';
import type {CatalogItem} from '../api/catalog';
import type {IngredientSlot, RecipeView} from './recipeView';
import {iconUrl, shownCandidate, type RecipeCursor} from './recipeUtils';
import {clearHoverTarget, setHoverTarget} from '../app/hoverTarget';
import {Icon} from '../ui/Icon';

/* 配方卡 = 关系态（也是后续链路图节点）的唯一原子。
   同一张卡在画布视图（下钻）和清单视图（浏览）里共用，避免两套配方渲染语言。

   结构完全来自后端的 refs：有几个槽、几行几列、哪些是标签候选，都是读出来的；
   读不出边（status='unsupported'）就画成规则卡，不补假网格。

   本文件只导出组件 —— 纯工具（iconUrl / shownCandidate / RecipeCursor）
   在 recipeUtils.ts，见那里的注释说明为什么必须分开。 */

export type {RecipeCursor};

export function RecipeCard({view, packId, itemById, cursor, viaTag, onDrill, onCycle, page, main}: {
  view: RecipeView;
  packId: string;
  itemById: Map<string, CatalogItem>;
  cursor: RecipeCursor;
  /** 这条关系是经某个标签命中的（焦点物品是该标签成员），标出来 */
  viaTag?: string | null;
  /** 点槽位：进到该物品的关系 */
  onDrill: (itemId: string) => void;
  /** 点候选标记：轮换到下一个候选 */
  onCycle: (slot: IngredientSlot, step: number) => void;
  /** 主卡翻页（清单卡不传） */
  page?: {index: number; total: number; onPrev: () => void; onNext: () => void};
  main?: boolean;
}) {
  /* 标签槽的可读类目词（木板 / 木台阶 …）。view.tagNotes 是归一化层算好的，
     槽位悬停提示直接复用它，免得同一件事在两个地方各推一遍。 */
  const labelByTag = new Map(view.tagNotes.map(t => [t.tagId, t.label]));
  const cell = (slot: IngredientSlot | null, extra?: string) => (
    <SlotCell key={extra ?? slot?.key ?? 'empty'} slot={slot} packId={packId} itemById={itemById}
      cursor={cursor} labelByTag={labelByTag} onDrill={onDrill} onCycle={onCycle}/>
  );
  const nameOf = (id: string) => itemById.get(id)?.displayName ?? id.split(':').pop() ?? id;

  let body: ReactNode = null;
  if (view.kind === 'grid') {
    const rows: ReactNode[] = [];
    for (let r = 0; r < view.rows; r++) {
      const row: ReactNode[] = [];
      for (let c = 0; c < view.cols; c++) {
        const index = r * view.cols + c;
        row.push(cell(view.grid[index] ?? null, `s${index}`));
      }
      rows.push(<div className="rc-row" key={`r${r}`}>{row}</div>);
    }
    body = (
      <>
        <div className="rc-pad">{rows}</div>
        {arrow(view)}
        {output(view, cell, cursor, nameOf, main)}
      </>
    );
  } else if (view.kind === 'single' || view.kind === 'smithing') {
    body = (
      <>
        <div className="rc-pad row">
          {view.inputs.map(s => cell(s, s.key))}
          {view.inputs.length === 0 && <span className="rc-none">输入未登记</span>}
        </div>
        {arrow(view)}
        {output(view, cell, cursor, nameOf, main)}
      </>
    );
  } else if (view.kind === 'rule') {
    body = <div className="rc-rule">{view.notes.map((n, i) => <div key={i}>{n}</div>)}</div>;
  } else {
    body = (
      <div className="rc-rule">
        <div>这条配方没有被结构化：后端保留了原始定义，但没算出输入输出边。</div>
        <div className="rc-quiet">所以这里只能给类型，给不出槽位 —— 是「读不出」，不是「没有」。</div>
      </div>
    );
  }

  return (
    <div className={`rc${main ? ' main' : ''}${page ? ' paged' : ''}`}>
      <div className="rc-head">
        <span className="rc-type">{view.machine}</span>
        <span className="rc-id" title={view.id}>{view.id}</span>
        {view.flags.map(f => <span className="rc-flag" key={f}>{f}</span>)}
        {viaTag && <span className="rc-flag gold" title={`焦点物品是 ${viaTag} 的成员，所以命中这条`}>经标签 #{viaTag.split(':').pop()}</span>}
        {page && (
          <span className="rc-page">
            <button type="button" className="rc-pg" disabled={page.total < 2}
              title="上一张（←）" onClick={page.onPrev}><Icon name="caretLeft" size={14}/></button>
            <span className="rc-count">{page.index + 1}/{page.total}</span>
            <button type="button" className="rc-pg" disabled={page.total < 2}
              title="下一张（→）" onClick={page.onNext}><Icon name="caretRight" size={14}/></button>
          </span>
        )}
      </div>
      <div className="rc-body">{body}</div>
      {view.tagNotes.length > 0 && (
        <div className="rc-tags">
          <span className="rc-quiet">任意</span>
          {view.tagNotes.map(t => (
            <span className="tag-chip" key={t.tagId}
              title={`${t.tagId}\n${t.candidates} 种同类物品，任意一种都行`}>
              {t.label}{t.slots > 1 ? ` ×${t.slots}` : ''}
            </span>
          ))}
          <span className="rc-quiet">同类的任意一种都行</span>
        </div>
      )}
    </div>
  );
}

function arrow(view: RecipeView) {
  const {time, xp} = view.meta;
  const meta = time === undefined ? '' : `${(time / 20).toFixed(1)}s${xp ? ` · ${xp}xp` : ''}`;
  return (
    <div className="rc-arrow">
      <span className="rc-machine">{view.machine}</span>
      <span className="rc-glyph">→</span>
      {meta && <span className="rc-meta">{meta}</span>}
    </div>
  );
}

function output(
  view: RecipeView,
  cell: (s: IngredientSlot | null, k?: string) => ReactNode,
  cursor: RecipeCursor,
  nameOf: (id: string) => string,
  main?: boolean,
) {
  if (!view.output) {
    return (
      <div className="rc-out">
        <span className="rc-none">{view.outputNote ?? '产物由运行时决定'}</span>
      </div>
    );
  }
  const shown = shownCandidate(view.output, cursor.cycle);
  return (
    <div className="rc-out">
      {cell(view.output, 'out')}
      <span className={`rc-outname${main ? '' : ' small'}`}>
        {shown ? nameOf(shown) : '产物'}
        {view.output.count > 1 && <span className="rc-times">×{view.output.count}</span>}
      </span>
    </div>
  );
}

function SlotCell({slot, packId, itemById, cursor, labelByTag, onDrill, onCycle}: {
  slot: IngredientSlot | null;
  packId: string;
  itemById: Map<string, CatalogItem>;
  cursor: RecipeCursor;
  /** 标签 id → 可读类目词（木板），只用于悬停提示 */
  labelByTag: Map<string, string>;
  onDrill: (itemId: string) => void;
  onCycle: (slot: IngredientSlot, step: number) => void;
}) {
  const wrap = useRef<HTMLSpanElement>(null);
  const multi = !!slot && slot.candidates.length > 1;
  const shown = slot ? shownCandidate(slot, cursor.cycle) : null;

  /* 槽位被卸载（切配方、翻页）时把悬停登记撤掉，
     否则 R/U 会继续作用在一个已经不在屏幕上的物品上。 */
  useEffect(() => () => { if (shown) clearHoverTarget(shown); }, [shown]);

  /* 滚轮停在这个槽上 = 轮换候选（JEI 的手感）。
     拦截范围严格限制在「真的有多个并列候选」的槽（`multi`）：单候选槽、空格子、
     规则卡都没有可换的东西，拦下来只会吞掉用户的页面滚动 —— 那正是首屏网格上
     出过的问题，见 ItemGrid 里那段注释。⌘/Ctrl+滚轮 一律放行，留给缩放。
     必须原生挂 + passive:false —— React 的 onWheel 走 root 上的 passive 监听，
     在那里 preventDefault 不生效，页面会跟着一起滚。 */
  useEffect(() => {
    const el = wrap.current;
    if (!el || !multi || !slot) return;
    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) return;
      e.preventDefault();
      e.stopPropagation();
      onCycle(slot, e.deltaY > 0 ? 1 : -1);
    };
    el.addEventListener('wheel', onWheel, {passive: false});
    return () => el.removeEventListener('wheel', onWheel);
  }, [multi, slot, onCycle]);

  if (!slot) return <span className="slot empty" aria-hidden/>;

  const item = shown ? itemById.get(shown) : undefined;
  const name = item?.displayName ?? (shown ? shown.split(':').pop() : '');
  const at = multi ? ((cursor.cycle[slot.key] ?? 0) % slot.candidates.length) + 1 : 1;
  const selected = cursor.sel === slot.key;
  const label = slot.tagId ? labelByTag.get(slot.tagId) : null;

  const title = shown
    ? `${name}\n${shown}`
      + (label ? `\n来自「任意 ${label}」标签（${slot.candidates.length} 种候选 · [ ] 或滚轮轮换）` : '')
      + (multi && !label ? `\n该槽 ${slot.candidates.length} 个并列候选（[ ] 或滚轮轮换）` : '')
      + '\n点击进它的关系'
    : `标签 ${slot.tagId} 没有解析出成员`;

  return (
    <span className={`slot-cell${selected ? ' sel' : ''}`} ref={wrap}
      onMouseOver={() => setHoverTarget(shown ?? null)}
      onMouseLeave={() => setHoverTarget(null)}>
      <button type="button" className="slot" title={title} disabled={!shown}
        data-hover-item={shown ?? undefined}
        onClick={() => { if (shown) onDrill(shown); }}>
        {item && item.iconStatus === 'ready'
          ? <img src={iconUrl(packId, shown!)} alt="" loading="lazy"/>
          : <span className="slot-ph">{name ? name.slice(0, 1) : '?'}</span>}
        {slot.count > 1 && <span className="slot-count">{slot.count}</span>}
        {slot.tagId && !multi && <span className="slot-any">任</span>}
      </button>
      {multi && (
        <button type="button" className="slot-cyc" title={`轮换候选（[ ] 或滚轮） · 共 ${slot.candidates.length} 种`}
          onClick={e => { e.stopPropagation(); onCycle(slot, 1); }}>
          <span className="cyc-dot"/>
          <span className="cyc-n">{at}/{slot.candidates.length}</span>
        </button>
      )}
    </span>
  );
}
