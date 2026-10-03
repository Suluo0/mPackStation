import {NavLink, useParams} from 'react-router-dom';
import {GLOBAL_NAV, PACK_NAV, packHref} from './nav';
import {usePackSummary} from './PackSummaryContext';
import {TaskPill} from './TaskPill';

/* 顶栏三段（设计文档 §3 顶栏三段表）。导航标签一律来自 nav.ts，禁在这里硬编码 ——
   两处各写一份是上一轮"导航膨胀"的根因。
   左段 = §4 的四个页面入口（工作台 / 概览 / 内容 / 交付），常驻；
   概览/内容/交付 在无包上下文时锚到最近编辑包（anchorPackId），一个包都没有才置灰。 */
export function TopBar({onOpenPalette}: {onOpenPalette: () => void}) {
  const {id} = useParams();
  const {pack, anchorPackId} = usePackSummary();

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
          {PACK_NAV.map((n, i) => anchorPackId ? (
            <NavLink key={n.to} to={packHref(anchorPackId, n.to)} end={i === 0}
              className={({isActive}) => `tb-link${isActive ? ' on' : ''}`}>
              {n.label}
            </NavLink>
          ) : (
            <span key={n.to} className="tb-link off" title="还没有整合包，先去工作台建一个">{n.label}</span>
          ))}
        </nav>
      </div>

      {/* 中段只有面包屑：入口已常驻左段，不再靠"进包后才多出三个 Tab"（设计文档 §3） */}
      {id && pack && (
        <div className="tb-mid">
          <span className="tb-crumb">{pack.name} ›</span>
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
