import {useCallback, useMemo} from 'react';
import {useSearchParams} from 'react-router-dom';

export type FocusKind = 'item' | 'recipe' | 'mod' | 'node' | 'doc';
export type Focus = {kind: FocusKind; id: string} | null;

/* 焦点在 URL 里只有一种表达：物品用 ?item=（沿用既有参数名），其余用 ?f= + ?fid=。
   这个 hook 是它的唯一读写入口 —— 组件不许自己碰这三个参数，否则会出现两处写、互相覆盖。 */
export function useFocus(): [Focus, (next: Focus, opts?: {mode?: string}) => void] {
  const [params, setParams] = useSearchParams();

  const focus = useMemo<Focus>(() => {
    const item = params.get('item');
    if (item) return {kind: 'item', id: item};
    const kind = params.get('f') as FocusKind | null;
    const id = params.get('fid');
    return kind && id ? {kind, id} : null;
  }, [params]);

  const setFocus = useCallback((next: Focus, opts?: {mode?: string}) => {
    setParams(prev => {
      const p = new URLSearchParams(prev);
      p.delete('item'); p.delete('f'); p.delete('fid');
      if (next) {
        if (next.kind === 'item') p.set('item', next.id);
        else { p.set('f', next.kind); p.set('fid', next.id); }
      }
      if (opts?.mode) p.set('mode', opts.mode);
      return p;
    }, {replace: true});
  }, [setParams]);

  return [focus, setFocus];
}
