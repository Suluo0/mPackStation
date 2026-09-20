/* 任务书连线数据流断言（设计 §5b）。
   运行：cd apps/web && node --experimental-strip-types scripts/test-quest-canvas-edges.ts
   覆盖：语义 A→B=B 依赖 A；edges 唯一图权威；保存前派生 prerequisites；
   C1/C2/C3 共用校验（自环/重复/环）；跨章允许。 */
import assert from 'node:assert/strict';
import {
  canAddEdge,
  draftWithSyncedPrerequisites,
  edgeIdFor,
  edgePath,
  findCycle,
  prerequisitesOf,
  removeEdge,
  wouldCreateCycle,
  QUEST_NODE_H,
  QUEST_NODE_W,
  type QuestDraftGraph,
  type QuestNodeDraft,
} from '../src/features/quest/questGraph.ts';

function node(id: string, chapterId: string, x: number, y: number): QuestNodeDraft {
  return {
    id,
    chapterId,
    title: id,
    description: '',
    icon: '',
    x,
    y,
    prerequisites: [],
    rewards: [],
    modRefs: [],
    position: 0,
  };
}

let passed = 0;
function ok(cond: boolean, msg: string) {
  if (!cond) {
    console.error('FAIL:', msg);
    process.exit(1);
  }
  passed += 1;
  console.log('ok -', msg);
}

const nodes = [
  node('a', 'ch1', 0, 0),
  node('b', 'ch1', 300, 0),
  node('c', 'ch2', 300, 200),
];
const draft: QuestDraftGraph = {
  book: {title: 't'},
  chapters: [
    {id: 'ch1', title: 'C1', description: '', coverColor: '', position: 0},
    {id: 'ch2', title: 'C2', description: '', coverColor: '', position: 1},
  ],
  nodes,
  edges: [],
};

/* C 校验矩阵（C1/C2/C3 共用 canAddEdge） */
ok(canAddEdge([], 'a', 'a').ok === false, '自环拒绝');
ok(canAddEdge([{id: edgeIdFor('a', 'b'), fromNodeId: 'a', toNodeId: 'b'}], 'a', 'b').ok === false, '重复边拒绝');
const okEdge = canAddEdge([], 'a', 'b');
ok(okEdge.ok === true, 'A→B 允许（B 依赖 A）');
ok(okEdge.ok && okEdge.edge.fromNodeId === 'a' && okEdge.edge.toNodeId === 'b', '边方向 from=A to=B');
ok(canAddEdge([], 'a', 'c').ok === true, '跨章边允许');

/* 环：创建时 warning 语义（wouldCreateCycle），保存时硬拦（findCycle） */
ok(wouldCreateCycle(nodes, [{id: 'eab', fromNodeId: 'a', toNodeId: 'b'}], 'b', 'a') === true, 'B→A 会成环');
ok(findCycle(nodes, [
  {id: 'eab', fromNodeId: 'a', toNodeId: 'b'},
  {id: 'eba', fromNodeId: 'b', toNodeId: 'a'},
]).length > 0, '存在环时 findCycle 非空 → 保存拦截');

/* 数据流：UI mutate edges → 保存前 derive prerequisites */
const withEdges: QuestDraftGraph = {
  ...draft,
  edges: [
    {id: edgeIdFor('a', 'b'), fromNodeId: 'a', toNodeId: 'b'},
    {id: edgeIdFor('a', 'c'), fromNodeId: 'a', toNodeId: 'c'},
  ],
};
const synced = draftWithSyncedPrerequisites(withEdges);
const b = synced.nodes.find(n => n.id === 'b')!;
const c = synced.nodes.find(n => n.id === 'c')!;
const a = synced.nodes.find(n => n.id === 'a')!;
ok(JSON.stringify(b.prerequisites) === JSON.stringify(['a']), 'B.prerequisites = [A]（入边派生）');
ok(JSON.stringify(c.prerequisites) === JSON.stringify(['a']), '跨章 C.prerequisites 含 A');
ok(a.prerequisites.length === 0, 'A 无入边则 prerequisites 空');
ok(JSON.stringify(prerequisitesOf(withEdges.edges, 'b')) === JSON.stringify(['a']), 'prerequisitesOf 与 edges 一致');

/* 删边后 Inspector 依赖列表同步消失（同一 draft.edges） */
const afterRemove = removeEdge(withEdges.edges, edgeIdFor('a', 'b'));
ok(prerequisitesOf(afterRemove, 'b').length === 0, '删 A→B 后 B 入边为空');
const resynced = draftWithSyncedPrerequisites({...withEdges, edges: afterRemove});
ok(resynced.nodes.find(n => n.id === 'b')!.prerequisites.length === 0, '删除后重新派生 prerequisites 同步消失');

/* 路径：世界坐标右缘→左缘（随节点 x/y 更新，拖节点自动重算） */
const p1 = edgePath({x: 0, y: 0}, {x: 400, y: 100});
const p2 = edgePath({x: 0, y: 0}, {x: 500, y: 100});
ok(p1 !== p2, '拖目标节点后 edgePath 变化');
ok(p1.includes(String(QUEST_NODE_W)), '起点用节点右缘 x+W');
ok(p1.includes(String(QUEST_NODE_H / 2)), '起点用节点垂直中点');

console.log('');
console.log(`quest-canvas-edges: ${passed} passed`);
