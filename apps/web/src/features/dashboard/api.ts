import {get, post} from '../../api/http';
import {
  dashboardSchema, taskListSchema, activityListSchema,
  systemHealthSchema, systemStatusSchema, onboardingSchema,
  mcVersionListSchema, packSchema,
  type DashboardData, type DashboardTask, type DashboardActivity,
  type SystemHealth, type SystemStatus, type Onboarding, type CreatedPack,
} from './types';
import {mockDashboard, mockTasks, mockActivities, mockHealth, mockStatus, mockOnboarding, mockMcVersions} from './mock';

/* 看板适配层 —— 组件只认这里的函数与 zod 类型，永远不直接碰 fetch/原始响应。
   后端未就绪期间 USE_MOCK = true；接真实后端时改为 false 即可，组件零改动。 */

const USE_MOCK = true;

const delay = <T>(value: T, ms = 200): Promise<T> => new Promise(resolve => window.setTimeout(() => resolve(value), ms));

export function fetchDashboard(): Promise<DashboardData> {
  return USE_MOCK ? delay(mockDashboard()) : get('/api/dashboard', dashboardSchema);
}

export function fetchTasks(): Promise<DashboardTask[]> {
  return USE_MOCK ? delay(mockTasks(), 100) : get('/api/tasks?recent=20', taskListSchema);
}

export function fetchActivities(): Promise<DashboardActivity[]> {
  return USE_MOCK ? delay(mockActivities()) : get('/api/activities?limit=10', activityListSchema);
}

export function fetchHealth(): Promise<SystemHealth> {
  return USE_MOCK ? delay(mockHealth(), 80) : get('/api/system/health', systemHealthSchema);
}

export function fetchStatus(): Promise<SystemStatus> {
  return USE_MOCK ? delay(mockStatus()) : get('/api/system/status', systemStatusSchema);
}

export function fetchOnboarding(): Promise<Onboarding> {
  return USE_MOCK ? delay(mockOnboarding(), 80) : get('/api/onboarding', onboardingSchema);
}

export function fetchMcVersions(): Promise<string[]> {
  return USE_MOCK ? delay(mockMcVersions) : get('/api/meta/mc-versions', mcVersionListSchema);
}

export type CreatePackInput = {name: string; mcVersion: string; loader: string; description?: string};

export function createPack(input: CreatePackInput): Promise<CreatedPack> {
  if (USE_MOCK) return delay({id: `pack-${Date.now()}`, name: input.name}, 400);
  return post('/api/packs', input, packSchema);
}

export type ImportPackInput = {source: 'curseforge' | 'modrinth' | 'local'; url?: string; filename?: string};

export function importPack(input: ImportPackInput): Promise<CreatedPack> {
  if (USE_MOCK) return delay({id: `pack-${Date.now()}`, name: input.filename ?? input.url ?? '导入的整合包'}, 400);
  return post('/api/packs/import', input, packSchema);
}
