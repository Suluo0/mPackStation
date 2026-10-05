import {useEffect, useState} from 'react';
import {fetchHealth, type SystemHealth} from '../api/system';
import {usePackSummary} from '../app/PackSummaryContext';

/* 健康面板（dock「健康」页签，2026-10-05）：原来「健康 97」点了弹窗，
   现在常驻在这里 —— 健康分 + 扣分项 + 环境信号（平台可达 / 存储 / CF 密钥）。 */
export function HealthPanel() {
  const {packId, health, score, error} = usePackSummary();
  const [sys, setSys] = useState<SystemHealth | null>(null);

  useEffect(() => {
    fetchHealth().then(setSys).catch(() => setSys(null));
    const t = window.setInterval(() => fetchHealth().then(setSys).catch(() => setSys(null)), 15000);
    return () => window.clearInterval(t);
  }, []);

  const rows: {label: string; value: string; bad?: boolean}[] = [];
  if (packId && health) {
    if (health.pendingErrors > 0) rows.push({label: '错误级待处理', value: `${health.pendingErrors}（每条 -8 分）`, bad: true});
    if (health.pendingWarnings > 0) rows.push({label: '警告级待处理', value: `${health.pendingWarnings}（每条 -3 分）`, bad: true});
  }
  if (sys) {
    rows.push({label: 'Modrinth', value: sys.modrinthReachable ? '可达' : '不可达', bad: !sys.modrinthReachable});
    rows.push({label: 'CurseForge', value: sys.curseforgeReachable ? '可达' : '不可达', bad: !sys.curseforgeReachable});
    rows.push({label: 'CurseForge 密钥', value: sys.curseforgeKeyConfigured ? '已配置' : '未配置 —— 设置页里填一次，会存进用户目录，重启不再丢', bad: !sys.curseforgeKeyConfigured});
    rows.push({label: '存储空间', value: sys.storageWritable ? '可写' : '不可写', bad: !sys.storageWritable});
  }

  return (
    <div className="health-panel">
      {packId && score !== undefined && (
        <div className="p-row">
          <span className="grow">健康分</span>
          <span className="ed-count" style={{color: score >= 90 ? 'var(--mc-success)' : score >= 60 ? 'var(--mc-orange)' : 'var(--mc-fail)'}}>{score}</span>
        </div>
      )}
      {packId && error && <div className="p-empty">{error}</div>}
      {rows.map(r => (
        <div key={r.label} className="p-row">
          <span className="grow">{r.label}</span>
          <span className="sub" style={r.bad ? {color: 'var(--mc-fail)'} : undefined}>{r.value}</span>
        </div>
      ))}
      {!packId && <div className="p-empty">选一个整合包后这里显示它的健康分与扣分项。</div>}
    </div>
  );
}
