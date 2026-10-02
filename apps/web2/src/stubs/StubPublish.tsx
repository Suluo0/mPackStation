import {useEffect, useState} from 'react';
import {listReleases, type Release} from '../api/releases';

/* 素面叶子：交付⑤发布。第三步换成 PublishSection（CurseForge / Modrinth 表单 + 轮询 + 重试）。 */
export function StubPublish({packId}: {packId: string}) {
  const [rows, setRows] = useState<Release[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listReleases(packId).then(setRows).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      <div className="rr-line">发布记录 {rows.length}</div>
      {rows.map(r => (
        <div key={r.id} className="rr-line">
          {r.provider} · {r.status}{r.errorCode ? ` · ${r.errorCode}` : ''}{r.errorMessage ? ` · ${r.errorMessage}` : ''}
        </div>
      ))}
      {rows.length === 0 && !error && <div className="rr-line">还没有发布记录。构建出 ready 的产物后即可发布。</div>}
    </div>
  );
}
