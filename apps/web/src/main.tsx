import {ConfigProvider, App as AntApp} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom';
import {AppShell} from './app/AppShell';
import {DashboardPage} from './pages/DashboardPage';
import {ContentEditorPage, DependenciesPage, PackModsPage, PackWorkbenchPage, PacksPage, PublishPage, QuestEditorPage, SettingsPage} from './pages/PackPages';
import {listPacks} from './features/pack/api';
import './features/dashboard/dashboard.css';
import './ui/workbench/workbench.css';
import './pages/pack-pages.css';

/* 无包上下文的入口路由(如侧栏直达 /mods)重定向到第一个真实整合包;
   一个包都没有时回到包列表页引导创建,绝不跳转到不存在的假包。 */
function DefaultPackRedirect({suffix}: {suffix: string}) {
  const [to, setTo] = useState<string | null>(null);
  useEffect(() => {
    void listPacks()
      .then(ps => setTo(ps[0] ? `/packs/${ps[0].id}${suffix}` : '/packs'))
      .catch(() => setTo('/packs'));
  }, [suffix]);
  return to ? <Navigate replace to={to}/> : null;
}

createRoot(document.getElementById('root')!).render(
  <ConfigProvider locale={zhCN} theme={{
    token: {
      colorPrimary: '#C9783B',
      colorInfo: '#4E8C86',
      colorLink: '#B96335',
      borderRadius: 10,
      colorBgLayout: '#F4F0E8',
      colorText: '#252522',
      colorTextSecondary: '#77736B',
    },
    components: {
      Button: {borderRadius: 9, controlHeight: 36, primaryShadow: '0 4px 12px rgba(91, 63, 39, 0.18)'},
      Progress: {defaultColor: '#C9783B'},
      Switch: {colorPrimary: '#C9783B', colorPrimaryHover: '#B96335'},
    },
  }}>
    <AntApp>
      <BrowserRouter>
        <Routes>
          <Route element={<AppShell/>}>
            <Route path="/" element={<DashboardPage/>}/>
            <Route path="/welcome" element={<DashboardPage forceEmpty/>}/>
            <Route path="/packs" element={<PacksPage/>}/>
            <Route path="/packs/:id" element={<PackWorkbenchPage/>}/>
            <Route path="/packs/:id/mods" element={<PackModsPage/>}/>
            <Route path="/packs/:id/dependencies" element={<DependenciesPage/>}/>
            <Route path="/packs/:id/content" element={<ContentEditorPage/>}/>
            <Route path="/packs/:id/quests" element={<QuestEditorPage/>}/>
            <Route path="/packs/:id/publish" element={<PublishPage/>}/>
            <Route path="/settings" element={<SettingsPage/>}/>
            <Route path="/mods" element={<DefaultPackRedirect suffix="/mods"/>}/>
            <Route path="/content" element={<DefaultPackRedirect suffix="/content"/>}/>
            <Route path="/quests" element={<DefaultPackRedirect suffix="/quests"/>}/>
            <Route path="/publish" element={<DefaultPackRedirect suffix="/publish"/>}/>
            <Route path="*" element={<Navigate replace to="/"/>}/>
          </Route>
        </Routes>
      </BrowserRouter>
    </AntApp>
  </ConfigProvider>,
);
