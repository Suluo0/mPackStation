import {useNavigate, useParams, useSearchParams} from 'react-router-dom';
import {useFocus} from '../app/useFocus';
import {useCatalog} from '../app/CatalogContext';
import {packHref, type ContentMode} from '../app/nav';

/* 素面叶子：焦点详情 + 三个传送门。第三步换成 FocusInspector（图标/名称/标签/配方摘要）。
   传送门必须带焦点参数（设计文档 §4.5 的 ⚠：旧实现三个按钮全不带参数，跳过去等于从头找）。
   这里不重写焦点参数 —— 当前 URL 里那份就是 useFocus 写入的唯一事实源，整份带过去即可。 */
export function StubFocusInspector() {
  const [focus] = useFocus();
  const {id} = useParams();
  const [params] = useSearchParams();
  const nav = useNavigate();
  const {itemById} = useCatalog();
  if (!focus || !id) return null;

  const item = focus.kind === 'item' ? itemById.get(focus.id) : undefined;
  const go = (mode: ContentMode) => {
    const q = new URLSearchParams(params);
    q.set('mode', mode);
    nav(`${packHref(id, 'content')}?${q.toString()}`);
  };

  return (
    <div>
      <div className="rr-line">{focus.kind} · {focus.id}</div>
      {item && <div className="rr-line">{item.displayName}</div>}
      <div style={{display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8}}>
        <button type="button" className="tb-btn" onClick={() => go('index')}>在索引态打开</button>
        <button type="button" className="tb-btn" onClick={() => go('edit')}>去魔改</button>
        <button type="button" className="tb-btn" onClick={() => go('graph')}>看关系</button>
      </div>
    </div>
  );
}
