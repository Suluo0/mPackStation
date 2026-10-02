import {useEffect, useState} from 'react';
import {fetchActivities, type DashboardActivity} from '../api/dashboard';

/* 素面叶子：最近动态。第三步换成 ActivityFeed（按类型图标 + 点进对应包）。 */
export function StubActivity() {
  const [rows, setRows] = useState<DashboardActivity[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchActivities().then(setRows).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      {rows.map(a => <div key={a.id} className="rr-line">{a.at} · {a.kind} · {a.text}</div>)}
      {rows.length === 0 && !error && <div className="rr-line">还没有动态。</div>}
    </div>
  );
}
