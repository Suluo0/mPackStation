# web2 第一步 · 框架层实施方案

> 三步「新开」路线的第 1 步。设计权威 = `docs/design/workbench-interaction-design.md` v2.1。
> 第一步只做一件事：**一个空框架，能跑起来、能打通后端、令牌可用**。不做任何界面。
> 执行者：任意模型。本文所有命令都可直接粘贴，所有判据都可机器核查。
>
> 三步分工：
> - 第一步（本文）框架层：构建配置 + 路由骨架 + API 客户端 + 设计令牌
> - 第二步 `web2-step2-shell-and-pages.md` 主体层：外壳 + 页面容器 + 状态层（全新写，不参考旧实现）
> - 第三步 `web2-step3-domain-widgets.md` 叶子层：领域组件（新写为主，用 headless 脚本当规格）

---

## 0. 铁律（违反任意一条即视为失败，回滚重做）

1. **不改后端一行**。`apps/server/`、`launcherCore/` 全部禁改。前端契约向后端现状对齐，不对齐就是前端的错。
2. **不碰这些端口**：5173 / 18765 / 18766 / 18871。**不碰 `/tmp/mpack-data`**（开发库）。
3. **不修改 `apps/web/` 下任何文件**。它是回退基线，一直保留到第三步验收通过、用户拍板替换为止。
4. **令牌只从环境变量注入**，禁止写进任何文件——包括 `.env`、`vite.config.ts`、源码、README、验收脚本。需要令牌时用命令现取（§6）。
5. **不 git commit / 不 git add**，除非用户明确要求。（本仓有并发写入者，`git add -A` 会带走别人的改动。）
6. **新增文件只准在 `apps/web2/` 下**。第一步结束时 `apps/web2` 之外零新增、零修改。
7. **不装新依赖**。依赖清单在 §4.1 固定；要加任何东西先停下来问。
8. **每节验收不绿不进下一节**。验收命令的输出要原样贴回给用户，不要只说"通过了"。

---

## 1. 现状（已核实的事实，直接用，不要再猜）

### 1.1 现有前端 `apps/web` 的框架层

| 事实 | 出处 |
|---|---|
| 依赖 6 个：`antd ^6.6.1`、`@ant-design/icons ^6.3.2`、`react ^19.2.0`、`react-dom ^19.2.0`、`react-router-dom ^7.9.0`、`zod ^4.1.0` | `apps/web/package.json` |
| 开发依赖 6 个：`@types/react`、`@types/react-dom`、`@vitejs/plugin-react ^5.0.0`、`playwright-core ^1.63.0`、`typescript ^5.9.0`、`vite ^7.1.0` | 同上 |
| `vite.config.ts` 33 行：`define.__MPACK_WRITE_TOKEN__` 注入写令牌；`server.host='0.0.0.0'`、`port=5273`；`proxy['/api'].target = VITE_API_TARGET \|\| 'http://127.0.0.1:18871'`；**`changeOrigin` 必须 false** | `apps/web/vite.config.ts:9-32` |
| `changeOrigin:false` 的原因：后端用透传的 Host 判定同源，改写成 true 后局域网 Origin 会被判跨站 403 | `apps/web/vite.config.ts:26-27` 注释 |
| 令牌解析顺序：`VITE_MPACK_TOKEN` 环境变量优先 → 否则读 `../../data/runtime-token` → 都没有则空字符串（**无硬编码兜底**） | `apps/web/vite.config.ts:9-16` |
| `tsconfig.json`：`target ES2022`、`moduleResolution bundler`、`strict`、`noUnusedLocals`、`noUnusedParameters`、`noFallthroughCasesInSwitch`、`verbatimModuleSyntax`、`allowImportingTsExtensions`、`jsx react-jsx`、`include:["src"]` | `apps/web/tsconfig.json` |
| API 客户端 14 文件 1025 行；内核是 `api/http.ts` 112 行（`ApiError` / `parseResponse` / `readError` / `WRITE_TOKEN` / `tokenHeaders` / `get` / `post` / `put` / `patch` / `del` / `putVoid`） | `apps/web/src/api/` |
| 设计令牌约 40 个 CSS 变量在 `:root`（圆角/间距/中性色/品牌色/语义色/阴影/动效） | `apps/web/src/features/dashboard/dashboard.css:4-59` |
| headless 验收脚本 8 个共 1733 行（`test-adv-layout` 454、`test-viewport` 279、`adv-tree-e2e.mjs` 220、`test-quest-deps` 115、`test-quest-canvas-edges` 108、`questGraph.check` 91、`questM1SaveBody` 19 + 2 个 png） | `apps/web/scripts/` |

### 1.2 现有前端的残渣（第一步不迁，第三步也不迁）

| 文件 | 行数 | 判定 |
|---|---|---|
| `src/features/content/advTreeDemo.tsx` | 108 | **死文件**，全仓 0 处 import（已核实） |
| `adv-tree-demo.html`（apps/web 根） | — | demo 残渣 |
| `public/design-previews.html` | 12KB | 设计预览残渣 |
| `src/ui/workbench/Workbench.tsx` + `workbench.css` | 25 + 35 | antd 薄包装（`WorkbenchCard`/`WorkbenchSectionHeader`/`WorkbenchButton`），有 2 处消费方但价值只是套 class；web2 直接用 antd + tokens，不要这层 |
| `src/app/AppShell.tsx` 里的 `ModulePlaceholder` | 8 | 零引用 |
| `InspectorRecipePreview`（`src/features/focus/InspectorRail.tsx:156`） | ~10 | 已导出、0 消费方 |

### 1.3 后端事实

| 事实 | 出处 / 实测 |
|---|---|
| 服务启动参数：`-addr`（默认 `127.0.0.1:18871`）、`-data`（默认 `../../data`），也认 `MPACK_DATA` 环境变量 | `apps/server/cmd/server/main.go:90-93` |
| **隔离栈此刻正在跑**：`/tmp/mpack-chain-server -addr 127.0.0.1:18872 -data /tmp/mpack-chain`，`GET /api/health` → 200 | `ps` + `curl` 实测 2026-10-01 |
| 隔离栈数据库 257MB（`/tmp/mpack-chain/mpackstation.db`），**含真实解析过的模组内容**，开发界面比空库好用得多 | `ls -la /tmp/mpack-chain/` |
| 鉴权：GET/HEAD/OPTIONS 免令牌，**例外是 `/api/fs/browse` 需令牌**；写一律需 `X-MPack-Token` | `apps/server/internal/httpapi/httpapi.go`（`tokenRequiredForRead`） |
| 实测：无令牌 `POST /api/packs` → **401**；无令牌 `GET /api/fs/browse?path=/tmp` → **401**；带令牌 → **200** | curl 实测 2026-10-01 |
| 隔离栈的令牌值 = `scripts/chain-test-run.sh` 里 `MPACK_TOKEN` 的默认值（形如 `chain-token-<日期>`）。**用命令现取，不要抄进任何文件** | `scripts/chain-test-run.sh:14` |
| 隔离栈没有 `runtime-token` 文件（启动时走了 `MPACK_TOKEN` 环境变量），所以 §6 的取令牌方式是从脚本里 grep | `ls /tmp/mpack-chain/` 实测 |
| 错误信封统一 `{"error":{"code":...,"message":...}}`；`api/http.ts` 已按此解析成 `ApiError(status, code)` | `apps/web/src/api/http.ts:30-52` |

---

## 2. 决策（有异议就在动手前提出，动手后不要擅自改）

| 编号 | 决策 | 理由 |
|---|---|---|
| **D1** | 新建 `apps/web2/`，与 `apps/web/` **并存** | 旧的不动 = 随时能对比、能回退。第三步验收通过后再由用户拍板是否替换 |
| **D2** | web2 的 dev 端口 = **5274** | 5273 是 `apps/web` 的；两个前端可能同时开着对比 |
| **D3** | proxy 默认目标 = **`http://127.0.0.1:18872`**（隔离栈），`VITE_API_TARGET` 可覆盖 | 18871 是禁碰的开发后端；18872 已在跑且有真实数据 |
| **D4** | 只迁三类资产：**`src/api/` 全量** · **设计令牌 `:root`** · **`public/paper-noise.svg`** | 这三样不是交互代码：api 是对上后端 172 条链路测试的**已验证契约**，令牌是视觉常量，svg 是素材。第二步、第三步要新写的是外壳、页面、领域组件，一行都不迁 |
| **D5** | CSS 组织改为 `styles/tokens.css`（只放变量）+ `styles/base.css`（只放 reset 与全局）+ 每个组件同名 css | 现状 `dashboard.css` 465 行把令牌和组件样式混装，令牌被埋在一个页面文件里，新框架不该继承这个结构 |
| **D6** | `scripts/`（1733 行 headless 验收）**第一步不迁**，第三步随被测模块一起迁 | 它们测的是 `advLayout`/`viewport`/`questGraph`，这些模块第三步才存在。现在迁过来就是一堆跑不红的死测试 |
| **D7** | 第一步的路由只挂 **5 条 + 占位组件**，不做重定向表 | 旧路由重定向是"从旧前端迁移"的事，web2 没有历史包袱，不需要 |

> **D4 的唯一替代方案**见 §8。如果用户否决迁移 `api/`，第三步的工作量会显著上升，且失去"契约已验证"这个保证。

---

## 3. 目标目录结构（第一步结束时长这样）

```
apps/web2/
├── package.json
├── vite.config.ts
├── tsconfig.json
├── index.html
├── public/
│   └── paper-noise.svg          ← 迁入
└── src/
    ├── vite-env.d.ts
    ├── main.tsx
    ├── styles/
    │   ├── tokens.css           ← 令牌（迁 + 改 3 个变量）
    │   └── base.css             ← 新写
    ├── api/                     ← 14 文件原样迁入，零改动
    └── app/
        ├── router.tsx           ← 新写：5 条路由
        └── Placeholder.tsx      ← 新写：临时占位，第二步删除
```

第一步结束时 `src` 下**只有** `api/`（迁入）+ `app/router.tsx` + `app/Placeholder.tsx` + `main.tsx` + `vite-env.d.ts` + 2 个 css。没有任何页面、没有任何 feature。

---

## 4. 具体文件（逐个可粘贴）

### 4.1 `apps/web2/package.json`

```json
{
  "name": "mpackstation-web2",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "tsc --noEmit"
  },
  "dependencies": {
    "@ant-design/icons": "^6.3.2",
    "antd": "^6.6.1",
    "react": "^19.2.0",
    "react-dom": "^19.2.0",
    "react-router-dom": "^7.9.0",
    "zod": "^4.1.0"
  },
  "devDependencies": {
    "@types/react": "^19.2.0",
    "@types/react-dom": "^19.2.0",
    "@vitejs/plugin-react": "^5.0.0",
    "playwright-core": "^1.63.0",
    "typescript": "^5.9.0",
    "vite": "^7.1.0"
  }
}
```

与 `apps/web` 的差别只有 `name` 和**去掉了 4 条 test:* 脚本**（那些脚本第三步才迁，见 D6）。依赖版本一字不改。

### 4.2 `apps/web2/vite.config.ts`

```ts
import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';

/* 写操作令牌注入(auth.md 决策 D-8)：VITE_MPACK_TOKEN 环境变量优先；否则读后端数据目录的
   runtime-token。禁止任何硬编码兜底 —— 注入为空时写请求会被后端 401，错误如实抛给界面。 */
function resolveWriteToken(): string {
  if (process.env.VITE_MPACK_TOKEN) return process.env.VITE_MPACK_TOKEN;
  try {
    return readFileSync(process.env.MPACK_TOKEN_FILE ?? resolve(__dirname, '../../data/runtime-token'), 'utf8').trim();
  } catch {
    return '';
  }
}

const token = resolveWriteToken();
if (!token) {
  console.warn('[web2] 写令牌为空：读接口可用，写接口会被后端 401。按方案 §6 的方式注入 VITE_MPACK_TOKEN。');
}

export default defineConfig({
  plugins: [react()],
  define: {__MPACK_WRITE_TOKEN__: JSON.stringify(token)},
  server: {
    host: '0.0.0.0',
    port: 5274,
    proxy: {
      // changeOrigin 必须保持 false：后端用透传的 Host 判定同源，改写后局域网 Origin 会被判成跨站 403。
      '/api': {target: process.env.VITE_API_TARGET || 'http://127.0.0.1:18872', changeOrigin: false},
    },
  },
});
```

与旧配置的三处差别，都是有意的：`port 5274`、默认 target `18872`、多一个 `MPACK_TOKEN_FILE` 环境变量入口（隔离栈没有 `runtime-token` 文件，见 §1.3）+ 令牌为空时告警。

### 4.3 `apps/web2/tsconfig.json`

与 `apps/web/tsconfig.json` **逐字一致**（`cp` 即可）。`strict` + `noUnusedLocals` + `noUnusedParameters` 是防漂移的电门，一个都不许关。

```bash
cp /Volumes/Evo/code/mPackStation/apps/web/tsconfig.json /Volumes/Evo/code/mPackStation/apps/web2/tsconfig.json
```

### 4.4 `apps/web2/index.html`

```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>工作台 · MC 整合包设计工具</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

### 4.5 `apps/web2/src/vite-env.d.ts`

```ts
/// <reference types="vite/client" />

/** 构建期由 vite.config.ts 的 define 注入。为空表示未配置令牌，写请求会 401。 */
declare const __MPACK_WRITE_TOKEN__: string;
```

### 4.6 `apps/web2/src/styles/tokens.css`

从 `apps/web/src/features/dashboard/dashboard.css:4-59` 复制整个 `:root` 块，然后做**且只做**这三处改动：

| 动作 | 变量 | 原因 |
|---|---|---|
| **删** | `--mc-sider-w: 220px` | v2.1 没有左侧导航栏 |
| **删** | `--mc-sider-w-fold: 72px` | 同上 |
| **加** | `--mc-topbar-h: 48px` | v2.1 顶栏高度 |
| **加** | `--mc-rail-w: 320px` | 右栏展开宽度 |
| **加** | `--mc-rail-w-fold: 56px` | 右栏折叠宽度 |
| **加** | `--mc-src-rail-w: 260px` / `--mc-src-rail-w-fold: 56px` | 内容页页内「来源栏」展开/折叠宽度（设计文档 §4.3） |

其余变量（圆角/间距/中性色/品牌色/语义色/阴影/动效）**一字不改**——语义色注释里写明了「绿=已解决，红/橙=待解决，蓝=可更新」，这是全站信号配色约定，改了会破坏一致性。

### 4.7 `apps/web2/src/styles/base.css`

```css
/* 只放全局 reset 与排版基线。任何组件样式都不许写在这里。 */
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--mc-bg);
  color: var(--mc-text);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif;
  font-size: 14px;
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}
a { color: inherit; text-decoration: none; }
button { font: inherit; }
```

### 4.8 `apps/web2/src/main.tsx`

```tsx
import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import {ConfigProvider} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {router} from './app/router';
import {RouterProvider} from 'react-router-dom';
import './styles/tokens.css';
import './styles/base.css';

/* antd 的主题变量必须引用 tokens.css 的同一套值，否则会出现「antd 组件一个色、
   自定义组件另一个色」。这里只映射主色与圆角，其余走 CSS 变量。 */
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={{token: {colorPrimary: '#c9783b', borderRadius: 12}}}>
      <RouterProvider router={router}/>
    </ConfigProvider>
  </StrictMode>,
);
```

> `colorPrimary` 与 `borderRadius` 的字面值必须与 `tokens.css` 的 `--mc-primary` / `--mc-radius` 一致。antd 的 theme token 不能吃 CSS 变量，这是唯一允许出现字面色的地方，改令牌时要同步改这里。

### 4.9 `apps/web2/src/app/router.tsx`

```tsx
import {createBrowserRouter} from 'react-router-dom';
import {Placeholder} from './Placeholder';

/* v2.1 的导航一共 5 项可点（设计文档 §4.5）：工作台 / 整合包 / 概览 / 内容 / 交付 + ⚙。
   第一步只挂路由骨架，页面实体在第二步写。包级 3 条共用 :id。 */
export const router = createBrowserRouter([
  {path: '/', element: <Placeholder name="工作台"/>},
  {path: '/packs', element: <Placeholder name="整合包"/>},
  {path: '/packs/:id', element: <Placeholder name="概览"/>},
  {path: '/packs/:id/content', element: <Placeholder name="内容"/>},
  {path: '/packs/:id/delivery', element: <Placeholder name="交付"/>},
  {path: '/settings', element: <Placeholder name="设置"/>},
]);
```

**这 6 条就是 web2 的全部路由**（5 个导航项 + ⚙ 设置）。第二步不许再加顶层路由；内容页的四态是 `?mode=` 查询参数，不是路由。

### 4.10 `apps/web2/src/app/Placeholder.tsx`

```tsx
import {useParams} from 'react-router-dom';

/* 临时占位：第二步会被真实页面替换并删除本文件。存在的意义是让第一步的路由可点验。 */
export function Placeholder({name}: {name: string}) {
  const {id} = useParams();
  return (
    <div style={{padding: 24}}>
      <h1 style={{margin: 0, fontSize: 18}}>{name}</h1>
      <p style={{color: 'var(--mc-muted)'}}>
        web2 第一步骨架 · 路由已通{ id ? ` · packId=${id}` : ''}
      </p>
    </div>
  );
}
```

---

## 5. `api/` 迁移（D4）

### 5.1 命令

```bash
cd /Volumes/Evo/code/mPackStation
mkdir -p apps/web2/src apps/web2/public
cp -R apps/web/src/api apps/web2/src/api
cp apps/web/public/paper-noise.svg apps/web2/public/paper-noise.svg
```

### 5.2 允许的改动：**零**

14 个文件一个字都不许改。它们是与后端对齐的契约层，改动必须先在 Go 侧找到事实依据。

### 5.3 迁移后必须核查（三条都要过）

```bash
cd /Volumes/Evo/code/mPackStation
# ① 文件数与行数与源一致
diff <(cd apps/web/src/api && wc -l *.ts) <(cd apps/web2/src/api && wc -l *.ts) && echo "api 迁移一致"
# ② 没有残留指向旧目录的相对导入
grep -rn "from '\.\./\.\./" apps/web2/src/api || echo "无跨层导入(api 层本来就该自含)"
# ③ 没有任何硬编码令牌
grep -rniE "chain-token|runtime-token|X-MPack-Token:\s*['\"][A-Za-z0-9]" apps/web2/src || echo "无硬编码令牌"
```

---

## 6. 开发环境怎么起（隔离，绝不碰开发栈）

```bash
cd /Volumes/Evo/code/mPackStation

# ① 确认隔离后端在跑（不在跑就用 scripts/chain-test-run.sh 起，或按 §6.1 手动起）
curl -s -o /dev/null -w 'health=%{http_code}\n' http://127.0.0.1:18872/api/health   # 期望 200

# ② 令牌现取到环境变量，不落盘、不写进任何文件
export VITE_MPACK_TOKEN=$(grep -o 'chain-token-[0-9]*' scripts/chain-test-run.sh | head -1)
[ -n "$VITE_MPACK_TOKEN" ] && echo "令牌已注入(值不打印)" || echo "令牌为空 —— 停下来查"

# ③ 装依赖并起 web2
cd apps/web2 && npm install && npm run dev      # → http://127.0.0.1:5274
```

### 6.1 如果隔离后端没在跑

```bash
cd /Volumes/Evo/code/mPackStation
export MPACK_TOKEN=$(grep -o 'chain-token-[0-9]*' scripts/chain-test-run.sh | head -1)
( cd apps/server && go build -o /tmp/mpack-chain-server ./cmd/server )
/tmp/mpack-chain-server -addr 127.0.0.1:18872 -data /tmp/mpack-chain &
```

**只准用 `-data /tmp/mpack-chain`**。用 `/tmp/mpack-data` 就是碰开发库，属于铁律 2 违规。

---

## 7. 验收（全部命令，输出原样贴回）

```bash
cd /Volumes/Evo/code/mPackStation/apps/web2

# A. 类型检查 0 错误
npm run test

# B. 生产构建成功
npm run build

# C. dev 起着时，代理通到隔离后端
curl -s -o /dev/null -w 'proxy_health=%{http_code}\n' http://127.0.0.1:5274/api/health      # 期望 200
curl -s http://127.0.0.1:5274/api/packs | head -c 200; echo                                  # 期望 JSON

# D. 鉴权语义与后端一致
curl -s -o /dev/null -w 'no_token_write=%{http_code}\n' -X POST http://127.0.0.1:5274/api/packs \
  -H 'content-type: application/json' -d '{}'                                                # 期望 401
curl -s -o /dev/null -w 'no_token_browse=%{http_code}\n' 'http://127.0.0.1:5274/api/fs/browse?path=/tmp'  # 期望 401
curl -s -o /dev/null -w 'with_token_browse=%{http_code}\n' -H "X-MPack-Token: $VITE_MPACK_TOKEN" \
  'http://127.0.0.1:5274/api/fs/browse?path=/tmp'                                            # 期望 200

# E. 铁律核查：改动范围只在 apps/web2
cd /Volumes/Evo/code/mPackStation
git status --porcelain | grep -v '^?? apps/web2/' | grep -v '^ M docs/' || echo "改动范围合规"

# F. 铁律核查：apps/web 一个字没动
git status --porcelain apps/web | grep -v 'pack-pages.css\|ModContentPage\|PackPages\|LauncherPage\|AppShell\|shell.css\|main.tsx\|dashboard\|QuestBookEditor\|api/' || echo "apps/web 无本轮新增改动"

# G. 死代码/残渣没被带进来
grep -rn 'advTreeDemo\|ModulePlaceholder\|ui/workbench' apps/web2/src || echo "无残渣"

# H. 路由数量 = 6，没有多余顶层路由
grep -c "path: '" apps/web2/src/app/router.tsx    # 期望 6
```

### 人工点验（3 条，浏览器里做）

1. 打开 `http://127.0.0.1:5274/` → 看到「工作台 · web2 第一步骨架」。
2. 手输 `http://127.0.0.1:5274/packs/anything/content` → 看到「内容 · packId=anything」。证明 `:id` 通。
3. 打开 devtools Console → **无红色报错**；启动 dev server 的终端里**没有** `[web2] 写令牌为空` 告警（有就说明 §6 的令牌没注入成功）。

---

## 8. 决策点（已拍板，实施时不要停下来问）

**D4：`api/` 1025 行原样迁过来 → 迁。** 2026-10-01 用户批复"按推荐的来就行"，本文默认值即为结论，不需要再回话确认。

| 选项 | 代价 | 收益 |
|---|---|---|
| **迁（结论）** | 1025 行旧代码留在新框架里 | 契约已被后端 172 条链路测试验证过；第三步只写 UI，不用重新推导 20+ 个端点的字段形状 |
| ~~不迁，从 Go handler 重写 zod schema~~ | 需要逐个读 `apps/server/internal/httpapi/routes_*.go` 反推响应结构，约 20+ 端点；重写完仍是 ~1000 行（zod 逐字段声明压不下去）；且**新写的 schema 未经验证**，第一次联调必然踩字段名/可空性错误 | 新框架里一行旧代码都没有 |

理由：`api/` 里没有交互逻辑，它是**契约的机器可读副本**，重写它不会让界面变好，只会把"已验证"变成"未验证"。

---

## 9. 禁止清单（第一步碰了就是错）

- 禁改 `apps/server/`、`launcherCore/`、`apps/web/`、`scripts/`
- 禁碰端口 5173 / 18765 / 18766 / 18871
- 禁用 `-data /tmp/mpack-data` 起任何后端
- 禁把令牌写进任何文件（含 `.env`、注释、验收脚本、本方案的替换值）
- 禁 `git add` / `git commit`
- 禁装 §4.1 之外的依赖
- 禁在第一步写任何页面、任何 feature 组件、任何业务 CSS
- 禁关闭 tsconfig 的 `strict` / `noUnusedLocals` / `noUnusedParameters`
- 禁把 `changeOrigin` 改成 `true`
- 禁增加第 7 条顶层路由

---

## 10. 交给第二步的接口约定

第一步结束时，第二步可以依赖这些既成事实：

| 提供物 | 位置 | 第二步怎么用 |
|---|---|---|
| 路由表 | `src/app/router.tsx` | 把 6 个 `Placeholder` 逐个换成真实页面；**不许加新条目** |
| API 客户端 | `src/api/*.ts` | 直接 import 调用；`ApiError.code` 可用于界面分支（如 `revision_conflict`） |
| 设计令牌 | `src/styles/tokens.css` | 所有尺寸/颜色只准引用变量，禁字面值（唯一例外见 §4.8 的 antd theme） |
| 全局样式 | `src/styles/base.css` | 组件样式一律写在自己的同名 css 里，禁往 base.css 堆 |
| 布局常量 | `--mc-topbar-h` / `--mc-rail-w` / `--mc-rail-w-fold` / `--mc-src-rail-w` / `--mc-src-rail-w-fold` | 第二步的外壳尺寸全部取自这 5 个变量 |
| 隔离环境 | 端口 5274 + 后端 18872 + `VITE_MPACK_TOKEN` | 第二步的点击验收都在这个环境里做 |

**第一步完成的标志**：§7 的 A–H 全绿 + 3 条人工点验通过。（§8 的 D4 已拍板为"迁"，不构成阻塞。）
