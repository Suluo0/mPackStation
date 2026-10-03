import {useMemo, useState} from 'react';
import {Modal, Input} from 'antd';
import {useUrlPatch, useUrlState, useFocus, MODES, TOOLS} from './url';
import {StubItemSearch} from './StubItemSearch';

type Props = {open: boolean; onOpenChange: (v: boolean) => void};

/* ⌘K 命令面板：模式/工具/设置命令 + 物品命中（保留当前 mode）。
   全局键盘在 useHotkeys，本组件只受控。 */
export function CommandPalette({open, onOpenChange}: Props) {
  const [query, setQuery] = useState('');
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const [focus, setFocus] = useFocus();

  const commands = useMemo(() => {
    const out: {label: string; patch: Record<string, string | null>}[] = [];
    if (packId) for (const m of MODES) out.push({label: `编辑器 · ${m.label}`, patch: {mode: m.mode}});
    for (const t of TOOLS) out.push({label: `面板 · ${t.label}`, patch: {tool: t.tool}});
    out.push({label: '设置', patch: {settings: '1'}});
    return out;
  }, [packId]);

  const hits = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? commands.filter(c => c.label.toLowerCase().includes(q)) : commands;
  }, [commands, query]);

  const close = () => { setQuery(''); onOpenChange(false); };

  const pickItem = (id: string) => {
    setFocus({kind: 'item', id});
    close();
  };

  const showItems = packId !== null && query.trim() !== '';

  return (
    <Modal open={open} onCancel={close} footer={null} title="命令面板" width={560}>
      <Input autoFocus placeholder="输入命令或物品名…" value={query} onChange={e => setQuery(e.target.value)}/>
      <div className="palette-group">
        <div className="palette-group-title">命令</div>
        {hits.map(c => (
          <div key={c.label} className="palette-item" onClick={() => { patch(c.patch); close(); }}>
            <span>{c.label}</span>
            {focus && <span className="pi-sub">保留当前焦点</span>}
          </div>
        ))}
        {hits.length === 0 && <div className="palette-empty">{showItems ? '没有匹配的命令，下面是物品。' : '没有匹配的命令。'}</div>}
      </div>
      {showItems && (
        <div className="palette-group">
          <div className="palette-group-title">物品</div>
          <StubItemSearch query={query} onPick={pickItem}/>
        </div>
      )}
    </Modal>
  );
}
