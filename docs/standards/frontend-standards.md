# 前端开发规范（apps/web3）

> 本文件是**规范**（规定应该怎么写），不是现状描述。与现状不符的地方以本文件为准，
> 差异项在 `docs/archive/frontend-refactor/00-plan.md` 里逐条迁移。
>
> 立此规范的直接原因：此前「要什么功能就写什么，全是写死的，一点复用性都没有」，
> 导致每修一个 bug 只在原地打补丁，改完还是散的。

---

## 1. 资源唯一来源

| 资源 | 唯一入口 | 禁止 |
|---|---|---|
| 图标 | `<Icon name="…"/>`（`ui/Icon.tsx` 的语义名映射表） | 内联 SVG、emoji/字符当图标（✓ ✕ ▸）、业务侧自绘 path |
| 颜色 | CSS 变量 `var(--mc-*)`（`styles/tokens.css` + `styles/themes.css`） | 裸十六进制、内联 style 里写字面量 |
| 间距/圆角/尺寸 | `--mc-sp-*` / `--mc-radius-*` / `--mc-*-h|w` | 硬编码 px（除 0 与 1px 描边） |
| 文本 | 语言包 key（`i18n/locales/*`） | 硬编码中文，含 `title` / `placeholder` / `aria-label` |
| 平台名/加载器 | `components/ProviderLabel` | 界面直接显示 `modrinth` / `fabric` 这类原始标识 |

新增图标一律用 **lucide-react**，在 `ICONS` 映射表登记语义名；`Icon` 遇到不认识的 name 返回 `null`（不画空 svg）。

---

## 2. 分层与依赖方向

```
ui/            基础组件（零业务）：Icon Button TextInput SearchInput
               Menu ContextMenu Popover Modal Tree Row Empty Chip Tooltip SplitPanel
      ↓
components/    组合组件（带语义、跨面板复用）：
               ItemPicker ItemIcon ModRow SearchBar HealthDock LogDock
               ProviderLabel VersionRail
      ↓
panels/ editor/ 业务面板与编辑器
      ↓
app/            应用骨架：AppFrame router url useHotkeys 各类 Context
```

- 依赖**只能自上而下**。禁止反向引用，禁止跨层（`panels/` 不许互相 import 内部组件，公共部分下沉到 `components/`）。
- `api/` 只放请求函数 + schema + 类型，**禁止出现 JSX**。
- 判定归属的口诀：**被两个以上面板用到 → 下沉**。

---

## 3. 原子类只能由组件产出

`.p-btn` / `.p-row` / `.p-title` / `.p-empty` / `.tp-icon-btn` 这类面板原子类，
**业务 div 不许自己拼**，必须调 `ui/` 组件。目的是让视觉改一次改全局。

---

## 4. 弹层统一口径

所有弹层（菜单 / popover / modal / 下拉）必须满足：

1. 渲染在同一个 Portal 容器（`document.body`），不留在业务 DOM 里。
2. **Esc 由弹层独占**：在**捕获阶段** `stopPropagation`。
   反面案例：全局快捷键 Esc = 「回上一态」，弹层 Esc = 关闭，两边一个挂 `document` 一个挂 `window`，
   结果是关菜单的同时把当前视图也切走了。
3. 外点关闭。
4. 定位统一做 flip / shift，**不得超出视口**。
   反面案例：健康分弹层挂在左下角状态条上却弹在右下角且被裁掉。

---

## 5. 交互统一口径

- **焦点框（选中态）只由显式操作产生**：点击，或方向键走格。滚轮、hover 一律不得移动焦点框。
- **滚轮只在有明确轮换对象的地方拦**（如多候选槽位轮换原材料），其余交给页面滚动。
  必须用原生 `addEventListener('wheel', fn, {passive:false})` —— React 的 `onWheel` 是 passive 的，`preventDefault()` 不生效。
- **`⌘/Ctrl + 滚轮` 一律放行**，留给浏览器缩放。
- **hover 与焦点是两件事**：hover 决定快捷操作作用在哪个对象上，焦点决定高亮框在哪。

---

## 6. 文案与 i18n

- 面向用户的文本一律进语言包，key 格式 `域.对象.语义`（如 `store.search.placeholder`）。
- **禁止拼接 key**：不得 `t('mod.' + kind)`，必须写全 —— 否则提取工具扫不到、改名即失效。
- 插值用命名参数 `{mod}`，禁止位置参数 `{0}`。
- 混排句子必须拆：`Mekanism 仅支持 Forge / NeoForge` 写成
  `t('store.fallback.only_loaders', {name, loaders})`，不要整句塞进语言包（否则加粗/变色做不了）。
- 后端返回 `code + args`，前端查语言包渲染；后端不返回整句文案。

---

## 7. 主题与样式

- 颜色一律 `var(--mc-*)`。语义命名，不写颜色名（要 `--mc-primary`，不要 `--mc-orange`）。
- 不随主题变（圆角/间距/尺寸）放 `tokens.css` 的 `:root`；随主题变（文本/背景/描边/hover/品牌/信号色）放 `themes.css` 按 `[data-theme="…"]` 分组。
- 新增语义色先在 `tokens.css` 补名字，再使用；不要凭空造。

---

## 8. 目录与命名

- 组件文件 PascalCase（`ModRow.tsx`），工具与上下文 camelCase（`useHotkeys.ts`）。
- 组件样式与该组件同目录（`ui/Button.css`），不往三个大 CSS 里继续堆。
- 新增文件先判断属于哪一层，放错层等于制造下一轮技术债。

---

## 9. 禁止项清单（速查）

- ❌ 内联 SVG / emoji 当图标
- ❌ 裸十六进制颜色、内联 style 里的颜色字面量
- ❌ 硬编码中文（含 title / placeholder / aria-label）
- ❌ 拼接 i18n key
- ❌ 业务 div 自己拼 `.p-btn` / `.p-row` 等原子类
- ❌ 弹层不进 Portal / 不处理 Esc / 不防溢出视口
- ❌ React 的 `onWheel` 里调 `preventDefault()`
- ❌ `panels/` 之间互相 import 内部组件

---

## 10. 验收口径

- 改动涉及交互 → **必须真浏览器（CDP）验收**，不能只跑 `tsc --noEmit`。
- 类型检查：`cd apps/web3 && ./node_modules/.bin/tsc --noEmit` 必须 0 错。
- 判断「滚轮该不该被拦」→ 断言 `defaultPrevented`。
- 判断「页面真的滚了」→ 用 `Input.dispatchMouseEvent type=mouseWheel`，合成 `WheelEvent` 不触发默认滚动。
- 控制台错误按视图逐个导航取差，才能定位是哪个视图在报。
- 后端改动：`go test ./internal/...` 全绿 + curl 实测。
