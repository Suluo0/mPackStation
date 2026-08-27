import {Checkbox} from 'antd';
import {ArrowRightOutlined, CompassOutlined, ExperimentOutlined, ImportOutlined, PlusOutlined, PlusSquareOutlined, RocketOutlined, ThunderboltOutlined} from '@ant-design/icons';
import type {Onboarding} from './types';
import {WorkbenchCard, WorkbenchButton} from '../../ui/workbench/Workbench';

/* 空态 · 迎新布局：Hero + 三入口卡 + 四步流程条 + 上手清单。 */

const checklist = [
  {key: 'curseforgeKey' as const, title: '配置 CurseForge API Key', desc: '完成后可搜索 CurseForge 模组', action: '去设置'},
  {key: 'firstPack' as const, title: '创建或导入第一个整合包', desc: '开始你的整合包设计之旅', action: null},
  {key: 'firstMod' as const, title: '添加第一个模组', desc: '搜索并添加第一个模组到包中', action: null},
];

const starterPacks = [
  {icon: <ThunderboltOutlined/>, tone: 'ember', title: '轻量生存', desc: '从探索、建造和 QoL 模组开始', tags: ['低门槛', '1.20.1']},
  {icon: <ExperimentOutlined/>, tone: 'teal', title: '科技自动化', desc: '让资源生产和机器链条成为主线', tags: ['自动化', 'NeoForge']},
  {icon: <RocketOutlined/>, tone: 'ochre', title: '冒险与任务', desc: '用结构、战斗和任务书组织旅程', tags: ['任务驱动', 'Fabric']},
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
      <section className="db-hero db-hero-stage">
        <div className="db-hero-copy">
          <span className="db-eyebrow">整合包工作台 / 01</span>
          <h1 className="db-hero-title">从一个想法，开始你的整合包</h1>
          <p className="db-hero-sub">搜索模组、锁定版本、编排内容，再一键打包。所有设计都在这里完成，不必反复启动游戏验证。</p>
          <div className="db-hero-actions">
            <WorkbenchButton tone="primary" size="large" icon={<PlusOutlined/>} onClick={onCreate}>新建第一个整合包</WorkbenchButton>
            <button className="db-hero-text-action" onClick={onDemo}>先看看示例 <ArrowRightOutlined /></button>
          </div>
          <div className="db-hero-proof"><span>✦</span> 依赖自动锁定&nbsp;&nbsp;·&nbsp;&nbsp;冲突实时提示&nbsp;&nbsp;·&nbsp;&nbsp;内容在线编辑</div>
        </div>
        <div className="db-hero-art" aria-label="整合包工作台预览">
          <div className="db-art-glow" />
          <div className="db-art-window">
            <div className="db-art-window-top"><span /><span /><span /><b>新整合包 / 工作台</b></div>
            <div className="db-art-window-body">
              <div className="db-art-cover"><div className="db-art-cover-sun" /><div className="db-art-cover-mountain" /><strong>WILD<br />FRONTIER</strong><small>1.20.1 · Fabric</small></div>
              <div className="db-art-panel">
                <span className="db-art-kicker">正在设计</span>
                <strong>轻量生存</strong>
                <div className="db-art-bar"><i /></div>
                <small>已选择 24 个模组</small>
                <div className="db-art-chips"><em>探索</em><em>建造</em><em>QoL</em></div>
              </div>
            </div>
          </div>
          <div className="db-art-float db-art-float-top"><span>✓</span> 依赖已锁定</div>
          <div className="db-art-float db-art-float-bottom"><span>24</span> 个模组正在设计</div>
        </div>
      </section>

      <div className="db-onboard-grid db-onboard-grid-redraw">
        <div className="db-entry-row db-entry-row-redraw">
          <WorkbenchCard className="db-entry-card db-entry-card-feature" onClick={onCreate}>
            <span className="db-entry-icon db-entry-icon-create"><PlusSquareOutlined/></span>
            <span className="db-h2">新建整合包</span>
            <span className="db-muted">从版本和加载器开始，边搜边加，逐步搭出自己的玩法。</span>
            <WorkbenchButton tone="primary">开始创建 <ArrowRightOutlined/></WorkbenchButton>
          </WorkbenchCard>
          <WorkbenchCard className="db-entry-card db-entry-card-compact" onClick={onImport}>
            <span className="db-entry-icon db-entry-icon-import"><ImportOutlined/></span>
            <span className="db-h2">导入现有整合包</span>
            <span className="db-muted">从 CurseForge、Modrinth 或本地 zip 接着改。</span>
            <WorkbenchButton tone="secondary" className="mc-btn-blue">导入 <ArrowRightOutlined/></WorkbenchButton>
          </WorkbenchCard>
          <WorkbenchCard className="db-entry-card db-entry-card-compact" onClick={onDemo}>
            <span className="db-entry-icon db-entry-icon-demo"><CompassOutlined/></span>
            <span className="db-h2">浏览示例包</span>
            <span className="db-muted">看看一个设计完成的整合包如何组织。</span>
            <WorkbenchButton tone="quiet" className="mc-btn-green">查看 <ArrowRightOutlined/></WorkbenchButton>
          </WorkbenchCard>
        </div>
        {onboarding && !stepsDone && (
          <WorkbenchCard className="db-checklist">
            <span className="db-eyebrow">准备工作 / 03</span>
            <h2 className="db-h2" style={{margin: '6px 0 4px'}}>上手三步</h2>
            <p className="db-muted" style={{margin: '0 0 10px'}}>完成这些设置，工作台就绪。</p>
            {checklist.map((item, i) => {
              const done = onboarding.steps[item.key];
              return (
                <div key={item.key} className={done ? 'db-check-item db-check-item-done' : 'db-check-item'}>
                  <Checkbox checked={done} disabled/>
                  <div style={{flex: 1}}><div className="db-check-text">{i + 1} {item.title}</div><div className="db-check-desc">{item.desc}</div></div>
                  {!done && item.action && <button className="db-link" onClick={onSettings}>{item.action}</button>}
                </div>
              );
            })}
          </WorkbenchCard>
        )}
      </div>

      <section className="db-starter-section" aria-labelledby="starter-title">
        <div className="db-section-head">
          <div>
            <h2 id="starter-title" className="db-h2">从一个方向开始</h2>
            <p className="db-muted db-section-kicker">不知道先做什么？这些是适合第一次制作的起点。</p>
          </div>
          <span className="db-muted">灵感参考</span>
        </div>
        <div className="db-starter-grid">
          {starterPacks.map(pack => (
            <article key={pack.title} className="db-starter-card">
              <div className={`db-starter-mark db-starter-mark-${pack.tone}`}>{pack.icon}</div>
              <div className="db-starter-copy">
                <h3>{pack.title}</h3>
                <p>{pack.desc}</p>
                <div className="db-starter-tags">{pack.tags.map(tag => <span key={tag}>{tag}</span>)}</div>
              </div>
              <ArrowRightOutlined className="db-starter-arrow" />
            </article>
          ))}
        </div>
      </section>

      <section className="db-capability-strip" aria-labelledby="capability-title">
        <div className="db-capability-intro">
          <span className="db-eyebrow">工作台流程</span>
          <h2 id="capability-title" className="db-h2">把想法变成可玩的整合包</h2>
          <p className="db-muted">每一步都在同一个包里完成，过程清楚，结果可控。</p>
        </div>
        <div className="db-capability-list">
          {['选模组','锁版本','改内容','出整包'].map((title, i) => (
            <div key={title} className="db-capability-item">
              <span>{`0${i + 1}`}</span>
              <div><strong>{title}</strong><p>{['按包的版本和加载器搜索','自动锁定依赖与兼容范围','在线编辑配方、结构和任务','校验完成后直接打包发布'][i]}</p></div>
            </div>
          ))}
        </div>
      </section>

    </>
  );
}

