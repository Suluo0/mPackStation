import {useState} from 'react';
import {Drawer, Input} from 'antd';
import {useParams} from 'react-router-dom';
import {searchAllMods, type SearchAllItem} from '../api/mods';

/* 素面叶子：添加模组抽屉（?rail=mods 时打开，§6.4）。
   第三步换成 ModSearchDrawer：双平台并发 + 镜像配对 + 选版本 + 真加模组。
   这里只做一次真实搜索并列出命中，不提供没有版本选择的「添加」按钮。 */
export function StubModSearch({open, onClose}: {open: boolean; onClose: () => void}) {
  const {id} = useParams();
  const [q, setQ] = useState('');
  const [hits, setHits] = useState<SearchAllItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    if (!id || !q.trim()) return;
    setError(null);
    try {
      const r = await searchAllMods(id, {q: q.trim(), limit: 8});
      setHits(r.items);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setHits([]);
    }
  };

  return (
    <Drawer title="添加模组" width={520} open={open} onClose={onClose}>
      <Input value={q} onChange={e => setQ(e.target.value)} onPressEnter={() => void run()} placeholder="搜索模组名…"/>
      <button type="button" className="tb-btn" onClick={() => void run()}>搜索</button>
      {error && <div className="rr-line">{error}</div>}
      {hits?.length === 0 && <div className="rr-line">没有命中。换个关键词，或在设置页确认 CurseForge Key 已配置。</div>}
      {(hits ?? []).map(h => (
        <div key={`${h.provider}-${h.id}`} className="rr-line">
          {h.name} · {h.provider} · {h.downloads ?? 0} 次下载
        </div>
      ))}
    </Drawer>
  );
}
