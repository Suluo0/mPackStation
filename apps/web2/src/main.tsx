import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import {ConfigProvider} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {RouterProvider} from 'react-router-dom';
import {router} from './app/router';
import './styles/tokens.css';
import './styles/base.css';

/* antd 的主题变量必须引用 tokens.css 的同一套值，否则会出现「antd 组件一个色、
   自定义组件另一个色」。这里只映射主色与圆角，其余走 CSS 变量。
   改 --mc-primary / --mc-radius 时必须同步改下面两个字面值。 */
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={{token: {colorPrimary: '#c9783b', borderRadius: 12}}}>
      <RouterProvider router={router}/>
    </ConfigProvider>
  </StrictMode>,
);
