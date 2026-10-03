import {useEffect, useState} from 'react';
import {fetchActivities, type DashboardActivity} from '../api/dashboard';

/* 侧栏页脚：最近动态（吸收自工作台动态流，全局）。固定三条，不占工具面板滚动区。 */
export function ActivityFooter() {
  const [acts, setActs] = useState<DashboardActivity[]>([]);

  useEffect(() => {
    void fetchActivities().then(a => setActs(a.slice(0, 3))).catch(() => setActs([]));
  }, []);

  if (acts.length === 0) return null;

  return (
    <div className="ctx-activity">
      {acts.map(a => (
        <div key={a.id} className="p-row" title={`${a.kind} · ${a.text}`}>
          <span className="dot" style={{background: a.kind === 'resolve' ? 'var(--mc-success)' : a.kind === 'build' ? 'var(--mc-blue)' : 'var(--mc-muted)'}}/>
          <span className="grow sub" style={{fontSize: 12}}>{a.text}</span>
        </div>
      ))}
    </div>
  );
}
