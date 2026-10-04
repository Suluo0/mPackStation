import {useEffect, useRef, useState} from 'react';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';
import {loaderLabel} from '../api/packs';
import {useUrlPatch, useUrlState} from '../app/url';
import {usePackSummary} from '../app/PackSummaryContext';
import {Icon} from '../ui/Icon';

/* 左上角包名菜单（现代 IDEA 的项目名位置，取代品牌字样 + 右上角⚙）：
   包列表（健康点）+ 只看待处理 + 新建/导入 + 编辑器设置。
   按钮文案 = 包名 · MC 版本 · 加载器（如「验证包-0025 · 1.21.1 · Fabric」）。 */
export function PackMenu() {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const {pack: summaryPack} = usePackSummary();
  const [open, setOpen] = useState(false);
  const [packs, setPacks] = useState<DashboardPack[] | null>(null);
  const [onlyPending, setOnlyPending] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open || packs) return;
    void fetchDashboard().then(d => setPacks(d.packs)).catch(() => setPacks([]));
  }, [open, packs]);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  const current = summaryPack ?? packs?.find(p => p.id === packId) ?? null;
  const pendingOf = (p: DashboardPack) => p.conflicts.pending + p.alerts.crashes + p.alerts.updatable;
  const rows = (packs ?? []).filter(p => !onlyPending || pendingOf(p) > 0);

  return (
    <div ref={ref} style={{position: 'relative'}}>
      <button type="button" className="pack-switcher" onClick={() => setOpen(v => !v)}
        title={current
          ? `${current.name} · ${current.mcVersion} · ${loaderLabel(current.loader)}`
          : '选择整合包'}>
        <span className="ps-name">{current ? current.name : '选择整合包'}</span>
        {current && <span className="ps-sep">·</span>}
        {current && <span className="ps-sub">{current.mcVersion}</span>}
        {current && <span className="ps-sep">·</span>}
        {current && <span className="ps-sub">{loaderLabel(current.loader)}</span>}
        <Icon name="caretDown" size={14}/>
      </button>
      {open && (
        <div className="ps-menu">
          {packs === null && <div className="ps-empty">载入中…</div>}
          {packs !== null && rows.length === 0 && (
            <div className="ps-empty">{onlyPending ? '没有待处理的包。' : '还没有整合包。'}</div>
          )}
          {rows.map(p => (
            <div key={p.id} className="ps-item"
              style={p.id === packId ? {background: 'var(--mc-primary-bg)'} : undefined}
              onClick={() => { patch({pack: p.id, mode: null, tool: null, type: null, doc: null, node: null, ns: null}); setOpen(false); }}>
              <span className="dot" style={{background: pendingOf(p) > 0 ? 'var(--mc-orange)' : 'var(--mc-success)'}}/>
              <span>{p.name}</span>
              <span className="meta">MC {p.mcVersion} · {p.modCount.total} 模组{pendingOf(p) > 0 ? ` · 待处理 ${pendingOf(p)}` : ''}</span>
            </div>
          ))}
          {packs !== null && packs.length > 0 && (
            <label className="ps-filter">
              <input type="checkbox" checked={onlyPending} onChange={e => setOnlyPending(e.target.checked)}/>
              只看待处理（{packs.filter(pendingOf).length}/{packs.length}）
            </label>
          )}
          <div className="ps-filter" style={{gap: 4}}>
            <button type="button" className="p-btn" onClick={() => { patch({pack: null, mode: null, tool: null, type: null, doc: null, node: null, ns: null}); setOpen(false); }}>
              新建 / 导入整合包
            </button>
            <button type="button" className="p-btn" onClick={() => { patch({settings: '1'}); setOpen(false); }}>
              编辑器设置…
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
