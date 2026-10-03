import {useEffect, useRef, useState} from 'react';
import {ignoreConflict, resolveConflict, type Conflict} from '../api/mods';
import {usePackSummary} from './PackSummaryContext';

/* 顶栏问题徽标的弹出层：冲突就地处置（VSCode Problems 的对齐物）。
   数据与徽标同源（usePackSummary.pendingConflicts），处置后 refresh 让三处信号同时更新。 */
export function ProblemsPopover({onClose}: {onClose: () => void}) {
  const {packId, pendingConflicts, refresh} = usePackSummary();
  const [busy, setBusy] = useState('');
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (ref.current?.contains(target) || target.closest('.problems-chip')) return;
      onClose();
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [onClose]);

  const act = async (id: string, kind: 'resolve' | 'ignore') => {
    if (!packId) return;
    setBusy(id + kind);
    setError(null);
    try {
      if (kind === 'resolve') await resolveConflict(packId, id);
      else await ignoreConflict(packId, id);
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const rows: Conflict[] = pendingConflicts;

  return (
    <div className="ps-menu problems-menu" ref={ref}>
      {error && <div className="p-empty">{error}</div>}
      {rows.length === 0 && !error && <div className="ps-empty">没有待解决冲突。</div>}
      {rows.map(c => (
        <div key={c.id} className="p-row" style={{alignItems: 'flex-start'}}>
          <span className="dot" style={{background: c.severity === 'error' ? 'var(--mc-fail)' : 'var(--mc-orange)', marginTop: 5}}/>
          <span className="stack grow" style={{whiteSpace: 'normal'}}>
            <span style={{whiteSpace: 'normal', wordBreak: 'break-all'}}>{c.summary}</span>
            <span className="sub" style={{whiteSpace: 'normal'}}>{c.severity} · {c.kind}</span>
          </span>
          <span style={{display: 'flex', gap: 3, flex: 'none'}}>
            <button type="button" className="p-btn" disabled={busy !== ''} onClick={() => void act(c.id, 'resolve')}>处置</button>
            <button type="button" className="p-btn" disabled={busy !== ''} onClick={() => void act(c.id, 'ignore')}>忽略</button>
          </span>
        </div>
      ))}
      {rows.length > 0 && (
        <div className="ps-filter">处置 / 忽略后徽标与健康分即时更新；构建闸门看 error 级。</div>
      )}
    </div>
  );
}
