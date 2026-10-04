import {createContext, useContext} from 'react';

/* 包名那一行的「动作插槽」。

   侧边栏的根行由 NavTree 渲染，但那一行上的按钮（分类管理 / 解析依赖 / 搜物品）
   的动作状态住在内容面板里 —— catOpen、mode、resolveDeps 全是面板自己的事。

   所以这里只传一个 DOM 插槽：NavTree 在根行留一个空 div，面板用 portal 把按钮
   送上去。**不把状态往上抬**：树一旦知道「搜索框现在是开着还是关着」，
   它就不再是目录树了。

   拿不到插槽（面板不在树里渲染）时 useNavRootSlot() 返回 null，
   调用方负责降级（不渲染按钮，而不是渲染到错误的位置）。 */
export const NavRootSlot = createContext<HTMLElement | null>(null);

export function useNavRootSlot(): HTMLElement | null {
  return useContext(NavRootSlot);
}
