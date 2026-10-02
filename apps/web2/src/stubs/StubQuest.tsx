import {useEffect, useState} from 'react';
import {useParams, useSearchParams} from 'react-router-dom';
import {getQuest, type QuestBook} from '../api/content';

/* 素面叶子：编排态的任务书。第三步换成 QuestPanel（画布 + Inspector 表单）。
   选中节点写 ?node=（§3.1），节点 id 也是 useFocus 的 f=node&fid= 的来源。 */
export function StubQuest() {
  const {id} = useParams();
  const [book, setBook] = useState<QuestBook | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [params, setParams] = useSearchParams();
  const node = params.get('node');

  useEffect(() => {
    if (!id) return;
    getQuest(id).then(setBook).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [id]);

  const nodes = book?.revision.draft.nodes ?? [];
  const pick = (nodeId: string) => setParams(p => {
    const n = new URLSearchParams(p);
    n.set('node', nodeId);
    return n;
  }, {replace: true});

  return (
    <div className="cp-panel">
      {error && <div className="rr-line">{error}</div>}
      <div className="rr-line">任务书：章节 {(book?.revision.draft.chapters ?? []).length} · 节点 {nodes.length}</div>
      {!error && nodes.length === 0 && <div className="rr-line">任务书还是空的。先在索引态挑一个物品，编排态就能带着它建第一个节点。</div>}
      {nodes.map(n => (
        <div key={n.id} className="rr-line" style={{cursor: 'pointer', color: node === n.id ? 'var(--mc-primary-deep)' : undefined}} onClick={() => pick(n.id)}>
          {n.title || '(未命名)'} · 章节 {n.chapterId}
        </div>
      ))}
    </div>
  );
}
