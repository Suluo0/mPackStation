# 主题管理专项实现方案

> 关联：`docs/frontend-refactor/00-plan.md` 第 7 节 B2（排在组件封装之前，因为所有组件都依赖 token）。
> 用户提问：颜色/底色/按钮/hover 是否有对应处理？换主题能力本轮一起规划掉。

---

## 1. 现状结论（已查明，别重复查）

**好消息：设计令牌已经存在且规范。**

`apps/web3/src/styles/tokens.css`（69 行）在 `:root` 上定义了完整的一层：

- 圆角：`--mc-radius` / `--mc-radius-panel` / `--mc-radius-pill`
- 间距刻度：`--mc-sp-xs` … `--mc-sp-xl`
- 布局：`--mc-topbar-h` `--mc-railicon-w` `--mc-panel-w` `--mc-statusbar-h` `--mc-dock-h`
- 中性色：`--mc-text` `--mc-text-2` `--mc-muted` `--mc-bg` `--mc-fill` `--mc-fill-2` `--mc-line` `--mc-line-strong` `--mc-hover`
- 品牌色：`--mc-primary` `--mc-primary-deep` `--mc-primary-bg`

文件头部注释还明确了「语义色注释是全站信号配色约定，不许改值」。

### 1.1 实测：收编工作量 ≈ 0（2026-10-04 扫描结果）

| 文件 | 裸十六进制 | `rgb()/hsl()` | `var(--mc-*)` 引用 |
|---|---|---|---|
| `editor/editor.css` (1086 行) | **0** | 5（阴影类） | 313 处 |
| `app/frame.css` (471 行) | **0** | 1 | 165 处 |
| `panels/panels.css` (287 行) | **0** | 1 | 69 处 |
| `styles/base.css` | **0** | 0 | 6 处 |
| `styles/tokens.css` | 全部（**这是定义处**） | 3（阴影） | 0 |

`.tsx` 里的内联颜色字面量（`style={{color:'#…'}}`）：**0 处**。

`tokens.css` 已定义的语义色齐全：中性色 + primary + success / fail / orange / gold / blue（各带 `-bg`）+ 阴影 + 动效 + 字体密度。

**所以本专项不设计、不收编，只做三件事：**

1. **加主题层**：把所有颜色从 `:root` 搬到 `themes.css`，按 `[data-theme="light"]` 分组。
2. **加第二套值**：`[data-theme="dark"]` 覆盖同名变量。
3. **加切换入口**：`<html data-theme>` + localStorage + 跟随系统 `prefers-color-scheme`。

（原计划里的「收编残留硬编码」这一步实测不需要，已勾销。）

---

## 2. 目标结构

```
apps/web3/src/styles/
├── tokens.css      基础刻度（圆角/间距/布局尺寸）—— 不随主题变，保留在 :root
├── themes.css      ★新建 · 各主题的颜色层（light / dark / …）
├── base.css        重置与全局（保留）
└── (组件样式逐步迁到 ui/ 组件同目录)
```

**切分原则**：
- **不随主题变**的量（圆角、间距、各类高度宽度）留在 `:root`，主题文件不管。
- **随主题变**的量（文本色、背景、填充、描边、hover、品牌色、信号色）全部搬进 `themes.css`，按主题选择器分组。

---

## 3. 实施步骤

| 步 | 动作 | 判据 |
|---|---|---|
| 1 | 扫描三个大 CSS 里的裸色值：`grep -n '#[0-9a-fA-F]\{3,8\}'`，出清单 | 清单落盘（本文件第 6 节） |
| 2 | 逐个判定：是「已知 token 的重复」→ 直接换成 `var(--mc-*)`；是「漏定义的语义色」→ 先在 tokens 里补一个语义名再换 | 无遗留裸色 |
| 3 | 新建 `themes.css`：把 `:root` 里的颜色层整体搬过去，改写成 `[data-theme="light"]` 分组 | 界面观感与现状一致 |
| 4 | 加 `[data-theme="dark"]` 一整套同名变量 | 切到 dark 时全站换色、无漏网 |
| 5 | 加切换入口：写在设置里，落地为 `<html data-theme="...">`；持久化到 localStorage；默认跟随系统 `prefers-color-scheme` | 刷新保持 |
| 6 | 规范落盘：CSS 里禁止再出现裸十六进制（颜色一律 `var(--mc-*)`） | 见 `docs/standards/frontend-standards.md` |

---

## 4. 语义色命名约定（补齐缺口时用）

已有：text / text-2 / muted / bg / fill / fill-2 / line / line-strong / hover / primary / primary-deep / primary-bg。

建议补齐（若扫描后发现确有用例才建，不要凭空造）：

| 语义名 | 用途 |
|---|---|
| `--mc-success` / `--mc-warn` / `--mc-fail` | 状态灯、健康分、校验结果 |
| `--mc-accent-bg` | 选中态底色（与 hover 区分开） |
| `--mc-overlay` | 弹层遮罩 |
| `--mc-shadow` | 弹层阴影（dark 主题要换） |
| `--mc-code-bg` | ID / 代码块底色 |

**命名规则**：`--mc-<语义>`，不写具体颜色名（不许 `--mc-orange`，要 `--mc-primary`）—— 否则换主题时名字就成了谎言。

---

## 5. 风险与对策

| 风险 | 对策 |
|---|---|
| 1086 行 `editor.css` 量太大，一次改怕漏 | 按 CSS 逐个文件做，每改完一个做一次真浏览器截图对比 |
| dark 主题下对比度不足 | 每个主题落完后逐屏截图核一次，重点看弹层与表格 |
| 组件内联 style 里也有颜色（`style={{background: 'var(--mc-fail)'}}` 之类） | 一并收编；内联 style 只允许引用 token，不允许写字面量 |

---

## 6. 扫描结论（2026-10-04 已执行）

| 检查项 | 结果 | 处置 |
|---|---|---|
| 三个业务 CSS 的裸十六进制 | **0 处** | 无需收编，勾销 |
| `.tsx` 内联颜色字面量 | **0 处** | 无需收编，勾销 |
| `rgb()` / `hsl()` | 共 7 处，全在阴影与品牌阴影 | 随主题一起搬进 `themes.css` |
| 颜色定义集中处 | `styles/tokens.css` 的 `:root` | 整体搬到 `themes.css` 的 light 分组 |

**唯一要做的是「拆分 + 加层」，不是「收编」。**

---

## 7. 进度

- [x] 扫描裸色值清单（结论：0 处，无需收编）
- [x] ~~收编三份业务 CSS~~（勾销，本就全走变量）
- [x] 新建 `styles/themes.css`（`:root`+light 并列兜底 / dark 两套同名变量；含「未设 data-theme 时不闪白」的兜底设计）
- [x] `app/theme.ts`（initTheme / applyTheme / readStoredTheme / tokenValue）
- [x] `main.tsx` 渲染前落主题；antd 令牌改读 CSS 变量（不再双份色值）
- [x] 设置弹窗加「跟随系统 / 浅色 / 深色」切换（`SettingsModal.tsx`）
- [x] 真浏览器验收 PASS 8/0（含深浅切换 body 背景实测、控制台 0 错误）
- [ ] antd 组件深色适配：ConfigProvider 加 `theme.darkAlgorithm`（深色下 antd 控件仍是浅色样式）
- [ ] 多主题扩展时补：`--mc-overlay` / `--mc-code-bg`（现无用例，不凭空造）
