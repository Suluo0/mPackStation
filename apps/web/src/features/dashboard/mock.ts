import type {DashboardData, DashboardTask, DashboardActivity, SystemHealth, SystemStatus, Onboarding} from './types';

/* 看板 mock 数据 —— 后端未就绪期间的唯一数据源。
   场景通过 URL 参数切换：?mock=empty（空态）/ ?mock=populated（有包态，默认）。 */

export type MockScenario = 'empty' | 'populated';

export function currentScenario(): MockScenario {
  return new URLSearchParams(window.location.search).get('mock') === 'empty' ? 'empty' : 'populated';
}

const hoursAgo = (h: number) => new Date(Date.now() - h * 3600_000).toISOString();

const populated: DashboardData = {
  lastEditedPackId: 'pack-adventure',
  todayResolvedCount: 7,
  packs: [
    {
      id: 'pack-adventure', name: '我的冒险整合包', iconUrl: null,
      mcVersion: '1.20.1', loader: 'neoforge', packVersion: '1.2.0',
      modCount: {total: 142, installed: 130, selected: 12},
      conflicts: {resolved: 12, pending: 3},
      edits: {recipes: 8, structures: 2, ores: 1, quests: 15},
      alerts: {crashes: 0, updatable: 5},
      lastEditedAt: hoursAgo(2), createdAt: hoursAgo(24 * 30),
    },
    {
      id: 'pack-tech', name: '科技探索', iconUrl: null,
      mcVersion: '1.19.2', loader: 'fabric', packVersion: '0.9.3',
      modCount: {total: 98, installed: 90, selected: 8},
      conflicts: {resolved: 20, pending: 0},
      edits: {recipes: 12, structures: 0, ores: 3, quests: 0},
      alerts: {crashes: 0, updatable: 2},
      lastEditedAt: hoursAgo(26), createdAt: hoursAgo(24 * 60),
    },
    {
      id: 'pack-magic', name: '魔法世界', iconUrl: null,
      mcVersion: '1.20.1', loader: 'forge', packVersion: '1.0.0',
      modCount: {total: 156, installed: 150, selected: 6},
      conflicts: {resolved: 8, pending: 5},
      edits: {recipes: 0, structures: 0, ores: 0, quests: 22},
      alerts: {crashes: 2, updatable: 0},
      lastEditedAt: hoursAgo(24 * 3), createdAt: hoursAgo(24 * 45),
    },
    {
      id: 'pack-survive', name: '生存挑战', iconUrl: null,
      mcVersion: '1.18.2', loader: 'fabric', packVersion: '0.8.1',
      modCount: {total: 76, installed: 70, selected: 6},
      conflicts: {resolved: 15, pending: 1},
      edits: {recipes: 4, structures: 1, ores: 0, quests: 9},
      alerts: {crashes: 0, updatable: 1},
      lastEditedAt: hoursAgo(24 * 5), createdAt: hoursAgo(24 * 90),
    },
    {
      id: 'pack-build', name: '建筑大师', iconUrl: null,
      mcVersion: '1.20.1', loader: 'neoforge', packVersion: '1.1.0',
      modCount: {total: 205, installed: 190, selected: 15},
      conflicts: {resolved: 25, pending: 0},
      edits: {recipes: 18, structures: 6, ores: 2, quests: 30},
      alerts: {crashes: 0, updatable: 3},
      lastEditedAt: hoursAgo(24 * 7), createdAt: hoursAgo(24 * 120),
    },
  ],
};

const empty: DashboardData = {packs: [], lastEditedPackId: null, todayResolvedCount: 0};

export function mockDashboard(): DashboardData {
  return currentScenario() === 'empty' ? empty : populated;
}

export function mockTasks(): DashboardTask[] {
  if (currentScenario() === 'empty') return [];
  return [
    {
      id: 'task-1', type: 'build-pack', title: '为「我的冒险整合包」打包',
      packId: 'pack-adventure', packName: '我的冒险整合包',
      status: 'running', progress: 62, error: null,
      startedAt: hoursAgo(0.2), finishedAt: null,
    },
    {
      id: 'task-2', type: 'index-mod', title: '为「科技探索」索引模组数据层（387/892）',
      packId: 'pack-tech', packName: '科技探索',
      status: 'running', progress: 43, error: null,
      startedAt: hoursAgo(0.5), finishedAt: null,
    },
    {
      id: 'task-3', type: 'update-preflight', title: '为「生存挑战」执行模组更新预演',
      packId: 'pack-survive', packName: '生存挑战',
      status: 'running', progress: 22, error: null,
      startedAt: hoursAgo(0.8), finishedAt: null,
    },
    {
      id: 'task-4', type: 'import-pack', title: '导入「魔法世界」',
      packId: 'pack-magic', packName: '魔法世界',
      status: 'failed', progress: 81, error: 'zip 中 manifest.json 缺少 minecraft 字段',
      startedAt: hoursAgo(30), finishedAt: hoursAgo(29.8),
    },
  ];
}

export function mockActivities(): DashboardActivity[] {
  if (currentScenario() === 'empty') return [];
  return [
    {id: 'a1', kind: 'resolve', text: '自动解决了「我的冒险整合包」的 2 个配方冲突', packId: 'pack-adventure', at: hoursAgo(0.4)},
    {id: 'a2', kind: 'add-mod', text: '向「我的冒险整合包」添加了 JEI', packId: 'pack-adventure', at: hoursAgo(0.6)},
    {id: 'a3', kind: 'build', text: '打包「科技探索」v0.9.3 成功', packId: 'pack-tech', at: hoursAgo(5)},
    {id: 'a4', kind: 'alert', text: '「魔法世界」出现 2 条崩溃报告待处理', packId: 'pack-magic', at: hoursAgo(26)},
    {id: 'a5', kind: 'edit', text: '修改了「生存挑战」的 3 条配方', packId: 'pack-survive', at: hoursAgo(24 * 4)},
    {id: 'a6', kind: 'import', text: '从 CurseForge 导入了「建筑大师」', packId: 'pack-build', at: hoursAgo(24 * 7)},
  ];
}

export function mockHealth(): SystemHealth {
  return {
    curseforgeKeyConfigured: false,
    modrinthReachable: true,
    curseforgeReachable: true,
    storageWritable: true,
    storageFreeBytes: 128 * 1024 ** 3,
  };
}

export function mockStatus(): SystemStatus {
  return {
    modrinthReachable: true,
    curseforgeReachable: true,
    cacheSizeBytes: 1.2 * 1024 ** 3,
    storageFreeBytes: 128 * 1024 ** 3,
  };
}

export function mockOnboarding(): Onboarding {
  const done = currentScenario() !== 'empty';
  return {steps: {curseforgeKey: false, firstPack: done, firstMod: done}};
}

export const mockMcVersions = ['1.21.4', '1.21.1', '1.20.6', '1.20.4', '1.20.1', '1.19.4', '1.19.2', '1.18.2', '1.16.5'];
