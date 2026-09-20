/* Simple quest icon catalog + display helper.
   Accepts minecraft-style ids (namespace:path). UI shows <img> when a known
   texture/data URL exists; otherwise an aria-label placeholder with the id. */

export type QuestIconEntry = {id: string; label: string; emoji: string};

/** 常用 minecraft 物品/概念图标目录（文本 catalog，可扩展）。 */
export const QUEST_ICON_CATALOG: QuestIconEntry[] = [
  {id: 'minecraft:book', label: '书', emoji: '📕'},
  {id: 'minecraft:writable_book', label: '书与笔', emoji: '📗'},
  {id: 'minecraft:wooden_pickaxe', label: '木镐', emoji: '⛏️'},
  {id: 'minecraft:stone_pickaxe', label: '石镐', emoji: '⛏️'},
  {id: 'minecraft:iron_pickaxe', label: '铁镐', emoji: '⛏️'},
  {id: 'minecraft:diamond_pickaxe', label: '钻石镐', emoji: '💎'},
  {id: 'minecraft:iron_ingot', label: '铁锭', emoji: '🔘'},
  {id: 'minecraft:gold_ingot', label: '金锭', emoji: '🟡'},
  {id: 'minecraft:diamond', label: '钻石', emoji: '💎'},
  {id: 'minecraft:emerald', label: '绿宝石', emoji: '💚'},
  {id: 'minecraft:redstone', label: '红石', emoji: '🔴'},
  {id: 'minecraft:coal', label: '煤炭', emoji: '⚫'},
  {id: 'minecraft:furnace', label: '熔炉', emoji: '🔥'},
  {id: 'minecraft:crafting_table', label: '工作台', emoji: '🛠️'},
  {id: 'minecraft:chest', label: '箱子', emoji: '📦'},
  {id: 'minecraft:ender_pearl', label: '末影珍珠', emoji: '🔮'},
  {id: 'minecraft:blaze_rod', label: '烈焰棒', emoji: '🔥'},
  {id: 'minecraft:nether_star', label: '下界之星', emoji: '⭐'},
  {id: 'minecraft:totem_of_undying', label: '不死图腾', emoji: '🗿'},
  {id: 'minecraft:experience_bottle', label: '附魔瓶', emoji: '✨'},
  {id: 'minecraft:map', label: '地图', emoji: '🗺️'},
  {id: 'minecraft:compass', label: '指南针', emoji: '🧭'},
];

const CATALOG_BY_ID = new Map(QUEST_ICON_CATALOG.map(e => [e.id, e]));

export function iconCatalogEntry(iconId: string | undefined | null): QuestIconEntry | null {
  if (!iconId) return null;
  return CATALOG_BY_ID.get(iconId) ?? null;
}

/** 节点图标：catalog 命中用 emoji，否则首字母占位。始终带 aria-label。 */
export function iconDisplay(iconId: string | undefined | null): {
  kind: 'catalog' | 'text' | 'empty';
  emoji: string;
  short: string;
  ariaLabel: string;
} {
  if (!iconId) {
    return {kind: 'empty', emoji: '◻', short: '?', ariaLabel: '未设置图标'};
  }
  const entry = iconCatalogEntry(iconId);
  if (entry) {
    return {kind: 'catalog', emoji: entry.emoji, short: entry.emoji, ariaLabel: `${entry.label} (${iconId})`};
  }
  const path = iconId.includes(':') ? iconId.split(':').pop() ?? iconId : iconId;
  const short = path.replace(/_/g, ' ').slice(0, 1) || '?';
  return {kind: 'text', emoji: short, short, ariaLabel: `图标 ${iconId}`};
}

/** 直接可渲染的 URL（http/data），否则 null → 用占位。 */
export function iconUrlOrNull(iconId: string | undefined | null): string | null {
  if (!iconId) return null;
  if (/^https?:\/\//i.test(iconId) || iconId.startsWith('data:')) return iconId;
  return null;
}
