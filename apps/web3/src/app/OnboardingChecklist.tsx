import {CHECKLIST, useOnboarding} from './OnboardingContext';
import {usePackSummary} from './PackSummaryContext';
import {useUrlPatch} from './url';

/* 上手清单浮层。完成态只读后端迎新域（禁从包数据反推）；
   跳转目标全部改为 V3 的 URL 参数。 */

export function OnboardingChecklist() {
  const {steps, loading} = useOnboarding();
  const {anchorPackId} = usePackSummary();
  const patch = useUrlPatch();
  if (loading || !steps) return null;
  const done = CHECKLIST.filter(s => steps[s.key]).length;
  if (done === CHECKLIST.length) return null;

  const target = (key: string): (() => void) | null => {
    if (key === 'firstPack') return () => patch({pack: null});
    if (key === 'curseforgeKey') return () => patch({settings: '1'});
    if (key === 'firstMod') return anchorPackId ? () => patch({pack: anchorPackId, tool: 'sources'}) : null;
    if (key === 'launcherReady') return anchorPackId ? () => patch({pack: anchorPackId, tool: 'run'}) : null;
    return null;
  };

  return (
    <div className="checklist">
      <span className="ck-progress">上手 {done}/{CHECKLIST.length}</span>
      {CHECKLIST.map(s => {
        const go = target(s.key);
        const cls = steps[s.key] ? 'ck-item done' : 'ck-item next';
        return go
          ? <button key={s.key} type="button" className={cls} style={{border: 'none', cursor: 'pointer', background: steps[s.key] ? 'none' : undefined}}
              onClick={go}>{steps[s.key] ? '✓' : '○'} {s.label}</button>
          : <span key={s.key} className={cls}>{steps[s.key] ? '✓' : '○'} {s.label}（先选一个包）</span>;
      })}
    </div>
  );
}
