import {useState} from 'react';
import {NavLink, Outlet} from 'react-router-dom';
import {
  AppstoreOutlined, BellOutlined, BookOutlined, CodeSandboxOutlined, DoubleLeftOutlined,
  DownOutlined, EditOutlined, HomeOutlined, QuestionCircleOutlined, RocketOutlined,
  SearchOutlined, SettingOutlined,
} from '@ant-design/icons';
import './shell.css';

/* 应用外壳：左侧导航 + 顶栏 + 内容区，所有页面共享。 */

const navItems = [
  {to: '/', label: '工作台', icon: <HomeOutlined/>, end: true},
  {to: '/packs', label: '整合包', icon: <AppstoreOutlined/>},
  {to: '/mods', label: '模组搜索', icon: <SearchOutlined/>},
  {to: '/content', label: '内容编辑', icon: <EditOutlined/>},
  {to: '/quests', label: '任务书', icon: <BookOutlined/>},
  {to: '/publish', label: '打包与发布', icon: <RocketOutlined/>},
  {to: '/settings', label: '设置', icon: <SettingOutlined/>},
];

export function AppShell() {
  const [collapsed, setCollapsed] = useState(false);
  return (
    <div className={collapsed ? 'app-shell app-shell-collapsed' : 'app-shell'}>
      <aside className="app-sider">
        <div className="app-brand">
          <span className="app-brand-logo"><CodeSandboxOutlined/></span>
          <span className="app-brand-name">整合包工作台</span>
        </div>
        <nav className="app-nav">
          {navItems.map(item => (
            <NavLink key={item.to} to={item.to} end={item.end} title={item.label}
                     className={({isActive}) => (isActive ? 'app-nav-item app-nav-item-active' : 'app-nav-item')}>
              <span className="app-nav-icon">{item.icon}</span>
              <span className="app-nav-label">{item.label}</span>
            </NavLink>
          ))}
        </nav>
        <button type="button" className="app-sider-fold" onClick={() => setCollapsed(v => !v)}>
          <DoubleLeftOutlined style={collapsed ? {transform: 'rotate(180deg)'} : undefined}/>
          <span className="app-nav-label">收起侧边栏</span>
        </button>
      </aside>
      <div className="app-body">
        <header className="app-topbar">
          <span className="app-topbar-icon" title="帮助"><QuestionCircleOutlined/></span>
          <span className="app-topbar-icon" title="通知"><BellOutlined/></span>
          <span className="app-user">
            <span className="app-avatar">S</span>
            <span className="app-user-name">Steve·tx</span>
            <DownOutlined style={{fontSize: 10}}/>
          </span>
        </header>
        <main className="app-content">
          <Outlet/>
        </main>
      </div>
    </div>
  );
}

/* 未实现模块的占位页：仅保证导航可达。 */
export function ModulePlaceholder({title}: {title: string}) {
  return (
    <div className="db-card" style={{padding: 24}}>
      <p className="db-h2">{title}</p>
      <p className="db-muted" style={{marginTop: 8}}>该模块将在后续里程碑实现。</p>
    </div>
  );
}
