import {useCallback, useRef, useState, type ReactNode} from 'react';
import {Spinner} from './Spinner';

/* 主-从（Master–Detail）布局骨架：大标题 + 左列表 + 右详情。

   这是项目里最高频的形态 —— 任务日志、魔改文档、任务节点检查器、商店命中项
   都是「选一条 → 看它的细节」。以前每处各写一遍：标题栏、空态、分隔条拖拽、
   「选中的那条被删了怎么办」，ModeEdit 甚至手抄了一份 mousemove 拖拽。

   组件**只收共性**：
     · 大标题栏（标题 + 补充计数 + 右端动作）
     · 左栏容器：滚动、空态、加载态、错误态、可拖宽度
     · 右栏容器：详情头（标题 + 动作）+ 详情体，未选中时的空态
     · 选中态的回落：selectedId 指向的项不在列表里时，selected 就是 null，
       调用方拿 null 渲染空态，不用自己写「选中项消失」的兜底（以前 BottomDock 手写过）
     · 可选的键盘上下导航

   组件**不收异构部分**：列表项长什么样、详情里渲染什么，一律由调用方给。
   刻意不做成「传 kind 进来、内部 switch」的超级组件 —— 日志是文本流、
   模组是版本表、任务节点是表单，它们的渲染没有交集，强行配置化只会得到
   一个配置项比 JSX 还长的表单引擎，加一种内容还要回来改组件内部。

   选中态是**受控**的（selectedId + onSelect）：ui 层不许碰 app/url，
   要不要进 URL 由调用方决定（ModeEdit 传 ?doc=，任务日志传本地 state）。 */
export type MasterDetailProps<T> = {
  /** 大标题。这个窗口在说什么 —— 「Log」「自建内容」「任务书」。 */
  title: ReactNode;
  /** 标题右旁的补充信息（计数、状态）。 */
  titleExtra?: ReactNode;
  /** 标题栏右端动作（收起、刷新、新建）。 */
  actions?: ReactNode;

  items: T[];
  itemKey: (item: T) => string;
  /** 列表项渲染。组件只给容器与选中标志，长什么样调用方说了算。 */
  renderItem: (item: T, selected: boolean) => ReactNode;
  /** 列表为空时的文案。 */
  listEmpty?: ReactNode;

  selectedId: string | null;
  onSelect: (id: string | null) => void;

  /** 详情体。拿到的 item 一定存在（选中项不存在时组件渲染空态）。 */
  renderDetail: (item: T) => ReactNode;
  /** 详情头部的标题。与 detailActions 都不传时，详情头整行不渲染。 */
  detailTitle?: (item: T) => ReactNode;
  /** 详情头部的动作（删除、复制、应用）。 */
  detailActions?: (item: T) => ReactNode;
  detailEmpty?: ReactNode;
  detailLoading?: boolean;

  listLoading?: boolean;
  error?: ReactNode;

  /** 左栏初始宽度与可拖范围（px）。 */
  listWidth?: number;
  minListWidth?: number;
  maxListWidth?: number;

  /** 方向键在列表中移动选中项。默认关：全局快捷键也用方向键走格，
      开了会和 useHotkeys 抢，需要开的调用方自己确认不冲突。 */
  keyboardNav?: boolean;

  /** 附加在根元素上的类名。骨架本身不定高不定宽 —— 尺寸由调用方给：
      底边栏传 .dock（固定高度），面板内的传一个 flex:1 的类。 */
  className?: string;
};

export function MasterDetail<T>({
  title, titleExtra, actions,
  items, itemKey, renderItem, listEmpty = '没有可显示的内容。',
  selectedId, onSelect,
  renderDetail, detailTitle, detailActions, detailEmpty = '左侧选一条查看细节。', detailLoading,
  listLoading, error,
  listWidth = 260, minListWidth = 150, maxListWidth = 520,
  keyboardNav = false,
  className,
}: MasterDetailProps<T>) {
  const [lw, setLw] = useState(listWidth);
  const bodyRef = useRef<HTMLDivElement>(null);

  /* 拖拽期间只改本地宽度，不上 URL：这是视图偏好，不是文档状态。 */
  const startDrag = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    const rect = bodyRef.current?.getBoundingClientRect();
    if (!rect) return;
    const move = (ev: MouseEvent) => {
      const next = Math.min(maxListWidth, Math.max(minListWidth, ev.clientX - rect.left));
      setLw(next);
    };
    const up = () => {
      document.removeEventListener('mousemove', move);
      document.removeEventListener('mouseup', up);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    document.addEventListener('mousemove', move);
    document.addEventListener('mouseup', up);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }, [minListWidth, maxListWidth]);

  /* 选中项可能已经不在列表里（任务结束被顶掉、文档被删）。
     这里算成 null，右栏就走空态 —— 调用方不必各自写这段兜底。 */
  const selected = selectedId == null ? null : items.find(i => itemKey(i) === selectedId) ?? null;

  const step = (delta: number) => {
    if (items.length === 0) return;
    const cur = items.findIndex(i => itemKey(i) === selectedId);
    const next = cur < 0
      ? (delta > 0 ? 0 : items.length - 1)
      : Math.min(items.length - 1, Math.max(0, cur + delta));
    onSelect(itemKey(items[next]));
  };

  return (
    <section className={`md${className ? ` ${className}` : ''}`}>
      <header className="md-head">
        <span className="md-title">{title}</span>
        {titleExtra != null && <span className="md-extra">{titleExtra}</span>}
        <span className="grow"/>
        {actions}
      </header>

      <div className="md-body" ref={bodyRef}>
        <div className="md-list" style={{width: lw}}
          tabIndex={keyboardNav ? 0 : undefined}
          onKeyDown={keyboardNav ? e => {
            if (e.key === 'ArrowDown') { e.preventDefault(); step(1); }
            else if (e.key === 'ArrowUp') { e.preventDefault(); step(-1); }
            else if (e.key === 'Home') { e.preventDefault(); if (items.length) onSelect(itemKey(items[0])); }
            else if (e.key === 'End') { e.preventDefault(); if (items.length) onSelect(itemKey(items[items.length - 1])); }
          } : undefined}>
          {error != null && <div className="p-empty">{error}</div>}
          {listLoading && (
            <div className="p-empty"><Spinner size={12} label="加载中"/> 加载中…</div>
          )}
          {!listLoading && items.length === 0 && error == null && <div className="p-empty">{listEmpty}</div>}
          {items.map(item => {
            const id = itemKey(item);
            return (
              /* 点击绑在包裹层：行里的按钮（暂停/重试）自己 stopPropagation 即可。
                 调用方就不必在每个列表项上重复写 onClick 选中。 */
              <div key={id} data-md-id={id} onClick={() => onSelect(id)}>
                {renderItem(item, id === selectedId)}
              </div>
            );
          })}
        </div>

        <div className="md-divider" role="separator" aria-orientation="vertical"
          title="拖动调整左栏宽度" onMouseDown={startDrag}/>

        <div className="md-detail">
          {selected ? (
            <>
              {(detailTitle || detailActions) && (
                <div className="md-detail-head">
                  <span className="md-detail-title">{detailTitle?.(selected)}</span>
                  <span className="grow"/>
                  {detailActions?.(selected)}
                </div>
              )}
              <div className="md-detail-body">
                {detailLoading
                  ? <div className="p-empty"><Spinner size={12} label="加载中"/> 加载中…</div>
                  : renderDetail(selected)}
              </div>
            </>
          ) : (
            <div className="md-detail-body"><div className="ed-placeholder">{detailEmpty}</div></div>
          )}
        </div>
      </div>
    </section>
  );
}
