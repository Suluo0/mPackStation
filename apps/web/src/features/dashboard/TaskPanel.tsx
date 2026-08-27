import {App, Button, Drawer, Progress} from 'antd';
import {useState} from 'react';
import {DatabaseOutlined, ImportOutlined, RocketOutlined, SyncOutlined} from '@ant-design/icons';
import type {ReactNode} from 'react';
import type {DashboardTask} from './types';
import {WorkbenchCard, WorkbenchSectionHeader} from '../../ui/workbench/Workbench';

/* 后台任务区（右列卡）：仅存在任务时渲染；进行中可暂停/取消，失败可重试/查看错误。 */

const taskIcon: Record<DashboardTask['type'], ReactNode> = {
  'build-pack': <RocketOutlined/>,
  'index-mod': <DatabaseOutlined/>,
  'import-pack': <ImportOutlined/>,
  'update-preflight': <SyncOutlined/>,
};

const runningVerb: Record<DashboardTask['type'], string> = {
  'build-pack': '打包中…',
  'index-mod': '索引中…',
  'import-pack': '导入中…',
  'update-preflight': '预演中…',
};

const statusText: Record<string, string> = {
  success: '已完成', failed: '失败', cancelled: '已取消', paused: '已暂停',
};

function taskSubText(task: DashboardTask): string {
  if (task.status === 'running') return runningVerb[task.type];
  return statusText[task.status] ?? task.status;
}

export function TaskPanel({tasks}: {tasks: DashboardTask[]}) {
  const {message} = App.useApp();
  const [errorOf, setErrorOf] = useState<DashboardTask | null>(null);

  if (tasks.length === 0) return null;

  const todo = (action: string) => message.info(`任务${action}将在接入真实后端后生效`);
  const showAll = () => message.info('任务中心将在后续版本提供');

  return (
    <WorkbenchCard className="db-tasks">
      <WorkbenchSectionHeader title="后台任务" action={<button className="db-link" onClick={showAll}>全部任务 ›</button>}/>
      {tasks.map(task => (
        <div key={task.id} className="db-task-row">
          <span className={task.status === 'failed' ? 'db-task-icon db-task-icon-failed' : 'db-task-icon'}>
            {taskIcon[task.type]}
          </span>
          <div className="db-task-main">
            <div className="db-task-title">
              <span className="db-task-title-text">{task.title}</span>
              <span className="db-task-pct">{Math.round(task.progress)}%</span>
            </div>
            <div className={task.status === 'failed' ? 'db-task-sub db-task-sub-failed' : 'db-task-sub'}>
              {taskSubText(task)}
            </div>
            <div className="db-task-progress">
              <Progress percent={Math.round(task.progress)} size="small" showInfo={false}
                        status={task.status === 'failed' ? 'exception' : 'normal'}/>
            </div>
            <div className="db-task-actions">
              {task.status === 'running' && (
                <>
                  <Button size="small" type="text" onClick={() => todo('暂停')}>暂停</Button>
                  <Button size="small" type="text" onClick={() => todo('取消')}>取消</Button>
                </>
              )}
              {task.status === 'failed' && (
                <>
                  <Button size="small" type="primary" onClick={() => todo('重试')}>重试</Button>
                  <Button size="small" type="text" onClick={() => setErrorOf(task)}>查看错误</Button>
                </>
              )}
            </div>
          </div>
        </div>
      ))}
      <div className="db-task-foot">
        <button className="db-link" onClick={showAll}>查看全部任务 →</button>
      </div>
      <Drawer title="任务错误详情" open={errorOf !== null} onClose={() => setErrorOf(null)} width={420}>
        <p className="db-h2" style={{marginBottom: 8}}>{errorOf?.title}</p>
        <pre style={{whiteSpace: 'pre-wrap', color: 'var(--mc-fail)'}}>{errorOf?.error ?? '无错误信息'}</pre>
      </Drawer>
    </WorkbenchCard>
  );
}
