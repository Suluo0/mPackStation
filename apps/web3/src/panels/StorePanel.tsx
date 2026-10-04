import {useCallback, useEffect, useRef, useState} from 'react';
import {
  addMod, listModVersions, searchAllMods, otherLoadersText, providerLabel,
  type ModVersion, type SearchAllItem, type CompatRecommendation,
} from '../api/mods';
import {loaderLabel} from '../api/packs';
import {useUrlState} from '../app/url';
import {usePackSummaryOptional} from '../app/PackSummaryContext';
import {SearchInput} from '../ui/SearchInput';
import {Spinner} from '../ui/Spinner';

/* 商店面板（第八轮用户需求）：模组搜索 + 兼容推荐 + 本地 zip 上传。
   与来源面板分离：来源 = 管理已有模组，商店 = 发现并添加新模组。
   包摘要用可选版本：加装成功后要刷新摘要，但拿不到摘要不该让整个面板崩掉。 */
export function StorePanel() {
  const {packId} = useUrlState();
  const summary = usePackSummaryOptional();
  const refresh = summary?.refresh ?? (() => undefined);
  const pack = summary?.pack ?? null;
  const [input, setInput] = useState('');
  const [hits, setHits] = useState<SearchAllItem[] | null>(null);
  /* 降级结果（后端 ModSearchAllResult.fallback）：按本包加载器一个都没搜到时，
     后端摘掉加载器限制重搜回来的「只支持其他加载器」的模组。只作说明，不可加装。 */
  const [altHits, setAltHits] = useState<SearchAllItem[]>([]);
  const [picked, setPicked] = useState<SearchAllItem | null>(null);
  const [versions, setVersions] = useState<ModVersion[]>([]);
  const [recs, setRecs] = useState<CompatRecommendation[]>([]);
  const [error, setError] = useState<string | null>(null);
  /* 加装中：结果行的「添加」要能看出自己在忙，否则点了没反应像坏了。 */
  const [adding, setAdding] = useState<string | null>(null);
  /* 搜索中：搜索是跨网络的（Modrinth + CurseForge 扇出），慢的时候能到几秒。
     这段时间输入框与结果区都得有反馈，否则看起来像输入没生效。 */
  const [searching, setSearching] = useState(false);
  const searchSeq = useRef(0);

  const search = useCallback(async () => {
    if (!packId || !input.trim() || input.trim().length < 2) return;
    const pid = packId;
    const seq = ++searchSeq.current;
    setError(null); setPicked(null); setHits(null); setAltHits([]); setSearching(true);
    try {
      const r = await searchAllMods(pid, {q: input.trim(), limit: 25});
      /* 过期响应丢弃：输入框 300ms 防抖会连发多次，先到的慢响应不该覆盖后到的。 */
      if (seq !== searchSeq.current) return;
      setHits(r.items);
      setAltHits(r.fallback ?? []);
    } catch (e) {
      if (seq === searchSeq.current) { setHits([]); setError(e instanceof Error ? e.message : String(e)); }
    } finally {
      /* 过期响应不许关掉转圈：那次搜索的结果被丢了，但后发的新搜索还在飞。 */
      if (seq === searchSeq.current) setSearching(false);
    }
  }, [packId, input]);

  useEffect(() => {
    if (!packId || input.trim().length < 2) return;
    const t = window.setTimeout(() => void search(), 300);
    return () => window.clearTimeout(t);
  }, [input, search, packId]);

  useEffect(() => {
    if (!packId) return;
    import('../api/mods').then(m => m.listModRecommendations(packId).then(setRecs).catch(() => setRecs([])));
  }, [packId]);

  const pick = async (h: SearchAllItem) => {
    setPicked(h); setVersions([]);
    try { setVersions(await listModVersions(packId!, h.provider, h.id)); }
    catch (e) { setError(e instanceof Error ? e.message : String(e)); }
  };

  const add = async (v: ModVersion) => {
    if (!packId || !picked) return;
    await addMod(packId!, {provider: picked.provider, projectId: picked.id, versionId: v.id, required: true});
    setPicked(null); setVersions([]); setHits(null); setAltHits([]); setInput('');
    refresh();
  };

  /* 一键加装：取第一个「本包装得上」的版本直接装，省掉先展开版本列表那一步。
     兼容性判定与 SourcesPanel.isCompatible 同口径：MC 版本必须命中，
     loaders 缺失不按加载器卡死（部分 CurseForge 条目不回报加载器）。
     一个兼容版本都没有时不硬装 —— 装上去也是废的，让用户进版本列表自己看。 */
  const quickAdd = async (h: SearchAllItem) => {
    if (!packId) return;
    setAdding(h.id);
    setError(null);
    try {
      const vs = await listModVersions(packId, h.provider, h.id);
      const v = vs.find(x => {
        const gv = x.gameVersions ?? [];
        if (pack && gv.length > 0 && !gv.includes(pack.mcVersion)) return false;
        const ld = (x.loaders ?? []).map(l => l.toLowerCase());
        return ld.length === 0 || !pack || ld.includes(pack.loader.toLowerCase());
      });
      if (!v) { setError(`「${h.name}」没有可用于本包（${packLabel}）的版本`); return; }
      await addMod(packId, {provider: h.provider, projectId: h.id, versionId: v.id, required: true});
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setAdding(null);
    }
  };

  if (!packId) return null;

  /* 本包口径，用于说明「为什么这些装不上」。拿不到包摘要时退化成「当前整合包」，
     文案仍然成立 —— 降级区本身的存在就是解释。 */
  const packLabel = pack ? `${pack.mcVersion} · ${loaderLabel(pack.loader)}` : '当前整合包';

  return (
    <div className="tp-body">
      <div className="p-section">
        <div className="p-title">模组搜索</div>
        {/* 搜索框走 ui/SearchInput：提交键是放大镜图标，不是「搜」字。 */}
        <SearchInput value={input} onChange={setInput} onSubmit={() => void search()} autoFocus
          busy={searching} placeholder="模组名 / 缩写 / slug…"/>
        {error && <div className="p-empty">{error}</div>}
        {searching && <div className="p-empty"><Spinner label="搜索中" style={{marginRight: 4}}/>搜索中…</div>}
        {hits?.length === 0 && altHits.length === 0 && !picked && (
          <div className="p-empty">没有命中。设置页确认 CurseForge Key 后再试。</div>
        )}
        {/* 行上有两层动作：单击 = 展开版本列表（要看清装的是哪个版本），
            「添加」/ 双击 = 按本包可用的版本直接装。
            以前只有单击一种，用户双击看到没反应，以为坏了。 */}
        {!picked && (hits ?? []).map(h => (
          <div key={`${h.provider}-${h.id}`} className="p-row click" onClick={() => void pick(h)}
            onDoubleClick={() => void quickAdd(h)}
            title={`${h.summary ?? ''}\n双击 = 直接装上本包可用的版本`}>
            <span className="grow">{h.name}</span>
            <span className="sub">{providerLabel(h.provider)}{h.mirror ? ' · 镜像' : ''}</span>
            <button type="button" className="p-btn" disabled={adding !== null}
              onClick={e => { e.stopPropagation(); void quickAdd(h); }}>
              {adding === h.id ? '装中…' : '添加'}
            </button>
          </div>
        ))}

        {/* 降级结果：同 MC 版本、但只支持其他加载器。这些**装不进**当前包，所以
            刻意不可点击 —— 让用户点到一半再失败，比一开始就说清楚更糟。
            文案不提「没有命中」：主结果里可能有几条平台的模糊命中（搜 mek 会返回
            NoEmotecraft 这种摘要沾边的），说「没有命中」与眼睛看到的相矛盾。 */}
        {!picked && altHits.length > 0 && (
          <>
            <div className="p-empty">
              本包（{packLabel}）没有真正匹配的模组。以下同 MC 版本、但只支持其他加载器的，装不进本包：
            </div>
            {altHits.map(h => (
              <div key={`alt-${h.provider}-${h.id}`} className="p-row na" title={h.summary ?? ''}>
                <span className="grow">{h.name}</span>
                <span className="sub">仅支持 {otherLoadersText(h.loaders, pack?.loader ?? '') || '其他加载器'}</span>
              </div>
            ))}
          </>
        )}
        {picked && (
          <div className="p-section">
            <div className="p-row">
              <span className="grow" style={{fontWeight: 600}}>{picked.name}</span>
              <button type="button" className="p-btn" onClick={() => setPicked(null)}>返回</button>
            </div>
            <div className="p-empty">选一个兼容版本（添加即钉版）：</div>
            {versions.map(v => (
              <div key={v.id} className="p-row click" onClick={() => void add(v).catch(e => setError(String(e)))}>
                <span className="grow">{v.versionNumber || v.name || v.id}</span>
                {(v.gameVersions ?? []).slice(0, 1).map(g => <span key={g} className="sub">{g}</span>)}
              </div>
            ))}
            {versions.length === 0 && <div className="p-empty">版本载入中…</div>}
          </div>
        )}
      </div>

      {recs.length > 0 && (
        <div className="p-section">
          <div className="p-title">兼容推荐 <span className="count">{recs.length}</span></div>
          {recs.map(r => (
            <div key={r.projectId} className="p-row" title={r.reason}>
              <span className="grow">{r.name}</span>
              <span className="sub">{providerLabel(r.provider)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
