import {Icon} from './Icon';
import {Spinner} from './Spinner';

/* 搜索输入：全站搜索框的唯一入口。
   提交按钮必须用放大镜图标，不能用「搜」字 —— 中文挤得下，英文 search 放不下，
   同一个组件不能在两种语言下换长度。 */
export function SearchInput({value, onChange, onSubmit, placeholder, busy, autoFocus, style}: {
  value: string;
  onChange: (v: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  busy?: boolean;
  autoFocus?: boolean;
  style?: React.CSSProperties;
}) {
  const submit = () => { if (!busy) onSubmit?.(); };
  return (
    <div className="ui-search" style={style}>
      {/* 忙起来要在输入框内右端转个 spinner：搜索是网络请求，几百毫秒没任何
          反馈时，用户会以为输入框坏了或者没触发。按钮在 input 外面，
          所以定位容器是这层 .ui-search-field。 */}
      <div className="ui-search-field">
        <input className="p-input" value={value} placeholder={placeholder} autoFocus={autoFocus}
          onChange={e => onChange(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') submit(); }}/>
        {busy && <span className="ui-search-spin"><Spinner size={13} label="搜索中"/></span>}
      </div>
      <button type="button" className="ui-search-btn" aria-label="搜索" title="搜索"
        disabled={busy} onClick={submit}>
        <Icon name="search" size={14}/>
      </button>
    </div>
  );
}
