/**
 * 进度树布局/Tab/requirements 纯函数单测。
 * 设计权威：docs/design/advancement-vanilla-experience.md v1.1
 * 运行：cd apps/web && node --experimental-strip-types scripts/test-adv-layout.ts
 */
import assert from 'node:assert/strict';
import {
  DRAG_CLICK_PX,
  FIT_PADDING,
  K_DEFAULT,
  K_FIT_MIN,
  K_MIN,
  NODE_SIZE,
  PAN_MARGIN,
  X_STEP,
  Y_STEP,
  classifyRequirements,
  clampPan,
  computePanBounds,
  describeRequirements,
  edgePath,
  fitTransform,
  fixtureAdvWideFlat,
  formatRewardsSummary,
  groupAdvancementTrees,
  interactiveZoom,
  isDragGesture,
  isRecipesAdvancement,
  itemToAdvNode,
  itemsToAdvTrees,
  layoutAdvancement,
  parentChain,
  parseAdvPayload,
  parseRequirements,
  parseRewards,
  resetTransform,
  sanitizeParents,
  screenToWorld,
  zoomAtAnchor,
  type AdvNode,
} from '../src/features/content/advLayout.ts';

let passed = 0;
const failures: string[] = [];

function test(name: string, fn: () => void): void {
  try {
    fn();
    passed += 1;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    failures.push(name);
    console.error(`  ✗ ${name}`);
    console.error(`    ${err instanceof Error ? err.message : String(err)}`);
  }
}

console.log('advLayout tests · vanilla-experience v1.1');

/* ---------- layout / sanitize / transform（既有逻辑保持） ---------- */

test('sanitize: missing parent → root, not dropped', () => {
  const nodes: AdvNode[] = [
    { id: 'a', parentId: 'missing', title: 'A', hasDisplay: true },
    { id: 'b', parentId: 'a', title: 'B', hasDisplay: false },
  ];
  const layout = layoutAdvancement(nodes);
  assert.equal(layout.nodes.length, 2);
  assert.equal(layout.missingParentCount, 1);
  assert.equal(layout.parentOf.get('a'), null);
  assert.equal(layout.parentOf.get('b'), 'a');
});

test('sanitize: cycle broken, entry node promoted to root', () => {
  const nodes: AdvNode[] = [
    { id: 'a', parentId: 'c', title: 'A', hasDisplay: false },
    { id: 'b', parentId: 'a', title: 'B', hasDisplay: false },
    { id: 'c', parentId: 'b', title: 'C', hasDisplay: false },
  ];
  const { parentOf, cycleBreaks } = sanitizeParents(nodes);
  assert.ok(cycleBreaks >= 1, `cycleBreaks=${cycleBreaks}`);
  const roots = [...parentOf.entries()].filter(([, p]) => p == null);
  assert.ok(roots.length >= 1);
  for (const id of parentOf.keys()) {
    const seen = new Set<string>();
    let cur: string | null = id;
    while (cur) {
      assert.ok(!seen.has(cur), `cycle remains at ${cur}`);
      seen.add(cur);
      cur = parentOf.get(cur) ?? null;
    }
  }
});

test('layout: depth x = depth * 152, leaf y spacing 92', () => {
  const nodes: AdvNode[] = [
    { id: 'r', parentId: null, title: 'R', hasDisplay: true },
    { id: 'c1', parentId: 'r', title: 'C1', hasDisplay: false },
    { id: 'c2', parentId: 'r', title: 'C2', hasDisplay: false },
    { id: 'g', parentId: 'c1', title: 'G', hasDisplay: false },
  ];
  const layout = layoutAdvancement(nodes);
  const byId = layout.nodeById;
  assert.equal(byId.get('r')!.depth, 0);
  assert.equal(byId.get('r')!.x, 0);
  assert.equal(byId.get('c1')!.x, X_STEP);
  assert.equal(byId.get('g')!.x, 2 * X_STEP);
  const c2 = byId.get('c2')!;
  const g = byId.get('g')!;
  assert.ok(Math.abs(c2.y - g.y) === Y_STEP || Math.abs(g.y - c2.y) === Y_STEP);
  const r = byId.get('r')!;
  const c1 = byId.get('c1')!;
  assert.ok(Math.abs(r.y - (c1.y + c2.y) / 2) < 1e-9);
});

test('layout: forest — two roots separated by ≥92', () => {
  const nodes: AdvNode[] = [
    { id: 'r1', parentId: null, title: 'R1', hasDisplay: true },
    { id: 'r1a', parentId: 'r1', title: 'R1a', hasDisplay: false },
    { id: 'r2', parentId: null, title: 'R2', hasDisplay: true },
    { id: 'r2a', parentId: 'r2', title: 'R2a', hasDisplay: false },
  ];
  const layout = layoutAdvancement(nodes);
  const r1a = layout.nodeById.get('r1a')!;
  const r2 = layout.nodeById.get('r2')!;
  assert.ok(r2.y - r1a.y >= Y_STEP - 1e-9, `gap=${r2.y - r1a.y}`);
  assert.equal(layout.nodes.filter(n => n.depth === 0).length, 2);
});

test('layout: wide-flat fixture ≥220 nodes, edges = n-1', () => {
  const nodes = fixtureAdvWideFlat(220);
  assert.ok(nodes.length >= 220);
  const layout = layoutAdvancement(nodes);
  assert.equal(layout.nodes.length, 220);
  assert.equal(layout.edges.length, 219);
  assert.equal(layout.cycleBreaks, 0);
});

test('k_fit formula: allows k < 0.35, floor 0.08', () => {
  const tall = { minX: 0, minY: 0, maxX: 200, maxY: 20000, width: 200, height: 20000 };
  const t1 = fitTransform(tall, 800, 500, FIT_PADDING);
  assert.ok(t1.k < K_MIN, `expected k_fit < ${K_MIN}, got ${t1.k}`);
  assert.ok(t1.k >= K_FIT_MIN - 1e-12);
  assert.ok(Math.abs(t1.k - K_FIT_MIN) < 1e-9, `clamped to K_FIT_MIN, got ${t1.k}`);

  const layout = layoutAdvancement(fixtureAdvWideFlat(12));
  const view = fitTransform(layout.bounds, 1000, 700, FIT_PADDING);
  assert.ok(view.k >= K_FIT_MIN);
  assert.ok(view.k <= 2.5);
  if (view.k > K_FIT_MIN + 1e-9) {
    const tl = screenToWorld(view, FIT_PADDING, FIT_PADDING);
    const br = screenToWorld(view, 1000 - FIT_PADDING, 700 - FIT_PADDING);
    assert.ok(tl.x <= layout.bounds.minX + 1e-6);
    assert.ok(tl.y <= layout.bounds.minY + 1e-6);
    assert.ok(br.x >= layout.bounds.maxX - 1e-6);
    assert.ok(br.y >= layout.bounds.maxY - 1e-6);
  }

  const wide = layoutAdvancement(fixtureAdvWideFlat(220));
  const wideView = fitTransform(wide.bounds, 900, 600, FIT_PADDING);
  assert.ok(wideView.k >= K_FIT_MIN);
});

test('anchor zoom: world point under screen point is invariant', () => {
  const view = { k: 1, tx: 100, ty: 50 };
  const sx = 320;
  const sy = 200;
  const before = screenToWorld(view, sx, sy);
  const next = zoomAtAnchor(view, 0.5, sx, sy);
  const after = screenToWorld(next, sx, sy);
  assert.ok(Math.abs(before.x - after.x) < 1e-9);
  assert.ok(Math.abs(before.y - after.y) < 1e-9);
});

test('interactive zoom clamps to [0.35, 2.5]', () => {
  const view = { k: K_MIN, tx: 0, ty: 0 };
  const out = interactiveZoom(view, 0.1, 0, 0);
  assert.equal(out.k, K_MIN);
  const out2 = interactiveZoom({ k: K_DEFAULT, tx: 0, ty: 0 }, 100, 0, 0);
  assert.equal(out2.k, 2.5);
});

test('reset returns k=1 focusing roots', () => {
  const layout = layoutAdvancement(fixtureAdvWideFlat(8));
  const t = resetTransform(layout.bounds, 800, 600);
  assert.equal(t.k, K_DEFAULT);
});

test('parent chain walks to root', () => {
  const nodes: AdvNode[] = [
    { id: 'r', parentId: null, title: 'R', hasDisplay: true },
    { id: 'a', parentId: 'r', title: 'A', hasDisplay: false },
    { id: 'b', parentId: 'a', title: 'B', hasDisplay: false },
  ];
  const layout = layoutAdvancement(nodes);
  const chain = parentChain('b', layout.parentOf);
  assert.deepEqual(chain, ['a', 'r']);
});

test('edgePath produces cubic bezier', () => {
  const d = edgePath({ x: 0, y: 0 }, { x: X_STEP, y: Y_STEP });
  assert.ok(d.startsWith('M'));
  assert.ok(d.includes('C'));
});

test('isDragGesture thresholds', () => {
  assert.equal(isDragGesture(0, 10), false);
  assert.equal(isDragGesture(0, 200), false);
  assert.equal(isDragGesture(2, 10), false);
  assert.equal(isDragGesture(DRAG_CLICK_PX, 10), false);
  assert.equal(isDragGesture(DRAG_CLICK_PX + 1, 10), true);
  assert.equal(isDragGesture(2, 200), true);
});

test('NODE_SIZE is 56 per design', () => {
  assert.equal(NODE_SIZE, 56);
});

test('pan only mutates tx/ty; bounds clamp extremes', () => {
  const layout = layoutAdvancement(fixtureAdvWideFlat(220));
  const fitted = fitTransform(layout.bounds, 900, 600);
  const dragged = { ...fitted, tx: fitted.tx + 120, ty: fitted.ty - 40 };
  assert.equal(dragged.k, fitted.k, 'pan must not change k');

  const pan = computePanBounds(layout.bounds, fitted, 900, 600, PAN_MARGIN);
  const wild = clampPan({ k: fitted.k, tx: 99999, ty: -99999 }, pan);
  assert.ok(wild.tx <= pan.maxTx + 1e-9);
  assert.ok(wild.tx >= pan.minTx - 1e-9);
  assert.ok(wild.ty <= pan.maxTy + 1e-9);
  assert.ok(wild.ty >= pan.minTy - 1e-9);
  // 内容完全装入视口时，pan 范围有限（小树不被甩飞）
  const small = layoutAdvancement([
    { id: 'r', parentId: null, title: 'R', hasDisplay: true },
    { id: 'c', parentId: 'r', title: 'C', hasDisplay: false },
  ]);
  const view = { k: 1, tx: 0, ty: 0 };
  const smallPan = computePanBounds(small.bounds, view, 800, 500, PAN_MARGIN);
  assert.ok(smallPan.maxTx - smallPan.minTx < 800, 'small tree pan is bounded');
  const clampedSmall = clampPan({ k: 1, tx: 5000, ty: 0 }, smallPan);
  assert.equal(clampedSmall.tx, smallPan.maxTx);
});

/* ---------- requirements 语义（OR(AND) 勿写反） ---------- */

test('requirements: [[a],[b],[c]] → 任一条件', () => {
  const req = parseRequirements([['a'], ['b'], ['c']]);
  assert.deepEqual(req, [['a'], ['b'], ['c']]);
  assert.equal(classifyRequirements(req), 'any');
  const view = describeRequirements(req);
  assert.equal(view.label, '任一条件');
  assert.deepEqual(view.groups, ['a', 'b', 'c']);
});

test('requirements: [[a,b,c]] → 全部条件', () => {
  const req = parseRequirements([['a', 'b', 'c']]);
  assert.equal(classifyRequirements(req), 'all');
  const view = describeRequirements(req);
  assert.equal(view.label, '全部条件');
  assert.deepEqual(view.groups, ['a 且 b 且 c']);
});

test('requirements: [[a,b],[c]] → (a 且 b) 或 c', () => {
  const req = parseRequirements([['a', 'b'], ['c']]);
  assert.equal(classifyRequirements(req), 'mixed');
  const view = describeRequirements(req);
  assert.equal(view.label, '(a 且 b) 或 c');
  assert.deepEqual(view.groups, ['(a 且 b)', 'c']);
});

test('requirements: empty / flat array handling', () => {
  assert.equal(classifyRequirements(undefined), 'empty');
  assert.equal(classifyRequirements([]), 'empty');
  assert.equal(classifyRequirements(parseRequirements(['a', 'b'])), 'all');
  assert.equal(describeRequirements(undefined).label, '（无 requirements）');
});

/* ---------- Tab 权威（path 启发式不得生成 Tab） ---------- */

test('Tab: 仅「无 parent 且有 display」成为 Tab', () => {
  const trees = groupAdvancementTrees([
    { id: 'ns:story/root', parentId: null, title: '故事', hasDisplay: true, iconItem: 'ns:book' },
    { id: 'ns:story/mine', parentId: 'ns:story/root', title: '挖矿', hasDisplay: true },
    { id: 'ns:adventure/root', parentId: null, title: '冒险', hasDisplay: true },
    { id: 'ns:orphan_no_disp', parentId: null, title: '无展示根', hasDisplay: false },
  ]);
  const tabs = trees.filter(t => t.kind === 'tab');
  assert.deepEqual(tabs.map(t => t.id).sort(), ['ns:adventure/root', 'ns:story/root']);
  // label 来自 display.title（此处 title 字段已 lang 解析）
  assert.equal(tabs.find(t => t.id === 'ns:story/root')!.label, '故事');
  // 子节点挂到 parent 所在 Tab，不另开 Tab
  const story = tabs.find(t => t.id === 'ns:story/root')!;
  assert.ok(story.nodes.some(n => n.id === 'ns:story/mine'));
  assert.ok(!tabs.some(t => t.id === 'ns:story/mine'));
  // parent∉S 且无 display → 未归组，不伪造 Tab
  const ungrouped = trees.find(t => t.kind === 'ungrouped');
  assert.ok(ungrouped, 'ungrouped group exists');
  assert.ok(ungrouped!.nodes.some(n => n.id === 'ns:orphan_no_disp'));
  assert.ok(!tabs.some(t => t.id === 'ns:orphan_no_disp'));
});

test('Tab: path 启发式不得生成 Tab（无 display 的 ns:adventure/root）', () => {
  const trees = groupAdvancementTrees([
    { id: 'ns:adventure/root', parentId: null, title: 'adventure', hasDisplay: false },
    { id: 'ns:adventure/kill', parentId: 'ns:adventure/root', title: 'Kill', hasDisplay: false },
  ]);
  const tabs = trees.filter(t => t.kind === 'tab');
  assert.equal(tabs.length, 0, 'path-looking root without display must NOT become Tab');
  const ungrouped = trees.find(t => t.kind === 'ungrouped');
  assert.ok(ungrouped);
  assert.equal(ungrouped!.nodes.length, 2);
});

test('Tab: recipes/** 无 display 不进玩家 Tab，默认折叠「配方解锁」', () => {
  const trees = groupAdvancementTrees([
    { id: 'ns:story/root', parentId: null, title: '故事', hasDisplay: true },
    { id: 'ns:recipes/misc/a', parentId: null, title: '配方A', hasDisplay: false, isRecipes: true },
    { id: 'ns:recipes/misc/b', parentId: 'ns:recipes/misc/a', title: '配方B', hasDisplay: false, isRecipes: true },
  ]);
  const tabs = trees.filter(t => t.kind === 'tab');
  assert.equal(tabs.length, 1);
  assert.equal(tabs[0].id, 'ns:story/root');
  assert.ok(!tabs.some(t => t.id.includes('recipes')));
  const recipes = trees.find(t => t.kind === 'recipes');
  assert.ok(recipes, 'recipes collapsible group present');
  assert.equal(recipes!.label, '配方解锁');
  assert.equal(recipes!.nodes.length, 2);
  // 配方组不应混入玩家 Tab 的节点
  assert.ok(!tabs[0].nodes.some(n => n.id.includes('recipes')));
});

test('isRecipesAdvancement detects advancement/recipes/**', () => {
  assert.equal(isRecipesAdvancement('minecraft:recipes/misc/crafting_table'), true);
  assert.equal(isRecipesAdvancement('ns:foo', 'data/ns/advancement/recipes/misc/x.json'), true);
  assert.equal(isRecipesAdvancement('ns:story/root'), false);
});

/* ---------- parse 补齐：display / rewards / requirements / icon.id ---------- */

test('parseAdvPayload: display.background/hidden/show_toast/announce_to_chat', () => {
  const parsed = parseAdvPayload({
    parent: 'ns:root',
    display: {
      icon: { id: 'ns:icon_item' },
      title: { translate: 'advancements.x.title' },
      description: { translate: 'advancements.x.desc' },
      frame: 'challenge',
      background: 'minecraft:textures/gui/advancements/backgrounds/adventure.png',
      hidden: true,
      show_toast: false,
      announce_to_chat: true,
    },
    requirements: [['a'], ['b']],
    rewards: { experience: 50, recipes: ['ns:foo'], loot: ['ns:bar'], function: 'ns:fn' },
    criteria: { a: { trigger: 'x' }, b: { trigger: 'y' } },
  }, 'ns:x');
  assert.equal(parsed.hasDisplay, true);
  assert.equal(parsed.iconItem, 'ns:icon_item', 'icon.id supported');
  assert.equal(parsed.frame, 'challenge');
  assert.equal(parsed.display?.background, 'minecraft:textures/gui/advancements/backgrounds/adventure.png');
  assert.equal(parsed.display?.hidden, true);
  assert.equal(parsed.display?.showToast, false);
  assert.equal(parsed.display?.announceToChat, true);
  assert.deepEqual(parsed.requirements, [['a'], ['b']]);
  assert.equal(parsed.rewards?.experience, 50);
  assert.deepEqual(parsed.rewards?.recipes, ['ns:foo']);
  assert.deepEqual(parsed.rewards?.loot, ['ns:bar']);
  assert.equal(parsed.rewards?.function, 'ns:fn');
});

test('parseAdvPayload: icon.item still works; no display → hasDisplay false', () => {
  const withItem = parseAdvPayload({ display: { icon: { item: 'minecraft:diamond' }, title: 'T' } }, 'x');
  assert.equal(withItem.iconItem, 'minecraft:diamond');
  assert.equal(withItem.hasDisplay, true);

  const noDisp = parseAdvPayload({ parent: 'gone', requirements: [['c']] }, 'ns:recipes/misc/z');
  assert.equal(noDisp.hasDisplay, false);
  assert.deepEqual(noDisp.requirements, [['c']]);
});

test('parseRewards + formatRewardsSummary', () => {
  const r = parseRewards({ experience: 100, recipes: ['a', 'b'] });
  assert.equal(r?.experience, 100);
  assert.equal(formatRewardsSummary(r), '100 XP · 配方×2');
  assert.equal(parseRewards(undefined), undefined);
  assert.equal(formatRewardsSummary(undefined), '');
});

test('itemToAdvNode / itemsToAdvTrees carry display meta + Tab filter', () => {
  const node = itemToAdvNode({
    key: 'ns:story/root',
    payload: {
      display: {
        icon: { item: 'ns:book' },
        title: { translate: 'advancements.story.root.title' },
        frame: 'task',
        background: 'ns:bg.png',
        hidden: false,
        show_toast: true,
        announce_to_chat: false,
      },
      requirements: [['root']],
    },
  }, { translateKey: k => (k.includes('story') ? '主线' : k) });
  assert.equal(node.hasDisplay, true);
  assert.equal(node.title, '主线');
  assert.equal(node.display?.background, 'ns:bg.png');
  assert.equal(node.display?.announceToChat, false);
  assert.equal(node.iconItem, 'ns:book');

  const trees = itemsToAdvTrees([
    {
      key: 'ns:story/root',
      payload: { display: { title: '主线', icon: { item: 'ns:book' } } },
    },
    {
      key: 'ns:recipes/misc/a',
      path: 'data/ns/advancement/recipes/misc/a.json',
      payload: { parent: 'ns:recipes/misc/root' },
    },
  ]);
  const tabs = trees.filter(t => t.kind === 'tab');
  assert.equal(tabs.length, 1);
  assert.equal(tabs[0].label, '主线');
  const recipes = trees.find(t => t.kind === 'recipes');
  assert.ok(recipes);
  assert.ok(!tabs[0].nodes.some(n => n.id.includes('recipes')));
});

test('itemToAdvNode keeps translated display title', () => {
  const node = itemToAdvNode({
    key: 'minecraft:adventure/kill',
    payload: {
      display: { title: { translate: 'advancements.adventure.kill_a_mob.title' } },
    },
  }, {
    translateKey: k => (k.startsWith('advancements.') ? '怪物猎人' : k),
  });
  assert.equal(node.title, '怪物猎人');
});

test('itemToAdvNode missing display → shortName fallback, no crash', () => {
  const node = itemToAdvNode({ key: 'mod:path/to/thing', payload: { parent: 'gone:nowhere' } });
  assert.equal(node.title, 'thing');
  assert.equal(node.parentId, 'gone:nowhere');
  assert.equal(node.hasDisplay, false);
  const layout = layoutAdvancement([node]);
  assert.equal(layout.nodes.length, 1);
  assert.equal(layout.missingParentCount, 1);
});

console.log(`\n${passed} passed, ${failures.length} failed`);
if (failures.length > 0) {
  process.exitCode = 1;
}
