/* 进度树布局与视图变换纯函数。
   设计权威：docs/active/design/advancement-vanilla-experience.md v1.1
   （advancement-tree-canvas.md 仅作历史手势细节参考，不作验收权威）。
   本模块无 DOM/React 依赖，可被 vitest / node 直接单测。 */

export type AdvFrame = 'task' | 'goal' | 'challenge';

/** display 可选元数据（hidden / toast / chat / background） */
export type AdvDisplayMeta = {
  background?: string | null;
  hidden?: boolean;
  showToast?: boolean;
  announceToChat?: boolean;
};

/** requirements：外层 OR，内层 AND（原版 OR(AND) 二维数组） */
export type AdvRequirements = string[][];

export type AdvRewards = {
  experience?: number;
  recipes?: string[];
  loot?: string[];
  function?: string;
};

export type AdvNode = {
  id: string;
  parentId: string | null;
  title: string;
  description?: string;
  iconItem?: string | null;
  iconUrl?: string | null;
  frame?: AdvFrame;
  criteria?: string[];
  requirements?: AdvRequirements;
  rewards?: AdvRewards;
  display?: AdvDisplayMeta;
  /** display 是否存在——Tab 权威判定 */
  hasDisplay: boolean;
  /** 是否属于 advancement/recipes/**（无 display 默认不进玩家树） */
  isRecipes?: boolean;
  /** 仅 fixture/演示；默认 false，避免假完成 */
  done?: boolean;
};

export type AdvTreeKind = 'tab' | 'recipes' | 'ungrouped';

export type AdvTree = {
  id: string;
  label: string;
  nodes: AdvNode[];
  /** tab=有 display 的 root；recipes=配方解锁折叠组；ungrouped=未归组/外部前置 */
  kind: AdvTreeKind;
  icon?: string | null;
  background?: string | null;
  /** 该分组内 sanitize 打断的环数（用于全局数据质量提示） */
  cycleBreaks?: number;
};

export type LayoutNode = AdvNode & {
  x: number;
  y: number;
  depth: number;
  treeRootId: string;
};

export type LayoutEdge = { parentId: string; childId: string };

export type LayoutBounds = {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
  width: number;
  height: number;
};

export type LayoutResult = {
  nodes: LayoutNode[];
  edges: LayoutEdge[];
  bounds: LayoutBounds;
  cycleBreaks: number;
  missingParentCount: number;
  parentOf: Map<string, string | null>;
  nodeById: Map<string, LayoutNode>;
};

export type ViewTransform = { k: number; tx: number; ty: number };

export type PanBounds = {
  minTx: number;
  maxTx: number;
  minTy: number;
  maxTy: number;
};

export const NODE_SIZE = 56;
export const X_STEP = 152;
export const Y_STEP = 92;
export const TREE_GAP = 92;
export const K_MIN = 0.35;
export const K_MAX = 2.5;
export const K_FIT_MIN = 0.08;
export const K_DEFAULT = 1;
export const FIT_PADDING = 48;
export const LABEL_HIDE_K = 0.55;
export const DRAG_CLICK_PX = 4;
export const DRAG_CLICK_MS = 180;
/** 有界 pan：内容至少露出的边距（px） */
export const PAN_MARGIN = 48;

/** id 最后一段作短名展示 */
export function shortName(id: string): string {
  const path = id.includes(':') ? id.slice(id.indexOf(':') + 1) : id;
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || id;
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

/**
 * 锚点缩放：保持屏幕点 (sx,sy) 下的世界坐标不变。
 * world = (screen - tx) / k
 */
export function zoomAtAnchor(
  current: ViewTransform,
  kNext: number,
  sx: number,
  sy: number,
): ViewTransform {
  const { k, tx, ty } = current;
  const wx = (sx - tx) / k;
  const wy = (sy - ty) / k;
  return { k: kNext, tx: sx - wx * kNext, ty: sy - wy * kNext };
}

/** 交互缩放：k 钳制在 [0.35, 2.5] */
export function interactiveZoom(
  current: ViewTransform,
  factor: number,
  sx: number,
  sy: number,
): ViewTransform {
  const kNext = clamp(current.k * factor, K_MIN, K_MAX);
  return zoomAtAnchor(current, kNext, sx, sy);
}

/**
 * 适应内容：按包围盒与视口求 k_fit，允许低于交互下限 0.35，下限 0.08。
 * 之后用户再缩放走交互钳制。
 */
export function fitTransform(
  bounds: LayoutBounds,
  viewportW: number,
  viewportH: number,
  padding = FIT_PADDING,
): ViewTransform {
  const contentW = Math.max(bounds.width, 1);
  const contentH = Math.max(bounds.height, 1);
  const availW = Math.max(viewportW - padding * 2, 1);
  const availH = Math.max(viewportH - padding * 2, 1);
  const raw = Math.min(availW / contentW, availH / contentH);
  const k = clamp(raw, K_FIT_MIN, K_MAX);
  const cx = bounds.minX + bounds.width / 2;
  const cy = bounds.minY + bounds.height / 2;
  return { k, tx: viewportW / 2 - cx * k, ty: viewportH / 2 - cy * k };
}

/** 重置视图：回到根节点附近（内容左缘），k=1 */
export function resetTransform(
  bounds: LayoutBounds,
  viewportW: number,
  viewportH: number,
): ViewTransform {
  const k = K_DEFAULT;
  const focusX = bounds.minX + NODE_SIZE / 2;
  const focusY = bounds.minY + bounds.height / 2;
  return { k, tx: viewportW / 2 - focusX * k, ty: viewportH / 2 - focusY * k };
}

/** 屏幕坐标 → 世界坐标 */
export function screenToWorld(
  view: ViewTransform,
  sx: number,
  sy: number,
): { x: number; y: number } {
  return { x: (sx - view.tx) / view.k, y: (sy - view.ty) / view.k };
}

/**
 * 有界 pan：内容始终与视口保持至少 margin 的重叠。
 * 内容完全放入视口时进一步收紧，避免小树被甩出画布。
 */
export function computePanBounds(
  bounds: LayoutBounds,
  view: ViewTransform,
  viewportW: number,
  viewportH: number,
  margin = PAN_MARGIN,
): PanBounds {
  const k = view.k;
  const { minX, minY, maxX, maxY } = bounds;
  const cw = (maxX - minX) * k;
  const ch = (maxY - minY) * k;

  let minTx = margin - maxX * k;
  let maxTx = viewportW - margin - minX * k;
  let minTy = margin - maxY * k;
  let maxTy = viewportH - margin - minY * k;

  if (cw + margin * 2 <= viewportW) {
    maxTx = Math.min(maxTx, viewportW - margin - maxX * k);
    minTx = Math.max(minTx, margin - minX * k);
  }
  if (ch + margin * 2 <= viewportH) {
    maxTy = Math.min(maxTy, viewportH - margin - maxY * k);
    minTy = Math.max(minTy, margin - minY * k);
  }

  if (minTx > maxTx) {
    const mid = (minTx + maxTx) / 2;
    minTx = mid;
    maxTx = mid;
  }
  if (minTy > maxTy) {
    const mid = (minTy + maxTy) / 2;
    minTy = mid;
    maxTy = mid;
  }
  return { minTx, maxTx, minTy, maxTy };
}

/** 将视图平移钳制进有界 pan 范围 */
export function clampPan(view: ViewTransform, pan: PanBounds): ViewTransform {
  return {
    k: view.k,
    tx: clamp(view.tx, pan.minTx, pan.maxTx),
    ty: clamp(view.ty, pan.minTy, pan.maxTy),
  };
}

export type SanitizeResult = {
  parentOf: Map<string, string | null>;
  cycleBreaks: number;
  missingParentIds: string[];
};

/**
 * 脏数据处理：
 * - parentId 指向不存在节点 → 视为根
 * - 自环 → 视为根
 * - 环：染色检测，打断回边（环入口节点提升为根）
 */
export function sanitizeParents(
  nodes: ReadonlyArray<Pick<AdvNode, 'id' | 'parentId'>>,
): SanitizeResult {
  const idSet = new Set(nodes.map(n => n.id));
  const parentOf = new Map<string, string | null>();
  const missingParentIds: string[] = [];

  for (const n of nodes) {
    const pid = n.parentId;
    if (!pid || pid === n.id || !idSet.has(pid)) {
      if (pid && pid !== n.id && !idSet.has(pid)) missingParentIds.push(n.id);
      parentOf.set(n.id, null);
    } else {
      parentOf.set(n.id, pid);
    }
  }

  let cycleBreaks = 0;
  const color = new Map<string, 0 | 1 | 2>();
  for (const n of nodes) {
    if (color.get(n.id) === 2) continue;
    const path: string[] = [];
    let cur: string | null = n.id;
    while (cur != null && color.get(cur) !== 2) {
      if (color.get(cur) === 1) {
        parentOf.set(cur, null);
        cycleBreaks += 1;
        break;
      }
      color.set(cur, 1);
      path.push(cur);
      cur = parentOf.get(cur) ?? null;
    }
    for (const id of path) color.set(id, 2);
  }

  return { parentOf, cycleBreaks, missingParentIds };
}

function emptyBounds(): LayoutBounds {
  return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0 };
}

/**
 * 只读森林布局：
 * - DFS 后序：叶子 y 连续，父 y = 均值(子)
 * - x = depth * 152；同层（叶子）间距 92
 * - 多棵 root 之间垂直间隔 ≥92
 * - x/y 表示节点中心
 */
export function layoutAdvancement(nodes: AdvNode[]): LayoutResult {
  const { parentOf, cycleBreaks, missingParentIds } = sanitizeParents(nodes);

  const children = new Map<string, string[]>();
  const roots: string[] = [];
  for (const n of nodes) {
    const p = parentOf.get(n.id) ?? null;
    if (!p) roots.push(n.id);
    else {
      const list = children.get(p);
      if (list) list.push(n.id);
      else children.set(p, [n.id]);
    }
  }
  roots.sort();
  for (const list of children.values()) list.sort();

  const pos = new Map<string, { x: number; y: number; depth: number; rootId: string }>();
  let leafCursor = 0;

  const visit = (id: string, depth: number, rootId: string): number => {
    const kids = children.get(id) ?? [];
    const x = depth * X_STEP;
    if (kids.length === 0) {
      const y = leafCursor;
      leafCursor += Y_STEP;
      pos.set(id, { x, y, depth, rootId });
      return y;
    }
    let sum = 0;
    for (const kid of kids) sum += visit(kid, depth + 1, rootId);
    const y = sum / kids.length;
    pos.set(id, { x, y, depth, rootId });
    return y;
  };

  for (let i = 0; i < roots.length; i += 1) {
    if (i > 0) leafCursor += TREE_GAP;
    visit(roots[i], 0, roots[i]);
  }

  const layoutNodes: LayoutNode[] = [];
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;

  for (const n of nodes) {
    const p = pos.get(n.id);
    if (!p) continue;
    const half = NODE_SIZE / 2;
    minX = Math.min(minX, p.x - half);
    minY = Math.min(minY, p.y - half);
    maxX = Math.max(maxX, p.x + half);
    maxY = Math.max(maxY, p.y + half);
    layoutNodes.push({
      ...n,
      parentId: parentOf.get(n.id) ?? null,
      x: p.x,
      y: p.y,
      depth: p.depth,
      treeRootId: p.rootId,
    });
  }

  const edges: LayoutEdge[] = [];
  for (const n of layoutNodes) {
    if (n.parentId && pos.has(n.parentId)) {
      edges.push({ parentId: n.parentId, childId: n.id });
    }
  }

  const bounds: LayoutBounds =
    layoutNodes.length === 0
      ? emptyBounds()
      : { minX, minY, maxX, maxY, width: maxX - minX, height: maxY - minY };

  const nodeById = new Map(layoutNodes.map(n => [n.id, n]));
  return {
    nodes: layoutNodes,
    edges,
    bounds,
    cycleBreaks,
    missingParentCount: missingParentIds.length,
    parentOf,
    nodeById,
  };
}

/** 自当前节点向上走父链（近→远），已断环故无死循环；额外 seen 防御 */
export function parentChain(
  nodeId: string,
  parentOf: Map<string, string | null>,
): string[] {
  const chain: string[] = [];
  const seen = new Set<string>([nodeId]);
  let cur = parentOf.get(nodeId) ?? null;
  while (cur && !seen.has(cur)) {
    seen.add(cur);
    chain.push(cur);
    cur = parentOf.get(cur) ?? null;
  }
  return chain;
}

/** 贝塞尔连线路径（父子中心 → 边缘，水平控制点） */
export function edgePath(parent: { x: number; y: number }, child: { x: number; y: number }): string {
  const half = NODE_SIZE / 2;
  const x1 = parent.x + half;
  const y1 = parent.y;
  const x2 = child.x - half;
  const y2 = child.y;
  const mid = (x1 + x2) / 2;
  return `M${x1},${y1} C${mid},${y1} ${mid},${y2} ${x2},${y2}`;
}

/* ---------- requirements 语义（OR(AND)，禁止写反） ---------- */

export type RequirementsMode = 'empty' | 'any' | 'all' | 'mixed';

export type RequirementsView = {
  mode: RequirementsMode;
  /** 任一条件 / 全部条件 / (a 且 b) 或 c / （无 requirements） */
  label: string;
  /** 展开的条件组（已按语义格式化） */
  groups: string[];
};

/**
 * 原版语义：外层数组 OR，内层数组 AND。
 * - [[a],[b],[c]] → 任一条件（a ∨ b ∨ c）
 * - [[a,b,c]]     → 全部条件（a ∧ b ∧ c）
 * - [[a,b],[c]]   → (a 且 b) 或 c
 */
export function classifyRequirements(req?: AdvRequirements | null): RequirementsMode {
  if (!req || !Array.isArray(req) || req.length === 0) return 'empty';
  const groups = req
    .filter(g => Array.isArray(g) && g.length > 0)
    .map(g => g.map(String));
  if (groups.length === 0) return 'empty';
  if (groups.length === 1) return 'all';
  if (groups.every(g => g.length === 1)) return 'any';
  return 'mixed';
}

export function describeRequirements(req?: AdvRequirements | null): RequirementsView {
  const mode = classifyRequirements(req);
  if (mode === 'empty') {
    return { mode, label: '（无 requirements）', groups: [] };
  }
  const groups = (req as AdvRequirements)
    .filter(g => Array.isArray(g) && g.length > 0)
    .map(g => g.map(String));

  if (mode === 'any') {
    return { mode, label: '任一条件', groups: groups.map(g => g[0]) };
  }
  if (mode === 'all') {
    const only = groups[0];
    return {
      mode,
      label: only.length === 1 ? `条件：${only[0]}` : '全部条件',
      groups: [only.join(' 且 ')],
    };
  }
  const parts = groups.map(g => (g.length === 1 ? g[0] : `(${g.join(' 且 ')})`));
  return { mode, label: parts.join(' 或 '), groups: parts };
}

export function formatRewardsSummary(rewards?: AdvRewards | null): string {
  if (!rewards) return '';
  const parts: string[] = [];
  if (typeof rewards.experience === 'number') parts.push(`${rewards.experience} XP`);
  if (rewards.recipes?.length) parts.push(`配方×${rewards.recipes.length}`);
  if (rewards.loot?.length) parts.push(`战利品×${rewards.loot.length}`);
  if (rewards.function) parts.push(`函数 ${rewards.function}`);
  return parts.join(' · ');
}

/* ---------- Tab 分组（原版语义，path 启发式禁止生成 Tab） ---------- */

/** advancement/recipes/** 路径判定 */
export function isRecipesAdvancement(id: string, path?: string | null): boolean {
  const raw = path || id;
  const norm = String(raw).replace(/\\/g, '/');
  const pathPart = norm.includes(':') ? norm.slice(norm.indexOf(':') + 1) : norm;
  return (
    pathPart.includes('advancement/recipes/') ||
    pathPart.startsWith('recipes/') ||
    pathPart.includes('/recipes/') ||
    /(^|:)recipes\//.test(norm)
  );
}

/**
 * Tab / 树权威（对齐原版，设计 §2.1）：
 * - root ⇔ (parent ∉ S 或 parent 为空) ∧ display 存在
 * - Tab = 每个 root 一棵树；label=display.title，icon=display.icon
 * - parent ∈ S → 挂到对应 root 的子树
 * - parent ∉ S 且无 display → 不伪造 Tab；进「未归组/外部前置」
 * - 无 display 的 recipes/** → 默认不进玩家树；折叠分组「配方解锁」
 * - path 启发式禁止创建 Tab
 */
export function groupAdvancementTrees(nodes: AdvNode[]): AdvTree[] {
  if (nodes.length === 0) return [];

  const { parentOf, cycleBreaks } = sanitizeParents(nodes);
  const byId = new Map(nodes.map(n => [n.id, n]));
  const idSet = new Set(nodes.map(n => n.id));

  const rootOf = new Map<string, string>();
  const findRoot = (id: string): string => {
    const cached = rootOf.get(id);
    if (cached) return cached;
    const seen = new Set<string>();
    let cur = id;
    let structuralRoot = id;
    while (true) {
      if (seen.has(cur)) break;
      seen.add(cur);
      const p = parentOf.get(cur) ?? null;
      if (!p || !idSet.has(p)) {
        structuralRoot = cur;
        break;
      }
      cur = p;
      structuralRoot = cur;
    }
    for (const s of seen) rootOf.set(s, structuralRoot);
    return structuralRoot;
  };

  const groups = new Map<string, AdvNode[]>();
  for (const n of nodes) {
    const r = findRoot(n.id);
    const list = groups.get(r);
    if (list) list.push(n);
    else groups.set(r, [n]);
  }

  const tabs: AdvTree[] = [];
  let recipeNodes: AdvNode[] = [];
  let ungroupedNodes: AdvNode[] = [];

  for (const [rootId, group] of groups) {
    const root = byId.get(rootId);
    if (!root) {
      ungroupedNodes = ungroupedNodes.concat(group);
      continue;
    }
    if (root.hasDisplay) {
      tabs.push({
        id: rootId,
        label: root.title || shortName(rootId),
        nodes: group,
        kind: 'tab',
        icon: root.iconUrl ?? root.iconItem ?? null,
        background: root.display?.background ?? null,
      });
      continue;
    }
    const recipes =
      root.isRecipes === true ||
      isRecipesAdvancement(rootId) ||
      group.every(n => n.isRecipes === true || isRecipesAdvancement(n.id));
    if (recipes) {
      recipeNodes = recipeNodes.concat(group);
    } else {
      ungroupedNodes = ungroupedNodes.concat(group);
    }
  }

  tabs.sort((a, b) => a.id.localeCompare(b.id));

  const trees: AdvTree[] = [...tabs];
  if (ungroupedNodes.length > 0) {
    trees.push({
      id: '__ungrouped__',
      label: '未归组/外部前置',
      nodes: ungroupedNodes,
      kind: 'ungrouped',
      cycleBreaks,
    });
  }
  if (recipeNodes.length > 0) {
    trees.push({
      id: '__recipes__',
      label: '配方解锁',
      nodes: recipeNodes,
      kind: 'recipes',
      cycleBreaks,
    });
  }
  if (trees.length > 0 && cycleBreaks > 0) {
    trees[0] = {...trees[0], cycleBreaks: (trees[0].cycleBreaks ?? 0) + cycleBreaks};
  }
  return trees;
}

/* ---------- payload → AdvNode ---------- */

function asText(value: unknown, fallback = ''): string {
  if (typeof value === 'string') return value;
  if (value && typeof value === 'object') {
    const o = value as Record<string, unknown>;
    if (typeof o.text === 'string') return o.text;
    if (typeof o.translate === 'string') return o.translate;
    if (Array.isArray(o.with) && typeof o.translate === 'string') {
      return o.translate;
    }
  }
  return fallback;
}

function parseFrame(value: unknown): AdvFrame | undefined {
  if (value === 'task' || value === 'goal' || value === 'challenge') return value;
  return undefined;
}

function parseCriteria(value: unknown): string[] | undefined {
  if (Array.isArray(value)) {
    return value.map(v => String(v));
  }
  if (value && typeof value === 'object') {
    return Object.keys(value as Record<string, unknown>);
  }
  return undefined;
}

/** icon.item | icon.id | 字符串 */
function parseIconItem(value: unknown): string | null {
  if (typeof value === 'string') return value;
  if (value && typeof value === 'object') {
    const o = value as Record<string, unknown>;
    if (typeof o.item === 'string') return o.item;
    if (typeof o.id === 'string') return o.id;
  }
  return null;
}

/** requirements 二维数组：外层 OR、内层 AND；扁平数组视为单组 AND */
export function parseRequirements(value: unknown): AdvRequirements | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  const first = value[0];
  if (Array.isArray(first)) {
    return (value as unknown[]).map(g =>
      Array.isArray(g) ? g.map(x => String(x)) : [String(g)],
    );
  }
  return [(value as unknown[]).map(x => String(x))];
}

export function parseRewards(value: unknown): AdvRewards | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const o = value as Record<string, unknown>;
  const out: AdvRewards = {};
  if (typeof o.experience === 'number' && Number.isFinite(o.experience)) {
    out.experience = o.experience;
  }
  if (Array.isArray(o.recipes)) out.recipes = o.recipes.map(String);
  if (Array.isArray(o.loot)) out.loot = o.loot.map(String);
  else if (typeof o.loot === 'string' && o.loot) out.loot = [o.loot];
  if (typeof o.function === 'string' && o.function) out.function = o.function;
  return Object.keys(out).length > 0 ? out : undefined;
}

export function parseDisplayMeta(display: unknown): AdvDisplayMeta | undefined {
  if (!display || typeof display !== 'object') return undefined;
  const d = display as Record<string, unknown>;
  const meta: AdvDisplayMeta = {};
  if (typeof d.background === 'string') meta.background = d.background;
  if (typeof d.hidden === 'boolean') meta.hidden = d.hidden;
  if (typeof d.show_toast === 'boolean') meta.showToast = d.show_toast;
  if (typeof d.announce_to_chat === 'boolean') meta.announceToChat = d.announce_to_chat;
  return meta;
}

export type AdvPayloadParse = {
  parentId: string | null;
  title: string;
  description?: string;
  iconItem: string | null;
  frame?: AdvFrame;
  criteria?: string[];
  requirements?: AdvRequirements;
  rewards?: AdvRewards;
  display?: AdvDisplayMeta;
  hasDisplay: boolean;
};

/** 解析 advancement payload（parent / display / criteria / requirements / rewards） */
export function parseAdvPayload(payload: unknown, id: string): AdvPayloadParse {
  const p = (payload && typeof payload === 'object' ? payload : {}) as Record<string, unknown>;
  const display = (p.display && typeof p.display === 'object' ? p.display : null) as
    | Record<string, unknown>
    | null;
  const parentId = typeof p.parent === 'string' && p.parent ? p.parent : null;
  const title = asText(display?.title, shortName(id));
  const description = display ? asText(display.description, '') || undefined : undefined;
  const iconItem = display ? parseIconItem(display.icon) : null;
  const frame = display ? parseFrame(display.frame) : undefined;
  const criteria = parseCriteria(p.criteria);
  const requirements = parseRequirements(p.requirements);
  const rewards = parseRewards(p.rewards);
  const displayMeta = parseDisplayMeta(display);
  return {
    parentId,
    title,
    description,
    iconItem,
    frame,
    criteria,
    requirements,
    rewards,
    display: displayMeta,
    hasDisplay: display != null,
  };
}

export type AdvNodeBuildOptions = {
  getIcon?: (itemId: string) => string | null;
  translateKey?: (key: string) => string;
};

/** 从 listModContent 的一条 advancement 记录组装 AdvNode */
export function itemToAdvNode(
  item: { id?: string; key?: string; payload?: unknown; path?: string },
  opts: AdvNodeBuildOptions = {},
): AdvNode {
  const id = item.key || item.id || item.path || 'unknown';
  const parsed = parseAdvPayload(item.payload, id);
  let title = parsed.title;
  if (opts.translateKey && title) {
    const translated = opts.translateKey(title);
    if (translated && translated !== title) title = translated;
  }
  let description = parsed.description;
  if (opts.translateKey && description) {
    const translated = opts.translateKey(description);
    if (translated && translated !== description) description = translated;
  }
  const iconUrl = parsed.iconItem && opts.getIcon ? opts.getIcon(parsed.iconItem) : null;
  const looksLikeLangKey = (s: string) => s.includes('.') && !s.includes(':') && !s.includes(' ');
  let finalTitle = title;
  if (looksLikeLangKey(finalTitle) && (finalTitle.startsWith('advancement') || finalTitle.startsWith('advancements'))) {
    finalTitle = shortName(id);
  }
  return {
    id,
    parentId: parsed.parentId,
    title: finalTitle,
    description,
    iconItem: parsed.iconItem,
    iconUrl,
    frame: parsed.frame,
    criteria: parsed.criteria,
    requirements: parsed.requirements,
    rewards: parsed.rewards,
    display: parsed.display,
    hasDisplay: parsed.hasDisplay,
    isRecipes: isRecipesAdvancement(id, item.path),
  };
}

/** 批量组装 + 分组（ModContentPage 入口）；Tab 仅来自 hasDisplay 的 root */
export function itemsToAdvTrees(
  items: ReadonlyArray<{ id?: string; key?: string; payload?: unknown; path?: string }>,
  opts: AdvNodeBuildOptions = {},
): AdvTree[] {
  const nodes = items.map(item => itemToAdvNode(item, opts));
  return groupAdvancementTrees(nodes);
}

/** 验收夹具：宽扁大树（≥220 节点） */
export function fixtureAdvWideFlat(count = 220): AdvNode[] {
  const nodes: AdvNode[] = [
    {
      id: 'fixture:wide_flat/root',
      parentId: null,
      title: 'wide_flat root',
      frame: 'task',
      hasDisplay: true,
    },
  ];
  for (let i = 0; i < count - 1; i += 1) {
    nodes.push({
      id: `fixture:wide_flat/n${i}`,
      parentId: 'fixture:wide_flat/root',
      title: `Node ${i}`,
      frame: i % 5 === 0 ? 'goal' : 'task',
      hasDisplay: false,
    });
  }
  return nodes;
}

/** 判断点击/拖动：位移阈值 4px；或按住 >180ms 且有移动 → 拖动 */
export function isDragGesture(distPx: number, heldMs: number): boolean {
  return distPx > DRAG_CLICK_PX || (heldMs > DRAG_CLICK_MS && distPx > 0);
}
