# Legacy Assets 迁移与接入 TODO

## [ ] P1: 多方块可视化编辑器接入内容工作台
- **目标**：将 `legacy-assets/frontend/multiblocks/`（`IsoPreview.tsx`, `GridPreview.tsx`）接入 `mPackStation` 的 `ContentEditorPage`。
- **任务细项**：
  - [ ] 在 `apps/web/src/features/content/` 创建 `multiblocks/` 目录并平移组件；
  - [ ] 对齐 mps 的 Design Token（CSS 变量）与样式体系；
  - [ ] 当 `kind === 'structure'` 时，由 `ContentEditorPage` 挂载可视化编辑器并与 `useContentEditor` 状态联动。

## [ ] P2: 模组搜索框支持粘贴 URL 智能解析
- **目标**：参考 `legacy-assets/services/parseUrl.ts`，为搜索功能增强 URL 识别能力。
- **任务细项**：
  - [ ] 在 Go 后端 `searchsvc` / `mods.go` 中移植 URL 解析正则（CF / MR / MC百科）；
  - [ ] 若用户在搜索框输入/粘贴 http(s) URL，优先按 URL 精准匹配并反查平台返回单卡。

## [ ] P3: 整合包深度差分对比工具集成
- **目标**：将 `legacy-assets/scripts/compare-mrpacks.ps1` 纳入 `scripts/` 工具库。
- **任务细项**：
  - [ ] 规范化脚本参数，支持相对路径；
  - [ ] 配套编写 Bash 版本（`.sh`）以符合开发规范；
  - [ ] 接入构建测试流水线作为可选验证步骤。

## [ ] P4: 全直链 mrpack 导出模式支持
- **目标**：参考 `legacy-assets/scripts/export-mrpack-all-links.py` 优化 `buildsvc`。
- **任务细项**：
  - [ ] 在 Go 后端打包服务中增加"直链瘦身模式"导出逻辑；
  - [ ] 保证对 PCL2 / HMCL 等国内主流启动器的友好兼容。
