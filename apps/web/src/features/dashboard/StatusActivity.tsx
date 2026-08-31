import type {DashboardActivity} from '../../api/dashboard';
import type {SystemStatus} from '../../api/system';
import {formatBytes, relativeTime} from './signals';
import {WorkbenchCard} from '../../ui/workbench/Workbench';

/* 环境与动态（弱存在区）：环境状态卡 + 最近动态卡。 */

export function StatusActivity({status, activities}: {
  status: SystemStatus | null;
  activities: DashboardActivity[];
}) {
  return (
    <div className="db-bottom-row">
      <WorkbenchCard className="db-status-card">
        <h2 className="db-h2" style={{marginBottom: 'var(--mc-sp-sm)'}}>环境状态</h2>
        {status === null ? (
          <span className="db-muted">状态未知 -</span>
        ) : (
          <>
            <div className="db-status-line">
              <span className={status.modrinthStatus === 'unavailable' ? 'db-dot db-dot-bad' : 'db-dot db-dot-ok'}/>
              <span className={status.modrinthStatus === 'unavailable' ? '' : 'db-muted'}>Modrinth {status.modrinthStatus === 'unavailable' ? '连接失败' : status.modrinthStatus === 'unknown' ? '尚未探测' : '连接正常'}</span>
            </div>
            <div className="db-status-line">
              <span className={status.curseforgeStatus === 'unavailable' ? 'db-dot db-dot-bad' : 'db-dot db-dot-ok'}/>
              <span className={status.curseforgeStatus === 'unavailable' ? '' : 'db-muted'}>CurseForge {status.curseforgeStatus === 'unavailable' ? '连接失败' : status.curseforgeStatus === 'unknown' ? '尚未探测' : '连接正常'}</span>
            </div>
            <div className="db-status-line db-muted">数据缓存 <span className="db-status-num">{formatBytes(status.cacheSizeBytes)}</span></div>
            <div className="db-status-line db-muted">存储剩余 <span className="db-status-num">{formatBytes(status.storageFreeBytes)}</span></div>
          </>
        )}
      </WorkbenchCard>
      <WorkbenchCard className="db-activity-card">
        <h2 className="db-h2" style={{marginBottom: 'var(--mc-sp-sm)'}}>最近动态</h2>
        {activities.length === 0 && <span className="db-muted">还没有动态，去创建第一个整合包吧。</span>}
        {activities.map(a => (
          <div key={a.id} className="db-activity">
            <span className="db-activity-time">{relativeTime(a.at)}</span>
            <span className={`db-activity-dot db-activity-${a.kind}`}/>
            <span>{a.text}</span>
          </div>
        ))}
      </WorkbenchCard>
    </div>
  );
}
