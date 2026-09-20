/* 共享无限画布：视口数学与手势决议纯函数。
   设计权威：docs/design/mp-infinite-canvas.md
   无 DOM/React 依赖，可被 node --experimental-strip-types 直接单测。 */

export type Viewport = { x: number; y: number; k: number };
export type Point = { x: number; y: number };
export type Size = { width: number; height: number };
export type Bounds = { minX: number; minY: number; maxX: number; maxY: number };

export type CanvasGesture = 'ps' | 'vanillaAdv';

/** 任务书默认（设计 §2） */
export const QUEST_ZOOM_MIN = 0.15;
export const QUEST_ZOOM_MAX = 2.5;
/** 进度交互默认（设计 §2）；fit 可更低 */
export const ADV_ZOOM_MIN = 0.2;
export const ADV_ZOOM_MAX = 2.5;
export const ADV_FIT_ZOOM_MIN = 0.08;
export const DEFAULT_ZOOM_MIN = 0.05;
export const DEFAULT_ZOOM_MAX = 5;

export const DRAG_CLICK_PX = 3;
export const GRID_BASE = 48;

export function clampZoom(k: number, min: number, max: number): number {
  if (!Number.isFinite(k)) return min;
  return Math.min(max, Math.max(min, k));
}

/** S = W * k + (x, y) */
export function worldToScreen(viewport: Viewport, wx: number, wy: number): Point {
  return { x: wx * viewport.k + viewport.x, y: wy * viewport.k + viewport.y };
}

/** W = (S - (x, y)) / k */
export function screenToWorld(viewport: Viewport, sx: number, sy: number): Point {
  return { x: (sx - viewport.x) / viewport.k, y: (sy - viewport.y) / viewport.k };
}

/** 以屏幕锚点缩放：该点下世界坐标不变。 */
export function zoomAtAnchor(viewport: Viewport, kNext: number, anchor: Point): Viewport {
  const w = screenToWorld(viewport, anchor.x, anchor.y);
  return { x: anchor.x - w.x * kNext, y: anchor.y - w.y * kNext, k: kNext };
}

/** 屏幕位移平移视口。 */
export function panBy(viewport: Viewport, dx: number, dy: number): Viewport {
  return { x: viewport.x + dx, y: viewport.y + dy, k: viewport.k };
}

/**
 * 适应内容 bounds。
 * k_fit = clamp(min(w/cw, h/ch), min, max)
 * x = (w - cw*k_fit)/2 - minX*k_fit
 * y = (h - ch*k_fit)/2 - minY*k_fit
 */
export function fitBounds(
  bounds: Bounds,
  size: Size,
  min: number,
  max: number,
  padding = 0,
): Viewport {
  const cw = Math.max(bounds.maxX - bounds.minX, 1);
  const ch = Math.max(bounds.maxY - bounds.minY, 1);
  const availW = Math.max(size.width - padding * 2, 1);
  const availH = Math.max(size.height - padding * 2, 1);
  const k = clampZoom(Math.min(availW / cw, availH / ch), min, max);
  return {
    x: (size.width - cw * k) / 2 - bounds.minX * k,
    y: (size.height - ch * k) / 2 - bounds.minY * k,
    k,
  };
}

/** 重置：k=1，原点对齐容器中心（设计 §4）。 */
export function resetViewport(size: Size): Viewport {
  return { x: size.width / 2, y: size.height / 2, k: 1 };
}

/** infinite-canvas 同款滚轮缩放因子。 */
export function wheelZoomFactor(deltaY: number): number {
  return Math.pow(1.1, -deltaY / 100);
}

export type ResolveWheelArgs = {
  deltaX: number;
  deltaY: number;
  altKey: boolean;
  ctrlKey: boolean;
  gesture: CanvasGesture;
  viewport: Viewport;
  /** 相对容器左上角的屏幕锚点 */
  anchor: Point;
  size: Size;
  min: number;
  max: number;
};

/**
 * 手势决议（用户拍板 2026-09-20）：
 * - ps：无修饰 → ty -= deltaY（纵向）；alt → 横向（Alt+滚轮用 deltaY，与 PS 一致）；ctrl → zoomAtAnchor
 * - vanillaAdv：无 ctrl → tx/ty 跟随 delta；ctrl → zoomAtAnchor
 * metaKey 在组件层折入 ctrlKey。
 */
export function resolveWheel(args: ResolveWheelArgs): Viewport {
  const { deltaX, deltaY, altKey, ctrlKey, gesture, viewport, anchor, min, max } = args;

  if (ctrlKey) {
    const kNext = clampZoom(viewport.k * wheelZoomFactor(deltaY), min, max);
    return zoomAtAnchor(viewport, kNext, anchor);
  }

  if (gesture === 'ps') {
    if (altKey) {
      // PS：Alt+滚轮把 deltaY 当横向；触控板有 deltaX 时优先用横向分量
      const hx = deltaX !== 0 ? deltaX : deltaY;
      return panBy(viewport, -hx, 0);
    }
    return panBy(viewport, 0, -deltaY);
  }

  // vanillaAdv：原版进度界面滚轮 = 平移
  return panBy(viewport, -deltaX, -deltaY);
}

/** 网格背景尺寸：gridSize = GRID_BASE * k */
export function gridMetrics(viewport: Viewport, base = GRID_BASE): { size: number; posX: number; posY: number } {
  const size = Math.max(base * viewport.k, 1);
  return {
    size,
    posX: viewport.x % size,
    posY: viewport.y % size,
  };
}

/** 节点拖拽：世界坐标 Δ = 屏幕 Δ / k */
export function screenDeltaToWorld(dx: number, dy: number, k: number): Point {
  return { x: dx / k, y: dy / k };
}

export function boundsOf(points: ReadonlyArray<Point>, pad = 0): Bounds | null {
  if (points.length === 0) return null;
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const p of points) {
    minX = Math.min(minX, p.x);
    minY = Math.min(minY, p.y);
    maxX = Math.max(maxX, p.x);
    maxY = Math.max(maxY, p.y);
  }
  return { minX: minX - pad, minY: minY - pad, maxX: maxX + pad, maxY: maxY + pad };
}
