import {useEffect, useState} from 'react';
import {getContent, listContent, saveContentDraft, validateContent, type ContentDocument, type ContentRevision} from '../api/content';
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
  /* 可写编辑器（2026-10-05 用户反馈：光建草稿不能改等于没魔改）：
     Code 面板从只读探针升级为 textarea + 保存草稿 + 校验。 */
  const [text, setText] = useState('');
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [editError, setEditError] = useState<string | null>(null);

  useEffect(() => {
    if (!packId) return;
    listContent(packId).then(setDocs).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  useEffect(() => {
    if (!packId || !doc) { setRev(null); return; }
    getContent(packId, doc).then(r => {
      setRev(r.revision);
      setText(r.revision ? JSON.stringify(r.revision.payload, null, 2) : '');
      setDirty(false);
      setNotice(null);
      setEditError(null);
    }).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId, doc]);

  if (!packId) return null;

  const saveDraft = async () => {
    if (!packId || !doc || !rev || busy) return;
    setBusy(true); setEditError(null); setNotice(null);
    try {
      const payload = JSON.parse(text); // 语法错在这里就地暴露
      const next = await saveContentDraft(packId, doc, rev.revision, payload);
      setRev(next); setText(JSON.stringify(next.payload, null, 2)); setDirty(false);
      setNotice(`已保存草稿 r${next.revision}`);
    } catch (e) {
      setEditError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const runValidate = async () => {
    if (!packId || !doc || busy) return;
    setBusy(true); setEditError(null); setNotice(null);
    try {
      if (dirty) { setEditError('先保存草稿再校验（校验走的是服务端已保存的版本）'); return; }
      const v = await validateContent(packId, doc);
      const bad = v.issues.filter(i => i.severity === 'error');
      setNotice(`校验：${v.status}${v.issues.length ? ` · ${v.issues.length} 条问题` : ' · 无问题'}`
        + (bad.length ? ` — ${bad.slice(0, 2).map(i => i.message).join('；')}` : ''));
    } catch (e) {
      setEditError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

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
        {notice && <span className="ed-count" style={{color: 'var(--mc-success)'}}>{notice}</span>}
        {editError && <span className="ed-count" style={{color: 'var(--mc-fail)'}}>{editError}</span>}
        {rev && <span className="ed-count">revision r{rev.revision} · {rev.state}{dirty ? ' · 未保存' : ''}</span>}
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
            <div className="sp-head">Code · payload JSON
              {rev && (
                <span className="sp-acts">
                  <button type="button" className="p-btn" disabled={busy} onClick={() => void runValidate()}>校验</button>
                  <button type="button" className="p-btn primary" disabled={busy || !dirty} onClick={() => void saveDraft()}>
                    {busy ? '保存中…' : dirty ? '保存草稿' : '已保存'}
                  </button>
                </span>
              )}
            </div>
            <div className="sp-body">
              {rev
                ? <textarea className="split-json" value={text} spellCheck={false}
                    onChange={e => { setText(e.target.value); setDirty(true); }}/>
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
