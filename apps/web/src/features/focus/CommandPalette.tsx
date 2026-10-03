import {useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate} from 'react-router-dom';
import {Modal} from 'antd';
import {
  AppstoreOutlined, BookOutlined, EditOutlined, ExperimentOutlined,
  GoldOutlined, HomeOutlined, RocketOutlined, SearchOutlined,
} from '@ant-design/icons';
import {useFocus} from './FocusContext';
import {usePackCatalog} from '../pack/PackCatalogContext';

/* Cmd/Ctrl+K 命令面板：一个入口跳到任何页面或任何物品。
   物品命中 → 设为全局焦点并跳合成器；页面命中 → 直接导航。
   这是「一个工作台」的快捷键骨架，替代在各页面间靠侧栏逐级点。 */

type Cmd = {key: string; label: string; hint?: string; icon: React.ReactNode; run: () => void};

export function CommandPalette({packId}: {packId: string}) {
  const navigate = useNavigate();
  const {items, iconUrl, itemById} = usePackCatalog();
  const {focusItem} = useFocus();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState('');
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setOpen(v => !v);
      } else if (e.key === 'Escape') {
        setOpen(false);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    if (open) { setQ(''); setCursor(0); setTimeout(() => inputRef.current?.focus(), 30); }
  }, [open]);

  const go = (suffix: string) => { setOpen(false); navigate(`/packs/${packId}${suffix}`); };

  const pageCmds: Cmd[] = useMemo(() => [
    {key: 'nav-overview', label: '概览', icon: <HomeOutlined/>, run: () => go('')},
    {key: 'nav-mods', label: '模组', icon: <AppstoreOutlined/>, run: () => go('/mods')},
    {key: 'nav-items', label: '物品大一统', icon: <GoldOutlined/>, run: () => go('/items')},
    {key: 'nav-recipes', label: '合成器', icon: <ExperimentOutlined/>, run: () => go('/recipes')},
    {key: 'nav-content', label: '模组内容', icon: <AppstoreOutlined/>, run: () => go('/content')},
    {key: 'nav-tweak', label: '魔改', icon: <EditOutlined/>, run: () => go('/tweak')},
    {key: 'nav-quests', label: '任务书', icon: <BookOutlined/>, run: () => go('/quests')},
    {key: 'nav-publish', label: '打包与发布', icon: <RocketOutlined/>, run: () => go('/publish')},
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [packId]);

  const query = q.trim().toLowerCase();
  const pages = query ? pageCmds.filter(c => c.label.toLowerCase().includes(query)) : pageCmds;
  const matchedItems = query
    ? items.filter(it => it.displayName.toLowerCase().includes(query) || it.id.toLowerCase().includes(query)).slice(0, 8)
    : [];

  const rows: (Cmd | {item: string})[] = [
    ...pages,
    ...matchedItems.map(it => ({item: it.id})),
  ];

  const pick = (idx: number) => {
    const row = rows[idx];
    if (!row) return;
    if ('run' in row) { row.run(); return; }
    const it = row.item;
    setOpen(false);
    focusItem(it, '命令面板');
    navigate(`/packs/${packId}/recipes`);
  };

  const onInputKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setCursor(c => Math.min(c + 1, rows.length - 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setCursor(c => Math.max(c - 1, 0)); }
    else if (e.key === 'Enter') { e.preventDefault(); pick(cursor); }
  };

  return (
    <Modal
      open={open}
      onCancel={() => setOpen(false)}
      footer={null}
      closable={false}
      width={560}
      styles={{body: {padding: 0}}}
      className="cmdk-modal"
    >
      <div className="cmdk-input-row">
        <SearchOutlined/>
        <input
          ref={inputRef}
          className="cmdk-input"
          placeholder="跳转到页面，或搜索物品…（↑↓ 选择，Enter 打开）"
          value={q}
          onChange={e => { setQ(e.target.value); setCursor(0); }}
          onKeyDown={onInputKey}
        />
        <kbd>ESC</kbd>
      </div>
      <div className="cmdk-list">
        {pages.length > 0 && <div className="cmdk-group">页面</div>}
        {pages.map((c, i) => (
          <button
            key={c.key}
            className={`cmdk-row ${cursor === i ? 'active' : ''}`}
            onMouseEnter={() => setCursor(i)}
            onClick={() => pick(i)}
          >
            <span className="cmdk-icon">{c.icon}</span>
            <span className="cmdk-label">{c.label}</span>
          </button>
        ))}
        {matchedItems.length > 0 && <div className="cmdk-group">物品</div>}
        {matchedItems.map((it, j) => {
          const idx = pages.length + j;
          return (
            <button
              key={it.id}
              className={`cmdk-row ${cursor === idx ? 'active' : ''}`}
              onMouseEnter={() => setCursor(idx)}
              onClick={() => pick(idx)}
            >
              <span className="cmdk-icon">
                {itemById.get(it.id)?.iconStatus === 'ready'
                  ? <img src={iconUrl(it.id)} alt=""/>
                  : <GoldOutlined/>}
              </span>
              <span className="cmdk-label">{it.displayName}</span>
              <span className="cmdk-hint">{it.id}</span>
            </button>
          );
        })}
        {!pages.length && !matchedItems.length && <div className="cmdk-empty">没有匹配项</div>}
      </div>
    </Modal>
  );
}
