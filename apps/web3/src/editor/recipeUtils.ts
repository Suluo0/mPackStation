import type {IngredientSlot} from './recipeView';

/* 配方卡相关的纯工具。刻意与 RecipeCard.tsx 分开：那个文件一旦同时导出组件和
   普通函数，React Fast Refresh 就失去「同构导出」前提，只能放弃热替换并把整棵树
   invalidate 重建 —— 表现是改样式把全站状态清空、Context 分叉报假错误。 */

/** 物品图标的唯一 URL 拼法（README/契约：图标端点只吃目录里已渲染出来的那些）。 */
export function iconUrl(packId: string, itemId: string): string {
  return `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;
}

/** 槽位当前应显示的候选（标签槽轮换的就是它）。 */
export function shownCandidate(slot: IngredientSlot, cycle: Record<string, number>): string | null {
  if (!slot.candidates.length) return null;
  if (slot.candidates.length === 1) return slot.candidates[0];
  const at = cycle[slot.key] ?? 0;
  return slot.candidates[((at % slot.candidates.length) + slot.candidates.length) % slot.candidates.length];
}

export type RecipeCursor = {
  /** 当前选中的槽位键（[ ] 轮换的目标） */
  sel: string | null;
  /** 槽位键 → 候选游标（瞬态，不进 URL） */
  cycle: Record<string, number>;
};
