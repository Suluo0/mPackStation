import {useEffect, useState} from 'react';
import {useParams, useSearchParams} from 'react-router-dom';
import {listMods, updateMod, type Mod} from '../api/mods';
import {useCatalog} from '../app/CatalogContext';
import {usePackSummary} from '../app/PackSummaryContext';

const DOT: Record<string, string> = {installed: 'var(--mc-success)', pending: 'var(--mc-orange)', disabled: 'var(--mc-muted)'};

/* 素面叶子：来源栏的模组列表。第三步换成 ModSourceRail（图标 + 抽屉 + 双平台镜像标记）。
   「贡献 N 物品」= 写 ?ns= + ?mode=index，这是旧 /mods 与 /items 两条路由合并后的行为（§6.4）。 */
export function StubModRail() {
  const {id} = useParams();
  const [mods, setMods] = useState<Mod[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [, setParams] = useSearchParams();
  const {namespaces} = useCatalog();
  const {refresh} = usePackSummary();

  const load = () => { if (id) listMods(id).then(setMods).catch(e => setError(e instanceof Error ? e.message : String(e))); };
  useEffect(load, [id]);

  const filterBy = (modId: string) => setParams(p => {
    const n = new URLSearchParams(p);
    n.set('ns', modId);
    n.set('mode', 'index');
    return n;
  }, {replace: true});

  const toggle = async (m: Mod) => {
    if (!id) return;
    await updateMod(id, m.id, {status: m.status === 'disabled' ? 'installed' : 'disabled'});
    load();
    refresh();
  };

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      <div className="rr-line">模组 {mods.length}</div>
      {mods.map(m => (
        <div key={m.id} className="rr-line">
          <span style={{color: DOT[m.status] ?? 'var(--mc-muted)'}}>●</span> {m.displayName}
          <button type="button" className="tb-btn" onClick={() => void toggle(m).catch(() => undefined)}>{m.status === 'disabled' ? '启用' : '停用'}</button>
          <button type="button" className="tb-btn" onClick={() => filterBy(m.canonicalModId)}>贡献 {namespaces.find(x => x.ns === m.canonicalModId)?.count ?? 0} 物品</button>
        </div>
      ))}
    </div>
  );
}
