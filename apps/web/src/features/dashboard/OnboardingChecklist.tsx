import {Checkbox} from 'antd';
import type {Onboarding} from './types';
import {WorkbenchCard} from '../../ui/workbench/Workbench';

const checklist = [
  {key: 'curseforgeKey' as const, title: '配置 CurseForge API Key', desc: '完成后可搜索 CurseForge 模组', action: '去设置'},
  {key: 'firstPack' as const, title: '创建或导入第一个整合包', desc: '开始你的整合包设计之旅', action: null},
  {key: 'firstMod' as const, title: '添加第一个模组', desc: '搜索并添加第一个模组到包中', action: null},
];

export function OnboardingChecklist({onboarding, onSettings}: {onboarding: Onboarding | null; onSettings: () => void}) {
  if (!onboarding || Object.values(onboarding.steps).every(Boolean)) return null;

  return (
    <WorkbenchCard className="db-checklist db-checklist-floating" aria-label="上手三步">
      <span className="db-eyebrow">准备工作 / 03</span>
      <h2 className="db-h2 db-checklist-title">上手三步</h2>
      <p className="db-muted db-checklist-intro">完成这些设置，工作台就绪。</p>
      {checklist.map((item, i) => {
        const done = onboarding.steps[item.key];
        return (
          <div key={item.key} className={done ? 'db-check-item db-check-item-done' : 'db-check-item'}>
            <Checkbox checked={done} disabled />
            <div className="db-check-copy"><div className="db-check-text">{i + 1} {item.title}</div><div className="db-check-desc">{item.desc}</div></div>
            {!done && item.action && <button className="db-link" onClick={onSettings}>{item.action}</button>}
          </div>
        );
      })}
    </WorkbenchCard>
  );
}
