import {useEffect, useRef} from 'react';
import {Link, useNavigate, useParams, useSearchParams} from 'react-router-dom';
import {usePackSummary} from '../app/PackSummaryContext';
import {useCatalog} from '../app/CatalogContext';
import {packHref, type ContentMode} from '../app/nav';
import {StubHealth} from '../stubs/StubHealth';
import {StubConflicts} from '../stubs/StubConflicts';
import {StubTasks} from '../stubs/StubTasks';

/* /packs/:id：概览是唯一「做判断」的页面（设计文档 §4.2），冲突就地处置。 */
export function OverviewPage() {
  const {id} = useParams();
  const [params] = useSearchParams();
  const nav = useNavigate();
  const {pack, pendingConflicts, tasks, loading, error} = usePackSummary();
  const {catalog, namespaces} = useCatalog();
  const conflictRef = useRef<HTMLDivElement>(null);

  /* ?panel=conflicts：展开并滚动到位（交付页闸门、顶栏告警都指向这里）。 */
  useEffect(() => {
    if (params.get('panel') === 'conflicts') conflictRef.current?.scrollIntoView({behavior: 'smooth', block: 'start'});
  }, [params]);

  if (!id) return null;
  /* tile 点击 = 切到内容页对应态，并带上当前 URL 的全部参数（焦点跟着走，§3.1）。 */
  const goMode = (mode: ContentMode) => {
    const q = new URLSearchParams(params);
    q.set('mode', mode);
    q.delete('panel');
    nav(`${packHref(id, 'content')}?${q.toString()}`);
  };

  const tiles: {label: string; value: number; mode: ContentMode}[] = [
    {label: '物品', value: catalog?.items.length ?? 0, mode: 'index'},
    {label: '配方', value: catalog?.recipes.length ?? 0, mode: 'graph'},
    {label: '魔改', value: pack ? pack.edits.recipes + pack.edits.structures + pack.edits.ores : 0, mode: 'edit'},
    {label: '任务', value: pack?.edits.quests ?? 0, mode: 'quest'},
  ];

  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      {error && <div className="rr-line">{error}</div>}
      <div className="cp-panel">
        {pack ? `${pack.name} · v${pack.packVersion}` : loading ? '载入中…' : '没有这个包'}
        <Link className="tb-btn" to={packHref(id, 'delivery')}>去交付出包</Link>
      </div>

      <div style={{display: 'flex', gap: 12, flexWrap: 'wrap'}}>
        <div className="cp-panel" style={{flex: 1, minWidth: 260}}>
          <div className="rr-title">健康</div>
          <StubHealth/>
        </div>
        <div className="cp-panel" style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
          {tiles.map(t => (
            <button key={t.label} type="button" className="tb-btn" onClick={() => goMode(t.mode)}>{t.label} {t.value}</button>
          ))}
        </div>
      </div>

      <div className="cp-panel" ref={conflictRef}>
        <div className="rr-title">冲突（待解决 {pendingConflicts.length}）</div>
        <StubConflicts/>
      </div>

      <div className="cp-panel">
        <div className="rr-title">来源模组前 5</div>
        {namespaces.slice(0, 5).map(n => <div key={n.ns} className="rr-line">{n.ns} · {n.count} 物品</div>)}
        <Link className="tb-btn" to={`${packHref(id, 'content')}?rail=mods`}>查看全部模组</Link>
      </div>

      <div className="cp-panel">
        <div className="rr-title">本包任务</div>
        <StubTasks tasks={tasks.filter(t => t.packId === id)}/>
      </div>
    </div>
  );
}
