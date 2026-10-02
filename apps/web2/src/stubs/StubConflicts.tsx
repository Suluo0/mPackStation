import {useState} from 'react';
import {ignoreConflict, resolveConflict} from '../api/mods';
import {usePackSummary} from '../app/PackSummaryContext';

/* 素面叶子：冲突就地处置（设计文档 §5-4）。成功后 refresh()，
   健康分、顶栏告警、交付闸门三处同时更新 —— 这就是 §7.3 第 11 条的判据。 */
export function StubConflicts() {
  const {packId, pendingConflicts, refresh} = usePackSummary();
  const [busy, setBusy] = useState('');
  const [error, setError] = useState<string | null>(null);

  if (!packId) return null;
  if (pendingConflicts.length === 0) return <div className="rr-line">没有待解决冲突。</div>;

  const act = async (id: string, kind: 'resolve' | 'ignore') => {
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

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      {pendingConflicts.map(c => (
        <div key={c.id} className="rr-line">
          {c.severity} · {c.kind} · {c.summary}
          <button type="button" className="tb-btn" disabled={busy !== ''} onClick={() => act(c.id, 'resolve')}>处置</button>
          <button type="button" className="tb-btn" disabled={busy !== ''} onClick={() => act(c.id, 'ignore')}>忽略</button>
        </div>
      ))}
    </div>
  );
}
