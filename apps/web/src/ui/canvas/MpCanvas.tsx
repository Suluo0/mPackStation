import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type MouseEvent as ReactMouseEvent,
} from 'react';
import {
  ADV_FIT_ZOOM_MIN,
  ADV_ZOOM_MAX,
  ADV_ZOOM_MIN,
  DRAG_CLICK_PX,
  GRID_BASE,
  QUEST_ZOOM_MAX,
  QUEST_ZOOM_MIN,
  clampZoom,
  fitBounds,
  gridMetrics,
  resolveWheel,
  resetViewport,
  screenToWorld,
  worldToScreen,
  type Bounds,
  type CanvasGesture,
  type Point,
  type Size,
  type Viewport,
} from './viewport';
import {MpZoomBar} from './MpZoomBar';
import './MpCanvas.css';

export type MpCanvasApi = {
  zoomAtCenter: (k: number) => void;
  zoomBy: (factor: number) => void;
  reset: () => void;
  fit: () => void;
  panTo: (wx: number, wy: number, opts?: { k?: number }) => void;
  getViewport: () => Viewport;
  getSize: () => Size;
  screenToWorld: (sx: number, sy: number) => Point;
  worldToScreen: (wx: number, wy: number) => Point;
  setViewport: (v: Viewport) => void;
};

export type MpNodeDragInfo = {
  nodeId: string;
  worldX: number;
  worldY: number;
  worldDx: number;
  worldDy: number;
  shiftKey: boolean;
};

export type MpLinkDragHandlers = {
  onStart: (nodeId: string, world: Point) => void;
  onMove: (world: Point) => void;
  onEnd: (world: Point, targetNodeId: string | null, sourceNodeId: string) => void;
};

export type MpCanvasProps = {
  viewport?: Viewport;
  initialViewport?: Viewport;
  onViewportChange?: (v: Viewport) => void;
  tool?: 'select' | 'pan';
  gesture?: CanvasGesture;
  background?: 'grid' | 'dots' | 'blank';
  minZoom?: number;
  maxZoom?: number;
  /** 适应时允许更低 k（进度树沿用 vanilla 设计） */
  fitMinZoom?: number;
  fitPadding?: number;
  getContentBounds?: () => Bounds | null;
  /** 每次视口提交前的钳制（进度有界 pan） */
  constrainViewport?: (v: Viewport) => Viewport;
  /** select + true：拖节点走 onNodeDrag；false/vanillaAdv：节点上拖动=平移 */
  nodeDraggable?: boolean;
  onNodeDrag?: (info: MpNodeDragInfo) => void;
  onNodeClick?: (nodeId: string, e: { shiftKey: boolean; clientX: number; clientY: number }) => void;
  onBackgroundClick?: () => void;
  onCanvasDoubleClickWorld?: (w: Point, e: ReactMouseEvent) => void;
  onDoubleClickNode?: (nodeId: string, e: ReactMouseEvent) => void;
  onContextMenuWorld?: (
    w: Point,
    e: ReactMouseEvent,
    info: {on: 'canvas' | 'node' | 'overlay'; nodeId?: string},
  ) => void;
  /** 端口拖拽连线（C2）：目标带 data-canvas-link-port */
  onLinkDrag?: MpLinkDragHandlers;
  expose?: (api: MpCanvasApi) => void;
  className?: string;
  worldClassName?: string | ((k: number) => string);
  worldStyle?: React.CSSProperties;
  containerTestId?: string;
  worldTestId?: string;
  ariaLabel?: string;
  tabIndex?: number;
  showZoomBar?: boolean;
  zoomBar?: ReactNode;
  zoomBarAlign?: 'left' | 'right';
  /** 画布覆盖层（不参与世界 transform）：hover 卡等 */
  overlay?: ReactNode;
  onKeyDown?: (e: ReactKeyboardEvent) => void;
  onKeyUp?: (e: ReactKeyboardEvent) => void;
  /** 覆盖重置行为；默认 k=1 居中 */
  onResetOverride?: () => void;
  onFitOverride?: () => void;
  children: ReactNode;
};

type DragState = {
  pointerId: number;
  mode: 'pan' | 'node' | 'link';
  startX: number;
  startY: number;
  startViewport: Viewport;
  moved: boolean;
  nodeId?: string;
  startWorld?: Point;
  shiftKey: boolean;
};

function isEditableTarget(target: EventTarget | null): boolean {
  const el = target instanceof Element ? target : null;
  if (!el) return false;
  return Boolean(
    el.closest(
      "input, textarea, select, [contenteditable='true'], .ant-select-dropdown, .ant-picker-dropdown",
    ),
  );
}

function defaultGestureZoom(gesture: CanvasGesture): { min: number; max: number; fitMin: number } {
  if (gesture === 'ps') return { min: QUEST_ZOOM_MIN, max: QUEST_ZOOM_MAX, fitMin: QUEST_ZOOM_MIN };
  return { min: ADV_ZOOM_MIN, max: ADV_ZOOM_MAX, fitMin: ADV_FIT_ZOOM_MIN };
}

function readNodeWorld(el: HTMLElement): Point {
  const nx = el.style.getPropertyValue('--nx');
  const ny = el.style.getPropertyValue('--ny');
  if (nx !== '' || ny !== '') {
    return { x: Number(nx || 0), y: Number(ny || 0) };
  }
  return { x: Number(el.style.left || 0), y: Number(el.style.top || 0) };
}

/**
 * 共享无限画布：一层世界 transform + 手势（ps / vanillaAdv）+ rAF 合帧。
 * 业务节点只写世界坐标 left/top，不自己算 transform。
 */
export function MpCanvas(props: MpCanvasProps) {
  const {
    viewport: viewportProp,
    initialViewport,
    onViewportChange,
    tool = 'select',
    gesture = 'ps',
    background = 'grid',
    minZoom,
    maxZoom,
    fitMinZoom,
    fitPadding = 24,
    getContentBounds,
    constrainViewport,
    nodeDraggable,
    onNodeDrag,
    onNodeClick,
    onBackgroundClick,
    onCanvasDoubleClickWorld,
    onDoubleClickNode,
    onContextMenuWorld,
    onLinkDrag,
    expose,
    className,
    worldClassName,
    worldStyle,
    containerTestId = 'mp-canvas',
    worldTestId = 'mp-world',
    ariaLabel = '无限画布',
    tabIndex = 0,
    showZoomBar = true,
    zoomBar,
    zoomBarAlign = 'left',
    overlay,
    onKeyDown,
    onKeyUp,
    onResetOverride,
    onFitOverride,
    children,
  } = props;

  const defaults = useMemo(() => defaultGestureZoom(gesture), [gesture]);
  const kMin = minZoom ?? defaults.min;
  const kMax = maxZoom ?? defaults.max;
  const kFitMin = fitMinZoom ?? defaults.fitMin;
  const canDragNode = nodeDraggable ?? (tool === 'select' && gesture === 'ps');

  const containerRef = useRef<HTMLDivElement>(null);
  const worldRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<DragState | null>(null);
  const frameRef = useRef<number | null>(null);
  const pendingRef = useRef<Viewport | null>(null);
  const spaceRef = useRef(false);
  const controlled = viewportProp != null;

  const [innerViewport, setInnerViewport] = useState<Viewport>(
    () => viewportProp ?? initialViewport ?? { x: 0, y: 0, k: 1 },
  );
  const viewportRef = useRef<Viewport>(viewportProp ?? innerViewport);
  const [displayK, setDisplayK] = useState<number>(() => viewportRef.current.k);
  const [isPanning, setIsPanning] = useState(false);

  const sizeRef = useRef<Size>({ width: 800, height: 500 });

  const measure = useCallback((): Size => {
    const el = containerRef.current;
    if (!el) return sizeRef.current;
    const next = { width: el.clientWidth || 800, height: el.clientHeight || 500 };
    sizeRef.current = next;
    return next;
  }, []);

  const applyDom = useCallback((v: Viewport) => {
    const world = worldRef.current;
    if (world) {
      world.style.transform = `translate(${v.x}px, ${v.y}px) scale(${v.k})`;
    }
    const grid = gridRef.current;
    if (grid) {
      const g = gridMetrics(v, GRID_BASE);
      grid.style.backgroundSize = `${g.size}px ${g.size}px`;
      grid.style.backgroundPosition = `${g.posX}px ${g.posY}px`;
    }
  }, []);

  const commit = useCallback(
    (raw: Viewport, opts?: { forceState?: boolean; allowFitMin?: boolean }) => {
      // 平移保持当前 k（含 fit 低倍）；显式缩放/重置才用交互 kMin
      const preserveK = opts?.allowFitMin || raw.k === viewportRef.current.k;
      const lo = preserveK ? Math.min(kFitMin, kMin, raw.k, viewportRef.current.k) : kMin;
      const clampedK = clampZoom(raw.k, lo, kMax);
      let next: Viewport = { x: raw.x, y: raw.y, k: clampedK };
      if (constrainViewport) next = constrainViewport(next);
      next = { x: next.x, y: next.y, k: clampZoom(next.k, Math.min(lo, next.k), kMax) };
      viewportRef.current = next;
      applyDom(next);
      if (!controlled) setInnerViewport(next);
      onViewportChange?.(next);
      if (frameRef.current == null) {
        frameRef.current = requestAnimationFrame(() => {
          frameRef.current = null;
          setDisplayK(viewportRef.current.k);
        });
      }
    },
    [applyDom, constrainViewport, controlled, kFitMin, kMax, kMin, onViewportChange],
  );

  /* 受控 prop → DOM（fit 允许低于交互 kMin） */
  useEffect(() => {
    if (!viewportProp) return;
    const lo = Math.min(kFitMin, kMin);
    let next: Viewport = { ...viewportProp, k: clampZoom(viewportProp.k, lo, kMax) };
    if (constrainViewport) next = constrainViewport(next);
    viewportRef.current = next;
    applyDom(next);
    setDisplayK(next.k);
  }, [viewportProp, applyDom, constrainViewport, kFitMin, kMax, kMin]);

  /* 首次挂载测量 + 默认视口 */
  useEffect(() => {
    measure();
    if (!controlled && !initialViewport) {
      const s = measure();
      const init = resetViewport(s);
      viewportRef.current = init;
      applyDom(init);
      setInnerViewport(init);
      setDisplayK(init.k);
    } else {
      applyDom(viewportRef.current);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => () => {
    if (frameRef.current != null) cancelAnimationFrame(frameRef.current);
  }, []);

  const getSize = useCallback(() => measure(), [measure]);

  const api: MpCanvasApi = useMemo(
    () => ({
      zoomAtCenter: (k: number) => {
        const s = measure();
        const anchor = { x: s.width / 2, y: s.height / 2 };
        const kNext = clampZoom(k, kMin, kMax);
        const w = screenToWorld(viewportRef.current, anchor.x, anchor.y);
        commit({ x: anchor.x - w.x * kNext, y: anchor.y - w.y * kNext, k: kNext }, { forceState: true });
      },
      zoomBy: (factor: number) => {
        const s = measure();
        const anchor = { x: s.width / 2, y: s.height / 2 };
        const cur = viewportRef.current;
        const kNext = clampZoom(cur.k * factor, kMin, kMax);
        const w = screenToWorld(cur, anchor.x, anchor.y);
        commit({ x: anchor.x - w.x * kNext, y: anchor.y - w.y * kNext, k: kNext });
      },
      reset: () => {
        if (onResetOverride) {
          onResetOverride();
          return;
        }
        commit(resetViewport(measure()), { forceState: true });
      },
      fit: () => {
        if (onFitOverride) {
          onFitOverride();
          return;
        }
        const bounds = getContentBounds?.();
        const s = measure();
        if (!bounds) {
          commit(resetViewport(s), { forceState: true });
          return;
        }
        commit(fitBounds(bounds, s, kFitMin, kMax, fitPadding), { forceState: true, allowFitMin: true });
      },
      panTo: (wx: number, wy: number, opts) => {
        const s = measure();
        const k = clampZoom(opts?.k ?? viewportRef.current.k, kMin, kMax);
        commit({ x: s.width / 2 - wx * k, y: s.height / 2 - wy * k, k });
      },
      getViewport: () => viewportRef.current,
      getSize,
      screenToWorld: (sx: number, sy: number) => screenToWorld(viewportRef.current, sx, sy),
      worldToScreen: (wx: number, wy: number) => worldToScreen(viewportRef.current, wx, wy),
      setViewport: (v: Viewport) => commit(v, { forceState: true, allowFitMin: true }),
    }),
    [
      commit,
      constrainViewport,
      fitPadding,
      getContentBounds,
      kFitMin,
      kMax,
      kMin,
      measure,
      getSize,
      onFitOverride,
      onResetOverride,
    ],
  );

  useEffect(() => {
    expose?.(api);
  }, [expose, api]);

  /* Space 临时 pan + 快捷键 0/1/+/- */
  useEffect(() => {
    const onKeyDownWin = (e: KeyboardEvent) => {
      if (isEditableTarget(e.target)) return;
      if (e.code === 'Space') {
        e.preventDefault();
        spaceRef.current = true;
        containerRef.current?.classList.add('is-panning');
      }
    };
    const onKeyUpWin = (e: KeyboardEvent) => {
      if (e.code === 'Space') {
        spaceRef.current = false;
        if (!dragRef.current) containerRef.current?.classList.remove('is-panning');
      }
    };
    const onBlur = () => {
      spaceRef.current = false;
      dragRef.current = null;
      setIsPanning(false);
      containerRef.current?.classList.remove('is-panning');
    };
    window.addEventListener('keydown', onKeyDownWin);
    window.addEventListener('keyup', onKeyUpWin);
    window.addEventListener('blur', onBlur);
    return () => {
      window.removeEventListener('keydown', onKeyDownWin);
      window.removeEventListener('keyup', onKeyUpWin);
      window.removeEventListener('blur', onBlur);
    };
  }, []);

  const localKeyDown = useCallback(
    (e: ReactKeyboardEvent) => {
      onKeyDown?.(e);
      if (e.defaultPrevented || isEditableTarget(e.target)) return;
      if (e.key === '0') {
        e.preventDefault();
        api.reset();
        return;
      }
      if (e.key === '1') {
        e.preventDefault();
        api.fit();
        return;
      }
      if (e.key === '+' || e.key === '=') {
        e.preventDefault();
        api.zoomBy(1.15);
        return;
      }
      if (e.key === '-' || e.key === '_') {
        e.preventDefault();
        api.zoomBy(1 / 1.15);
      }
    },
    [api, onKeyDown],
  );

  /* wheel：passive:false + resolveWheel */
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      const target = e.target instanceof Element ? e.target : null;
      if (
        target?.closest(
          "[data-canvas-no-zoom],.ant-modal,.ant-popover,.ant-dropdown,.ant-select-dropdown,.ant-picker-dropdown",
        )
      ) {
        return;
      }
      e.preventDefault();
      const rect = el.getBoundingClientRect();
      const anchor = { x: e.clientX - rect.left, y: e.clientY - rect.top };
      const next = resolveWheel({
        deltaX: e.deltaX,
        deltaY: e.deltaY,
        altKey: e.altKey,
        ctrlKey: e.ctrlKey || e.metaKey,
        gesture,
        viewport: viewportRef.current,
        anchor,
        size: measure(),
        min: kMin,
        max: kMax,
      });
      // resolveWheel 可能产生越界 k（ctrl 路径已 clamp）；commit 再钳一次
      commit(next);
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [commit, gesture, kMax, kMin, measure]);

  const screenPoint = useCallback((clientX: number, clientY: number): Point => {
    const rect = containerRef.current?.getBoundingClientRect();
    return {
      x: clientX - (rect?.left ?? 0),
      y: clientY - (rect?.top ?? 0),
    };
  }, []);

  const onPointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      const target = e.target instanceof Element ? e.target : null;
      if (target?.closest('[data-canvas-no-zoom]')) return;
      if (target?.closest('.ant-modal,.ant-popover,.ant-dropdown')) return;

      const sp = screenPoint(e.clientX, e.clientY);
      const world = screenToWorld(viewportRef.current, sp.x, sp.y);

      // C2：端口拖拽连线
      const portEl = target?.closest('[data-canvas-link-port]') as HTMLElement | null;
      if (portEl && onLinkDrag && tool === 'select') {
        e.preventDefault();
        e.stopPropagation();
        e.currentTarget.setPointerCapture(e.pointerId);
        const sourceId = portEl.dataset.nodeId || portEl.closest('[data-node-id]')?.getAttribute('data-node-id') || '';
        dragRef.current = {
          pointerId: e.pointerId,
          mode: 'link',
          startX: e.clientX,
          startY: e.clientY,
          startViewport: viewportRef.current,
          moved: false,
          nodeId: sourceId,
          startWorld: world,
          shiftKey: e.shiftKey,
        };
        onLinkDrag.onStart(sourceId, world);
        return;
      }

      const nodeEl = target?.closest('[data-node-id]') as HTMLElement | null;
      const temporaryPan = spaceRef.current || e.ctrlKey || e.button === 1;
      const background = !nodeEl && !portEl;

      // 中键 / Space / Ctrl：始终平移
      if (temporaryPan || e.button === 1 || (tool === 'pan' && background) || (!canDragNode && nodeEl && e.button === 0)) {
        if (e.button !== 1 && !spaceRef.current && !e.ctrlKey && tool !== 'pan' && canDragNode) {
          // fall through to node/select logic
        } else {
          e.preventDefault();
          e.currentTarget.setPointerCapture(e.pointerId);
          dragRef.current = {
            pointerId: e.pointerId,
            mode: 'pan',
            startX: e.clientX,
            startY: e.clientY,
            startViewport: viewportRef.current,
            moved: false,
            shiftKey: e.shiftKey,
            nodeId: nodeEl?.dataset.nodeId,
          };
          setIsPanning(true);
          containerRef.current?.classList.add('is-panning');
          return;
        }
      }

      // select：左键拖节点
      if (e.button === 0 && nodeEl && canDragNode && tool === 'select') {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        const startWorld = readNodeWorld(nodeEl);
        dragRef.current = {
          pointerId: e.pointerId,
          mode: 'node',
          startX: e.clientX,
          startY: e.clientY,
          startViewport: viewportRef.current,
          moved: false,
          nodeId: nodeEl.dataset.nodeId,
          startWorld,
          shiftKey: e.shiftKey,
        };
        return;
      }

      // select：左键空白 / 只读节点 → 可平移 + click 回调
      if (e.button === 0) {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        dragRef.current = {
          pointerId: e.pointerId,
          mode: 'pan',
          startX: e.clientX,
          startY: e.clientY,
          startViewport: viewportRef.current,
          moved: false,
          shiftKey: e.shiftKey,
          nodeId: nodeEl?.dataset.nodeId,
        };
        setIsPanning(true);
        containerRef.current?.classList.add('is-panning');
      }
    },
    [canDragNode, onLinkDrag, screenPoint, tool],
  );

  /* rAF 合帧：pointermove 只写 pending，一帧一次 commit / 直接 transform */
  useEffect(() => {
    const flush = () => {
      frameRef.current = null;
      if (pendingRef.current) {
        const p = pendingRef.current;
        pendingRef.current = null;
        viewportRef.current = p;
        applyDom(p);
        setDisplayK(p.k);
        if (controlled) onViewportChange?.(p);
        else {
          setInnerViewport(p);
          onViewportChange?.(p);
        }
      }
    };

    const onPointerMove = (e: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId) return;
      const dx = e.clientX - drag.startX;
      const dy = e.clientY - drag.startY;
      if (Math.hypot(dx, dy) > DRAG_CLICK_PX) drag.moved = true;

      if (drag.mode === 'pan') {
        const next = {
          x: drag.startViewport.x + dx,
          y: drag.startViewport.y + dy,
          k: drag.startViewport.k,
        };
        const constrained = constrainViewport ? constrainViewport(next) : next;
        pendingRef.current = constrained;
        viewportRef.current = constrained;
        applyDom(constrained);
        if (frameRef.current == null) {
          frameRef.current = requestAnimationFrame(flush);
        }
        return;
      }

      if (drag.mode === 'node' && drag.nodeId && drag.startWorld && drag.moved) {
        const k = viewportRef.current.k;
        const worldDx = dx / k;
        const worldDy = dy / k;
        onNodeDrag?.({
          nodeId: drag.nodeId,
          worldX: drag.startWorld.x + worldDx,
          worldY: drag.startWorld.y + worldDy,
          worldDx,
          worldDy,
          shiftKey: drag.shiftKey,
        });
        return;
      }

      if (drag.mode === 'link' && drag.nodeId) {
        const sp = screenPoint(e.clientX, e.clientY);
        const w = screenToWorld(viewportRef.current, sp.x, sp.y);
        onLinkDrag?.onMove(w);
      }
    };

    const onPointerUp = (e: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId) return;
      dragRef.current = null;
      setIsPanning(false);
      containerRef.current?.classList.remove('is-panning');

      // 落盘 pan 最终值
      if (pendingRef.current) {
        const p = pendingRef.current;
        pendingRef.current = null;
        viewportRef.current = p;
        applyDom(p);
        if (controlled) onViewportChange?.(p);
        else {
          setInnerViewport(p);
          onViewportChange?.(p);
        }
        setDisplayK(p.k);
      }

      const dx = e.clientX - drag.startX;
      const dy = e.clientY - drag.startY;
      const dragged = drag.moved || Math.hypot(dx, dy) > DRAG_CLICK_PX;

      if (drag.mode === 'link' && drag.nodeId) {
        const sp = screenPoint(e.clientX, e.clientY);
        const w = screenToWorld(viewportRef.current, sp.x, sp.y);
        const el = document.elementFromPoint(e.clientX, e.clientY);
        const nodeEl = el?.closest('[data-node-id]') as HTMLElement | null;
        const targetId = nodeEl?.dataset.nodeId ?? null;
        onLinkDrag?.onEnd(w, targetId, drag.nodeId);
        return;
      }

      if (!dragged) {
        if (drag.nodeId && (drag.mode === 'node' || drag.mode === 'pan')) {
          onNodeClick?.(drag.nodeId, {
            shiftKey: drag.shiftKey,
            clientX: e.clientX,
            clientY: e.clientY,
          });
        } else if (!drag.nodeId) {
          onBackgroundClick?.();
        }
      }
    };

    window.addEventListener('pointermove', onPointerMove);
    window.addEventListener('pointerup', onPointerUp);
    window.addEventListener('pointercancel', onPointerUp);
    return () => {
      window.removeEventListener('pointermove', onPointerMove);
      window.removeEventListener('pointerup', onPointerUp);
      window.removeEventListener('pointercancel', onPointerUp);
    };
  }, [
    applyDom,
    constrainViewport,
    controlled,
    onBackgroundClick,
    onLinkDrag,
    onNodeClick,
    onNodeDrag,
    onViewportChange,
    screenPoint,
  ]);

  const onDoubleClick = useCallback(
    (e: ReactMouseEvent<HTMLDivElement>) => {
      const target = e.target instanceof Element ? e.target : null;
      if (target?.closest('[data-canvas-no-zoom],[data-canvas-link-port]')) return;
      const nodeEl = target?.closest('[data-node-id]') as HTMLElement | null;
      const sp = screenPoint(e.clientX, e.clientY);
      const w = screenToWorld(viewportRef.current, sp.x, sp.y);
      if (nodeEl) {
        onDoubleClickNode?.(nodeEl.dataset.nodeId ?? '', e);
        return;
      }
      onCanvasDoubleClickWorld?.(w, e);
    },
    [onCanvasDoubleClickWorld, onDoubleClickNode, screenPoint],
  );

  const onContextMenu = useCallback(
    (e: ReactMouseEvent<HTMLDivElement>) => {
      // 屏蔽浏览器默认右键菜单，改由业务画布菜单接管（用户要求）。
      e.preventDefault();
      const target = e.target instanceof Element ? e.target : null;
      if (target?.closest('[data-canvas-no-zoom]')) {
        onContextMenuWorld?.(screenToWorld(viewportRef.current, 0, 0), e, {on: 'overlay'});
        return;
      }
      const nodeEl = target?.closest('[data-node-id]');
      const sp = screenPoint(e.clientX, e.clientY);
      const w = screenToWorld(viewportRef.current, sp.x, sp.y);
      if (nodeEl) {
        onContextMenuWorld?.(w, e, {on: 'node', nodeId: nodeEl.getAttribute('data-node-id') ?? ''});
        return;
      }
      onContextMenuWorld?.(w, e, {on: 'canvas'});
    },
    [onContextMenuWorld, screenPoint],
  );

  const worldClass =
    typeof worldClassName === 'function' ? worldClassName(displayK) : (worldClassName ?? 'mp-world');

  const current = viewportRef.current;

  return (
    <div
      ref={containerRef}
      className={`mp-canvas background-${background} ${isPanning ? 'is-panning' : ''} ${className ?? ''}`.trim()}
      data-testid={containerTestId}
      data-canvas-gesture={gesture}
      role="application"
      aria-label={ariaLabel}
      tabIndex={tabIndex}
      onKeyDown={localKeyDown}
      onKeyUp={onKeyUp}
      onPointerDown={onPointerDown}
      onDoubleClick={onDoubleClick}
      onContextMenu={onContextMenu}
    >
      <div ref={gridRef} className="mp-canvas-grid" aria-hidden="true" />
      <div
        ref={worldRef}
        className={worldClass}
        data-testid={worldTestId}
        style={{
          transform: `translate(${current.x}px, ${current.y}px) scale(${current.k})`,
          ...worldStyle,
        }}
      >
        {children}
      </div>
      {overlay}
      {showZoomBar ? (
        zoomBar ?? (
          <MpZoomBar
            k={displayK}
            min={kMin}
            max={kMax}
            onZoomBy={api.zoomBy}
            onZoomTo={k => api.zoomAtCenter(k)}
            onReset={api.reset}
            onFit={api.fit}
            className={zoomBarAlign === 'right' ? 'align-right' : undefined}
          />
        )
      ) : null}
    </div>
  );
}

export default MpCanvas;
export {MpZoomBar};
export type {Viewport, Point, Size, Bounds, CanvasGesture};
