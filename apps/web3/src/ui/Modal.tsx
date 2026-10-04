import {useEffect, useRef, type ReactNode} from 'react';
import {createPortal} from 'react-dom';

/* 弹层唯一实现（右键菜单走 ui/ContextMenu —— 它跟着光标定位，规则不同）。

   三条硬约束，都是踩过的坑，改这个文件时必须保住：

   1. 必须走 portal（document.body）。曾经把弹窗渲染在调用方的 DOM 子树里，
      任务书画布宿主（.qc-host）对左键无条件 setPointerCapture，
      弹窗里的 click 被重定向到宿主，表现为「要双击才触发」。portal 出去后
      不再经过任何捕获型宿主，这一类问题整体消失。

   2. Esc 必须由弹层独占。全局快捷键里 Esc = 「回上一态」，两边一个挂 document、
      一个挂 window，本来就都会收到 —— 结果是关掉弹窗的同时把当前视图也切走了。
      在捕获阶段 stopPropagation 截住。

   3. 根节点 onPointerDown stopPropagation：防宿主捕获指针 / 防触发画布拖拽。
      背景点击 = 关闭，内容区自己 stopPropagation。 */
export function Modal({onClose, children, width, className}: {
  onClose: () => void;
  children: ReactNode;
  /** 内容宽度。不传走 .mini-modal 的默认 320px。 */
  width?: number;
  /** 内容容器的类名，默认 .mini-modal。传了就整体替换 —— 追加会和默认类
      抢同一批属性（宽度 / 内距），而 ui.css 与业务 css 的加载顺序不保证。 */
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const esc = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      e.stopPropagation();
      e.preventDefault();
      onCloseRef.current();
    };
    document.addEventListener('keydown', esc, {capture: true});
    return () => document.removeEventListener('keydown', esc, {capture: true});
  }, []);

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div ref={ref}
        className={className ?? 'mini-modal'}
        style={width ? {width} : undefined}
        onClick={e => e.stopPropagation()}
        onPointerDown={e => e.stopPropagation()}>
        {children}
      </div>
    </div>,
    document.body,
  );
}
