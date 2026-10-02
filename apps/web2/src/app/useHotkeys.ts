import {useEffect, useState} from 'react';
import {useLocation, useNavigate, useSearchParams} from 'react-router-dom';
import {CONTENT_MODES, type ContentMode} from './nav';

/* 全局键盘（设计文档 §5-10）。返回 [面板开关, setter] 给 AppShell。
   输入框聚焦时一律不响应单键快捷键，否则在 JSON 编辑器里敲 e/q 会切态。 */
export function useHotkeys(): [boolean, (v: boolean) => void] {
  const [open, setOpen] = useState(false);
  const nav = useNavigate();
  const loc = useLocation();
  const [params, setParams] = useSearchParams();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const typing = !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault(); setOpen(v => !v); return;
      }
      if (e.key === 'Escape') {
        if (open) { setOpen(false); return; }
        // Esc 回上一态：按 CONTENT_MODES 顺序退一格，已在 index 则不动
        if (!loc.pathname.endsWith('/content')) return;
        const cur = (params.get('mode') ?? 'index') as ContentMode;
        const i = CONTENT_MODES.findIndex(m => m.mode === cur);
        if (i > 0) setParams(p => { const n = new URLSearchParams(p); n.set('mode', CONTENT_MODES[i - 1].mode); return n; }, {replace: false});
        return;
      }
      if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
      if (!loc.pathname.endsWith('/content')) return;

      const k = e.key.toLowerCase();
      if (k === 'e' || k === 'q') {
        e.preventDefault();
        setParams(p => { const n = new URLSearchParams(p); n.set('mode', k === 'e' ? 'edit' : 'quest'); return n; }, {replace: false});
      }
      if (e.key === 'Enter') {
        const cur = params.get('mode');
        if (cur === 'index') {
          e.preventDefault();
          setParams(p => { const n = new URLSearchParams(p); n.set('mode', 'graph'); return n; }, {replace: false});
        }
      }
      if (e.key.startsWith('Arrow')) {
        // 方向键走物品网格：交给索引态叶子处理，这里只负责不抢事件
        void nav; // 保留 nav 引用以便后续跳转命令使用
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, loc.pathname, params, setParams, nav]);

  return [open, setOpen];
}
