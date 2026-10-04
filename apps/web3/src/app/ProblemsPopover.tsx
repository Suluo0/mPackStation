import {useEffect, useRef, useState} from 'react';
import {addMod, ignoreConflict, listModVersions, resolveConflict, resolvePack, type Conflict} from '../api/mods';
import {usePackSummary} from './PackSummaryContext';

/* 顶栏问题徽标的弹出层：冲突就地处置（VSCode Problems 的对齐物）。
   数据与徽标同源（usePackSummary.pendingConflicts），处置后 refresh 让三处信号同时更新。 */
export function ProblemsPopover({onClose}: {onClose: () => void}) {
  const {packId, pack, pendingConflicts, refresh} = usePackSummary();
  const [busy, setBusy] = useState('');
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      /* 健康分按钮（StatusBar）也挂一个本弹层实例，同样算开关本身。 */
      if (ref.current?.contains(target) || target.closest('.problems-chip, [data-problems-toggle]')) return;
      onClose();
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [onClose]);

  const act = async (id: string, kind: 'resolve' | 'ignore') => {
    if (!packId) return;
    setBusy(id + kind);
    setError(null);
    try {
      if (kind === 'resolve') await resolveConflict(packId, id);
      else await ignoreConflict(packId, id);
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const rows: Conflict[] = pendingConflicts;

  /* 缺前置的一键补装。后端在生成依赖冲突时已经把该装哪个塞进了
     detail.missingProjectID（mods.go 的 conflict 构造处），前端 schema
     也一直接着 —— 之前没人读它。这里默认按 Modrinth 取（依赖解析的
     project id 绝大多数来自 Modrinth；CurseForge id 会查空，落到提示）。 */
  const missingOf = (c: Conflict): string | null => {
    if (c.kind !== 'dependency' || c.severity !== 'error') return null;
    const d = c.detail as Record<string, unknown> | null;
    const pid = d && typeof d.missingProjectID === 'string' ? d.missingProjectID : '';
    return pid || null;
  };
  const addMissing = async (c: Conflict, pid: string) => {
    if (!packId) return;
    setBusy(c.id + 'add');
    setError(null);
    try {
      const vs = await listModVersions(packId, 'modrinth', pid);
      /* 与 SourcesPanel.isCompatible 同口径：MC 版本必须命中；loaders 缺失不卡死。 */
      const compat = vs.find(v => {
        const gv = v.gameVersions ?? [];
        const ld = (v.loaders ?? []).map(l => l.toLowerCase());
        const gvOk = gv.length === 0 || gv.includes(pack?.mcVersion ?? '');
        const ldOk = ld.length === 0 || ld.includes((pack?.loader ?? '').toLowerCase());
        return gvOk && ldOk;
      });
      if (!compat) {
        setError(`在 Modrinth 上没找到 ${pid} 匹配本包（${pack?.mcVersion ?? '?'}）的版本，请到商店手动搜索安装。`);
        return;
      }
      await addMod(packId, {provider: 'modrinth', projectId: pid, versionId: compat.id, required: true});
      /* 装完顺手重解析，缺前置的冲突随新依赖图自动结案。 */
      await resolvePack(packId);
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  return (
    <div className="ps-menu problems-menu" ref={ref}>
      {error && <div className="p-empty">{error}</div>}
      {rows.length === 0 && !error && <div className="ps-empty">没有待解决冲突。</div>}
      {rows.map(c => (
        <div key={c.id} className="p-row" style={{alignItems: 'flex-start'}}>
          <span className="dot" style={{background: c.severity === 'error' ? 'var(--mc-fail)' : 'var(--mc-orange)', marginTop: 5}}/>
          <span className="stack grow" style={{whiteSpace: 'normal'}}>
            <span style={{whiteSpace: 'normal', wordBreak: 'break-all'}}>{c.summary}</span>
            <span className="sub" style={{whiteSpace: 'normal'}}>{c.severity} · {c.kind}</span>
          </span>
          <span style={{display: 'flex', gap: 3, flex: 'none'}}>
            {missingOf(c) && (
              <button type="button" className="p-btn" title={`自动安装 ${missingOf(c)} 并重新解析依赖`}
                disabled={busy !== ''}
                onClick={() => void addMissing(c, missingOf(c)!)}>装前置</button>
            )}
            <button type="button" className="p-btn" disabled={busy !== ''} onClick={() => void act(c.id, 'resolve')}>处置</button>
            <button type="button" className="p-btn" disabled={busy !== ''} onClick={() => void act(c.id, 'ignore')}>忽略</button>
          </span>
        </div>
      ))}
      {rows.length > 0 && (
        <div className="ps-filter">处置 / 忽略后徽标与健康分即时更新；构建闸门看 error 级。</div>
      )}
    </div>
  );
}
