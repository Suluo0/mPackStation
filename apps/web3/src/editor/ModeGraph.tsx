import {useCatalog} from '../app/CatalogContext';
import type {CatalogRecipe} from '../api/catalog';
import {useFocus, useUrlPatch} from '../app/url';

/* 关系态 v0：双向配方清单。材料点击 → 重新居中是全站联动感最强的交互，
   v0 先以行点击设焦点保住语义；JEI 网格渲染与无限下钻由 3B 落地。 */
export function ModeGraph() {
  const {itemById, recipesByOutput, recipesByInput, unstructuredByMention} = useCatalog();
  const [focus, setFocus] = useFocus();
  const patch = useUrlPatch();

  if (!focus) {
    return (
      <div className="ed-placeholder" style={{display: 'flex', flexDirection: 'column'}}>
        先选一个焦点：在索引里点一个物品，或按 <b>⌘K</b> 搜。
        <div style={{marginTop: 8}}>
          <button type="button" className="p-btn primary" onClick={() => patch({mode: 'index'})}>去索引</button>
        </div>
      </div>
    );
  }

  const outs = recipesByOutput.get(focus.id) ?? [];
  const ins = recipesByInput.get(focus.id) ?? [];
  /* 模组配方大量用标签代替物品（c:pellets/antimatter），所以要把焦点物品的标签
     也当作提及键查一遍，否则断点会漏报。 */
  const blind = (() => {
    const keys = [focus.id, ...(itemById.get(focus.id)?.tags ?? [])];
    const seen = new Set<string>();
    const out: CatalogRecipe[] = [];
    for (const k of keys) {
      for (const r of unstructuredByMention.get(k) ?? []) {
        if (seen.has(r.id)) continue;
        seen.add(r.id);
        out.push(r);
      }
    }
    return out;
  })();
  const blindTypes = [...new Set(blind.map(r => r.type))];

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">关系</span>
        <span className="mono" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 12, color: 'var(--mc-text-2)'}}>{focus.id}</span>
        <span className="grow"/>
        <span className="ed-count">作为产物 {outs.length} · 作为原料 {ins.length}{blind.length > 0 ? ` · 未结构化 ${blind.length}` : ''}</span>
      </div>
      <div className="ed-scroll">
        <div className="graph-cols">
          <div className="graph-col">
            <div className="g-head">作为产物 <span className="count" style={{color: 'var(--mc-muted)'}}>{outs.length}</span></div>
            <div className="g-body">
              {outs.map(r => (
                <div key={`o-${r.id}`} className="p-row click" onClick={() => patch({mode: 'edit'})} title="去魔改这条配方">
                  <span className="grow">{r.type}</span>
                  <span className="mono">{r.id}</span>
                </div>
              ))}
              {outs.length === 0 && <div className="p-empty">{blind.length > 0 ? `没有可读出的产物边：${blind.length} 条配方提到它，但类型（${blindTypes.join('、')}）没被结构化。` : '目录里没有以它为产物的配方。'}</div>}
            </div>
          </div>
          <div className="graph-col">
            <div className="g-head">作为原料 <span className="count" style={{color: 'var(--mc-muted)'}}>{ins.length}</span></div>
            <div className="g-body">
              {ins.map(r => {
                const other = r.refs.find(x => x.id !== focus.id && x.role === 'output');
                return (
                  <div key={`i-${r.id}`} className="p-row click"
                    title={other ? `居中到 ${other.id}` : ''}
                    onClick={() => { if (other) setFocus({kind: 'item', id: other.id}); }}>
                    <span className="grow">{r.type}</span>
                    <span className="mono">{other?.id ?? r.id}</span>
                  </div>
                );
              })}
              {ins.length === 0 && <div className="p-empty">{blind.length > 0 ? `没有可读出的原料边：${blind.length} 条配方提到它，但类型（${blindTypes.join('、')}）没被结构化。` : '目录里没有以它为原料的配方。'}</div>}
            </div>
          </div>
          {blind.length > 0 && (
            <div className="graph-col">
              <div className="g-head">链路断点 · 未结构化配方 <span className="count" style={{color: 'var(--mc-muted)'}}>{blind.length}</span></div>
              <div className="g-body">
                {blind.slice(0, 40).map(r => (
                  <div key={`b-${r.id}`} className="p-row" title={r.diagnostics.join('; ')}>
                    <span className="grow">{r.type}</span>
                    <span className="mono">{r.id}</span>
                  </div>
                ))}
                {blind.length > 40 && <div className="p-empty">…另有 {blind.length - 40} 条</div>}
                <div className="p-empty">
                  这些配方提到了它，但解析器不认识配方类型，只留了原始定义、没算出输入输出边 ——
                  递归链到这里断开，是「读不出」不是「没有」。
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </>
  );
}
