import {useEffect, useState} from 'react';
import {fetchHealth, type SystemHealth} from '../api/system';
import {Icon} from '../ui/Icon';
import {usePackSummary} from './PackSummaryContext';
import {useUrlPatch, useUrlState} from './url';
import {loaderLabel} from '../api/packs';

/* 状态条（V3 §1）：环境健康 + 包/目录摘要 + 任务摘要 + 停靠开关。
   「判断」从概览页沉降为常驻信号。
   命令面板入口只在顶栏（右上）—— 这里曾经也放了一个 ⌘K，两个按钮走同一个
   openPalette，功能完全一样，纯冗余。 */
export function StatusBar() {
  const {packId, dock, dtab} = useUrlState();
  const patch = useUrlPatch();
  const {pack, health, score, tasks, loading, error} = usePackSummary();
  const [sys, setSys] = useState<SystemHealth | null>(null);

  useEffect(() => {
    fetchHealth().then(setSys).catch(() => setSys(null));
  }, []);

  const live = tasks.filter(t => t.status === 'running' || t.status === 'queued' || t.status === 'paused');
  const envOk = sys ? (sys.modrinthReachable || sys.curseforgeReachable) && sys.storageWritable : null;
  const alerts = pack?.alerts ?? {crashes: 0, updatable: 0};

  return (
    <footer className="statusbar">
      {/* 健康分可点：弹明细（扣分项）。之前是纯 span 点不了，且 title
          错挂着系统环境文案 —— 环境提示移到圆点的 title 上。 */}
      {/* 健康分可点：打开底部 dock 的「健康」页签（2026-10-05：弹窗取消）。 */}
      <button type="button"
        className={`seg-btn${dock === true && dtab === 'problems' ? ' on' : ''}`}
        title={health && (health.pendingErrors > 0 || health.pendingWarnings > 0) || alerts.crashes > 0 || alerts.updatable > 0
          ? `扣分项：错误 ${health?.pendingErrors ?? 0}（-8/个） · 警告 ${health?.pendingWarnings ?? 0}（-3/个） · 崩溃 ${alerts.crashes}（-6/次） · 可更新 ${alerts.updatable}（-1/个）\n点击在下方工作台查看`
          : '没有任何扣分项'}
        onClick={() => patch({dock: '1', dtab: 'problems'})}>
        <span className="dot" style={{background: envOk === null ? 'var(--mc-muted)' : envOk ? 'var(--mc-success)' : 'var(--mc-fail)'}}
          title={sys ? `Modrinth ${sys.modrinthReachable ? '可达' : '不可达'} · CurseForge ${sys.curseforgeReachable ? '可达' : '不可达'} · 存储可写 ${sys.storageWritable ? '是' : '否'}` : ''}/>
        {packId ? `健康 ${score}` : '工作台'}
      </button>
      {pack && <span className="seg">{pack.name} · v{pack.packVersion} · {pack.mcVersion} · {loaderLabel(pack.loader)}</span>}
      {packId && health && <span className="seg">模组 {health.installed}/{health.mods}</span>}
      {packId && !pack && loading && <span className="seg">载入中…</span>}
      {packId && error && <span className="seg" style={{color: 'var(--mc-fail)'}}>{error}</span>}
      <span className="grow"/>
      {live.length > 0 && (
        <button type="button" className={`seg-btn${dock ? ' on' : ''}`} onClick={() => patch({dock: dock ? null : '1'})}>
          ⏳ {live.length} 个任务 · {live[0].title || live[0].type} {live[0].progress}%
        </button>
      )}
      <button type="button" className={`seg-btn${dock && dtab === 'log' ? ' on' : ''}`}
        onClick={() => patch(dock && dtab === 'log' ? {dock: null} : {dock: '1', dtab: 'log'})}
        style={{display: 'inline-flex', alignItems: 'center', gap: 4}}>
        <Icon name={dock ? 'caretDown' : 'caretUp'} size={12}/> 日志
      </button>
    </footer>
  );
}
