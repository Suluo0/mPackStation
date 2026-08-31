import {useCallback, useEffect, useRef, useState} from 'react';
import {fetchActivities, fetchDashboard, type DashboardActivity, type DashboardData} from '../api/dashboard';
import {fetchTasks, type Task} from '../api/tasks';
import {fetchHealth, fetchStatus, type SystemHealth, type SystemStatus} from '../api/system';

/* 看板场景:聚合五个数据源 + 任务轮询(仅在有 running 任务且页面可见时每 3 秒刷新)。 */
export function useDashboard() {
  const [dashboard, setDashboard] = useState<DashboardData | null>(null);
  const [dashError, setDashError] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [activities, setActivities] = useState<DashboardActivity[] | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [status, setStatus] = useState<SystemStatus | null>(null);

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

  return {
    dashboard, dashError, clearDashError: () => setDashError(null),
    tasks, activities, health, status,
    loadDashboard, loadHealth, refreshTasks,
  };
}
