import {NavLink, useParams} from 'react-router-dom';
import {GLOBAL_NAV, PACK_NAV, packHref} from './nav';
import {usePackSummary} from './PackSummaryContext';
import {TaskPill} from './TaskPill';

/* 顶栏三段（设计文档 §3 顶栏三段表）。导航标签一律来自 nav.ts，禁在这里硬编码 ——
   两处各写一份是上一轮"导航膨胀"的根因。可点项 = 全局 2 + 包内 3 + ⚙。 */
export function TopBar({onOpenPalette}: {onOpenPalette: () => void}) {
  const {id} = useParams();
  const {pack} = usePackSummary();

  return (
    <header className="app-topbar">
      <div className="tb-left">
        <span className="tb-brand">◆ mPackStation</span>
        <nav className="tb-nav">
          {GLOBAL_NAV.map(n => (
            <NavLink key={n.to} to={n.to} end className={({isActive}) => `tb-link${isActive ? ' on' : ''}`}>
              {n.label}
            </NavLink>
          ))}
        </nav>
      </div>

      {/* 无包上下文时整段不渲染，不是渲染成灰色 */}
      {id && pack && (
        <div className="tb-mid">
          <span className="tb-crumb">{pack.name} ›</span>
          <nav className="tb-nav">
            {PACK_NAV.map((n, i) => (
              <NavLink key={n.to} to={packHref(id, n.to)} end={i === 0}
                className={({isActive}) => `tb-link${isActive ? ' on' : ''}`}>
                {n.label}
              </NavLink>
            ))}
          </nav>
        </div>
      )}

      <div className="tb-right">
        <TaskPill/>
        <button type="button" className="tb-btn" onClick={onOpenPalette}>搜索 ⌘K</button>
        <NavLink to="/settings" className={({isActive}) => `tb-link${isActive ? ' on' : ''}`}>⚙</NavLink>
      </div>
    </header>
  );
}
