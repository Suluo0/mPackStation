import {createBrowserRouter} from 'react-router-dom';
import {AppShell} from './AppShell';
import {WorkbenchPage} from '../pages/WorkbenchPage';
import {PacksPage} from '../pages/PacksPage';
import {OverviewPage} from '../pages/OverviewPage';
import {ContentPage} from '../pages/ContentPage';
import {DeliveryPage} from '../pages/DeliveryPage';
import {SettingsPage} from '../pages/SettingsPage';

/* v2.1 的导航一共 5 项可点（设计文档 §4.5）：工作台 / 整合包 / 概览 / 内容 / 交付 + ⚙。
   这 6 条就是 web2 的全部顶层路由；内容页四态是 ?mode= 查询参数，不是路由。 */
export const router = createBrowserRouter([
  {
    element: <AppShell/>,
    children: [
      {path: '/', element: <WorkbenchPage/>},
      {path: '/packs', element: <PacksPage/>},
      {path: '/packs/:id', element: <OverviewPage/>},
      {path: '/packs/:id/content', element: <ContentPage/>},
      {path: '/packs/:id/delivery', element: <DeliveryPage/>},
      {path: '/settings', element: <SettingsPage/>},
    ],
  },
]);
