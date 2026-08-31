import {Progress} from 'antd';
import {ArrowRightOutlined, StarOutlined} from '@ant-design/icons';
import type {DashboardPack} from '../../api/dashboard';
import {AlertSignals, ConflictSignal, EditCounts, SignalGroup, relativeTime} from './signals';
import {packCoverGradient} from './cover';
import {WorkbenchCard, WorkbenchButton} from '../../ui/workbench/Workbench';

/* 继续工作卡：最近编辑的包，全页视觉权重最高。 */

export function PackCover({id, small}: {id: string; small?: boolean}) {
  return (
    <span className={small ? 'db-cover db-cover-sm' : 'db-cover'}
          style={{background: packCoverGradient(id)}}/>
  );
}

export function ContinueCard({pack, onOpen}: {pack: DashboardPack; onOpen: (id: string) => void}) {
  const progress = pack.modCount.total > 0 ? Math.round((pack.modCount.installed / pack.modCount.total) * 100) : 0;
  return (
    <WorkbenchCard className="db-continue">
      <div className="db-continue-info">
        <PackCover id={pack.id}/>
        <div className="db-continue-info-text">
          <div className="db-h2" style={{display: 'flex', alignItems: 'center', gap: 6}}>
            {pack.name} <StarOutlined style={{fontSize: 14, color: 'var(--mc-muted)'}}/>
          </div>
          <div style={{marginTop: 6}}>
            <span className="db-env-tag db-env-tag-mc">MC {pack.mcVersion}</span>
            <span className="db-env-tag db-env-tag-loader">{loaderLabel(pack.loader)}</span>
          </div>
          <div className="db-muted" style={{marginTop: 6}}>{pack.modCount.total} 个模组 · 最后编辑 {relativeTime(pack.lastEditedAt)}</div>
        </div>
      </div>
      <div className="db-continue-signals">
        <SignalGroup label="健康状态">
          <ConflictSignal resolved={pack.conflicts.resolved} pending={pack.conflicts.pending}/>
        </SignalGroup>
        <SignalGroup label="内容编辑">
          <EditCounts edits={pack.edits}/>
        </SignalGroup>
        <SignalGroup label="告警">
          <AlertSignals crashes={pack.alerts.crashes} updatable={pack.alerts.updatable}/>
        </SignalGroup>
      </div>
      <div className="db-continue-action">
        <WorkbenchButton tone="primary" size="large" onClick={() => onOpen(pack.id)}>继续设计 <ArrowRightOutlined/></WorkbenchButton>
        <div className="db-continue-meta"><span>上次编辑：{relativeTime(pack.lastEditedAt)}</span></div>
        <div className="db-continue-meta"><span>模组安装</span><span>{pack.modCount.installed}/{pack.modCount.total} · {progress}%</span></div>
        <Progress percent={progress} size="small" showInfo={false}/>
      </div>
    </WorkbenchCard>
  );
}

export function loaderLabel(loader: string): string {
  return {forge: 'Forge', neoforge: 'NeoForge', fabric: 'Fabric', quilt: 'Quilt'}[loader] ?? loader;
}
