/* 共享无限画布 viewport 纯函数单测。
   设计权威：docs/active/design/mp-infinite-canvas.md
   运行：cd apps/web && node --experimental-strip-types scripts/test-viewport.ts
*/
import assert from 'node:assert/strict';
import {
  ADV_ZOOM_MAX,
  ADV_ZOOM_MIN,
  QUEST_ZOOM_MAX,
  QUEST_ZOOM_MIN,
  boundsOf,
  clampZoom,
  fitBounds,
  gridMetrics,
  panBy,
  resetViewport,
  resolveWheel,
  screenDeltaToWorld,
  screenToWorld,
  wheelZoomFactor,
  worldToScreen,
  zoomAtAnchor,
  type Viewport,
} from '../src/ui/canvas/viewport.ts';

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

const EPS = 1e-9;
const near = (a: number, b: number, eps = EPS) => assert.ok(Math.abs(a - b) < eps, `${a} ≉ ${b}`);

console.log('viewport tests · mp-infinite-canvas M1');

/* ---------- 坐标互逆 ---------- */

test('screenToWorld / worldToScreen round-trip < 1e-6', () => {
  const v: Viewport = { x: 120.5, y: -40.25, k: 0.73 };
  const w = screenToWorld(v, 300, 200);
  const s = worldToScreen(v, w.x, w.y);
  assert.ok(Math.abs(s.x - 300) < 1e-6, `sx ${s.x}`);
  assert.ok(Math.abs(s.y - 200) < 1e-6, `sy ${s.y}`);
  const s2 = worldToScreen(v, 10, 20);
  const w2 = screenToWorld(v, s2.x, s2.y);
  assert.ok(Math.abs(w2.x - 10) < 1e-6);
  assert.ok(Math.abs(w2.y - 20) < 1e-6);
});

/* ---------- 锚点缩放不变性 ---------- */

test('zoomAtAnchor keeps world point under anchor fixed', () => {
  const v: Viewport = { x: 50, y: 80, k: 1.2 };
  const anchor = { x: 220, y: 140 };
  const before = screenToWorld(v, anchor.x, anchor.y);
  const next = zoomAtAnchor(v, 0.6, anchor);
  const after = screenToWorld(next, anchor.x, anchor.y);
  assert.ok(Math.abs(after.x - before.x) < 1e-9, `wx ${after.x} vs ${before.x}`);
  assert.ok(Math.abs(after.y - before.y) < 1e-9, `wy ${after.y} vs ${before.y}`);
  near(after.x, before.x);
  near(after.y, before.y);
});

test('zoomAtAnchor with k unchanged is identity', () => {
  const v: Viewport = { x: 10, y: 20, k: 1.5 };
  const n = zoomAtAnchor(v, 1.5, { x: 5, y: 5 });
  near(n.x, v.x);
  near(n.y, v.y);
  near(n.k, v.k);
});

/* ---------- pan / clamp ---------- */

test('panBy adds screen deltas', () => {
  const v = panBy({ x: 1, y: 2, k: 0.5 }, -10, 30);
  near(v.x, -9);
  near(v.y, 32);
  near(v.k, 0.5);
});

test('clampZoom respects bounds and non-finite', () => {
  near(clampZoom(0.01, QUEST_ZOOM_MIN, QUEST_ZOOM_MAX), QUEST_ZOOM_MIN);
  near(clampZoom(99, QUEST_ZOOM_MIN, QUEST_ZOOM_MAX), QUEST_ZOOM_MAX);
  near(clampZoom(1, QUEST_ZOOM_MIN, QUEST_ZOOM_MAX), 1);
  near(clampZoom(Number.NaN, 0.2, 2.5), 0.2);
  near(clampZoom(1.1, ADV_ZOOM_MIN, ADV_ZOOM_MAX), 1.1);
});

/* ---------- fitBounds ---------- */

test('fitBounds centers content and uses min(w/cw,h/ch)', () => {
  const bounds = { minX: 100, minY: 50, maxX: 500, maxY: 250 }; // 400 x 200
  const size = { width: 800, height: 400 };
  const v = fitBounds(bounds, size, 0.05, 5);
  // k = min(800/400, 400/200) = 2
  near(v.k, 2);
  // x = (800 - 400*2)/2 - 100*2 = 0 - 200 = -200
  near(v.x, -200);
  // y = (400 - 200*2)/2 - 50*2 = 0 - 100 = -100
  near(v.y, -100);
  // content center maps to viewport center
  const c = worldToScreen(v, 300, 150);
  near(c.x, 400);
  near(c.y, 200);
});

test('fitBounds clamps k to max and padding', () => {
  const bounds = { minX: 0, minY: 0, maxX: 100, maxY: 100 };
  const size = { width: 1000, height: 1000 };
  const v = fitBounds(bounds, size, 0.05, 2.5, 50);
  // raw = min(900/100, 900/100)=9 → clamp 2.5
  near(v.k, 2.5);
});

test('fitBounds empty-ish bounds never NaN', () => {
  const v = fitBounds({ minX: 0, minY: 0, maxX: 0, maxY: 0 }, { width: 400, height: 300 }, 0.08, 2.5);
  assert.ok(Number.isFinite(v.k) && Number.isFinite(v.x) && Number.isFinite(v.y));
  assert.ok(v.k >= 0.08);
});

/* ---------- reset / helpers ---------- */

test('resetViewport centers at k=1', () => {
  const r = resetViewport({ width: 800, height: 600 });
  near(r.k, 1);
  near(r.x, 400);
  near(r.y, 300);
});

test('wheelZoomFactor: scroll up zooms in', () => {
  assert.ok(wheelZoomFactor(-100) > 1);
  assert.ok(wheelZoomFactor(100) < 1);
  near(wheelZoomFactor(0), 1);
});

test('screenDeltaToWorld divides by k', () => {
  const d = screenDeltaToWorld(100, -50, 2);
  near(d.x, 50);
  near(d.y, -25);
});

test('gridMetrics scales with k', () => {
  const g = gridMetrics({ x: 10, y: 20, k: 2 }, 48);
  near(g.size, 96);
});

test('boundsOf returns null on empty', () => {
  assert.equal(boundsOf([]), null);
  const b = boundsOf([{ x: 0, y: 0 }, { x: 10, y: 20 }], 2);
  assert.ok(b);
  near(b!.minX, -2);
  near(b!.maxY, 22);
});

/* ---------- resolveWheel 矩阵 ---------- */

const baseV: Viewport = { x: 100, y: 200, k: 1 };
const anchor = { x: 300, y: 150 };
const size = { width: 800, height: 600 };
const range = { min: 0.15, max: 2.5 };

function wheel(partial: Partial<Parameters<typeof resolveWheel>[0]>) {
  return resolveWheel({
    deltaX: 0,
    deltaY: 0,
    altKey: false,
    ctrlKey: false,
    gesture: 'ps',
    viewport: baseV,
    anchor,
    size,
    min: range.min,
    max: range.max,
    ...partial,
  });
}

test('ps · 无修饰 → ty -= deltaY，tx 不变', () => {
  const n = wheel({ deltaY: 40 });
  near(n.x, 100);
  near(n.y, 200 - 40);
  near(n.k, 1);
  const n2 = wheel({ deltaY: -30 });
  near(n2.y, 200 - -30);
});

test('ps · 无修饰忽略 deltaX（纵向优先）', () => {
  const n = wheel({ deltaX: 25, deltaY: 10 });
  near(n.x, 100);
  near(n.y, 190);
});

test('ps · Alt+滚轮（deltaX=0）→ tx -= deltaY，ty 不变', () => {
  const n = wheel({ altKey: true, deltaY: 40, deltaX: 0 });
  near(n.x, 100 - 40);
  near(n.y, 200);
  near(n.k, 1);
});

test('ps · Alt + 触控板 deltaX → tx -= deltaX 优先', () => {
  const n = wheel({ altKey: true, deltaX: 18, deltaY: 40 });
  near(n.x, 100 - 18);
  near(n.y, 200);
});

test('ps · Ctrl+滚轮 → zoomAtAnchor，锚点世界坐标不变', () => {
  const before = screenToWorld(baseV, anchor.x, anchor.y);
  const n = wheel({ ctrlKey: true, deltaY: -120 });
  assert.ok(n.k > 1, `zoom in k=${n.k}`);
  const after = screenToWorld(n, anchor.x, anchor.y);
  assert.ok(Math.abs(after.x - before.x) < 1e-9);
  assert.ok(Math.abs(after.y - before.y) < 1e-9);
});

test('ps · Ctrl 缩放受 min/max 钳制', () => {
  const n = wheel({ ctrlKey: true, deltaY: 100000 });
  near(n.k, range.min);
  const n2 = wheel({ ctrlKey: true, deltaY: -100000 });
  near(n2.k, range.max);
});

test('vanillaAdv · 无 ctrl → tx/ty 跟随 delta', () => {
  const n = wheel({ gesture: 'vanillaAdv', deltaX: 12, deltaY: -30 });
  near(n.x, 100 - 12);
  near(n.y, 200 - -30);
  near(n.k, 1);
});

test('vanillaAdv · Ctrl+滚轮 → 锚点缩放', () => {
  const before = screenToWorld(baseV, anchor.x, anchor.y);
  const n = wheel({ gesture: 'vanillaAdv', ctrlKey: true, deltaY: -80 });
  assert.ok(n.k > 1);
  const after = screenToWorld(n, anchor.x, anchor.y);
  assert.ok(Math.abs(after.x - before.x) < 1e-9);
  assert.ok(Math.abs(after.y - before.y) < 1e-9);
});

test('vanillaAdv · Alt 不改变平移语义（仍双轴 pan）', () => {
  const n = wheel({ gesture: 'vanillaAdv', altKey: true, deltaX: 5, deltaY: 7 });
  near(n.x, 100 - 5);
  near(n.y, 200 - 7);
});

test('gesture 矩阵对照：同输入 ps vs vanillaAdv 结果不同', () => {
  const ps = wheel({ deltaY: 20, deltaX: 10 });
  const adv = wheel({ gesture: 'vanillaAdv', deltaY: 20, deltaX: 10 });
  assert.ok(ps.x !== adv.x || ps.y !== adv.y);
  near(ps.y, adv.y);
  assert.ok(adv.x !== ps.x, 'adv pans X, ps does not');
});

/* ---------- 任务书节点拖拽数学 ---------- */

test('node drag: Δworld = Δscreen / k', () => {
  const k = 0.5;
  const start = { x: 40, y: 60 };
  const dsx = 30;
  const dsy = -15;
  const d = screenDeltaToWorld(dsx, dsy, k);
  near(start.x + d.x, 40 + 60);
  near(start.y + d.y, 60 - 30);
});

console.log('');
console.log(`viewport: ${passed} passed, ${failures.length} failed`);
if (failures.length) {
  console.error('FAILURES:', failures.join(', '));
  process.exit(1);
}
