import {useCallback, useEffect, useState} from 'react';
import {createVersion, listArtifacts, listDeliveryChecks, listReleases, listVersions, type Artifact, type DeliveryCheck, type PackVersion, type Release} from '../api/releases';
import {usePackSummary} from '../app/PackSummaryContext';
import {useUrlState} from '../app/url';

/* 构建面板（Git 分支语义）：版本登记 + 交付检查 + 产物清单（下载）+ 发布记录（只读，表单 3F 落地）。
   闸门口径：有 error 级待解决冲突时禁用登记动作，比后端 assertPackBuildable 取严。 */
export function BuildPanel() {
  const {packId} = useUrlState();
  const {refresh, pendingConflicts, health} = usePackSummary();
  const [versions, setVersions] = useState<PackVersion[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [releases, setReleases] = useState<Release[]>([]);
  const [checks, setChecks] = useState<DeliveryCheck[] | null>(null);
  const [version, setVersion] = useState('');
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!packId) return;
    listVersions(packId).then(setVersions).catch(e => setError(String(e)));
    listArtifacts(packId).then(setArtifacts).catch(e => setError(String(e)));
    listReleases(packId).then(setReleases).catch(() => setReleases([]));
    listDeliveryChecks(packId).then(r => setChecks(r.items)).catch(() => setChecks([]));
  }, [packId]);
  useEffect(load, [load]);

  if (!packId) return null;
  const errorConflicts = pendingConflicts.filter(c => c.severity === 'error').length;
  const gated = errorConflicts > 0;

  const addVersion = async () => {
    if (!version.trim() || gated) return;
    await createVersion(packId, {version: version.trim(), channel: 'release'});
    setVersion('');
    load();
    refresh();
  };

  return (
    <>
      <div className="tp-head"><span>构建</span></div>
      <div className="tp-body">
      <div className="p-section">
        <div className="p-title">版本 <span className="count">{versions.length}</span></div>
        {error && <div className="p-empty">{error}</div>}
        <div style={{display: 'flex', gap: 4}}>
          <input className="p-input" style={{flex: 1, minWidth: 0}} value={version} placeholder="新版本号，如 0.4.0"
            onChange={e => setVersion(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') void addVersion().catch(e => setError(String(e))); }}/>
          <button type="button" className="p-btn primary" disabled={gated || !version.trim()}
            onClick={() => void addVersion().catch(e => setError(String(e)))}>登记</button>
        </div>
        {gated && <div className="p-empty">有 {errorConflicts} 条 error 级冲突未解决，构建闸门已禁用本段动作。</div>}
        {versions.map(v => (
          <div key={v.id} className="p-row">
            <span className="grow">{v.version}</span>
            <span className="sub">{v.channel}</span>
          </div>
        ))}
        {versions.length === 0 && !error && <div className="p-empty">还没有版本。登记一个版本后才会跑交付检查。</div>}
      </div>

      <div className="p-section">
        <div className="p-title">产物 <span className="count">{artifacts.length}</span></div>
        {artifacts.map(a => (
          <div key={a.id} className="p-row">
            <span className="dot" style={{background: a.status === 'ready' ? 'var(--mc-success)' : a.status === 'failed' ? 'var(--mc-fail)' : 'var(--mc-muted)'}}/>
            <span className="grow" title={a.fileName}>{a.fileName}</span>
            <span className="sub">{Math.max(1, Math.round(a.sizeBytes / 1024))} KB</span>
            {a.status === 'ready' && (
              <a className="p-btn" href={`/api/packs/${encodeURIComponent(packId)}/artifacts/${encodeURIComponent(a.id)}/download`} download>下载</a>
            )}
          </div>
        ))}
        {artifacts.length === 0 && <div className="p-empty">还没有产物。构建（3F 落地：导出目录 + 内核装配）后出现在这里。</div>}
      </div>

      <div className="p-section">
        <div className="p-title">交付检查 <span className="count">{checks?.length ?? 0}</span></div>
        {(checks ?? []).map((c, i) => (
          <div key={`${c.kind}-${i}`} className="p-row">
            <span className="dot" style={{background: c.status === 'pass' ? 'var(--mc-success)' : c.status === 'fail' ? 'var(--mc-fail)' : 'var(--mc-muted)'}}/>
            <span className="grow">{c.detail || c.kind}</span>
            <span className="sub">{c.status}</span>
          </div>
        ))}
        {checks?.length === 0 && (
          <div className="p-empty">还没有检查记录。登记一个版本后，交付闸门会自动跑检查。</div>
        )}
      </div>

      <div className="p-section">
        <div className="p-title">发布记录 <span className="count">{releases.length}</span></div>
        {releases.map(r => (
          <div key={r.id} className="p-row">
            <span className="grow">{r.provider}</span>
            <span className="sub">{r.status}{r.errorCode ? ` · ${r.errorCode}` : ''}</span>
          </div>
        ))}
        {releases.length === 0 && (
          <div className="p-empty">还没有发布记录。发布到 CurseForge / Modrinth 的表单与轮询由 3F 落地（无凭证时后端按 by-design 拒绝）。</div>
        )}
      </div>

      {health && (
        <div className="p-section">
          <div className="p-title">闸门状态</div>
          <div className="p-row">
            <span className="dot" style={{background: health.healthy ? 'var(--mc-success)' : 'var(--mc-orange)'}}/>
            <span className="grow sub">{health.healthy ? '健康，可以构建' : `待处理：错误 ${health.pendingErrors} · 警告 ${health.pendingWarnings}`}</span>
          </div>
        </div>
      )}
      </div>
    </>
  );
}
