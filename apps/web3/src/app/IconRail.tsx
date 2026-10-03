import {TOOLS, useUrlPatch, useUrlState} from './url';
import {usePackSummary} from './PackSummaryContext';
import {Icon} from './Icon';

/* 图标轨（第二轮反馈后三个工具）：来源 / 构建（Git 分支图标）/ 运行。
   徽标 = 运行面板的活跃任务数；问题不再占轨道（顶栏徽标负责）。 */
export function IconRail() {
  const {tool} = useUrlState();
  const patch = useUrlPatch();
  const {tasks} = usePackSummary();

  const live = tasks.filter(t => t.status === 'running' || t.status === 'queued' || t.status === 'paused').length;

  return (
    <nav className="icon-rail">
      {TOOLS.map(t => (
        <button key={t.tool} type="button"
          className={`rail-btn${tool === t.tool ? ' on' : ''}`}
          title={t.label} aria-label={t.label}
          onClick={() => patch({tool: t.tool})}>
          <Icon name={t.tool}/>
          {t.tool === 'run' && live > 0 && <span className="rail-badge">{live}</span>}
        </button>
      ))}
      <div className="rail-spacer"/>
    </nav>
  );
}
