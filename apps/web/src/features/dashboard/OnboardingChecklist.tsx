import {useEffect, useRef, useState} from 'react';
import {Checkbox, App} from 'antd';
import type {Onboarding} from '../../api/onboarding';
import {launchPrismLogin} from '../../api/system';
import {WorkbenchCard} from '../../ui/workbench/Workbench';

/* 悬浮上手清单(全页面右上角)。第 4 步只负责登录账号:点击唤起 Prism GUI,
   用户在 Prism 里完成微软登录后,后端检测 accounts.json 自动打勾。
   (Prism 的自动安装属于首次启动流程,由任务系统承载,见 installPrism。) */

const checklist = [
  {key: 'curseforgeKey' as const, title: '配置 CurseForge API Key', desc: '完成后可搜索 CurseForge 模组', action: '去设置'},
  {key: 'firstPack' as const, title: '创建或导入第一个整合包', desc: '开始你的整合包设计之旅', action: null},
  {key: 'firstMod' as const, title: '添加第一个模组', desc: '搜索并添加第一个模组到包中', action: null},
  {key: 'prismAccount' as const, title: '登录调试启动器账号', desc: '唤起 Prism 登录一次正版微软账号,完成后自动打勾', action: '去登录'},
];

export function OnboardingChecklist({onboarding, onSettings, onRefresh}: {onboarding: Onboarding | null; onSettings: () => void; onRefresh?: () => void}) {
  const {message} = App.useApp();
  const [launching, setLaunching] = useState(false);
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  // 组件卸载时清掉轮询,避免残留定时器在已卸载页面上继续触发刷新。
  useEffect(() => () => {
    if (pollTimer.current !== null) clearInterval(pollTimer.current);
  }, []);

  if (!onboarding || Object.values(onboarding.steps).every(Boolean)) return null;

  const runLogin = () => {
    setLaunching(true);
    launchPrismLogin()
      .then(() => {
        message.info('Prism 已唤起,请在其中添加微软账号;完成后这里会自动打勾');
        // 轮询直到账号就绪(登录动作在用户侧,留 10 分钟上限)
        if (pollTimer.current !== null) clearInterval(pollTimer.current);
        let tries = 0;
        pollTimer.current = setInterval(() => {
          tries += 1;
          onRefresh?.();
          if (tries >= 120 && pollTimer.current !== null) {
            clearInterval(pollTimer.current);
            pollTimer.current = null;
          }
        }, 5000);
      })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)))
      .finally(() => setLaunching(false));
  };

  return (
    <WorkbenchCard className="db-checklist db-checklist-floating" aria-label="上手四步">
      <span className="db-eyebrow">准备工作 / 04</span>
      <h2 className="db-h2 db-checklist-title">上手四步</h2>
      <p className="db-muted db-checklist-intro">完成这些设置,工作台就绪。</p>
      {checklist.map((item, i) => {
        const done = onboarding.steps[item.key];
        const isPrism = item.key === 'prismAccount';
        return (
          <div key={item.key} className={done ? 'db-check-item db-check-item-done' : 'db-check-item'}>
            <Checkbox checked={done} disabled />
            <div className="db-check-copy"><div className="db-check-text">{i + 1} {item.title}</div><div className="db-check-desc">{item.desc}</div></div>
            {!done && item.action && (
              <button className="db-link" disabled={isPrism && launching}
                onClick={isPrism ? runLogin : onSettings}>
                {isPrism && launching ? '唤起中…' : item.action}
              </button>
            )}
          </div>
        );
      })}
    </WorkbenchCard>
  );
}
