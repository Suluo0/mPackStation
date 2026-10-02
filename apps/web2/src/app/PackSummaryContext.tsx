import {createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode} from 'react';
import {useParams} from 'react-router-dom';
import {packHealth, listLocks, listConflicts, type PackHealth, type Lock, type Conflict} from '../api/mods';
import {fetchTasks, type Task} from '../api/tasks';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';

/* ↑ 这些导出名已逐个核对过真实文件（api/mods.ts、api/tasks.ts、api/dashboard.ts）。
   注意坑：dashboard 域没有「按 packId 取单包」的端点，只有全局的 fetchDashboard()，
   返回 {packs[], lastEditedPackId, todayResolvedCount}，本包要自己 find。
   任务域的函数名是 fetchTasks 不是 listTasks。 */

export type PackSummary = {
  packId: string | null;
  pack: DashboardPack | null;
  health: PackHealth | null;
  locks: Lock[];
  pendingConflicts: Conflict[];
  tasks: Task[];
  score: number;          // 健康分，公式见下
  alertCount: number;
  loading: boolean;
  error: string | null;
  refresh: () => void;    // 任何写操作成功后调用；这是「改完立刻看得见」的唯一机制
};

const Ctx = createContext<PackSummary | null>(null);

export function usePackSummary(): PackSummary {
  const v = useContext(Ctx);
  if (!v) throw new Error('usePackSummary 必须在 PackSummaryProvider 内使用');
  return v;
}

/* 健康分公式必须只此一处。设计文档 §4.2 的 ⚠ 指出旧实现把公式埋在组件里，
   导致右栏和概览各算一份、可能不一致。
   扣分口径全部取自后端已返回的计数字段（PackHealth.pendingErrors / pendingWarnings、
   DashboardPack.alerts.crashes / alerts.updatable），不自己数数组：
   error 级待解决 -8，warning 级 -3，崩溃告警 -6，可更新模组 -1；下限 0 上限 100。 */
function computeScore(h: PackHealth | null, p: DashboardPack | null): number {
  return Math.max(0, Math.min(100,
    100
    - (h?.pendingErrors ?? 0) * 8
    - (h?.pendingWarnings ?? 0) * 3
    - (p?.alerts.crashes ?? 0) * 6
    - (p?.alerts.updatable ?? 0),
  ));
}

/* Conflict.status 在契约里是自由字符串（api/mods.ts 的 conflictSchema），
   后端 resolve/ignore 会写成 'resolved' / 'ignored'，其余一律算待解决。
   不要写成 status === 'pending' —— 那个值后端未必用。 */
const SETTLED = new Set(['resolved', 'ignored']);

export function PackSummaryProvider({children}: {children: ReactNode}) {
  const {id} = useParams();
  const packId = id ?? null;
  const [pack, setPack] = useState<DashboardPack | null>(null);
  const [health, setHealth] = useState<PackHealth | null>(null);
  const [locks, setLocks] = useState<Lock[]>([]);
  const [pending, setPending] = useState<Conflict[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!packId) { setPack(null); setHealth(null); setLocks([]); setPending([]); return; }
    setLoading(true);
    try {
      const [dash, h, l, c] = await Promise.all([
        fetchDashboard(), packHealth(packId), listLocks(packId), listConflicts(packId),
      ]);
      setPack(dash.packs.find(p => p.id === packId) ?? null);
      setHealth(h); setLocks(l);
      setPending(c.filter(x => !SETTLED.has(x.status)));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [packId]);

  useEffect(() => { void load(); }, [load]);

  /* 任务轮询固定 5s。放在 context 而不是页面，这样切态/切页都不会丢进度
     （设计文档 §5-6）。fetchTasks 是全局的，按 packId 过滤在消费方做。 */
  useEffect(() => {
    const tick = async () => {
      try { setTasks(await fetchTasks()); } catch { /* 轮询失败不打扰用户，下一轮再试 */ }
    };
    void tick();
    const t = window.setInterval(tick, 5000);
    return () => window.clearInterval(t);
  }, []);

  const value = useMemo<PackSummary>(() => ({
    packId, pack, health, locks, pendingConflicts: pending, tasks,
    score: computeScore(health, pack),
    alertCount: (health?.pendingErrors ?? 0) + (pack?.alerts.crashes ?? 0),
    loading, error, refresh: () => void load(),
  }), [packId, pack, health, locks, pending, tasks, loading, error, load]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
