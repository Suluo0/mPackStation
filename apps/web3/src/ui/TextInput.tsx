/* 单行文本输入：全站输入框的唯一入口（搜索框走 SearchInput，它多一个提交键）。

   皮肤沿用 .p-input，理由同 Button —— 90 处调用点还在用裸 class，
   组件与裸 class 并存期间视觉必须一致。组件收口的是键盘行为：
   Enter / Escape 以前每个调用点各写一遍 onKeyDown，有的漏了 Escape。 */
export function TextInput({
  value, onChange, placeholder, autoFocus, disabled, onEnter, onEscape,
  ariaLabel, className = '', style,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
  disabled?: boolean;
  onEnter?: () => void;
  onEscape?: () => void;
  ariaLabel?: string;
  className?: string;
  style?: React.CSSProperties;
}) {
  return (
    <input className={`p-input${className ? ` ${className}` : ''}`}
      value={value} placeholder={placeholder} autoFocus={autoFocus} disabled={disabled}
      aria-label={ariaLabel} style={style}
      onChange={e => onChange(e.target.value)}
      onKeyDown={e => {
        if (e.key === 'Enter') onEnter?.();
        if (e.key === 'Escape') onEscape?.();
      }}/>
  );
}
