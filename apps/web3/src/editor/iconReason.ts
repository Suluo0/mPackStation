/* 后端图标渲染器给出的失败原因（item_model_resources.go / 迁移 0026）翻成人话。
   独立成模块而不是挂在 ItemGrid 里：那个文件同时导出组件（ItemGrid）和普通函数
   （iconReasonText），会破坏 React Fast Refresh 的「同构导出」前提 ——
   vite 只能放弃热替换、把整棵树 invalidate 重建。表现就是改一行样式，
   全站状态被清空、Context 出现新旧两份而组件读到 null，
   报出「usePackSummary 必须在 PackSummaryProvider 内使用」这类假错误。 */

const ICON_REASON: Record<string, string> = {
  unsupported_model: '模型结构渲染器不支持',
  missing_texture: '缺少贴图文件',
  requires_tint: '需要生物群系着色（颜色由游戏运行时决定）',
  unsupported_rotation: '模型旋转角度渲染器不支持',
};

export function iconReasonText(reason: string): string {
  if (!reason) return '';
  if (reason.startsWith('runtime_generated_model')) return '由游戏运行时渲染（方块实体：箱子 / 床 / 旗帜 这类）';
  if (reason.startsWith('loader:')) return `加载器 ${reason.slice('loader:'.length)} 未提供贴图`;
  return ICON_REASON[reason] ?? reason;
}
