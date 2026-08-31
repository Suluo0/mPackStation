import {useEffect, useRef, useState} from 'react';
import {App} from 'antd';
import {
  addMod, listMods, listModVersions, removeMod, searchAllMods, updateMod,
  type Mod, type ModVersion, type SearchAllItem,
} from '../api/mods';
import type {Pack} from '../api/packs';

/* 模组目录场景:双平台搜索 → 选版本 → 添加,以及已装列表的启用/移除。
   版本按当前包的 mcVersion/loader 标记兼容性;单平台失败只提示不阻塞。 */
export function useModSearch(packId: string | undefined, pack: Pack | null) {
  const {message} = App.useApp();
  const [installed, setInstalled] = useState<Mod[]>([]);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchAllItem[]>([]);
  const [searchErrors, setSearchErrors] = useState<Record<string, string>>({});
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState(false);
  const [versions, setVersions] = useState<Record<string, ModVersion[]>>({});
  const [choice, setChoice] = useState<Record<string, string>>({});
  const [error, setError] = useState('');

  const refresh = () => {
    if (packId) void listMods(packId).then(setInstalled).catch(e => setError(e instanceof Error ? e.message : String(e)));
  };
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  useEffect(() => { if (packId) refreshRef.current(); }, [packId]);

  const compatible = (v: ModVersion) => {
    if (!pack) return true;
    const mcOk = !v.gameVersions?.length || v.gameVersions.includes(pack.mcVersion);
    const ldOk = !v.loaders?.length || v.loaders.some(l => l.toLowerCase() === pack.loader.toLowerCase());
    return mcOk && ldOk;
  };

  const keyOf = (p: SearchAllItem) => `${p.provider}:${p.id}`;

  const runSearch = () => {
    if (!packId || !query.trim()) return;
    setSearching(true); setError('');
    searchAllMods(packId, {q: query.trim(), limit: 20, mcVersion: pack?.mcVersion, loader: pack?.loader})
      .then(v => { setResults(v.items); setSearchErrors(v.errors ?? {}); setSearched(true); setVersions({}); setChoice({}); })
      .catch(e => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setSearching(false));
  };

  const loadVersions = (p: SearchAllItem) => {
    if (!packId || versions[keyOf(p)]) return;
    listModVersions(packId, p.provider, p.id).then(vs => {
      // 后端已按日期新→旧;这里稳定排序把兼容当前包的版本提到最前,
      // 默认选中即"最新兼容版",不兼容的沉底但仍可见。
      const sorted = [...vs].sort((a, b) => Number(compatible(b)) - Number(compatible(a)));
      setVersions(prev => ({...prev, [keyOf(p)]: sorted}));
      const first = sorted[0];
      if (first) setChoice(prev => ({...prev, [keyOf(p)]: first.id}));
    }).catch(e => message.error(e instanceof Error ? e.message : String(e)));
  };

  const add = (p: SearchAllItem) => {
    if (!packId) return;
    const versionId = choice[keyOf(p)];
    if (!versionId) { message.error('请先选择版本'); return; }
    addMod(packId, {provider: p.provider, projectId: p.id, versionId, required: true})
      .then(() => { message.success('已添加到整合包'); refresh(); })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)));
  };

  const toggleInstalled = (m: Mod) => {
    if (!packId) return;
    const op = m.status === 'disabled'
      ? updateMod(packId, m.id, {status: 'enabled'})
      : removeMod(packId, m.id);
    op.then(() => { message.success(m.status === 'disabled' ? '已启用' : '已移除'); refresh(); })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)));
  };

  return {
    installed, query, setQuery, results, searchErrors, searching, searched,
    versions, choice, setChoice, error,
    compatible, keyOf, runSearch, loadVersions, add, toggleInstalled,
  };
}
