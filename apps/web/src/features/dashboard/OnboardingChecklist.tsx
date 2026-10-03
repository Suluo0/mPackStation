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
  const [open, setOpen] = useState(false);

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

  const remaining = checklist.filter(item => !onboarding.steps[item.key]).length;

  // 默认收成右下角一枚小胶囊。原来是 position:fixed 右上角 300px 常驻卡片,
  // 正好压在工作台右栏「后台任务」上(截图实测标题被裁成「后台任…」),
  // 而且每个页面都盖住右上角内容。展开状态由用户点出来,不再自动遮挡。
  if (!open) {
    /* 收起态只留一枚圆形计数钮,文字悬停/聚焦时才展开。
       之前常驻的「上手清单」四字让胶囊有 ~130px 宽,正好压住任务书页右侧属性面板的
       「外观 / 形状」两栏(截图实测);悬浮元素 unavoidably 会盖东西,那就把占位压到最小。 */
    return (
      <button
        type="button"
        className="db-checklist-pill"
        title={`上手清单（还有 ${remaining} 步未完成）`}
        aria-label={`展开上手清单,还有 ${remaining} 步未完成`}
        onClick={() => setOpen(true)}
      >
        <span className="db-checklist-pill-dot">{remaining}</span>
        <span className="db-checklist-pill-text">上手清单</span>
      </button>
    );
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
      <div className="db-checklist-head">
        <div>
          <span className="db-eyebrow">准备工作</span>
          <h2 className="db-h2 db-checklist-title">上手清单</h2>
        </div>
        <button type="button" className="db-checklist-close" aria-label="收起上手清单" onClick={() => setOpen(false)}>×</button>
      </div>
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
