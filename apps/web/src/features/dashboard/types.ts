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
  /* 后端只把 index/build/import/resolve 四种 kind 映射成下面这几个值，
     download / publish / cache_gc 等会给出未映射的原值甚至空串。
     这里放宽成 string，避免未知类型让整条任务列表解析失败。 */
  type: z.string(),
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

/* 整合包 —— 对齐后端 service.Pack。 */
export const packSchema = z.object({
  id: z.string(),
  name: z.string(),
  iconUrl: z.string().nullish(),
  mcVersion: z.string(),
  loader: loaderEnum,
  loaderVersion: z.string().nullish(),
  description: z.string().nullish(),
  status: z.string(),
  packVersion: z.string(),
  createdAt: z.string().nullish(),
  updatedAt: z.string().nullish(),
});
export type CreatedPack = z.infer<typeof packSchema>;

/* 导入第一步：解析来源，产出待确认的预览（后端 service.ImportPreview）。 */
export const importSourceEnum = z.enum(['curseforge', 'modrinth', 'local']);
export type ImportSource = z.infer<typeof importSourceEnum>;

export const importPreviewSchema = z.object({
  id: z.string(),
  token: z.string(),
  inputHash: z.string(),
  source: z.string(),
  expiresAt: z.string().nullish(),
  entryCount: z.number().int(),
  packName: z.string().nullish(),
});
export type ImportPreview = z.infer<typeof importPreviewSchema>;

/* 导入第二步：确认后立刻入队，返回任务与包标识；真正的导入在后台任务里跑。
   注意：响应里的 task 字段是原始 task.Task（Go 字段名，无 json tag），
   和这里看板用的 service.Task 不是同一套结构，因此不纳入契约——
   前端只需要 taskId 去轮询 /api/tasks 即可。 */
export const importConfirmSchema = z.object({
  importId: z.string(),
  taskId: z.string(),
  /* Pack is created by the asynchronous worker; it is null at enqueue time. */
  packId: z.string().nullable(),
  reused: z.boolean(),
});
export type ImportConfirm = z.infer<typeof importConfirmSchema>;
