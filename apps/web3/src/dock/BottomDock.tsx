import {useEffect, useRef, useState} from 'react';
import {fetchTaskLog, type TaskLogEvent} from '../api/launcher';
import {type Task} from '../api/tasks';
import {useUrlPatch, useUrlState} from '../app/url';
import {usePackSummary} from '../app/PackSummaryContext';

const STATUS_TEXT: Record<Task['status'], string> = {
  queued: '排队', running: '进行中', success: '成功', failed: '失败', cancelled: '已取消', paused: '已暂停',
};

/* 底部停靠（V3 §1）：左任务列表右日志流。日志 = GET /api/tasks/{id}/log（NDJSON），
   任务运行中每 3s 轮询；没有 command 输入框——后端没有可执行命令的端点，禁假 affordance。 */
export function BottomDock() {
  const {dock} = useUrlState();
  const patch = useUrlPatch();
  const {tasks} = usePackSummary();

  const [selected, setSelected] = useState<string | null>(null);
  const [lines, setLines] = useState<TaskLogEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const logRef = useRef<HTMLDivElement>(null);

  /* 默认选中最近一个运行中/最近的任务 */
  useEffect(() => {
    if (selected && tasks.some(t => t.id === selected)) return;
    const live = tasks.find(t => t.status === 'running' || t.status === 'queued') ?? tasks[0];
    setSelected(live?.id ?? null);
  }, [tasks, selected]);

  useEffect(() => {
    if (!dock || !selected) { setLines([]); return; }
    let alive = true;
    const pull = async () => {
      try {
        const events = await fetchTaskLog(selected);
        if (alive) { setLines(events); setError(null); }
      } catch (e) {
        if (alive) setError(e instanceof Error ? e.message : String(e));
      }
    };
    void pull();
    const t = window.setInterval(pull, 3000);
    return () => { alive = false; window.clearInterval(t); };
  }, [dock, selected]);

  useEffect(() => {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  if (!dock) return null;
  const current = tasks.find(t => t.id === selected) ?? null;

  return (
    <section className="dock">
      <div className="dock-tasks">
        {tasks.length === 0 && <div className="p-empty">还没有后台任务。</div>}
        {tasks.map(t => (
          <div key={t.id} className={`p-row click${t.id === selected ? ' on' : ''}`} onClick={() => setSelected(t.id)}>
            <span className="dot" style={{background: t.status === 'success' ? 'var(--mc-success)' : t.status === 'failed' ? 'var(--mc-fail)' : t.status === 'running' ? 'var(--mc-orange)' : 'var(--mc-muted)'}}/>
            <span className="grow" title={t.title || t.type}>{t.title || t.type}</span>
            <span className="sub">{STATUS_TEXT[t.status]} {t.status === 'running' || t.status === 'queued' ? `${t.progress}%` : ''}</span>
          </div>
        ))}
      </div>
      <div className="dock-log">
        <div className="dock-log-head">
          <span>日志</span>
          {current && <span>{current.title || current.type} · {STATUS_TEXT[current.status]}</span>}
          <span className="grow" style={{flex: 1}}/>
          <span className="sub">{lines.length} 条</span>
          <button type="button" className="p-btn" onClick={() => patch({dock: null})}>收起</button>
        </div>
        <div className="dock-log-body" ref={logRef}>
          {error && <div className="log-line err">{error}</div>}
          {lines.map(ev => (
            <div key={ev.id} className={`log-line${ev.status === 'failed' ? ' err' : ev.status === 'success' ? ' ok' : ''}`}>
              <span className="seq">{ev.sequence}</span>{ev.message}
            </div>
          ))}
          {!error && lines.length === 0 && <div className="log-line">选中一个任务查看日志。</div>}
        </div>
      </div>
    </section>
  );
}
