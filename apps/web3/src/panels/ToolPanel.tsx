import {useCallback, useRef} from 'react';
import {useUrlState} from '../app/url';
import {ActivityFooter} from './ActivityFooter';
import {SourcesPanel} from './SourcesPanel';
import {BuildPanel} from './BuildPanel';
import {RunPanel} from './RunPanel';
import './panels.css';

/* 工具面板列（第五轮反馈）：面板自带顶栏（来源的 🔍/➕ 动作在顶栏右对齐）。
   一次只展开一个工具（V3 §1 铁律），拖右缘调宽 260~480。无包时不渲染整列。 */
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
      {tool === 'sources' && <SourcesPanel/>}
      {tool === 'build' && <BuildPanel/>}
      {tool === 'run' && <RunPanel/>}
      <ActivityFooter/>
      <div className="tp-resizer" onMouseDown={onMouseDown} role="separator" aria-orientation="vertical"/>
    </aside>
  );
}
