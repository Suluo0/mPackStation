import {useEffect, useState} from 'react';
import {getContent, listContent, type ContentDocument, type ContentRevision} from '../api/content';
import {useUrlPatch, useUrlState} from '../app/url';

/* 魔改态 v0：文档列表 + Design|Code Split 布局（V3 §6 用户决策 6）。
   探针边界：payload 只读展示，可写编辑器/校验/应用/历史由 3C 按原规格落地；
   分隔线可拖，用来定这个布局本身。 */
export function ModeEdit() {
  const {packId, doc} = useUrlState();
  const patch = useUrlPatch();
  const [docs, setDocs] = useState<ContentDocument[]>([]);
  const [rev, setRev] = useState<ContentRevision | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [dividerPct, setDividerPct] = useState(55);

  useEffect(() => {
    if (!packId) return;
    listContent(packId).then(setDocs).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  useEffect(() => {
    if (!packId || !doc) { setRev(null); return; }
    getContent(packId, doc).then(r => setRev(r.revision)).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId, doc]);

  if (!packId) return null;

  const payloadText = rev ? JSON.stringify(rev.payload, null, 2) : '';

  const startDrag = () => {
    const move = (e: MouseEvent) => {
      const el = document.querySelector('.split') as HTMLElement | null;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      setDividerPct(Math.min(85, Math.max(20, ((e.clientX - rect.left) / rect.width) * 100)));
    };
    const up = () => {
      document.removeEventListener('mousemove', move);
      document.removeEventListener('mouseup', up);
      document.body.style.cursor = '';
    };
    document.addEventListener('mousemove', move);
    document.addEventListener('mouseup', up);
    document.body.style.cursor = 'col-resize';
  };

  return (
    <div style={{flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column'}}>
      <div className="ed-toolbar">
        <span className="title">魔改</span>
        <span className="sub">{docs.length} 份文档 · 选中写 ?doc=</span>
        <span className="grow"/>
        {rev && <span className="ed-count">revision r{rev.revision} · {rev.state}</span>}
      </div>
      <div style={{flex: 1, minHeight: 0, display: 'flex'}}>
        <div className="split-docs">
          {error && <div className="p-empty">{error}</div>}
          {docs.map(d => (
            <div key={d.id} className={`p-row click${doc === d.id ? ' on' : ''}`} onClick={() => patch({doc: d.id})}
              title={d.title}>
              <span className="grow">{d.title || d.slug}</span>
              <span className="sub">{d.kind}</span>
            </div>
          ))}
          {docs.length === 0 && !error && (
            <div className="p-empty">这个包还没有魔改文档。索引态选一个配方后，「去魔改」会建第一份（3C 落地入口）。</div>
          )}
        </div>
        <div className="split">
          <div className="split-pane" style={{flexBasis: `calc(${dividerPct}% - 3px)`}}>
            <div className="sp-head">Code · payload JSON（只读探针，可写编辑器由 3C 落地）</div>
            <div className="sp-body">
              {rev
                ? <textarea className="split-json" readOnly value={payloadText} spellCheck={false}/>
                : <div className="ed-placeholder">{doc ? '这份文档还没有修订内容。' : '左侧选一份文档。'}</div>}
            </div>
          </div>
          <div className="split-divider" onMouseDown={startDrag} role="separator" aria-orientation="vertical">◇</div>
          <div className="split-pane">
            <div className="sp-head">Design · 图形预览</div>
            <div className="sp-body">
              <div className="ed-placeholder">
                JEI 网格实时预览由 <b>3B RecipeView</b> 落地：改 JSON 即重画，
                支持 shaped / shapeless / 机器配方 / ae2 transform / matter_cannon / 特殊配方中文说明。
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
