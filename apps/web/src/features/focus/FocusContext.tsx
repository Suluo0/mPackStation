import {createContext, useCallback, useContext, useMemo, useState, type ReactNode} from 'react';

/* 工作台共享焦点：全工作台的「当前正在看的那个东西」。
   任何页面点一个物品/配方/模组，就 setFocus 把它设为焦点；常驻右侧 Inspector 读焦点渲染详情与跳转入口。
   这是把五个孤立模块串成一条链路的关键 —— 焦点即上下文，Inspector 即传送门。 */

export type FocusType = 'item' | 'recipe' | 'mod' | 'quest' | 'tag';

export type Focus = {
  type: FocusType;
  id: string;
  /** 可选的展示名，缺省时由消费方从目录取名。 */
  label?: string;
  /** 来源页面，用于 Inspector 显示「从 X 聚焦」。 */
  from?: string;
};

export type FocusValue = {
  focus: Focus | null;
  setFocus: (f: Focus | null) => void;
  focusItem: (id: string, from?: string) => void;
  focusRecipe: (id: string, from?: string) => void;
  focusMod: (id: string, label?: string, from?: string) => void;
  clear: () => void;
  /** Inspector 是否展开（用户可折叠）。 */
  open: boolean;
  setOpen: (v: boolean) => void;
  toggle: () => void;
};

const FocusContext = createContext<FocusValue | null>(null);

export function FocusProvider({children}: {children: ReactNode}) {
  const [focus, setFocusState] = useState<Focus | null>(null);
  const [open, setOpen] = useState(true);

  const setFocus = useCallback((f: Focus | null) => {
    setFocusState(f);
    if (f) setOpen(true);
  }, []);
  const focusItem = useCallback((id: string, from?: string) => setFocus({type: 'item', id, from}), [setFocus]);
  const focusRecipe = useCallback((id: string, from?: string) => setFocus({type: 'recipe', id, from}), [setFocus]);
  const focusMod = useCallback((id: string, label?: string, from?: string) => setFocus({type: 'mod', id, label, from}), [setFocus]);
  const clear = useCallback(() => setFocusState(null), []);
  const toggle = useCallback(() => setOpen(v => !v), []);

  const value = useMemo<FocusValue>(() => ({
    focus, setFocus, focusItem, focusRecipe, focusMod, clear, open, setOpen, toggle,
  }), [focus, setFocus, focusItem, focusRecipe, focusMod, clear, open, toggle]);

  return <FocusContext.Provider value={value}>{children}</FocusContext.Provider>;
}

export function useFocus(): FocusValue {
  const ctx = useContext(FocusContext);
  if (!ctx) throw new Error('useFocus 必须在 <FocusProvider> 内使用');
  return ctx;
}
