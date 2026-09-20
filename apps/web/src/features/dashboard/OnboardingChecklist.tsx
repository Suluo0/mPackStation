import {useEffect, useState} from 'react';
import {Checkbox, App} from 'antd';
import type {Onboarding} from '../../api/onboarding';
import {acknowledgeOnboarding} from '../../api/onboarding';
import {WorkbenchCard} from '../../ui/workbench/Workbench';

/* 悬浮上手清单(全页面右上角)。
   离线启动优先：自研 mPackLauncher 填写离线用户名即可，不要求正版/Microsoft 登录。
   第 4 步引导去启动台配置游戏目录与离线账号，用户可手动确认完成。 */

type ChecklistItem = {
  key: 'curseforgeKey' | 'firstPack' | 'firstMod' | 'launcherReady';
  title: string;
  desc: string;
  action: null | 'settings' | 'launcher' | 'ack';
};

const checklist: ChecklistItem[] = [
  {key: 'curseforgeKey', title: '配置 CurseForge API Key', desc: '可选；配置后可搜索 CurseForge 模组', action: 'settings'},
  {key: 'firstPack', title: '创建或导入第一个整合包', desc: '开始你的整合包设计之旅', action: null},
  {key: 'firstMod', title: '添加第一个模组', desc: '搜索并添加第一个模组到包中', action: null},
  {key: 'launcherReady', title: '配置启动台（离线即可）', desc: '选游戏目录 + 填离线用户名，无需正版账号', action: 'launcher'},
];

export function OnboardingChecklist({
  onboarding,
  onSettings,
  onRefresh,
  onLauncher,
}: {
  onboarding: Onboarding | null;
  onSettings: () => void;
  onRefresh?: () => void;
  onLauncher?: () => void;
}) {
  const {message} = App.useApp();
  const [busy, setBusy] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    if (!onboarding) return;
    const steps = onboarding.steps;
    const required = [steps.curseforgeKey, steps.firstPack, steps.firstMod, steps.launcherReady];
    if (required.every(Boolean)) setDismissed(true);
  }, [onboarding]);

  if (!onboarding || dismissed) return null;
  if (onboarding.steps.prismAccount && [onboarding.steps.curseforgeKey, onboarding.steps.firstPack, onboarding.steps.firstMod, onboarding.steps.launcherReady].every(Boolean)) {
    return null;
  }

  const ackLauncher = () => {
    setBusy(true);
    acknowledgeOnboarding({launcherReady: true})
      .then(() => {
        message.success('已确认启动台配置（离线启动，无需正版）');
        onRefresh?.();
      })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)))
      .finally(() => setBusy(false));
  };

  return (
    <WorkbenchCard className="db-checklist db-checklist-floating" aria-label="上手清单">
      <span className="db-eyebrow">准备工作</span>
      <h2 className="db-h2 db-checklist-title">上手清单</h2>
      <p className="db-muted db-checklist-intro">离线启动即可，无需正版账号。</p>
      {checklist.map((item, i) => {
        const done = Boolean(onboarding.steps[item.key]);
        return (
          <div key={item.key} className={done ? 'db-check-item db-check-item-done' : 'db-check-item'}>
            <Checkbox checked={done} disabled />
            <div className="db-check-copy">
              <div className="db-check-text">{i + 1} {item.title}</div>
              <div className="db-check-desc">{item.desc}</div>
            </div>
            {!done && item.action === 'settings' && (
              <button type="button" className="db-link" onClick={onSettings}>去设置</button>
            )}
            {!done && item.action === 'launcher' && (
              <span style={{display: 'inline-flex', gap: 8}}>
                <button type="button" className="db-link" onClick={() => (onLauncher ? onLauncher() : onSettings())}>去启动台</button>
                <button type="button" className="db-link" disabled={busy} onClick={ackLauncher}>
                  {busy ? '确认中…' : '我已配置'}
                </button>
              </span>
            )}
          </div>
        );
      })}
    </WorkbenchCard>
  );
}
