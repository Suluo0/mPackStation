import type {QuestChapter, QuestEdge, QuestNode} from '../api/content';

/* 任务书草稿的共享纯工具。编排态（editor/ModeQuest）和左栏章节面板（panels/QuestPanel）
   都要造空草稿、生成临时 id、调章节顺序、删章连带清节点 —— 放这里，免得两边各推一份
   然后在「删章要不要清节点」这种地方慢慢长歪。
   刻意与组件分文件：Fast Refresh 要求组件模块只导出组件。 */

export type QuestDraft = {
  chapters: QuestChapter[];
  nodes: QuestNode[];
  edges: QuestEdge[];
};

/** 本地临时 id。后端只要求同一次修订内唯一，重名由 UNIQUE(revision_id, id) 兜底。 */
export const rid = (p: string) => `${p}-${Math.random().toString(36).slice(2, 8)}`;

export function makeChapter(index: number, title?: string): QuestChapter {
  return {id: rid('ch'), title: title ?? `第${index + 1}章`, description: '', coverColor: '', position: index};
}

/** 空草稿带一章：一章都没有的任务书在后端过不了校验，不如开局就给个落点。 */
export function emptyQuestDraft(): QuestDraft {
  return {chapters: [makeChapter(0, '第一章')], nodes: [], edges: []};
}

/** position 归一到与数组下标一致。后端有 UNIQUE(revision_id, position)，
    重排后不重新编号会撞约束。 */
export function reindex(chapters: QuestChapter[]): QuestChapter[] {
  return chapters.map((c, i) => (c.position === i ? c : {...c, position: i}));
}

/** 把 chapterId 那一章移动 delta 步（-1 上移 / +1 下移），越界原样返回。 */
export function moveChapter(chapters: QuestChapter[], chapterId: string, delta: number): QuestChapter[] {
  const sorted = [...chapters].sort((a, b) => a.position - b.position);
  const i = sorted.findIndex(c => c.id === chapterId);
  const j = i + delta;
  if (i < 0 || j < 0 || j >= sorted.length) return chapters;
  const tmp = sorted[i];
  sorted[i] = sorted[j];
  sorted[j] = tmp;
  return reindex(sorted);
}

/** 改名（顺带把标题写回 title，空标题由调用方兜底）。 */
export function renameChapter(chapters: QuestChapter[], chapterId: string, title: string): QuestChapter[] {
  return chapters.map(c => (c.id === chapterId ? {...c, title} : c));
}

/** 删章必须连带删掉它名下的节点，以及连到这些节点的边 —— quest_nodes 对
    (revision_id, chapter_id) 有外键，留着会让整次保存被判 422，而不是只丢这一章。 */
export function removeChapter(draft: QuestDraft, chapterId: string): QuestDraft {
  const gone = new Set(draft.nodes.filter(n => n.chapterId === chapterId).map(n => n.id));
  return {
    chapters: reindex(draft.chapters.filter(c => c.id !== chapterId)),
    nodes: draft.nodes.filter(n => n.chapterId !== chapterId),
    edges: draft.edges.filter(e => !gone.has(e.fromNodeId) && !gone.has(e.toNodeId)),
  };
}

/** 画布上节点的边框色。优先用章节自己设的 coverColor，没设就按章节顺序取色 ——
    画布上「这一簇属于哪一章」全靠颜色区分，不能全一个色。 */
const CHAPTER_COLORS = [
  '#c9783b', '#3b82f6', '#16a34a', '#a855f7',
  '#0ea5e9', '#ea8600', '#e5484d', '#64748b',
];

export function chapterColor(chapters: QuestChapter[], chapterId: string): string {
  const sorted = [...chapters].sort((a, b) => a.position - b.position);
  const i = sorted.findIndex(c => c.id === chapterId);
  const own = sorted[i]?.coverColor;
  if (own) return own;
  return CHAPTER_COLORS[(i < 0 ? sorted.length : i) % CHAPTER_COLORS.length];
}
