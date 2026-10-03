/* 导航项的唯一事实源。TopBar 渲染它，CommandPalette 也用它生成页面命令 ——
   两处各写一份是上一轮"导航膨胀"的根因之一。 */
/* NavScope 单独命名：验收 §7.2-B 用 grep 数导航项条数，
   把取值内联进类型定义会让类型行也被计进去，那个判据就数不准了。 */
export type NavScope = 'global' | 'pack';
export type NavItem = {to: string; label: string; scope: NavScope; keys?: string};

/* 顶栏入口按设计文档 §4 的四个页面能力常驻：工作台 + 概览 / 内容 / 交付。
   「整合包 /packs」先屏蔽（不删）：§4.1 工作台已经把包列表 + 只看待处理 + 新建/导入全包了，
   /packs 是同一能力的第二个入口。路由仍在（router.tsx 未动），第三步接真实 PackList 时再定去留。 */
export const GLOBAL_NAV: NavItem[] = [
  {to: '/', label: '工作台', scope: 'global'},
  // 屏蔽中：{to: '/packs', label: '整合包'}（同上 scope 取值），理由见上注释。
];

/* 包内三项。v2.1 把旧的 10 项收敛成这 3 项，多一项就是违反设计文档 §2 的减法判据。 */
export const PACK_NAV: NavItem[] = [
  {to: '', label: '概览', scope: 'pack'},
  {to: 'content', label: '内容', scope: 'pack'},
  {to: 'delivery', label: '交付', scope: 'pack'},
];

/* 内容页四态。它们是查询参数不是路由（铁律 3）。 */
export const CONTENT_MODES = [
  {mode: 'index', label: '索引', hotkey: ''},
  {mode: 'graph', label: '关系', hotkey: 'Enter'},
  {mode: 'edit', label: '魔改', hotkey: 'E'},
  {mode: 'quest', label: '编排', hotkey: 'Q'},
] as const;

export type ContentMode = typeof CONTENT_MODES[number]['mode'];

/** 包内导航的绝对路径：PACK_NAV 的 to 是相对包根的，'' 即概览页。 */
export function packHref(packId: string, to: string): string {
  const base = `/packs/${encodeURIComponent(packId)}`;
  return to ? `${base}/${to}` : base;
}

/** ⌘K 的「页面/态命令」由这里生成（§5.6），标签只有上面三组常量一个来源。
 *  入参是锚点包（当前包或最近编辑包）；一个包都没有时只出全局项 + 设置。 */
export function pageCommands(anchorPackId: string | null): {label: string; href: string; mode?: ContentMode}[] {
  const out: {label: string; href: string; mode?: ContentMode}[] = GLOBAL_NAV.map(n => ({label: n.label, href: n.to}));
  if (anchorPackId) {
    out.push({label: PACK_NAV[0].label, href: packHref(anchorPackId, PACK_NAV[0].to)});
    for (const m of CONTENT_MODES) {
      out.push({label: `内容·${m.label}`, href: packHref(anchorPackId, 'content'), mode: m.mode});
    }
    out.push({label: PACK_NAV[2].label, href: packHref(anchorPackId, PACK_NAV[2].to)});
  }
  out.push({label: '设置', href: '/settings'});
  return out;
}
