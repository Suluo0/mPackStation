import {usePackSummary} from '../app/PackSummaryContext';

/* 素面叶子：健康区。分数公式只在 PackSummaryContext.computeScore 里，这里只显示。
   第三步换成 HealthPanel（分项扣分说明 + 可点跳转）。 */
export function StubHealth() {
  const {pack, health, pendingConflicts, score, error} = usePackSummary();
  if (error) return <div className="rr-line">{error}</div>;
  if (!pack || !health) return <div className="rr-line">还没有健康数据。</div>;

  return (
    <div>
      <div className="rr-line">健康分 {score}</div>
      <div className="rr-line">
        模组 {health.mods}（已装 {health.installed}）· 待处理错误 {health.pendingErrors} · 警告 {health.pendingWarnings}
      </div>
      <div className="rr-line">
        冲突：待解决 {pack.conflicts.pending} · 已解决 {pack.conflicts.resolved} · 当前未处置 {pendingConflicts.length}
      </div>
      <div className="rr-line">
        告警：崩溃 {pack.alerts.crashes} · 可更新 {pack.alerts.updatable}
      </div>
      <div className="rr-line">
        魔改：配方 {pack.edits.recipes} · 结构 {pack.edits.structures} · 矿脉 {pack.edits.ores} · 任务 {pack.edits.quests}
      </div>
    </div>
  );
}
