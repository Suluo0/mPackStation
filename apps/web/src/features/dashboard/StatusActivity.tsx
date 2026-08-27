import type {DashboardActivity, SystemStatus} from './types';
import {formatBytes, relativeTime} from './signals';

/* 环境与动态（弱存在区）：环境状态卡 + 最近动态卡。 */

export function StatusActivity({status, activities}: {
  status: SystemStatus | null;
  activities: DashboardActivity[];
}) {
  return (
    <div className="db-bottom-row">
      <div className="db-card db-status-card">
        <h2 className="db-h2" style={{marginBottom: 'var(--mc-sp-sm)'}}>环境状态</h2>
        {status === null ? (
          <span className="db-muted">状态未知 -</span>
        ) : (
          <>
            <div className="db-status-line">
              <span className={status.modrinthReachable ? 'db-dot db-dot-ok' : 'db-dot db-dot-bad'}/>
              <span className={status.modrinthReachable ? 'db-muted' : ''}>Modrinth {status.modrinthReachable ? '连接正常' : '连接失败'}</span>
            </div>
            <div className="db-status-line">
              <span className={status.curseforgeReachable ? 'db-dot db-dot-ok' : 'db-dot db-dot-bad'}/>
              <span className={status.curseforgeReachable ? 'db-muted' : ''}>CurseForge {status.curseforgeReachable ? '连接正常' : '连接失败'}</span>
            </div>
            <div className="db-status-line db-muted">数据缓存 <span className="db-status-num">{formatBytes(status.cacheSizeBytes)}</span></div>
            <div className="db-status-line db-muted">存储剩余 <span className="db-status-num">{formatBytes(status.storageFreeBytes)}</span></div>
          </>
        )}
      </div>
      <div className="db-card db-activity-card">
        <h2 className="db-h2" style={{marginBottom: 'var(--mc-sp-sm)'}}>最近动态</h2>
        {activities.length === 0 && <span className="db-muted">还没有动态，去创建第一个整合包吧。</span>}
        {activities.map(a => (
          <div key={a.id} className="db-activity">
            <span className="db-activity-time">{relativeTime(a.at)}</span>
            <span className={`db-activity-dot db-activity-${a.kind}`}/>
            <span>{a.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
