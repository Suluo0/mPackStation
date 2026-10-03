import {useCallback, useMemo} from 'react';
import {useSearchParams} from 'react-router-dom';

/* URL 是状态的单一事实源（V3 §2）。这里收口全部查询参数的读写，
   组件不许自己 new URLSearchParams 拼同义参数。 */

export const MODES = [
  {mode: 'index', label: '索引', hotkey: ''},
  {mode: 'graph', label: '关系', hotkey: 'Enter'},
  {mode: 'edit', label: '魔改', hotkey: 'E'},
  {mode: 'quest', label: '编排', hotkey: 'Q'},
] as const;

export type Mode = typeof MODES[number]['mode'];

export const TOOLS = [
  {tool: 'sources', label: '来源'},
  {tool: 'build', label: '构建'},
  {tool: 'run', label: '运行'},
] as const;

export type Tool = typeof TOOLS[number]['tool'];

export function isMode(v: string | null): v is Mode {
  return !!v && MODES.some(m => m.mode === v);
}

export function isTool(v: string | null): v is Tool {
  return !!v && TOOLS.some(t => t.tool === v);
}

/** 当前 URL 的结构化读取。 */
export function useUrlState() {
  const [params] = useSearchParams();
  return useMemo(() => {
    const pack = params.get('pack');
    const mode = params.get('mode');
    const tool = params.get('tool');
    return {
      packId: pack || null,
      mode: isMode(mode) ? mode : 'index',
      tool: isTool(tool) ? tool : 'focus',
      dock: params.get('dock') === '1',
      item: params.get('item'),
      focusKind: params.get('f') as string | null,
      focusId: params.get('fid'),
      ns: params.get('ns'),
      /* ?ns= 是目录命名空间（jei/minecraft，物品 id 前缀）；?src= 是内部模组 id
         （mod-xxx，模组内容解析 API 的路径参数）。两者并存：树下钻同时写这两个。 */
      src: params.get('src'),
      type: params.get('type'),
      view: (params.get('view') === 'table' ? 'table' : 'grid') as 'table' | 'grid',
      /* 复杂过滤器：?f=kind:value:on,...（value 可含冒号）+ ?fmode=and|or + ?fs=1 开关 */
      f: params.get('f'),
      fmode: params.get('fmode') === 'or' ? 'or' : 'and',
      fs: params.get('fs'),
      sort: params.get('sort') ?? 'def',
      dir: params.get('dir') === 'desc' ? 'desc' : 'asc',
      doc: params.get('doc'),
      node: params.get('node'),
      q: params.get('q') ?? '',
      settings: params.get('settings') === '1',
    };
  }, [params]);
}

/** 参数补丁写法：一次 setState 改若干键，其余保留（默认 replace 不刷历史栈）。 */
export function useUrlPatch() {
  const [, setParams] = useSearchParams();
  return useCallback((patch: Record<string, string | null>, opts?: {push?: boolean}) => {
    setParams(prev => {
      const p = new URLSearchParams(prev);
      for (const [k, v] of Object.entries(patch)) {
        if (v === null || v === '') p.delete(k); else p.set(k, v);
      }
      return p;
    }, {replace: !opts?.push});
  }, [setParams]);
}

/** 焦点的读写（与 web2 useFocus 同一契约：物品 ?item=，其余 ?f=&fid=）。 */
export function useFocus(): [{kind: string; id: string} | null, (next: {kind: string; id: string} | null, opts?: {mode?: Mode}) => void] {
  const state = useUrlState();
  const patch = useUrlPatch();
  const focus = useMemo(() => {
    if (state.item) return {kind: 'item', id: state.item};
    if (state.focusKind && state.focusId) return {kind: state.focusKind, id: state.focusId};
    return null;
  }, [state.item, state.focusKind, state.focusId]);
  const setFocus = useCallback((next: {kind: string; id: string} | null, opts?: {mode?: Mode}) => {
    const p: Record<string, string | null> = {item: null, f: null, fid: null};
    if (next) {
      if (next.kind === 'item') p.item = next.id;
      else { p.f = next.kind; p.fid = next.id; }
    }
    if (opts?.mode) p.mode = opts.mode;
    patch(p);
  }, [patch]);
  return [focus, setFocus];
}
