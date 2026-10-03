import {useMemo, useState} from 'react';
import {useCatalog} from '../app/CatalogContext';
import {
  applyFilter, condKind, encodeFilterSpec, parseFilterSpec, TAG_CONCEPTS,
  type FilterCond, type FilterMode,
} from '../app/catalogSearch';
import {useUrlPatch, useUrlState} from '../app/url';
import {Icon} from '../app/Icon';

/* 复杂过滤器（用户定稿：JEI 式前缀，无类型选择器、无语法学习成本）。
   一个标准输入框，前缀分派：# 标签（概念词或标签 ID）、@ 来源（模组命名空间）、
   裸词 = 文本。条件行可启用/禁用/仅启用/删除；连接词 全部满足/任一满足。
   改动实时生效（URL ?f= ?fmode= 单一事实源）。 */
const KIND_LABELS = {tag: '标签', ns: '来源', text: '文本'} as const;

export function FilterBuilder() {
  const {f, fmode, fs} = useUrlState();
  const patch = useUrlPatch();
  const {namespaces} = useCatalog();
  const {conds, mode} = useMemo(() => parseFilterSpec(f, fmode), [f, fmode]);
  const activeCount = conds.filter(c => c.on).length;
  const open = fs === '1';

  const write = (next: FilterCond[]) => patch({f: next.length ? encodeFilterSpec(next) : null});
  const update = (i: number, p: Partial<FilterCond>) =>
    write(conds.map((c, k) => (k === i ? {...c, ...p} : c)));
  const remove = (i: number) => write(conds.filter((_, k) => k !== i));
  const solo = (i: number) => write(conds.map((c, k) => ({...c, on: k === i})));
  const add = (value: string) => {
    if (!value.trim()) return;
    write([...conds, {value: value.trim(), on: true}]);
  };

  return (
    <span className="fs-anchor">
      <button type="button" className={`p-btn${conds.length ? ' on-view' : ''}`}
        title="复杂搜索：多条件筛选（# 标签 / @ 来源 / 裸词文本）"
        onClick={() => patch({fs: open ? null : '1'})}>
        <Icon name="filter" size={12}/> 复杂搜索{conds.length ? ` ${activeCount}/${conds.length}` : ''}
      </button>
      <FilterDropdown conds={conds} mode={mode} names={namespaces}
        onUpdate={update} onRemove={remove} onSolo={solo} onAdd={add}
        onMode={m => patch({fmode: m === 'and' ? null : m})}/>
    </span>
  );
}

function FilterDropdown({conds, mode, names, onUpdate, onRemove, onSolo, onAdd, onMode}: {
  conds: FilterCond[];
  mode: FilterMode;
  names: {ns: string; count: number}[];
  onUpdate: (i: number, p: Partial<FilterCond>) => void;
  onRemove: (i: number) => void;
  onSolo: (i: number) => void;
  onAdd: (value: string) => void;
  onMode: (m: FilterMode) => void;
}) {
  const {fs} = useUrlState();
  const {catalog} = useCatalog();
  /* 每个启用条件的单独命中数：0 命中标红，空转条件一眼可见 */
  const hitCounts = useMemo(
    () => conds.map(c => (c.on ? applyFilter(catalog?.items ?? [], [{...c, on: true}], 'and').length : null)),
    [conds, catalog],
  );
  /* 建议列表：# 概念词 + @ 真实来源（只含有贡献的，不产内容模组不出现）+ 成员最多的标签 */
  const suggestions = useMemo(() => {
    const out: {value: string; label: string}[] = [];
    for (const [k, v] of Object.entries(TAG_CONCEPTS)) out.push({value: `#${k}`, label: `标签概念：${k}（${v.length} 组标签）`});
    for (const n of names.slice(0, 12)) out.push({value: `@${n.ns}`, label: `来源：${n.ns}（${n.count} 物品）`});
    const topTags = [...(catalog?.tags ?? [])].sort((a, b) => b.members.length - a.members.length).slice(0, 20);
    for (const t of topTags) out.push({value: `#${t.id}`, label: `标签：${t.id}（${t.members.length} 物品）`});
    return out;
  }, [names, catalog]);

  if (fs !== '1') return null;

  return (
    <div className="head-dd fs-dd" onClick={e => e.stopPropagation()}>
      <div className="ss-head">
        <span>条件</span>
        <span style={{flex: 1}}/>
        <span className="fs-mode">
          <button type="button" className={`p-btn${mode === 'and' ? ' on-view' : ''}`} title="启用的条件同时满足"
            onClick={() => onMode('and')}>全部满足</button>
          <button type="button" className={`p-btn${mode === 'or' ? ' on-view' : ''}`} title="启用的条件命中任一即可"
            onClick={() => onMode('or')}>任一满足</button>
        </span>
      </div>
      <div className="ss-body">
        {conds.length === 0 && (
          <div className="p-empty">还没有条件。前缀分派：<b>#</b> 标签（#tools）、<b>@</b> 来源（@jei）、裸词 = 文本（肉）。</div>
        )}
        {conds.map((c, i) => {
          const kind = condKind(c.value);
          return (
            <div key={`${i}-${c.value}`} className={`fs-row${c.on ? '' : ' off'}`}>
              <span className={`fs-kind kind-${kind}`}>{KIND_LABELS[kind]}</span>
              <SuggestInput value={c.value} suggestions={suggestions}
                onChange={v => onUpdate(i, {value: v})}/>
              {hitCounts[i] !== null && (
                <span className={`fs-hit${hitCounts[i] === 0 ? ' zero' : ''}`}
                  title="这个条件单独命中的物品数">
                  {hitCounts[i]}
                </span>
              )}
              <button type="button" className={`fs-toggle${c.on ? ' on' : ''}`}
                title={c.on ? '启用中，点击禁用' : '已禁用，点击启用'}
                onClick={() => onUpdate(i, {on: !c.on})}>{c.on ? '启用' : '禁用'}</button>
              <button type="button" className="p-btn" title="仅启用这一个条件，其余全部禁用"
                disabled={c.on && conds.filter(x => x.on).length === 1}
                onClick={() => onSolo(i)}>仅启</button>
              <button type="button" className="ctx-clear" style={{color: 'var(--mc-muted)'}}
                aria-label="删除条件" onClick={() => onRemove(i)}>✕</button>
            </div>
          );
        })}
        <AddRow onAdd={onAdd} suggestions={suggestions}/>
      </div>
    </div>
  );
}

function AddRow({onAdd, suggestions}: {
  onAdd: (value: string) => void;
  suggestions: {value: string; label: string}[];
}) {
  const [value, setValue] = useState('');
  const submit = () => { onAdd(value); setValue(''); };
  return (
    <div className="fs-row fs-add">
      <SuggestInput value={value} onChange={setValue}
        placeholder="# 标签（#tools）　@ 来源（@jei）　裸词 = 文本（肉）"
        suggestions={suggestions}
        onEnter={submit}/>
      <button type="button" className="p-btn primary" disabled={!value.trim()} onClick={submit}>添加</button>
    </div>
  );
}

/* 自绘建议输入框（第八轮反馈：原生 datalist 的三角箭头丑且不居中，弃用）。
   聚焦/输入时向下弹建议，15 行内滚动，行 = 值 + 说明；Esc 关闭。 */
function SuggestInput({value, onChange, placeholder, suggestions, onEnter}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  suggestions: {value: string; label: string}[];
  onEnter?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const hits = useMemo(() => {
    const q = value.trim().toLowerCase();
    const pool = q ? suggestions.filter(sg => sg.value.toLowerCase().includes(q) || sg.label.toLowerCase().includes(q)) : suggestions;
    return pool.slice(0, 15);
  }, [value, suggestions]);
  return (
    <span className="si-wrap">
      <input className="p-input fs-val" value={value} placeholder={placeholder}
        onFocus={() => setOpen(true)}
        onChange={e => { onChange(e.target.value); setOpen(true); }}
        onKeyDown={e => {
          if (e.key === 'Escape') setOpen(false);
          if (e.key === 'Enter') { onEnter?.(); setOpen(false); }
        }}
        onBlur={() => { window.setTimeout(() => setOpen(false), 150); }}/>
      {open && hits.length > 0 && (
        <div className="head-dd si-dd">
          {hits.map(sg => (
            <div key={sg.value} className="p-row click"
              onMouseDown={e => { e.preventDefault(); onChange(sg.value); setOpen(false); }}>
              <span className="grow" style={{fontFamily: 'var(--mc-font-mono)', fontSize: 11.5}}>{sg.value}</span>
              <span className="sub">{sg.label}</span>
            </div>
          ))}
        </div>
      )}
    </span>
  );
}
