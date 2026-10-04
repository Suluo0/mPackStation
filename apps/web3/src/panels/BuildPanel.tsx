import {useCallback, useEffect, useState} from 'react';
import {
  buildPack, createVersion, listArtifacts, listDeliveryChecks, listReleases, listVersions, runDeliveryChecks,
  type Artifact, type DeliveryCheck, type PackVersion, type Release,
} from '../api/releases';
import {listExportDirs, browseDirectories, registerExportDir, type ExportDir, type FsBrowse} from '../api/fs';
import {usePackSummary} from '../app/PackSummaryContext';
import {useCatalog} from '../app/CatalogContext';
import {useUrlState} from '../app/url';
import {Icon} from '../ui/Icon';
import {Modal} from '../ui/Modal';

/* 构建面板（版本/构建语义 + 四个构建动作）：
   注意口径：这里没有 Git —— 后端不存在任何 git 集成，「分支图标」是早期
   设计稿的说法（2026-10-04 用户反馈「有 Git 跟踪但不能提交」源于此）。
   真实能力 = 版本登记 → 锁定 → .mrpack 构建 → 交付检查 → 发布记录（本地），
   平台发布（CurseForge/Modrinth）尚未实现，见下方发布记录空态文案。
   顶部四个动作 = 这个工具真正要产出的东西 ——
     1 构建发布包      走包内权威清单装配 .mrpack（正式产物）
     2 检查包是否存在异常 把真实信号（冲突 / 健康 / 目录 / 版本 / 任务书）落成交付检查
     3 生成一键导入文件  调试用，钉在一个 draft 版本上，随时可重复生成
     4 工作台内容重建   重建物品目录（原在索引态工具栏最右侧，见「重排目录」反馈）
   下面几段是状态回读：版本、产物、交付检查、发布记录。

   闸门口径：有 error 级待解决冲突时禁用登记与构建，比后端 assertPackBuildable 取严。
   产物必须落在一个已批准（带 .mpackstation-export 标记）的目录里，所以先选目录。 */

const DIR_KEY = 'web3.exportDir';

function detailText(raw: string): string {
  if (!raw) return '';
  try {
    const v = JSON.parse(raw) as {message?: string};
    return typeof v?.message === 'string' ? v.message : raw;
  } catch { return raw; }
}

export function BuildPanel() {
  const {packId} = useUrlState();
  const {refresh, pendingConflicts, health} = usePackSummary();
  const {status: catalogStatus, rebuild} = useCatalog();
  const [versions, setVersions] = useState<PackVersion[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [releases, setReleases] = useState<Release[]>([]);
  const [checks, setChecks] = useState<DeliveryCheck[] | null>(null);
  const [version, setVersion] = useState('');
  const [selectedVersion, setSelectedVersion] = useState<PackVersion | null>(null);
  const [dirs, setDirs] = useState<ExportDir[]>([]);
  const [dirName, setDirName] = useState<string>(() => localStorage.getItem(DIR_KEY) ?? '');
  const [picking, setPicking] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!packId) return;
    listVersions(packId).then(setVersions).catch(e => setError(String(e)));
    listArtifacts(packId).then(setArtifacts).catch(e => setError(String(e)));
    listReleases(packId).then(setReleases).catch(() => setReleases([]));
    listDeliveryChecks(packId).then(r => setChecks(r.items)).catch(() => setChecks([]));
    listExportDirs().then(list => {
      setDirs(list);
      setDirName(cur => list.some(d => d.name === cur) ? cur : (list[0]?.name ?? ''));
    }).catch(() => setDirs([]));
  }, [packId]);
  useEffect(load, [load]);
  useEffect(() => { if (dirName) localStorage.setItem(DIR_KEY, dirName); }, [dirName]);

  if (!packId) return null;
  const errorConflicts = pendingConflicts.filter(c => c.severity === 'error').length;
  const gated = errorConflicts > 0;
  /* 构建的版本：优先选中的，否则第一个正式版，再否则任意一个。 */
  const target = selectedVersion ?? versions.find(v => v.channel === 'release') ?? versions[0] ?? null;

  /* 四个动作各自需要的前置条件不同：只有产物落盘才需要目录，只有产物才需要版本；
     检查与重建不写文件，不该被目录卡住。 */
  const guard = (need: {version?: boolean; dir?: boolean}) => {
    if (need.dir && !dirName) return '先选一个导出目录（产物要落在一个已批准的本机目录里）。';
    if (need.version && !target) return '先登记一个版本 —— 构建产物必须挂在某个版本上。';
    if (gated) return `有 ${errorConflicts} 条 error 级冲突未解决，构建已禁用。`;
    return null;
  };

  const addVersion = async () => {
    if (!version.trim() || gated) return;
    await createVersion(packId, {version: version.trim(), channel: 'release'});
    setVersion('');
    load();
    refresh();
  };

  const doBuildRelease = async () => {
    const stop = guard({version: true, dir: true});
    if (stop) { setError(stop); return; }
    setBusy('release'); setError(null); setMsg(null);
    try {
      const r = await buildPack(packId, {packVersionId: target!.id, exportDirName: dirName});
      setMsg(`发布包已生成：${r.artifact.fileName}（${Math.max(1, Math.round(r.artifact.sizeBytes / 1024))} KB，${r.artifact.kind}）`);
      load(); refresh();
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(null); }
  };

  const doBuildDebug = async () => {
    const stop = guard({dir: true});
    if (stop) { setError(stop); return; }
    setBusy('debug'); setError(null); setMsg(null);
    try {
      /* 调试包钉在一个 draft 版本上：不占用正式版本号，随时可重复生成。 */
      let v = versions.find(x => x.channel === 'draft') ?? null;
      if (!v) {
        v = await createVersion(packId, {version: `${version.trim() || '0.0.0'}-debug`, channel: 'draft', changelog: '调试用一键导入包'});
      }
      const r = await buildPack(packId, {packVersionId: v.id, exportDirName: dirName});
      setMsg(`一键导入文件已生成：${r.artifact.fileName}（${Math.max(1, Math.round(r.artifact.sizeBytes / 1024))} KB）—— 直接拖进启动器即可导入。`);
      load(); refresh();
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(null); }
  };

  /* 检查包是否存在异常：读真实信号（冲突 / 健康 / 目录新鲜度 / 版本 / 任务书），
     组装成后端认得的那几种交付检查后落库 —— 结论可回读，不是一次性的 toast。 */
  const doCheck = async () => {
    /* 检查不写产物、也不该被闸门挡住 —— 它正是用来把「有多少条 error」算清楚的手段。 */
    if (!target) { setError('先登记一个版本 —— 检查结论要挂在一个版本上。'); return; }
    setBusy('check'); setError(null); setMsg(null);
    try {
      const warns = pendingConflicts.length - errorConflicts;
      const items: DeliveryCheck[] = [
        {kind: 'conflict', status: errorConflicts > 0 ? 'blocked' : warns > 0 ? 'warning' : 'passed',
          detail: JSON.stringify({message: `未解决冲突 ${pendingConflicts.length} 条（error ${errorConflicts} · warning ${warns}）`, total: pendingConflicts.length})},
        {kind: 'dependency', status: (health?.pendingErrors ?? 0) > 0 ? 'warning' : 'passed',
          detail: JSON.stringify({message: health?.healthy ? '依赖与健康检查通过' : `待处理错误 ${health?.pendingErrors ?? 0} · 警告 ${health?.pendingWarnings ?? 0}`})},
        {kind: 'content', status: catalogStatus?.stale ? 'warning' : 'passed',
          detail: JSON.stringify({message: catalogStatus?.stale ? '物品目录已过期，需重建后再交付' : `物品目录 r${catalogStatus?.builtRevision ?? 0} 是最新的`})},
        {kind: 'version', status: 'passed',
          detail: JSON.stringify({message: `构建目标版本 ${target!.version}（${target!.channel}）`})},
        {kind: 'quest', status: 'passed',
          detail: JSON.stringify({message: '任务书由编排态「校验」单独把关'})},
      ];
      const r = await runDeliveryChecks(packId, {packVersionId: target!.id, checks: items});
      const bad = r.items.filter(c => c.status === 'blocked').length;
      setChecks(r.items);
      setMsg(bad > 0 ? `检查完成：${bad} 项阻断，先处理再构建。` : '检查完成：没有阻断项。');
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(null); }
  };

  const doRebuildCatalog = async () => {
    setBusy('catalog'); setError(null); setMsg(null);
    try {
      await rebuild();
      setMsg('已触发目录重建，进度见底部任务；完成后物品与配方立刻可用。');
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(null); }
  };

  return (
    <>
      <div className="tp-head"><span>构建</span></div>
      <div className="tp-body">
        <div className="p-section">
          <div className="p-title">产物目录</div>
          <div className="p-row">
            <span className="dot" style={{background: dirName ? 'var(--mc-success)' : 'var(--mc-orange)'}}/>
            <select className="p-input" style={{flex: 1, minWidth: 0}} value={dirName}
              onChange={e => setDirName(e.target.value)} title="构建产物写到哪个本机目录">
              {dirs.length === 0 && <option value="">还没有批准过的目录</option>}
              {dirs.map(d => <option key={d.name} value={d.name}>{d.name} · {d.directory}</option>)}
            </select>
            <button type="button" className="p-btn" onClick={() => setPicking(true)}>选…</button>
          </div>
          {dirs.length === 0 && <div className="p-empty">还没有导出目录。点「选…」挑一个本机目录，系统会在里面放一个标记文件再启用。</div>}
        </div>

        <div className="p-section">
          <div className="p-title">构建动作</div>
          <div className="build-actions">
            <button type="button" className="p-btn primary" disabled={!!busy}
              onClick={() => void doBuildRelease()}
              title="按包内权威清单装配 .mrpack 发布包">
              <Icon name="hammer" size={13}/> {busy === 'release' ? '构建中…' : '构建发布包'}
            </button>
            <button type="button" className="p-btn" disabled={!!busy}
              onClick={() => void doCheck()}
              title="检查冲突、依赖健康、目录新鲜度、版本与任务书，结论落库可回读">
              {busy === 'check' ? '检查中…' : '检查包是否存在异常'}
            </button>
            <button type="button" className="p-btn" disabled={!!busy}
              onClick={() => void doBuildDebug()}
              title="生成一个可直接拖进启动器的调试用整合包文件（钉在 draft 版本上，可重复生成）">
              {busy === 'debug' ? '生成中…' : '生成一键导入文件'}
            </button>
            <button type="button" className="p-btn" disabled={!!busy}
              onClick={() => void doRebuildCatalog()}
              title="重建物品/配方目录（原「重建目录」，已从索引工具栏挪到此处）">
              {busy === 'catalog' ? '重建中…' : '工作台内容重建'}
            </button>
          </div>
          {error && <div className="p-empty">{error}</div>}
          {msg && <div className="p-empty" style={{color: 'var(--mc-success)'}}>{msg}</div>}
        </div>

        <div className="p-section">
          <div className="p-title">版本 <span className="count">{versions.length}</span></div>
          <div style={{display: 'flex', gap: 4}}>
            <input className="p-input" style={{flex: 1, minWidth: 0}} value={version} placeholder="新版本号，如 0.4.0"
              onChange={e => setVersion(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') void addVersion().catch(e => setError(String(e))); }}/>
            <button type="button" className="p-btn primary" disabled={gated || !version.trim()}
              onClick={() => void addVersion().catch(e => setError(String(e)))}>登记</button>
          </div>
          {gated && <div className="p-empty">有 {errorConflicts} 条 error 级冲突未解决，构建闸门已禁用本段动作。</div>}
          {versions.map(v => (
            <div key={v.id} className={`p-row click${target?.id === v.id ? ' on' : ''}`}
              onClick={() => setSelectedVersion(v)} title="点击选中此版本进行构建">
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
              <span className="sub">{a.kind}</span>
              <span className="sub">{Math.max(1, Math.round(a.sizeBytes / 1024))} KB</span>
              {a.status === 'ready' && (
                <a className="p-btn" href={`/api/packs/${encodeURIComponent(packId)}/artifacts/${encodeURIComponent(a.id)}/download`} download>下载</a>
              )}
            </div>
          ))}
          {artifacts.length === 0 && <div className="p-empty">还没有产物。用上面的「构建发布包」生成一个。</div>}
        </div>

        <div className="p-section">
          <div className="p-title">交付检查 <span className="count">{checks?.length ?? 0}</span></div>
          {(checks ?? []).map((c, i) => (
            <div key={`${c.kind}-${i}`} className="p-row">
              <span className="dot" style={{background: c.status === 'passed' ? 'var(--mc-success)' : c.status === 'blocked' ? 'var(--mc-fail)' : 'var(--mc-orange)'}}/>
              <span className="grow">{detailText(c.detail) || c.kind}</span>
              <span className="sub">{c.kind} · {c.status}</span>
            </div>
          ))}
          {checks?.length === 0 && (
            <div className="p-empty">还没有检查记录。点「检查包是否存在异常」跑一次。</div>
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
            <div className="p-empty">平台发布（CurseForge / Modrinth）尚未实现：当前产物是 .mrpack 文件，可直接导入启动器。接入平台发布需要 API 凭证与上传链路，届时这里会出现发布入口。</div>
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

      {picking && (
        <DirPicker onClose={() => setPicking(false)}
          onPicked={name => { setDirName(name); setPicking(false); load(); }}/>
      )}
    </>
  );
}

/* 目录选择器：后端枚举（浏览器拿不到服务端绝对路径），选定后立即注册成批准目录。 */
function DirPicker({onClose, onPicked}: {onClose: () => void; onPicked: (name: string) => void}) {
  const [browse, setBrowse] = useState<FsBrowse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const go = useCallback((path?: string) => {
    browseDirectories(path).then(b => { setBrowse(b); setError(null); }).catch(e => setError(String(e)));
  }, []);
  useEffect(() => { go(); }, [go]);

  const take = async (path: string) => {
    const name = path.split('/').filter(Boolean).pop() || 'export';
    setBusy(true);
    try {
      await registerExportDir(name, path);
      onPicked(name);
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };

  return (
    /* 弹层走 ui/Modal：portal 出去 + Esc 独占（内联渲染时画布宿主会抢走点击，
       Esc 不截住会连视图一起切走）。内容仍是这个面板自己的目录列表。 */
    <Modal onClose={onClose} className="dir-modal">
      <>
        <div className="p-title">选择产物目录</div>
        <div className="dm-path">
          <span className="mono grow">{browse?.path ?? '…'}</span>
          <button type="button" className="p-btn" onClick={() => browse?.parent && go(browse.parent)}>上一级</button>
          <button type="button" className="p-btn primary" disabled={!browse || busy}
            onClick={() => browse && void take(browse.path)}>用这个目录</button>
        </div>
        {error && <div className="p-empty">{error}</div>}
        <div className="dm-list">
          {(browse?.directories ?? []).map(d => (
            <div key={d.path} className="p-row click" onClick={() => go(d.path)} title={d.path}>
              <span className="grow">{d.name}</span>
              <span className="sub" style={{display: 'inline-flex', alignItems: 'center', gap: 3}}>
                进入 <Icon name="caretRight" size={12}/>
              </span>
              <button type="button" className="p-btn" disabled={busy}
                onClick={e => { e.stopPropagation(); void take(d.path); }}>选它</button>
            </div>
          ))}
          {browse && browse.directories.length === 0 && <div className="p-empty">这个目录下没有子目录。</div>}
        </div>
        {!!browse?.suggested.length && (
          <>
            <div className="p-title" style={{marginTop: 6}}>常用位置</div>
            {browse.suggested.map(d => (
              <div key={d.path} className="p-row click" onClick={() => void take(d.path)} title={d.path}>
                <span className="grow">{d.name}</span>
                <span className="sub">{d.path}</span>
              </div>
            ))}
          </>
        )}
        <div style={{display: 'flex', justifyContent: 'flex-end', marginTop: 8}}>
          <button type="button" className="p-btn" onClick={onClose}>取消</button>
        </div>
      </>
    </Modal>
  );
}
