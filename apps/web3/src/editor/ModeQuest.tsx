import {useEffect, useState} from 'react';
import {getQuest, type QuestBook} from '../api/content';
import {useUrlPatch, useUrlState} from '../app/url';

/* 编排态 v0：节点密表格。无限画布（拖拽/连线/Inspector）由 3D 落地；
   章节列表在侧栏「来源」面板（编排态整体替换）。 */
export function ModeQuest() {
  const {packId, node} = useUrlState();
  const patch = useUrlPatch();
  const [book, setBook] = useState<QuestBook | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!packId) return;
    getQuest(packId).then(setBook).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  if (!packId) return null;

  const nodes = book?.revision.draft.nodes ?? [];
  const chapterTitle = (id: string) => book?.revision.draft.chapters.find(c => c.id === id)?.title || id;

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">编排</span>
        <span className="sub">任务书 · 章节 {book?.revision.draft.chapters.length ?? 0} · 节点 {nodes.length}</span>
        <span className="grow"/>
        {book && <span className="ed-count">draft r{book.revision.revision}</span>}
      </div>
      <div className="ed-scroll">
        {error && <div className="ed-placeholder"><b>{error}</b></div>}
        {!error && nodes.length === 0 && (
          <div className="ed-placeholder">
            任务书还是空的。<br/>
            无限画布（双击建节点 / 拖拽连线 / 分组 Inspector / 模拟完成预览）由 <b>3D QuestPanel</b> 落地；
            「魔改完一条配方，顺手生成任务节点」的入口在 3C。
          </div>
        )}
        {!error && nodes.length > 0 && (
          <table className="ed-table">
            <thead>
              <tr><th style={{width: '40%'}}>节点</th><th>章节</th><th>前置</th><th>奖励</th></tr>
            </thead>
            <tbody>
              {nodes.map(n => (
                <tr key={n.id} className={`click${node === n.id ? ' on' : ''}`} onClick={() => patch({node: n.id})}>
                  <td>{n.title || '(未命名)'}</td>
                  <td>{chapterTitle(n.chapterId)}</td>
                  <td className="mono">{n.prerequisites.length}</td>
                  <td className="mono">{n.rewards.length}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
