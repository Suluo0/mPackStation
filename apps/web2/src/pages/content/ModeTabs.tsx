import {useSearchParams} from 'react-router-dom';
import {CONTENT_MODES, type ContentMode} from '../../app/nav';

/* 内容页四态切换条（§6.4）。切态用 replace:false，这样浏览器后退能在四态之间走；
   参数只写 mode，焦点参数原样留在 URL 里 —— 「切态不丢焦点」的实现约束。 */
export function ModeTabs({value}: {value: ContentMode}) {
  const [, setParams] = useSearchParams();

  const pick = (mode: ContentMode) => {
    if (mode === value) return;
    setParams(p => { const n = new URLSearchParams(p); n.set('mode', mode); return n; }, {replace: false});
  };

  const cur = CONTENT_MODES.find(m => m.mode === value);
  return (
    <div className="mode-tabs">
      {CONTENT_MODES.map(m => (
        <button key={m.mode} type="button" className={`mode-tab${m.mode === value ? ' on' : ''}`} onClick={() => pick(m.mode)}>{m.label}</button>
      ))}
      {cur?.hotkey && <span className="mode-hotkey">快捷键 {cur.hotkey} · Esc 回上一态</span>}
    </div>
  );
}
