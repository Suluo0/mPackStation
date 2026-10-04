import {useSyncExternalStore, type MouseEvent} from 'react';

/* 鼠标当前悬停的物品（全局单例）。
   为什么需要它：JEI 那套手感是「鼠标指着哪个物品，R/U 就作用于哪个」，
   而不是「先点一下选中、再按 R」——后者多一次点击，且鼠标位置和生效对象
   可以完全不在一处（焦点留在上一个物品上），看起来像按错了。

   悬停是**瞬态**，不该进 URL（进 URL 会把鼠标移动写进历史栈）。
   所以用模块级单例 + useSyncExternalStore：任意组件都能读，任意列表都能写，
   不引入新的 Provider 层级，也不算 React 状态。 */

let current: string | null = null;
const listeners = new Set<() => void>();

export function setHoverTarget(id: string | null) {
  if (current === id) return;
  current = id;
  for (const l of listeners) l();
}

/** 只读：当前悬停的物品 id（没有则 null）。 */
export function useHoverTarget(): string | null {
  return useSyncExternalStore(
    cb => { listeners.add(cb); return () => { listeners.delete(cb); }; },
    () => current,
    () => null,
  );
}

/** 卸载时清理：只有当自己还是当前悬停目标时才清，避免踩掉别人的登记。 */
export function clearHoverTarget(id: string) {
  if (current === id) setHoverTarget(null);
}

/* 列表容器的统一挂载点：鼠标进入子元素时登记、离开容器时清空。
   用事件委托而不是给每个格子挂 onMouseEnter —— 目录里几千行时，
   逐行挂监听的代价和 diff 开销都不划算。 */
export const HOVER_ATTR = 'data-hover-item';

export function hoverProps(): {
  onMouseOver: (e: MouseEvent<HTMLElement>) => void;
  onMouseLeave: () => void;
} {
  return {
    onMouseOver: e => {
      const el = (e.target as HTMLElement | null)?.closest(`[${HOVER_ATTR}]`) as HTMLElement | null;
      setHoverTarget(el?.getAttribute(HOVER_ATTR) ?? null);
    },
    onMouseLeave: () => setHoverTarget(null),
  };
}
