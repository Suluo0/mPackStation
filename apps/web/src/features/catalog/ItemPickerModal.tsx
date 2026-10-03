import {useMemo, useState} from 'react';
import {Input, Modal, Spin} from 'antd';
import {SearchOutlined} from '@ant-design/icons';
import {ItemCatalogGrid} from './ItemCatalogGrid';
import {usePackCatalog} from '../pack/PackCatalogContext';
import './item-catalog.css';

/* 通用物品选择器：复用包级共享目录 + ItemCatalogGrid，弹窗选一个物品 ID。
   任务书奖励/目标、任何需要「填物品 ID」的地方都用它，替代盲填字符串 —— 让任务书也接入物品通用语。 */

export function ItemPickerModal({open, onClose, onPick, title = '选择物品'}: {
  open: boolean;
  onClose: () => void;
  onPick: (itemId: string) => void;
  title?: string;
}) {
  const {phase, items, iconUrl} = usePackCatalog();
  const [q, setQ] = useState('');

  const filtered = useMemo(() => {
    const kw = q.trim().toLowerCase();
    if (!kw) return items;
    return items.filter(it => it.displayName.toLowerCase().includes(kw) || it.id.toLowerCase().includes(kw));
  }, [items, q]);

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width={760}
      title={title}
      styles={{body: {paddingTop: 8}}}
    >
      <Input
        allowClear
        autoFocus
        prefix={<SearchOutlined/>}
        placeholder="搜索物品名称或 ID"
        value={q}
        onChange={e => setQ(e.target.value)}
        style={{marginBottom: 12}}
      />
      {phase === 'ready' ? (
        <div style={{maxHeight: '60vh', overflow: 'auto'}}>
          <ItemCatalogGrid
            items={filtered}
            iconUrl={iconUrl}
            limit={400}
            onSelect={(id) => { onPick(id); onClose(); }}
            emptyText={items.length ? '没有匹配的物品。' : '物品目录为空，请先在物品页构建目录。'}
          />
        </div>
      ) : (
        <div className="empty-inline"><Spin/> <span style={{marginLeft: 8}}>物品目录加载中…（如未构建，请到「物品」页构建）</span></div>
      )}
    </Modal>
  );
}
