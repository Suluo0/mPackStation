# 进度树画布交互设计（仿原版 MC）

- 状态：设计方案 v1.1（盲审有条件通过后的修订稿，可开发）
- 日期：2026-09-20
- 盲审：`general-1` 结论「有条件通过」；下列 §4.2bis / §5bis / §11bis / §15 已合入
- 范围：内容编辑 → 进度（advancement）树状预览的可拖动/可缩放交互
- 非范围：任务书（quest）编辑器、方块图标渲染清晰度、后端 schema

## 1. 目标

用户在浏览包内 `kind=advancement` 内容时，需要像原版 Minecraft 进度界面一样：

1. 在**无限画布**上查看整棵进度树（不止一屏）
2. **拖动**平移画布（必须流畅，否则不可用）
3. **缩放**查看细节与全局（滚轮/触控板/按钮）
4. 点击节点查看详情（criteria / 奖励 / 父链）
5. 与列表/筛选共存：筛选时高亮匹配，不破坏拖动手势

当前静态 demo 缺拖动，大图不可用——本设计解决可导航性。

## 2. 用户与场景

| 角色 | 场景 |
|---|---|
| 整合包作者 | 检查原版/模组进度树是否完整、父子关系是否合理 |
| 策划 | 对照 FTB 式依赖链，规划任务书前置条件 |
| QA | 快速定位某个 advancement 的 parent 链 |

## 3. 信息架构

```text
内容编辑
└─ 进度 Tab（kind=advancement）
   ├─ 顶栏：树选择（冒险/主线/…）· 搜索 · 缩放控件 · 重置视图
   ├─ 画布（主区，可拖可缩）
   │    节点 + 父子连线
   └─ 右栏 Inspector：选中节点详情 + 父链
```

与现有 `ModContentPage`：进度仍是一级子 Tab；树选择为画布内的分段控件（若按 namespace/root 分组）。

## 4. 交互规范

### 4.1 平移（必须）

| 输入 | 行为 |
|---|---|
| 鼠标左键拖空白 | 平移画布 |
| 鼠标左键拖节点 | **不**移动节点（只读预览），改为平移（避免误以为可编辑） |
| 空格 + 拖 | 平移（编辑器习惯） |
| 触控单指拖 | 平移 |
| 触控板双指拖 | 映射为平移（wheel 含 deltaX/deltaY 且非 ctrl） |

实现要点：

- 使用 `transform: translate(tx,ty) scale(k)` 单层变换；节点与 SVG 边共用同一变换
- `pointerdown/move/up` + `setPointerCapture`；`touch-action: none` 于画布
- 拖动中节点加 `cursor: grabbing`，禁止文字选中
- 惯性可选（P2，默认关闭，保证可预期）

### 4.2 缩放

| 输入 | 行为 |
|---|---|
| Ctrl/Cmd + 滚轮 | 以指针为锚点缩放 |
| 触控板 pinch（ctrlKey wheel） | 同上 |
| 工具栏 + / − | 以画布中心缩放 |
| 双击节点 | 放大并居中该节点（×1.5，上限） |
| 双击空白 | 缩小一级（下限） |

- 缩放范围（交互）：`k ∈ [0.35, 2.5]`，默认 `k = 1`
- **适应内容（k_fit）单独计算**：按包围盒与视口求 `k_fit`，允许低至 **0.08**；仅「适应内容」可进入 `[0.08, 0.35)`，之后用户再缩放走交互钳制
- 锚点缩放公式：保持屏幕点下世界坐标不变
- 节点标签在 `k < 0.55` 时隐藏，仅留图标（减负）
- 空格+拖：仅画布 `focus` 时生效，输入框内不劫持

### 4.3 视图操作

| 操作 | 行为 |
|---|---|
| 重置视图 | 回到根节点附近，k=1 |
| 适应内容 | 计算节点包围盒，padding 48px，k 钳制在范围内 |
| 点击节点 | 选中 + Inspector；不自动飞行（避免拖动后乱跳） |
| 搜索命中 | 列表外高亮；可选「定位到第一个命中」按钮 |

### 4.4 手势冲突

| 冲突 | 裁决 |
|---|---|
| 滚轮 vs 缩放 | 无 ctrl → 纵向平移；有 ctrl → 缩放 |
| 点击 vs 拖动 | 位移 > 4px 或按住 > 180ms 且移动 → 拖动；否则 click |
| 搜索框滚轮 | 不劫持页面滚动 |

## 5. 布局算法（只读树）

- 按 `parent` 建有向森林；缺 parent 的为根
- DFS 后序：叶子 `y` 连续；父节点 `y = 均值(子)`
- `x = depth * 152`，`y 同层间距 92`
- 多棵树（多个 root）垂直间隔 **≥92**（与同层间距一致）
- 画布内容包围盒决定滚动范围；变换层最小尺寸 = 包围盒 * max(k, k_fit_min) + margin

### 5bis. 真实数据树分组与脏数据（盲审必补）

从 pack 内 `kind=advancement` 结果组装 `trees[]`：

1. 解析 `payload.parent` / `display` / `criteria` / `rewards`；`id` 用 content key
2. **分组优先级**：
   - A. `id` 路径第二段有稳定 root 且存在对应 root 节点（如 `adventure/root`）→ 按该 root 分树
   - B. 否则按 `id` 的 namespace + 路径第一段（`minecraft:adventure`）
   - C. 兜底：单树「全部」
3. **脏数据**：
   - `parentId` 指向不存在节点 → 该节点视为根，不丢弃
   - **环**：遍历染色检测；打断回边，节点提升为根，并在画布错误条提示「已打断 N 处循环引用」
   - 缺 `display` → 标题回退 `shortName(id)`；缺 icon → frame 色块 + 首字占位
4. recipes 类扁平树：M1 不做聚类，但验收夹具必须含宽扁大树

## 6. 视觉

对齐 `docs/design/design-system.md` + 原版进度气质，但不抄像素字体：

| 项 | 值 |
|---|---|
| 画布底 | 深矿物色 `#1c221e`，仅画布区深色 |
| 节点 | 56×56，圆角 6，边框区分 task/goal/challenge |
| 连线 | 贝塞尔，2px；选中相关 3px 金色 |
| 工具条 | 画布右下角浮动：缩放 % / + / − / 适应 / 重置 |
| 阴影 | 仅浮起工具条；节点用描边+内阴影 |

## 7. 状态

| 状态 | 表现 |
|---|---|
| 加载 | 与列表同骨架，画布空态一句引导 |
| 空数据 | 「该包暂无进度数据，先解析模组」 |
| 筛选无命中 | 节点全 dim + 顶栏提示 |
| 错误 | 画布内错误条，保留重试 |

## 8. 性能

| 项 | 预算 |
|---|---|
| 节点 | 原版单树可 >200；DOM 节点 + SVG path，单次 layout O(n) |
| 拖动 | 仅改容器 transform，不重排节点 DOM |
| 缩放 | 同 transform；工具条数字节流更新 |
| 若 n>400 | P2：虚拟化仅渲染视口内节点（本期不做，接口预留） |

## 9. 可访问性

- 画布 `role="application"` + 说明
- 键盘：方向键平移 24px，`+/-` 缩放，`0` 重置，Tab 进入节点列表（侧栏可聚焦节点）
- 焦点环可见；不依赖颜色单独区分 frame 类型（图标+文案）

## 10. 组件 API（接入 ModContentPage）

```ts
type AdvNode = {
  id: string;
  parentId: string | null;
  title: string;
  description?: string;
  iconUrl?: string | null;
  frame?: 'task' | 'goal' | 'challenge';
  criteria?: string[];
  rewards?: Record<string, unknown>;
  done?: boolean; // 仅演示/未来玩家态
};

function AdvancementTreeView(props: {
  trees: { id: string; label: string; nodes: AdvNode[] }[];
  getIcon?: (itemId: string) => string | null;
  onSelect?: (node: AdvNode | null) => void;
}): JSX.Element;
```

数据源：`listModContent(packId, modId, {kind:'advancement'})` + `payload.parent/display/criteria`；`getIcon` 接现有 catalog/icon resolve。

## 11. 验收标准（v1.1 可测化）

1. **合成夹具** `fixture-adv-wide-flat`：≥220 节点、宽扁结构；拖动时仅改变换层 `transform`，节点 DOM 不被重排（代码审查 + 拖动前后节点 `offsetLeft` 不变）
2. Ctrl+滚轮以指针为中心缩放；工具条显示 `k` 为百分比（aria-label 含 percent）
3. 点击节点不触发误平移（位移阈值 4px）；拖空白/拖节点均可平移
4. 「适应内容」：对 fixture 计算 `k_fit`，**允许 k_fit&lt;0.35**，适应后整树包围盒落在视口内（含 48px padding）
5. 选中后 Inspector 显示 title/id/criteria/父链
6. 筛选不破坏拖动；无命中有提示
7. 环/缺失 parent：不崩溃；环被提升为根并有提示
8. 单元测试：布局深度、森林、环打断、k_fit 公式、锚点缩放公式
9. 盲审通过后开发；测试代理与验收代理分别签字

### 11bis. 性能可观测约束

- 拖动帧内禁止读取 `offsetTop/offsetLeft` 布局属性写回（lint/review 检查）
- 拖动中不调用完整 `renderGraph()` 重建节点；只更新容器 transform（筛选/切换树时才重建）

## 12. 风险

| 风险 | 缓解 |
|---|---|
| 原版 recipes 树扁平且节点极多 | 默认对 recipes 根下按 path 二级聚类（可展开）—— **本期 P2，验收不阻塞** |
| 与只读内容页职责混淆 | 明确只读，不做连线拖拽编辑 |
| 浏览器缩放与页面滚动冲突 | 仅画布内 preventDefault ctrl+wheel |

## 13. 里程碑

| 阶段 | 内容 |
|---|---|
| M1 | 布局算法 + 静态树 + 拖动/缩放/工具条 + Inspector |
| M2 | 接真实 advancement API + 图标 + 筛选 |
| P2 | recipes 聚类、键盘增强、虚拟化 |

## 14. 开放问题 — 盲审裁决（已合入）

1. **只读是否拖节点改坐标？** **否。** 拖节点=平移；编辑坐标另立项。
2. **recipes 聚类是否 M1？** **否，保持 P2**；M1 验收必须用宽扁 ≥220 节点夹具。
3. **k 下限 0.35？** **交互保持 0.35；适应允许 k_fit≥0.08。**

## 15. 盲审结论摘要

- 结论：**有条件通过**（已按严重问题修订本稿）
- 交互核心（transform + pointer capture）可实现
- 开发前已补：k_fit 分离、树分组/脏数据、可测验收、环处理
- files: `docs/design/advancement-tree-blind-review-packet.md`
