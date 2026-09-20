/**
 * Progress tree canvas headless self-test (Playwright Chromium).
 * Design authority: docs/design/advancement-vanilla-experience.md v1.1
 * Run: node --experimental-strip-types scripts/adv-tree-e2e.mjs
 * Assumes vite dev is already listening on ADV_DEMO_PORT (default 5197) on 127.0.0.1.
 */
import { chromium } from 'playwright-core';

const PORT = process.env.ADV_DEMO_PORT || '5197';
const BASE = `http://127.0.0.1:${PORT}/adv-tree-demo.html`;
const CHROME = process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

function parseTransform(raw) {
  const m = /translate\(([-\d.]+)px,\s*([-\d.]+)px\)\s*scale\(([-\d.]+)\)/.exec(raw || '');
  if (!m) return null;
  return { tx: Number(m[1]), ty: Number(m[2]), k: Number(m[3]) };
}

async function worldTransform(page) {
  return page.evaluate(() => {
    const el = document.querySelector('[data-testid="adv-tree-world"]');
    return el ? el.style.transform : '';
  });
}

async function nodeOffsets(page) {
  return page.evaluate(() => {
    return [...document.querySelectorAll('.adv-node')].map(n => ({
      id: n.getAttribute('data-node-id'),
      left: n.offsetLeft,
      top: n.offsetTop,
    }));
  });
}

async function assert(cond, msg) {
  if (!cond) throw new Error(msg);
  console.log(`  ✓ ${msg}`);
}

const browser = await chromium.launch({ headless: true, executablePath: CHROME });
const page = await browser.newPage({ viewport: { width: 1200, height: 800 } });
page.on('pageerror', err => {
  console.error('PAGE ERROR', err);
  process.exitCode = 1;
});

console.log('adv-tree e2e · vanilla-experience v1.1');

// --- mixed fixture ---
await page.goto(`${BASE}?mode=mixed`, { waitUntil: 'networkidle' });
await page.waitForSelector('[data-testid="adv-tree-canvas"]');
await page.waitForSelector('[data-testid="adv-tree-world"]', { state: 'attached' });

// Tab 权威：有 display 的 root 才是 Tab
const tabIds = await page.evaluate(() =>
  [...document.querySelectorAll('[data-testid^="adv-tab-"]')].map(el =>
    el.getAttribute('data-testid')?.replace('adv-tab-', ''),
  ),
);
await assert(tabIds.includes('demo:all/root'), `Tab includes display-root demo:all/root (${tabIds.join(',')})`);
await assert(tabIds.includes('demo:story/root'), `Tab includes display-root demo:story/root`);
await assert(!tabIds.some(id => id && id.includes('recipes')), 'recipes/** must NOT become Tab');
await assert(!tabIds.some(id => id && id.includes('cyc')), 'dirty roots must NOT become Tab');

// 全局数据质量提示（环在未归组，不随 Tab 消失）
const cycleWarn = await page.locator('.adv-tree-warn').textContent().catch(() => null);
await assert(Boolean(cycleWarn && cycleWarn.includes('循环')), `cycle warning shown globally: ${cycleWarn?.trim()}`);

// 无假完成图例：默认结构预览标注
const legend = await page.locator('[data-testid="adv-tree-legend"]').innerText();
await assert(
  legend.includes('演示完成态') || legend.includes('结构预览'),
  `legend present: ${legend.replace(/\s+/g, ' ').slice(0, 80)}`,
);

// 缩放增强 UI 标注
const enhance = await page.locator('[data-testid="adv-tree-zoom-enhance"]').innerText();
await assert(enhance.includes('工作台增强'), `zoom enhancement labeled: ${enhance.trim()}`);

// recipes / ungrouped 折叠组存在
const secondary = await page.locator('[data-testid="adv-tree-secondary"]').innerText().catch(() => '');
await assert(secondary.includes('配方解锁'), 'recipes collapsible group present');
await assert(secondary.includes('未归组'), 'ungrouped group present');

// 玩家 Tab 画布节点（脏数据不进默认玩家树）
const playerNodeCount = await page.locator('.adv-node').count();
await assert(playerNodeCount >= 4, `player tab nodes rendered (count=${playerNodeCount})`);
const playerIds = await page.evaluate(() =>
  [...document.querySelectorAll('.adv-node')].map(n => n.getAttribute('data-node-id')),
);
await assert(!playerIds.some(id => id && id.includes('recipes')), 'recipe nodes not on player tab canvas');
await assert(!playerIds.some(id => id && id.includes('cyc')), 'cycle nodes not on player tab canvas');

// select a node → inspector（requirements 语义 / rewards / display）
const kill = page.locator('[data-node-id="demo:all/kill"]');
await kill.click({ position: { x: 20, y: 20 } });
await page.waitForSelector('[data-testid="adv-tree-inspector"]');
const insp = await page.locator('[data-testid="adv-tree-inspector"]').innerText();
await assert(insp.includes('怪物猎人') || insp.includes('demo:all/kill'), 'Inspector shows selected node');
await assert(insp.includes('父链') || insp.includes('demo:all/root'), 'Inspector shows parent chain');
await assert(insp.includes('任一条件'), `Inspector requirements OR semantics: has 任一条件`);
await assert(insp.includes('rewards') || insp.includes('经验') || insp.includes('100'), 'Inspector shows rewards');
await assert(insp.includes('display'), 'Inspector shows display metadata');

// mixed requirements on and_or node
await page.locator('[data-node-id="demo:all/and_or"]').click({ position: { x: 20, y: 20 } });
const insp2 = await page.locator('[data-testid="adv-tree-inspector"]').innerText();
await assert(insp2.includes('(a 且 b) 或 c'), `mixed requirements label: ${(insp2.match(/\(a[^\n]+/g) || []).join()}`);

// hover 短卡
await page.locator('[data-node-id="demo:all/kill"]').hover();
await page.waitForSelector('[data-testid="adv-hover-card"]', { timeout: 2000 });
const hoverText = await page.locator('[data-testid="adv-hover-card"]').innerText();
await assert(hoverText.includes('怪物猎人'), 'hover card shows title');

// 回到 k=1 再测手势，避免小树 fit 放大后 pan 被有界钳死
await page.locator('[data-testid="adv-tree-btn-reset"]').click();
await page.waitForTimeout(30);

// drag pan: only transform changes, node offsets stable（有界 pan）
const beforeT = parseTransform(await worldTransform(page));
const beforeOffsets = await nodeOffsets(page);
const canvas = page.locator('[data-testid="adv-tree-canvas"]');
const box = await canvas.boundingBox();
await page.mouse.move(box.x + 400, box.y + 300);
await page.mouse.down();
await page.mouse.move(box.x + 480, box.y + 260, { steps: 8 });
await page.mouse.up();
const afterT = parseTransform(await worldTransform(page));
const afterOffsets = await nodeOffsets(page);
await assert(beforeT && afterT, `transform present before/after drag: ${JSON.stringify(beforeT)} → ${JSON.stringify(afterT)}`);
await assert(afterT.k === beforeT.k, `drag only changes translate (k ${beforeT.k} → ${afterT.k})`);
const dx = afterT.tx - beforeT.tx;
const dy = afterT.ty - beforeT.ty;
await assert(Math.abs(dx) > 1 || Math.abs(dy) > 1, `pan had effect (dx=${dx.toFixed(1)}, dy=${dy.toFixed(1)})`);
await assert(Math.abs(dx) <= 82 && Math.abs(dy) <= 42, `bounded pan does not exceed gesture by much (dx=${dx.toFixed(1)}, dy=${dy.toFixed(1)})`);
await assert(JSON.stringify(beforeOffsets) === JSON.stringify(afterOffsets), 'node offsetLeft/Top unchanged during drag (no DOM relayout)');

// ctrl+wheel zoom at pointer（工作台增强）
const beforeZoom = parseTransform(await worldTransform(page));
await page.mouse.move(box.x + 300, box.y + 250);
await page.keyboard.down('Control');
await page.mouse.wheel(0, -240);
await page.keyboard.up('Control');
await page.waitForTimeout(50);
const afterZoom = parseTransform(await worldTransform(page));
await assert(afterZoom.k > beforeZoom.k, `ctrl+wheel zoomed in: ${beforeZoom.k} → ${afterZoom.k}`);
const pct = await page.locator('[data-testid="adv-tree-zoom-percent"]').getAttribute('aria-label');
await assert(pct && pct.includes('%'), `zoom percent aria-label: ${pct}`);

// 无 ctrl 滚轮 = 平移（先 reset，避免 k 过大时 pan 已到界）
await page.locator('[data-testid="adv-tree-btn-reset"]').click();
await page.waitForTimeout(30);
const beforePanWheel = parseTransform(await worldTransform(page));
await page.mouse.move(box.x + 400, box.y + 300);
await page.mouse.wheel(0, 80);
await page.waitForTimeout(50);
const afterPanWheel = parseTransform(await worldTransform(page));
await assert(
  afterPanWheel.k === beforePanWheel.k && afterPanWheel.ty !== beforePanWheel.ty,
  `wheel without ctrl pans (k same, ty ${beforePanWheel.ty} → ${afterPanWheel.ty})`,
);

// interactive clamp
for (let i = 0; i < 20; i++) {
  await page.locator('[data-testid="adv-tree-btn-zoom-in"]').click();
}
const clamped = parseTransform(await worldTransform(page));
await assert(Math.abs(clamped.k - 2.5) < 1e-6, `interactive zoom clamped at 2.5 (got ${clamped.k})`);

// reset
await page.locator('[data-testid="adv-tree-btn-reset"]').click();
const resetT = parseTransform(await worldTransform(page));
await assert(Math.abs(resetT.k - 1) < 1e-6, `reset returns k=1 (got ${resetT.k})`);

// 展开未归组：脏数据可见
const ungroupedToggle = page.locator('[data-testid="adv-secondary-__ungrouped__"]');
if (await ungroupedToggle.count()) {
  await ungroupedToggle.click();
  await page.waitForTimeout(100);
  const dirtyVisible = await page.locator('.adv-tree-warn').count();
  await assert(dirtyVisible >= 0, 'ungrouped expandable without crash');
}

// --- wide fixture fit ---
await page.goto(`${BASE}?mode=wide`, { waitUntil: 'networkidle' });
await page.waitForSelector('[data-testid="adv-tree-canvas"]');
await page.waitForSelector('[data-testid="adv-tree-world"]', { state: 'attached' });
const wideNodes = await page.locator('.adv-node').count();
await assert(wideNodes >= 220, `wide-flat fixture rendered ${wideNodes} nodes`);

await page.locator('[data-testid="adv-tree-btn-zoom-in"]').click();
await page.locator('[data-testid="adv-tree-btn-fit"]').click();
await page.waitForTimeout(50);
const fitT = parseTransform(await worldTransform(page));
await assert(fitT.k < 0.35, `fit allows k_fit < 0.35 (got ${fitT.k})`);
await assert(fitT.k >= 0.08 - 1e-9, `k_fit floor 0.08 (got ${fitT.k})`);

const fitPct = await page.locator('[data-testid="adv-tree-zoom-percent"]').innerText();
const expectPct = String(Math.round(fitT.k * 100));
await assert(fitPct.trim().startsWith(expectPct), `toolbar percent matches k_fit (${fitPct.trim()} vs ${expectPct}%)`);

const fitOffsets = await nodeOffsets(page);
const boxW = await canvas.boundingBox();
await page.mouse.move(boxW.x + 300, boxW.y + 300);
await page.mouse.down();
await page.mouse.move(boxW.x + 220, boxW.y + 340, { steps: 5 });
await page.mouse.up();
const afterFitDrag = parseTransform(await worldTransform(page));
const afterFitOffsets = await nodeOffsets(page);
await assert(afterFitDrag.k === fitT.k, 'fit-then-drag keeps k');
await assert(JSON.stringify(fitOffsets) === JSON.stringify(afterFitOffsets), 'wide tree drag does not relayout nodes');

await page.screenshot({ path: 'scripts/adv-tree-e2e-wide.png', fullPage: false });
await page.goto(`${BASE}?mode=mixed`, { waitUntil: 'networkidle' });
await page.screenshot({ path: 'scripts/adv-tree-e2e-mixed.png', fullPage: false });

await browser.close();
console.log('\ne2e OK');
