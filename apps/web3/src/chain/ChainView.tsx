import {useMemo} from 'react';
import {useCatalog} from '../app/CatalogContext';
import {ItemIcon} from '../app/ItemIcon';
import {hoverProps} from '../app/hoverTarget';
import type {Chain, ChainNode} from '../chain/expand';

/* 维度一渲染（方案 §6）：每层一个 .graph-col，节点 = 数量，箭头 = 机器·时长，断点成列。
   选择器沿用 editor.css 的 .graph-col > .g-head（作用域在 .graph-col 内，必须用这套类名才带样式）。 */

const TICKS_PER_SEC = 20;

function tickLabel(node: ChainNode): string {
  if (!node.via) return '';
  const t = node.via.tick;
  if (t.mode === 'unknown') return `${node.via.machine} · 节拍未核`;
  if (t.mode === 'rate' && t.rate) {
    return `${node.via.machine} · ${t.rate.inQty}${t.rate.inUnit}/t → ${t.rate.outQty}${t.rate.outUnit}/t`;
  }
  const parallel = t.parallel ?? 1;
  const ticks = node.via.cycles * (t.ticksPerCycle ?? 0) / parallel;
  return `${node.via.machine} · ${t.ticksPerCycle} t/次 × ${node.via.cycles} = ${(ticks / TICKS_PER_SEC).toFixed(1)}s`;
}

function leafLabel(node: ChainNode): string {
  switch (node.leaf) {
    case 'gather': return '原版采集';
    case 'no-recipe': return '目录无配方';
    case 'cycle': return '配方闭环';
    case 'unresolved': return '配方无法解析';
    case 'tag-unresolvable': return '标签无成员';
    case 'cap': return '超出深度/节点上限';
    default: return '';
  }
}

export function ChainView({chain}: {chain: Chain}) {
  // 按深度分层渲染。每个节点带一条遍历路径当 React key：同一层里同一个物品
  // 可能出现在多条子配方下（id+depth 会撞），只有路径是唯一的。
  const layers = useMemo(() => {
    const byDepth = new Map<number, {node: ChainNode; key: string}[]>();
    const walk = (node: ChainNode, path: string) => {
      if (!byDepth.has(node.depth)) byDepth.set(node.depth, []);
      byDepth.get(node.depth)!.push({node, key: path});
      node.children.forEach((child, i) => walk(child, `${path}.${i}`));
    };
    walk(chain.root, 'r');
    return [...byDepth.entries()].sort((a, b) => a[0] - b[0]);
  }, [chain]);

  const {itemById, packId} = useCatalog();
  const nameOf = (id: string) => itemById.get(id)?.displayName ?? id;

  const brokenByDepth = useMemo(() => {
    const m = new Map<number, ChainNode[]>();
    for (const b of chain.broken) {
      if (!m.has(b.depth)) m.set(b.depth, []);
      m.get(b.depth)!.push(b);
    }
    return m;
  }, [chain.broken]);

  return (
    <div className="chain-root">
      <div className="chain-summary">
        共 {chain.nodes} 个节点 · 原料合计 {chain.totals.items.size} 种
        {chain.totals.energy > 0 && ` · 需能量 ${chain.totals.energy.toLocaleString()} J`}
        · 断点 {chain.broken.length} 处
      </div>
      <div className="chain-cols">
        {layers.map(([depth, entries]) => (
          <div key={depth} className="graph-col">
            <div className="g-head">深度 {depth}{depth === 0 && ' · 终极物品'}</div>
            <div className="g-body" {...hoverProps()}>
              {entries.map(({node, key: nodeKey}) => {
                const it = itemById.get(node.id);
                return (
                <div key={nodeKey}>
                  <div className="p-row" data-hover-item={node.id}
                    title={`${nameOf(node.id)} × ${node.need.toFixed(1)}${node.via ? ` · ${node.via.recipeId}` : ''}`}>
                    {it && <ItemIcon packId={packId} item={it} size={18}/>}
                    <span className="grow">{nameOf(node.id)}</span>
                    <span className="mono">×{node.need % 1 === 0 ? node.need : node.need.toFixed(1)}</span>
                  </div>
                  {node.via && node.depth > 0 && (
                    <div style={{fontSize: 11, color: 'var(--mc-muted)', padding: '0 4px 2px'}}>{tickLabel(node)}</div>
                  )}
                </div>
                );
              })}
              {(brokenByDepth.get(depth) ?? []).map((node, i) => {
                const it = itemById.get(node.id);
                return (
                <div key={`b-${depth}-${i}`} className="p-row" style={{color: 'var(--mc-muted)'}}
                  data-hover-item={node.id}>
                  {it && <ItemIcon packId={packId} item={it} size={18}/>}
                  <span className="grow">{nameOf(node.id)}</span>
                  <span className="sub">{leafLabel(node)}</span>
                </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
