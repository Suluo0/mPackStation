import {useEffect, useState} from 'react';
import {NavLink, Outlet, useLocation, useNavigate} from 'react-router-dom';
import {
  AppstoreOutlined, BookOutlined, CodeSandboxOutlined, DatabaseOutlined, DoubleLeftOutlined,
  EditOutlined, FileTextOutlined, FolderOpenOutlined, HomeOutlined, CheckCircleOutlined, RocketOutlined,
  WarningOutlined, SettingOutlined, SearchOutlined, ApartmentOutlined, PlayCircleOutlined,
} from '@ant-design/icons';
import './shell.css';
import {fetchOnboarding, type Onboarding} from '../api/onboarding';
import {listPacks} from '../api/packs';
import {fetchDashboard, type DashboardPack} from '../api/dashboard';
import {OnboardingChecklist} from '../features/dashboard/OnboardingChecklist';

/* 应用外壳：左侧导航 + 顶栏 + 内容区，所有页面共享。 */

const primaryNav = [
  {to: '/', label: '工作台', icon: <HomeOutlined/>, end: true},
  {to: '/packs', label: '整合包', icon: <AppstoreOutlined/>},
];

const packNav = [
  {suffix: '', label: '概览', icon: <HomeOutlined/>, end: true},
  {suffix: '/mods', label: '模组', icon: <SearchOutlined/>},
  {suffix: '/dependencies', label: '依赖与冲突', icon: <ApartmentOutlined/>},
  {suffix: '/content', label: '内容编辑', icon: <EditOutlined/>},
  {suffix: '/quests', label: '任务书', icon: <BookOutlined/>},
  {suffix: '/publish', label: '打包与发布', icon: <RocketOutlined/>},
  {suffix: '/launcher', label: '启动器', icon: <PlayCircleOutlined/>},
];

export function AppShell() {
  const [collapsed, setCollapsed] = useState(false);
  const [onboarding, setOnboarding] = useState<Onboarding | null>(null);
  const refreshOnboarding = () => { void fetchOnboarding().then(setOnboarding).catch(() => setOnboarding(null)); };
  useEffect(refreshOnboarding, []);
  const location = useLocation();
  const navigate = useNavigate();
  const packMatch = location.pathname.match(/^\/packs\/([^/]+)/);
  const packId = packMatch?.[1];
  const inPack = Boolean(packId);
  /* 不在包页面时,侧栏包导航落到第一个真实整合包;没有包则去列表页。 */
  const [firstPackId, setFirstPackId] = useState<string | null>(null);
  useEffect(() => {
    if (packId) return;
    void listPacks().then(ps => setFirstPackId(ps[0]?.id ?? '')).catch(() => setFirstPackId(''));
  }, [packId]);
  const activePackId = packId ?? firstPackId ?? '';
  /* 底边栏状态数据：跟随当前激活包，来自 /api/dashboard 聚合读模型。 */
  const [dashPacks, setDashPacks] = useState<DashboardPack[]>([]);
  useEffect(() => {
    void fetchDashboard().then(d => setDashPacks(d.packs)).catch(() => setDashPacks([]));
  }, [location.pathname]);
  const activePack = dashPacks.find(p => p.id === activePackId) ?? null;
  const modTotal = activePack?.modCount.total ?? 0;
  const modInstalled = activePack?.modCount.installed ?? 0;
  const alertCount = (activePack?.alerts.crashes ?? 0) + (activePack?.alerts.updatable ?? 0);
  /* 顶栏包上下文信息迁入底边栏「索引进度」格（原索引进度内容已删除）。 */
  const path = location.pathname;
  const sectionLabel = (() => {
    if (path === '/settings') return '设置';
    if (!activePackId) return path.startsWith('/packs') ? '整合包列表' : '工作台';
    const hit = packNav.find(n =>
      n.suffix === ''
        ? path === `/packs/${activePackId}` || path === `/packs/${activePackId}/`
        : path.startsWith(`/packs/${activePackId}${n.suffix}`),
    );
    return hit?.label ?? '包工作台';
  })();
  return (
    <div className={collapsed ? 'app-shell app-shell-collapsed' : 'app-shell'}>
      <aside className="app-sider">
        <div className="app-brand">
          <span className="app-brand-logo"><CodeSandboxOutlined/></span>
          <div className="app-brand-copy">
            <span className="app-brand-name">整合包工作台</span>
            <span className="app-brand-sub"><i /> WORKBENCH <i /></span>
          </div>
        </div>
        <NavLink to="/settings" className="app-workbench-settings" title="工作台设置">
          <SettingOutlined/><span>工作台设置</span>
        </NavLink>
        <nav className="app-nav">
          <div className="app-nav-group">
            <span className="app-nav-caption">工作空间</span>
            {primaryNav.map(item => (
            <NavLink key={item.to} to={item.to} end={item.end} title={item.label}
                     className={({isActive}) => (isActive ? 'app-nav-item app-nav-item-active' : 'app-nav-item')}>
              <span className="app-nav-icon">{item.icon}</span>
              <span className="app-nav-label">{item.label}</span>
            </NavLink>
            ))}
          </div>
          <div className="app-nav-group app-nav-pack-group">
            <span className="app-nav-caption">{inPack ? '当前整合包' : '创作工具'}</span>
            <div className="app-nav-pack-name"><span className="app-nav-pack-dot" /> <span className="app-nav-label">包工作台</span></div>
            {packNav.map(item => {
              const to = activePackId ? `/packs/${activePackId}${item.suffix}` : '/packs';
              return <NavLink key={item.suffix} to={to} end={item.end} title={item.label}
                className={({isActive}) => (isActive ? 'app-nav-item app-nav-item-active' : 'app-nav-item')}>
                <span className="app-nav-icon">{item.icon}</span>
                <span className="app-nav-label">{item.label}</span>
              </NavLink>;
            })}
          </div>
        </nav>
        <button type="button" className="app-sider-fold" onClick={() => setCollapsed(v => !v)}>
          <DoubleLeftOutlined style={collapsed ? {transform: 'rotate(180deg)'} : undefined}/>
          <span className="app-nav-label">收起侧边栏</span>
        </button>
      </aside>
      <div className="app-body">
        <main className="app-content">
          <Outlet/>
        </main>
      </div>
      <OnboardingChecklist
        onboarding={onboarding}
        onSettings={() => navigate('/settings')}
        onRefresh={refreshOnboarding}
        onLauncher={() => navigate(activePackId ? `/packs/${activePackId}/launcher` : '/packs')}
      />
      <footer className="app-statusbar">
        <div className="app-status-item"><span className="app-status-icon"><DatabaseOutlined /></span><div><strong>{modTotal} 个模组</strong><small>模组库总数</small></div></div>
        <div className="app-status-item"><span className="app-status-check"><CheckCircleOutlined /></span><div><strong>{modInstalled} 已安装</strong><small>已选择模组</small></div></div>
        <div className="app-status-item app-status-index app-status-pack" data-testid="statusbar-pack-context" title={activePack?.name ?? '未选择整合包'}>
          <span className="app-status-icon app-status-pack-cover" aria-hidden>
            {activePack?.name?.slice(0, 1) ?? '—'}
          </span>
          <div className="app-status-index-body">
            <div className="app-status-index-head">
              <strong className="app-status-pack-name">{activePack?.name ?? '未选择整合包'}</strong>
              <b className="app-status-pack-saved">已保存</b>
            </div>
            <div className="app-status-meta" data-testid="statusbar-pack-meta">
              <span>{activePack ? `MC ${activePack.mcVersion}` : 'MC —'}</span>
              <span className="app-status-pack-loader">{activePack?.loader ?? '—'}</span>
              <span className="tabular">v{activePack?.packVersion ?? '—'}</span>
              <span className="app-status-pack-section">{sectionLabel}</span>
            </div>
          </div>
        </div>
        <div className="app-status-item app-status-alert"><span className="app-status-warn"><WarningOutlined /></span><div><strong>{alertCount > 0 ? `${alertCount} 个告警` : '0 个告警'}</strong><small>{alertCount > 0 ? '存在待处理告警' : '健康状态良好'}</small></div></div>
        <div className="app-status-actions">
          <button type="button" onClick={() => navigate('/')}> <FileTextOutlined /> 查看日志</button>
          <button type="button" onClick={() => navigate('/settings')}> <FolderOpenOutlined /> 输出目录</button>
        </div>
      </footer>
    </div>
  );
}

/* 兼容旧链接的轻量状态页；新的工作流均在包上下文内实现。 */
export function ModulePlaceholder({title}: {title: string}) {
  return (
    <div className="db-card" style={{padding: 24}}>
      <p className="db-h2">{title}</p>
      <p className="db-muted" style={{marginTop: 8}}>该模块将在后续里程碑实现。</p>
    </div>
  );
}
