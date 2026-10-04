/* 右键菜单已迁到 ui/ContextMenu.tsx（基础组件层），并补了 useContextMenu 钩子 ——
   调用方不再自己管 x/y state。这里只留转发，老的 import 路径不会断；
   新代码请直接 import {useContextMenu} from '../ui/ContextMenu'。 */
export {ContextMenu, useContextMenu, type MenuItem} from '../ui/ContextMenu';
