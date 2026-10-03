import {createBrowserRouter} from 'react-router-dom';
import {AppFrame} from './AppFrame';

/* V3 只有一条路由：所有状态都在查询参数里（§2 URL 契约）。
   设置不是路由，是 ⚙ 打开的弹窗（?settings=1 可深链）。 */
export const router = createBrowserRouter([
  {path: '*', element: <AppFrame/>},
]);
