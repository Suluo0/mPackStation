/* 逆向链路引擎 · expand：入参 = 终极物品 id + 数量，出参 = 递归到原版采集的完整链路。
   算法（方案 §4）：BFS + 路径栈；出边三级降级（结构化 refs → interpret(payload) → gather 表）；
   多配方默认选一条（status parsed > 产出量大 > item_tag 少 > id 字典序），UI 留「换一条」；
   数量传递 childNeed = ceil(need/outQty*inQty)；环检测入栈即断；非物品输入进 totals.energy。
   标签引用取目录成员代表展开，无成员记断点 tag-unresolvable。 */

import type {CatalogItem, CatalogRecipe, CatalogTag} from '../api/catalog';
import {interpret, type Ref} from './interpret';
import {resolveTick, type Tick} from './ticks';
import rulesJson from './chainRules.json';

const RULES = rulesJson as Record<string, any>;

export type ChainNode = {
  id: string;
  need: number;
  unit: 'item' | 'mB';
  depth: number;
  via?: {recipeId: string; type: string; machine: string; tick: Tick; cycles: number};
  leaf?: 'gather' | 'no-recipe' | 'cycle' | 'unresolved' | 'cap' | 'tag-unresolvable';
  children: ChainNode[];
};

export type Chain = {
  root: ChainNode;
  nodes: number;
  broken: ChainNode[];
  totals: {energy: number; items: Map<string, number>};
};

export type ChainCtx = {
  recipes: CatalogRecipe[];
  items: CatalogItem[];
  tags: CatalogTag[];
};

const MAX_NODES = 600;
const MAX_DEPTH = 24;

type RecipeEdge = {recipe: CatalogRecipe; inputs: Ref[]; outputs: Ref[]};

/** 出边三级降级：结构化 refs（后端已解析 status=parsed）→ interpret(payload) 前端自算。 */
function outgoing(recipe: CatalogRecipe): RecipeEdge | null {
  if (recipe.status === 'parsed') {
    const inputs: Ref[] = [];
    const outputs: Ref[] = [];
    for (const ref of recipe.refs) {
      const r: Ref = {kind: ref.kind === 'item_tag' ? 'item_tag' : 'item', id: ref.id,
        qty: ref.count, unit: 'item'};
      (ref.role === 'output' ? outputs : inputs).push(r);
    }
    if (inputs.length && outputs.length) {
      return {recipe, inputs, outputs};
    }
    return null;
  }
  const interpreted = interpret(recipe.type, recipe.payload);
  if (!interpreted.ok) return null;
  return {recipe, inputs: interpreted.inputs, outputs: interpreted.outputs};
}

function pickRecipe(edges: RecipeEdge[]): RecipeEdge {
  return [...edges].sort((a, b) => {
    const pa = a.recipe.status === 'parsed' ? 1 : 0;
    const pb = b.recipe.status === 'parsed' ? 1 : 0;
    if (pa !== pb) return pb - pa;
    const oa = Math.max(...a.outputs.filter(r => r.unit === 'item').map(r => r.qty), 0);
    const ob = Math.max(...b.outputs.filter(r => r.unit === 'item').map(r => r.qty), 0);
    if (oa !== ob) return ob - oa;
    const ta = a.inputs.filter(r => r.kind === 'item_tag').length;
    const tb = b.inputs.filter(r => r.kind === 'item_tag').length;
    if (ta !== tb) return ta - tb;
    return a.recipe.id.localeCompare(b.recipe.id);
  })[0];
}

export function expandChain(
  ctx: ChainCtx,
  targetId: string,
  qty = 1,
  opts: {maxNodes?: number; maxDepth?: number; speedUpgrades?: number} = {},
): Chain {
  const maxNodes = opts.maxNodes ?? MAX_NODES;
  const maxDepth = opts.maxDepth ?? MAX_DEPTH;

  const totals = {energy: 0, items: new Map<string, number>()};
  const broken: ChainNode[] = [];
  let nodeCount = 0;

  // 按输出物品建索引（只保留能解出边的配方）
  const byOutput = new Map<string, RecipeEdge[]>();
  for (const recipe of ctx.recipes) {
    const edge = outgoing(recipe);
    if (!edge) continue;
    for (const ref of edge.outputs) {
      if (ref.unit !== 'item') continue;
      const bucket = byOutput.get(ref.id);
      if (bucket) bucket.push(edge);
      else byOutput.set(ref.id, [edge]);
    }
  }

  const gatherTable = RULES.gather ?? {};
  const root: ChainNode = {id: targetId, need: qty, unit: 'item', depth: 0, children: []};

  function visit(node: ChainNode, stack: Set<string>): void {
    nodeCount += 1;
    if (nodeCount > maxNodes || node.depth > maxDepth) {
      node.leaf = 'cap';
      broken.push(node);
      return;
    }
    if (stack.has(node.id)) {
      node.leaf = 'cycle';
      broken.push(node);
      return;
    }

    const edges = byOutput.get(node.id) ?? [];
    if (edges.length === 0) {
      if (gatherTable[node.id]) {
        node.leaf = 'gather';
        totals.items.set(node.id, (totals.items.get(node.id) ?? 0) + node.need);
      } else {
        node.leaf = 'no-recipe';
        broken.push(node);
      }
      return;
    }

    const edge = pickRecipe(edges);
    const itemOutputs = edge.outputs.filter(r => r.unit === 'item');
    const outQty = Math.max(...itemOutputs.map(r => r.qty), 1);
    const cycles = Math.ceil(node.need / outQty);
    const tick = resolveTick(edge.recipe.type, (edge.recipe.payload ?? {}) as Record<string, unknown>, RULES);
    node.via = {
      recipeId: edge.recipe.id,
      type: edge.recipe.type,
      machine: edge.recipe.type.split(':').slice(1).join(':') || edge.recipe.type,
      tick, cycles,
    };

    const payload = (edge.recipe.payload ?? {}) as Record<string, unknown>;
    for (const key of ['energy', 'power', 'heat', 'mana']) {
      const v = payload[key];
      if (typeof v === 'number') totals.energy += v * cycles;
    }

    stack.add(node.id);
    for (const input of edge.inputs) {
      const child: ChainNode = {
        id: input.id,
        need: input.unit === 'mB' ? node.need * input.qty / outQty : Math.ceil(node.need / outQty * input.qty),
        unit: input.unit, depth: node.depth + 1, children: [],
      };
      if (input.kind === 'item_tag') {
        const members = ctx.tags.find(tg => tg.id === input.id)?.members ?? [];
        if (members.length === 0) {
          child.leaf = 'tag-unresolvable';
          node.children.push(child);
          broken.push(child);
          continue;
        }
        child.id = members[0];
      }
      node.children.push(child);
      visit(child, stack);
    }
    stack.delete(node.id);
  }

  visit(root, new Set());
  return {root, nodes: nodeCount, broken, totals};
}
