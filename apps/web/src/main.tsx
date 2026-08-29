import {ConfigProvider, App as AntApp} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {createRoot} from 'react-dom/client';
import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom';
import {AppShell} from './app/AppShell';
import {DashboardPage} from './pages/DashboardPage';
import {ContentEditorPage, DependenciesPage, PackModsPage, PackWorkbenchPage, PacksPage, PublishPage, QuestEditorPage, SettingsPage} from './pages/PackPages';
import './features/dashboard/dashboard.css';
import './ui/workbench/workbench.css';
import './pages/pack-pages.css';

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
            <Route path="/mods" element={<Navigate replace to="/packs/tech/mods"/>}/>
            <Route path="/content" element={<Navigate replace to="/packs/tech/content"/>}/>
            <Route path="/quests" element={<Navigate replace to="/packs/tech/quests"/>}/>
            <Route path="/publish" element={<Navigate replace to="/packs/tech/publish"/>}/>
            <Route path="*" element={<Navigate replace to="/"/>}/>
          </Route>
        </Routes>
      </BrowserRouter>
    </AntApp>
  </ConfigProvider>,
);
