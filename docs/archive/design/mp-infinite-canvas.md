# mPackStation 共享无限画布设计方案

- 状态：设计稿 v1.1（**已实现并通过测试/验收**；手势以 §4 决议为准）
- 用户拍板：任务书 `gesture=ps`（滚轮纵移/Alt 横移/Ctrl 缩放）；进度锁 `vanillaAdv`；minimap 不进 M1
- 参考实现（本机只读）：`/Volumes/Evo/visual/projects/infinite-canvas`
  - 核心：`web/src/components/canvas/infinite-canvas.tsx`
  - 类型：`web/src/types/canvas.ts` → `ViewportTransform {x,y,k}`
  - 缩放坞：`canvas-zoom-controls.tsx`
  - 小地图：`canvas-mini-map.tsx`
  - 文档：`docs/content/docs/canvas/canvas-shortcuts.zh-CN.mdx`
- 适用范围：任务书编辑画布 + 进度树浏览画布（共用一套手势与视口，业务节点不同）
- 非范围：AI 节点生成、组节点、插件系统、素材库（infinite-canvas 的业务层我们不要）

---

## 0. 一句话

把「视口变换 + 手势」抽成 **`MpCanvas` 单一组件**（对齐 infinite-canvas 的 `InfiniteCanvas`）：  
**一个 `ViewportTransform`、一层 CSS transform、滚轮锚点缩放、空格/中键平移、rAF 合帧**；任务书/进度只提供 `children` 节点与 `onNodeDrag` 等业务回调。

我们更简单：节点少、只连线不生成媒体；实现水平体现在 **数学正确 + 手势克制 + 可测**，不堆功能。

---

## 1. 从 infinite-canvas 学到的实现要点（将逐条落到我们设计）

| # | 原实现做法 | 我们采纳 |
|---|---|---|
| 1 | `ViewportTransform = {x, y, k}` 单状态 | 同；世界坐标与屏幕坐标只认这一套 |
| 2 | 世界层：`transform: translate(x,y) scale(k)`，`origin: top left` | 同 |
| 3 | 缩放锚点：`world = (mouse - viewport.xy)/k`，再反推新 x/y | 同公式 |
| 4 | 滚轮：`factor = Math.pow(1.1, deltaY 符号处理)`，clamp `[0.05, 5]` | 同；我们默认 **滚轮=锚点缩放**（与 infinite-canvas 一致） |
| 5 | 平移：中键 **或** Space/Ctrl 临时切 pan；`setPointerCapture` | 同；任务书默认 tool=`select`（拖节点），空格/中键/右键空白=平移 |
| 6 | 移动判定 `>3px` 才算拖动，否则 click/deselect | 同 |
| 7 | pan 时 `requestAnimationFrame` 合帧，避免每次 pointermove setState 风暴 | 同 |
| 8 | 网格：`gridSize = 48 * k`，`backgroundPosition = viewport.x % gridSize` | 同（可关） |
| 9 | 底部缩放坞：minimap / 重置 / **滑杆 5%–500%** / 百分比 / 快捷键说明 | 同结构；minimap P2 |
| 10 | `data-canvas-no-zoom` 排除弹层/工具条 | 同约定 |
| 11 | 双击空白、右键菜单、拖放 | 任务书：双击空白=新建任务；右键=上下文菜单（创建/粘贴）；进度只读无创建 |
| 12 | 视口持久化进 project | 任务书可把 `viewport` 写入 draft.meta（可选）；进度不持久化 |

**我们有意不同（更简单）：**

- 进度树浏览：若要对齐**原版 MC**，滚轮=平移、Ctrl+滚轮=缩放（见 §5 双模式）。
- 无框选多节点（P2）、无复制粘贴组（P2）——设计预留事件，不进 v1。
- 节点数通常 <300：DOM + SVG 连线即可，不做 canvas 虚拟化。

---

## 2. 坐标系与数学（实现核心）

```text
屏幕点 S = (sx, sy)   // 相对容器左上角，单位 CSS px
世界点 W = (wx, wy)
视口   V = (x, y, k)

S = W * k + (x, y)
W = (S - (x, y)) / k

// 以鼠标为锚缩放：该点下世界坐标不变
k' = clamp(k * factor, kMin, kMax)
x' = sx - wx * k'
y' = sy - wy * k'

// 平移
x' = x + dx, y' = y + dy   // dx,dy 为屏幕位移

// 节点拖拽（世界坐标）
wx' = wx + dx / k, wy' = wy + dy / k

// 适应内容 bounds
content = union(node.bounds)  // 含连线可选 padding
k_fit = min(w/cw, h/ch) 钳制
x = (w - cw*k_fit)/2 - minX*k_fit
y = (h - ch*k_fit)/2 - minY*k_fit
```

实现约束：

- 变换 **只写在一层** `world` DOM（或 SVG `<g transform>`）；拖动/缩放中 **禁止** 重排节点列表。
- `kMin/kMax`：任务书 `[0.15, 2.5]`；进度交互 `[0.2, 2.5]`，适应可更低（沿 vanilla 设计 k_fit≥0.08）。
- 精度：坐标用 float64；保存任务书时 `x/y` 可 `Math.round(w*100)/100`。

---

## 3. 组件 API（给业务页用）

```ts
export type MpViewport = { x: number; y: number; k: number };

export type MpCanvasProps = {
  /** 初始/受控视口；不传则内部管理 */
  viewport?: MpViewport;
  onViewportChange?: (v: MpViewport) => void;

  /** 默认工具：select=拖节点/点选；pan=拖空白平移 */
  tool?: 'select' | 'pan';

  /** 手势方案：ps（任务书：滚轮纵移/Alt 横移/Ctrl 缩放）| vanillaAdv（进度：已锁） */
  gesture?: 'ps' | 'vanillaAdv';

  background?: 'grid' | 'dots' | 'blank';
  minZoom?: number;
  maxZoom?: number;

  /** 世界内容 bbox，用于「适应」；缺省由注册节点计算 */
  getContentBounds?: () => { minX: number; minY: number; maxX: number; maxY: number } | null;

  onCanvasPointerDownWorld?: (w: { x: number; y: number }, e: PointerEvent) => void;
  onCanvasDoubleClickWorld?: (w: { x: number; y: number }, e: MouseEvent) => void;
  onContextMenuWorld?: (w: { x: number; y: number }, e: MouseEvent) => void;
  onBackgroundClick?: () => void; // 位移≤3px

  /** 程序化视口 */
  expose?: (api: {
    zoomAtCenter: (k: number) => void;
    zoomBy: (factor: number) => void;
    reset: () => void;
    fit: () => void;
    panTo: (wx: number, wy: number, opts?: { k?: number }) => void;
    getViewport: () => MpViewport;
    screenToWorld: (sx: number, sy: number) => { x: number; y: number };
    worldToScreen: (wx: number, wy: number) => { x: number; y: number };
  }) => void;

  children: React.ReactNode; // 世界层内容（绝对定位节点 + SVG 边）
};
```

**`MpCanvas` 内部结构（对齐 infinite-canvas）：**

```text
div.mp-canvas (relative, overflow:hidden, touch-action:none)
  ├─ MpGrid (pointer-events:none; 按 viewport 画网格)
  ├─ div.mp-world (absolute, origin 0 0, transform translate scale)
  │    ├─ svg.mp-edges
  │    └─ 节点 children（absolute left/top = 世界坐标）
  └─ 门户/覆盖层（缩放坞）— 带 data-canvas-no-zoom，不参与世界 transform
```

业务节点 **不** 自己算 transform；只写 `style={{left: wx, top: wy}}`。

---

## 4. 手势表（v1）

| 输入 | gesture=`ps`（任务书，**已拍板**） | gesture=`vanillaAdv`（进度，**已锁**） |
|---|---|---|
| 左键拖空白 / pan 工具 / Space+拖 / 中键 | 平移 | 平移 |
| 左键拖节点（select） | 节点移动（业务 `onNodeDrag`） | 只读 → 视为平移 |
| 滚轮 | **纵向平移**（PS） | **平移**（deltaX/deltaY） |
| Alt + 滚轮 | **横向平移**（PS） | 横向平移（同 deltaX） |
| Ctrl/Cmd + 滚轮 | **锚点缩放** | **锚点缩放** |
| 点击空白 ≤3px | 取消选中 | 取消选中 |
| 双击空白 | 业务：新建任务节点 | 无 / 可适应视图 |
| 右键 | 业务菜单 | 可选：定位/复制 id |
| 滑杆 | 设置 k（锚点=视口中心） | 同 |
| 重置 | `{x: w/2, y: h/2, k:1}` 或 fit 到内容 | fit / reset |
| 适应 | 按 `getContentBounds` | 同 |

**手势决议（用户 2026-09-20）：**  
1. 任务书：滚轮走滚动逻辑，Alt 横向，与 PS 对齐；缩放用 Ctrl+滚轮与滑杆。  
2. 进度：锁死 vanillaAdv（对齐原版 MC）。  
3. Minimap：数据整理 + 等效渲染（AABB→小地图投影），**不进 M1**。

快捷键（与 infinite-canvas 文档同级）：

- `Space` 临时 pan（输入框内不劫持）  
- `0` 重置视图 · `1` 适应内容 · `+/-` 缩放  
- 任务书编辑：`Delete` 删选中节点/边；`Esc` 取消选中  

---

## 5. 双业务接入

### 5.1 任务书 `QuestBookEditor`

- `tool='select'`，`gesture='ps'`
- 滚轮：纵向 pan；Alt+滚轮：横向 pan；Ctrl+滚轮：锚点缩放
- 节点：`onPointerDown` 选中 + 拖拽 → 写 `node.x/y`（世界坐标，`dx/k`）
- 连线：SVG 在世界层；边 path 用世界坐标
- 双击空白：创建任务（先 type）
- Shift+点击另一节点：加前置
- 视口可选写入 `draft.book.meta.viewport`

### 5.2 进度 `AdvancementTreeView`

- `gesture='vanillaAdv'`（**已锁**，对齐原版：滚轮平移）
- 节点 **不可拖**；点击=选中 Inspector；hover 短卡用 `worldToScreen`
- Tab 切换 / 适应内容：`panTo` + `fit`
- 缩放坞文案保留「工作台增强」标注

---

## 5b. 任务书连线：方式 / 处理 / 数据流转（必读，与画布同等重要）

> 画布只解决「怎么动视口/节点」；**连线是任务书的产品语义**。本节为连线交互与数据流权威；与 `quest-book-ftb-experience.md` §2.1/§2.3b 一致，冲突时以体验稿的数据权威为准。

### 5b.1 语义（先钉死）

| 概念 | 定义 |
|---|---|
| 边 `edge A→B` | **B 依赖 A**（A 是 B 的前置；FTB：`B.dependencies` 含 A） |
| 图权威 | **`draft.edges`** 唯一可写图结构 |
| `nodes[].prerequisites` | **只读派生** = 入边 `fromNodeId` 集合 |
| 依赖逻辑 | 在 **目标节点 B** 上：`dependencyRequirement` + `minRequiredDependencies` |
| 布局权威 | `nodes[].x/y`（拖节点）；`position` 仅列表序 |

### 5b.2 连线创建方式（v1 全做）

| # | 方式 | 手势 | 落点 |
|---|---|---|---|
| C1 | **点击链** | 选中 A → `Shift+点击 B` | `edges += {from:A,to:B}`；若 A∉B 的前置语义则成功 |
| C2 | **端口拖拽**（画布感） | select 下从节点 **右侧端口** 按住拖到 B 松开 | 同上；拖拽中临时预览线 |
| C3 | **Inspector** | 依赖组「添加前置」下拉选 A，作用于当前选中 B | 同上，无需画布 |

统一校验（创建瞬间）：

1. `A ≠ B`（禁自环）  
2. 不重复：已有 `A→B` → 提示「已存在该前置」不插入  
3. 环：若 `A` 可从 `B` 到达 → **允许插入但标记环**（预览提示）或 **拒绝**——**v1 采用：创建时 warning，保存时硬拦**（与现网一致）  
4. 跨章：允许；边与 Inspector 列表显示 `[章节名]`

### 5b.3 连线处理（视觉 + 操作）

```text
世界层 svg.mp-edges
  path.edge[data-edge-id]  from → to
  marker 箭头（指向依赖方 B）
  状态 class：default | selected | cycle | cross-chapter | preview-temp
```

| 操作 | 行为 |
|---|---|
| 点击边 | 选中边；Inspector 显示「前置 A → 任务 B」+「删除该前置」 |
| Delete / Backspace | 删选中边（或选中节点时删节点+关联边） |
| 拖节点 | 仅更新相连边 path（世界坐标重算） |
| 视口 pan/zoom | 边在世界层，随 transform，**不**每帧重算 |
| 线型 | v1：贝塞尔或折线 + **箭头 marker**；选中加粗/品牌色 |
| 环 | cycle 边/节点描边提示；顶部条「存在循环依赖，保存将失败」 |

### 5b.4 数据流转（端到端）

```text
┌─ UI 交互 ─────────────────────────────────────────┐
│ C1/C2/C3 → mutate draft.edges（内存唯一图）        │
│ Inspector 依赖列表 ← 由 edges 计算入边（只读）      │
│ dependencyRequirement / minN → 写在目标节点 B 上   │
└──────────────────┬───────────────────────────────┘
                   │ 保存（prepareSaveDraft）
                   ▼
┌─ 派生（保存前强制） ─────────────────────────────┐
│ draftWithSyncedPrerequisites(draft)               │
│ ∀n: n.prerequisites = [e.from | e.to==n.id]       │
│ 本地 findCycle(edges) → 有环则阻止 PUT            │
└──────────────────┬───────────────────────────────┘
                   │ PUT /quests/draft  If-Match
                   ▼
┌─ 后端 SaveQuestDraft ────────────────────────────┐
│ 校验：端点存在/自环/重复边/环/orphan/token/reward │
│ 持久化 edges + prerequisites + meta(shape/task…)  │
│ （服务端暂不强制从 edges 重算 prerequisites——遗留）│
└──────────────────┬───────────────────────────────┘
                   │ GET /quests
                   ▼
┌─ 读回 / 预览 ────────────────────────────────────┐
│ 画布用 edges 画线；面板用 prerequisites 列表       │
│ 预览模拟：对 B 求值 deps + requirement + minN     │
│ 导出 FTB 时：to.dependencies = prerequisites 派生 │
└──────────────────────────────────────────────────┘
```

**双向一致规则：**

- 画布增删边 ⇔ Inspector 依赖列表必须同一帧更新（同一 draft state）  
- 禁止 Inspector 直接改 `prerequisites` 数组  
- 刷新后以 GET 的 `edges` 为准重画；`prerequisites` 仅作展示/兼容字段  

### 5b.5 与 FTB / 预览的关系

| 层 | 使用的数据 |
|---|---|
| 画布连线 | `edges` |
| 预览锁定 | `prerequisites`（与 edges 等价）+ `dependencyRequirement` + `minRequiredDependencies` |
| FTB 导出（P2） | `B.dependencies := prerequisites`；`dependency_requirement` / `min_required_dependencies` 原样 |

### 5b.6 连线相关验收（补充 §9）

1. C1/C2/C3 至少两种可用；创建后画布出现带箭头的边  
2. 选中边可删；删后 Inspector 依赖列表同步消失  
3. 拖 B 时入边跟着走；pan/zoom 不重算错误  
4. 保存后 GET：`edges` 非空且 `prerequisites` 与入边集合一致  
5. 自环/重复边有明确提示；环在保存被拦  
6. 跨章边可见 `[章节]` 标识  

---

## 6. 缩放坞 UI（对齐 infinite-canvas 左下）

```text
┌─────────────────────────────────────────┐
│ [地图] [重置] [适应] [ − ] ──●── [ + ] 68% [?] │
└─────────────────────────────────────────┘
```

- 地图：P2 minimap（见 §8）  
- 滑杆：`min*100 … max*100`，拖动时锚点=视口中心  
- `?`：快捷键 Modal（中英一句表）  
- 坞本身 `data-canvas-no-zoom` + `stopPropagation`（infinite-canvas 同款，防止 pointer 被画布抢走）

---

## 7. 性能与实现纪律

| 项 | 做法 |
|---|---|
| 合帧 | pointermove 只写 `pendingViewport`，rAF 一次 `setState` / 直接写 DOM style |
| 高频路径 | pan/zoom **优先直接改 `world.style.transform`**，React viewport state 可节流同步（工具条百分比 100ms） |
| 禁止 | 每次 move 重算整图 layout、重挂全部节点 |
| 边数 | O(edges) path 更新；拖节点时只更新该节点 `left/top` 与相连边 |
| 网格 | 纯 CSS gradient，不每帧重绘 canvas |
| 测试 | 纯函数：`screenToWorld/worldToScreen/zoomAtAnchor/fitBounds/clamp`；手势：可单测 `isPanGesture/shouldZoom` |

---

## 8. 分期

| 阶段 | 内容 |
|---|---|
| **M1（本设计落地）** | `MpCanvas` + 网格 + 双 gesture + 缩放坞（无 minimap）+ 节点拖拽 API + fit/reset + 任务书/进度接入 |
| **M2** | minimap、框选、多选拖动、右键菜单完善、viewport 持久化 |
| **P2** | 组节点、对齐吸附、复制粘贴、触摸 pinch |

---

## 9. 验收标准（实现水平可检查）

1. **数学**：单测覆盖锚点缩放不变性、fit 公式、screen/world 互逆（误差 &lt; 1e-6）  
2. **任务书**：`gesture=ps`——滚轮纵向平移、Alt 横向、Ctrl+滚轮锚点缩放；空格/中键平移；拖节点 `Δworld=Δscreen/k`；连线 C1/C2/C3 + 箭头 + edges→prerequisites  
3. **进度**：`vanillaAdv` 锁死——滚轮平移、Ctrl 缩放；节点不可拖  
4. **工具条**：滑杆与 `%` 同步；重置/适应可用；坞不触发画布 pan  
5. **性能**：200 节点拖画布时节点 `offsetLeft` 不变（只改 transform）  
6. **不回归**：任务书保存 edges↔prerequisites；进度 Tab=有 display root  
7. **连线**：见 §5b.6（创建/删除/派生/环拦）  

---

## 10. 与现网差异（为何还要做）

| 现网 | 本设计 |
|---|---|
| 任务书/进度各自手写 pan/zoom，不一致 | 统一 `MpCanvas` + gesture 模式 |
| 任务书滚轮语义含糊（验收曾指出死代码） | infinite 模式：滚轮=缩放，文档写死 |
| 进度与原版手势靠散落实现 | vanillaAdv 模式显式化 |
| 无网格、无缩放坞滑杆、无 Space 临时 pan | 对齐 infinite-canvas 体验基线 |
| 坐标换算散落组件内 | `screenToWorld` 等集中可测 |

---

## 11. 参考源码摘录（便于对照实现）

**锚点缩放 + pan rAF**（infinite-canvas `infinite-canvas.tsx`）：

```ts
const factor = Math.pow(1.1, -event.deltaY / 100); // 他们写 delta=-deltaY
const newScale = clamp(viewport.k * factor, 0.05, 5);
const worldX = (mouseX - viewport.x) / viewport.k;
onViewportChange({
  x: mouseX - worldX * newScale,
  y: mouseY - worldY * newScale,
  k: newScale,
});
// pan: nextViewportRef + requestAnimationFrame 合帧
```

**世界层：**

```tsx
style={{ transform: `translate(${x}px, ${y}px) scale(${k})` }}
```

**缩放坞：** slider `5..500` 对应 `k=0.05..5`，显示 `Math.round(k*100)%`。

---

## 12. 开放问题 — 决议（用户已拍板）

1. 任务书手势：**`ps`**（滚轮纵移、Alt 横移、Ctrl 缩放），非 infinite 滚轮缩放。  
2. 进度：**锁死 `vanillaAdv`**。  
3. M1：**不做 minimap**（数据整理+等效渲染，M2 再做）。  

---

## 13. 实现状态（2026-09-20）

**已完成 M1**：`apps/web/src/ui/canvas/`（viewport + MpCanvas + MpZoomBar）；任务书/进度已接入；连线 §5b 已落地。  
测试：viewport 24、quest-canvas-edges 16、layout 28、`go test` 全绿；独立测试+验收 **允许结束**。  

M2 未做：minimap、右键菜单业务接线、`?` 快捷键弹层、服务端强制 edges→prerequisites。
