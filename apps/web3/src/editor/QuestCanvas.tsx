import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import type {PointerEvent as RPointerEvent} from 'react';
import type {QuestEdge, QuestNode} from '../api/content';
import {Icon} from '../ui/Icon';
import {useContextMenu} from '../ui/ContextMenu';

/* 任务书画布 —— 与 FTB Quests 同一套交互：
   网格背景 / 节点自由摆放 / 拖拽移动 / 前置依赖连线（带箭头）/ 缩放平移 /
   空白处双击建节点 / 右键菜单。

   坐标口径：node.x / node.y 是「网格单位」（FTB 用的就是 doubles），不是像素。
   1 单位渲染成 UNIT px，再乘缩放。后端 `x REAL` 正是这个意思 ——
   之前 addNode 里写的 `(i%6)*110` 是像素残留，已一并改掉。

   滚轮口径与别处一致：滚轮平移画布，⌘/Ctrl+滚轮**放行**（留给浏览器缩放），
   缩放走右下角按钮和 +/-/0 键。滚轮永远不改变选中项 —— 选中只由点击产生。 */

const UNIT = 28;          // 1 网格单位 = 28px
const NODE = 2;           // 节点默认边长 = 2 单位（node.size 可覆盖）
const MIN_K = 0.3;
const MAX_K = 2.4;
const SNAP = 0.5;         // 拖拽吸附步长（单位）

type View = {x: number; y: number; k: number};
type Drag = {id: string; ox: number; oy: number; gx: number; gy: number; x: number; y: number};

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
const snap = (v: number) => Math.round(v / SNAP) * SNAP;
const sideOf = (n: QuestNode) => (n.size && n.size > 0 ? n.size : NODE) * UNIT;

/* 从矩形中心往目标点连线，落在边框上的那个点 —— 线才不会插进节点里。
   边长按节点自己的 size 传进来，大小不一的节点也算得对。 */
function borderPoint(cx: number, cy: number, tx: number, ty: number, half: number): [number, number] {
  const dx = tx - cx, dy = ty - cy;
  if (dx === 0 && dy === 0) return [cx, cy];
  const sx = dx === 0 ? Infinity : half / Math.abs(dx);
  const sy = dy === 0 ? Infinity : half / Math.abs(dy);
  const s = Math.min(sx, sy);
  return [cx + dx * s, cy + dy * s];
}

export function QuestCanvas({nodes, edges, selectedId, scopeKey, colorOf, iconOf,
  onSelect, onMove, onConnect, onCreate, onDelete}: {
  nodes: QuestNode[];
  edges: QuestEdge[];
  selectedId: string | null;
  /** 画布范围标识（当前章节 id，或全部章节用的常量）。只有它变才重新适应窗口 ——
      否则加一个节点、拖一下都会跳视图。 */
  scopeKey: string;
  colorOf: (node: QuestNode) => string;
  iconOf: (node: QuestNode) => string | null;
  onSelect: (id: string | null) => void;
  onMove: (id: string, x: number, y: number) => void;
  onConnect: (from: string, to: string) => void;
  onCreate: (x: number, y: number) => void;
  onDelete: (id: string) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<View>({x: 0, y: 0, k: 1});
  /* 拖拽中的实时位置。不直接写回 draft：拖动连发几十个 pointermove，
     每次都 setDraft 会把整棵树（含所有图标）重渲染一遍。松手才 commit。 */
  const [drag, setDrag] = useState<Drag | null>(null);
  const [link, setLink] = useState<{from: string; x: number; y: number} | null>(null);
  const [pan, setPan] = useState<{sx: number; sy: number; vx: number; vy: number} | null>(null);
  /* 右键菜单走 ui/ContextMenu：坐标、Esc、出视口翻转都在组件里，这里只给条目。 */
  const menu = useContextMenu();

  const byId = useMemo(() => new Map(nodes.map(n => [n.id, n])), [nodes]);

  /* ---- 坐标换算 ---- */
  const toWorld = useCallback((clientX: number, clientY: number, v: View): [number, number] => {
    const r = host.current?.getBoundingClientRect();
    return [((clientX - (r?.left ?? 0)) - v.x) / (UNIT * v.k),
      ((clientY - (r?.top ?? 0)) - v.y) / (UNIT * v.k)];
  }, []);

  /* ---- 适应窗口 ---- */
  const fit = useCallback(() => {
    const el = host.current;
    if (!el) return;
    const w = el.clientWidth, h = el.clientHeight;
    if (nodes.length === 0) { setView({x: w / 2, y: h / 2, k: 1}); return; }
    const pad = 1.5;
    const size = (n: QuestNode) => (n.size && n.size > 0 ? n.size : NODE);
    const x0 = Math.min(...nodes.map(n => n.x)) - pad;
    const y0 = Math.min(...nodes.map(n => n.y)) - pad;
    const x1 = Math.max(...nodes.map(n => n.x + size(n))) + pad;
    const y1 = Math.max(...nodes.map(n => n.y + size(n))) + pad;
    const k = clamp(Math.min(w / ((x1 - x0) * UNIT), h / ((y1 - y0) * UNIT)), MIN_K, 1.4);
    setView({
      k,
      x: (w - (x1 - x0) * UNIT * k) / 2 - x0 * UNIT * k,
      y: (h - (y1 - y0) * UNIT * k) / 2 - y0 * UNIT * k,
    });
  }, [nodes]);

  /* fit 依赖 nodes，直接放进 effect 依赖里就会「每次改草稿都跳视图」。
     这里只让 scopeKey 变化触发重适应，实际要跑的 fit 从 ref 取最新的。 */
  const fitRef = useRef(fit);
  fitRef.current = fit;
  const hadNodes = useRef(false);
  useLayoutEffect(() => {
    if (nodes.length > 0 && !hadNodes.current) fitRef.current();   // 数据异步到达
    hadNodes.current = nodes.length > 0;
  }, [nodes.length]);
  useEffect(() => {
    hadNodes.current = false;                                      // 换章节：允许再适应一次
    fitRef.current();
  }, [scopeKey]);

  /* ---- 滚轮：平移画布。⌘/Ctrl 放行给浏览器。必须原生 + passive:false。 ---- */
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) return;                          // 放行，留给浏览器缩放
      e.preventDefault();
      const shifted = e.shiftKey && e.deltaX === 0;
      const dx = shifted ? e.deltaY : e.deltaX;
      const dy = shifted ? 0 : e.deltaY;
      setView(v => ({...v, x: v.x - dx, y: v.y - dy}));
    };
    el.addEventListener('wheel', onWheel, {passive: false});
    return () => el.removeEventListener('wheel', onWheel);
  }, []);

  /* ---- 缩放：围绕给定屏幕点（不传则视口中心） ---- */
  const zoomAt = useCallback((factor: number, clientX?: number, clientY?: number) => {
    const r = host.current?.getBoundingClientRect();
    const px = clientX === undefined || !r ? (r?.width ?? 0) / 2 : clientX - r.left;
    const py = clientY === undefined || !r ? (r?.height ?? 0) / 2 : clientY - r.top;
    setView(v => {
      const k = clamp(v.k * factor, MIN_K, MAX_K);
      const f = k / v.k;
      return {k, x: px - (px - v.x) * f, y: py - (py - v.y) * f};
    });
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
      if (e.key === '+' || e.key === '=') { e.preventDefault(); zoomAt(1.2); }
      else if (e.key === '-' || e.key === '_') { e.preventDefault(); zoomAt(1 / 1.2); }
      else if (e.key === '0') { e.preventDefault(); fitRef.current(); }
      else if ((e.key === 'Delete' || e.key === 'Backspace') && selectedId) {
        /* 选中即删。检查器的标题输入框已不再 autoFocus（见 ModeQuest），
           所以选中任务后焦点通常落在画布上，这里收得到。 */
        e.preventDefault();
        onDelete(selectedId);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [zoomAt, selectedId, onDelete]);

  /* ---- 指针 ---- */
  const onHostDown = (e: RPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0 && e.button !== 1) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    setPan({sx: e.clientX, sy: e.clientY, vx: view.x, vy: view.y});
    if (e.button === 0) onSelect(null);
  };
  const onHostMove = (e: RPointerEvent<HTMLDivElement>) => {
    if (pan) {
      setView(v => ({...v, x: pan.vx + (e.clientX - pan.sx), y: pan.vy + (e.clientY - pan.sy)}));
    }
    if (link) {
      const [wx, wy] = toWorld(e.clientX, e.clientY, view);
      setLink(l => (l ? {...l, x: wx, y: wy} : l));
    }
  };
  const onHostUp = (e: RPointerEvent<HTMLDivElement>) => {
    if (link) {
      /* pointerup 的 target 是捕获元素（宿主），拿不到下面那层 —— 用 elementFromPoint 找落点。 */
      const hit = document.elementFromPoint(e.clientX, e.clientY)?.closest('[data-qnode]');
      const to = hit?.getAttribute('data-qnode');
      if (to && to !== link.from) onConnect(link.from, to);
      setLink(null);
    }
    setPan(null);
  };

  const startDrag = (e: RPointerEvent<HTMLDivElement>, n: QuestNode) => {
    if (e.button !== 0) return;
    e.stopPropagation();
    /* Shift 点另一个节点 = 以当前选中项为前置连过去，省得去拖那个小圆点。 */
    if (e.shiftKey && selectedId && selectedId !== n.id) { onConnect(selectedId, n.id); return; }
    const [gx, gy] = toWorld(e.clientX, e.clientY, view);
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    onSelect(n.id);
    setDrag({id: n.id, ox: n.x, oy: n.y, gx, gy, x: n.x, y: n.y});
  };
  const moveDrag = (e: RPointerEvent<HTMLDivElement>) => {
    if (!drag) return;
    const [wx, wy] = toWorld(e.clientX, e.clientY, view);
    /* 用「按下时的世界坐标」当锚点算位移，指针在节点内的抓取偏移就自然抵消了；
       直接拿当前世界坐标当节点位置会让节点跳到指针中心。 */
    setDrag(d => (d ? {...d, x: snap(d.ox + (wx - d.gx)), y: snap(d.oy + (wy - d.gy))} : d));
  };
  const endDrag = () => {
    if (drag) {
      const n = byId.get(drag.id);
      if (n && (n.x !== drag.x || n.y !== drag.y)) onMove(drag.id, drag.x, drag.y);
    }
    setDrag(null);
  };

  const posOf = (n: QuestNode) => {
    const local = drag && drag.id === n.id ? drag : n;
    return {left: local.x * UNIT, top: local.y * UNIT};
  };

  const selected = selectedId ? byId.get(selectedId) : null;
  const empty = nodes.length === 0;

  return (
    <div className={`qc-host${empty ? ' qc-empty' : ''}`} ref={host}
      style={{
        '--q-unit': `${UNIT * view.k}px`,
        backgroundPosition: `${view.x}px ${view.y}px`,
      } as React.CSSProperties}
      onPointerDown={onHostDown}
      onPointerMove={onHostMove}
      onPointerUp={onHostUp}
      onPointerCancel={onHostUp}
      onDoubleClick={e => {
        if ((e.target as HTMLElement).closest('[data-qnode]')) return;
        const [wx, wy] = toWorld(e.clientX, e.clientY, view);
        onCreate(snap(wx), snap(wy));
      }}
      onContextMenu={e => {
        /* 节点上的右键由节点自己接（下面 stopPropagation），这里让给它。 */
        if ((e.target as HTMLElement).closest('[data-qnode]')) return;
        const [wx, wy] = toWorld(e.clientX, e.clientY, view);
        menu.open(e, [
          {label: '在这里新建任务', icon: 'plus', action: () => onCreate(snap(wx), snap(wy))},
          {separator: true},
          {label: '适应窗口', action: () => fitRef.current()},
          {label: '回到 100%', action: () => setView(v => ({...v, k: 1}))},
        ]);
      }}>

      {empty && (
        <div className="qc-empty-box">
          <Icon name="quest" size={20}/>
          <div>这一章还没有任务。<b>空白处双击</b>就能建一个，右键也能调出菜单。</div>
          <div className="sub">
            连前置：拖节点右缘的小圆点到另一个节点上；或先选中上游节点，再 Shift 点下游节点。
          </div>
        </div>
      )}

      {/* 内容层：整体平移 + 缩放。节点与连线都在这一层，缩放时一起变。 */}
      <div className="qc-layer" style={{transform: `translate(${view.x}px, ${view.y}px) scale(${view.k})`}}>
        <svg className="qc-wires" aria-hidden>
          <defs>
            <marker id="qc-arrow" viewBox="0 0 8 8" refX="6.5" refY="4"
              markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
              <path d="M0.5 0.8 7.5 4 0.5 7.2z" className="qc-arrow-off"/>
            </marker>
            <marker id="qc-arrow-on" viewBox="0 0 8 8" refX="6.5" refY="4"
              markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
              <path d="M0.5 0.8 7.5 4 0.5 7.2z" className="qc-arrow-on-fill"/>
            </marker>
          </defs>
          {edges.map(e => {
            const a = byId.get(e.fromNodeId), b = byId.get(e.toNodeId);
            if (!a || !b) return null;
            const pa = posOf(a), pb = posOf(b);
            const sa = sideOf(a), sb = sideOf(b);
            const acx = pa.left + sa / 2, acy = pa.top + sa / 2;
            const bcx = pb.left + sb / 2, bcy = pb.top + sb / 2;
            const [sx, sy] = borderPoint(acx, acy, bcx, bcy, sa / 2);
            const [tx, ty] = borderPoint(bcx, bcy, acx, acy, sb / 2);
            const on = selectedId === e.fromNodeId || selectedId === e.toNodeId;
            return <line key={e.id} x1={sx} y1={sy} x2={tx} y2={ty}
              className={`qc-wire${on ? ' on' : ''}`}
              markerEnd={`url(#${on ? 'qc-arrow-on' : 'qc-arrow'})`}/>;
          })}
          {link && (() => {
            const a = byId.get(link.from);
            if (!a) return null;
            const p = posOf(a), sa = sideOf(a);
            /* 终点就是指针的世界坐标（x2/y2 直接用 link 像素）——
               之前多加了一个 sa/2（半个节点边长），虚线终点漂在鼠标右下，
               偏移量还随源节点尺寸变（size2 时正好 28px），就是从这里抄残的。 */
            return <line x1={p.left + sa / 2} y1={p.top + sa / 2}
              x2={link.x * UNIT} y2={link.y * UNIT} className="qc-wire draft"/>;
          })()}
        </svg>

        {nodes.map(n => {
          const p = posOf(n);
          const icon = iconOf(n);
          const on = selectedId === n.id;
          const hasParent = edges.some(e => e.toNodeId === n.id);
          const hasChild = edges.some(e => e.fromNodeId === n.id);
          return (
            <div key={n.id} data-qnode={n.id}
              className={`qc-node${on ? ' on' : ''}${n.invisible ? ' faded' : ''}${drag?.id === n.id ? ' dragging' : ''}`}
              style={{
                left: p.left, top: p.top, width: sideOf(n), height: sideOf(n),
                borderColor: colorOf(n),
                borderRadius: n.shape === 'square' ? 3 : n.shape === 'circle' ? '50%' : 8,
              }}
              title={`${n.title || '(未命名)'}\n拖动移动 · 右键更多 · 右缘圆点拖出来连前置`}
              onPointerDown={e => startDrag(e, n)}
              onPointerMove={moveDrag}
              onPointerUp={endDrag}
              onPointerCancel={endDrag}
              onContextMenu={e => menu.open(e, [
                {label: '编辑这个任务', icon: 'wrench', action: () => onSelect(n.id)},
                {separator: true},
                {label: '删除任务', icon: 'trash', danger: true, action: () => onDelete(n.id)},
              ])}>
              {hasParent && <span className="qc-mark in"/>}
              {hasChild && <span className="qc-mark out"/>}
              {icon
                ? <img src={icon} alt="" draggable={false}/>
                /* 没设图标就给个书签占位，别拿标题前两字当占位 ——
                   标题在下方标签里已经有了，「新任」「验收」读起来像被截断的词。 */
                : <span className="qc-node-ph"><Icon name="quest" size={20}/></span>}
              {n.optional && <span className="qc-node-badge" title="可选任务">?</span>}
              <span className="qc-node-label">{n.title || '(未命名)'}</span>
              <span className="qc-link" title="拖到另一个任务上，建立前置关系"
                onPointerDown={e => {
                  e.stopPropagation(); e.preventDefault();
                  /* 抓住指针：拖出画布边缘时 onHostMove/onHostUp 靠冒泡收
                     pointermove/up —— 不抓的话出界即断，虚线永久卡住。 */
                  (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
                  const [wx, wy] = toWorld(e.clientX, e.clientY, view);
                  setLink({from: n.id, x: wx, y: wy});
                }}/>
            </div>
          );
        })}
      </div>

      <div className="qc-zoom" onPointerDown={e => e.stopPropagation()}>
        <button type="button" className="p-btn" title="缩小（-）" onClick={() => zoomAt(1 / 1.2)}>−</button>
        <span className="qc-zoom-val">{Math.round(view.k * 100)}%</span>
        <button type="button" className="p-btn" title="放大（+）" onClick={() => zoomAt(1.2)}>+</button>
        <button type="button" className="p-btn" title="适应窗口（0）" onClick={() => fitRef.current()}>适应</button>
        <button type="button" className="p-btn" title="回到 100%" onClick={() => setView(v => ({...v, k: 1}))}>1:1</button>
      </div>

      {!empty && (
        <div className="qc-hud">
          {selected
            ? `已选中：${selected.title || '(未命名)'}`
            : '空白处双击新建 · 拖节点移动 · 拖右缘圆点连前置'}
        </div>
      )}

      {menu.menu}
    </div>
  );
}
