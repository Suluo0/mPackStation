# Legacy Assets 遗留高价值资产提取区

> 本目录存放从历史版本（`mc_dev/tools/mod-pool` 与 `mc_dev/tools/mod-pool-java`）中提炼出的高价值组件、算法与运维脚本，供 mPackStation 演进时参考与接入。

---

## 目录索引

```text
legacy-assets/
├── TODO.md                         # 待办事项与接入规划
├── README.md                       # 本说明文档
├── frontend/
│   ├── multiblocks/                # 多方块 2D/3D 可视化编辑器组件
│   │   ├── GridPreview.tsx         # 2D 分层网格微缩/编辑组件
│   │   ├── IsoPreview.tsx          # 2.5D 轴测纯 CSS 3D 渲染与拖拽交互组件
│   │   ├── MultiblocksPage.tsx     # 多方块主从设计页参考实现
│   │   └── styles-multiblock.css   # 多方块与 3D 轴测相关样式
│   └── mods/
│       └── DetailComponents.tsx    # 模组多平台状态/环境/哈希校验卡片片段
├── services/
│   └── parseUrl.ts                 # 模组落地页 URL 智能反解器（CF / Modrinth / MC百科）
└── scripts/
    ├── compare-mrpacks.ps1         # 跨整合包 (.mrpack) 深度哈希差分对比脚本
    └── export-mrpack-all-links.py  # PCL2 风格全直链 mrpack 转换算法参考
```

---

## 各资产接入指引

### 1. 多方块结构编辑器 (`frontend/multiblocks/`)
- **价值**：零 WebGL/Three.js 依赖，纯 CSS 3D 轴测渲染（旋转、分层切片、控制器拖拽）。
- **接入目标**：`apps/web/src/pages/PackPages.tsx` 中的 `ContentEditorPage`（当内容类型为 `kind === 'structure'` 时使用）。
- **对接服务**：mps 后端 `contentsvc`（`/api/packs/{id}/contents`）。

### 2. URL 智能反解逻辑 (`services/parseUrl.ts`)
- **价值**：支持粘贴 `curseforge.com`、`modrinth.com`、`mcmod.cn` 网页 URL 直接识别并加入模组。
- **接入目标**：`apps/server/internal/service/mods.go`（在 `ModSearchAll` 增加 URL 识别分支）。

### 3. mrpack 深度差分对比脚本 (`scripts/compare-mrpacks.ps1`)
- **价值**：深度比对两个 `.mrpack` 的 manifest 依赖、文件哈希及 overrides。
- **接入目标**：`scripts/` 目录，作为 CI 与发布验收的断言验证脚本。

### 4. 全直链 mrpack 生成算法 (`scripts/export-mrpack-all-links.py`)
- **价值**：将 CurseForge 嵌入式 jar 转为直链清单，大幅减少整合包体积并支持 PCL2 等启动器高速下载。
- **接入目标**：`apps/server/internal/service/build_publish.go`（作为构建导出时的扩展选项）。

### 5. 模组详情 UI 组件片段 (`frontend/mods/DetailComponents.tsx`)
- **价值**：多平台标签三态联动、平台链接与文件版本状态。
- **接入目标**：`apps/web/src/features/` 模组详情抽屉/卡片布局参考。
