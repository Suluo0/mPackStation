import {useEffect, useState} from 'react';
import {Link} from 'react-router-dom';
import {fetchDashboard, type DashboardData} from '../api/dashboard';
import {fetchHealth, type SystemHealth} from '../api/system';
import {usePackSummary} from '../app/PackSummaryContext';
import {CHECKLIST, useOnboarding} from '../app/OnboardingContext';
import {StubPackList} from '../stubs/StubPackList';
import {StubTasks} from '../stubs/StubTasks';
import {StubActivity} from '../stubs/StubActivity';
import {CreatePackForm, ImportPackForm} from './PacksPage';

/* /：工作台（设计文档 §4.1）。四步流程条与上手清单同源：都读 useOnboarding().steps + CHECKLIST，
   本文件不发 GET /api/onboarding（§7.2 检查 K 要求调用点全站唯一）。 */
export function WorkbenchPage() {
  const [dash, setDash] = useState<DashboardData | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [error, setError] = useState<string | null>(null);
  const {tasks} = usePackSummary();
  const {steps} = useOnboarding();

  useEffect(() => {
    fetchDashboard().then(setDash).catch(e => setError(e instanceof Error ? e.message : String(e)));
    fetchHealth().then(setHealth).catch(() => setHealth(null));
  }, []);

  const warns: string[] = [];
  if (error) warns.push(`数据读取失败：${error}`);
  if (health && !health.storageWritable) warns.push('后端数据目录不可写，产出与缓存会失败');
  if (health && !health.modrinthReachable && !health.curseforgeReachable) warns.push('两个模组平台都不可达');

  const last = dash?.packs.find(p => p.id === dash.lastEditedPackId) ?? null;

  /* 空态必须指向下一个动作（§5-9）：一个包都没有 → 迎新视图，Hero + 两个真表单 + 四步条。 */
  if (dash && dash.packs.length === 0) {
    return (
      <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        <div className="cp-panel">
          <div className="rr-title" style={{fontSize: 18}}>先把第一个整合包放进来</div>
          <div className="rr-line">建一个新包，或从 CurseForge / Modrinth 链接导入现成的包。</div>
          <CreatePackForm/>
          <ImportPackForm/>
        </div>
        <div className="cp-panel">
          <div className="rr-title">上手四步</div>
          {CHECKLIST.map(s => (
            <div key={s.key} className="rr-line">{steps?.[s.key] ? '✓' : '○'} {s.label}</div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      {warns.length > 0 && (
        <div className="cp-panel">{warns.map(w => <div key={w} className="rr-line">{w}</div>)}</div>
      )}
      <div style={{display: 'flex', gap: 12, flexWrap: 'wrap'}}>
        <div className="cp-panel" style={{flex: 1, minWidth: 280}}>
          <div className="rr-title">最近编辑</div>
          {last ? (
            <>
              <div className="rr-line">{last.name} · MC {last.mcVersion} · {last.loader} · {last.modCount.total} 模组</div>
              <div className="rr-line">冲突待解决 {last.conflicts.pending} · 崩溃告警 {last.alerts.crashes}</div>
              <Link className="tb-btn" to={`/packs/${encodeURIComponent(last.id)}`}>打开</Link>
            </>
          ) : <div className="rr-line">还没有编辑记录。从下面的包列表挑一个，或新建一个包。</div>}
        </div>
        <div className="cp-panel" style={{flex: 1, minWidth: 280}}>
          <div className="rr-title">后台任务</div>
          <StubTasks tasks={tasks}/>
        </div>
      </div>
      <div className="cp-panel">
        <div className="rr-title">整合包</div>
        <StubPackList/>
      </div>
      <div className="cp-panel">
        <div className="rr-title">最近动态 · 今日已解决 {dash?.todayResolvedCount ?? 0}</div>
        <StubActivity/>
      </div>
    </div>
  );
}
