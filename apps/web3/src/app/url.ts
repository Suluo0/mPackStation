import {useCallback, useMemo} from 'react';
import {useSearchParams} from 'react-router-dom';

/* URL 是状态的单一事实源（V3 §2）。这里收口全部查询参数的读写，
   组件不许自己 new URLSearchParams 拼同义参数。 */

export const MODES = [
  {mode: 'index', label: '索引', hotkey: ''},
  {mode: 'graph', label: '关系', hotkey: 'Enter'},
  {mode: 'chain', label: '链路', hotkey: 'C'},
  {mode: 'edit', label: '魔改', hotkey: 'E'},
  {mode: 'quest', label: '编排', hotkey: 'Q'},
] as const;

export type Mode = typeof MODES[number]['mode'] | 'chain';

/* tool 名沿用老的（改名字要动 URL 与一堆调用点），只有展示文案按新职责改。
   第 2 区（图标轨）四个按钮 = 项目管理 / 新增模组 / 任务书 / Get Version；
   自建内容 / 构建 / 运行 是顶边栏右侧的动作按钮，不占图标轨（04-align-pack-root.md §7）。 */
export const TOOLS = [
  {tool: 'sources', label: '项目管理'},
  {tool: 'store', label: '新增模组'},
  {tool: 'quest', label: '任务书'},
  {tool: 'releases', label: 'Get Version'},
  {tool: 'content', label: '自建内容'},
  {tool: 'build', label: '构建'},
  {tool: 'run', label: '运行'},
] as const;

/* 图标轨 = 第 2 区：一个按钮一件事。顺序即轨道从上到下的顺序。 */
export const RAIL_TOOLS = [
  'sources', 'store', 'quest', 'releases',
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
      /* 底部 dock 的页签：log / problems。徽标与状态条点击时用
         patch({dock:'1', dtab:...}) 直接落到对应页签，不再弹窗。 */
      dtab: params.get('dtab') === 'problems' ? 'problems' as const : 'log' as const,
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
      /* ?chap= 编排态当前选中的章节。左栏章节面板点一章就是写它，
         这样「点章节」能直接落到那一章，而不是总回到第一章。 */
      chap: params.get('chap'),
      /* ?ck= 自建内容的种类（recipe / structure / ore）—— 后端 validContentKind
         只认这三种，传别的会被 422 拒。左栏「自建内容管理」的子节点写它。 */
      ck: params.get('ck'),
      /* ?qscope=all 让画布一次显示整本书（跨章节连线才看得见）。
         默认只看当前章节 —— FTB 也是一章一屏。 */
      qscope: params.get('qscope') === 'all' ? 'all' : 'chap',
      q: params.get('q') ?? '',
      settings: params.get('settings') === '1',
      /* 关系态（mode=graph）私有状态：
         rd   = 方向，out=作为产物 / in=作为原料（对应 JEI 的 R / U）
         rv   = 视图，canvas=焦点下钻 / list=双向清单
         r    = 当前方向下正在看第几条配方（可分享的位置感）
         rpath= 下钻轨迹（逗号分隔物品 id），面包屑与「返回上层」的数据源
         瞬态状态（槽位选中、标签候选游标）一律不进 URL。 */
      rd: params.get('rd') === 'in' ? 'in' : 'out',
      rv: params.get('rv') === 'list' ? 'list' : 'canvas',
      r: Math.max(0, Number(params.get('r') ?? 0) || 0),
      rpath: params.get('rpath') ?? '',
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

/** 焦点的读写（与 web2 useFocus 同一契约：物品 ?item=，其余 ?f=&fid=）。
    opts.patch 用来把「换焦点的同时要改的其它参数」并进同一次写入 ——
    例如关系态下钻要同时写 rpath。分两次写会多一次导航。 */
export function useFocus(): [
  {kind: string; id: string} | null,
  (next: {kind: string; id: string} | null, opts?: {mode?: Mode; patch?: Record<string, string | null>}) => void,
] {
  const state = useUrlState();
  const patch = useUrlPatch();
  const focus = useMemo(() => {
    if (state.item) return {kind: 'item', id: state.item};
    if (state.focusKind && state.focusId) return {kind: state.focusKind, id: state.focusId};
    return null;
  }, [state.item, state.focusKind, state.focusId]);
  const setFocus = useCallback(
    (next: {kind: string; id: string} | null, opts?: {mode?: Mode; patch?: Record<string, string | null>}) => {
      const p: Record<string, string | null> = {item: null, f: null, fid: null};
      if (next) {
        if (next.kind === 'item') p.item = next.id;
        else { p.f = next.kind; p.fid = next.id; }
      }
      if (opts?.mode) p.mode = opts.mode;
      if (opts?.patch) Object.assign(p, opts.patch);
      patch(p);
    }, [patch]);
  return [focus, setFocus];
}
