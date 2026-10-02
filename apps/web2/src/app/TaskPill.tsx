import {useState} from 'react';
import {Drawer} from 'antd';
import {usePackSummary} from './PackSummaryContext';
import {StubTasks} from '../stubs/StubTasks';

/* 非终态：queued/running/paused。Task.status 的枚举里没有 pending（api/tasks.ts:13），
   别按 'pending' 判，否则活跃任务永远不进胶囊。 */
const LIVE = new Set(['queued', 'running', 'paused']);

/* 长任务不打断当前工作（设计文档 §5-6）：轮询在 PackSummaryContext，
   这里只是它的一个视图；离开页面、切态、切页都不会丢进度。 */
export function TaskPill() {
  const {tasks} = usePackSummary();
  const [open, setOpen] = useState(false);
  const live = tasks.filter(t => LIVE.has(t.status));
  if (live.length === 0) return null;

  const latest = live[0];
  const paused = live.every(t => t.status === 'paused');

  return (
    <>
      <button type="button" className="task-pill" onClick={() => setOpen(true)}
        title={live.map(t => `${t.title || t.type}：${t.status} ${t.progress}%`).join('\n')}>
        <span aria-hidden>{paused ? '⏸' : '⏳'}</span>
        <span>{latest.title || latest.type}</span>
        <span>{latest.progress}%</span>
        {live.length > 1 && <span>· 共 {live.length} 个</span>}
      </button>
      <Drawer title="后台任务" width={520} open={open} onClose={() => setOpen(false)}>
        <StubTasks tasks={tasks}/>
      </Drawer>
    </>
  );
}
