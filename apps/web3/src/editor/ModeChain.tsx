import {useMemo} from 'react';
import {useCatalog} from '../app/CatalogContext';
import {useFocus, useUrlPatch} from '../app/url';
import {expandChain} from '../chain/expand';
import {ChainView} from '../chain/ChainView';

/* 链路态：读焦点 → expandChain 逆向递归 → ChainView 渲染。
   焦点对象就是终极物品；数量暂定 1，后续可加数量编辑。 */
export function ModeChain() {
  const [focus] = useFocus();
  const patch = useUrlPatch();
  const {catalog} = useCatalog();

  const chain = useMemo(() => {
    if (!focus || focus.kind !== 'item' || !catalog) return null;
    return expandChain({recipes: catalog.recipes, items: catalog.items, tags: catalog.tags}, focus.id, 1);
  }, [focus, catalog]);

  if (!focus || focus.kind !== 'item') {
    return (
      <div className="ed-placeholder">
        先选一个终极物品：在索引里点一个，或按 <b>⌘K</b> 搜。
        <div style={{marginTop: 8}}>
          <button type="button" className="p-btn primary" onClick={() => patch({mode: 'index'})}>去索引</button>
        </div>
      </div>
    );
  }

  if (!chain) {
    return <div className="ed-placeholder">计算中…</div>;
  }

  return <ChainView chain={chain}/>;
}
