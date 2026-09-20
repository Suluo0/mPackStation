import {useCallback, useEffect, useMemo, useState} from 'react';
import {useParams} from 'react-router-dom';
import {Button, Select, Tag, Empty, Spin, Modal} from 'antd';
import {
  AppstoreOutlined, BookOutlined, DatabaseOutlined, ExperimentOutlined,
  FileTextOutlined, FireOutlined, GiftOutlined, GlobalOutlined, PlayCircleOutlined, ReloadOutlined,
  TagsOutlined,
} from '@ant-design/icons';
import {PackContext} from './PackPages';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import {listContentSources, type Mod} from '../api/mods';
import {
  getModContentRun, listModContent, parseModContent, resolveModContentIcons,
  type ModContentItem, type ModContentRun,
} from '../api/modContent';
import {ApiError} from '../api/http';
import {getCatalogStatus, getItemCatalog, rebuildItemCatalog, type CatalogItem, type CatalogTag} from '../api/catalog';
import {AdvancementTreeView} from '../features/content/AdvancementTreeView';
import {itemsToAdvTrees} from '../features/content/advLayout';

/* 内容编辑页：接入模组内容提取引擎。
   顶部选模组 → 解析状态 + 触发解析 → 页内三级子菜单(按 kind 过滤) → 内容列表 → 点击查看 payload 详情。 */

type KindTab = {
  kind: string;
  label: string;
  icon: React.ReactNode;
};

const KIND_TABS: KindTab[] = [
  {kind: '', label: '全部', icon: <AppstoreOutlined/>},
  {kind: 'recipe', label: '配方', icon: <ExperimentOutlined/>},
  {kind: 'item_model', label: '物品模型', icon: <GiftOutlined/>},
  {kind: 'structure', label: '结构', icon: <DatabaseOutlined/>},
  {kind: 'worldgen', label: '地形生成', icon: <FireOutlined/>},
  {kind: 'loot_table', label: '战利品', icon: <TagsOutlined/>},
  {kind: 'advancement', label: '进度', icon: <BookOutlined/>},
  {kind: 'tag', label: '标签', icon: <TagsOutlined/>},
  {kind: 'metadata', label: '元数据', icon: <FileTextOutlined/>},
  {kind: 'lang', label: '语言', icon: <GlobalOutlined/>},
];

const kindLabel = (kind: string) => KIND_TABS.find(t => t.kind === kind)?.label ?? kind;

/* 原版/特殊合成类型：运行时逻辑，无固定合成表。 */
function isSpecialRecipeType(type: string): boolean {
  return type.startsWith('minecraft:crafting_special_') || type === 'minecraft:crafting_decorated_pot';
}

/* payload 是否属于特殊/动态配方（无 ingredients/result）。 */
function isSpecialRecipePayload(payload: unknown): boolean {
  if (!payload || typeof payload !== 'object') return false;
  const p = payload as Record<string, unknown>;
  const type = String(p.type || '');
  if (type === 'ae2:matter_cannon') return false;
  if (isSpecialRecipeType(type)) return true;
  return type !== '' && p.ingredients === undefined && p.ingredient === undefined
    && p.result === undefined && p.output === undefined;
}

/* 解析配方 payload, 构建 3x3 合成网格。支持 crafting_shaped / crafting_shapeless。special 返回空网格。 */
function parseRecipeGrid(payload: unknown): {grid: (string | null)[][]; output: {id: string; count: number} | null; type: string; special: boolean} {
  if (!payload || typeof payload !== 'object') return {grid: [], output: null, type: 'unknown', special: false};
  const p = payload as Record<string, unknown>;
  const type = String(p.type || 'unknown');
  const result = p.result as Record<string, unknown> | undefined;
  const output = result ? {id: String(result.id || result.item || ''), count: Number(result.count || 1)} : null;
  const special = isSpecialRecipePayload(payload);

  // crafting_shaped: pattern + key
  if (!special && type === 'minecraft:crafting_shaped' && Array.isArray(p.pattern) && p.key && typeof p.key === 'object') {
    const key = p.key as Record<string, {item?: string; id?: string; tag?: string}>;
    const grid: (string | null)[][] = [];
    for (let row = 0; row < 3; row++) {
      const gridRow: (string | null)[] = [];
      const patternRow = String((p.pattern as string[])[row] || '');
      for (let col = 0; col < 3; col++) {
        const char = patternRow[col] || ' ';
        if (char === ' ') { gridRow.push(null); continue; }
        const keyEntry = key[char];
        const itemId = keyEntry?.item || keyEntry?.id || (keyEntry?.tag ? `#${keyEntry.tag}` : '');
        gridRow.push(itemId || null);
      }
      grid.push(gridRow);
    }
    return {grid, output, type, special: false};
  }

  // crafting_shapeless / ae2:transform: ingredients 数组 → 输出
  if (!special && (type === 'minecraft:crafting_shapeless' || type === 'ae2:transform') && Array.isArray(p.ingredients)) {
    const grid: (string | null)[][] = [[null, null, null], [null, null, null], [null, null, null]];
    let idx = 0;
    for (const ing of p.ingredients as Array<{item?: string; id?: string; tag?: string}>) {
      const itemId = ing?.item || ing?.id || (ing?.tag ? `#${ing.tag}` : '');
      if (itemId && idx < 9) {
        grid[Math.floor(idx / 3)][idx % 3] = itemId;
        idx++;
      }
    }
    return {grid, output, type, special: false};
  }

  return {grid: [], output, type, special};
}

/* 配方解锁进度（advancement）不是合成配方：payload 有 parent/criteria，无 type。 */
function isAdvancementPayload(payload: unknown): boolean {
  if (!payload || typeof payload !== 'object') return false;
  const p = payload as Record<string, unknown>;
  return (typeof p.criteria === 'object' && p.criteria !== null)
    || (typeof p.parent === 'string' && String(p.parent).includes('recipes/'))
    || (typeof p.rewards === 'object' && p.rewards !== null && typeof p.type !== 'string');
}

/* JEI 风格配方合成网格视图: 3x3 输入 + 箭头 + 输出。special 显示运行时逻辑说明。 */
function RecipeViewer({payload, translateKey, getItemIcon, onSelect, itemKind}: {payload: unknown; translateKey: (k: string) => string; getItemIcon: (k: string) => string | null; onSelect: (id: string) => void; itemKind?: string}) {
  const {grid, output, type, special} = parseRecipeGrid(payload);

  if (itemKind === 'advancement' || (type === 'unknown' && isAdvancementPayload(payload))) {
    return (
      <div className="recipe-viewer unsupported">
        这是<strong>配方解锁进度（advancement）</strong>，不是合成配方。
        原版会把 <code>data/&lt;ns&gt;/advancement/recipes/**</code> 用来解锁配方展示，真正的合成配方在 <code>recipe/</code> 目录。
      </div>
    );
  }

  // ae2:matter_cannon 弹药属性, 不是合成配方
  if (type === 'ae2:matter_cannon' && payload && typeof payload === 'object') {
    const p = payload as Record<string, unknown>;
    const ammo = p.ammo as {item?: string; id?: string} | undefined;
    const ammoId = ammo?.item || ammo?.id || '';
    const weight = Number(p.weight || 0);
    return (
      <div className="recipe-viewer ammo-viewer">
        <div className="ammo-card">
          <div className="ammo-label">弹药</div>
          <div className="recipe-slot filled">
            {ammoId && getItemIcon(ammoId) && <img className="recipe-icon" src={getItemIcon(ammoId)!} alt=""/>}
            <span className="recipe-item-name">{ammoId ? translateKey(ammoId) : '未知'}</span>
          </div>
          <div className="ammo-weight">重量: <strong>{weight}</strong></div>
        </div>
      </div>
    );
  }

  if (special) {
    return (
      <div className="recipe-viewer special-recipe">
        <div className="special-recipe-note">特殊合成配方（运行时逻辑，无固定合成表）</div>
        <div className="special-recipe-type"><code>{type}</code></div>
        {type.includes('bookcloning') && (
          <div className="special-recipe-hint">
            书与笔复制：将「书与笔」与「已写成的书」放入合成栏，可复制书的内容（每份副本消耗书与笔的墨水）。
          </div>
        )}
        {output && (
          <div className="special-recipe-output">
            可能产出：<code onClick={() => onSelect(output.id)} style={{cursor: 'pointer'}}>{translateKey(output.id)}</code>
          </div>
        )}
      </div>
    );
  }

  if (!grid.length) {
    return <div className="recipe-viewer unsupported">该配方类型暂不支持合成网格预览: <code>{type}</code></div>;
  }
  return (
    <div className="recipe-viewer">
      <div className="recipe-grid">
        {grid.map((row, ri) => (
          <div key={ri} className="recipe-row">
            {row.map((itemId, ci) => (
              <div key={ci} className={`recipe-slot ${itemId ? 'filled' : 'empty'}`} onClick={() => itemId && onSelect(itemId)} title={itemId?.startsWith('#') ? `${itemId}（标签；点击查看所有候选素材）` : itemId || ''}>
                {itemId && (
                  <>
                    {getItemIcon(itemId) && <img className="recipe-icon" src={getItemIcon(itemId)!} alt=""/>}
                    <span className="recipe-item-name">{translateKey(itemId)}</span>
                  </>
                )}
              </div>
            ))}
          </div>
        ))}
      </div>
      <div className="recipe-arrow">→</div>
      <div className="recipe-output">
        {output ? (
          <div className="recipe-slot filled output-slot" onClick={() => onSelect(output.id)} title={output.id}>
            {getItemIcon(output.id) && <img className="recipe-icon" src={getItemIcon(output.id)!} alt=""/>}
            <span className="recipe-item-name">{translateKey(output.id)}</span>
            {output.count > 1 && <span className="recipe-count">×{output.count}</span>}
          </div>
        ) : <div className="recipe-slot empty"><span className="recipe-item-name">无输出</span></div>}
      </div>
    </div>
  );
}

export function ModContentPage() {
  const {id = ''} = useParams();
  const [mods, setMods] = useState<Mod[]>([]);
  const [modsLoading, setModsLoading] = useState(true);
  const [selectedModId, setSelectedModId] = useState<string>('');
  const [activeKind, setActiveKind] = useState<string>('');
  const [run, setRun] = useState<ModContentRun | null>(null);
  const [items, setItems] = useState<ModContentItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [parsing, setParsing] = useState(false);
  const [error, setError] = useState<string>('');
  const [detailItem, setDetailItem] = useState<ModContentItem | null>(null);
  const [catalogItems, setCatalogItems] = useState<Record<string, CatalogItem>>({});
  const [catalogTags, setCatalogTags] = useState<Record<string, CatalogTag>>({});
  const [relationKey, setRelationKey] = useState('');
  const [itemIconMap, setItemIconMap] = useState<Record<string, string>>({});

  const [iconsLoading, setIconsLoading] = useState(false);
  const [iconWarnings, setIconWarnings] = useState<string[]>([]);
  const [iconRefresh, setIconRefresh] = useState(0);
  const [catalogRefresh, setCatalogRefresh] = useState(0);
  const [catalogRebuilding, setCatalogRebuilding] = useState(false);
  /* 进度页：树视图 / 表格切换（M1 树画布） */
  const [advViewMode, setAdvViewMode] = useState<'tree' | 'table'>('tree');

  /* 把模组内容 ID(ae2:misc/fluix_pearl) 翻译成语言文件里的显示名。
     语言文件 key 格式: item.ae2.misc.fluix_pearl / block.ae2.misc.fluix_pearl */
  const translateKey = useCallback((key: string): string => {
    if (!key) return key;
    if (key.startsWith('#')) return catalogTags[key.slice(1)]?.displayName || key;
    return catalogItems[key]?.displayName || key;
  }, [catalogItems, catalogTags]);

  /* 对配方类型, 从 payload 提取输出物品 ID 和数量, 返回显示信息。
     special/动态配方无固定产物 → 「特殊配方」或 lang 名，避免裸 key。 */
  const getRecipeDisplay = useCallback((item: ModContentItem): {name: string; techId: string} | null => {
    if (item.kind !== 'recipe' || !item.payload) return null;
    try {
      const payload = typeof item.payload === 'string' ? JSON.parse(item.payload) : item.payload;
      const type = String(payload?.type || '');
      if (isSpecialRecipePayload(payload) || item.isDynamic) {
        const techId = type || item.key;
        const candidates = [`recipe.${item.key}`, `recipe.${type}`, `recipe.minecraft.${item.key.split(':').pop()}`, item.key];
        let name = '特殊配方';
        for (const k of candidates) {
          if (!k) continue;
          const t = translateKey(k);
          if (t && t !== k) { name = t; break; }
        }
        return {name, techId};
      }
      const result = payload?.result;
      if (!result?.id) return null;
      const id = result.id as string;
      const count = result.count ?? 1;
      const name = translateKey(id);
      return {name: count > 1 ? `${name} ×${count}` : name, techId: id};
    } catch {
      return null;
    }
  }, [translateKey]);

  /* 加载包内模组列表。 */
  useEffect(() => {
    let cancelled = false;
    setModsLoading(true);
    listContentSources(id).then(ms => {
      if (cancelled) return;
      setMods(ms);
      if (ms.length && !selectedModId) {
        // 优先选 modrinth 来源(cursecdn/forge 可能需要 API key); 没有则选第一个
        const modrinth = ms.find(m => m.source === 'modrinth');
        setSelectedModId(modrinth ? modrinth.id : ms[0].id);
      }
    }).catch(e => setError(String(e))).finally(() => !cancelled && setModsLoading(false));
    return () => { cancelled = true; };
  }, [id]);

  /* 加载选中模组的解析状态 + 内容列表。 */
  const loadContent = useCallback(async () => {
    if (!selectedModId) return;
    setLoading(true);
    setError('');
    try {
      const [listRes, runRes] = await Promise.all([
        listModContent(id, selectedModId, {kind: activeKind || undefined, limit: activeKind === 'advancement' ? 500 : 200}),
        getModContentRun(id, selectedModId).catch(e => {
          if (e instanceof ApiError && e.status === 404) return null;
          throw e;
        }),
      ]);
      setItems(listRes.items);
      setTotal(listRes.total);
      setRun(runRes);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        setItems([]); setTotal(0); setRun(null);
      } else {
        setError(String(e));
      }
    } finally {
      setLoading(false);
    }
  }, [id, selectedModId, activeKind]);

  useEffect(() => { void loadContent(); }, [loadContent]);

  /* 加载包级目录；初始化任务运行时轮询，成功后名称和标签关系一次性切换。 */
  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    getItemCatalog(id, 'zh_cn').then(catalog => {
      if (cancelled) return;
      setCatalogItems(Object.fromEntries(catalog.items.map(item => [item.id, item])));
      setCatalogTags(Object.fromEntries(catalog.tags.filter(tag => tag.registry === 'item').map(tag => [tag.id, tag])));
    }).catch(async e => {
      if (cancelled) return;
      if (e instanceof ApiError && e.status === 409) {
        const status = await getCatalogStatus(id).catch(() => null);
        if (!cancelled && status && (status.status === 'pending' || status.status === 'running')) timer = window.setTimeout(() => setCatalogRefresh(v => v + 1), 2000);
        if (!cancelled && status?.status === 'failed') setIconWarnings(old => [...old, `内容目录初始化失败：${status.lastError}`]);
      }
    });
    return () => { cancelled = true; if (timer) window.clearTimeout(timer); };
  }, [id, catalogRefresh, run?.parsedAt]);

  /* 加载选中模组的 item_icon(物品栏渲染图标), 构建 物品ID→dataURL 映射。 */
  useEffect(() => {
    if (!selectedModId) { setItemIconMap({}); return; }
    let cancelled = false;
    setItemIconMap({});
    setIconsLoading(true);
    setIconWarnings([]);
    resolveModContentIcons(id, selectedModId).then(res => {
      if (cancelled) return;
      const map: Record<string, string> = {};
      for (const item of res.items) {
        if (!item.payload) continue;
        try {
          const payload = typeof item.payload === 'string' ? JSON.parse(item.payload) : item.payload;
          if (payload?.data) {
            map[item.key] = `data:${payload.mime || 'image/png'};base64,${payload.data}`;
          }
        } catch { /* skip invalid */ }
      }
      for (const [tag, representative] of Object.entries(res.tagIcons)) {
        if (map[representative]) map[tag] = map[representative];
      }
      setItemIconMap(map);
      setIconWarnings(res.warnings);
    }).catch(() => {
      if (!cancelled) setIconWarnings(['图标加载失败，请重试。']);
    }).finally(() => { if (!cancelled) setIconsLoading(false); });
    return () => { cancelled = true; };
  }, [id, selectedModId, run?.parsedAt, iconRefresh]);

  /* 根据物品 ID 查找物品栏图标。 */
  const getItemIcon = useCallback((itemId: string): string | null => {
    if (!itemId || !itemIconMap) return null;
    if (itemId.startsWith('#')) {
      const tag = catalogTags[itemId.slice(1)];
      for (const member of tag?.members || []) {
        if (itemIconMap[member]) return itemIconMap[member];
        if (catalogItems[member]?.iconStatus === 'ready') return `/api/packs/${encodeURIComponent(id)}/catalog/icon?itemId=${encodeURIComponent(member)}`;
      }
    }
    if (itemIconMap[itemId]) return itemIconMap[itemId];
    if (catalogItems[itemId]?.iconStatus === 'ready') return `/api/packs/${encodeURIComponent(id)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;
    return null;
  }, [id, itemIconMap, catalogItems, catalogTags]);

  /* 触发解析。 */
  const handleParse = useCallback(async () => {
    if (!selectedModId) return;
    setParsing(true);
    setError('');
    try {
      await parseModContent(id, selectedModId, true);
      /* 异步任务,轮询 run 状态直到 succeeded/failed。 */
      const poll = async (attempt: number): Promise<void> => {
        if (attempt > 60) return; /* 最多轮询约 2 分钟 */
        await new Promise(r => setTimeout(r, 2000));
        const r = await getModContentRun(id, selectedModId).catch(() => null);
        setRun(r);
        if (r && (r.status === 'succeeded' || r.status === 'failed')) {
          await loadContent();
          return;
        }
        return poll(attempt + 1);
      };
      await poll(0);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        /* 同一 sha1 已解析,直接刷新。 */
        await loadContent();
      } else {
        setError(String(e));
      }
    } finally {
      setParsing(false);
    }
  }, [id, selectedModId, loadContent]);

  const selectedMod = useMemo(() => mods.find(m => m.id === selectedModId), [mods, selectedModId]);

  const runStatusTag = (r: ModContentRun | null) => {
    if (!r) return <Tag color="default">未解析</Tag>;
    if (r.status === 'succeeded') return <Tag color="success">解析完成</Tag>;
    if (r.status === 'running') return <Tag color="processing">解析中</Tag>;
    if (r.status === 'failed') return <Tag color="error">解析失败</Tag>;
    return <Tag>{r.status}</Tag>;
  };

  const handleReloadResources = useCallback(async () => {
    setIconRefresh(v => v + 1);
    setCatalogRebuilding(true);
    try {
      await rebuildItemCatalog(id, 'zh_cn');
      for (let attempt = 0; attempt < 60; attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 1000));
        const status = await getCatalogStatus(id);
        if (status.status === 'failed') throw new Error(status.lastError || '目录重建失败');
        if (status.status === 'succeeded' && !status.stale) break;
      }
      setCatalogRefresh(v => v + 1);
    } catch (e) {
      setIconWarnings([`内容目录重建提交失败：${String(e)}`]);
    } finally {
      setCatalogRebuilding(false);
    }
  }, [id]);

  const selectedRelationTag = relationKey.startsWith('#') ? catalogTags[relationKey.slice(1)] : undefined;
  const selectedRelationItem = !relationKey.startsWith('#') ? catalogItems[relationKey] : undefined;

  /* kind=advancement：payload.parent/display/criteria → 进度树（§5bis 分组） */
  const advTrees = useMemo(() => {
    if (activeKind !== 'advancement' || items.length === 0) return [];
    return itemsToAdvTrees(items, {getIcon: getItemIcon, translateKey});
  }, [activeKind, items, getItemIcon, translateKey]);

  const showAdvTree = activeKind === 'advancement' && advViewMode === 'tree' && advTrees.length > 0;

  return (
    <div className="workspace-page mod-content-page">
      <PackContext
        active="内容编辑"
        action={
          <span style={{display: 'inline-flex', gap: 8, alignItems: 'center'}}>
            <Button
              icon={<ReloadOutlined/>}
              loading={iconsLoading || catalogRebuilding}
              onClick={() => void handleReloadResources()}
            >
              重建目录与图标
            </Button>
            <WorkbenchButton
              tone="primary"
              icon={<PlayCircleOutlined/>}
              loading={parsing}
              disabled={!selectedModId}
              onClick={handleParse}
            >
              {run ? '重新解析' : '开始解析'}
            </WorkbenchButton>
          </span>
        }
      />
      <div className="page-heading compact">
        <div>
          <span className="eyebrow">MOD CONTENT / EXTRACTION</span>
          <h1>内容编辑</h1>
          <p>从模组 jar 提取配方、物品、结构与语言内容。选模组 → 解析 → 按类型浏览。</p>
        </div>
      </div>

      <WorkbenchCard className="mod-toolbar-card">
        <div className="mod-toolbar">
          <div className="mod-toolbar-field">
            <span className="field-label">模组</span>
            <Select
              value={selectedModId || undefined}
              onChange={setSelectedModId}
              loading={modsLoading}
              placeholder="选择一个模组"
              style={{minWidth: 280, flex: 1}}
              options={mods.map(m => ({
                label: m.origin === 'builtin' ? `Minecraft [原版] (${m.status})` : `${m.displayName} [${m.source}] (${m.status})`,
                value: m.id,
              }))}
            />
          </div>
          {selectedModId && (
            <div className="mod-metrics">
              <div><span>解析状态</span><strong>{runStatusTag(run)}</strong></div>
              <div><span>已解析</span><strong className="tabular">{run?.parsedCount ?? 0}</strong></div>
              <div><span>动态配方</span><strong className="tabular">{run?.dynamicCount ?? 0}</strong></div>
              <div><span>解析错误</span><strong className="tabular">{run?.errorCount ?? 0}</strong></div>
              <div><span>总文件</span><strong className="tabular">{run?.totalFiles ?? 0}</strong></div>
            </div>
          )}
        </div>
        {selectedModId && (iconsLoading || iconWarnings.length > 0) && (
          <div className="toolbar-note" role="status">
            {iconsLoading ? '正在加载物品图标（首次需准备原版资源）…' : iconWarnings.join(' ')}
          </div>
        )}
        {run?.errorMessage && <div className="parse-error">错误: {run.errorMessage}</div>}
        {!selectedModId && mods.length === 0 && !modsLoading && (
          <Empty description="该整合包还没有添加模组"/>
        )}
      </WorkbenchCard>

      {selectedModId && (
        <>
          <div className="mod-content-tabs" role="tablist" aria-label="内容类型">
            {KIND_TABS.map(tab => (
              <button
                key={tab.kind || 'all'}
                type="button"
                role="tab"
                aria-selected={activeKind === tab.kind}
                className={`mod-content-tab ${activeKind === tab.kind ? 'active' : ''}`}
                onClick={() => setActiveKind(tab.kind)}
              >
                <span className="tab-icon">{tab.icon}</span>
                <span className="tab-label">{tab.label}</span>
              </button>
            ))}
          </div>

          <WorkbenchCard className="mod-content-list">
            <WorkbenchSectionHeader
              title={`${activeKind ? kindLabel(activeKind) : '全部内容'} (${total})`}
              action={<span style={{display: 'inline-flex', gap: 8, alignItems: 'center'}}>
                {selectedMod ? <span className="mod-name">{selectedMod.displayName}</span> : null}
                {activeKind === 'advancement' && (
                  <span className="adv-view-toggle" role="group" aria-label="进度视图">
                    <button
                      type="button"
                      className={advViewMode === 'tree' ? 'active' : ''}
                      onClick={() => setAdvViewMode('tree')}
                    >
                      树
                    </button>
                    <button
                      type="button"
                      className={advViewMode === 'table' ? 'active' : ''}
                      onClick={() => setAdvViewMode('table')}
                    >
                      表格
                    </button>
                  </span>
                )}
                <Button size="small" icon={<ReloadOutlined/>} onClick={loadContent} disabled={loading}>刷新</Button>
              </span>}
            />
            {loading && <div className="list-loading"><Spin/></div>}
            {error && <div className="list-error">{error}</div>}
            {!loading && !error && items.length === 0 && (
              <Empty
                description={run ? '该分类下没有内容。切换其他类型或重新解析。' : '尚未解析。点击右上角「开始解析」提取模组内容。'}
              />
            )}
            {!loading && !error && showAdvTree && (
              <div className="adv-tree-wrap">
                <AdvancementTreeView trees={advTrees} getIcon={getItemIcon}/>
              </div>
            )}
            {!loading && !error && items.length > 0 && !showAdvTree && (
              <table className="mod-content-table">
                <thead>
                  <tr>
                    <th style={{width: 120}}>类型</th>
                    <th>名称 / 键 (Key)</th>
                    <th>路径</th>
                    <th style={{width: 80}}>动态</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map(item => (
                    <tr key={item.id} className="content-row" onClick={() => setDetailItem(item)}>
                      <td><Tag color="blue">{kindLabel(item.kind)}</Tag></td>
                      <td className="content-key">
                        {item.key ? (() => {
                          const recipeDisp = getRecipeDisplay(item);
                          if (recipeDisp) {
                            return (
                              <>
                                <div className="key-display">{recipeDisp.name}</div>
                                <div className="key-techid">{item.key} → {recipeDisp.techId}</div>
                              </>
                            );
                          }
                          const display = translateKey(item.key);
                          return (
                            <>
                              <div className="key-display">{display}</div>
                              {display !== item.key && <div className="key-techid">{item.key}</div>}
                            </>
                          );
                        })() : '—'}
                      </td>
                      <td className="content-path">{item.path}</td>
                      <td>
                        {(() => {
                          const special = item.kind === 'recipe' && (isSpecialRecipePayload(item.payload) || item.isDynamic);
                          if (special) return <Tag color="orange">特殊/动态</Tag>;
                          if (item.isDynamic) return <Tag color="orange">动态</Tag>;
                          return <span className="muted">—</span>;
                        })()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </WorkbenchCard>
        </>
      )}

      {/* 详情弹窗 */}
      <Modal
        title={detailItem ? `${kindLabel(detailItem.kind)}: ${detailItem.key || detailItem.path}` : ''}
        open={Boolean(detailItem)}
        onCancel={() => setDetailItem(null)}
        footer={null}
        width={720}
      >
        {detailItem && (
          <div className="content-detail">
            <div className="detail-meta">
              <p><strong>ID:</strong> {detailItem.id}</p>
              <p><strong>路径:</strong> <code>{detailItem.path}</code></p>
              <p><strong>动态配方:</strong> {detailItem.isDynamic ? (detailItem.kind === 'recipe' && isSpecialRecipePayload(detailItem.payload) ? '特殊/动态' : '是') : '否'}</p>
              {detailItem.parseError && <p className="parse-error"><strong>解析错误:</strong> {detailItem.parseError}</p>}
            </div>
            {detailItem.kind === 'recipe' && !isAdvancementPayload(detailItem.payload) && (
              <div className="detail-recipe">
                <h4>{isSpecialRecipePayload(detailItem.payload) || detailItem.isDynamic ? '配方说明' : '合成配方 (JEI 视图)'}</h4>
                <RecipeViewer payload={detailItem.payload} translateKey={translateKey} getItemIcon={getItemIcon} onSelect={setRelationKey} itemKind={detailItem.kind}/>
              </div>
            )}
            {(detailItem.kind === 'advancement' || isAdvancementPayload(detailItem.payload)) && (
              <div className="detail-recipe">
                <h4>内容说明</h4>
                <RecipeViewer payload={detailItem.payload} translateKey={translateKey} getItemIcon={getItemIcon} onSelect={setRelationKey} itemKind="advancement"/>
              </div>
            )}
            <div className="detail-payload">
              <h4>Payload (原始 JSON)</h4>
              <pre>{JSON.stringify(detailItem.payload, null, 2)}</pre>
            </div>
          </div>
        )}
      </Modal>
      <Modal title={selectedRelationTag ? `材料标签：#${selectedRelationTag.id}` : selectedRelationItem?.displayName || relationKey} open={Boolean(relationKey)} onCancel={() => setRelationKey('')} footer={null} width={560}>
        {selectedRelationTag && (
          <div>
            <p>{selectedRelationTag.displayName} · {selectedRelationTag.members.length} 个候选物品 · {selectedRelationTag.status}</p>
            {selectedRelationTag.members.map(member => <div key={member} className="recipe-slot filled" style={{display:'flex',marginBottom:8,cursor:'pointer'}} onClick={() => setRelationKey(member)}>{getItemIcon(member) && <img className="recipe-icon" src={getItemIcon(member)!} alt=""/>}<span className="recipe-item-name">{translateKey(member)}<small style={{display:'block'}}>{member}</small></span></div>)}
            {selectedRelationTag.diagnostics.map(message => <p key={message} className="parse-error">{message}</p>)}
          </div>
        )}
        {selectedRelationItem && (
          <div>
            <p><code>{selectedRelationItem.id}</code></p>
            <h4>所属物品标签</h4>
            {selectedRelationItem.tags.length ? selectedRelationItem.tags.map(tag => <Tag key={tag} style={{cursor:'pointer',marginBottom:8}} onClick={() => setRelationKey(`#${tag}`)}>#{tag}</Tag>) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有已解析的标签关联"/>}
          </div>
        )}
        {!selectedRelationTag && !selectedRelationItem && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="目录中暂未找到该条目"/>}
      </Modal>
    </div>
  );
}
