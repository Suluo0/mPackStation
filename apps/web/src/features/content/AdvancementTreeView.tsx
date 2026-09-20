import {
  useCallback, useEffect, useMemo, useRef, useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import {
  LABEL_HIDE_K,
  K_DEFAULT,
  K_MAX,
  K_MIN,
  NODE_SIZE,
  PAN_MARGIN,
  clamp,
  computePanBounds,
  describeRequirements,
  edgePath,
  fitTransform,
  formatRewardsSummary,
  layoutAdvancement,
  parentChain,
  resetTransform,
  type AdvNode,
  type AdvTree,
  type LayoutNode,
  type ViewTransform,
} from './advLayout';
import {MpCanvas, MpZoomBar, type MpCanvasApi, type Viewport} from '../../ui/canvas';
import './AdvancementTreeView.css';

export type {AdvNode, AdvTree};

export type AdvancementTreeViewProps = {
  trees: AdvTree[];
  getIcon?: (itemId: string) => string | null;
  onSelect?: (node: AdvNode | null) => void;
  className?: string;
};

type HoverState = {
  node: LayoutNode;
  x: number;
  y: number;
} | null;

function frameClass(frame?: AdvNode['frame']): string {
  return frame ? `frame-${frame}` : 'frame-task';
}

function frameLabel(frame?: AdvNode['frame']): string {
  return frame ?? 'task';
}

function frameColor(frame?: AdvNode['frame']): string {
  if (frame === 'goal') return '#d4a84b';
  if (frame === 'challenge') return '#c9a0e8';
  return '#e8e2d4';
}

function NodeIcon({node, getIcon}: {node: AdvNode; getIcon?: (id: string) => string | null}) {
  const url = node.iconUrl || (node.iconItem && getIcon ? getIcon(node.iconItem) : null);
  if (url) return <img className="adv-node-icon" src={url} alt="" draggable={false}/>;
  return <span className="adv-node-icon placeholder" aria-hidden="true">{(node.title || node.id).slice(0, 1)}</span>;
}

function HoverCard({state}: {state: NonNullable<HoverState>}) {
  const n = state.node;
  const req = describeRequirements(n.requirements);
  const rewards = formatRewardsSummary(n.rewards);
  return (
    <div
      className="adv-hover-card"
      data-testid="adv-hover-card"
      style={{left: state.x, top: state.y}}
      role="tooltip"
    >
      <div className="hover-title" style={{color: frameColor(n.frame)}}>{n.title}</div>
      {n.description ? <div className="hover-desc">{n.description}</div> : null}
      <div className="hover-meta">
        <span className={`hover-frame ${frameClass(n.frame)}`}>{frameLabel(n.frame)}</span>
        {n.done ? <span className="hover-done">演示完成</span> : null}
      </div>
      {req.mode !== 'empty' ? (
        <div className="hover-req" data-testid="adv-hover-requirements">
          <span className="hover-label">条件</span> {req.label}
        </div>
      ) : null}
      {rewards ? (
        <div className="hover-rewards" data-testid="adv-hover-rewards">
          <span className="hover-label">奖励</span> {rewards}
        </div>
      ) : null}
      {(n.title || n.id) && <div className="hover-id"><code>{n.id}</code></div>}
    </div>
  );
}

function Inspector({node, parentOf}: {node: AdvNode; parentOf: Map<string, string | null>}) {
  const chain = parentChain(node.id, parentOf);
  const req = describeRequirements(node.requirements);
  const rewards = node.rewards;
  const display = node.display;
  return (
    <aside className="adv-tree-inspector" data-testid="adv-tree-inspector" aria-label="节点详情">
      <h3>Inspector</h3>
      <div className="insp-title">{node.title}</div>
      <div className="insp-id"><code>{node.id}</code></div>
      <dl>
        <div>
          <dt>类型 frame</dt>
          <dd data-testid="insp-frame">{frameLabel(node.frame)}</dd>
        </div>
        {node.parentId ? (
          <div>
            <dt>parent</dt>
            <dd><code>{node.parentId}</code></dd>
          </div>
        ) : null}
        {node.description ? <div><dt>说明</dt><dd>{node.description}</dd></div> : null}
        <div>
          <dt>requirements</dt>
          <dd data-testid="insp-requirements">
            <div className="insp-req-label">{req.label}</div>
            {req.groups.length > 0 && (
              <ul className="insp-list">
                {req.groups.map((g, i) => <li key={`${i}-${g}`}>{g}</li>)}
              </ul>
            )}
            {node.requirements && node.requirements.length > 0 && (
              <div className="insp-req-raw muted">
                JSON: <code>{JSON.stringify(node.requirements)}</code>
              </div>
            )}
          </dd>
        </div>
        {node.criteria && node.criteria.length > 0 ? (
          <div>
            <dt>criteria</dt>
            <dd>
              <ul className="insp-list">
                {node.criteria.map(c => <li key={c}>{c}</li>)}
              </ul>
            </dd>
          </div>
        ) : null}
        <div>
          <dt>rewards</dt>
          <dd data-testid="insp-rewards">
            {rewards ? (
              <ul className="insp-list">
                {typeof rewards.experience === 'number' && (
                  <li key="xp">经验：<code>{rewards.experience}</code></li>
                )}
                {rewards.recipes?.length ? (
                  <li key="recipes">
                    解锁配方：
                    <ul className="insp-list">
                      {rewards.recipes.map(r => <li key={r}><code>{r}</code></li>)}
                    </ul>
                  </li>
                ) : null}
                {rewards.loot?.length ? (
                  <li key="loot">
                    战利品：
                    <ul className="insp-list">
                      {rewards.loot.map(r => <li key={r}><code>{r}</code></li>)}
                    </ul>
                  </li>
                ) : null}
                {rewards.function ? (
                  <li key="fn">函数：<code>{rewards.function}</code></li>
                ) : null}
              </ul>
            ) : (
              <span className="muted">（无）</span>
            )}
          </dd>
        </div>
        <div>
          <dt>display 元数据</dt>
          <dd data-testid="insp-display">
            {node.hasDisplay || display ? (
              <ul className="insp-list">
                <li>display：{node.hasDisplay ? '有' : '无'}</li>
                <li>hidden：{display?.hidden == null ? '—' : String(display.hidden)}</li>
                <li>show_toast：{display?.showToast == null ? '—' : String(display.showToast)}</li>
                <li>announce_to_chat：{display?.announceToChat == null ? '—' : String(display.announceToChat)}</li>
                <li>background：{display?.background ? <code>{display.background}</code> : '—'}</li>
              </ul>
            ) : (
              <span className="muted">（无 display，不进 Tab）</span>
            )}
          </dd>
        </div>
        <div>
          <dt>父链</dt>
          <dd>
            {chain.length === 0 ? (
              <span className="muted">（根节点）</span>
            ) : (
              <ol className="insp-chain" data-testid="insp-chain">
                {chain.map(id => <li key={id}><code>{id}</code></li>)}
              </ol>
            )}
          </dd>
        </div>
      </dl>
    </aside>
  );
}

export function AdvancementTreeView({trees, getIcon, onSelect, className}: AdvancementTreeViewProps) {
  const canvasApiRef = useRef<MpCanvasApi | null>(null);
  const [viewport, setViewport] = useState<Viewport>({x: 0, y: 0, k: K_DEFAULT});

  const firstTabId = useMemo(
    () => trees.find(t => t.kind === 'tab')?.id ?? trees.find(t => t.kind !== 'recipes')?.id ?? trees[0]?.id ?? '',
    [trees],
  );
  const [treeId, setTreeId] = useState<string>(() => firstTabId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [toolbarK, setToolbarK] = useState<number>(K_DEFAULT);
  const [hover, setHover] = useState<HoverState>(null);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {};
    for (const t of trees) {
      if (t.kind === 'recipes') init[t.id] = true;
    }
    return init;
  });

  const activeTree = useMemo(
    () => trees.find(t => t.id === treeId) ?? trees.find(t => t.id === firstTabId) ?? trees[0] ?? null,
    [trees, treeId, firstTabId],
  );

  const layout = useMemo(
    () => (activeTree ? layoutAdvancement(activeTree.nodes) : null),
    [activeTree],
  );

  const selected = useMemo(() => {
    if (!layout || !selectedId) return null;
    return layout.nodeById.get(selectedId) ?? null;
  }, [layout, selectedId]);

  const demoDoneCount = useMemo(
    () => layout?.nodes.filter(n => n.done === true).length ?? 0,
    [layout],
  );
  const hasDemoDone = demoDoneCount > 0;

  const toMp = useCallback((v: ViewTransform): Viewport => ({x: v.tx, y: v.ty, k: v.k}), []);
  const toAdv = useCallback((v: Viewport): ViewTransform => ({k: v.k, tx: v.x, ty: v.y}), []);

  const constrainViewport = useCallback((v: Viewport): Viewport => {
    if (!layout) return v;
    const {width, height} = canvasApiRef.current?.getSize() ?? {width: 800, height: 500};
    const pan = computePanBounds(layout.bounds, toAdv(v), width, height, PAN_MARGIN);
    return {
      k: v.k,
      x: clamp(v.x, pan.minTx, pan.maxTx),
      y: clamp(v.y, pan.minTy, pan.maxTy),
    };
  }, [layout, toAdv]);

  const applyAdvView = useCallback((next: ViewTransform) => {
    const s = canvasApiRef.current?.getSize() ?? {width: 800, height: 500};
    void s;
    const mp = constrainViewport(toMp(next));
    setViewport(mp);
    setToolbarK(mp.k);
    canvasApiRef.current?.setViewport(mp);
  }, [constrainViewport, toMp]);

  const handleFit = useCallback(() => {
    if (!layout) return;
    const s = canvasApiRef.current?.getSize() ?? {width: 800, height: 500};
    applyAdvView(fitTransform(layout.bounds, s.width, s.height));
  }, [layout, applyAdvView]);

  const handleReset = useCallback(() => {
    if (!layout) return;
    const s = canvasApiRef.current?.getSize() ?? {width: 800, height: 500};
    applyAdvView(resetTransform(layout.bounds, s.width, s.height));
  }, [layout, applyAdvView]);

  const zoomAt = useCallback((factor: number) => {
    const api = canvasApiRef.current;
    if (!api) return;
    const s = api.getSize();
    const v = api.getViewport();
    const ax = s.width / 2;
    const ay = s.height / 2;
    const kNext = clamp(v.k * factor, K_MIN, K_MAX);
    const w = api.screenToWorld(ax, ay);
    applyAdvView({k: kNext, tx: ax - w.x * kNext, ty: ay - w.y * kNext});
  }, [applyAdvView]);

  const selectNode = useCallback((node: AdvNode | null) => {
    setSelectedId(node?.id ?? null);
    onSelect?.(node);
  }, [onSelect]);

  useEffect(() => {
    if (!trees.length) {
      setSelectedId(null);
      return;
    }
    if (!trees.some(t => t.id === treeId)) setTreeId(firstTabId || trees[0].id);
  }, [trees, treeId, firstTabId]);

  useEffect(() => {
    setSelectedId(null);
    setHover(null);
    onSelect?.(null);
  }, [activeTree?.id, onSelect]);

  useEffect(() => {
    if (!layout) return;
    const s = canvasApiRef.current?.getSize() ?? {width: 800, height: 500};
    applyAdvView(fitTransform(layout.bounds, s.width, s.height));
  }, [layout, applyAdvView]);

  const onKeyDown = useCallback((e: ReactKeyboardEvent) => {
    const api = canvasApiRef.current;
    if (!api) return;
    const v = api.getViewport();
    const step = 24;
    if (e.key === 'ArrowLeft') { e.preventDefault(); applyAdvView({k: v.k, tx: v.x + step, ty: v.y}); return; }
    if (e.key === 'ArrowRight') { e.preventDefault(); applyAdvView({k: v.k, tx: v.x - step, ty: v.y}); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); applyAdvView({k: v.k, tx: v.x, ty: v.y + step}); return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); applyAdvView({k: v.k, tx: v.x, ty: v.y - step}); }
  }, [applyAdvView]);

  const onNodeClick = useCallback((nodeId: string) => {
    if (!layout) return;
    selectNode(layout.nodeById.get(nodeId) ?? null);
  }, [layout, selectNode]);

  const onBackgroundClick = useCallback(() => {
    selectNode(null);
  }, [selectNode]);

  const onDoubleClickNode = useCallback((nodeId: string) => {
    if (!layout) return;
    const node = layout.nodeById.get(nodeId);
    if (!node) return;
    const api = canvasApiRef.current;
    const s = api?.getSize() ?? {width: 800, height: 500};
    const kNext = clamp((api?.getViewport().k ?? K_DEFAULT) * 1.5, K_MIN, K_MAX);
    applyAdvView({k: kNext, tx: s.width / 2 - node.x * kNext, ty: s.height / 2 - node.y * kNext});
    selectNode(node);
  }, [layout, applyAdvView, selectNode]);

  const onNodeEnter = useCallback((e: ReactPointerEvent, node: LayoutNode) => {
    const parent = e.currentTarget.closest('.mp-canvas') as HTMLElement | null;
    const pr = parent?.getBoundingClientRect();
    if (!pr) return;
    setHover({node, x: e.clientX - pr.left + 14, y: e.clientY - pr.top + 14});
  }, []);

  const onNodeLeave = useCallback(() => setHover(null), []);

  if (!trees.length) {
    return (
      <div className={`adv-tree-empty ${className ?? ''}`.trim()}>
        该包暂无进度数据，先解析模组
      </div>
    );
  }

  const cycleBreaks = Math.max(
    layout?.cycleBreaks ?? 0,
    ...trees.map(t => t.cycleBreaks ?? 0),
  );
  const playerTabs = trees.filter(t => t.kind === 'tab');
  const secondaryTrees = trees.filter(t => t.kind !== 'tab');
  const showPicker = playerTabs.length + secondaryTrees.length > 1;

  return (
    <div className={`adv-tree-root ${className ?? ''}`.trim()}>
      <div className="adv-tree-topbar">
        {showPicker && (
          <div className="adv-tree-picker" role="tablist" aria-label="进度 Tab（原版：有 display 的 root）">
            {playerTabs.map(t => (
              <button
                key={t.id}
                type="button"
                role="tab"
                data-testid={`adv-tab-${t.id}`}
                aria-selected={t.id === activeTree?.id}
                className={`adv-tab ${t.id === activeTree?.id ? 'active' : ''}`}
                onClick={() => setTreeId(t.id)}
              >
                {t.icon ? (
                  <img className="tab-icon" src={t.icon} alt="" draggable={false}/>
                ) : (
                  <span className="tab-icon placeholder" aria-hidden="true">{(t.label || '?').slice(0, 1)}</span>
                )}
                <span className="tab-label">{t.label}</span>
                <span className="count">{t.nodes.length}</span>
              </button>
            ))}
          </div>
        )}
        <div className="adv-tree-legend" data-testid="adv-tree-legend">
          {hasDemoDone ? (
            <span className="legend-demo">
              演示完成态 · 非玩家存档
              <span className="legend-swatch done"/>已完成
              <span className="legend-swatch pending"/>未完成
            </span>
          ) : (
            <span className="legend-preview" data-testid="adv-tree-preview-note">
              结构预览 · 非玩家存档（无 done 不画完成态）
            </span>
          )}
          <span className="legend-gesture">
            拖动/滚轮=平移 · <strong>缩放：工作台增强</strong>（Ctrl+滚轮）
          </span>
        </div>
      </div>

      {secondaryTrees.length > 0 && (
        <div className="adv-tree-secondary" data-testid="adv-tree-secondary">
          {secondaryTrees.map(t => {
            const isCollapsed = collapsed[t.id] ?? t.kind === 'recipes';
            const isActive = t.id === activeTree?.id;
            return (
              <div key={t.id} className={`secondary-group kind-${t.kind}`}>
                <button
                  type="button"
                  className={`secondary-toggle ${isActive ? 'active' : ''}`}
                  data-testid={`adv-secondary-${t.id}`}
                  aria-expanded={!isCollapsed}
                  onClick={() => {
                    setCollapsed(prev => ({...prev, [t.id]: !(prev[t.id] ?? t.kind === 'recipes')}));
                    if (isCollapsed || !isActive) setTreeId(t.id);
                  }}
                >
                  <span className="chevron">{isCollapsed ? '▸' : '▾'}</span>
                  <span className="secondary-label">{t.label}</span>
                  <span className="count">{t.nodes.length}</span>
                  {t.kind === 'recipes' ? <span className="secondary-hint">默认不进玩家树</span> : null}
                </button>
                {!isCollapsed && (
                  <div className="secondary-body">
                    <button
                      type="button"
                      className={`secondary-open ${isActive ? 'active' : ''}`}
                      onClick={() => setTreeId(t.id)}
                    >
                      在画布中查看
                    </button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {cycleBreaks > 0 && (
        <div className="adv-tree-warn" role="status">已打断 {cycleBreaks} 处循环引用</div>
      )}
      <div className="adv-tree-main">
        <MpCanvas
          className="adv-tree-canvas"
          containerTestId="adv-tree-canvas"
          worldClassName={k => `adv-tree-world mp-world ${k < LABEL_HIDE_K ? 'hide-labels' : ''}`}
          worldTestId="adv-tree-world"
          gesture="vanillaAdv"
          tool="pan"
          background="blank"
          minZoom={K_MIN}
          maxZoom={K_MAX}
          fitMinZoom={0.08}
          viewport={viewport}
          onViewportChange={v => {
            setViewport(v);
            setToolbarK(v.k);
          }}
          constrainViewport={constrainViewport}
          getContentBounds={() => (layout ? {
            minX: layout.bounds.minX,
            minY: layout.bounds.minY,
            maxX: layout.bounds.maxX,
            maxY: layout.bounds.maxY,
          } : null)}
          nodeDraggable={false}
          ariaLabel="进度树画布：拖动/滚轮平移（有界），Ctrl+滚轮缩放（工作台增强），方向键平移，+/- 缩放，0 重置"
          expose={api => { canvasApiRef.current = api; }}
          onNodeClick={onNodeClick}
          onBackgroundClick={onBackgroundClick}
          onDoubleClickNode={onDoubleClickNode}
          onKeyDown={onKeyDown}
          onResetOverride={handleReset}
          onFitOverride={handleFit}
          zoomBar={
            <MpZoomBar
              k={toolbarK}
              min={K_MIN}
              max={K_MAX}
              className="adv-tree-zoombar align-right"
              testId="adv-tree-zoombar"
              percentTestId="adv-tree-zoom-percent"
              btnTestIds={{
                zoomIn: 'adv-tree-btn-zoom-in',
                zoomOut: 'adv-tree-btn-zoom-out',
                fit: 'adv-tree-btn-fit',
                reset: 'adv-tree-btn-reset',
                slider: 'adv-tree-zoom-slider',
              }}
              leftSlot={
                <span className="zoom-enhance" data-testid="adv-tree-zoom-enhance" title="原版进度界面无缩放，此为 Web 工作台增强">
                  缩放：工作台增强
                </span>
              }
              onZoomBy={f => zoomAt(f)}
              onZoomTo={k => {
                const cur = canvasApiRef.current?.getViewport().k ?? toolbarK;
                zoomAt(k / Math.max(cur, 0.001));
              }}
              onReset={handleReset}
              onFit={handleFit}
            />
          }
          overlay={hover ? <HoverCard state={hover}/> : null}
        >
          {layout && (
            <svg className="adv-tree-edges" width={1} height={1} style={{overflow: 'visible'}} aria-hidden="true">
              {layout.edges.map(edge => {
                const p = layout.nodeById.get(edge.parentId);
                const c = layout.nodeById.get(edge.childId);
                if (!p || !c) return null;
                const related = selectedId && (edge.parentId === selectedId || edge.childId === selectedId);
                return (
                  <path
                    key={`${edge.parentId}->${edge.childId}`}
                    d={edgePath(p, c)}
                    className={related ? 'edge related' : 'edge'}
                  />
                );
              })}
            </svg>
          )}
          {layout?.nodes.map((n: LayoutNode) => {
            const isRoot = n.depth === 0;
            return (
              <button
                key={n.id}
                type="button"
                data-node-id={n.id}
                data-testid="adv-tree-node"
                className={[
                  'adv-node',
                  frameClass(n.frame),
                  selectedId === n.id ? 'selected' : '',
                  isRoot ? 'is-root' : '',
                  hasDemoDone ? (n.done ? 'done' : 'pending-demo') : '',
                ].filter(Boolean).join(' ')}
                style={{
                  left: n.x - NODE_SIZE / 2,
                  top: n.y - NODE_SIZE / 2,
                  width: NODE_SIZE,
                  height: NODE_SIZE,
                }}
                title={n.title}
                onPointerEnter={e => onNodeEnter(e, n)}
                onPointerLeave={onNodeLeave}
              >
                <NodeIcon node={n} getIcon={getIcon}/>
                <span className="adv-node-label">{n.title}</span>
              </button>
            );
          })}
        </MpCanvas>
        {selected ? (
          <Inspector node={selected} parentOf={layout!.parentOf}/>
        ) : (
          <aside className="adv-tree-inspector empty" data-testid="adv-tree-inspector-empty">
            <h3>Inspector</h3>
            <p className="muted">点击画布中的进度节点，查看 requirements / rewards / display 元数据与父链。</p>
          </aside>
        )}
      </div>
    </div>
  );
}

export default AdvancementTreeView;
