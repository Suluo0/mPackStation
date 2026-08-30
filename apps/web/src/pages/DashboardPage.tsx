import {useCallback, useEffect, useRef, useState} from 'react';
import {App, Button, Skeleton} from 'antd';
import {PlusOutlined, ImportOutlined} from '@ant-design/icons';
import {useNavigate} from 'react-router-dom';
import {
  fetchActivities, fetchDashboard, fetchHealth, fetchStatus, fetchTasks,
  deletePack as deletePackRequest,
} from '../features/dashboard/api';
import type {DashboardActivity, DashboardData, DashboardTask, SystemHealth, SystemStatus} from '../features/dashboard/types';
import {EnvHealthBanner} from '../features/dashboard/EnvHealthBanner';
import {OnboardingView} from '../features/dashboard/OnboardingView';
import {ContinueCard} from '../features/dashboard/ContinueCard';
import {PackList} from '../features/dashboard/PackList';
import {TaskPanel} from '../features/dashboard/TaskPanel';
import {StatusActivity} from '../features/dashboard/StatusActivity';
import {CreatePackModal, ImportPackModal} from '../features/dashboard/PackModals';

/* 看板页：迎新（空态）/ 数据总览（有包态）二态页面。 */

export function DashboardPage({forceEmpty = false}: {forceEmpty?: boolean}) {
  const {message} = App.useApp();
  const navigate = useNavigate();

  const [dashboard, setDashboard] = useState<DashboardData | null>(null);
  const [dashError, setDashError] = useState<string | null>(null);
  const [tasks, setTasks] = useState<DashboardTask[]>([]);
  const [activities, setActivities] = useState<DashboardActivity[] | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);

  const loadDashboard = useCallback(() => {
    fetchDashboard().then(setDashboard).catch(err => setDashError(err instanceof Error ? err.message : String(err)));
  }, []);

  const loadHealth = useCallback(() => {
    fetchHealth().then(setHealth).catch(() => setHealth(null));
  }, []);

  const refreshTasks = useCallback(() => {
    void fetchTasks().then(setTasks).catch(() => undefined);
  }, []);

  useEffect(() => {
    loadDashboard();
    loadHealth();
    void fetchActivities().then(setActivities).catch(() => setActivities([]));
    void fetchStatus().then(setStatus).catch(() => setStatus(null));
    void fetchTasks().then(setTasks).catch(() => setTasks([]));
  }, [loadDashboard, loadHealth]);

  // 任务轮询：仅在有 running 任务且页面可见时每 3 秒刷新
  const timerRef = useRef<number | null>(null);
  useEffect(() => {
    const hasRunning = tasks.some(t => t.status === 'running');
    if (!hasRunning) return;
    const tick = () => {
      if (!document.hidden) void fetchTasks().then(setTasks).catch(() => undefined);
    };
    timerRef.current = window.setInterval(tick, 3000);
    return () => {
      if (timerRef.current !== null) window.clearInterval(timerRef.current);
    };
  }, [tasks]);

  const openPack = (id: string) => navigate(`/packs/${id}`);

  const deletePack = async (id: string) => {
    try {
      await deletePackRequest(id);
      message.success('整合包已删除');
      loadDashboard();
    } catch (err) {
      message.error(`删除失败：${err instanceof Error ? err.message : String(err)}`);
    }
  };

  const onCreated = (id: string) => {
    setCreateOpen(false);
    openPack(id);
  };
  const onImported = (_id: string | null) => {
    setImportOpen(false);
    loadDashboard();
    void fetchTasks().then(setTasks).catch(() => undefined);
  };

  if (dashError) {
    return (
      <div className="db-page">
        <div className="db-card" style={{padding: 24, textAlign: 'center'}}>
          <p className="db-h2">看板数据加载失败</p>
          <p className="db-muted" style={{margin: '8px 0 16px'}}>{dashError}</p>
          <Button type="primary" onClick={() => {setDashError(null); loadDashboard();}}>重试</Button>
        </div>
      </div>
    );
  }

  if (dashboard === null) {
    return (
      <div className="db-page">
        <Skeleton active paragraph={{rows: 2}}/>
        <Skeleton active paragraph={{rows: 4}}/>
        <Skeleton active paragraph={{rows: 6}}/>
      </div>
    );
  }

  const visiblePacks = forceEmpty ? [] : dashboard.packs;
  const isEmpty = visiblePacks.length === 0;
  const lastEdited = visiblePacks.find(p => p.id === dashboard.lastEditedPackId) ?? visiblePacks[0];

  return (
    <div className="db-page">
      {health && <EnvHealthBanner health={health} onRetry={loadHealth} suppressKeys={isEmpty ? ['cf-key'] : []}/>} 

      {isEmpty ? (
        <OnboardingView
          onCreate={() => setCreateOpen(true)}
          onImport={() => setImportOpen(true)}
          onDemo={() => message.info('示例包将在后续版本提供')}
        />
      ) : (
        <>
          <div className="db-header-row">
            <div>
              <h1 className="db-h1">工作台</h1>
              <span className="db-muted">{visiblePacks.length} 个整合包 · 今日已解决 {dashboard.todayResolvedCount} 个问题</span>
            </div>
            <div className="db-header-actions">
              <Button icon={<ImportOutlined/>} onClick={() => setImportOpen(true)}>导入整合包</Button>
              <Button type="primary" className="mc-btn-cta" icon={<PlusOutlined/>} onClick={() => setCreateOpen(true)}>新建整合包</Button>
            </div>
          </div>

          <div className="db-dash-grid">
            <div className="db-dash-main">
              {lastEdited && <ContinueCard pack={lastEdited} onOpen={openPack}/>}

              <PackList packs={visiblePacks} onOpen={openPack} onDelete={deletePack}/>

              <StatusActivity status={status} activities={activities ?? []}/>
            </div>

            <TaskPanel tasks={tasks} onChanged={refreshTasks}/>
          </div>
        </>
      )}

      <CreatePackModal open={createOpen} existing={visiblePacks} onClose={() => setCreateOpen(false)} onCreated={onCreated}/>
      <ImportPackModal open={importOpen} onClose={() => setImportOpen(false)} onImported={onImported}/>
    </div>
  );
}
