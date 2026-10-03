import {useMemo, useState} from 'react';
import {Modal, Input} from 'antd';
import {useLocation, useNavigate, useSearchParams} from 'react-router-dom';
import {pageCommands, type ContentMode} from './nav';
import {useFocus} from './useFocus';
import {usePackSummary} from './PackSummaryContext';
import {StubItemSearch} from '../stubs/StubItemSearch';

type Props = {open: boolean; onOpenChange: (v: boolean) => void};

/* ⌘K 命令面板（设计文档 §4.5）：切态 + 设焦点。
   全局键盘监听在 useHotkeys 里，本组件只受控，避免两处监听。 */
export function CommandPalette({open, onOpenChange}: Props) {
  const [query, setQuery] = useState('');
  const nav = useNavigate();
  const loc = useLocation();
  const [params, setParams] = useSearchParams();
  const {packId, anchorPackId} = usePackSummary();
  const [focus, setFocus] = useFocus();

  const commands = useMemo(() => pageCommands(anchorPackId), [anchorPackId]);
  const hits = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? commands.filter(c => c.label.toLowerCase().includes(q)) : commands;
  }, [commands, query]);

  const close = () => { setQuery(''); onOpenChange(false); };

  /** 跨页导航把当前查询参数带上（焦点跟着走）；页内锚点参数不带过去。 */
  const goto = (href: string, mode?: ContentMode) => {
    const u = new URL(href, window.location.origin);
    if (u.pathname !== loc.pathname) {
      const q = new URLSearchParams(params);
      q.delete('panel'); q.delete('step');
      if (mode) q.set('mode', mode); else q.delete('mode');
      const qs = q.toString();
      nav(`${u.pathname}${qs ? `?${qs}` : ''}`, {replace: false});
    } else {
      setParams(p => {
        const n = new URLSearchParams(p);
        if (mode) n.set('mode', mode); else n.delete('mode');
        return n;
      }, {replace: false});
    }
    close();
  };

  /* 物品命中：保留当前 mode，不硬跳某个页面（设计文档 §4.5 的 ⚠）。 */
  const pickItem = (id: string) => {
    setFocus({kind: 'item', id}, {mode: (params.get('mode') ?? undefined) as ContentMode | undefined});
    close();
  };

  const showItems = packId !== null && query.trim() !== '';

  return (
    <Modal open={open} onCancel={close} footer={null} title="命令面板" width={560}>
      <Input autoFocus placeholder="输入命令或物品名…" value={query} onChange={e => setQuery(e.target.value)}/>
      <div className="palette-group">
        <div className="palette-group-title">页面与态</div>
        {hits.map(c => (
          <div key={c.href + (c.mode ?? '')} className="palette-item" onClick={() => goto(c.href, c.mode)}>
            <span>{c.label}</span>
            {focus && <span className="pi-sub">保留当前焦点</span>}
          </div>
        ))}
        {hits.length === 0 && !showItems && <div className="palette-empty">没有匹配的命令。</div>}
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
