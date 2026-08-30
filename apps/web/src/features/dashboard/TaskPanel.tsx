import {App, Button, Drawer, Progress} from 'antd';
import {useState} from 'react';
import {DatabaseOutlined, ImportOutlined, RocketOutlined, SyncOutlined} from '@ant-design/icons';
import type {ReactNode} from 'react';
import type {DashboardTask} from './types';
import {cancelTask, pauseTask, resumeTask, retryTask} from './api';
import {WorkbenchCard, WorkbenchSectionHeader} from '../../ui/workbench/Workbench';

/* 后台任务区（右列卡）：仅存在任务时渲染；进行中可暂停/取消，失败可重试/查看错误。 */

/* type 是开放字符串（后端存在未映射的任务类型），所以这里一律带兜底。 */
const taskIcon: Record<string, ReactNode> = {
  'build-pack': <RocketOutlined/>,
  'index-mod': <DatabaseOutlined/>,
  'import-pack': <ImportOutlined/>,
  'update-preflight': <SyncOutlined/>,
};

const runningVerb: Record<string, string> = {
  'build-pack': '打包中…',
  'index-mod': '索引中…',
  'import-pack': '导入中…',
  'update-preflight': '预演中…',
};

const statusText: Record<string, string> = {
  success: '已完成', failed: '失败', cancelled: '已取消', paused: '已暂停',
};

function taskSubText(task: DashboardTask): string {
  if (task.status === 'running') return runningVerb[task.type] ?? '处理中…';
  return statusText[task.status] ?? task.status;
}

type TaskAction = (id: string) => Promise<unknown>;

export function TaskPanel({tasks, onChanged}: {tasks: DashboardTask[]; onChanged: () => void}) {
  const {message} = App.useApp();
  const [errorOf, setErrorOf] = useState<DashboardTask | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  if (tasks.length === 0) return null;

  const run = async (taskId: string, label: string, fn: TaskAction) => {
    setBusyId(taskId);
    try {
      await fn(taskId);
      message.success(`任务已${label}`);
      onChanged();
    } catch (err) {
      message.error(`${label}失败：${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusyId(null);
    }
  };

  const showAll = () => message.info('任务中心将在后续版本提供');

  return (
    <WorkbenchCard className="db-tasks">
      <WorkbenchSectionHeader title="后台任务" action={<button className="db-link" onClick={showAll}>全部任务 ›</button>}/>
      {tasks.map(task => (
        <div key={task.id} className="db-task-row">
          <span className={task.status === 'failed' ? 'db-task-icon db-task-icon-failed' : 'db-task-icon'}>
            {taskIcon[task.type] ?? <SyncOutlined/>}
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
                  <Button size="small" type="text" loading={busyId === task.id}
                          onClick={() => void run(task.id, '暂停', pauseTask)}>暂停</Button>
                  <Button size="small" type="text" disabled={busyId === task.id}
                          onClick={() => void run(task.id, '取消', cancelTask)}>取消</Button>
                </>
              )}
              {task.status === 'paused' && (
                <Button size="small" type="text" loading={busyId === task.id}
                        onClick={() => void run(task.id, '继续', resumeTask)}>继续</Button>
              )}
              {task.status === 'failed' && (
                <>
                  <Button size="small" type="primary" loading={busyId === task.id}
                          onClick={() => void run(task.id, '重试', retryTask)}>重试</Button>
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
