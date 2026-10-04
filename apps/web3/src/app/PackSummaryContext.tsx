import {createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode} from 'react';
import {packHealth, listLocks, listConflicts, type PackHealth, type Lock, type Conflict} from '../api/mods';
import {fetchTasks, type Task} from '../api/tasks';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';
import {useUrlState} from './url';

/* 迁自 web2 PackSummaryContext，一处适配：packId 从 useParams().id 改为 URL ?pack=。
   健康分公式只此一处：error 级待解决 -8，warning -3，崩溃 -6，可更新 -1，下限 0 上限 100。 */

export type PackSummary = {
  packId: string | null;
  anchorPackId: string | null;
  pack: DashboardPack | null;
  health: PackHealth | null;
  locks: Lock[];
  pendingConflicts: Conflict[];
  tasks: Task[];
  score: number;
  alertCount: number;
  loading: boolean;
  error: string | null;
  refresh: () => void;
};

const Ctx = createContext<PackSummary | null>(null);

export function usePackSummary(): PackSummary {
  const v = useContext(Ctx);
  if (!v) throw new Error('usePackSummary 必须在 PackSummaryProvider 内使用');
  return v;
}

/* 可选版本：给「可以没有它，但不能因此白屏」的面板用（商店这类独立面板）。
   为什么需要：开发时 vite 的 Fast Refresh 在文件同时导出组件和普通函数时会放弃
   热替换、把整棵树 invalidate 重建；重建的一瞬间新旧两份 Context 并存，
   旧组件读到 null 就抛「必须在 Provider 内使用」。那是假错误，但会让整个界面
   崩掉，用户看到的是「商店点不了」。真放错位置时，这里退化成空实现而不是崩。 */
export function usePackSummaryOptional(): PackSummary | null {
  return useContext(Ctx);
}

function computeScore(h: PackHealth | null, p: DashboardPack | null): number {
  return Math.max(0, Math.min(100,
    100
    - (h?.pendingErrors ?? 0) * 8
    - (h?.pendingWarnings ?? 0) * 3
    - (p?.alerts.crashes ?? 0) * 6
    - (p?.alerts.updatable ?? 0),
  ));
}

/* Conflict.status 是自由字符串；resolve/ignore 会写 'resolved'/'ignored'，其余算待解决。 */
const SETTLED = new Set(['resolved', 'ignored']);

export function PackSummaryProvider({children}: {children: ReactNode}) {
  const {packId} = useUrlState();
  const [pack, setPack] = useState<DashboardPack | null>(null);
  const [lastEdited, setLastEdited] = useState<string | null>(null);
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

  /* 无包上下文时取最近编辑包，给 ⌘K/迎新当锚点；包内不额外发请求。 */
  useEffect(() => {
    if (packId) return;
    void fetchDashboard().then(d => setLastEdited(d.lastEditedPackId)).catch(() => setLastEdited(null));
  }, [packId]);

  /* 任务轮询 5s，放 context：切 tab/切面板/开关停靠都不丢进度（v2.1 §5-6 沿用）。 */
  useEffect(() => {
    const tick = async () => {
      try { setTasks(await fetchTasks()); } catch { /* 下一轮再试 */ }
    };
    void tick();
    const t = window.setInterval(tick, 5000);
    return () => window.clearInterval(t);
  }, []);

  const value = useMemo<PackSummary>(() => ({
    packId, anchorPackId: packId ?? lastEdited, pack, health, locks, pendingConflicts: pending, tasks,
    score: computeScore(health, pack),
    alertCount: (health?.pendingErrors ?? 0) + (pack?.alerts.crashes ?? 0),
    loading, error, refresh: () => void load(),
  }), [packId, lastEdited, pack, health, locks, pending, tasks, loading, error, load]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
