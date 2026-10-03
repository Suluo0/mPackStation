import {useCallback} from 'react';
import {useNavigate} from 'react-router-dom';
import {Button, Empty, Tag, Tooltip} from 'antd';
import {
  CloseOutlined, EditOutlined, ExperimentOutlined, GoldOutlined,
  PartitionOutlined, RightOutlined, TagsOutlined,
} from '@ant-design/icons';
import {useFocus} from './FocusContext';
import {usePackCatalog} from '../pack/PackCatalogContext';
import {RecipeViewer} from '../recipe/RecipeViewer';

/* 常驻右侧 Inspector：读取全局焦点，渲染当前对象的详情 + 跨模块传送入口。
   这是把孤立页面串成工作台的「传送门」—— 在任何页面点一个物品/配方，这里立刻给出
   「看合成 / 魔改 / 去物品页 / 在模组里找」等动作，焦点即上下文。 */

const TYPE_LABEL: Record<string, string> = {
  item: '物品', recipe: '配方', mod: '模组', quest: '任务', tag: '标签',
};

export function InspectorRail({packId}: {packId: string}) {
  const navigate = useNavigate();
  const {focus, focusItem, clear, open, toggle} = useFocus();
  const {itemById, tagById, recipesByOutput, recipesByInput, displayName, iconUrl} = usePackCatalog();

  const go = (suffix: string) => navigate(`/packs/${packId}${suffix}`);
  /* 聚焦某物品并跳到合成器：目标页读焦点后自动居中。 */
  const focusItemAndGo = (itemId: string, suffix: string) => {
    focusItem(itemId, 'Inspector');
    navigate(`/packs/${packId}${suffix}`);
  };

  if (!open) {
    return (
      <button type="button" className="inspector-rail-collapsed" onClick={toggle} title="展开 Inspector">
        <PartitionOutlined/>
        <span className="inspector-rail-vtext">INSPECTOR</span>
      </button>
    );
  }

  const item = focus?.type === 'item' ? itemById.get(focus.id) ?? null : null;
  const tag = focus?.type === 'tag' ? tagById.get(focus.id) ?? null : null;
  const outCount = focus?.type === 'item' ? (recipesByOutput.get(focus.id)?.length ?? 0) : 0;
  const inCount = focus?.type === 'item' ? (recipesByInput.get(focus.id)?.length ?? 0) : 0;

  return (
    <aside className="inspector-rail">
      <div className="inspector-head">
        <span className="inspector-kicker">INSPECTOR</span>
        <span className="inspector-actions">
          <Tooltip title="折叠"><Button size="small" type="text" icon={<RightOutlined/>} onClick={toggle}/></Tooltip>
        </span>
      </div>

      {!focus && (
        <div className="inspector-body inspector-empty">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={<span>还没有焦点。<br/>在物品页或合成器里点一个物品，<br/>这里会显示它的详情与可跳转的动作。</span>}
          />
        </div>
      )}

      {focus && (
        <div className="inspector-body">
          <div className="inspector-type">
            <Tag color="orange">{TYPE_LABEL[focus.type] ?? focus.type}</Tag>
            {focus.from && <span className="inspector-from">来自 {focus.from}</span>}
            <Button size="small" type="text" icon={<CloseOutlined/>} onClick={clear} className="inspector-clear"/>
          </div>

          {focus.type === 'item' && item && (
            <>
              <div className="inspector-hero">
                <span className="icg-icon lg">
                  {item.iconStatus === 'ready'
                    ? <img src={iconUrl(item.id)} alt=""/>
                    : <span className="icg-icon-ph">{item.displayName.slice(0, 1)}</span>}
                </span>
                <div className="inspector-hero-text">
                  <strong>{item.displayName}</strong>
                  <code>{item.id}</code>
                </div>
              </div>
              <div className="inspector-stats">
                <div><span>合成方式</span><b>{outCount}</b></div>
                <div><span>被用于</span><b>{inCount}</b></div>
                <div><span>标签</span><b>{item.tags.length}</b></div>
              </div>
              {item.tags.length > 0 && (
                <div className="inspector-tags">
                  {item.tags.slice(0, 12).map(t => <Tag key={t}>#{t}</Tag>)}
                </div>
              )}
              <div className="inspector-actions-list">
                <Button block icon={<ExperimentOutlined/>} onClick={() => focusItemAndGo(item.id, '/recipes')}>
                  查看合成关系
                </Button>
                <Button block icon={<GoldOutlined/>} onClick={() => go('/items')}>
                  在物品页打开
                </Button>
                <Button block icon={<EditOutlined/>} onClick={() => go('/tweak')}>
                  去魔改
                </Button>
              </div>
            </>
          )}

          {focus.type === 'item' && !item && (
            <div className="inspector-empty"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`未找到物品 ${focus.id}`}/></div>
          )}

          {focus.type === 'tag' && tag && (
            <>
              <div className="inspector-hero">
                <span className="icg-icon lg"><span className="icg-icon-ph"><TagsOutlined/></span></span>
                <div className="inspector-hero-text"><strong>{tag.displayName}</strong><code>#{tag.id}</code></div>
              </div>
              <div className="inspector-stats"><div><span>成员</span><b>{tag.members.length}</b></div><div><span>注册表</span><b>{tag.registry}</b></div></div>
              <div className="inspector-tags">
                {tag.members.slice(0, 20).map(m => (
                  <Tag key={m} className="inspector-member" onClick={() => { focusItemAndGo(m, '/recipes'); }}>{displayName(m)}</Tag>
                ))}
              </div>
            </>
          )}

          {focus.type === 'recipe' && (
            <div className="inspector-recipe">
              <code className="inspector-recipe-id">{focus.id}</code>
              <div className="empty-inline">配方详情请在合成器中查看。</div>
              <Button block icon={<ExperimentOutlined/>} onClick={() => go('/recipes')}>打开合成器</Button>
            </div>
          )}

          {focus.type === 'mod' && (
            <div className="inspector-recipe">
              <strong>{focus.label ?? focus.id}</strong>
              <Button block icon={<ExperimentOutlined/>} onClick={() => go('/content')}>打开模组内容</Button>
            </div>
          )}

          {focus.type === 'quest' && (
            <div className="inspector-recipe">
              <strong>{focus.label ?? focus.id}</strong>
              <Button block onClick={() => go('/quests')}>打开任务书</Button>
            </div>
          )}
        </div>
      )}
    </aside>
  );
}

/* Inspector 内嵌迷你配方预览（供焦点为配方时复用）。 */
export function InspectorRecipePreview({payload}: {payload: unknown}) {
  const {displayName, iconUrl, itemById} = usePackCatalog();
  const {focusItem} = useFocus();
  const getItemIcon = useCallback((k: string): string | null => {
    if (!k || k.startsWith('#')) return null;
    const it = itemById.get(k);
    return it && it.iconStatus === 'ready' ? iconUrl(k) : null;
  }, [itemById, iconUrl]);
  return <RecipeViewer payload={payload} translateKey={displayName} getItemIcon={getItemIcon} onSelect={id => !id.startsWith('#') && focusItem(id, 'Inspector')}/>;
}
