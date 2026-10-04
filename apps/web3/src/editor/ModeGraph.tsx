import {useCallback, useEffect, useMemo, useState} from 'react';
import type {CatalogRecipe} from '../api/catalog';
import {useCatalog} from '../app/CatalogContext';
import {Icon} from '../ui/Icon';
import {useFocus, useUrlPatch, useUrlState} from '../app/url';
import {RecipeCard} from './RecipeCard';
import {iconUrl, type RecipeCursor} from './recipeUtils';
import {collectRecipes, makeRecipeViewCache, type IngredientSlot, type RecipeHit, type RecipeView} from './recipeView';

/* 关系态 v1（方向 C：一个配方卡原子 + 两种排布）
   - 画布：单方向下钻。焦点卡可翻页（← →），右侧是同一方向的其它配方，点槽位进它的关系。
   - 清单：产物/原料双向并排，全部用配方卡密铺 —— 保住 v0 的双向对照能力。

   对齐 JEI 的能力：R/U（?rd=）、按真实 refs 画结构、标签槽候选轮换（[ ]）、
   经标签命中的关系（标出 #tag）、真实图标走 /catalog/icon、下钻轨迹进 ?rpath=。
   刻意不对齐的：分页/滚动双导航、界面缩放、面板宽高滑杆 —— 那是「游戏里没有滚轮和窗口」
   的妥协产物，搬进网页只会变成反直觉的设置项。

   读不出边的配方（后端 status='unsupported'）一律画规则卡，不补假网格；
   它们没有物品边，所以任何物品的双向清单里都翻不到，只能单独成段。 */

const EMPTY: CatalogRecipe[] = [];
const SIDE_LIMIT = 12;
const CARD_LIMIT = 60;
const TRAIL_LIMIT = 12;

/** 这张卡上可以轮换候选的槽位（标签槽或多个并列候选的槽）。 */
function cycleable(view: RecipeView): IngredientSlot[] {
  const all = view.kind === 'grid' ? view.grid.filter((s): s is IngredientSlot => s !== null) : view.inputs;
  return all.filter(s => s.candidates.length > 1);
}

export function ModeGraph() {
  const {packId, rd, rv, r, rpath} = useUrlState();
  const {catalog, itemById, tagById, recipesByOutput, recipesByInput, unstructuredByMention} = useCatalog();
  const [focus, setFocus] = useFocus();
  const patch = useUrlPatch();
  /* 瞬态状态：选中的槽位 + 每个槽位的候选游标。不进 URL（切换/分享时无意义）。 */
  const [cursor, setCursor] = useState<RecipeCursor>({sel: null, cycle: {}});

  const viewOf = useMemo(() => makeRecipeViewCache({itemById, tagById}), [itemById, tagById]);

  const focusId = focus && focus.kind === 'item' ? focus.id : null;
  const focusItem = focusId ? itemById.get(focusId) : undefined;
  const tagKeys = useMemo(() => focusItem?.tags ?? [], [focusItem]);

  /* 「物品 id → 配方」的直接索引对标签槽是空的（后端的标签原料只有一条 ref，
     id 是标签本身、不展开成员），所以焦点的标签也要当提及键查一遍 ——
     否则金合欢原木的「用途」里看不到金合欢木板。逻辑在 recipeView.collectRecipes，
     速览浮层用的是同一份。 */
  const outs = useMemo(
    () => (focusId ? collectRecipes(recipesByOutput.get(focusId), recipesByOutput, tagKeys) : []),
    [focusId, recipesByOutput, tagKeys]);
  const ins = useMemo(
    () => (focusId ? collectRecipes(recipesByInput.get(focusId), recipesByInput, tagKeys) : []),
    [focusId, recipesByInput, tagKeys]);

  const list = rd === 'in' ? ins : outs;
  const pos = Math.min(r, Math.max(0, list.length - 1));
  const current = list[pos];
  const currentView = current ? viewOf(current.rec) : null;

  const trail = useMemo(
    () => (rpath ? rpath.split(',').filter(id => itemById.has(id)) : []),
    [rpath, itemById]);

  /* 提到焦点、但后端没结构化（零边）的配方：链在这里断开。 */
  const blind = useMemo(() => {
    if (!focusId) return EMPTY;
    const keys = [focusId, ...(itemById.get(focusId)?.tags ?? [])];
    const seen = new Set<string>();
    const out: CatalogRecipe[] = [];
    for (const k of keys) {
      for (const rec of unstructuredByMention.get(k) ?? []) {
        if (seen.has(rec.id)) continue;
        seen.add(rec.id);
        out.push(rec);
      }
    }
    return out;
  }, [focusId, itemById, unstructuredByMention]);

  const unparsed = useMemo(
    () => (catalog?.recipes ?? EMPTY).filter(rec => rec.status !== 'parsed'),
    [catalog]);

  const drill = useCallback((itemId: string) => {
    if (!focusId || itemId === focusId) return;
    const next = [...trail, focusId].slice(-TRAIL_LIMIT);
    setFocus({kind: 'item', id: itemId}, {patch: {rpath: next.join(',') || null, r: null}});
  }, [focusId, trail, setFocus]);

  const goUp = useCallback(() => {
    const prev = trail[trail.length - 1];
    if (!prev) return;
    setFocus({kind: 'item', id: prev}, {patch: {rpath: trail.slice(0, -1).join(',') || null, r: null}});
  }, [trail, setFocus]);

  const cycleSlot = useCallback((slot: IngredientSlot, step: number) => {
    setCursor(cur => {
      const n = Math.max(1, slot.candidates.length);
      const at = (((cur.cycle[slot.key] ?? 0) + step) % n + n) % n;
      return {sel: slot.key, cycle: {...cur.cycle, [slot.key]: at}};
    });
  }, []);

  /* 本态私有键盘：← → 换配方，[ ] 轮换候选，Backspace 返回上层。
     输入框聚焦时一律不响应（与全局 useHotkeys 同一约定）。 */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable || el.tagName === 'SELECT')) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === 'Backspace') { e.preventDefault(); goUp(); return; }
      if (!currentView) return;
      if (rv === 'canvas' && list.length > 1) {
        if (e.key === 'ArrowRight') { e.preventDefault(); patch({r: String((pos + 1) % list.length)}); return; }
        if (e.key === 'ArrowLeft') { e.preventDefault(); patch({r: String((pos - 1 + list.length) % list.length)}); return; }
      }
      if (e.key === '[' || e.key === ']') {
        const slots = cycleable(currentView);
        if (slots.length === 0) return;
        e.preventDefault();
        const active = slots.find(s => s.key === cursor.sel) ?? slots[0];
        cycleSlot(active, e.key === ']' ? 1 : -1);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [currentView, cursor.sel, cycleSlot, goUp, list.length, patch, pos, rv]);

  if (!packId) return null;

  if (!focusId) {
    return (
      <div className="ed-placeholder" style={{display: 'flex', flexDirection: 'column'}}>
        先选一个焦点：在索引里点一个物品，或按 <b>⌘K</b> 搜。
        <div style={{marginTop: 8}}>
          <button type="button" className="p-btn primary" onClick={() => patch({mode: 'index'})}>去索引</button>
        </div>
      </div>
    );
  }

  const name = focusItem?.displayName ?? focusId.split(':').pop() ?? focusId;
  const outCount = outs.length;
  const inCount = ins.length;
  const otherCount = rd === 'in' ? outCount : inCount;
  const crumbs = [...trail, focusId];
  const card = (entry: RecipeHit, opts?: {onDrill?: (id: string) => void}) => (
    <RecipeCard key={entry.rec.id} view={viewOf(entry.rec)} packId={packId} itemById={itemById}
      cursor={cursor} viaTag={entry.viaTag} onDrill={opts?.onDrill ?? drill} onCycle={cycleSlot}/>
  );

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">关系</span>
        {focusItem?.iconStatus === 'ready' && <img className="rg-focus-icon" alt="" src={iconUrl(packId, focusId)}/>}
        <span className="rg-focus-name" title={name}>{name}</span>
        <span className="rg-focus-id" title={focusId}>{focusId}</span>
        <span className="grow"/>
        {rv === 'canvas' && (
          <div className="view-switch" role="group" aria-label="关系方向">
            <button type="button" className={`p-btn${rd === 'out' ? ' on-view' : ''}`}
              title="以它为产物的配方（快捷键 R）" onClick={() => patch({rd: null, r: null})}>
              作为产物 {outCount} <kbd>R</kbd>
            </button>
            <button type="button" className={`p-btn${rd === 'in' ? ' on-view' : ''}`}
              title="以它为原料的配方（快捷键 U）" onClick={() => patch({rd: 'in', r: null})}>
              作为原料 {inCount} <kbd>U</kbd>
            </button>
          </div>
        )}
        <div className="view-switch" role="group" aria-label="关系视图">
          <button type="button" className={`p-btn${rv === 'canvas' ? ' on-view' : ''}`}
            title="焦点下钻：一次看一条配方，点槽位继续下钻" onClick={() => patch({rv: null})}>画布</button>
          <button type="button" className={`p-btn${rv === 'list' ? ' on-view' : ''}`}
            title="双向清单：产物与原料并排，卡片密铺" onClick={() => patch({rv: 'list'})}>清单</button>
        </div>
      </div>

      <div className="ed-scroll">
        <div className="rg-crumbs">
          {crumbs.map((id, i) => {
            const last = i === crumbs.length - 1;
            const item = itemById.get(id);
            return (
              <span className="rg-crumb-wrap" key={`${id}-${i}`}>
                <button type="button" className={`rg-crumb${last ? ' cur' : ''}`}
                  title={`${item?.displayName ?? id}\n${id}`}
                  onClick={() => {
                    if (last) return;
                    setFocus({kind: 'item', id}, {patch: {rpath: crumbs.slice(0, i).join(',') || null, r: null}});
                  }}>
                  {item?.iconStatus === 'ready' && <img alt="" src={iconUrl(packId, id)}/>}
                  <span>{item?.displayName ?? id.split(':').pop()}</span>
                </button>
                {!last && <span className="rg-crumb-sep"><Icon name="caretRight" size={12}/></span>}
              </span>
            );
          })}
          {trail.length > 0 && (
            <button type="button" className="p-btn rg-up" title="返回上层（Backspace）" onClick={goUp}>↰ 返回上层</button>
          )}
        </div>

        {rv === 'canvas' ? (
          currentView ? (
            <div className="rg-cols">
              <div className="rg-main">
                <RecipeCard view={currentView} packId={packId} itemById={itemById} cursor={cursor}
                  viaTag={current?.viaTag ?? null} onDrill={drill} onCycle={cycleSlot} main
                  page={{
                    index: pos,
                    total: list.length,
                    onPrev: () => patch({r: String((pos - 1 + list.length) % list.length)}),
                    onNext: () => patch({r: String((pos + 1) % list.length)}),
                  }}/>
                <div className="rg-pos">
                  {rd === 'in' ? '作为原料' : '作为产物'} {list.length} 条 · 位置 {pos + 1}/{list.length}
                  {list.length > 1 && ' · ← → 换配方'}
                  {cycleable(currentView).length > 0 && ' · [ ] 或滚轮轮换候选'}
                  {' · 点材料槽下钻'}
                </div>
              </div>
              <aside className="rg-side">
                <div className="p-title">同一焦点的其它配方 <span className="count">{Math.max(0, list.length - 1)}</span></div>
                {list.length > 1 ? (
                  <div className="rg-side-list">
                    {list.slice(0, SIDE_LIMIT).map((entry, i) => i === pos ? null : (
                      <div className="rg-side-item" key={entry.rec.id}>
                        <button type="button" className="rg-side-head" title="切到这一条"
                          onClick={() => patch({r: String(i)})}>
                          <span className="rg-side-type">{viewOf(entry.rec).machine}</span>
                          <span className="rg-side-n">{i + 1}</span>
                        </button>
                        {card(entry, {onDrill: () => patch({r: String(i)})})}
                      </div>
                    ))}
                    {list.length - 1 > SIDE_LIMIT && (
                      <div className="p-empty">…另有 {list.length - 1 - SIDE_LIMIT} 条，切「清单」看全部。</div>
                    )}
                  </div>
                ) : (
                  <div className="p-empty">这是该方向唯一的一条。</div>
                )}
              </aside>
            </div>
          ) : (
            <div className="rg-miss">
              <div className="p-empty">
                目录里没有以它为{rd === 'in' ? '原料' : '产物'}的配方。
                {otherCount > 0 ? ' 它在另一个方向有配方。' : ' 这是采集类物品（原版没有配方）。'}
                {otherCount > 0 && (
                  <button type="button" className="p-btn"
                    onClick={() => patch({rd: rd === 'in' ? null : 'in', r: null})}>
                    切到{rd === 'in' ? '作为产物' : '作为原料'}（{otherCount} 条）
                  </button>
                )}
              </div>
            </div>
          )
        ) : (
          <div className="rg-cols list">
            <section className="rg-col">
              <div className="p-title">作为产物 <span className="count">{outCount}</span></div>
              <div className="rg-cards">
                {outs.slice(0, CARD_LIMIT).map(entry => card(entry))}
                {outs.length === 0 && <div className="p-empty">目录里没有以它为产物的配方。</div>}
                {outs.length > CARD_LIMIT && <div className="p-empty">…另有 {outs.length - CARD_LIMIT} 条未渲染。</div>}
              </div>
            </section>
            <section className="rg-col">
              <div className="p-title">作为原料 <span className="count">{inCount}</span></div>
              <div className="rg-cards">
                {ins.slice(0, CARD_LIMIT).map(entry => card(entry))}
                {ins.length === 0 && <div className="p-empty">目录里没有以它为原料的配方。</div>}
                {ins.length > CARD_LIMIT && <div className="p-empty">…另有 {ins.length - CARD_LIMIT} 条未渲染。</div>}
              </div>
            </section>
          </div>
        )}

        {blind.length > 0 && (
          <section className="rg-block">
            <div className="p-title">链路断点 · 提到它但没结构化的配方 <span className="count">{blind.length}</span></div>
            <div className="rg-cards">{blind.slice(0, 20).map(rec => card({rec, viaTag: null}))}</div>
            <div className="p-empty">
              这些配方提到了它，但后端没结构化这种类型，只留了原始定义、没算出输入输出边 ——
              递归链到这里断开，是「读不出」不是「没有」。
            </div>
          </section>
        )}

        {unparsed.length > 0 && (
          <details className="rg-block">
            <summary className="p-title"><span className="rg-caret"><Icon name="caretRight" size={12}/></span> 未结构化配方 · 目录全部 <span className="count">{unparsed.length}</span></summary>
            <div className="p-empty">
              这些配方没有物品边，所以不会出现在任何物品的双向清单里。原版「特殊配方」就是这一类：
              它们是运行时判定规则（染色、复制地图、修耐久…），本来就没有槽位图，画成规则卡才是它们的真实形态。
            </div>
            <div className="rg-cards">{unparsed.map(rec => card({rec, viaTag: null}))}</div>
          </details>
        )}
      </div>
    </>
  );
}
