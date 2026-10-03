import {createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode} from 'react';
import {
  getItemCatalog, getCatalogStatus, rebuildItemCatalog,
  type ItemCatalog, type CatalogItem, type CatalogRecipe, type CatalogTag,
} from '../api/catalog';
import {useUrlState} from './url';

export type CatalogState = {
  packId: string | null;
  catalog: ItemCatalog | null;
  status: Awaited<ReturnType<typeof getCatalogStatus>> | null;
  locale: string;
  setLocale: (v: string) => void;
  /* 派生索引：一律在这里算一次，组件不许各自 reduce 全量数组（几千个物品 × 每次渲染 = 卡） */
  itemById: Map<string, CatalogItem>;
  tagById: Map<string, CatalogTag>;
  recipesByOutput: Map<string, CatalogRecipe[]>;   // 物品 id → 它作为产物的配方
  recipesByInput: Map<string, CatalogRecipe[]>;    // 物品 id → 它作为原料的配方
  /* 物品 id → 提到它但解析器没结构化（status!='parsed'）的配方：这些配方零边，
     链在这里断开，界面必须能区分「真没有配方」和「有配方但读不出边」。 */
  unstructuredByMention: Map<string, CatalogRecipe[]>;
  namespaces: {ns: string; count: number}[];       // 来源栏/索引态的命名空间计数
  rebuild: () => Promise<void>;                    // 触发重建；轮询由本 context 负责
  refreshing: boolean;
  error: string | null;
};

const Ctx = createContext<CatalogState | null>(null);

export function useCatalog(): CatalogState {
  const v = useContext(Ctx);
  if (!v) throw new Error('useCatalog 必须在 CatalogProvider 内使用');
  return v;
}

const NS_RE = /^([^:]+):/;
/* 从原始 payload 里抠出 `命名空间:路径` 形式的物品 id（JSON 字符串值）。 */
const ID_IN_JSON_RE = /"[a-z0-9_.-]+:[a-z0-9_.\/-]+"/g;

export function CatalogProvider({children}: {children: ReactNode}) {
  const {packId} = useUrlState();
  const [locale, setLocale] = useState('zh_cn');
  const [catalog, setCatalog] = useState<ItemCatalog | null>(null);
  const [status, setStatus] = useState<CatalogState['status']>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!packId) { setCatalog(null); setStatus(null); return; }
    setRefreshing(true);
    try {
      const [c, s] = await Promise.all([getItemCatalog(packId, locale), getCatalogStatus(packId)]);
      setCatalog(c); setStatus(s); setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRefreshing(false);
    }
  }, [packId, locale]);

  useEffect(() => { void load(); }, [load]);

  /* 重建/解析进行中时轮询 status，落到终态后自动重载目录。
     放在 context 而不是页面：切态切页都不能丢进度（设计文档 §5-6）。 */
  useEffect(() => {
    if (!packId) return;
    const busy = status?.status === 'pending' || status?.status === 'running';
    if (!busy) return;
    const t = window.setInterval(async () => {
      try {
        const s = await getCatalogStatus(packId);
        setStatus(s);
        if (s.status === 'succeeded' || s.status === 'failed') await load();
      } catch { /* 下一轮再试 */ }
    }, 3000);
    return () => window.clearInterval(t);
  }, [packId, status?.status, load]);

  const derived = useMemo(() => {
    const itemById = new Map<string, CatalogItem>();
    const tagById = new Map<string, CatalogTag>();
    const recipesByOutput = new Map<string, CatalogRecipe[]>();
    const recipesByInput = new Map<string, CatalogRecipe[]>();
    const unstructuredByMention = new Map<string, CatalogRecipe[]>();
    const nsCount = new Map<string, number>();
    for (const it of catalog?.items ?? []) {
      itemById.set(it.id, it);
      const ns = NS_RE.exec(it.id)?.[1] ?? '(无命名空间)';
      nsCount.set(ns, (nsCount.get(ns) ?? 0) + 1);
    }
    for (const tg of catalog?.tags ?? []) tagById.set(tg.id, tg);
    for (const rc of catalog?.recipes ?? []) {
      for (const ref of rc.refs) {
        const bucket = ref.role === 'output' ? recipesByOutput : recipesByInput;
        const arr = bucket.get(ref.id);
        if (arr) arr.push(rc); else bucket.set(ref.id, [rc]);
      }
      if (rc.status === 'parsed') continue;
      const seen = new Set<string>();
      for (const m of JSON.stringify(rc.payload).matchAll(ID_IN_JSON_RE)) {
        const id = m[0].slice(1, -1);
        if (seen.has(id)) continue;
        seen.add(id);
        const arr = unstructuredByMention.get(id);
        if (arr) arr.push(rc); else unstructuredByMention.set(id, [rc]);
      }
    }
    const namespaces = [...nsCount.entries()].map(([ns, count]) => ({ns, count})).sort((a, b) => b.count - a.count);
    return {itemById, tagById, recipesByOutput, recipesByInput, unstructuredByMention, namespaces};
  }, [catalog]);

  const value = useMemo<CatalogState>(() => ({
    packId, catalog, status, locale, setLocale, ...derived,
    rebuild: async () => {
      if (!packId) return;
      await rebuildItemCatalog(packId, locale);
      await load();   // 触发一次 status 拉取，让轮询接管
    },
    refreshing, error,
  }), [packId, catalog, status, locale, derived, load, refreshing, error]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
