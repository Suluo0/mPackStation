import {z} from 'zod';

/* 看板数据契约 —— 规格来源：tools/doc/dashboard-page-prompt.md 第 5 节。 */

export const loaderEnum = z.enum(['forge', 'neoforge', 'fabric', 'quilt']);
export type Loader = z.infer<typeof loaderEnum>;

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
  lastEditedAt: z.string(),
  createdAt: z.string(),
});
export type DashboardPack = z.infer<typeof dashboardPackSchema>;

export const dashboardSchema = z.object({
  packs: z.array(dashboardPackSchema),
  lastEditedPackId: z.string().nullable(),
  todayResolvedCount: z.number().int(),
});
export type DashboardData = z.infer<typeof dashboardSchema>;

export const taskSchema = z.object({
  id: z.string(),
  type: z.enum(['index-mod', 'build-pack', 'import-pack', 'update-preflight']),
  title: z.string(),
  packId: z.string().nullable(),
  packName: z.string().nullable(),
  status: z.enum(['running', 'success', 'failed', 'cancelled', 'paused']),
  progress: z.number().min(0).max(100),
  error: z.string().nullable(),
  startedAt: z.string(),
  finishedAt: z.string().nullable(),
});
export const taskListSchema = z.array(taskSchema);
export type DashboardTask = z.infer<typeof taskSchema>;

export const activitySchema = z.object({
  id: z.string(),
  kind: z.enum(['add-mod', 'resolve', 'build', 'alert', 'edit', 'import']),
  text: z.string(),
  packId: z.string().nullable(),
  at: z.string(),
});
export const activityListSchema = z.array(activitySchema);
export type DashboardActivity = z.infer<typeof activitySchema>;

export const systemHealthSchema = z.object({
  curseforgeKeyConfigured: z.boolean(),
  modrinthReachable: z.boolean(),
  curseforgeReachable: z.boolean(),
  storageWritable: z.boolean(),
  storageFreeBytes: z.number(),
});
export type SystemHealth = z.infer<typeof systemHealthSchema>;

export const systemStatusSchema = z.object({
  modrinthReachable: z.boolean(),
  curseforgeReachable: z.boolean(),
  cacheSizeBytes: z.number(),
  storageFreeBytes: z.number(),
});
export type SystemStatus = z.infer<typeof systemStatusSchema>;

export const onboardingSchema = z.object({
  steps: z.object({
    curseforgeKey: z.boolean(),
    firstPack: z.boolean(),
    firstMod: z.boolean(),
  }),
});
export type Onboarding = z.infer<typeof onboardingSchema>;

export const mcVersionListSchema = z.array(z.string());

export const packSchema = z.object({id: z.string(), name: z.string()});
export type CreatedPack = z.infer<typeof packSchema>;
