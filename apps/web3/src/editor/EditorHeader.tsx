import {useState} from 'react';
import {useCatalog} from '../app/CatalogContext';
import {MODES, useFocus, useUrlPatch, useUrlState} from '../app/url';

/* 编辑区头（四透镜 = 二级能力，选中对象才出现）。
   点击上下文名展开详情：图标 + 多语言名称表（全站唯一落点，3A 验收必查）+
   所属标签 + 来源 evidence + 传送门。非物品焦点只显示 kind+id，不放假内容。 */
const HOTKEYS: Partial<Record<string, string>> = {graph: '↵', edit: 'E', quest: 'Q'};

export function EditorHeader() {
  const {packId, mode} = useUrlState();
  const patch = useUrlPatch();
  const [focus, setFocus] = useFocus();
  const {itemById} = useCatalog();
  const [detail, setDetail] = useState(false);

  if (!packId || !focus) return null;
  const item = focus.kind === 'item' ? itemById.get(focus.id) : undefined;
  const name = item?.displayName ?? (focus.kind === 'item' ? focus.id.split(':').pop() : focus.id);

  return (
    <>
      <div className="ed-header">
        <span className="eh-ctx" onClick={() => setDetail(v => !v)} role="button" title="展开/收起详情">
          {item && item.iconStatus === 'ready' && (
            <img className="eh-icon" alt=""
              src={`/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(focus.id)}`}/>
          )}
          <span className="ctx-glyph">{item ? '' : '◎'}</span>
          <span className="eh-name">{name}</span>
          <span className="eh-kind">{focus.kind}</span>
          <span className="eh-caret">{detail ? '▾' : '▸'}</span>
        </span>
        <span className="grow" style={{flex: 1}}/>
        <button type="button" className="ctx-clear" aria-label="清除焦点" onClick={() => { setFocus(null); setDetail(false); }}>✕</button>
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
      {detail && (
        <div className="ed-header-detail">
          {item ? (
            <>
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
            </>
          ) : (
            <div className="p-row"><span className="mono">{focus.kind} · {focus.id}</span></div>
          )}
        </div>
      )}
    </>
  );
}

