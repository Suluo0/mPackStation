import {useEffect, useState} from 'react';
import {createVersion, listArtifacts, listVersions, type Artifact, type PackVersion} from '../api/releases';
import {usePackSummary} from '../app/PackSummaryContext';

/* 素面叶子：交付②构建。第三步换成真实构建段（导出目录选择 + 快照 + 内核构建）。
   产物下载走 <a href download>：该端点免令牌且返回二进制，不能用 api/http.ts 的 JSON 封装。 */
export function StubBuild({packId}: {packId: string}) {
  const [versions, setVersions] = useState<PackVersion[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [version, setVersion] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {refresh, pendingConflicts} = usePackSummary();

  /* 闸门口径见 §6.5：数据源是 usePackSummary().pendingConflicts。
     注意这比后端 assertPackBuildable 严（后端只拦 error 级、只拦 /build），
     取严的结果是「多点一下处理冲突」，不会反过来放行了崩票。 */
  const load = () => {
    listVersions(packId).then(setVersions).catch(e => setError(String(e)));
    listArtifacts(packId).then(setArtifacts).catch(e => setError(String(e)));
  };
  useEffect(load, [packId]);

  const addVersion = async () => {
    if (!version.trim() || pendingConflicts.length > 0) return;
    await createVersion(packId, {version: version.trim(), channel: 'release'});
    setVersion('');
    load();
    refresh();
  };

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      <input className="tb-btn" value={version} onChange={e => setVersion(e.target.value)} placeholder="新版本号，如 0.4.0"/>
      <button type="button" className="tb-btn" disabled={pendingConflicts.length > 0}
        onClick={() => void addVersion().catch(e => setError(String(e)))}>注册版本</button>
      {pendingConflicts.length > 0 && <div className="rr-line">待处理冲突 {pendingConflicts.length} 条，构建闸门已禁用本段动作。</div>}
      <div className="rr-line">版本 {versions.length} · 产物 {artifacts.length}</div>
      {versions.map(v => <div key={v.id} className="rr-line">{v.version} · {v.channel} · {v.createdAt}</div>)}
      {artifacts.map(a => (
        <div key={a.id} className="rr-line">
          {a.fileName} · {a.status} · {a.sizeBytes} B
          <a className="tb-btn" href={`/api/packs/${encodeURIComponent(packId)}/artifacts/${encodeURIComponent(a.id)}/download`} download>下载</a>
        </div>
      ))}
    </div>
  );
}
