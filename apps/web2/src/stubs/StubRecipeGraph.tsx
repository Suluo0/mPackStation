import {useCatalog} from '../app/CatalogContext';
import {useFocus} from '../app/useFocus';

/* 素面叶子：关系态。第三步换成 RecipeGraphPanel（JEI 式配方图 + 缩放平移）。
   焦点从 URL 读，所以切态切页都不丢（§7.3 第 5、6 条）。 */
export function StubRecipeGraph() {
  const [focus] = useFocus();
  const {recipesByOutput, recipesByInput} = useCatalog();
  if (!focus) return <div className="cp-panel"><div className="rr-line">先在索引态或 ⌘K 选一个焦点。</div></div>;

  const outs = recipesByOutput.get(focus.id) ?? [];
  const ins = recipesByInput.get(focus.id) ?? [];

  return (
    <div className="cp-panel">
      <div className="rr-line">{focus.id}：作为产物 {outs.length} 个配方 · 作为原料 {ins.length} 个配方</div>
      {outs.map(r => <div key={`o-${r.id}`} className="rr-line">产物 · {r.type} · {r.id}</div>)}
      {ins.map(r => <div key={`i-${r.id}`} className="rr-line">原料 · {r.type} · {r.id}</div>)}
      {outs.length + ins.length === 0 && <div className="rr-line">目录里没有与它相关的配方。</div>}
    </div>
  );
}
