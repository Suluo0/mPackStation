import type {Task} from '../api/tasks';

const STATUS_TEXT: Record<Task['status'], string> = {
  queued: '排队中', running: '进行中', success: '成功', failed: '失败', cancelled: '已取消', paused: '已暂停',
};

/* 素面叶子：任务列表。第三步换成真实任务面板（暂停/继续/取消/重试 + 错误详情）。 */
export function StubTasks({tasks}: {tasks: Task[]}) {
  if (tasks.length === 0) return <div className="rr-line">没有后台任务。</div>;
  return (
    <div>
      {tasks.map(t => (
        <div key={t.id} className="rr-line">
          {t.title || t.type} · {STATUS_TEXT[t.status]} · {t.progress}%
          {t.packName ? ` · ${t.packName}` : ''}
          {t.error ? ` · ${t.error}` : ''}
        </div>
      ))}
    </div>
  );
}
