import {useCallback, useEffect, useState} from 'react';
import {installLauncher, launchLauncher, listLauncherInstalls, type LauncherInstall} from '../api/launcher';
import {usePackSummary} from '../app/PackSummaryContext';
import {useUrlPatch, useUrlState} from '../app/url';

/* 运行面板：游戏目录 + 安装记录 + 启动配置。动作全部真接口；
   安装/启动入队为后台任务，成功触发后自动弹底部停靠看进度与日志（V3 §1）。 */
export function RunPanel() {
  const {packId} = useUrlState();
  const patch = useUrlPatch();
  const {refresh, tasks} = usePackSummary();
  const [dir, setDir] = useState(() => localStorage.getItem('web3.mcDir') ?? '');
  const [username, setUsername] = useState(() => localStorage.getItem('web3.username') ?? 'Player');
  const [installs, setInstalls] = useState<LauncherInstall[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    if (!packId || !dir.trim()) { setInstalls([]); return; }
    listLauncherInstalls(packId, dir.trim()).then(setInstalls).catch(() => setInstalls([]));
  }, [packId, dir]);
  useEffect(load, [load]);

  if (!packId) return null;

  const installed = installs[0];
  const launchTasks = tasks.filter(t => t.packId === packId && (t.type.includes('launch') || t.type.includes('install'))).slice(0, 5);

  const doInstall = async () => {
    if (!installed) return;
    setBusy(true); setError(null);
    try {
      await installLauncher({version: installed.versionId, minecraftDir: dir.trim(), packId});
      patch({dock: '1'});
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const doLaunch = async () => {
    if (!dir.trim() || !username.trim()) return;
    setBusy(true); setError(null);
    localStorage.setItem('web3.mcDir', dir.trim());
    localStorage.setItem('web3.username', username.trim());
    try {
      await launchLauncher({username: username.trim(), minecraftDir: dir.trim(), packId});
      patch({dock: '1'});
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  return (
    <>
      <div className="tp-head"><span>运行</span></div>
      <div className="tp-body">
      <div className="p-section">
        <div className="p-title">游戏目录</div>
        <input className="p-input" style={{width: '100%'}} value={dir} placeholder="/path/to/.minecraft"
          onChange={e => setDir(e.target.value)} onBlur={load}/>
        <div className="p-row">
          <span className="dot" style={{background: installed ? 'var(--mc-success)' : 'var(--mc-muted)'}}/>
          <span className="grow sub">{installed ? `已装 ${installed.versionId}` : dir.trim() ? '该目录还没有本包的安装记录' : '先填游戏目录'}</span>
        </div>
        {error && <div className="p-empty">{error}</div>}
      </div>

      <div className="p-section">
        <div className="p-title">启动配置</div>
        <div className="p-row">
          <span className="sub" style={{width: 44}}>用户名</span>
          <input className="p-input" style={{flex: 1, minWidth: 0}} value={username} onChange={e => setUsername(e.target.value)}/>
        </div>
        <div style={{display: 'flex', gap: 4}}>
          <button type="button" className="p-btn" disabled={!installed || busy} onClick={() => void doInstall()}
            title={installed ? '' : '需要该目录已有安装记录'}>重装版本</button>
          <button type="button" className="p-btn primary" disabled={!dir.trim() || !username.trim() || busy}
            onClick={() => void doLaunch()}>▶ 启动 Minecraft</button>
        </div>
        <div className="p-empty">启动入参由后端从该目录已装版本解析（fabric-loader-&lt;loader&gt;-&lt;mc&gt;）；进度与日志看底部停靠。</div>
      </div>

      <div className="p-section">
        <div className="p-title">本包运行任务 <span className="count">{launchTasks.length}</span></div>
        {launchTasks.map(t => (
          <div key={t.id} className="p-row">
            <span className="dot" style={{background: t.status === 'success' ? 'var(--mc-success)' : t.status === 'failed' ? 'var(--mc-fail)' : 'var(--mc-orange)'}}/>
            <span className="grow sub">{t.title || t.type}</span>
            <span className="sub">{t.progress}%</span>
          </div>
        ))}
        {launchTasks.length === 0 && <div className="p-empty">还没有安装/启动任务。</div>}
      </div>
      </div>
    </>
  );
}
