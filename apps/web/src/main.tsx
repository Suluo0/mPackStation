import {ConfigProvider, App as AntApp} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {createRoot} from 'react-dom/client';
import {BrowserRouter, Navigate, Route, Routes, useParams} from 'react-router-dom';
import {AppShell, ModulePlaceholder} from './app/AppShell';
import {DashboardPage} from './pages/DashboardPage';
import './features/dashboard/dashboard.css';
import './ui/workbench/workbench.css';

/* 整合包工作台占位：页面将在独立里程碑按提示词文档实现，此处仅保证导航可达。 */
function PackWorkbenchPlaceholder() {
  const {id} = useParams();
  return (
    <div className="db-card" style={{padding: 24}}>
      <p className="db-h2">整合包工作台（{id}）</p>
      <p className="db-muted" style={{marginTop: 8}}>该页面将在独立里程碑实现。</p>
    </div>
  );
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
            <Route path="/packs" element={<ModulePlaceholder title="整合包"/>}/>
            <Route path="/packs/:id" element={<PackWorkbenchPlaceholder/>}/>
            <Route path="/mods" element={<ModulePlaceholder title="模组搜索"/>}/>
            <Route path="/content" element={<ModulePlaceholder title="内容编辑"/>}/>
            <Route path="/quests" element={<ModulePlaceholder title="任务书"/>}/>
            <Route path="/publish" element={<ModulePlaceholder title="打包与发布"/>}/>
            <Route path="/settings" element={<ModulePlaceholder title="设置"/>}/>
            <Route path="*" element={<Navigate replace to="/"/>}/>
          </Route>
        </Routes>
      </BrowserRouter>
    </AntApp>
  </ConfigProvider>,
);
