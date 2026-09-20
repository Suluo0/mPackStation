/* M1 任务图编辑：draft.edges 为图权威；保存前把 prerequisites 同步为指向该节点的 fromNodeId 列表。
   P0 FTB 扩展：节点/章节/book 可选字段向后兼容（缺省时前端默认）。
   权威设计：docs/design/quest-book-ftb-experience.md v1.1 */

export type DependencyRequirement = 'all_completed' | 'one_completed' | 'all_started' | 'one_started';

export type QuestTaskDraft = {
  id: string;
  type: string; // item | checkmark | advancement | dimension | kill | location | stat | xp
  itemId?: string;
  count?: number;
  title?: string;
  advancementId?: string;
  dimension?: string;
  entityId?: string;
  x?: number;
  y?: number;
  z?: number;
  statId?: string;
  value?: number;
  xp?: number;
  [key: string]: unknown;
};

export type QuestRewardDraft = {
  kind: string; // item | experience | command | unlock
  item?: string;
  amount?: number;
  experience?: number;
  command?: string;
  unlockId?: string;
  [key: string]: unknown;
};

export type QuestNodeDraft = {
  id: string;
  chapterId: string;
  title: string;
  subtitle?: string;
  description: string;
  icon: string;
  x: number;
  y: number;
  shape?: string; // circle | square | rounded | diamond | hexagon
  size?: number;
  optional?: boolean;
  invisible?: boolean;
  dependencyRequirement?: DependencyRequirement | string;
  minRequiredDependencies?: number;
  prerequisites: unknown[];
  tasks?: QuestTaskDraft[];
  rewards: unknown[];
  modRefs: unknown[];
  position: number;
};

export type QuestChapterDraft = {
  id: string;
  title: string;
  description: string;
  coverColor: string;
  icon?: string;
  position: number;
};

export type QuestBookDraft = {
  title?: string;
  icon?: string;
  progressionMode?: 'default' | 'linear' | 'flexible' | string;
};

export type QuestEdgeDraft = {id: string; fromNodeId: string; toNodeId: string};

export type QuestDraftGraph = {
  book?: QuestBookDraft;
  chapters: QuestChapterDraft[];
  nodes: QuestNodeDraft[];
  edges: QuestEdgeDraft[];
};

export const QUEST_NODE_W = 168;
/** 视觉高度近似值（布局/吸附用；实际 min-height 以 CSS 为准） */
export const QUEST_NODE_H_EST = 72;
/** 拖拽对齐吸附阈值（世界坐标） */
export const QUEST_SNAP_THRESHOLD = 8;
export const QUEST_NODE_H = 92;
export const QUEST_COL_GAP = 200;
export const QUEST_ROW_GAP = 140;
export const QUEST_ORIGIN = 48;
export const QUEST_COLS = 4;

export const QUEST_SHAPES = ['circle', 'square', 'rounded', 'diamond', 'hexagon'] as const;
export const QUEST_TASK_TYPES = [
  {value: 'item', label: '物品'},
  {value: 'checkmark', label: '勾选确认'},
  {value: 'advancement', label: '进度'},
  {value: 'dimension', label: '维度'},
  {value: 'kill', label: '击杀'},
  {value: 'location', label: '坐标'},
  {value: 'stat', label: '统计'},
  {value: 'xp', label: '经验'},
] as const;
export const QUEST_REWARD_KINDS = [
  {value: 'item', label: '物品'},
  {value: 'experience', label: '经验'},
  {value: 'command', label: '命令'},
  {value: 'unlock', label: '解锁'},
] as const;

export function defaultNodeFields(): Pick<
  QuestNodeDraft,
  'subtitle' | 'shape' | 'size' | 'optional' | 'invisible' | 'dependencyRequirement' | 'minRequiredDependencies' | 'tasks' | 'prerequisites' | 'rewards' | 'modRefs' | 'icon' | 'description'
> {
  return {
    subtitle: '',
    description: '',
    icon: '',
    shape: 'circle',
    size: 1,
    optional: false,
    invisible: false,
    dependencyRequirement: 'all_completed',
    minRequiredDependencies: 0,
    tasks: [],
    prerequisites: [],
    rewards: [],
    modRefs: [],
  };
}

/** 旧 draft 补全新字段默认值（向后兼容）。 */
export function normalizeNode(n: Partial<QuestNodeDraft> & {id: string; chapterId: string}): QuestNodeDraft {
  const d = defaultNodeFields();
  return {
    id: n.id,
    chapterId: n.chapterId,
    title: n.title ?? n.id,
    subtitle: n.subtitle ?? d.subtitle,
    description: n.description ?? d.description,
    icon: n.icon ?? d.icon,
    x: Number.isFinite(n.x) ? (n.x as number) : 0,
    y: Number.isFinite(n.y) ? (n.y as number) : 0,
    shape: n.shape ?? d.shape,
    size: n.size ?? d.size,
    optional: n.optional ?? d.optional,
    invisible: n.invisible ?? d.invisible,
    dependencyRequirement: n.dependencyRequirement ?? d.dependencyRequirement,
    minRequiredDependencies: n.minRequiredDependencies ?? d.minRequiredDependencies,
    prerequisites: n.prerequisites ?? d.prerequisites,
    tasks: n.tasks ?? d.tasks,
    rewards: n.rewards ?? d.rewards,
    modRefs: n.modRefs ?? d.modRefs,
    position: n.position ?? 0,
  };
}

export function normalizeChapter(c: Partial<QuestChapterDraft> & {id: string}): QuestChapterDraft {
  return {
    id: c.id,
    title: c.title ?? c.id,
    description: c.description ?? '',
    coverColor: c.coverColor ?? '#C9783B',
    icon: c.icon ?? '',
    position: c.position ?? 0,
  };
}

export function normalizeDraft(raw: Partial<QuestDraftGraph> | null | undefined): QuestDraftGraph {
  return {
    book: {
      title: raw?.book?.title ?? '',
      icon: raw?.book?.icon ?? '',
      progressionMode: raw?.book?.progressionMode ?? 'flexible',
    },
    chapters: (raw?.chapters ?? []).map(normalizeChapter),
    nodes: (raw?.nodes ?? []).map(normalizeNode),
    edges: raw?.edges ?? [],
  };
}

export function gridX(position: number): number {
  return QUEST_ORIGIN + (Math.max(0, position) % QUEST_COLS) * QUEST_COL_GAP;
}

export function gridY(position: number): number {
  return QUEST_ORIGIN + Math.floor(Math.max(0, position) / QUEST_COLS) * QUEST_ROW_GAP;
}

export function isUnpositioned(n: {x: number; y: number}): boolean {
  return !Number.isFinite(n.x) || !Number.isFinite(n.y) || (n.x === 0 && n.y === 0);
}

/** 缺失 x/y 的节点按 chapter 内 position 置入网格；已有坐标的保留。 */
export function ensureNodeCoords(nodes: QuestNodeDraft[]): {nodes: QuestNodeDraft[]; changed: boolean} {
  let changed = false;
  const occupied = new Set(nodes.filter(n => !isUnpositioned(n)).map(n => `${n.x},${n.y}`));
  const next = nodes.map(n => {
    if (!isUnpositioned(n)) return n;
    const chapterPeers = nodes.filter(m => m.chapterId === n.chapterId);
    const slot = Math.max(0, chapterPeers.findIndex(m => m.id === n.id));
    let x = gridX(n.position > 0 ? n.position : slot);
    let y = gridY(n.position > 0 ? n.position : slot);
    // 避开已占用格子（同章多节点默认全 0 时逐个错开）
    while (occupied.has(`${x},${y}`)) {
      x += QUEST_COL_GAP;
      if (x > QUEST_ORIGIN + (QUEST_COLS - 1) * QUEST_COL_GAP + 40) {
        x = QUEST_ORIGIN;
        y += QUEST_ROW_GAP;
      }
    }
    occupied.add(`${x},${y}`);
    changed = true;
    return {...n, x, y};
  });
  return {nodes: next, changed};
}

export function edgeIdFor(fromNodeId: string, toNodeId: string): string {
  return `e_${fromNodeId}__${toNodeId}`;
}

export function findEdge(edges: QuestEdgeDraft[], fromNodeId: string, toNodeId: string): QuestEdgeDraft | undefined {
  return edges.find(e => e.fromNodeId === fromNodeId && e.toNodeId === toNodeId);
}

/** 是否可以从 from → to 添加边（不含环检查）。 */
export function canAddEdge(
  edges: QuestEdgeDraft[],
  fromNodeId: string,
  toNodeId: string,
): {ok: true; edge: QuestEdgeDraft} | {ok: false; reason: string} {
  if (!fromNodeId || !toNodeId) return {ok: false, reason: '前置源或目标节点为空'};
  if (fromNodeId === toNodeId) return {ok: false, reason: '不能把节点连到自己'};
  if (findEdge(edges, fromNodeId, toNodeId)) return {ok: false, reason: '该前置连线已存在'};
  return {ok: true, edge: {id: edgeIdFor(fromNodeId, toNodeId), fromNodeId, toNodeId}};
}

/** Kahn 拓扑：返回环中的节点 id 列表；无环则空数组。 */
export function findCycle(nodes: {id: string}[], edges: QuestEdgeDraft[]): string[] {
  const indeg = new Map<string, number>();
  const adj = new Map<string, string[]>();
  for (const n of nodes) {
    indeg.set(n.id, 0);
    adj.set(n.id, []);
  }
  for (const e of edges) {
    if (!indeg.has(e.fromNodeId) || !indeg.has(e.toNodeId)) continue;
    adj.get(e.fromNodeId)!.push(e.toNodeId);
    indeg.set(e.toNodeId, (indeg.get(e.toNodeId) ?? 0) + 1);
  }
  const q: string[] = [];
  for (const [id, d] of indeg) if (d === 0) q.push(id);
  let seen = 0;
  while (q.length) {
    const id = q.shift()!;
    seen++;
    for (const to of adj.get(id) ?? []) {
      const d = (indeg.get(to) ?? 0) - 1;
      indeg.set(to, d);
      if (d === 0) q.push(to);
    }
  }
  if (seen >= nodes.length) return [];
  // 从仍入度 >0 的点回走，给出一个环提示节点集合
  const stuck = [...indeg.entries()].filter(([, d]) => d > 0).map(([id]) => id);
  return stuck;
}

/** 添加边是否会立刻成环（在临时边上做拓扑）。 */
export function wouldCreateCycle(nodes: {id: string}[], edges: QuestEdgeDraft[], fromNodeId: string, toNodeId: string): boolean {
  const trial = [...edges, {id: 'trial', fromNodeId, toNodeId}];
  return findCycle(nodes, trial).length > 0;
}

export function removeEdge(edges: QuestEdgeDraft[], edgeId: string): QuestEdgeDraft[] {
  return edges.filter(e => e.id !== edgeId);
}

export function removePrerequisite(edges: QuestEdgeDraft[], fromNodeId: string, toNodeId: string): QuestEdgeDraft[] {
  return edges.filter(e => !(e.fromNodeId === fromNodeId && e.toNodeId === toNodeId));
}

/** 入边：指向 nodeId 的前置节点 id 列表。 */
export function prerequisitesOf(edges: QuestEdgeDraft[], nodeId: string): string[] {
  return edges.filter(e => e.toNodeId === nodeId).map(e => e.fromNodeId);
}

/** 保存用：draft.edges 权威 → 各节点 prerequisites 同步。 */
export function draftWithSyncedPrerequisites(draft: QuestDraftGraph): QuestDraftGraph {
  return {
    ...draft,
    nodes: draft.nodes.map(n => ({
      ...n,
      prerequisites: prerequisitesOf(draft.edges, n.id).map(id => id),
    })),
  };
}

export function nodeLabel(nodes: {id: string; title: string}[], id: string): string {
  return nodes.find(n => n.id === id)?.title || id;
}

export function stageSize(nodes: {x: number; y: number}[]): {width: number; height: number} {
  let maxX = 0;
  let maxY = 0;
  for (const n of nodes) {
    maxX = Math.max(maxX, n.x + QUEST_NODE_W + QUEST_ORIGIN);
    maxY = Math.max(maxY, n.y + QUEST_NODE_H + QUEST_ORIGIN);
  }
  return {width: Math.max(720, maxX), height: Math.max(480, maxY)};
}

/** 父→子连线路径（节点右缘中点 → 子节点左缘中点）。 */
export function edgePath(from: {x: number; y: number}, to: {x: number; y: number}): string {
  const x1 = from.x + QUEST_NODE_W;
  const y1 = from.y + QUEST_NODE_H / 2;
  const x2 = to.x;
  const y2 = to.y + QUEST_NODE_H / 2;
  const dx = Math.max(40, Math.abs(x2 - x1) * 0.45);
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
}

/** 创建带完整默认字段的新任务节点。 */
export function createQuestNode(input: {
  id: string;
  chapterId: string;
  title: string;
  position: number;
  x?: number;
  y?: number;
  taskType?: string;
}): QuestNodeDraft {
  const d = defaultNodeFields();
  const taskType = input.taskType ?? 'item';
  const tasks: QuestTaskDraft[] =
    taskType === 'checkmark'
      ? [{id: `${input.id}-t1`, type: 'checkmark', title: input.title}]
      : [{id: `${input.id}-t1`, type: taskType, itemId: '', count: 1}];
  return {
    ...d,
    id: input.id,
    chapterId: input.chapterId,
    title: input.title,
    x: input.x ?? gridX(input.position),
    y: input.y ?? gridY(input.position),
    position: input.position,
    tasks,
  };
}

/** 单轴吸附：将 origin 对齐到 others 产生的边/中心线（阈值内）。 */
export function snapAxis(
  origin: number,
  size: number,
  otherOrigins: number[],
  threshold = QUEST_SNAP_THRESHOLD,
): {value: number; guides: number[]} {
  const offsets = [0, size / 2, size];
  const lines: number[] = [];
  for (const o of otherOrigins) {
    for (const off of [0, size / 2, size]) lines.push(o + off);
  }
  let bestDelta = 0;
  let bestDist = Number.POSITIVE_INFINITY;
  let bestGuide = 0;
  for (const line of lines) {
    for (const off of offsets) {
      const delta = line - (origin + off);
      const dist = Math.abs(delta);
      if (dist <= threshold && dist < bestDist) {
        bestDist = dist;
        bestDelta = delta;
        bestGuide = line;
      }
    }
  }
  if (!Number.isFinite(bestDist)) return {value: origin, guides: []};
  return {value: origin + bestDelta, guides: [bestGuide]};
}

/** 二维吸附：同章其它节点的 x/y 边与中心。 */
export function snapNodePosition(
  x: number,
  y: number,
  others: {x: number; y: number}[],
  opts?: {width?: number; height?: number; threshold?: number},
): {x: number; y: number; guidesV: number[]; guidesH: number[]} {
  const w = opts?.width ?? QUEST_NODE_W;
  const h = opts?.height ?? QUEST_NODE_H_EST;
  const th = opts?.threshold ?? QUEST_SNAP_THRESHOLD;
  const sx = snapAxis(x, w, others.map(o => o.x), th);
  const sy = snapAxis(y, h, others.map(o => o.y), th);
  return {x: sx.value, y: sy.value, guidesV: sx.guides, guidesH: sy.guides};
}
