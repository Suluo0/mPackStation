import {useEffect, useRef, useState} from 'react';
import {fetchTaskLog, type TaskLogEvent} from '../api/launcher';
import {cancelTask, pauseTask, resumeTask, retryTask, type Task} from '../api/tasks';
import {useUrlPatch, useUrlState} from '../app/url';
import {usePackSummary} from '../app/PackSummaryContext';
import {ProblemsPanel} from './ProblemsPanel';
import {HealthPanel} from './HealthPanel';
import {Button} from '../ui/Button';
import {DetailFields} from '../ui/DetailFields';
import {MasterDetail} from '../ui/MasterDetail';

const STATUS_TEXT: Record<Task['status'], string> = {
  queued: '排队', running: '进行中', success: '成功', failed: '失败', cancelled: '已取消', paused: '已暂停',
};

const STATUS_DOT: Record<Task['status'], string> = {
  queued: 'var(--mc-muted)', running: 'var(--mc-orange)', success: 'var(--mc-success)',
  failed: 'var(--mc-fail)', cancelled: 'var(--mc-muted)', paused: 'var(--mc-muted)',
};

function clock(iso: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleTimeString('zh-CN', {hour12: false});
}

function elapsed(t: Task): string {
  if (!t.startedAt) return '—';
  const a = new Date(t.startedAt).getTime();
  const b = t.finishedAt ? new Date(t.finishedAt).getTime() : Date.now();
  if (Number.isNaN(a)) return '—';
  const ms = Math.max(0, b - a);
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}

/* 底部停靠（V3 §1）：大标题 Log + 左任务列表 + 右详情。
   日志 = GET /api/tasks/{id}/log（NDJSON），任务运行中每 3s 轮询；
   没有 command 输入框——后端没有可执行命令的端点，禁假 affordance。

   布局走 ui/MasterDetail：以前这里手写过「选中项不在列表里就回落到第一条」，
   那份兜底现在由组件统一给。 */
export function BottomDock() {
  const {dock, dtab} = useUrlState();
  const patch = useUrlPatch();
  const {tasks, pendingConflicts} = usePackSummary();

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

  /* 两页签（2026-10-05 用户反馈：健康不是独立页面——打开问题就应先看到健康概览）：
     顶栏徽标与状态条健康分点击后直接落到问题页签，不再弹窗。日志页签保持原 MasterDetail。 */
  const tabs = [
    {key: 'log' as const, label: `日志${tasks.length ? ` · ${tasks.length}` : ''}`},
    {key: 'problems' as const, label: `问题${pendingConflicts.length ? ` · ${pendingConflicts.length}` : ''}`},
  ];
  const openTab = (key: 'log' | 'problems') =>
    patch(dock && dtab === key ? {dock: null} : {dock: '1', dtab: key});

  return (
    <section className="md dock">
      <header className="md-head">
        <span className="md-title">工作台</span>
        <span className="dock-tabs">
          {tabs.map(tb => (
            <button key={tb.key} type="button"
              className={`dock-tab${dtab === tb.key ? ' on' : ''}`}
              aria-pressed={dtab === tb.key}
              onClick={() => openTab(tb.key)}>{tb.label}</button>
          ))}
        </span>
        <span style={{flex: 1}}/>
        <Button onClick={() => patch({dock: null})}>收起</Button>
      </header>
      {dtab === 'problems' && (
        <div className="dock-body dock-scroll">
          <HealthPanel/>
          <ProblemsPanel/>
        </div>
      )}
      {dtab === 'log' && (
    <MasterDetail<Task>
      className="dock-log"
      title="任务日志"
      titleExtra={tasks.length > 0 ? `${tasks.length} 个任务` : null}
      items={tasks}
      itemKey={t => t.id}
      selectedId={selected}
      onSelect={setSelected}
      listWidth={240} minListWidth={160} maxListWidth={420}
      listEmpty="还没有后台任务。"
      renderItem={(t, on) => (
        <div className={`p-row click${on ? ' on' : ''}`} title={t.title || t.type}>
          <span className="dot" style={{background: STATUS_DOT[t.status]}}/>
          <span className="grow">{t.title || t.type}</span>
          <span className="sub">
            {STATUS_TEXT[t.status]} {t.status === 'running' || t.status === 'queued' ? `${t.progress}%` : ''}
          </span>
          {(t.status === 'running' || t.status === 'queued') && (
            <button type="button" className="p-btn" title="暂停" onClick={e => { e.stopPropagation(); void pauseTask(t.id).catch(() => undefined); }}>⏸</button>
          )}
          {t.status === 'paused' && (
            <button type="button" className="p-btn" title="继续" onClick={e => { e.stopPropagation(); void resumeTask(t.id).catch(() => undefined); }}>▶</button>
          )}
          {(t.status === 'failed' || t.status === 'cancelled') && (
            <button type="button" className="p-btn" title="重试" onClick={e => { e.stopPropagation(); void retryTask(t.id).catch(() => undefined); }}>↺</button>
          )}
          {(t.status === 'running' || t.status === 'queued' || t.status === 'paused') && (
            <button type="button" className="p-btn" title="取消" onClick={e => { e.stopPropagation(); void cancelTask(t.id).catch(() => undefined); }}>✕</button>
          )}
        </div>
      )}
      detailTitle={t => `${t.title || t.type} · ${STATUS_TEXT[t.status]}`}
      detailActions={() => (
        /* 真能力：把这条任务的日志按「序号 消息」拼成纯文本进剪贴板。
           没有后端端点支持导出，所以不做「导出文件」按钮。 */
        <Button title="复制这条任务的全部日志"
          onClick={() => void navigator.clipboard?.writeText(
            lines.map(ev => `${ev.sequence}  ${ev.message}`).join('\n'))}>
          复制日志
        </Button>
      )}
      detailEmpty="左侧选一个任务查看日志。"
      renderDetail={t => (
        <>
          <DetailFields fields={[
            {label: '类型', value: t.type, mono: true},
            {label: '状态', value: STATUS_TEXT[t.status],
              tone: t.status === 'failed' ? 'fail' : t.status === 'success' ? 'ok' : undefined},
            {label: '进度', value: `${t.progress}%`},
            {label: '开始', value: clock(t.startedAt), mono: true},
            {label: '结束', value: clock(t.finishedAt), mono: true},
            {label: '耗时', value: elapsed(t), mono: true},
            ...(t.error ? [{label: '错误', value: t.error, wide: true, tone: 'fail' as const}] : []),
          ]}/>
          <div className="dock-log-body" ref={logRef}>
            {error && <div className="log-line err">{error}</div>}
            {lines.map(ev => (
              <div key={ev.id} className={`log-line${ev.status === 'failed' ? ' err' : ev.status === 'success' ? ' ok' : ''}`}>
                <span className="seq">{ev.sequence}</span>{ev.message}
              </div>
            ))}
            {!error && lines.length === 0 && <div className="log-line">这条任务还没有日志。</div>}
          </div>
        </>
      )}
    />
      )}
    </section>
  );
}
