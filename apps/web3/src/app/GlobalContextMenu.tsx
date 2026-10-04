import {useEffect, useState} from 'react';
import {useUrlPatch, useUrlState} from './url';
import {ContextMenu, type MenuItem} from '../ui/ContextMenu';

/* 全局右键：把浏览器的原生菜单整个关掉。
   原生菜单里那几项（返回 / 重新加载 / 另存为 / 检查…）对一个单页工作台来说
   全是噪声 —— 没有一项对应当前光标下的对象。所以默认一律拦掉，
   换成这个应用自己的菜单。

   两条例外，都是必需的：
   1. 输入框 / 可编辑区域放行 —— 粘贴、拼写检查、输入法候选都依赖原生菜单，
      拦掉是纯粹的破坏（用户想粘一个模组名进来却发现没有「粘贴」）。
   2. 某个界面已经自己处理了（e.defaultPrevented 为真）—— 说明它有更贴切的菜单
      （模组行、章节行、画布节点、索引行…），让给它，别抢。

   React 的事件挂在根容器上、这里是 document 上的监听，冒泡顺序保证本函数最后跑，
   所以能可靠地看到 React 那边有没有 preventDefault。 */
export function GlobalContextMenu({onOpenPalette}: {onOpenPalette: () => void}) {
  const {packId, mode, view} = useUrlState();
  const patch = useUrlPatch();
  const [at, setAt] = useState<{x: number; y: number} | null>(null);

  useEffect(() => {
    const onCtx = (e: MouseEvent) => {
      const el = e.target as HTMLElement | null;
      if (!el) return;
      /* 可编辑区域：交给浏览器 */
      if (el.closest('input, textarea, [contenteditable=""], [contenteditable="true"]')) return;
      /* 菜单自己身上：拦住原生菜单，但不要再叠一层自己的 */
      if (el.closest('.ctx-menu')) { e.preventDefault(); return; }
      /* 已经有更贴切的菜单了 */
      if (e.defaultPrevented) return;
      e.preventDefault();
      setAt({x: e.clientX, y: e.clientY});
    };
    document.addEventListener('contextmenu', onCtx);
    return () => document.removeEventListener('contextmenu', onCtx);
  }, []);

  if (!at) return null;

  /* 兜底菜单只放「跟光标下对象无关」的动作 —— 任何位置都成立，不猜用户点到了什么。
     带对象的动作由各自的界面在自己的 onContextMenu 里给。 */
  const items: MenuItem[] = [];
  if (mode === 'index') {
    items.push({
      label: view === 'grid' ? '切到表格视图' : '切到网格视图',
      action: () => patch({view: view === 'grid' ? 'table' : 'grid'}),
    });
    items.push({separator: true});
  }
  items.push({label: '命令面板（⌘K）', action: onOpenPalette});
  items.push({separator: true});
  items.push({
    label: '复制整合包 ID', disabled: !packId,
    action: () => { void navigator.clipboard?.writeText(packId ?? '').catch(() => undefined); },
  });
  items.push({
    label: '复制当前地址',
    action: () => { void navigator.clipboard?.writeText(location.href).catch(() => undefined); },
  });
  items.push({separator: true});
  items.push({label: '编辑器设置…', action: () => patch({settings: '1'})});

  return <ContextMenu x={at.x} y={at.y} items={items} onClose={() => setAt(null)}/>;
}
