import {useEffect, useState} from 'react';
import {useFocus, useUrlPatch, useUrlState, MODES, type Mode} from './url';
import {useHoverTarget} from './hoverTarget';

/* 全局键盘（v2.1 §5-10 沿用）：⌘K 面板、Enter/E/Q 切态、Esc 回上一态，
   外加 JEI 那两把钥匙 —— R 看它怎么合成（作为产物）、U 看它能干什么（作为原料）。
   输入框聚焦时一律不响应单键快捷键。方向键走格由网格自己处理（ItemGrid）。

   R/U 的作用对象是**鼠标悬停的物品**，没有悬停才退回当前焦点 ——
   JEI 的手感就是「指着哪个看哪个」，不需要先点一下选中。 */
export function useHotkeys(): [boolean, (v: boolean) => void] {
  const [open, setOpen] = useState(false);
  const {packId, mode, rd} = useUrlState();
  const [focus, setFocus] = useFocus();
  const hovered = useHoverTarget();
  const patch = useUrlPatch();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const typing = !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable || el.tagName === 'SELECT');

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault(); setOpen(v => !v); return;
      }
      if (e.key === 'Escape') {
        if (open) { setOpen(false); return; }
        if (!packId) return;
        const i = MODES.findIndex(m => m.mode === mode);
        if (i > 0) patch({mode: MODES[i - 1].mode});
        return;
      }
      if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
      if (!packId) return;

      const k = e.key.toLowerCase();
      if (k === 'e' || k === 'q') {
        e.preventDefault();
        patch({mode: (k === 'e' ? 'edit' : 'quest') as Mode});
      }
      /* R / U：鼠标指着哪个物品就作用于哪个；没指着才用当前焦点。
         任何透镜下都能按，直接落进关系态的对应方向。 */
      const targetId = hovered ?? (focus?.kind === 'item' ? focus.id : null);
      if ((k === 'r' || k === 'u') && targetId) {
        const wantIn = k === 'u';
        /* 已经就是这个物品的这个方向，别把下钻位置重置掉。 */
        if (mode === 'graph' && focus?.kind === 'item' && focus.id === targetId && (rd === 'in') === wantIn) return;
        e.preventDefault();
        /* 焦点与透镜一次写完：否则关系态读到的还是上一个物品。 */
        setFocus({kind: 'item', id: targetId}, {patch: {mode: 'graph', rd: wantIn ? 'in' : null, r: null, rpath: null}});
      }
      if (e.key === 'Enter' && mode === 'index' && focus?.kind === 'item') {
        e.preventDefault();
        setFocus(focus, {patch: {mode: 'graph'}});
      }
    };    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, packId, mode, rd, focus, hovered, setFocus, patch]);

  return [open, setOpen];
}
