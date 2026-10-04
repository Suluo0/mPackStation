import {useUrlPatch, useUrlState} from './url';
import {usePackSummary} from './PackSummaryContext';
import {PackMenu} from './PackMenu';
import {ProblemsPopover} from './ProblemsPopover';
import {Icon} from '../ui/Icon';
import {useState} from 'react';

/* 顶栏（第三轮反馈：四个编辑器 tab 降为二级能力，挪进编辑区头——选中对象后才出现）。
   顶栏只剩一级能力：包名菜单 + 问题徽标 + 任务胶囊 + 包级动作 + ⌘K。

   右侧三个动作按钮（自建内容 / 构建 / 运行）是对**当前包**做一次动作，
   不是「进入某个内容区」，所以不占左侧按钮轨（04-align-pack-root.md §7）。
   它们换的是侧边栏内容，所以当前 tool 命中时按钮要亮着。 */
const ACTIONS = [
  {tool: 'content', label: '自建内容', icon: 'wrench'},
  {tool: 'build', label: '构建', icon: 'build'},
  {tool: 'run', label: '运行', icon: 'run'},
] as const;

export function TopBar({onOpenPalette}: {onOpenPalette: () => void}) {
  const {packId, tool} = useUrlState();
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
        {packId && ACTIONS.map(a => (
          <button key={a.tool} type="button"
            className={`tb-btn tb-act${tool === a.tool ? ' on' : ''}`}
            onClick={() => patch({tool: a.tool})}>
            <Icon name={a.icon} size={12}/> {a.label}
          </button>
        ))}
        <button type="button" className="tb-btn" onClick={onOpenPalette}>
          <Icon name="search" size={12}/> <span style={{marginLeft: 4}}>⌘K</span>
        </button>
      </div>
    </header>
  );
}
