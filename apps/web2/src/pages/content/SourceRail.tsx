import {useEffect, useState} from 'react';
import {useSearchParams} from 'react-router-dom';
import {StubModRail} from '../../stubs/StubModRail';
import {StubModSearch} from '../../stubs/StubModSearch';
import {StubQuestChapters} from '../../stubs/StubQuestChapters';
import type {ContentMode} from '../../app/nav';

const FOLD_KEY = 'web2.srcrail.folded';

/* 内容页页内左列（设计文档 §3 末段 + §4.3）：默认折叠 56px，展开 260px，折叠态记 localStorage。
   编排态整体换成章节 rail，不是两条并存（§7.3 第 15 条）。 */
export function SourceRail({mode}: {mode: ContentMode}) {
  const [params, setParams] = useSearchParams();
  const railMods = params.get('rail') === 'mods';
  /* 键名是 folded，存的值就必须是折叠态：'1' 折叠、'0' 展开、缺省折叠（§6.4 默认 56px）。
     早先这里把值写反了（'1' 表示展开），验收第 14 条实测时暴露。 */
  const [open, setOpen] = useState(() => localStorage.getItem(FOLD_KEY) === '0');

  useEffect(() => { if (railMods) setOpen(true); }, [railMods]);

  const toggle = () => {
    const next = !open;
    setOpen(next);
    localStorage.setItem(FOLD_KEY, next ? '0' : '1');
  };

  const closeRail = () => setParams(p => { const n = new URLSearchParams(p); n.delete('rail'); return n; }, {replace: true});

  return (
    <aside className={`src-rail${open ? ' open' : ''}`}>
      <button type="button" className="src-toggle" onClick={toggle} aria-label={open ? '折叠来源栏' : '展开来源栏'}>
        {open ? '«' : '☰'}
      </button>
      {open && (
        <div className="src-body">
          {mode === 'quest' ? <StubQuestChapters/> : <StubModRail/>}
          {mode !== 'quest' && <button type="button" className="tb-btn" onClick={() => setParams(p => { const n = new URLSearchParams(p); n.set('rail', 'mods'); return n; }, {replace: true})}>添加模组</button>}
        </div>
      )}
      <StubModSearch open={railMods} onClose={closeRail}/>
    </aside>
  );
}
