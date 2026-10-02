import {useEffect, useState} from 'react';
import {useParams} from 'react-router-dom';
import {getQuest, type QuestBook} from '../api/content';

/* 素面叶子：编排态的章节 rail（来源栏整体换成它，§6.4）。第三步换成 QuestChapterRail。 */
export function StubQuestChapters() {
  const {id} = useParams();
  const [book, setBook] = useState<QuestBook | null>(null);

  useEffect(() => {
    if (!id) return;
    getQuest(id).then(setBook).catch(() => setBook(null));
  }, [id]);

  const chapters = [...(book?.revision.draft.chapters ?? [])].sort((a, b) => a.position - b.position);
  return (
    <div>
      <div className="rr-line">章节 {chapters.length}</div>
      {chapters.map(c => <div key={c.id} className="rr-line">{c.title || '(未命名章节)'}</div>)}
    </div>
  );
}
