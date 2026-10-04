import {RAIL_TOOLS, TOOLS, useUrlPatch, useUrlState} from './url';
import {Icon} from '../ui/Icon';

/* 第 2 区 · 侧边按钮栏（六区骨架，见 docs/frontend-refactor/04-align-pack-root.md §8）。

   一个按钮一件事：项目管理 / 新增模组 / 任务书 / Get Version。
   点按钮换的是**第 3 区（侧边栏）的内容**，不是在这条轨道上展开子菜单、也不是
   在侧边栏里再叠一棵树 —— 那两种做法都试过，都被否了。

   自建内容 / 构建 / 运行 是对当前包做一次动作，归顶边栏右侧，不占这条轨道。 */
const LABEL = new Map(TOOLS.map(t => [t.tool, t.label]));

export function IconRail() {
  const {tool} = useUrlState();
  const patch = useUrlPatch();

  return (
    <nav className="icon-rail">
      {RAIL_TOOLS.map(name => (
        <button key={name} type="button"
          className={`rail-btn${tool === name ? ' on' : ''}`}
          title={LABEL.get(name) ?? name} aria-label={LABEL.get(name) ?? name}
          onClick={() => patch({tool: name})}>
          <Icon name={name}/>
        </button>
      ))}
      <div className="rail-spacer"/>
    </nav>
  );
}
