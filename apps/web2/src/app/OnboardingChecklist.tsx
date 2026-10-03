import {NavLink} from 'react-router-dom';
import {CHECKLIST, useOnboarding} from './OnboardingContext';
import {packHref} from './nav';
import {usePackSummary} from './PackSummaryContext';

/* 跳转目标：键 → 路径。带 :id 的项在没有任何包时禁用（不许跳到 /packs/undefined）。 */
const NEEDS_PACK = new Set(['firstMod', 'launcherReady']);

export function OnboardingChecklist() {
  const {steps, loading} = useOnboarding();
  const {anchorPackId: id} = usePackSummary();
  if (loading || !steps) return null;              // 未拉到就不渲染，禁默认打勾
  const done = CHECKLIST.filter(s => steps[s.key]).length;
  if (done === CHECKLIST.length) return null;      // 全部完成后整个胶囊消失

  /* 目标页：新建/导入在「工作台」§4.1 里（顶栏整合包入口已屏蔽），所以一律先落 /。 */
  const target = (key: string): string | null => {
    if (key === 'firstPack') return '/';
    if (key === 'curseforgeKey') return '/settings';
    if (!id) return NEEDS_PACK.has(key) ? null : '/';
    return key === 'firstMod' ? `${packHref(id, 'content')}?rail=mods` : `${packHref(id, 'delivery')}?step=install`;
  };

  /* 完成态只读后端迎新域，不从包数据反推（设计文档 §4.5 + api/onboarding.ts）。
     打勾由动作真实完成的那一侧调用（§6.1 建包/导包、§6.4 加模组、§6.6 存 Key、§6.5 起窗），
     这里只负责带路。 */
  return (
    <div className="checklist">
      <span className="ck-progress">上手 {done}/{CHECKLIST.length}</span>
      {CHECKLIST.map(s => {
        const to = target(s.key);
        const cls = steps[s.key] ? 'ck-item done' : 'ck-item next';
        return to
          ? <NavLink key={s.key} to={to} className={cls}>{steps[s.key] ? '✓' : '○'} {s.label}</NavLink>
          : <span key={s.key} className={cls}>{steps[s.key] ? '✓' : '○'} {s.label}（先选一个包）</span>;
      })}
    </div>
  );
}
