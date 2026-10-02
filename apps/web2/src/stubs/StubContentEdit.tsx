import {useEffect, useState} from 'react';
import {useParams, useSearchParams} from 'react-router-dom';
import {listContent, type ContentDocument} from '../api/content';

/* 素面叶子：魔改（写）态的文档列表。第三步换成 ContentEditPanel（JSON 编辑器 + 校验/应用）。
   选中写 ?doc=（§3.1），apply 成功后由第三步调 usePackSummary().refresh()。 */
export function StubContentEdit() {
  const {id} = useParams();
  const [docs, setDocs] = useState<ContentDocument[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [params, setParams] = useSearchParams();
  const doc = params.get('doc');

  useEffect(() => {
    if (!id) return;
    listContent(id).then(setDocs).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [id]);

  const pick = (documentId: string) => setParams(p => {
    const n = new URLSearchParams(p);
    n.set('doc', documentId);
    return n;
  }, {replace: true});

  return (
    <div className="cp-panel">
      {error && <div className="rr-line">{error}</div>}
      <div className="rr-line">内容文档 {docs.length} 份</div>
      {docs.length === 0 && !error && <div className="rr-line">这个包还没有魔改文档。索引态里选一个配方再去魔改可以建第一份。</div>}
      {docs.map(d => (
        <div key={d.id} className="rr-line" style={{cursor: 'pointer', color: doc === d.id ? 'var(--mc-primary-deep)' : undefined}} onClick={() => pick(d.id)}>
          {d.kind} · {d.title} · {d.updatedAt}
        </div>
      ))}
    </div>
  );
}
