import {useCallback, useRef} from 'react';
import {useUrlState} from '../app/url';
import {NavTree} from '../app/NavTree';
import {ActivityFooter} from './ActivityFooter';
import {QuestPanel} from './QuestPanel';
import {StorePanel} from './StorePanel';
import {BuildPanel} from './BuildPanel';
import {RunPanel} from './RunPanel';
import {ContentPanel} from './ContentPanel';
import {ReleasesPanel} from './ReleasesPanel';
import './panels.css';

/* 侧边栏（六区骨架里的第 3 区）：**一块区域**，内容由第 2 区（图标轨）的按钮决定。

   「项目管理」= 包根目录树（根 + 模组管理 / 任务书管理 / 自建内容管理），
   展开分组即内联该类的清单 —— 旧的「来源」面板的内容整块搬进「模组管理」，
   不再单独占一屏。其余按钮各自是自己的面板。

   滚动由各面板自己的 .tp-body 接管，树本身在内容超长时自己滚。
   拖右缘调宽 260~480。无包时不渲染整列。 */
export function ToolPanel({width, onResize}: {width: number; onResize: (w: number) => void}) {
  const {packId, tool} = useUrlState();
  const dragging = useRef(false);

  const onMouseDown = useCallback(() => {
    dragging.current = true;
    const move = (e: MouseEvent) => {
      if (!dragging.current) return;
      onResize(e.clientX - 44); // 减去图标轨宽度
    };
    const up = () => {
      dragging.current = false;
      document.removeEventListener('mousemove', move);
      document.removeEventListener('mouseup', up);
      document.body.style.cursor = '';
    };
    document.addEventListener('mousemove', move);
    document.addEventListener('mouseup', up);
    document.body.style.cursor = 'col-resize';
  }, [onResize]);

  if (!packId) return null;

  return (
    <aside className="tool-panel" style={{width}}>
      {tool === 'sources' && <NavTree/>}
      {tool === 'quest' && <QuestPanel/>}
      {tool === 'store' && <StorePanel/>}
      {tool === 'build' && <BuildPanel/>}
      {tool === 'run' && <RunPanel/>}
      {tool === 'content' && <ContentPanel/>}
      {tool === 'releases' && <ReleasesPanel/>}
      <ActivityFooter/>
      <div className="tp-resizer" onMouseDown={onMouseDown} role="separator" aria-orientation="vertical"/>
    </aside>
  );
}
