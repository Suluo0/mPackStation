import {useEffect, useState} from 'react';
import {useNavigate} from 'react-router-dom';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';

const pending = (p: DashboardPack) => p.conflicts.pending + p.alerts.crashes + p.alerts.updatable > 0;

/* 素面叶子：包列表 + 「只看待处理」开关。开关放在本组件里，PackList 的契约才是零 props（§9）。
   第三步换成 PackList（卡片 + 健康分 + 新建/导入入口）。 */
export function StubPackList() {
  const [packs, setPacks] = useState<DashboardPack[]>([]);
  const [onlyPending, setOnlyPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const nav = useNavigate();

  useEffect(() => { fetchDashboard().then(d => setPacks(d.packs)).catch(e => setError(e instanceof Error ? e.message : String(e))); }, []);

  const rows = onlyPending ? packs.filter(pending) : packs;
  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      <label className="rr-line">
        <input type="checkbox" checked={onlyPending} onChange={e => setOnlyPending(e.target.checked)}/> 只看待处理（{packs.filter(pending).length}/{packs.length}）
      </label>
      {rows.map(p => (
        <div key={p.id} className="rr-line">
          {p.name} · MC {p.mcVersion} · {p.loader} · {p.modCount.total} 模组 · 待处理 {pending(p) ? p.conflicts.pending + p.alerts.crashes + p.alerts.updatable : 0}
          <button type="button" className="tb-btn" onClick={() => nav(`/packs/${encodeURIComponent(p.id)}`)}>打开</button>
        </div>
      ))}
      {rows.length === 0 && !error && <div className="rr-line">{onlyPending ? '没有待处理的包。' : '一个包都没有。'}</div>}
    </div>
  );
}
