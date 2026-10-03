import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import {ConfigProvider} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {RouterProvider} from 'react-router-dom';
import {router} from './app/router';
import './styles/tokens.css';
import './styles/base.css';
import './app/frame.css';

/* antd 主题变量引用 tokens.css 同一套值；V3 密排版把圆角降到 4（§5 视觉约定）。
   改 --mc-primary / --mc-radius 时必须同步改下面两个字面值。 */
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider locale={zhCN} theme={{token: {colorPrimary: '#c9783b', borderRadius: 4, fontSize: 13}}}>
      <RouterProvider router={router}/>
    </ConfigProvider>
  </StrictMode>,
);
