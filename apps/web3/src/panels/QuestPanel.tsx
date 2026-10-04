import {useCallback, useEffect, useMemo, useState} from 'react';
import {getQuest, saveQuestDraft, type QuestBook} from '../api/content';
import {ApiError} from '../api/http';
import {useUrlPatch, useUrlState} from '../app/url';
import {Icon} from '../ui/Icon';
import {
  emptyQuestDraft, makeChapter, moveChapter, removeChapter, renameChapter, type QuestDraft,
} from '../editor/questDraft';
import {useContextMenu, type MenuItem} from '../ui/ContextMenu';

/* 章节面板（左栏 tool=quest）：任务书的章节入口 + 管理。
   此前这里是一个纯只读列表 —— 零按钮、无右键，空态还写着「画布（3D 落地）里可以建」，
   而那个画布并不存在，用户看到的就是「什么都干不了」。
   真正能编排的编辑器在编排态（mode=quest），但它没有任何可见入口，
   所以这个面板现在承担两件事：一是把入口给出来，二是就地做章节级的增删改排序。

   每次操作立即落库（不像编排态那样攒一份草稿再显式保存）：章节级操作足够小，
   攒草稿只会让「左栏」和「编排态」两份状态互相覆盖。If-Match 带当前 revision，
   别处先保存过就会失败并提示，而不是静默覆盖。 */
export function QuestPanel() {
  const {packId, chap} = useUrlState();
  const patch = useUrlPatch();
  const [book, setBook] = useState<QuestBook | null>(null);
  const [draft, setDraft] = useState<QuestDraft>(emptyQuestDraft);
  const [hasBook, setHasBook] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const menu = useContextMenu();
  const chapterItems = (id: string): MenuItem[] => [
    {label: '在编排态打开', icon: 'quest', action: () => open(id)},
    {separator: true},
    {
      label: '重命名', icon: 'wrench',
      action: () => {
        const c = chapters.find(x => x.id === id);
        setRenameValue(c?.title ?? '');
        setRenaming(id);
      },
    },
    {label: '上移', icon: 'caretUp', action: () => save({...draft, chapters: moveChapter(draft.chapters, id, -1)})},
    {label: '下移', icon: 'caretDown', action: () => save({...draft, chapters: moveChapter(draft.chapters, id, 1)})},
    {separator: true},
    {label: '删除章节（连同它的节点）', icon: 'trash', danger: true, action: () => save(removeChapter(draft, id))},
  ];
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');

  const load = useCallback(() => {
    if (!packId) return;
    getQuest(packId)
      .then(b => {
        setBook(b); setHasBook(true);
        const d = b.revision.draft;
        setDraft({chapters: d.chapters, nodes: d.nodes, edges: d.edges});
        setError(null);
      })
      .catch(e => {
        /* 「还没有任务书」是 404，属于正常空态而不是错误 —— 以前把它当错误显示，
           结果空态只剩一行红字，连「创建」按钮都不给。 */
        setHasBook(false);
        if (e instanceof ApiError && e.status === 404) { setError(null); return; }
        setError(e instanceof Error ? e.message : String(e));
      });
  }, [packId]);
  useEffect(load, [load]);

  /* 直接写库。next 是改完的整份草稿，book 元信息原样带回去。 */
  const commit = async (next: QuestDraft, rev: number) => {
    if (!packId) return;
    setBusy(true); setError(null);
    try {
      await saveQuestDraft(packId, rev, {...next, book: book?.revision.draft.book ?? {title: '任务书'}});
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  /* 建书：If-Match 传 0，后端按「还没有任何修订」判。 */
  const createBook = async () => {
    if (!packId) return;
    setBusy(true); setError(null);
    try {
      await saveQuestDraft(packId, 0, {...emptyQuestDraft(), book: {title: '任务书'}});
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const chapters = useMemo(
    () => [...draft.chapters].sort((a, b) => a.position - b.position),
    [draft.chapters]);

  const save = (next: QuestDraft) => {
    if (!book) return;
    void commit(next, book.revision.revision);
  };

  const commitRename = (id: string) => {
    const title = renameValue.trim() || '未命名章节';
    setRenaming(null);
    save({...draft, chapters: renameChapter(draft.chapters, id, title)});
  };

  const addChapter = () => {
    const next = makeChapter(chapters.length);
    save({...draft, chapters: [...chapters, next]});
    patch({mode: 'quest', chap: next.id});
  };

  const open = (id: string) => patch({mode: 'quest', chap: id});

  if (!packId) return null;

  return (
    <>
      <div className="tp-top">
        <div className="tp-head">
          <span>章节 <span className="count">{hasBook ? chapters.length : 0}</span></span>
          <span style={{flex: 1}}/>
          <button type="button" className="tp-icon-btn" title="在编排态打开任务书（也可以按 Q）"
            disabled={!hasBook} onClick={() => patch({mode: 'quest'})}><Icon name="quest" size={14}/></button>
          <button type="button" className="tp-icon-btn" title="新建章节"
            disabled={!hasBook || busy} onClick={addChapter}><Icon name="plus" size={14}/></button>
        </div>
      </div>

      <div className="tp-body">
        {error && <div className="p-empty">{error}</div>}

        {!hasBook && !error && (
          <div className="p-empty">
            这个包还没有任务书。
            <div style={{marginTop: 8}}>
              <button type="button" className="p-btn primary" disabled={busy}
                onClick={() => void createBook()}>创建任务书</button>
            </div>
            <div style={{marginTop: 8, lineHeight: 1.6}}>
              建好后就能在这里加章节，排好顺序后再进编排态加节点、连前置。
            </div>
          </div>
        )}

        {hasBook && chapters.map(c => {
          const n = draft.nodes.filter(x => x.chapterId === c.id).length;
          const active = chap === c.id;
          return (
            <div key={c.id} className={`p-row click${active ? ' on' : ''}`}
              title={`${c.title || '(未命名章节)'}\n单击在编排态打开 · 右键改名字/排序/删除`}
              onClick={() => { if (renaming !== c.id) open(c.id); }}
              onContextMenu={e => menu.open(e, chapterItems(c.id))}>
              <span className="grow" style={{overflow: 'hidden', textOverflow: 'ellipsis'}}>
                {renaming === c.id
                  ? <input className="p-input" autoFocus value={renameValue}
                      style={{width: '100%'}}
                      onClick={e => e.stopPropagation()}
                      onChange={e => setRenameValue(e.target.value)}
                      onKeyDown={e => {
                        if (e.key === 'Enter') commitRename(c.id);
                        if (e.key === 'Escape') setRenaming(null);
                      }}
                      onBlur={() => commitRename(c.id)}/>
                  : (c.title || '(未命名章节)')}
              </span>
              <span className="sub">{n}</span>
            </div>
          );
        })}

        {hasBook && chapters.length === 0 && (
          <div className="p-empty">还没有章节。点上面的「+」新建一个。</div>
        )}
      </div>

      {menu.menu}
    </>
  );
}
