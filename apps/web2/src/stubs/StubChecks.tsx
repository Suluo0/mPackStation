import {useEffect, useState} from 'react';
import {listDeliveryChecks, type DeliveryCheck} from '../api/releases';

/* 素面叶子：交付①检查。第三步换成真实检查段（重新检查 + 逐项定位）。
   没有检查记录就显示空态，禁 POST 占位 check（设计文档 §4.4 的 ⚠）。 */
export function StubChecks({packId}: {packId: string}) {
  const [checks, setChecks] = useState<DeliveryCheck[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listDeliveryChecks(packId).then(r => setChecks(r.items)).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      {(checks ?? []).map((c, i) => (
        <div key={`${c.kind}-${i}`} className="rr-line">
          {c.status} · {c.kind} · {c.detail}
        </div>
      ))}
      {checks?.length === 0 && <div className="rr-line">还没有检查记录。先在下面「② 构建」注册一个版本，交付闸门才会跑检查。</div>}
    </div>
  );
}
