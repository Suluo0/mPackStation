import type {ReactNode} from 'react';
import {CheckCircleFilled, ExclamationCircleFilled, SyncOutlined} from '@ant-design/icons';

/* 健康信号 —— 全站唯一渲染逻辑（规格：dashboard-page-prompt.md 4.2.1）。
   规则集中在此：
   - 冲突永远渲染为「已解决 N · 待解决 M」，先扬后警；M>0 红色加粗，M=0 全组灰色低调。
   - 崩溃/可更新为告警：>0 才渲染，=0 不渲染，未知(null)显示「-」而非 0。 */

export function formatCount(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
}

export function formatBytes(bytes: number): string {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
}

export function relativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const minutes = Math.floor(diffMs / 60_000);
  if (minutes < 1) return '刚刚';
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  if (days === 1) return '昨天';
  if (days < 7) return `${days} 天前`;
  return new Date(iso).toLocaleDateString('zh-CN');
}

export function ConflictSignal({resolved, pending}: {resolved: number | null; pending: number | null}) {
  if (resolved === null || pending === null) return <span className="sig sig-dim">冲突 -</span>;
  if (pending === 0) {
    return <span className="sig sig-dim"><CheckCircleFilled/>已解决 {formatCount(resolved)} · 无待解决</span>;
  }
  return (
    <span className="sig">
      <span className="sig-resolved"><CheckCircleFilled/> <span className="sig-num">{formatCount(resolved)}</span> 已解决</span>
      <span className="sig-dim">·</span>
      <span className="sig-pending"><span className="sig-num">{formatCount(pending)}</span> 待解决</span>
    </span>
  );
}

export function CrashSignal({count}: {count: number | null}) {
  if (count === null) return <span className="sig sig-dim">崩溃 -</span>;
  if (count === 0) return null;
  return <span className="sig sig-crash"><ExclamationCircleFilled/>崩溃 {formatCount(count)}</span>;
}

export function UpdatableSignal({count}: {count: number | null}) {
  if (count === null) return <span className="sig sig-dim">可更新 -</span>;
  if (count === 0) return null;
  return <span className="sig sig-update"><SyncOutlined/>可更新 {formatCount(count)}</span>;
}

export function AlertSignals({crashes, updatable}: {crashes: number | null; updatable: number | null}) {
  const crash = <CrashSignal count={crashes}/>;
  const update = <UpdatableSignal count={updatable}/>;
  if (!crash && !update) return <span className="sig sig-dim">无告警</span>;
  return <span className="sig" style={{gap: 12}}>{crash}{update}</span>;
}

export function EditCounts({edits}: {edits: {recipes: number; structures: number; ores: number; quests: number}}) {
  const total = edits.recipes + edits.structures + edits.ores + edits.quests;
  if (total === 0) return <span className="sig sig-dim">尚未编辑</span>;
  return (
    <span className="sig sig-dim">
      配方 {formatCount(edits.recipes)} · 结构 {formatCount(edits.structures)} · 矿脉 {formatCount(edits.ores)} · 任务 {formatCount(edits.quests)}
    </span>
  );
}

const taskStatusMeta: Record<string, {text: string; color: string; bg: string}> = {
  running: {text: '运行中', color: 'var(--mc-accent)', bg: 'var(--mc-accent-bg)'},
  success: {text: '已完成', color: 'var(--mc-success)', bg: 'var(--mc-success-bg)'},
  failed: {text: '失败', color: 'var(--mc-fail)', bg: 'var(--mc-fail-bg)'},
  cancelled: {text: '已取消', color: 'var(--mc-muted)', bg: 'var(--mc-bg)'},
  paused: {text: '已暂停', color: 'var(--mc-muted)', bg: 'var(--mc-bg)'},
};

export function TaskStatusTag({status}: {status: string}) {
  const meta = taskStatusMeta[status] ?? taskStatusMeta.cancelled;
  return (
    <span style={{color: meta.color, background: meta.bg, borderRadius: 'var(--mc-radius)', padding: '1px 8px', fontSize: 12, whiteSpace: 'nowrap'}}>
      {meta.text}
    </span>
  );
}

export function SignalGroup({label, children}: {label: string; children: ReactNode}) {
  return (
    <div className="db-signal-group">
      <span className="db-signal-label">{label}</span>
      {children}
    </div>
  );
}
