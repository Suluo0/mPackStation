import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import {RouterProvider} from 'react-router-dom';
import {router} from './app/router';
import {ThemeProvider} from './app/ThemeContext';
import {initTheme} from './app/theme';
import './styles/tokens.css';
import './styles/themes.css';
import './styles/base.css';
import './app/frame.css';
import './ui/ui.css';

/* 主题必须在渲染前落定：否则首帧会先吃 themes.css 里 :root 的 light 兜底再跳变。
   之后要切主题走 ThemeProvider（它同时负责把令牌喂给 antd），不要直接动 data-theme。 */
initTheme();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <RouterProvider router={router}/>
    </ThemeProvider>
  </StrictMode>,
);
