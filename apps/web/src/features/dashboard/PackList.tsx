import {useMemo, useState} from 'react';
import {App, Button, Dropdown, Switch, Tooltip} from 'antd';
import {MoreOutlined, PlayCircleOutlined} from '@ant-design/icons';
import type {DashboardPack} from './types';
import {AlertSignals, ConflictSignal, relativeTime} from './signals';
import {PackCover, loaderLabel} from './ContinueCard';

/* 整合包行式列表：每包一行的健康一览，默认按最后编辑倒序，支持「只看待处理」。 */

export function PackList({packs, onOpen, onDelete}: {
  packs: DashboardPack[];
  onOpen: (id: string) => void;
  onDelete: (id: string) => Promise<void>;
}) {
  const {message, modal} = App.useApp();
  const [onlyPending, setOnlyPending] = useState(false);

  const rows = useMemo(() => {
    const sorted = [...packs].sort((a, b) => b.lastEditedAt.localeCompare(a.lastEditedAt));
    return onlyPending ? sorted.filter(p => p.conflicts.pending > 0 || p.alerts.crashes > 0) : sorted;
  }, [packs, onlyPending]);

  const confirmDelete = (pack: DashboardPack) => {
    modal.confirm({
      title: `删除「${pack.name}」？`,
      content: '不会删除已下载的模组文件缓存。',
      okText: '删除', okButtonProps: {danger: true}, cancelText: '取消',
      onOk: async () => {
        await onDelete(pack.id);
        message.success(`已删除「${pack.name}」`);
      },
    });
  };

  return (
    <div className="db-card db-packlist">
      <div className="db-section-head">
        <h2 className="db-h2">全部整合包</h2>
        <label className="db-muted" style={{cursor: 'pointer', userSelect: 'none', display: 'flex', alignItems: 'center', gap: 8}}>
          只看待处理
          <Switch size="small" checked={onlyPending} onChange={setOnlyPending}/>
        </label>
      </div>
      <div className="db-pack-grid db-pack-row-head">
        <span>整合包</span>
        <span className="db-col-hide-sm">环境</span>
        <span className="db-col-hide-sm">模组数</span>
        <span>健康状态</span>
        <span className="db-col-hide-sm">最后编辑</span>
        <span>操作</span>
      </div>
      {rows.map(pack => (
        <div key={pack.id} className="db-pack-grid db-pack-row" onClick={() => onOpen(pack.id)}>
          <div className="db-pack-name">
            <PackCover id={pack.id} small/>
            <span className="db-pack-name-text" title={pack.name}>{pack.name}</span>
            <span className="db-pack-ver">v{pack.packVersion}</span>
          </div>
          <div className="db-col-hide-sm">
            <span className="db-env-tag db-env-tag-mc">{pack.mcVersion}</span>
            <span className="db-env-tag db-env-tag-loader">{loaderLabel(pack.loader)}</span>
          </div>
          <div className="db-col-hide-sm">
            <Tooltip title={`已安装 ${pack.modCount.installed} · 已选择未装 ${pack.modCount.selected}`}>
              <div>
                <span className="db-mod-count">{pack.modCount.total}</span> <span className="db-muted">模组</span>
                <div className="db-muted">已装 {pack.modCount.installed} · 未装 {pack.modCount.selected}</div>
              </div>
            </Tooltip>
          </div>
          <div className="db-pack-health">
            <ConflictSignal resolved={pack.conflicts.resolved} pending={pack.conflicts.pending}/>
            <AlertSignals crashes={pack.alerts.crashes} updatable={pack.alerts.updatable}/>
          </div>          <span className="db-muted db-col-hide-sm">{relativeTime(pack.lastEditedAt)}</span>
          <div onClick={e => e.stopPropagation()} style={{display: 'flex', gap: 4}}>
            <Tooltip title="打开">
              <Button type="text" icon={<PlayCircleOutlined/>} onClick={() => onOpen(pack.id)}/>
            </Tooltip>
            <Dropdown menu={{
              items: [
                {key: 'rename', label: '重命名'},
                {key: 'duplicate', label: '复制包'},
                {key: 'export', label: '一键打包'},
                {type: 'divider'},
                {key: 'delete', label: '删除', danger: true},
              ],
              onClick: ({key, domEvent}) => {
                domEvent.stopPropagation();
                if (key === 'delete') confirmDelete(pack);
                else message.info('该功能将在后续版本提供');
              },
            }} trigger={['click']}>
              <Button type="text" icon={<MoreOutlined/>}/>
            </Dropdown>
          </div>
        </div>
      ))}
      {rows.length === 0 && <div className="db-muted" style={{padding: 'var(--mc-sp) 0'}}>没有待处理的整合包。</div>}
    </div>
  );
}
