import {useUrlPatch, useUrlState} from './url';
import {usePackSummary} from './PackSummaryContext';
import {PackMenu} from './PackMenu';
import {ProblemsPopover} from './ProblemsPopover';
import {Icon} from './Icon';
import {useState} from 'react';

/* 顶栏（第三轮反馈：四个编辑器 tab 降为二级能力，挪进编辑区头——选中对象后才出现）。
   顶栏只剩一级能力：包名菜单 + 问题徽标 + 任务胶囊 + ⌘K。 */
export function TopBar({onOpenPalette}: {onOpenPalette: () => void}) {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const {pack, tasks, pendingConflicts} = usePackSummary();
  const [problemsOpen, setProblemsOpen] = useState(false);

  const errs = pendingConflicts.filter(c => c.severity === 'error').length;
  const warns = pendingConflicts.filter(c => c.severity !== 'error').length;
  const liveTask = tasks.find(t => t.status === 'running' || t.status === 'queued' || t.status === 'paused');

  return (
    <header className="topbar">
      <PackMenu/>

      <div className="tb-right">
        {packId && (errs > 0 ? (
          <button type="button" className="problems-chip err" title={`${errs} 个 error 级问题`}
            onClick={() => setProblemsOpen(v => !v)}>
            <Icon name="problems" size={12}/> {errs}
          </button>
        ) : warns > 0 ? (
          <button type="button" className="problems-chip warn" title={`${warns} 个 warning 级问题`}
            onClick={() => setProblemsOpen(v => !v)}>
            <Icon name="problems" size={12}/> {warns}
          </button>
        ) : (
          <button type="button" className="problems-chip clean" title="没有待解决问题"
            onClick={() => setProblemsOpen(v => !v)}>
            ✓
          </button>
        ))}
        {problemsOpen && <ProblemsPopover onClose={() => setProblemsOpen(false)}/>}
        {liveTask && (
          <button type="button" className="task-chip" title={`${liveTask.title || liveTask.type} · ${liveTask.progress}%`}
            onClick={() => patch({dock: '1'})}>
            <span>⏳</span>
            <span className="t-name">{liveTask.title || liveTask.type}</span>
            <span>{liveTask.progress}%</span>
          </button>
        )}
        {pack && <span className="ps-sub">v{pack.packVersion}</span>}
        <button type="button" className="tb-btn" onClick={onOpenPalette}>
          <Icon name="search" size={12}/> <span style={{marginLeft: 4}}>⌘K</span>
        </button>
      </div>
    </header>
  );
}
