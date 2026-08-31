import {useState} from 'react';
import {App, Button, Skeleton} from 'antd';
import {PlusOutlined, ImportOutlined} from '@ant-design/icons';
import {useNavigate} from 'react-router-dom';
import {deletePack} from '../api/packs';
import {useDashboard} from '../hooks/useDashboard';
import {EnvHealthBanner} from '../features/dashboard/EnvHealthBanner';
import {OnboardingView} from '../features/dashboard/OnboardingView';
import {ContinueCard} from '../features/dashboard/ContinueCard';
import {PackList} from '../features/dashboard/PackList';
import {TaskPanel} from '../features/dashboard/TaskPanel';
import {StatusActivity} from '../features/dashboard/StatusActivity';
import {CreatePackModal, ImportPackModal} from '../features/dashboard/PackModals';

/* 看板页:迎新(空态)/ 数据总览(有包态)二态页面。
   数据聚合与任务轮询在 useDashboard;本组件只管布局与弹窗开关。 */

export function DashboardPage({forceEmpty = false}: {forceEmpty?: boolean}) {
  const {message} = App.useApp();
  const navigate = useNavigate();
  const {
    dashboard, dashError, clearDashError,
    tasks, activities, health, status,
    loadDashboard, loadHealth, refreshTasks,
  } = useDashboard();

  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);

  const openPack = (id: string) => navigate(`/packs/${id}`);

  const onDelete = async (id: string) => {
    try {
      await deletePack(id);
      message.success('整合包已删除');
      loadDashboard();
    } catch (err) {
      message.error(`删除失败:${err instanceof Error ? err.message : String(err)}`);
    }
  };

  const onCreated = (id: string) => {
    setCreateOpen(false);
    openPack(id);
  };
  const onImported = (_id: string | null) => {
    setImportOpen(false);
    loadDashboard();
    refreshTasks();
  };

  if (dashError) {
    return (
      <div className="db-page">
        <div className="db-card" style={{padding: 24, textAlign: 'center'}}>
          <p className="db-h2">看板数据加载失败</p>
          <p className="db-muted" style={{margin: '8px 0 16px'}}>{dashError}</p>
          <Button type="primary" onClick={() => {clearDashError(); loadDashboard();}}>重试</Button>
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

              <PackList packs={visiblePacks} onOpen={openPack} onDelete={onDelete}/>

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
