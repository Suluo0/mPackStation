import {z} from 'zod';
import {get} from './http';
import {loaderEnum} from './packs';

/* 看板域:工作台聚合读模型与动态流。规格来源:tools/doc/dashboard-page-prompt.md 第 5 节。 */

export const dashboardPackSchema = z.object({
  id: z.string(),
  name: z.string(),
  iconUrl: z.string().nullable(),
  mcVersion: z.string(),
  loader: loaderEnum,
  packVersion: z.string(),
  modCount: z.object({total: z.number().int(), installed: z.number().int(), selected: z.number().int()}),
  conflicts: z.object({resolved: z.number().int(), pending: z.number().int()}),
  edits: z.object({recipes: z.number().int(), structures: z.number().int(), ores: z.number().int(), quests: z.number().int()}),
  alerts: z.object({crashes: z.number().int(), updatable: z.number().int()}),
  lastEditedAt: z.iso.datetime(),
  createdAt: z.iso.datetime(),
});
export type DashboardPack = z.infer<typeof dashboardPackSchema>;

export const dashboardSchema = z.object({
  packs: z.array(dashboardPackSchema),
  lastEditedPackId: z.string().nullable(),
  todayResolvedCount: z.number().int(),
});
export type DashboardData = z.infer<typeof dashboardSchema>;

export const activitySchema = z.object({
  id: z.string(),
  kind: z.enum(['add-mod', 'resolve', 'build', 'alert', 'edit', 'import']),
  text: z.string(),
  packId: z.string().nullable(),
  at: z.iso.datetime(),
});
export type DashboardActivity = z.infer<typeof activitySchema>;

const activityListSchema = z.object({
  items: z.array(activitySchema),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
});

export function fetchDashboard(): Promise<DashboardData> {
  return get('/api/dashboard', dashboardSchema);
}

export function fetchActivities(): Promise<DashboardActivity[]> {
  return get('/api/activities?limit=10', activityListSchema).then(v => v.items);
}
