import {useState, type ReactNode} from 'react';
import {useUrlState} from './url';
import {usePackSummaryOptional} from './PackSummaryContext';
import {loaderLabel} from '../api/packs';
import {Icon} from '../ui/Icon';
import {NavRootSlot} from './navRootSlot';
import {SourcesPanel} from '../panels/SourcesPanel';
import {QuestPanel} from '../panels/QuestPanel';
import {ContentPanel} from '../panels/ContentPanel';

/* 第 3 区 · 侧边栏的**唯一内容**：包根目录。

   根 = 这个包本身，下面三类内容各一个分组，**展开即内联该类的清单**。

   旧的「来源」面板不再单独占一屏：它的内容整块搬进「模组管理」节点下。
   所以这里只有一块区域 —— 不存在「上面一棵树、下面一个面板」那种两块结构
   （那种结构做过一次，被否了）。

   分组上的计数来自看板的聚合读模型（modCount / edits），不额外发请求。 */
export function NavTree() {
  const {packId} = useUrlState();
  const summary = usePackSummaryOptional();
  const pack = summary?.pack ?? null;
  /* 默认展开「模组管理」：进包第一眼要看到装了什么。
     折叠态是本地 state，故意不进 URL —— 分享链接不该带上谁展开了哪一枝。 */
  const [open, setOpen] = useState<Record<string, boolean>>({mods: true});
  /* 根行右侧的动作插槽：内容面板把它的按钮 portal 进来（navRootSlot.ts）。
     用 state 存节点而不是 ref —— ref 拿到时首次渲染已经过去，面板会错过插槽。 */
  const [slot, setSlot] = useState<HTMLDivElement | null>(null);

  if (!packId) return null;

  const groups: {key: string; label: string; icon: string; count?: string; body: () => ReactNode}[] = [
    {
      key: 'mods', label: '模组管理', icon: 'layers',
      count: pack ? `${pack.modCount.installed}/${pack.modCount.total}` : undefined,
      body: () => <SourcesPanel/>,
    },
    {
      key: 'quest', label: '任务书管理', icon: 'quest',
      count: pack ? String(pack.edits.quests) : undefined,
      body: () => <QuestPanel/>,
    },
    {
      key: 'content', label: '自建内容管理', icon: 'wrench',
      count: pack ? String(pack.edits.recipes + pack.edits.structures + pack.edits.ores) : undefined,
      body: () => <ContentPanel/>,
    },
  ];

  return (
    <div className="nav-tree">
      <div className="nav-root">
        <Icon name="sources" size={13}/>
        <span className="grow">{pack?.name ?? '当前包'}</span>
        {pack && <span className="nav-meta">{pack.mcVersion} · {loaderLabel(pack.loader)}</span>}
        {/* 当前分组的面板把自己的动作按钮送到这里 —— 按钮跟着包名走，
            不再悬在面板顶上变成孤儿（动作归谁，就摆在那一行）。 */}
        <div className="nav-acts" ref={setSlot}/>
      </div>

      <NavRootSlot.Provider value={slot}>
        {groups.map(g => {
        const on = !!open[g.key];
        return (
          <div key={g.key} className="nav-group">
            <button type="button" className="nav-branch"
              aria-expanded={on} onClick={() => setOpen(v => ({...v, [g.key]: !on}))}>
              <Icon name={on ? 'caretDown' : 'caretRight'} size={12}/>
              <Icon name={g.icon} size={13}/>
              <span className="grow">{g.label}</span>
              {g.count && <span className="nav-count">{g.count}</span>}
            </button>
            {on && <div className="nav-children">{g.body()}</div>}
          </div>
        );})}
      </NavRootSlot.Provider>
    </div>
  );
}
