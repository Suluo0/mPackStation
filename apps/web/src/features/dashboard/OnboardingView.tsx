import {Button, Checkbox} from 'antd';
import {ArrowRightOutlined, CompassOutlined, ImportOutlined, PlusOutlined, PlusSquareOutlined} from '@ant-design/icons';
import type {Onboarding} from './types';

/* 空态 · 迎新布局：Hero + 三入口卡 + 四步流程条 + 上手清单。 */

const flowSteps = [
  {name: '选', desc: '搜索并筛选模组'},
  {name: '锁', desc: '版本与依赖锁定'},
  {name: '改', desc: '配方/结构/任务书在线编辑'},
  {name: '装', desc: '一键打包安装'},
];

const checklist = [
  {key: 'curseforgeKey' as const, title: '配置 CurseForge API Key', desc: '完成后可搜索 CurseForge 模组', action: '去设置'},
  {key: 'firstPack' as const, title: '创建或导入第一个整合包', desc: '开始你的整合包设计之旅', action: null},
  {key: 'firstMod' as const, title: '添加第一个模组', desc: '搜索并添加第一个模组到包中', action: null},
];

export function OnboardingView({onboarding, onCreate, onImport, onDemo, onSettings}: {
  onboarding: Onboarding | null;
  onCreate: () => void;
  onImport: () => void;
  onDemo: () => void;
  onSettings: () => void;
}) {
  const stepsDone = onboarding ? Object.values(onboarding.steps).every(Boolean) : false;
  return (
    <>
      <div className="db-hero">
        <h1 className="db-hero-title">从搜索到打包，不启动游戏，完成你的整合包</h1>
        <p className="db-hero-sub">双平台检索 · 依赖自动锁定 · 冲突自动解决 · 配方/结构/任务书在线配置 · 一键打包</p>
        <Button type="primary" size="large" className="mc-btn-cta" icon={<PlusOutlined/>} onClick={onCreate}>新建第一个整合包</Button>
      </div>

      <div className="db-onboard-grid">
        <div style={{display: 'flex', flexDirection: 'column', gap: 'var(--mc-sp-lg)'}}>
          <div className="db-entry-row">
            <div className="db-card db-entry-card" onClick={onCreate}>
              <span className="db-entry-icon db-entry-icon-create"><PlusSquareOutlined/></span>
              <span className="db-h2">新建整合包</span>
              <span className="db-muted">从零开始：选版本、选加载器，边搜边加</span>
              <Button type="primary" className="mc-btn-cta">开始创建 <ArrowRightOutlined/></Button>
            </div>
            <div className="db-card db-entry-card" onClick={onImport}>
              <span className="db-entry-icon db-entry-icon-import"><ImportOutlined/></span>
              <span className="db-h2">导入现有整合包</span>
              <span className="db-muted">从 CurseForge / Modrinth / 本地 zip 导入，接着改</span>
              <Button type="primary" className="mc-btn-blue">导入 <ArrowRightOutlined/></Button>
            </div>
            <div className="db-card db-entry-card" onClick={onDemo}>
              <span className="db-entry-icon db-entry-icon-demo"><CompassOutlined/></span>
              <span className="db-h2">浏览示例包</span>
              <span className="db-muted">看看一个设计完成的整合包长什么样</span>
              <Button type="primary" className="mc-btn-green">查看 <ArrowRightOutlined/></Button>
            </div>
          </div>

          <div className="db-card db-flow">
            {flowSteps.map((step, i) => (
              <div key={step.name} style={{display: 'contents'}}>
                <div className="db-flow-step">
                  <span className={`db-flow-badge db-flow-badge-${i + 1}`}>{i + 1}</span>
                  <div>
                    <div className="db-flow-name">{step.name}</div>
                    <div className="db-muted">{step.desc}</div>
                  </div>
                </div>
                {i < flowSteps.length - 1 && <span className="db-flow-arrow">→</span>}
              </div>
            ))}
          </div>
        </div>

        {onboarding && !stepsDone && (
          <div className="db-card db-checklist">
            <h2 className="db-h2" style={{marginBottom: 'var(--mc-sp-sm)'}}>上手三步</h2>
            {checklist.map((item, i) => {
              const done = onboarding.steps[item.key];
              return (
                <div key={item.key} className={done ? 'db-check-item db-check-item-done' : 'db-check-item'}>
                  <Checkbox checked={done} disabled/>
                  <div style={{flex: 1}}>
                    <div className="db-check-text">{i + 1} {item.title}</div>
                    <div className="db-check-desc">{item.desc}</div>
                  </div>
                  {!done && item.action && <button className="db-link" onClick={onSettings}>{item.action}</button>}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </>
  );
}
