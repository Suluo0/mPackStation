import {useEffect, useState} from 'react';
import {fetchHealth, type SystemHealth} from '../api/system';
import {usePackSummary} from './PackSummaryContext';
import {useUrlPatch, useUrlState} from './url';

/* 状态条（V3 §1）：环境健康 + 包/目录摘要 + 任务摘要 + 停靠开关。
   「判断」从概览页沉降为常驻信号。 */
export function StatusBar({onOpenPalette}: {onOpenPalette: () => void}) {
  const {packId, dock} = useUrlState();
  const patch = useUrlPatch();
  const {pack, health, score, tasks, loading, error} = usePackSummary();
  const [sys, setSys] = useState<SystemHealth | null>(null);

  useEffect(() => {
    fetchHealth().then(setSys).catch(() => setSys(null));
  }, []);

  const live = tasks.filter(t => t.status === 'running' || t.status === 'queued' || t.status === 'paused');
  const envOk = sys ? (sys.modrinthReachable || sys.curseforgeReachable) && sys.storageWritable : null;

  return (
    <footer className="statusbar">
      <span className="seg" title={sys ? `Modrinth ${sys.modrinthReachable ? '可达' : '不可达'} · CurseForge ${sys.curseforgeReachable ? '可达' : '不可达'} · 存储可写 ${sys.storageWritable ? '是' : '否'}` : ''}>
        <span className="dot" style={{background: envOk === null ? 'var(--mc-muted)' : envOk ? 'var(--mc-success)' : 'var(--mc-fail)'}}/>
        {packId ? `健康 ${score}` : '工作台'}
      </span>
      {pack && <span className="seg">{pack.name} · v{pack.packVersion} · MC {pack.mcVersion} · {pack.loader}</span>}
      {packId && health && <span className="seg">模组 {health.installed}/{health.mods}</span>}
      {packId && !pack && loading && <span className="seg">载入中…</span>}
      {packId && error && <span className="seg" style={{color: 'var(--mc-fail)'}}>{error}</span>}
      <span className="grow"/>
      {live.length > 0 && (
        <button type="button" className={`seg-btn${dock ? ' on' : ''}`} onClick={() => patch({dock: dock ? null : '1'})}>
          ⏳ {live.length} 个任务 · {live[0].title || live[0].type} {live[0].progress}%
        </button>
      )}
      <button type="button" className={`seg-btn${dock ? ' on' : ''}`} onClick={() => patch({dock: dock ? null : '1'})}>
        ▴ 日志
      </button>
      <button type="button" className="seg-btn" onClick={onOpenPalette}>⌘K</button>
    </footer>
  );
}
