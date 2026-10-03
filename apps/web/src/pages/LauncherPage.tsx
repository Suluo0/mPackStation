import {useCallback, useEffect, useRef, useState} from 'react';
import {App} from 'antd';
import {DownloadOutlined, FolderOpenOutlined, PlayCircleOutlined} from '@ant-design/icons';
import {useParams} from 'react-router-dom';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import {usePack} from '../hooks/usePack';
import {fetchTasks, type Task} from '../api/tasks';
import {
  fetchTaskLog,
  installLauncher,
  launchLauncher,
  listLauncherInstalls,
  type LauncherInstall,
  type TaskLogEvent,
} from '../api/launcher';
import {acknowledgeOnboarding} from '../api/onboarding';
import {TaskStatusTag, relativeTime} from '../features/dashboard/signals';
import {DirectoryPicker} from '../features/common/DirectoryPicker';
import './pack-pages.css';

const LAUNCHER_TYPES = ['launcher_install', 'launcher_launch'];

export function LauncherPage() {
  const {id} = useParams();
  const {message} = App.useApp();
  const {pack} = usePack(id);
  const [minecraftDir, setMinecraftDir] = useState('');
  const [username, setUsername] = useState('');
  const [javaPath, setJavaPath] = useState('');
  const [xmxMb, setXmxMb] = useState(4096);
  const [task, setTask] = useState<Task | null>(null);
  const [logs, setLogs] = useState<TaskLogEvent[]>([]);
  const [busy, setBusy] = useState(false);
  const [dirPickerOpen, setDirPickerOpen] = useState(false);
  const [javaPickerOpen, setJavaPickerOpen] = useState(false);
  const [installs, setInstalls] = useState<LauncherInstall[]>([]);
  const busyRef = useRef(false);

  const refresh = useCallback(async (): Promise<boolean> => {
    const list = await fetchTasks();
    const mine = list
      .filter(t => LAUNCHER_TYPES.includes(t.type))
      .sort((a, b) => (b.startedAt ?? '').localeCompare(a.startedAt ?? ''))[0] ?? null;
    setTask(mine);
    if (mine) {
      try { setLogs(await fetchTaskLog(mine.id)); } catch { /* ignore */ }
    }
    return mine?.status === 'running' || mine?.status === 'queued';
  }, []);

  useEffect(() => {
    let stopped = false;
    let timer: number | null = null;
    const loop = async () => {
      let running = false;
      try { running = await refresh(); } catch { /* ignore */ }
      if (stopped) return;
      if (running && timer === null) {
        timer = window.setInterval(() => { void refresh().catch(() => undefined); }, 3000);
      } else if (!running && timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    };
    void loop();
    return () => {
      stopped = true;
      if (timer !== null) clearInterval(timer);
    };
  }, [refresh]);

  /* 目录里已装好的版本：启动必须用安装时落库的 versionId，而不是包上的 mcVersion。
     见后端 launcher_installs 表（安装完成时写入）与 resolveLaunchVersion。 */
  useEffect(() => {
    const dir = minecraftDir.trim();
    if (!dir) { setInstalls([]); return; }
    let stopped = false;
    const load = async () => {
      try {
        const list = await listLauncherInstalls(id, dir);
        if (!stopped) setInstalls(list);
      } catch { if (!stopped) setInstalls([]); }
    };
    void load();
    return () => { stopped = true; };
  }, [id, minecraftDir]);

  const guardBusy = () => {
    if (busyRef.current) return false;
    busyRef.current = true;
    setBusy(true);
    return true;
  };
  const releaseBusy = () => { busyRef.current = false; setBusy(false); };

  /* 离线启动优先：配置过游戏目录/账号即视为启动台就绪，不必正版登录。 */
  const markLauncherReady = () => {
    void acknowledgeOnboarding({launcherReady: true}).catch(() => undefined);
  };

  const validateCommon = (): string | null => {
    if (!pack) return '整合包尚未加载';
    if (!minecraftDir.trim()) return '请先选择游戏目录';
    return null;
  };

  const onInstall = async () => {
    const invalid = validateCommon();
    if (invalid) { message.error(invalid); return; }
    if (!guardBusy()) return;
    try {
      const r = await installLauncher({
        version: pack!.mcVersion,
        loader: pack!.loader,
        // 包上记录了加载器版本就必须带上：启动器侧 --loader-version 缺省是 latest,
        // 会让装出来的加载器和包锁定的版本不一致。
        loaderVersion: pack!.loaderVersion ?? undefined,
        minecraftDir: minecraftDir.trim(),
        // 归属到当前包：安装记录（versionId）按包+目录索引，换包/换目录才能找对版本。
        packId: id,
      });
      message.success(`安装任务已入队（${r.taskId.slice(0, 8)}…）`);
      markLauncherReady();
      void refresh().catch(() => undefined);
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally { releaseBusy(); }
  };

  const onLaunch = async () => {
    const invalid = validateCommon();
    if (invalid) { message.error(invalid); return; }
    if (!username.trim()) { message.error('请填写启动账号（离线用户名，无需正版）'); return; }
    if (xmxMb < 512) { message.error('内存至少 512 MB'); return; }
    if (!guardBusy()) return;
    try {
      const r = await launchLauncher({
        // 启动的是「这个目录里装好的那个版本」：优先用安装记录里的 versionId。
        // 列表为空时留空，由服务端 resolveLaunchVersion 兜底解析/给出明确提示。
        version: installs[0]?.versionId,
        username: username.trim(),
        minecraftDir: minecraftDir.trim(),
        packId: id,
        javaPath: javaPath.trim() || undefined,
        xmxMb: xmxMb || undefined,
      });
      message.success(`启动任务已入队（${r.taskId.slice(0, 8)}…）`);
      markLauncherReady();
      void refresh().catch(() => undefined);
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally { releaseBusy(); }
  };

  return (
    <div className="workspace-page">
      <div className="page-heading compact">
        <div>
          <span className="eyebrow">LAUNCHER</span>
          <h1>启动器</h1>
          <p>自研 mPackLauncher：离线用户名即可启动，不要求正版/Microsoft 登录。目录用选择器选取。</p>
        </div>
      </div>

      <WorkbenchCard className="catalog-card">
        <div className="launcher-instance">
          <div className="launcher-head">
            <div className="launcher-name"><strong>{pack?.name ?? '加载中…'}</strong></div>
            <div className="launcher-env">
              <span className="db-env-tag db-env-tag-mc">MC {pack?.mcVersion ?? '…'}</span>
              <span className="db-env-tag db-env-tag-loader">{pack?.loader ?? '…'}</span>
              <span className="db-muted">v{pack?.packVersion ?? '…'}</span>
            </div>
          </div>
          <div className="launcher-fields">
            <label className="launcher-field launcher-field-wide">
              <span>游戏目录（必填）</span>
              <div className="launcher-field-row">
                <input
                  placeholder="Minecraft 实例目录"
                  value={minecraftDir}
                  onChange={e => setMinecraftDir(e.target.value)}
                />
                <WorkbenchButton icon={<FolderOpenOutlined/>} onClick={() => setDirPickerOpen(true)}>选择目录</WorkbenchButton>
              </div>
            </label>
            <label className="launcher-field">
              <span>Java 路径（可选）</span>
              <div className="launcher-field-row">
                <input placeholder="留空使用 PATH 中的 java" value={javaPath} onChange={e => setJavaPath(e.target.value)}/>
                <WorkbenchButton tone="quiet" icon={<FolderOpenOutlined/>} onClick={() => setJavaPickerOpen(true)}>选择</WorkbenchButton>
              </div>
            </label>
            <label className="launcher-field">
              <span>启动账号（离线，必填）</span>
              <input placeholder="任意离线用户名，无需正版/Microsoft" value={username} onChange={e => setUsername(e.target.value)}/>
            </label>
            <label className="launcher-field">
              <span>内存 (MB)</span>
              <input type="number" min={512} step={512} value={xmxMb} onChange={e => setXmxMb(Number(e.target.value) || 0)}/>
            </label>
          </div>
          <div className="launcher-installs">
            {installs.length > 0 ? (
              installs.map(i => (
                <span className="db-muted" key={i.id}>
                  已安装 <b>{i.versionId}</b>（{i.loader || 'vanilla'} · MC {i.mcVersion} · {relativeTime(i.installedAt)}）
                </span>
              ))
            ) : (
              <span className="db-muted">
                {minecraftDir.trim() ? '该目录还没有安装记录，先点「安装游戏」。' : '选择游戏目录后显示该目录已安装的版本。'}
              </span>
            )}
          </div>
          <div className="launcher-actions">
            <WorkbenchButton onClick={onInstall} icon={<DownloadOutlined/>} loading={busy}>安装游戏</WorkbenchButton>
            <WorkbenchButton tone="primary" onClick={onLaunch} icon={<PlayCircleOutlined/>} loading={busy}>启动游戏</WorkbenchButton>
          </div>
        </div>
      </WorkbenchCard>

      <div className="launcher-grid">
        <WorkbenchCard className="catalog-card">
          <WorkbenchSectionHeader title="安装 / 启动状态" action={task ? <TaskStatusTag status={task.status}/> : undefined}/>
          {task ? (
            <div className="launcher-task">
              <div className="launcher-task-row"><span className="db-muted">{task.title}</span><b className="tabular">{task.progress}%</b></div>
              <div className="launcher-progress"><i style={{width: `${task.progress}%`}}/></div>
              {task.error && <div className="empty-inline">失败：{task.error}</div>}
            </div>
          ) : (
            <div className="empty-inline">还没有启动器任务。选择游戏目录后点击「安装游戏」或「启动游戏」。</div>
          )}
        </WorkbenchCard>

        <WorkbenchCard className="catalog-card">
          <WorkbenchSectionHeader title="游戏日志"/>
          <pre className="launcher-log">{logs.length > 0 ? logs.map(l => l.message).join('\n') : '（暂无日志）'}</pre>
        </WorkbenchCard>
      </div>

      <DirectoryPicker
        open={dirPickerOpen}
        title="选择游戏目录"
        value={minecraftDir}
        onClose={() => setDirPickerOpen(false)}
        onSelect={setMinecraftDir}
      />
      <DirectoryPicker
        open={javaPickerOpen}
        title="选择 Java 目录"
        value={javaPath}
        onClose={() => setJavaPickerOpen(false)}
        onSelect={setJavaPath}
      />
    </div>
  );
}
