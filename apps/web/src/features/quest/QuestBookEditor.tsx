import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {App, Button, Divider, Input, InputNumber, Select, Switch, Tag} from 'antd';
import {
  CheckCircleFilled, EyeOutlined, LockOutlined, PlusOutlined, ThunderboltOutlined,
} from '@ant-design/icons';
import {
  QUEST_NODE_H, QUEST_NODE_W, QUEST_REWARD_KINDS, QUEST_SHAPES, QUEST_TASK_TYPES,
  canAddEdge, createQuestNode, defaultNodeFields, draftWithSyncedPrerequisites, edgePath,
  ensureNodeCoords, findCycle, gridX, gridY, nodeLabel, normalizeDraft, prerequisitesOf,
  removeEdge, removePrerequisite, snapNodePosition, wouldCreateCycle,
  type QuestDraftGraph, type QuestEdgeDraft, type QuestNodeDraft, type QuestRewardDraft, type QuestTaskDraft,
} from './questGraph';
import {
  DEP_UI_OPTIONS, depPayloadToUi, depUiToPayload, evaluateDependencies,
  type DepUiMode,
} from './questDeps';
import {QUEST_ICON_CATALOG, iconDisplay, iconUrlOrNull} from './questIcons';
import {MpCanvas, MpZoomBar, type MpCanvasApi, type Bounds} from '../../ui/canvas';
import './QuestBookEditor.css';

export type QuestBookEditorProps = {
  packId: string;
  book: {revision: {draft: unknown; revision: number; state: string}} | null;
  busy?: boolean;
  onSave: (draft: QuestDraftGraph, ifMatch: number) => void | Promise<void>;
  onValidate?: () => void;
  onApply?: () => void;
  revision?: number;
};

type EditorMode = 'edit' | 'preview';
type ToastItem = {id: string; text: string};
type LinkDraft = {fromId: string; x: number; y: number} | null;

const LEGEND_ITEMS = [
  {cls: 'is-selected', label: '编辑选中'},
  {cls: 'is-locked', label: '锁定'},
  {cls: 'is-optional', label: '可选'},
  {cls: 'is-completed', label: '已完成'},
  {cls: 'is-editing', label: '编辑中'},
];

function NodeIcon({node}: {node: QuestNodeDraft}) {
  const url = iconUrlOrNull(node.icon);
  const d = iconDisplay(node.icon);
  if (url) {
    return <img className="qb-node-icon" src={url} alt="" draggable={false} aria-label={d.ariaLabel}/>;
  }
  return (
    <span
      className="qb-node-icon placeholder"
      aria-label={d.ariaLabel}
      title={node.icon || '未设置图标'}
    >
      {d.emoji}
    </span>
  );
}

function shapeClass(shape?: string): string {
  switch (shape) {
    case 'square': return 'shape-square';
    case 'rounded': return 'shape-rounded';
    case 'diamond': return 'shape-diamond';
    case 'hexagon': return 'shape-hexagon';
    case 'circle':
    default: return 'shape-circle';
  }
}

function TaskFields({task, onChange}: {task: QuestTaskDraft; onChange: (t: QuestTaskDraft) => void}) {
  const t = task.type;
  return (
    <div className="qb-task-fields">
      {t === 'item' && (
        <>
          <label>物品 ID</label>
          <Input size="small" value={task.itemId ?? ''} onChange={e => onChange({...task, itemId: e.target.value})} placeholder="minecraft:stone"/>
          <label>数量</label>
          <InputNumber size="small" min={1} value={task.count ?? 1} onChange={v => onChange({...task, count: Number(v) || 1})}/>
        </>
      )}
      {t === 'checkmark' && (
        <>
          <label>确认标题</label>
          <Input size="small" value={task.title ?? ''} onChange={e => onChange({...task, title: e.target.value})}/>
        </>
      )}
      {t === 'advancement' && (
        <>
          <label>进度 ID</label>
          <Input size="small" value={task.advancementId ?? ''} onChange={e => onChange({...task, advancementId: e.target.value})} placeholder="minecraft:story/mine_stone"/>
        </>
      )}
      {t === 'dimension' && (
        <>
          <label>维度</label>
          <Input size="small" value={task.dimension ?? ''} onChange={e => onChange({...task, dimension: e.target.value})} placeholder="minecraft:the_nether"/>
        </>
      )}
      {t === 'kill' && (
        <>
          <label>实体 ID</label>
          <Input size="small" value={task.entityId ?? ''} onChange={e => onChange({...task, entityId: e.target.value})} placeholder="minecraft:zombie"/>
        </>
      )}
      {t === 'location' && (
        <div className="qb-inline-3">
          <InputNumber size="small" placeholder="X" value={task.x} onChange={v => onChange({...task, x: Number(v)})}/>
          <InputNumber size="small" placeholder="Y" value={task.y} onChange={v => onChange({...task, y: Number(v)})}/>
          <InputNumber size="small" placeholder="Z" value={task.z} onChange={v => onChange({...task, z: Number(v)})}/>
        </div>
      )}
      {t === 'stat' && (
        <>
          <label>统计 ID</label>
          <Input size="small" value={task.statId ?? ''} onChange={e => onChange({...task, statId: e.target.value})}/>
          <label>目标值</label>
          <InputNumber size="small" min={1} value={task.value} onChange={v => onChange({...task, value: Number(v)})}/>
        </>
      )}
      {t === 'xp' && (
        <>
          <label>经验值</label>
          <InputNumber size="small" min={0} value={task.xp} onChange={v => onChange({...task, xp: Number(v)})}/>
        </>
      )}
    </div>
  );
}

function RewardFields({reward, onChange}: {reward: QuestRewardDraft; onChange: (r: QuestRewardDraft) => void}) {
  const k = reward.kind;
  return (
    <div className="qb-task-fields">
      {k === 'item' && (
        <>
          <label>物品 ID</label>
          <Input size="small" value={reward.item ?? ''} onChange={e => onChange({...reward, item: e.target.value})} placeholder="minecraft:diamond"/>
          <label>数量</label>
          <InputNumber size="small" min={1} value={reward.amount ?? 1} onChange={v => onChange({...reward, amount: Number(v) || 1})}/>
        </>
      )}
      {k === 'experience' && (
        <>
          <label>经验值</label>
          <InputNumber size="small" min={0} value={reward.experience ?? 0} onChange={v => onChange({...reward, experience: Number(v) || 0})}/>
        </>
      )}
      {k === 'command' && (
        <>
          <label>命令</label>
          <Input size="small" value={reward.command ?? ''} onChange={e => onChange({...reward, command: e.target.value})} placeholder="/give @p minecraft:stone 1"/>
        </>
      )}
      {k === 'unlock' && (
        <>
          <label>解锁 ID</label>
          <Input size="small" value={reward.unlockId ?? ''} onChange={e => onChange({...reward, unlockId: e.target.value})} placeholder="minecraft:story/iron_pickaxe"/>
        </>
      )}
    </div>
  );
}

function IconPicker({value, onChange}: {value: string; onChange: (v: string) => void}) {
  return (
    <div className="qb-icon-picker">
      <div className="qb-icon-current">
        {(() => {
          const d = iconDisplay(value);
          const url = iconUrlOrNull(value);
          return url
            ? <img src={url} alt="" aria-label={d.ariaLabel}/>
            : <span className="qb-node-icon placeholder" aria-label={d.ariaLabel} title={value || '未设置图标'}>{d.emoji}</span>;
        })()}
        <code>{value || '—'}</code>
      </div>
      <Select
        size="small"
        style={{width: '100%'}}
        showSearch
        allowClear
        placeholder="从目录选择"
        value={QUEST_ICON_CATALOG.some(e => e.id === value) ? value : undefined}
        onChange={v => onChange(v ?? '')}
        optionFilterProp="label"
        options={QUEST_ICON_CATALOG.map(e => ({value: e.id, label: `${e.emoji} ${e.label} · ${e.id}`}))}
      />
      <Input size="small" value={value} onChange={e => onChange(e.target.value)} placeholder="或直接输入 modid:item / URL"/>
    </div>
  );
}

/**
 * 仿 FTB 任务书编辑器：章节 rail + 可拖画布 + 分组 Inspector + 图例 + 编辑/预览。
 * 预览为纯前端模拟（simulatedCompleted / claimed），不写真实存档。
 */
export function QuestBookEditor(props: QuestBookEditorProps) {
  const {book, busy, onSave, onValidate, onApply, revision = 0} = props;
  const {message} = App.useApp();

  const [draft, setDraft] = useState<QuestDraftGraph | null>(null);
  const [chapterId, setChapterId] = useState('');
  const [nodeId, setNodeId] = useState('');
  const [selectedEdgeId, setSelectedEdgeId] = useState('');
  const [linkFromId, setLinkFromId] = useState('');
  const [prereqPick, setPrereqPick] = useState('');
  const [mode, setMode] = useState<EditorMode>('edit');
  const [newTaskType, setNewTaskType] = useState('item');
  const [simulatedCompleted, setSimulatedCompleted] = useState<Set<string>>(new Set());
  const [claimed, setClaimed] = useState<Set<string>>(new Set());
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [linkDraft, setLinkDraft] = useState<LinkDraft>(null);
  const canvasApiRef = useRef<MpCanvasApi | null>(null);
  const [canvasK, setCanvasK] = useState(1);
  const [snapGuides, setSnapGuides] = useState<{v: number[]; h: number[]}>({v: [], h: []});
  /** 画布右键菜单：屏蔽浏览器默认菜单后在世界坐标处新建节点等。 */
  const [ctxMenu, setCtxMenu] = useState<{
    screenX: number;
    screenY: number;
    world: {x: number; y: number};
    on: 'canvas' | 'node';
    nodeId?: string;
  } | null>(null);

  useEffect(() => {
    if (!book) { setDraft(null); return; }
    const raw = normalizeDraft(book.revision.draft as QuestDraftGraph);
    const {nodes} = ensureNodeCoords(raw.nodes);
    const next = {...raw, nodes};
    setDraft(next);
    setChapterId(next.chapters[0]?.id ?? '');
    setNodeId(next.nodes[0]?.id ?? '');
    setSelectedEdgeId('');
    setLinkFromId('');
    setPrereqPick('');
    setSimulatedCompleted(new Set());
    setClaimed(new Set());
  }, [book]);

  useEffect(() => {
    if (!snapGuides.v.length && !snapGuides.h.length) return;
    const t = window.setTimeout(() => setSnapGuides({v: [], h: []}), 400);
    return () => window.clearTimeout(t);
  }, [snapGuides]);

  const pushToast = useCallback((text: string) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    setToasts(t => [...t, {id, text}]);
    window.setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 2800);
  }, []);

  const activeChapter = draft?.chapters.find(c => c.id === chapterId) ?? draft?.chapters[0];
  const chapterNodes = useMemo(
    () => (draft?.nodes ?? []).filter(n => n.chapterId === (activeChapter?.id ?? '')),
    [draft, activeChapter],
  );
  const contentBounds = useMemo((): Bounds | null => {
    if (!chapterNodes.length) return null;
    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    for (const n of chapterNodes) {
      minX = Math.min(minX, n.x);
      minY = Math.min(minY, n.y);
      maxX = Math.max(maxX, n.x + QUEST_NODE_W);
      maxY = Math.max(maxY, n.y + QUEST_NODE_H);
    }
    return {minX, minY, maxX, maxY};
  }, [chapterNodes]);
  const getContentBounds = useCallback(() => contentBounds, [contentBounds]);
  const selectedNode = chapterNodes.find(n => n.id === nodeId) ?? chapterNodes[0];
  const allNodes = draft?.nodes ?? [];
  const edges = draft?.edges ?? [];
  const selectedEdge = edges.find(e => e.id === selectedEdgeId);
  const incoming = selectedNode ? prerequisitesOf(edges, selectedNode.id) : [];
  const cycleNodes = useMemo(() => (draft ? findCycle(draft.nodes, draft.edges) : []), [draft]);

  const prereqsOfNode = useCallback(
    (n: QuestNodeDraft) => prerequisitesOf(draft?.edges ?? [], n.id),
    [draft],
  );

  const nodeEval = useMemo(() => {
    const map = new Map<string, {unlocked: boolean; missing: string[]}>();
    for (const n of allNodes) {
      const pre = prereqsOfNode(n);
      map.set(n.id, evaluateDependencies(pre, n.dependencyRequirement, n.minRequiredDependencies, simulatedCompleted));
    }
    return map;
  }, [allNodes, prereqsOfNode, simulatedCompleted]);

  const chapterHasAlert = useCallback((cid: string) => {
    return (draft?.nodes ?? []).some(n => {
      if (n.chapterId !== cid) return false;
      const ev = nodeEval.get(n.id);
      return mode === 'preview'
        ? (ev && !ev.unlocked && !simulatedCompleted.has(n.id) && prereqsOfNode(n).length > 0)
          || simulatedCompleted.has(n.id)
        : false;
    });
  }, [draft, nodeEval, mode, simulatedCompleted, prereqsOfNode]);

  const updateNode = (id: string, patch: Partial<QuestNodeDraft>) => {
    setDraft(d => d ? {
      ...d,
      nodes: d.nodes.map(n => n.id === id ? {...n, ...patch} : n),
    } : d);
  };

  const updateSelected = (patch: Partial<QuestNodeDraft>) => {
    if (!selectedNode) return;
    updateNode(selectedNode.id, patch);
  };

  const addChapter = () => {
    if (!draft) return;
    const idx = draft.chapters.length + 1;
    const nextId = `ch${idx}`;
    const chapter = {id: nextId, title: `章节 ${idx}`, description: '', coverColor: '#4E8C86', icon: 'minecraft:book', position: idx - 1};
    const node = createQuestNode({id: `${nextId}-n1`, chapterId: nextId, title: '新任务', position: 0, x: gridX(0), y: gridY(0), taskType: newTaskType});
    setDraft({...draft, chapters: [...draft.chapters, chapter], nodes: [...draft.nodes, node]});
    setChapterId(nextId);
    setNodeId(node.id);
  };

  const addNode = (world?: {x: number; y: number}) => {
    if (!activeChapter || !draft) return;
    const peers = draft.nodes.filter(n => n.chapterId === activeChapter.id);
    const idx = peers.length + 1;
    const position = idx - 1;
    const node = createQuestNode({
      id: `${activeChapter.id}-n${idx}`,
      chapterId: activeChapter.id,
      title: `任务 ${idx}`,
      position,
      x: world ? world.x : gridX(position),
      y: world ? world.y : gridY(position),
      taskType: newTaskType,
    });
    setDraft({...draft, nodes: [...draft.nodes, node]});
    setNodeId(node.id);
    setCtxMenu(null);
  };

  const removeNodeById = (id: string) => {
    if (!draft) return;
    const nextEdges = draft.edges.filter(e => e.fromNodeId !== id && e.toNodeId !== id);
    setDraft({...draft, nodes: draft.nodes.filter(n => n.id !== id), edges: nextEdges});
    if (nodeId === id) setNodeId('');
    setSelectedEdgeId('');
    setCtxMenu(null);
    message.success('已删除节点及关联前置');
  };

  const onCanvasContextMenu = (
    w: {x: number; y: number},
    e: {clientX: number; clientY: number},
    info: {on: 'canvas' | 'node' | 'overlay'; nodeId?: string},
  ) => {
    if (info.on === 'overlay') {
      setCtxMenu(null);
      return;
    }
    if (info.on === 'node' && info.nodeId) {
      setNodeId(info.nodeId);
      setSelectedEdgeId('');
      setCtxMenu({
        screenX: e.clientX,
        screenY: e.clientY,
        world: w,
        on: 'node',
        nodeId: info.nodeId,
      });
      return;
    }
    setCtxMenu({
      screenX: e.clientX,
      screenY: e.clientY,
      world: w,
      on: 'canvas',
    });
  };

  useEffect(() => {
    if (!ctxMenu) return;
    const close = () => setCtxMenu(null);
    const onKey = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') setCtxMenu(null);
    };
    window.addEventListener('pointerdown', close);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('pointerdown', close);
      window.removeEventListener('keydown', onKey);
    };
  }, [ctxMenu]);

  const tryCreateEdge = (fromNodeId: string, toNodeId: string) => {
    if (!draft) return;
    if (fromNodeId === toNodeId) {
      message.warning('不能把节点连到自己');
      return;
    }
    const check = canAddEdge(draft.edges, fromNodeId, toNodeId);
    if (!check.ok) {
      message.warning(check.reason === '该前置连线已存在' ? '已存在该前置' : check.reason);
      return;
    }
    if (wouldCreateCycle(draft.nodes, draft.edges, fromNodeId, toNodeId)) {
      message.warning('该连线会形成环，已创建；保存前请调整');
    }
    setDraft({...draft, edges: [...draft.edges, check.edge]});
    setSelectedEdgeId(check.edge.id);
    message.success(`已连接 ${nodeLabel(allNodes, fromNodeId)} → ${nodeLabel(allNodes, toNodeId)}`);
  };

  const onNodeActivate = (n: QuestNodeDraft, shiftKey: boolean) => {
    if (mode === 'preview') {
      setNodeId(n.id);
      return;
    }
    if (shiftKey && selectedNode && selectedNode.id !== n.id) {
      tryCreateEdge(selectedNode.id, n.id);
      setNodeId(n.id);
      return;
    }
    setNodeId(n.id);
    setSelectedEdgeId('');
    setLinkFromId(shiftKey ? n.id : '');
  };

  const removeEdgeById = (edgeId: string) => {
    if (!draft) return;
    setDraft({...draft, edges: removeEdge(draft.edges, edgeId)});
    if (selectedEdgeId === edgeId) setSelectedEdgeId('');
  };

  const removePrereq = (fromNodeId: string) => {
    if (!selectedNode || !draft) return;
    setDraft({...draft, edges: removePrerequisite(draft.edges, fromNodeId, selectedNode.id)});
  };

  const addPrereqFromPick = () => {
    if (!selectedNode || !prereqPick) return;
    tryCreateEdge(prereqPick, selectedNode.id);
    setPrereqPick('');
  };

  const prepareSaveDraft = (): QuestDraftGraph | null => {
    if (!draft) return null;
    const cycle = findCycle(draft.nodes, draft.edges);
    if (cycle.length) {
      message.error(`任务图存在环，无法保存。涉及节点：${cycle.map(id => nodeLabel(draft.nodes, id)).join('、')}`);
      return null;
    }
    return draftWithSyncedPrerequisites(draft);
  };

  const saveDraft = () => {
    const payload = prepareSaveDraft();
    if (!payload) return;
    void onSave(payload, revision);
  };

  /* ---- preview simulation ---- */
  const toggleComplete = (n: QuestNodeDraft) => {
    if (mode !== 'preview') return;
    const ev = nodeEval.get(n.id) ?? evaluateDependencies(prereqsOfNode(n), n.dependencyRequirement, n.minRequiredDependencies, simulatedCompleted);
    if (!ev.unlocked && !simulatedCompleted.has(n.id)) {
      const missingLabels = ev.missing.map(id => {
        const mn = allNodes.find(x => x.id === id);
        const ch = draft?.chapters.find(c => c.id === mn?.chapterId);
        const cross = ch && ch.id !== n.chapterId ? `[${ch.title}] ` : '';
        return `${cross}${nodeLabel(allNodes, id)}`;
      });
      pushToast(`「${n.title}」还缺前置：${missingLabels.join('、')}`);
      return;
    }
    const next = new Set(simulatedCompleted);
    if (next.has(n.id)) {
      next.delete(n.id);
      pushToast(`已取消完成「${n.title}」`);
    } else {
      next.add(n.id);
      pushToast(`已完成「${n.title}」！`);
    }
    setSimulatedCompleted(next);
  };

  const claimRewards = (n: QuestNodeDraft) => {
    if (mode !== 'preview') return;
    if (!simulatedCompleted.has(n.id)) {
      pushToast(`先完成「${n.title}」再领取奖励`);
      return;
    }
    if (claimed.has(n.id)) return;
    const next = new Set(claimed);
    next.add(n.id);
    setClaimed(next);
    const count = n.rewards?.length ?? 0;
    pushToast(count ? `已领取「${n.title}」的 ${count} 项奖励` : `「${n.title}」无可领取奖励，已标记已领取`);
  };

  /* ---- canvas：MpCanvas gesture=ps + 连线 C1/C2/C3 ---- */
  const onCanvasNodeDrag = useCallback((info: {nodeId: string; worldX: number; worldY: number}) => {
    if (mode !== 'edit' || !draft) return;
    const chapterId = draft.nodes.find(n => n.id === info.nodeId)?.chapterId ?? '';
    const others = draft.nodes
      .filter(n => n.id !== info.nodeId && n.chapterId === chapterId)
      .map(n => ({x: n.x, y: n.y}));
    const snapped = snapNodePosition(info.worldX, info.worldY, others);
    setSnapGuides({v: snapped.guidesV, h: snapped.guidesH});
    setDraft(d => d ? {
      ...d,
      nodes: d.nodes.map(n => n.id === info.nodeId ? {...n, x: snapped.x, y: snapped.y} : n),
    } : d);
  }, [mode, draft]);

  const onCanvasNodeClick = useCallback((nodeId: string, e: {shiftKey: boolean}) => {
    const n = allNodes.find(x => x.id === nodeId);
    if (!n) return;
    onNodeActivate(n, e.shiftKey);
  }, [allNodes, onNodeActivate]);

  const onLinkDragStart = useCallback((_id: string, _world: {x: number; y: number}) => {
    setLinkDraft({fromId: _id, x: _world.x, y: _world.y});
  }, []);

  const onLinkDragMove = useCallback((world: {x: number; y: number}) => {
    setLinkDraft(prev => prev ? {...prev, x: world.x, y: world.y} : prev);
  }, []);

  const onLinkDragEnd = useCallback((_world: {x: number; y: number}, targetNodeId: string | null, sourceNodeId: string) => {
    setLinkDraft(null);
    if (mode !== 'edit') return;
    if (!targetNodeId || targetNodeId === sourceNodeId) {
      if (targetNodeId === sourceNodeId) message.warning('不能把节点连到自己');
      return;
    }
    tryCreateEdge(sourceNodeId, targetNodeId);
  }, [mode, tryCreateEdge]);

  /* Delete：删选中边 / 节点 */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (mode !== 'edit' || !draft) return;
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable || t.closest('.ant-select,.ant-picker,.ant-dropdown'))) {
        return;
      }
      if (e.key !== 'Delete' && e.key !== 'Backspace') return;
      if (selectedEdgeId) {
        e.preventDefault();
        setDraft(d => d ? {...d, edges: removeEdge(d.edges, selectedEdgeId)} : d);
        setSelectedEdgeId('');
        return;
      }
      if (nodeId) {
        e.preventDefault();
        const id = nodeId;
        setDraft(d => d ? {
          ...d,
          nodes: d.nodes.filter(n => n.id !== id),
          edges: d.edges.filter(ed => ed.fromNodeId !== id && ed.toNodeId !== id),
        } : d);
        setNodeId('');
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [mode, draft, selectedEdgeId, nodeId]);

  const chapterEdges = edges.filter(e => {
    const from = allNodes.find(n => n.id === e.fromNodeId);
    const to = allNodes.find(n => n.id === e.toNodeId);
    return from?.chapterId === activeChapter?.id || to?.chapterId === activeChapter?.id;
  });

  const inspectorTitle = mode === 'preview' ? '预览详情' : '属性';
  const selectedMissing = selectedNode
    ? (nodeEval.get(selectedNode.id)?.missing ?? [])
    : [];

  const depUi = selectedNode
    ? depPayloadToUi(selectedNode.dependencyRequirement, selectedNode.minRequiredDependencies)
    : {mode: 'all_completed' as DepUiMode, minN: 0};

  if (!draft) {
    return <div className="qb-empty">正在准备任务书…</div>;
  }

  return (
    <div className={`qb-root mode-${mode}`} data-mode={mode}>
      <div className="qb-topbar">
        <div className="qb-topbar-left">
          <Input
            className="qb-book-title"
            value={draft.book?.title ?? ''}
            placeholder="任务书标题"
            onChange={e => setDraft({...draft, book: {...draft.book, title: e.target.value}})}
            disabled={mode === 'preview'}
          />
          <Select
            size="small"
            style={{width: 130}}
            value={draft.book?.progressionMode ?? 'flexible'}
            disabled={mode === 'preview'}
            onChange={v => setDraft({...draft, book: {...draft.book, progressionMode: v}})}
            options={[
              {value: 'default', label: '默认'},
              {value: 'linear', label: '线性'},
              {value: 'flexible', label: '灵活'},
            ]}
          />
        </div>
        <div className="qb-topbar-mid">
          <span className={`qb-mode-badge ${mode}`} data-testid="quest-mode-badge" aria-live="polite">
            {mode === 'edit' ? '编辑' : '预览'}
          </span>
          <div className="qb-mode-switch" role="group" aria-label="编辑或预览">
            <button type="button" className={mode === 'edit' ? 'active' : ''} onClick={() => setMode('edit')}>编辑</button>
            <button type="button" className={mode === 'preview' ? 'active' : ''} onClick={() => setMode('preview')}>
              <EyeOutlined/> 预览
            </button>
          </div>
        </div>
        <div className="qb-topbar-right">
          {mode === 'edit' && (
            <>
              <Select
                size="small"
                style={{width: 120}}
                value={newTaskType}
                onChange={setNewTaskType}
                options={QUEST_TASK_TYPES.map(t => ({value: t.value, label: `+ ${t.label}`}))}
              />
              <Button size="small" onClick={() => addNode()}><PlusOutlined/> 任务</Button>
              <Button size="small" loading={busy} onClick={saveDraft}>保存草稿</Button>
              {onValidate && <Button size="small" onClick={onValidate}>校验</Button>}
              {onApply && <Button size="small" onClick={onApply}>应用</Button>}
            </>
          )}
          {mode === 'preview' && (
            <Button size="small" onClick={() => { setSimulatedCompleted(new Set()); setClaimed(new Set()); pushToast('已重置预览模拟'); }}>重置模拟</Button>
          )}
        </div>
      </div>

      <div className="qb-legend" data-testid="quest-legend" aria-label="图例">
        {LEGEND_ITEMS.map(item => (
          <span key={item.cls} className="qb-legend-item">
            <i className={`qb-legend-swatch ${item.cls}`}/>{item.label}
          </span>
        ))}
        <span className="qb-legend-item muted">滚轮纵向 · Alt 横向 · Ctrl 缩放 · Shift+点连线 · 右侧端口拖拽连线</span>
      </div>

      {cycleNodes.length > 0 && mode === 'edit' && (
        <div className="quest-banner warn" role="alert">
          存在循环依赖，保存将失败：{cycleNodes.map(nid => nodeLabel(allNodes, nid)).join('、')} — 请删除环上的前置后再保存
        </div>
      )}

      <div className="qb-layout">
        <aside className="chapter-rail" aria-label="章节列表">
          <div className="rail-label">
            <span>章节</span>
            {mode === 'edit' && <Button size="small" type="text" onClick={addChapter}>新增</Button>}
          </div>
          {draft.chapters.map((c, i) => {
            const nodes = draft.nodes.filter(n => n.chapterId === c.id);
            const alert = chapterHasAlert(c.id);
            const chIcon = iconDisplay(c.icon);
            return (
              <button
                key={c.id}
                type="button"
                className={`${c.id === activeChapter?.id ? 'selected' : ''} ${alert ? 'has-alert' : ''}`}
                onClick={() => {
                  setChapterId(c.id);
                  setNodeId(draft.nodes.find(n => n.chapterId === c.id)?.id ?? '');
                  setSelectedEdgeId('');
                  setLinkFromId('');
                }}
              >
                <span className={`chapter-number chapter-${(i % 3) + 1}`} aria-label={chIcon.ariaLabel} title={c.icon || c.title}>
                  {c.icon ? chIcon.emoji : i + 1}
                </span>
                <span>
                  {c.title}
                  <small>{nodes.length} 个任务</small>
                </span>
                {alert && mode === 'preview' && <em className="qb-alert-badge" aria-label="章节提醒">!</em>}
              </button>
            );
          })}
          {!draft.chapters.length && <div className="empty-inline">还没有章节</div>}
        </aside>

        <section className="quest-canvas qb-canvas">
          <div className="canvas-header">
            <strong>{activeChapter?.title ?? '未选择章节'}</strong>
            <span className="canvas-hint">
              {mode === 'preview'
                ? '点击节点查看 / 标记完成 / 领取奖励'
                : selectedNode ? `已选「${selectedNode.title}」· Shift+点目标 / 端口拖拽 = 加前置 · 拖节点写坐标` : '右键新建节点 · Shift/端口连线 · 滚轮平移 · Alt 横移 · Ctrl 缩放'}
            </span>
            {mode === 'edit' && <Button size="small" onClick={() => addNode()}>添加任务节点</Button>}
          </div>
          <div className="qb-canvas-wrap" style={{position: 'relative'}}>
          <MpCanvas
            className="qb-viewport"
            containerTestId="quest-canvas"
            worldClassName="mp-world qb-world"
            worldTestId="quest-world"
            gesture="ps"
            tool="select"
            background="grid"
            minZoom={0.15}
            maxZoom={2.5}
            ariaLabel="任务书编辑画布"
            getContentBounds={getContentBounds}
            nodeDraggable={mode === 'edit'}
            onViewportChange={v => setCanvasK(v.k)}
            expose={api => { canvasApiRef.current = api; }}
            onNodeDrag={onCanvasNodeDrag}
            onNodeClick={onCanvasNodeClick}
            onBackgroundClick={() => { setNodeId(''); setSelectedEdgeId(''); setLinkFromId(''); setCtxMenu(null); setSnapGuides({v: [], h: []}); }}
            onCanvasDoubleClickWorld={w => { if (mode === 'edit') addNode(w); }}
            onContextMenuWorld={mode === 'edit' ? onCanvasContextMenu : undefined}
            onLinkDrag={mode === 'edit' ? {
              onStart: onLinkDragStart,
              onMove: onLinkDragMove,
              onEnd: onLinkDragEnd,
            } : undefined}
            zoomBar={
              <MpZoomBar
                k={canvasK}
                min={0.15}
                max={2.5}
                testId="quest-zoom-bar"
                percentTestId="quest-zoom-percent"
                btnTestIds={{
                  zoomIn: 'quest-zoom-btn-in',
                  zoomOut: 'quest-zoom-btn-out',
                  fit: 'quest-zoom-btn-fit',
                  reset: 'quest-zoom-btn-reset',
                  slider: 'quest-zoom-slider',
                }}
                onZoomBy={f => canvasApiRef.current?.zoomBy(f)}
                onZoomTo={k => canvasApiRef.current?.zoomAtCenter(k)}
                onReset={() => canvasApiRef.current?.reset()}
                onFit={() => canvasApiRef.current?.fit()}
              />
            }
          >
            {(snapGuides.v.length > 0 || snapGuides.h.length > 0) && (
              <div className="qb-snap-guides" aria-hidden>
                {snapGuides.v.map((x, i) => (
                  <span key={`v${i}`} className="qb-snap-guide v" style={{left: x}}/>
                ))}
                {snapGuides.h.map((y, i) => (
                  <span key={`h${i}`} className="qb-snap-guide h" style={{top: y}}/>
                ))}
              </div>
            )}
            <svg className="quest-edge-layer qb-edge-layer" width={1} height={1} style={{overflow: 'visible'}} aria-hidden>
              <defs>
                <marker
                  id="mp-quest-arrow"
                  viewBox="0 0 10 10"
                  refX="9"
                  refY="5"
                  markerWidth="7"
                  markerHeight="7"
                  orient="auto-start-reverse"
                >
                  <path d="M 0 0 L 10 5 L 0 10 z" className="quest-arrow-head"/>
                </marker>
              </defs>
              {chapterEdges.map(e => {
                const from = allNodes.find(n => n.id === e.fromNodeId);
                const to = allNodes.find(n => n.id === e.toNodeId);
                if (!from || !to) return null;
                const d = edgePath(from, to);
                const isSelected = e.id === selectedEdgeId;
                const isCycleEdge = cycleNodes.includes(from.id) && cycleNodes.includes(to.id)
                  || cycleNodes.includes(from.id) || cycleNodes.includes(to.id);
                const fromCh = draft.chapters.find(c => c.id === from.chapterId);
                const toCh = draft.chapters.find(c => c.id === to.chapterId);
                const isCross = from.chapterId !== to.chapterId;
                return (
                  <g
                    key={e.id}
                    data-edge-id={e.id}
                    className={[
                      'quest-edge',
                      isSelected ? 'is-selected' : '',
                      isCycleEdge ? 'is-cycle' : '',
                      isCross ? 'is-cross-chapter' : '',
                    ].filter(Boolean).join(' ')}
                  >
                    <path d={d} className="quest-edge-hit" data-edge-id={e.id} onClick={() => { setSelectedEdgeId(e.id); setNodeId(e.toNodeId); }} />
                    <path d={d} className="quest-edge-line" markerEnd="url(#mp-quest-arrow)"/>
                    {isCross && fromCh && toCh && (
                      <text className="quest-edge-cross-label" x={(from.x + to.x) / 2} y={(from.y + to.y) / 2 - 6}>
                        [{fromCh.title} → {toCh.title}]
                      </text>
                    )}
                  </g>
                );
              })}
              {linkDraft && (() => {
                const from = allNodes.find(n => n.id === linkDraft.fromId);
                if (!from) return null;
                const x1 = from.x + QUEST_NODE_W;
                const y1 = from.y + QUEST_NODE_H / 2;
                return (
                  <path
                    className="quest-edge-preview"
                    data-testid="quest-link-preview"
                    d={`M ${x1} ${y1} L ${linkDraft.x} ${linkDraft.y}`}
                  />
                );
              })()}
            </svg>
            {chapterNodes.map(n => {
              const ev = nodeEval.get(n.id) ?? {unlocked: true, missing: []};
              const isCompleted = mode === 'preview' && simulatedCompleted.has(n.id);
              const isLocked = mode === 'preview' && !ev.unlocked && !isCompleted;
              const isClaimed = mode === 'preview' && claimed.has(n.id);
              const prereqCount = prereqsOfNode(n).length;
              return (
                <button
                  key={n.id}
                  type="button"
                  data-node-id={n.id}
                  data-testid={`quest-node-${n.id}`}
                  data-world-x={n.x}
                  data-world-y={n.y}
                  className={[
                    'quest-node',
                    'qb-node',
                    shapeClass(n.shape),
                    n.id === selectedNode?.id ? 'is-selected' : '',
                    n.id === linkFromId ? 'is-link-source' : '',
                    cycleNodes.includes(n.id) ? 'is-cycle' : '',
                    n.optional ? 'is-optional' : '',
                    isCompleted ? 'is-completed' : '',
                    isLocked ? 'is-locked' : '',
                    mode === 'edit' && n.id === selectedNode?.id ? 'is-editing' : '',
                  ].filter(Boolean).join(' ')}
                  style={{
                    left: n.x,
                    top: n.y,
                    width: QUEST_NODE_W,
                    minHeight: QUEST_NODE_H,
                    ['--nx' as string]: String(n.x),
                    ['--ny' as string]: String(n.y),
                  }}
                  aria-label={`${n.title}${isLocked ? ' 锁定' : ''}${n.optional ? ' 可选' : ''}`}
                  title={n.subtitle || n.description || n.title}
                >
                  <span className="qb-node-head">
                    <NodeIcon node={n}/>
                    <span className="qb-node-title">{n.title}</span>
                    {n.optional && <Tag className="qb-tag-opt">可选</Tag>}
                    {mode === 'preview' && isLocked && <LockOutlined className="qb-lock" aria-label="锁定"/>}
                    {mode === 'preview' && isCompleted && !n.invisible && (
                      <em className="qb-node-bang" aria-label="已完成提醒">!</em>
                    )}
                  </span>
                  <small className="qb-node-sub">{n.subtitle || n.description || '—'}</small>
                  {mode === 'preview' && isLocked && prereqCount > 0 && (
                    <small className="qb-node-lock-note">还缺 {ev.missing.length} 项前置</small>
                  )}
                  {mode === 'preview' && isClaimed && <small className="qb-node-claimed">已领取</small>}
                  {mode === 'edit' && (
                    <span
                      className="qb-node-port"
                      data-canvas-link-port=""
                      data-node-id={n.id}
                      data-testid={`quest-port-${n.id}`}
                      title="拖到目标节点创建前置（B 依赖 A）"
                      aria-label="连线端口"
                    />
                  )}
                </button>
              );
            })}
            {!chapterNodes.length && (
              <div className="empty-inline" style={{color: '#c9d4cd'}}>该章节还没有任务节点</div>
            )}
          </MpCanvas>
          {ctxMenu && (
            <div
              className="qb-context-menu"
              data-testid="quest-context-menu"
              style={{left: ctxMenu.screenX, top: ctxMenu.screenY}}
              onPointerDown={e => e.stopPropagation()}
              onContextMenu={e => e.preventDefault()}
              role="menu"
            >
              {ctxMenu.on === 'canvas' && mode === 'edit' && (
                <>
                  <button type="button" role="menuitem" data-testid="quest-ctx-add-node" onClick={() => addNode(ctxMenu.world)}>
                    <PlusOutlined/> 在此处新建任务节点
                  </button>
                  <button type="button" role="menuitem" onClick={() => { addChapter(); setCtxMenu(null); }}>
                    新建章节
                  </button>
                  <button type="button" role="menuitem" onClick={() => { canvasApiRef.current?.fit(); setCtxMenu(null); }}>
                    适应内容
                  </button>
                </>
              )}
              {ctxMenu.on === 'node' && mode === 'edit' && ctxMenu.nodeId && (
                <>
                  <div className="qb-context-title">{nodeLabel(allNodes, ctxMenu.nodeId)}</div>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      const nid = ctxMenu.nodeId!;
                      setCtxMenu(null);
                      if (selectedNode && selectedNode.id !== nid) tryCreateEdge(nid, selectedNode.id);
                      else message.info('先点击目标节点，再右键前置节点可连线');
                    }}
                  >
                    将此节点设为当前任务的前置
                  </button>
                  <button type="button" role="menuitem" data-testid="quest-ctx-delete-node" onClick={() => removeNodeById(ctxMenu.nodeId!)}>
                    删除该节点
                  </button>
                </>
              )}
              <button type="button" role="menuitem" className="is-quiet" onClick={() => setCtxMenu(null)}>取消</button>
            </div>
          )}
          </div>
        </section>

        <aside className="quest-inspector qb-inspector" aria-label={inspectorTitle}>
          <div className="rail-label">{inspectorTitle}</div>
          {selectedNode ? (
            <>
              {mode === 'preview' ? (
                <>
                  <div className="qb-preview-card">
                    <h4>{selectedNode.title}</h4>
                    {selectedNode.subtitle && <p className="qb-sub">{selectedNode.subtitle}</p>}
                    <p>{selectedNode.description || '暂无描述'}</p>
                    <div className="qb-section">
                      <strong>任务目标 tasks</strong>
                      {(selectedNode.tasks ?? []).length === 0 && <div className="empty-inline">未配置 tasks</div>}
                      {(selectedNode.tasks ?? []).map((t, idx) => (
                        <div key={t.id || idx} className="qb-list-row">
                          {QUEST_TASK_TYPES.find(x => x.value === t.type)?.label ?? t.type}
                          {t.itemId ? ` · ${t.itemId}${t.count ? ` ×${t.count}` : ''}` : ''}
                          {t.title ? ` · ${t.title}` : ''}
                          {t.advancementId ? ` · ${t.advancementId}` : ''}
                        </div>
                      ))}
                    </div>
                    <div className="qb-section">
                      <strong>奖励 rewards</strong>
                      {(selectedNode.rewards ?? []).length === 0 && <div className="empty-inline">未配置 rewards</div>}
                      {(selectedNode.rewards as QuestRewardDraft[] ?? []).map((r, idx) => (
                        <div key={idx} className="qb-list-row">
                          {QUEST_REWARD_KINDS.find(x => x.value === r.kind)?.label ?? r.kind}
                          {r.item ? ` · ${r.item} ×${r.amount ?? 1}` : ''}
                          {r.experience != null && r.kind === 'experience' ? ` · ${r.experience}` : ''}
                          {r.command ? ` · ${r.command}` : ''}
                          {r.unlockId ? ` · ${r.unlockId}` : ''}
                        </div>
                      ))}
                    </div>
                    <div className="qb-section">
                      <strong>依赖</strong>
                      <div className="qb-list-row">
                        {depPayloadToUi(selectedNode.dependencyRequirement, selectedNode.minRequiredDependencies).mode === 'min_n'
                          ? `至少 ${selectedNode.minRequiredDependencies}`
                          : DEP_UI_OPTIONS.find(o => o.mode === depUi.mode)?.label}
                      </div>
                      {incoming.length === 0 && <div className="qb-list-row muted">无前置</div>}
                      {incoming.map(fromId => {
                        const fn = allNodes.find(n => n.id === fromId);
                        const fch = draft.chapters.find(c => c.id === fn?.chapterId);
                        const cross = fch && fch.id !== selectedNode.chapterId;
                        const done = simulatedCompleted.has(fromId);
                        return (
                          <div key={fromId} className="qb-list-row">
                            {cross && <Tag color="blue">{fch.title}</Tag>}
                            <span className={done ? 'text-success' : ''}>{nodeLabel(allNodes, fromId)}</span>
                            {done ? <CheckCircleFilled className="text-success"/> : <span>未完成</span>}
                          </div>
                        );
                      })}
                    </div>
                    {selectedMissing.length > 0 && (
                      <div className="qb-missing" role="status">
                        <strong>还缺前置</strong>
                        {selectedMissing.map(id => {
                          const mn = allNodes.find(x => x.id === id);
                          const ch = draft.chapters.find(c => c.id === mn?.chapterId);
                          const cross = ch && ch.id !== selectedNode.chapterId;
                          return (
                            <div key={id} className="qb-list-row">
                              {cross && <Tag color="blue">{ch.title}</Tag>}
                              {nodeLabel(allNodes, id)}
                            </div>
                          );
                        })}
                      </div>
                    )}
                    <div className="qb-preview-actions">
                      <Button
                        block
                        type={simulatedCompleted.has(selectedNode.id) ? 'default' : 'primary'}
                        onClick={() => toggleComplete(selectedNode)}
                      >
                        {simulatedCompleted.has(selectedNode.id) ? '取消完成' : '标记完成'}
                      </Button>
                      <Button
                        block
                        disabled={!simulatedCompleted.has(selectedNode.id) || claimed.has(selectedNode.id)}
                        onClick={() => claimRewards(selectedNode)}
                      >
                        {claimed.has(selectedNode.id) ? '已领取' : '领取奖励'}
                      </Button>
                    </div>
                  </div>
                </>
              ) : (
                <>
                  <div className="qb-group" data-testid="insp-group-basic">
                    <h4>基础</h4>
                    <label>任务标题</label>
                    <Input value={selectedNode.title} onChange={e => updateSelected({title: e.target.value})}/>
                    <label>副标题</label>
                    <Input value={selectedNode.subtitle ?? ''} onChange={e => updateSelected({subtitle: e.target.value})}/>
                    <label>描述</label>
                    <Input.TextArea rows={3} value={selectedNode.description} onChange={e => updateSelected({description: e.target.value})}/>
                    <label>图标</label>
                    <IconPicker value={selectedNode.icon ?? ''} onChange={v => updateSelected({icon: v})}/>
                  </div>

                  <div className="qb-group" data-testid="insp-group-appearance">
                    <h4>外观</h4>
                    <label>形状</label>
                    <Select
                      size="small"
                      style={{width: '100%'}}
                      value={selectedNode.shape ?? 'circle'}
                      onChange={v => updateSelected({shape: v})}
                      options={QUEST_SHAPES.map(s => ({value: s, label: s}))}
                    />
                    <label>尺寸</label>
                    <InputNumber
                      size="small"
                      min={0.5}
                      max={3}
                      step={0.1}
                      value={selectedNode.size ?? 1}
                      onChange={v => updateSelected({size: Number(v) || 1})}
                    />
                    <label>坐标 X / Y（拖画布节点可改）</label>
                    <div className="qb-inline-2">
                      <InputNumber size="small" value={Math.round(selectedNode.x)} onChange={v => updateSelected({x: Number(v) || 0})}/>
                      <InputNumber size="small" value={Math.round(selectedNode.y)} onChange={v => updateSelected({y: Number(v) || 0})}/>
                    </div>
                    <div className="rail-note">节点 ID：{selectedNode.id}</div>
                  </div>

                  <div className="qb-group" data-testid="insp-group-deps">
                    <h4>依赖</h4>
                    <label>完成条件</label>
                    <Select
                      size="small"
                      style={{width: '100%'}}
                      value={depUi.mode}
                      onChange={(m: DepUiMode) => {
                        const p = depUiToPayload(m, depUi.minN || 1);
                        updateSelected(p);
                      }}
                      options={DEP_UI_OPTIONS.map(o => ({value: o.mode, label: o.label}))}
                    />
                    {depUi.mode === 'min_n' && (
                      <>
                        <label>至少 N 个前置完成</label>
                        <InputNumber
                          size="small"
                          min={1}
                          value={depUi.minN || 1}
                          onChange={v => updateSelected(depUiToPayload('min_n', Number(v) || 1))}
                        />
                      </>
                    )}
                    <div className="rail-note">
                      payload: <code>{selectedNode.dependencyRequirement || 'all_completed'}</code>
                      {(selectedNode.minRequiredDependencies ?? 0) > 0 && (
                        <> · minRequiredDependencies=<code>{selectedNode.minRequiredDependencies}</code></>
                      )}
                    </div>
                    <label>前置（由连线派生 edges，只读）</label>
                    <div className="quest-prereq-list">
                      {incoming.length === 0 && <div className="empty-inline">暂无前置。Shift+点击来源节点即可添加。</div>}
                      {incoming.map(fromId => (
                        <div key={fromId} className="quest-prereq-row">
                          <span title={fromId}>{nodeLabel(allNodes, fromId)}</span>
                          <Button size="small" type="text" danger onClick={() => removePrereq(fromId)}>移除</Button>
                        </div>
                      ))}
                    </div>
                    <div className="quest-prereq-add">
                      <Select
                        style={{flex: 1}}
                        size="small"
                        placeholder="选择节点添加为前置"
                        value={prereqPick || undefined}
                        onChange={setPrereqPick}
                        options={allNodes
                          .filter(n => n.id !== selectedNode.id && !incoming.includes(n.id))
                          .map(n => ({value: n.id, label: n.title}))}
                      />
                      <Button size="small" onClick={addPrereqFromPick} disabled={!prereqPick}>添加</Button>
                    </div>
                  </div>

                  <div className="qb-group" data-testid="insp-group-visible">
                    <h4>可见</h4>
                    <div className="qb-switch-row">
                      <span>可选任务 optional</span>
                      <Switch size="small" checked={!!selectedNode.optional} onChange={v => updateSelected({optional: v})}/>
                    </div>
                    <div className="qb-switch-row">
                      <span>隐藏 invisible</span>
                      <Switch size="small" checked={!!selectedNode.invisible} onChange={v => updateSelected({invisible: v})}/>
                    </div>
                  </div>

                  <div className="qb-group" data-testid="insp-group-tasks">
                    <h4>任务 tasks</h4>
                    {(selectedNode.tasks ?? []).map((task, idx) => (
                      <div key={task.id || idx} className="qb-item-card">
                        <div className="qb-item-head">
                          <Select
                            size="small"
                            style={{flex: 1}}
                            value={task.type}
                            onChange={type => {
                              const tasks = [...(selectedNode.tasks ?? [])];
                              tasks[idx] = {...tasks[idx], type};
                              updateSelected({tasks});
                            }}
                            options={QUEST_TASK_TYPES.map(t => ({value: t.value, label: t.label}))}
                          />
                          <Button size="small" danger type="text" onClick={() => {
                            const tasks = (selectedNode.tasks ?? []).filter((_, i) => i !== idx);
                            updateSelected({tasks});
                          }}>删除</Button>
                        </div>
                        <TaskFields
                          task={task}
                          onChange={t => {
                            const tasks = [...(selectedNode.tasks ?? [])];
                            tasks[idx] = t;
                            updateSelected({tasks});
                          }}
                        />
                      </div>
                    ))}
                    <Button
                      block
                      size="small"
                      type="dashed"
                      onClick={() => {
                        const tasks = [...(selectedNode.tasks ?? []), {
                          id: `${selectedNode.id}-t${(selectedNode.tasks?.length ?? 0) + 1}`,
                          type: 'item', itemId: '', count: 1,
                        }];
                        updateSelected({tasks});
                      }}
                    >
                      <PlusOutlined/> 添加 task
                    </Button>
                  </div>

                  <div className="qb-group" data-testid="insp-group-rewards">
                    <h4>奖励 rewards</h4>
                    {(selectedNode.rewards as QuestRewardDraft[] ?? []).map((reward, idx) => (
                      <div key={idx} className="qb-item-card">
                        <div className="qb-item-head">
                          <Select
                            size="small"
                            style={{flex: 1}}
                            value={reward.kind}
                            onChange={kind => {
                              const rewards = [...(selectedNode.rewards as QuestRewardDraft[] ?? [])];
                              rewards[idx] = {...rewards[idx], kind};
                              updateSelected({rewards});
                            }}
                            options={QUEST_REWARD_KINDS.map(r => ({value: r.value, label: r.label}))}
                          />
                          <Button size="small" danger type="text" onClick={() => {
                            const rewards = (selectedNode.rewards as QuestRewardDraft[] ?? []).filter((_, i) => i !== idx);
                            updateSelected({rewards});
                          }}>删除</Button>
                        </div>
                        <RewardFields
                          reward={reward}
                          onChange={r => {
                            const rewards = [...(selectedNode.rewards as QuestRewardDraft[] ?? [])];
                            rewards[idx] = r;
                            updateSelected({rewards});
                          }}
                        />
                      </div>
                    ))}
                    <Button
                      block
                      size="small"
                      type="dashed"
                      onClick={() => {
                        const rewards = [...(selectedNode.rewards as QuestRewardDraft[] ?? []), {kind: 'experience', experience: 10}];
                        updateSelected({rewards});
                      }}
                    >
                      <PlusOutlined/> 添加 reward
                    </Button>
                  </div>
                </>
              )}
            </>
          ) : (
            <div className="empty-inline">选择画布上的任务节点</div>
          )}

          {selectedEdge && mode === 'edit' && (
            <>
              <Divider>选中的连线</Divider>
              <div className="quest-prereq-row">
                <span>
                  {nodeLabel(allNodes, selectedEdge.fromNodeId)} → {nodeLabel(allNodes, selectedEdge.toNodeId)}
                </span>
              </div>
              <Button block danger size="small" onClick={() => removeEdgeById(selectedEdge.id)}>删除该前置连线</Button>
            </>
          )}
          {mode === 'edit' && (
            <>
              <Divider/>
              <Button block loading={busy} onClick={saveDraft}>保存草稿</Button>
              <div className="rail-note" style={{marginTop: 8}}>
                图权威源：draft.edges；保存时自动同步 prerequisites。
                预览模式可模拟完成与领奖（仅前端）。
              </div>
              {!draft.chapters.length && (
                <Button
                  block
                  style={{marginTop: 8}}
                  onClick={() => {
                    const d = defaultNodeFields();
                    const next = normalizeDraft({
                      chapters: [{id: 'ch1', title: '开始', description: '', coverColor: '#C9783B', icon: 'minecraft:book', position: 0}],
                      nodes: [{
                        ...d,
                        id: 'n1',
                        chapterId: 'ch1',
                        title: '入门',
                        x: gridX(0),
                        y: gridY(0),
                        position: 0,
                      }],
                      edges: [],
                    });
                    setDraft({
                      ...next,
                      nodes: ensureNodeCoords(next.nodes).nodes,
                    });
                  }}
                >
                  恢复默认结构
                </Button>
              )}
            </>
          )}
        </aside>
      </div>

      <div className="qb-toasts" aria-live="polite" data-testid="quest-toasts">
        {toasts.map(t => (
          <div key={t.id} className="qb-toast"><ThunderboltOutlined/> {t.text}</div>
        ))}
      </div>
    </div>
  );
}

export type {QuestEdgeDraft};
