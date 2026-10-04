import {useCallback, useEffect, useLayoutEffect, useRef, useState, type MouseEvent as ReactMouseEvent, type ReactNode} from 'react';
import {createPortal} from 'react-dom';
import {Icon} from './Icon';

/* 右键菜单：全站唯一实现。任何界面要菜单，都用 useContextMenu() 拿到 open + menu，
   自己只负责按光标下的对象给出条目文本与动作 —— 不再各写一套 x/y state。

   三条硬约束，都是踩过的坑，改这个文件时必须保住：

   1. 必须走 portal（document.body）。曾经直接渲染在调用方 DOM 子树里，
      任务书画布宿主（.qc-host）对左键无条件 setPointerCapture，
      菜单项的 click 被重定向到宿主，表现为「要双击才触发、菜单不消失」。
      portal 出去后菜单不再经过任何捕获型宿主，这一类问题整体消失。

   2. Esc 必须由菜单独占。全局快捷键里 Esc = 「回上一态」，两边的 keydown
      监听一个在 document、一个在 window，本来就都会收到 —— 结果是关掉菜单的
      同时把当前视图也切走了。在捕获阶段 stopPropagation 截住。

   3. 根节点 onPointerDown stopPropagation：防宿主捕获指针 / 防触发画布拖拽；
      onContextMenu preventDefault：在菜单上再右键时别让宿主又开一个菜单。

   外点检测用 ref.contains —— 子菜单挂在同一个根 div 里（不用二级 portal），
   点子菜单里的项仍算「内部」，不会先把菜单关掉。 */

/** 分隔线。判别联合而不是「label 可选」—— 分隔项本来就没有文本，
    让类型表达出来，调用方就不用再写 `{separator: true, label: '', action: () => {}}`。 */
export type MenuSeparator = {separator: true};

export type MenuEntry = {
  label: string;
  action?: () => void;
  /** 图标语义名，走 Icon。不传则只有文字。 */
  icon?: string;
  danger?: boolean;
  disabled?: boolean;
  /** 子菜单。有 submenu 时不执行 action，悬停展开下一级；结构相同，可再嵌。 */
  submenu?: MenuItem[];
};

export type MenuItem = MenuEntry | MenuSeparator;

type MenuState = {x: number; y: number; items: MenuItem[]};

export function ContextMenu({x, y, items, onClose}: MenuState & {onClose: () => void}) {
  const ref = useRef<HTMLDivElement>(null);
  /* onClose 多为调用方内联箭头函数，引用每次渲染都变；存进 ref 让监听只绑一次，
     否则每次渲染都解绑/重绑，和点击序列抢时序。 */
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const [openIdx, setOpenIdx] = useState<number | null>(null);
  /* 定位要量过才知道会不会出视口。量完才显示，否则会先在错误位置闪一帧。 */
  const [at, setAt] = useState<{left: number; top: number} | null>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const pad = 8;
    let left = x;
    let top = y;
    if (left + r.width > window.innerWidth - pad) left = Math.max(pad, x - r.width);
    if (top + r.height > window.innerHeight - pad) top = Math.max(pad, y - r.height);
    setAt({left, top});
  }, [x, y, items]);

  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (ref.current?.contains(e.target as Node)) return;
      onCloseRef.current();
    };
    const esc = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      e.stopPropagation();
      e.preventDefault();
      onCloseRef.current();
    };
    /* 窗口尺寸变化后原坐标可能已经出视口，直接关掉比留个飘在外面的菜单好。 */
    const onResize = () => onCloseRef.current();
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', esc, {capture: true});
    window.addEventListener('resize', onResize);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', esc, {capture: true});
      window.removeEventListener('resize', onResize);
    };
  }, []);

  const flip = at !== null && at.left < x;

  return createPortal(
    <div ref={ref} className="ctx-menu" role="menu"
      style={{left: at?.left ?? x, top: at?.top ?? y, visibility: at ? 'visible' : 'hidden'}}
      onClick={e => e.stopPropagation()}
      onPointerDown={e => e.stopPropagation()}
      onContextMenu={e => e.preventDefault()}>
      {items.map((item, i) => {
        if ('separator' in item) return <div key={i} className="ctx-menu-sep"/>;
        if (item.submenu?.length) {
          const open = openIdx === i;
          return (
            <div key={i} className="ctx-sub-wrap"
              onMouseEnter={() => setOpenIdx(i)}
              onMouseLeave={() => setOpenIdx(cur => (cur === i ? null : cur))}>
              <button type="button"
                className={`ctx-menu-item ctx-has-sub${open ? ' open' : ''}`}
                disabled={item.disabled}
                aria-haspopup="menu" aria-expanded={open}>
                {item.icon && <Icon name={item.icon} size={13}/>}
                <span className="grow">{item.label}</span>
                <Icon name="caretRight" size={12}/>
              </button>
              {open && (
                <div className={`ctx-menu ctx-menu-sub${flip ? ' flip' : ''}`} role="menu">
                  {item.submenu.map((sub, j) =>
                    'separator' in sub
                      ? <div key={j} className="ctx-menu-sep"/>
                      : (
                        <button key={j} type="button"
                          className={`ctx-menu-item${sub.danger ? ' danger' : ''}`}
                          disabled={sub.disabled}
                          onClick={() => { sub.action?.(); onCloseRef.current(); }}>
                          {sub.icon && <Icon name={sub.icon} size={13}/>}
                          {sub.label}
                        </button>
                      ))}
                </div>
              )}
            </div>
          );
        }
        return (
          <button key={i} type="button"
            className={`ctx-menu-item${item.danger ? ' danger' : ''}`}
            disabled={item.disabled}
            onMouseEnter={() => setOpenIdx(null)}
            onClick={() => { item.action?.(); onCloseRef.current(); }}>
            {item.icon && <Icon name={item.icon} size={13}/>}
            {item.label}
          </button>
        );
      })}
    </div>,
    document.body,
  );
}

type OpenFn = (
  e: ReactMouseEvent,
  items: MenuItem[] | ((e: ReactMouseEvent) => MenuItem[]),
) => void;

/** 调用方唯一入口。用法：`onContextMenu={e => open(e, buildItems())}` + `{menu}`。
    条目可以传函数（需要用到事件或那一刻的最新 state 时用），也可以直接传数组。 */
export function useContextMenu(): {open: OpenFn; close: () => void; menu: ReactNode} {
  const [state, setState] = useState<MenuState | null>(null);
  const close = useCallback(() => setState(null), []);
  const open: OpenFn = useCallback((e, items) => {
    e.preventDefault();
    e.stopPropagation();
    setState({x: e.clientX, y: e.clientY, items: typeof items === 'function' ? items(e) : items});
  }, []);
  const menu = state ? <ContextMenu {...state} onClose={close}/> : null;
  return {open, close, menu};
}
