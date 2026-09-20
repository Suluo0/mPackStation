/* M1 quest graph pure-function assertions. Run:
   node --experimental-strip-types apps/web/scripts/questGraph.check.ts
*/
import {
  canAddEdge,
  draftWithSyncedPrerequisites,
  ensureNodeCoords,
  findCycle,
  gridX,
  gridY,
  prerequisitesOf,
  wouldCreateCycle,
  type QuestDraftGraph,
  type QuestNodeDraft,
} from '../src/features/quest/questGraph.ts';

function assert(cond: boolean, msg: string) {
  if (!cond) {
    console.error('FAIL:', msg);
    process.exit(1);
  }
  console.log('ok -', msg);
}

const node = (id: string, chapterId: string, position: number, x = 0, y = 0): QuestNodeDraft => ({
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
  position,
});

// ensureNodeCoords: 未定位节点写入网格
{
  const {nodes, changed} = ensureNodeCoords([node('a', 'ch1', 0), node('b', 'ch1', 1)]);
  assert(changed, 'ensureNodeCoords marks changed');
  assert(nodes[0].x === gridX(0) && nodes[0].y === gridY(0), 'first node on grid origin slot');
  assert(nodes[1].x === gridX(1) && nodes[1].y === gridY(1), 'second node on next grid slot');
  assert(nodes[0].x !== 0 || nodes[0].y !== 0, 'default coords are not left at 0,0');
}

// canAddEdge
{
  const edges = [{id: 'e_a__b', fromNodeId: 'a', toNodeId: 'b'}];
  assert(canAddEdge(edges, 'a', 'a').ok === false, 'reject self edge');
  assert(canAddEdge(edges, 'a', 'b').ok === false, 'reject duplicate edge');
  const ok = canAddEdge(edges, 'b', 'c');
  assert(ok.ok === true, 'accept new edge b→c');
  assert(ok.ok && ok.edge.fromNodeId === 'b' && ok.edge.toNodeId === 'c', 'edge payload correct');
}

// cycle detection
{
  const nodes = [node('a', 'ch1', 0, 10, 10), node('b', 'ch1', 1, 200, 10), node('c', 'ch1', 2, 400, 10)];
  assert(findCycle(nodes, []).length === 0, 'empty graph has no cycle');
  assert(findCycle(nodes, [
    {id: 'e1', fromNodeId: 'a', toNodeId: 'b'},
    {id: 'e2', fromNodeId: 'b', toNodeId: 'c'},
  ]).length === 0, 'chain has no cycle');
  assert(wouldCreateCycle(nodes, [
    {id: 'e1', fromNodeId: 'a', toNodeId: 'b'},
    {id: 'e2', fromNodeId: 'b', toNodeId: 'c'},
  ], 'c', 'a') === true, 'c→a would cycle');
  assert(wouldCreateCycle(nodes, [
    {id: 'e1', fromNodeId: 'a', toNodeId: 'b'},
  ], 'b', 'c') === false, 'b→c does not cycle');
}

// prerequisites sync from edges
{
  const draft: QuestDraftGraph = {
    chapters: [{id: 'ch1', title: 'C', description: '', coverColor: '', position: 0}],
    nodes: [node('a', 'ch1', 0, 48, 48), node('b', 'ch1', 1, 248, 48)],
    edges: [{id: 'e_a__b', fromNodeId: 'a', toNodeId: 'b'}],
  };
  const synced = draftWithSyncedPrerequisites(draft);
  assert(Array.isArray(synced.nodes[1].prerequisites), 'prerequisites is array');
  assert(JSON.stringify(synced.nodes[1].prerequisites) === JSON.stringify(['a']), 'B.prerequisites = [A]');
  assert(JSON.stringify(synced.nodes[0].prerequisites) === JSON.stringify([]), 'A.prerequisites empty');
  assert(prerequisitesOf(draft.edges, 'b')[0] === 'a', 'prerequisitesOf lists incoming');
  // edges remain authoritative and unchanged by sync
  assert(synced.edges.length === 1 && synced.edges[0].fromNodeId === 'a', 'edges preserved through sync');
}

console.log('\nquest-graph M1 assertions: ALL PASS');
