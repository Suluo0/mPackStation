import {Icon} from './Icon';
import {Spinner} from './Spinner';

/* 按钮唯一实现。

   皮肤沿用 .p-btn / .tp-icon-btn：这两套类名散在 90 处调用点里，一次换掉风险大，
   而它们本来就是 ui 层的皮肤 —— 组件与裸 class 并存期间视觉完全一致，迁移可以渐进。
   组件真正收口的是**行为**，以前每处各写一遍：
     · loading：自动禁用 + 转圈（以前写成 disabled + 文字「装中…」，各写各的）
     · icon：图标与文字的间距由组件保证，不用每个调用点塞 style
     · active：默认变体走 .on-view，图标按钮走 .on（两个类名是历史遗留）
     · aria-label：纯图标按钮必须给，否则读屏软件只念出一个「按钮」 */
export type ButtonVariant = 'default' | 'primary' | 'danger' | 'ghost';

export function Button({
  variant = 'default',
  icon,
  active = false,
  loading = false,
  disabled = false,
  onClick,
  children,
  title,
  ariaLabel,
  className = '',
  style,
}: {
  variant?: ButtonVariant;
  icon?: string;
  active?: boolean;
  loading?: boolean;
  disabled?: boolean;
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
  children?: React.ReactNode;
  title?: string;
  ariaLabel?: string;
  className?: string;
  style?: React.CSSProperties;
}) {
  const base = variant === 'ghost' ? 'tp-icon-btn' : 'p-btn';
  const mods = [
    variant === 'primary' ? 'primary' : '',
    variant === 'danger' ? 'ui-danger' : '',
    active ? (variant === 'ghost' ? 'on' : 'on-view') : '',
  ].filter(Boolean).join(' ');

  return (
    <button type="button"
      className={`${base}${mods ? ` ${mods}` : ''}${icon || loading ? ' ui-btn-icon' : ''}${className ? ` ${className}` : ''}`}
      title={title} aria-label={ariaLabel} style={style}
      disabled={disabled || loading}
      onClick={onClick}>
      {loading ? <Spinner size={12} label="处理中"/> : icon ? <Icon name={icon} size={12}/> : null}
      {children}
    </button>
  );
}
