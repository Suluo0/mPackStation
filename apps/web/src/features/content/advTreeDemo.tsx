/* 仅用于 headless 自测的独立挂载页，不参与主应用路由。 */
import {createRoot} from 'react-dom/client';
import {AdvancementTreeView} from './AdvancementTreeView';
import {
  fixtureAdvWideFlat,
  groupAdvancementTrees,
  type AdvNode,
} from './advLayout';

const mixed: AdvNode[] = [
  {
    id: 'demo:all/root',
    parentId: null,
    title: '冒险',
    frame: 'task',
    criteria: ['root'],
    description: '冒险树根',
    hasDisplay: true,
    iconItem: 'minecraft:iron_sword',
    display: {
      background: 'minecraft:textures/gui/advancements/backgrounds/adventure.png',
      showToast: true,
      announceToChat: true,
    },
  },
  {
    id: 'demo:all/kill',
    parentId: 'demo:all/root',
    title: '怪物猎人',
    frame: 'goal',
    criteria: ['killed'],
    description: '击杀敌对生物',
    hasDisplay: true,
    requirements: [['killed'], ['killed_by'], ['player_killed']],
    rewards: {experience: 100, recipes: ['demo:crossbow']},
  },
  {
    id: 'demo:all/boss',
    parentId: 'demo:all/kill',
    title: '劲弩手',
    frame: 'challenge',
    criteria: ['bow'],
    hasDisplay: true,
    requirements: [['bow', 'arrow', 'enemy']],
    rewards: {experience: 500},
    done: true,
  },
  {
    id: 'demo:all/and_or',
    parentId: 'demo:all/root',
    title: '混合条件',
    frame: 'task',
    hasDisplay: true,
    requirements: [['a', 'b'], ['c']],
    description: '(a 且 b) 或 c',
  },
  {id: 'demo:all/orphan', parentId: 'demo:all/missing_parent', title: '孤儿', frame: 'task', hasDisplay: false},
  {id: 'demo:all/cyc_a', parentId: 'demo:all/cyc_b', title: '环A', hasDisplay: false},
  {id: 'demo:all/cyc_b', parentId: 'demo:all/cyc_a', title: '环B', hasDisplay: false},
  {
    id: 'demo:story/root',
    parentId: null,
    title: '故事',
    frame: 'task',
    hasDisplay: true,
    display: {showToast: true, announceToChat: false, hidden: false},
  },
  {
    id: 'demo:story/mine',
    parentId: 'demo:story/root',
    title: '石器时代',
    frame: 'task',
    criteria: ['mine_stone'],
    hasDisplay: true,
    requirements: [['mine_stone']],
  },
  {
    id: 'demo:recipes/misc/a',
    parentId: null,
    title: '配方解锁A',
    hasDisplay: false,
    isRecipes: true,
  },
  {
    id: 'demo:recipes/misc/b',
    parentId: 'demo:recipes/misc/a',
    title: '配方解锁B',
    hasDisplay: false,
    isRecipes: true,
  },
];

const trees = groupAdvancementTrees(mixed);
const wideTrees = groupAdvancementTrees(fixtureAdvWideFlat(220));

const rootEl = document.getElementById('root');
if (!rootEl) throw new Error('no root');

const mode = new URLSearchParams(location.search).get('mode') ?? 'mixed';

createRoot(rootEl).render(
  <div style={{padding: 16, height: '100vh', boxSizing: 'border-box'}}>
    <h1 style={{font: '600 14px sans-serif'}}>advancement tree demo · mode={mode}</h1>
    <div style={{height: 'calc(100% - 40px)'}}>
      <AdvancementTreeView trees={mode === 'wide' ? wideTrees : trees}/>
    </div>
  </div>,
);
