import {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode} from 'react';
import {
  getCatalogStatus, getItemCatalog, rebuildItemCatalog,
  type CatalogItem, type CatalogRecipe, type CatalogTag, type ItemCatalog,
} from '../../api/catalog';
import {ApiError} from '../../api/http';

/* 包级共享目录 store：整包只加载一次 /catalog，向工作台各页面提供同一份物品/标签/配方索引。
   这是「物品作为通用语」的数据底座 —— 合成器、魔改、任务书、模组页都从这里取名、取图标、取关系，
   不再各自 fetch。phase 状态机与单页版本一致（loading/ready/building/failed + 轮询）。 */

export type CatalogPhase = 'loading' | 'ready' | 'building' | 'failed';

export type PackCatalogValue = {
  phase: CatalogPhase;
  error: string;
  catalog: ItemCatalog | null;
  items: CatalogItem[];
  itemById: Map<string, CatalogItem>;
  tagById: Map<string, CatalogTag>;
  recipesByOutput: Map<string, CatalogRecipe[]>;
  recipesByInput: Map<string, CatalogRecipe[]>;
  displayName: (id: string) => string;
  iconUrl: (itemId: string) => string;
  reload: () => Promise<void>;
  rebuild: () => Promise<void>;
  rebuilding: boolean;
};

const PackCatalogContext = createContext<PackCatalogValue | null>(null);

export function PackCatalogProvider({packId, locale = 'zh_cn', children}: {packId: string; locale?: string; children: ReactNode}) {
  const [catalog, setCatalog] = useState<ItemCatalog | null>(null);
  const [phase, setPhase] = useState<CatalogPhase>('loading');
  const [error, setError] = useState('');
  const [rebuilding, setRebuilding] = useState(false);
  const pollRef = useRef<number | undefined>(undefined);

  const load = useCallback(async () => {
    if (!packId) return;
    setPhase('loading');
    setError('');
    try {
      const c = await getItemCatalog(packId, locale);
      setCatalog(c);
      setPhase('ready');
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        const st = await getCatalogStatus(packId).catch(() => null);
        if (st && (st.status === 'pending' || st.status === 'running')) setPhase('building');
        else if (st?.status === 'failed') { setPhase('failed'); setError(st.lastError || '物品目录构建失败'); }
        else { setPhase('failed'); setError('物品目录尚未构建。'); }
      } else {
        setPhase('failed');
        setError(e instanceof Error ? e.message : String(e));
      }
    }
  }, [packId, locale]);

  useEffect(() => { void load(); }, [load]);

  useEffect(() => {
    if (phase !== 'building') return;
    let cancelled = false;
    pollRef.current = window.setInterval(async () => {
      const st = await getCatalogStatus(packId).catch(() => null);
      if (cancelled || !st) return;
      if (st.status === 'succeeded' && !st.stale) {
        if (pollRef.current) window.clearInterval(pollRef.current);
        await load();
      } else if (st.status === 'failed') {
        if (pollRef.current) window.clearInterval(pollRef.current);
        setPhase('failed');
        setError(st.lastError || '构建失败');
      }
    }, 2000);
    return () => { cancelled = true; if (pollRef.current) window.clearInterval(pollRef.current); };
  }, [phase, packId, load]);

  const rebuild = useCallback(async () => {
    if (!packId) return;
    setRebuilding(true);
    try {
      await rebuildItemCatalog(packId, locale);
      setPhase('building');
    } catch (e) {
      setPhase('failed');
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRebuilding(false);
    }
  }, [packId, locale]);

  const iconUrl = useCallback(
    (itemId: string) => `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`,
    [packId],
  );

  const indexes = useMemo(() => {
    const itemById = new Map<string, CatalogItem>();
    for (const it of catalog?.items ?? []) itemById.set(it.id, it);
    const tagById = new Map<string, CatalogTag>();
    for (const t of catalog?.tags ?? []) tagById.set(t.id, t);
    const recipesByOutput = new Map<string, CatalogRecipe[]>();
    const recipesByInput = new Map<string, CatalogRecipe[]>();
    const push = (m: Map<string, CatalogRecipe[]>, key: string, r: CatalogRecipe) => {
      const arr = m.get(key);
      if (arr) { if (!arr.includes(r)) arr.push(r); } else m.set(key, [r]);
    };
    for (const r of catalog?.recipes ?? []) {
      for (const ref of r.refs) {
        if (ref.kind !== 'item') continue;
        if (ref.role === 'output') push(recipesByOutput, ref.id, r);
        else if (ref.role === 'input') push(recipesByInput, ref.id, r);
      }
    }
    return {itemById, tagById, recipesByOutput, recipesByInput};
  }, [catalog]);

  const displayName = useCallback((id: string): string => {
    if (!id) return id;
    if (id.startsWith('#')) return indexes.tagById.get(id.slice(1))?.displayName || id;
    return indexes.itemById.get(id)?.displayName || id;
  }, [indexes]);

  const value = useMemo<PackCatalogValue>(() => ({
    phase, error, catalog,
    items: catalog?.items ?? [],
    itemById: indexes.itemById,
    tagById: indexes.tagById,
    recipesByOutput: indexes.recipesByOutput,
    recipesByInput: indexes.recipesByInput,
    displayName, iconUrl,
    reload: load, rebuild, rebuilding,
  }), [phase, error, catalog, indexes, displayName, iconUrl, load, rebuild, rebuilding]);

  return <PackCatalogContext.Provider value={value}>{children}</PackCatalogContext.Provider>;
}

export function usePackCatalog(): PackCatalogValue {
  const ctx = useContext(PackCatalogContext);
  if (!ctx) throw new Error('usePackCatalog 必须在 <PackCatalogProvider> 内使用');
  return ctx;
}
