import {useCallback, useEffect, useMemo, useState} from 'react';
import {useNavigate, useParams, useSearchParams} from 'react-router-dom';
import {Button, Empty, Input, Spin, Tag} from 'antd';
import {ExperimentOutlined, ReloadOutlined, SearchOutlined, ThunderboltOutlined} from '@ant-design/icons';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import {ItemCatalogGrid} from '../features/catalog/ItemCatalogGrid';
import {RecipeViewer} from '../features/recipe/RecipeViewer';
import {usePackCatalog} from '../features/pack/PackCatalogContext';
import {useFocus} from '../features/focus/FocusContext';
import type {CatalogRecipe} from '../api/catalog';
import '../features/catalog/item-catalog.css';

/* 合成器页（物品中心 · 双向）：选一个物品，同时看它「怎么合出来」(作为产物) 和「被用于什么」(作为原料)。
   配方网格里点任意素材 → 设为焦点并就地重新居中，形成可无限穿透的浏览链，不再是死胡同弹窗。
   数据全部来自包级共享目录 store，与物品页/Inspector 同源。 */

export function RecipeBrowserPage() {
  const {id = ''} = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const {phase, error, items, itemById, recipesByOutput, recipesByInput, displayName, iconUrl, rebuild, rebuilding, reload} = usePackCatalog();
  const {focus, focusItem} = useFocus();
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(null);

  /* 焦点为物品时，合成器跟随居中（Inspector / Cmd+K / 其它页设的焦点都能穿透到这里）。 */
  useEffect(() => {
    if (focus?.type === 'item') setSelectedId(focus.id);
  }, [focus]);

  /* 深链 ?item=<id>：供 Inspector「查看合成关系」与可分享 URL 直接居中某物品。 */
  const paramItem = searchParams.get('item');
  useEffect(() => {
    if (paramItem) { setSelectedId(paramItem); focusItem(paramItem, '深链'); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [paramItem]);

  const getItemIcon = useCallback((k: string): string | null => {
    if (!k || k.startsWith('#')) return null;
    const it = itemById.get(k);
    return it && it.iconStatus === 'ready' ? iconUrl(k) : null;
  }, [itemById, iconUrl]);

  /* 点素材：设全局焦点 + 本页重新居中。 */
  const onSelect = useCallback((itemId: string) => {
    if (itemId.startsWith('#')) return; // 标签暂不作为居中目标
    setSelectedId(itemId);
    focusItem(itemId, '合成器');
  }, [focusItem]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return items;
    return items.filter(it => it.displayName.toLowerCase().includes(q) || it.id.toLowerCase().includes(q));
  }, [items, query]);

  const asOutput: CatalogRecipe[] = useMemo(
    () => (selectedId ? recipesByOutput.get(selectedId) ?? [] : []), [selectedId, recipesByOutput]);
  const asInput: CatalogRecipe[] = useMemo(
    () => (selectedId ? recipesByInput.get(selectedId) ?? [] : []), [selectedId, recipesByInput]);

  const selected = selectedId ? itemById.get(selectedId) ?? null : null;

  if (phase === 'loading') {
    return <div className="workspace-page"><WorkbenchCard className="catalog-card"><div className="empty-inline"><Spin/></div></WorkbenchCard></div>;
  }
  if (phase === 'failed') {
    return <div className="workspace-page">
      <div className="page-heading compact"><div><span className="eyebrow">RECIPE BROWSER</span><h1>合成</h1></div></div>
      <WorkbenchCard className="catalog-card"><div className="empty-inline">
        {error || '物品目录不可用，无法浏览合成关系。'}
        <div style={{marginTop: 12}}>
          <WorkbenchButton tone="primary" icon={<ThunderboltOutlined/>} loading={rebuilding} onClick={() => void rebuild()}>构建物品目录</WorkbenchButton>
        </div>
      </div></WorkbenchCard>
    </div>;
  }

  return (
    <div className="workspace-page">
      <div className="page-heading compact">
        <div>
          <span className="eyebrow">RECIPE BROWSER</span>
          <h1>合成</h1>
          <p>以物品为中心双向浏览：它怎么合出来、又被用于什么。点配方里的任意素材即可穿透过去。</p>
        </div>
        <span style={{display: 'inline-flex', gap: 8}}>
          <Button icon={<ReloadOutlined/>} onClick={() => void reload()}>刷新</Button>
          <Button icon={<ReloadOutlined/>} loading={rebuilding} onClick={() => void rebuild()}>重建目录</Button>
        </span>
      </div>

      {phase === 'building' && (
        <WorkbenchCard className="catalog-card"><div className="empty-inline"><Spin/> <span style={{marginLeft: 8}}>物品目录构建中…</span></div></WorkbenchCard>
      )}

      {phase === 'ready' && (
        <div className="rb-layout">
          <aside className="rb-picker">
            <Input
              allowClear
              prefix={<SearchOutlined/>}
              placeholder="搜索物品"
              value={query}
              onChange={e => setQuery(e.target.value)}
            />
            <div className="rb-picker-scroll">
              <ItemCatalogGrid
                items={filtered}
                iconUrl={iconUrl}
                selectedId={selectedId}
                onSelect={onSelect}
                limit={400}
                emptyText="没有匹配的物品。"
              />
            </div>
          </aside>

          <main className="rb-main">
            {!selected ? (
              <WorkbenchCard className="catalog-card rb-empty">
                <Empty
                  image={<ExperimentOutlined style={{fontSize: 40, color: 'var(--mc-primary)'}}/>}
                  description="从左侧选一个物品，查看它的合成方式与用途。"
                />
              </WorkbenchCard>
            ) : (
              <>
                <WorkbenchCard className="catalog-card rb-focus-head">
                  <span className="icg-icon">
                    {selected.iconStatus === 'ready'
                      ? <img src={iconUrl(selected.id)} alt=""/>
                      : <span className="icg-icon-ph">{selected.displayName.slice(0, 1)}</span>}
                  </span>
                  <span className="rb-focus-title">
                    <strong>{selected.displayName}</strong>
                    <code>{selected.id}</code>
                  </span>
                  <span className="rb-focus-actions">
                    <Button size="small" onClick={() => navigate(`/packs/${id}/items`)}>在物品页打开</Button>
                  </span>
                </WorkbenchCard>

                <WorkbenchCard className="catalog-card">
                  <WorkbenchSectionHeader
                    title={`合成方式 · 作为产物 (${asOutput.length})`}
                    action={<Tag color="green">怎么得到它</Tag>}
                  />
                  {asOutput.length
                    ? asOutput.map(r => (
                        <div className="rb-recipe" key={r.id}>
                          <div className="rb-recipe-meta"><code>{r.type}</code>{r.status !== 'ok' && <Tag color="gold">{r.status}</Tag>}</div>
                          <RecipeViewer payload={r.payload} translateKey={displayName} getItemIcon={getItemIcon} onSelect={onSelect}/>
                        </div>
                      ))
                    : <div className="empty-inline">没有以该物品为产物的配方（可能是挖掘/掉落/战利品来源）。</div>}
                </WorkbenchCard>

                <WorkbenchCard className="catalog-card">
                  <WorkbenchSectionHeader
                    title={`被用于 · 作为原料 (${asInput.length})`}
                    action={<Tag color="blue">它能合成什么</Tag>}
                  />
                  {asInput.length
                    ? asInput.map(r => (
                        <div className="rb-recipe" key={r.id}>
                          <div className="rb-recipe-meta"><code>{r.type}</code>{r.status !== 'ok' && <Tag color="gold">{r.status}</Tag>}</div>
                          <RecipeViewer payload={r.payload} translateKey={displayName} getItemIcon={getItemIcon} onSelect={onSelect}/>
                        </div>
                      ))
                    : <div className="empty-inline">没有把该物品作为原料的配方。</div>}
                </WorkbenchCard>
              </>
            )}
          </main>
        </div>
      )}
    </div>
  );
}
