import {useEffect, useRef, useState} from 'react';
import {Link, useParams, useSearchParams} from 'react-router-dom';
import {usePackSummary} from '../app/PackSummaryContext';
import {packHref} from '../app/nav';
import {StubChecks} from '../stubs/StubChecks';
import {StubBuild} from '../stubs/StubBuild';
import {StubInstall} from '../stubs/StubInstall';
import {StubLaunch} from '../stubs/StubLaunch';
import {StubPublish} from '../stubs/StubPublish';

const SECTIONS = [
  {key: 'checks', title: '① 检查'},
  {key: 'build', title: '② 构建'},
  {key: 'install', title: '③ 安装'},
  {key: 'launch', title: '④ 起窗'},
  {key: 'publish', title: '⑤ 发布'},
] as const;

/* /packs/:id/delivery：五段纵向，一屏走完（设计文档 §4.4）。 */
export function DeliveryPage() {
  const {id} = useParams();
  const [params] = useSearchParams();
  const {pendingConflicts} = usePackSummary();
  const [lit, setLit] = useState(false);
  const refs = useRef<Record<string, HTMLDivElement | null>>({});

  /* ?step=install → 挂载后滚动到第③段并高亮 2s。 */
  useEffect(() => {
    if (params.get('step') !== 'install') return;
    refs.current.install?.scrollIntoView({behavior: 'smooth', block: 'start'});
    setLit(true);
    const t = window.setTimeout(() => setLit(false), 2000);
    return () => window.clearTimeout(t);
  }, [params]);

  if (!id) return null;

  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      {SECTIONS.map(s => (
        <div key={s.key} ref={el => { refs.current[s.key] = el; }} className="cp-panel"
          style={{outline: s.key === 'install' && lit ? '2px solid var(--mc-primary)' : 'none'}}>
          <div className="rr-title">{s.title}</div>
          {s.key === 'checks' && <>
            <StubChecks packId={id}/>
            {pendingConflicts.length > 0 && (
              <div className="rr-line">
                有待解决冲突 {pendingConflicts.length} 条，后端闸门会拒绝构建。
                <Link className="tb-btn" to={`${packHref(id, '')}?panel=conflicts`}>去概览处理</Link>
              </div>
            )}
          </>}
          {s.key === 'build' && <StubBuild packId={id}/>}
          {s.key === 'install' && <StubInstall packId={id}/>}
          {s.key === 'launch' && <StubLaunch packId={id}/>}
          {s.key === 'publish' && <StubPublish packId={id}/>}
        </div>
      ))}
    </div>
  );
}
