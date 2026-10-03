import {useEffect, useState} from 'react';
import {useUrlPatch, useUrlState, MODES, type Mode} from './url';

/* 全局键盘（v2.1 §5-10 沿用）：⌘K 面板、Enter/E/Q 切态、Esc 回上一态。
   输入框聚焦时一律不响应单键快捷键。方向键走格由网格自己处理（ItemGrid）。 */
export function useHotkeys(): [boolean, (v: boolean) => void] {
  const [open, setOpen] = useState(false);
  const {packId, mode} = useUrlState();
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
      if (e.key === 'Enter' && mode === 'index') {
        e.preventDefault();
        patch({mode: 'graph'});
      }
    };    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, packId, mode, patch]);

  return [open, setOpen];
}
