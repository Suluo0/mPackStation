import {useState} from 'react';
import {useCatalog} from '../app/CatalogContext';
import {Icon} from '../ui/Icon';
import {MODES, useFocus, useUrlPatch, useUrlState} from '../app/url';

/* 编辑区头（五透镜始终可见，不依赖焦点——用户可随时切态）。
   有焦点时左侧显示上下文（图标/名称/kind/✕清除），无焦点时显示提示文字。
   点击上下文展开详情（多语言名称/标签/传送门）。 */
const HOTKEYS: Partial<Record<string, string>> = {graph: '↵', edit: 'E', quest: 'Q', chain: 'C'};

export function EditorHeader() {
  const {packId, mode} = useUrlState();
  const patch = useUrlPatch();
  const [focus, setFocus] = useFocus();
  const {itemById} = useCatalog();
  const [detail, setDetail] = useState(false);

  if (!packId) return null;
  const item = focus?.kind === 'item' ? itemById.get(focus.id) : undefined;
  const name = item?.displayName ?? (focus?.kind === 'item' ? focus.id.split(':').pop() : '');

  return (
    <>
      <div className="ed-header">
        {/* 上下文区域：有焦点显示对象，无焦点显示引导 */}
        {focus ? (
          <span className="eh-ctx" onClick={() => setDetail(v => !v)} role="button" title="展开/收起详情">
            {item && item.iconStatus === 'ready' && (
              <img className="eh-icon" alt=""
                src={`/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(focus.id)}`}/>
            )}
            <span className="ctx-glyph">{item ? '' : '◎'}</span>
            <span className="eh-name">{name}</span>
            <span className="eh-kind">{focus.kind}</span>
            <span className="eh-caret"><Icon name={detail ? 'caretDown' : 'caretRight'} size={13}/></span>
            <button type="button" className="ctx-clear" aria-label="清除焦点"
              onClick={e => { e.stopPropagation(); setFocus(null); setDetail(false); }}>✕</button>
          </span>
        ) : (
          <span className="eh-ctx" style={{opacity: 0.55}}>
            <span className="sub">未选中物品 —— 点索引或 ⌘K 选一个</span>
          </span>
        )}
        <span style={{flex: 1}}/>
        <nav className="eh-lenses">
          {MODES.map(m => (
            <button key={m.mode} type="button"
              className={`eh-lens${m.mode === mode ? ' on' : ''}`}
              onClick={() => patch({mode: m.mode})}>
              {m.label}
              {HOTKEYS[m.mode] && <sup className="eh-key">{HOTKEYS[m.mode]}</sup>}
            </button>
          ))}
        </nav>
      </div>
      {focus && detail && item && (
        <div className="ed-header-detail">
          <div className="ehd-block">
            <div className="p-title">多语言名称</div>
            {item.names.map(n => (
              <div key={`${n.locale}-${n.key}`} className="p-row">
                <span className="sub" style={{width: 52, flex: 'none'}}>{n.locale}</span>
                <span className="grow">{n.name}</span>
                <span className="sub">{n.source}</span>
              </div>
            ))}
          </div>
          <div className="ehd-block">
            <div className="p-title">所属标签 <span className="count">{item.tags.length}</span></div>
            <div style={{display: 'flex', gap: 4, flexWrap: 'wrap', padding: '0 6px'}}>
              {item.tags.map(t => <span key={t} className="tag-chip">{t}</span>)}
              {item.tags.length === 0 && <span className="p-empty" style={{padding: 0}}>没有标签。</span>}
            </div>
          </div>
          <div className="ehd-block">
            <div className="p-title">来源依据</div>
            <div className="p-row"><span className="mono" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 11.5, color: 'var(--mc-muted)'}}>{item.evidence}</span></div>
          </div>
          <div className="ehd-block">
            <div className="p-title">传送门</div>
            <div style={{display: 'flex', gap: 4, flexWrap: 'wrap', padding: '0 6px 4px'}}>
              <button type="button" className="p-btn" onClick={() => patch({mode: 'index'})}>在索引打开</button>
              <button type="button" className="p-btn" onClick={() => patch({mode: 'graph'})}>看关系</button>
              <button type="button" className="p-btn" onClick={() => patch({mode: 'edit'})}>去魔改</button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
