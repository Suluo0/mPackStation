import {useEffect, useMemo, useState} from 'react';
import {useSearchParams} from 'react-router-dom';
import {Button, Drawer, Input, Select, Spin, Tag} from 'antd';
import {ReloadOutlined, SearchOutlined, ThunderboltOutlined} from '@ant-design/icons';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import type {CatalogItem} from '../api/catalog';
import {ItemCatalogGrid} from '../features/catalog/ItemCatalogGrid';
import {usePackCatalog} from '../features/pack/PackCatalogContext';
import {useFocus} from '../features/focus/FocusContext';
import '../features/catalog/item-catalog.css';

/* 物品大一统页：包内全部物品的可浏览索引，是工作台各链路的焦点起点。
   数据来自包级共享目录 store（不再各自 fetch）；点物品即设为全局焦点，
   右侧 Inspector 立刻给出合成/魔改/模组的传送入口。 */

export function ItemCatalogPage() {
  const {phase, error, catalog, items, iconUrl, rebuild, rebuilding, reload} = usePackCatalog();
  const {focusItem} = useFocus();
  const [searchParams] = useSearchParams();
  const [query, setQuery] = useState('');
  const [ns, setNs] = useState<string>('');
  const [selectedId, setSelectedId] = useState<string | null>(null);

  /* 深链 ?ns=<命名空间>：从模组页「贡献物品」跳转进来时预选命名空间过滤。 */
  const paramNs = searchParams.get('ns');
  useEffect(() => { if (paramNs) setNs(paramNs); }, [paramNs]);

  const onSelect = (id: string) => {
    setSelectedId(id);
    focusItem(id, '物品页');
  };

  const namespaces = useMemo(() => {
    const counts = new Map<string, number>();
    for (const it of items) {
      const key = it.id.split(':')[0] || 'minecraft';
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1])
      .map(([value, count]) => ({value, label: `${value} · ${count}`}));
  }, [items]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return items.filter(it =>
      (!ns || it.id.startsWith(`${ns}:`))
      && (!q || it.displayName.toLowerCase().includes(q) || it.id.toLowerCase().includes(q)));
  }, [items, query, ns]);

  const selected: CatalogItem | null = useMemo(
    () => items.find(i => i.id === selectedId) ?? null,
    [items, selectedId],
  );

  const total = items.length;

  return (
    <div className="workspace-page">
      <div className="page-heading compact">
        <div>
          <span className="eyebrow">ITEM CATALOG</span>
          <h1>物品</h1>
          <p>包内全部物品的统一索引。选一个物品作为焦点，右侧 Inspector 会给出穿透到合成、模组与任务书的入口。</p>
        </div>
        <span style={{display: 'inline-flex', gap: 8}}>
          <Button icon={<ReloadOutlined/>} onClick={() => void reload()}>刷新</Button>
          <Button icon={<ReloadOutlined/>} loading={rebuilding} onClick={() => void rebuild()}>
            {total ? '重建目录' : '构建物品目录'}
          </Button>
        </span>
      </div>

      {phase === 'failed' && (
        <WorkbenchCard className="catalog-card">
          <div className="empty-inline">
            {error || '物品目录不可用。'}
            <div style={{marginTop: 12}}>
              <WorkbenchButton tone="primary" icon={<ThunderboltOutlined/>} loading={rebuilding} onClick={() => void rebuild()}>
                构建物品目录
              </WorkbenchButton>
            </div>
          </div>
        </WorkbenchCard>
      )}

      {phase === 'building' && (
        <WorkbenchCard className="catalog-card">
          <div className="empty-inline"><Spin/> <span style={{marginLeft: 8}}>物品目录构建中，首次需准备原版资源，请稍候…</span></div>
        </WorkbenchCard>
      )}

      {phase === 'loading' && (
        <WorkbenchCard className="catalog-card"><div className="empty-inline"><Spin/></div></WorkbenchCard>
      )}

      {phase === 'ready' && (
        <>
          <WorkbenchCard className="catalog-card">
            <div className="catalog-toolbar">
              <Input
                allowClear
                autoFocus
                prefix={<SearchOutlined/>}
                placeholder="搜索物品名称或 ID"
                value={query}
                onChange={e => setQuery(e.target.value)}
              />
              <Select
                showSearch
                allowClear
                placeholder="命名空间"
                value={ns || undefined}
                onChange={v => setNs(v ?? '')}
                options={namespaces}
                optionFilterProp="label"
              />
              <Button icon={<ReloadOutlined/>} onClick={() => void reload()}>刷新</Button>
            </div>
            {catalog && catalog.warnings.length > 0 && (
              <p className="icg-toolbar-note">{catalog.warnings.join(' · ')}</p>
            )}
          </WorkbenchCard>

          <WorkbenchCard className="catalog-card">
            <WorkbenchSectionHeader
              title={`物品 (${filtered.length}${filtered.length !== total ? ` / ${total}` : ''})`}
              action={<span className="db-muted">共 {namespaces.length} 个命名空间</span>}
            />
            <ItemCatalogGrid
              items={filtered}
              iconUrl={iconUrl}
              selectedId={selectedId}
              onSelect={onSelect}
              emptyText={total ? '没有匹配的物品。调整搜索或筛选条件。' : '目录为空。添加并解析模组后重建目录。'}
            />
          </WorkbenchCard>
        </>
      )}

      <Drawer
        title="物品详情"
        open={Boolean(selected)}
        onClose={() => setSelectedId(null)}
        width={420}
      >
        {selected && (
          <div className="icg-detail">
            <div className="icg-detail-head">
              <span className="icg-icon">
                {selected.iconStatus === 'ready'
                  ? <img src={iconUrl(selected.id)} alt=""/>
                  : <span className="icg-icon-ph">{selected.displayName.slice(0, 1)}</span>}
              </span>
              <span className="icg-detail-title">
                <strong>{selected.displayName}</strong>
                <code>{selected.id}</code>
              </span>
            </div>

            <div className="icg-detail-sec">
              <h4>属性</h4>
              <div className="icg-detail-rows">
                <div><span>语言</span><b>{selected.resolvedLocale || '—'}</b></div>
                <div><span>图标</span><b>{selected.iconStatus === 'ready' ? '就绪' : selected.iconStatus || '—'}</b></div>
                <div><span>来源</span><b>{selected.evidence || '—'}</b></div>
              </div>
            </div>

            <div className="icg-detail-sec">
              <h4>所属标签</h4>
              {selected.tags.length
                ? selected.tags.map(t => <Tag key={t} style={{marginBottom: 6}}>#{t}</Tag>)
                : <span className="db-muted">无标签关联</span>}
            </div>

            {selected.names.length > 0 && (
              <div className="icg-detail-sec">
                <h4>多语言名称</h4>
                <table className="icg-names-table">
                  <tbody>
                    {selected.names.map(n => (
                      <tr key={`${n.locale}-${n.key}`}>
                        <td>{n.locale}</td>
                        <td>{n.name}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </Drawer>
    </div>
  );
}
