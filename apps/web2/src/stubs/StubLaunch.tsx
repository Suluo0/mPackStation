import {usePackSummary} from '../app/PackSummaryContext';

/* 素面叶子：交付④起窗。第三步换成 LaunchSection（用户名/内存/Java 路径 + 启动 + 实时日志轮询）。
   这里只显示任务域里与本包相关的真实记录，不渲染没有参数表单的启动按钮。 */
export function StubLaunch({packId}: {packId: string}) {
  const {tasks} = usePackSummary();
  const mine = tasks.filter(t => t.packId === packId);
  const recent = mine.slice(0, 6);

  return (
    <div>
      <div className="rr-line">本包任务 {mine.length} 条（最近 {recent.length} 条）</div>
      {recent.map(t => (
        <div key={t.id} className="rr-line">{t.title || t.type} · {t.status} · {t.progress}%{t.error ? ` · ${t.error}` : ''}</div>
      ))}
      {recent.length === 0 && <div className="rr-line">本包还没有后台任务记录。装好版本后即可起窗。</div>}
    </div>
  );
}
