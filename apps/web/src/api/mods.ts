import {z} from 'zod';
import {del, get, patch, post} from './http';

/* 模组域:已装模组、平台搜索、版本选择、依赖解析/锁/冲突、包健康。 */

const page = <T extends z.ZodTypeAny>(item: T) => z.object({
  items: z.array(item),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
});

export const modSchema = z.object({
  id: z.string(),
  packId: z.string(),
  source: z.string(),
  projectId: z.string().nullable(),
  versionId: z.string().nullable(),
  displayName: z.string(),
  fileName: z.string(),
  sha1: z.string().nullable(),
  status: z.string(),
  required: z.boolean(),
  addedAt: z.iso.datetime(),
  updatedAt: z.iso.datetime(),
});
export type Mod = z.infer<typeof modSchema>;

export const lockSchema = z.object({
  id: z.string(),
  packId: z.string(),
  schemaVersion: z.number().int(),
  snapshot: z.string(),
  sha256: z.string(),
  createdAt: z.iso.datetime(),
});
export type Lock = z.infer<typeof lockSchema>;

export const conflictSchema = z.object({
  id: z.string(),
  packId: z.string(),
  fingerprint: z.string(),
  kind: z.string(),
  severity: z.string(),
  status: z.string(),
  summary: z.string(),
  detailPath: z.string().nullable(),
  detail: z.record(z.string(), z.unknown()).nullable(),
  resolvedAt: z.iso.datetime().nullable(),
});
export type Conflict = z.infer<typeof conflictSchema>;

export const packHealthSchema = z.object({
  packId: z.string(),
  mods: z.number().int(),
  installed: z.number().int(),
  pendingErrors: z.number().int(),
  pendingWarnings: z.number().int(),
  healthy: z.boolean(),
});
export type PackHealth = z.infer<typeof packHealthSchema>;

export const projectSchema = z.object({
  id: z.string(),
  slug: z.string().optional(),
  name: z.string(),
  summary: z.string().optional(),
  iconUrl: z.string().optional(),
  downloads: z.number().optional(),
});
export type Project = z.infer<typeof projectSchema>;

export const modSearchSchema = z.object({
  items: z.array(projectSchema),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
});

export const modVersionSchema = z.object({
  id: z.string(),
  projectId: z.string().optional(),
  name: z.string().optional(),
  versionNumber: z.string().optional(),
  gameVersions: z.array(z.string()).optional(),
  loaders: z.array(z.string()).optional(),
});
export type ModVersion = z.infer<typeof modVersionSchema>;

/* 双平台并行搜索:单平台失败只进 errors 映射,不阻塞另一平台。 */
export const searchAllItemSchema = projectSchema.extend({provider: z.string()});
export type SearchAllItem = z.infer<typeof searchAllItemSchema>;
export const modSearchAllSchema = z.object({
  items: z.array(searchAllItemSchema),
  errors: z.record(z.string(), z.string()).nullable(),
  total: z.number().int(),
  next_cursor: z.string().nullable(),
});

export type ModSearchQuery = Record<string, string | number | undefined>;

function qsOf(query: ModSearchQuery): string {
  const qs = new URLSearchParams();
  Object.entries(query).forEach(([k, v]) => v !== undefined && qs.set(k, String(v)));
  return qs.toString();
}

export const listMods = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/mods`, page(modSchema)).then(v => v.items);
export const searchMods = (packId: string, query: ModSearchQuery) =>
  get(`/api/packs/${encodeURIComponent(packId)}/mod-search?${qsOf(query)}`, modSearchSchema);
export const searchAllMods = (packId: string, query: ModSearchQuery) =>
  get(`/api/packs/${encodeURIComponent(packId)}/mod-search?${qsOf(query)}`, modSearchAllSchema);
export const listModVersions = (packId: string, provider: string, projectId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/mod-versions?provider=${encodeURIComponent(provider)}&projectId=${encodeURIComponent(projectId)}`, page(modVersionSchema)).then(v => v.items);

export const addMod = (packId: string, body: unknown) =>
  post(`/api/packs/${encodeURIComponent(packId)}/mods`, body, modSchema);
export const updateMod = (packId: string, modId: string, body: unknown) =>
  patch(`/api/packs/${encodeURIComponent(packId)}/mods/${encodeURIComponent(modId)}`, body, modSchema);
export const removeMod = (packId: string, modId: string) =>
  del(`/api/packs/${encodeURIComponent(packId)}/mods/${encodeURIComponent(modId)}`);

/* 依赖解析:lock 的完整快照结构由后端版本化演进,前端目前只用状态与计数。 */
export const resolvePack = (packId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/resolve`, {}, z.object({lock: z.unknown(), status: z.string()}));
export const listLocks = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/locks`, page(lockSchema)).then(v => v.items);
export const listConflicts = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/conflicts`, page(conflictSchema)).then(v => v.items);
export const packHealth = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/health`, packHealthSchema);
