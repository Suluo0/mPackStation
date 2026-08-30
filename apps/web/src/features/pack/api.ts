import {z} from 'zod';
import {del, get, patch, post} from '../../api/http';

const page = <T extends z.ZodTypeAny>(item: T) => z.object({items: z.array(item), next_cursor: z.string().nullable().optional(), total: z.number().int().optional()});
export const packSchema = z.object({id:z.string(), name:z.string(), iconUrl:z.string().nullish(), mcVersion:z.string(), loader:z.string(), loaderVersion:z.string().nullish(), description:z.string().nullish(), status:z.string(), packVersion:z.string(), createdAt:z.string().nullish(), updatedAt:z.string().nullish()});
export type Pack = z.infer<typeof packSchema>;
export const modSchema = z.object({id:z.string(), packId:z.string(), source:z.string(), projectId:z.string().optional(), versionId:z.string().optional(), displayName:z.string(), fileName:z.string(), sha1:z.string().optional(), status:z.string(), required:z.boolean(), addedAt:z.string(), updatedAt:z.string()});
export type Mod = z.infer<typeof modSchema>;
export const lockSchema = z.object({id:z.string(), packId:z.string(), schemaVersion:z.number().int(), snapshot:z.string(), sha256:z.string(), createdAt:z.string()});
export const conflictSchema = z.object({id:z.string(), packId:z.string(), fingerprint:z.string(), kind:z.string(), severity:z.string(), status:z.string(), summary:z.string(), detailPath:z.string().optional(), detail:z.record(z.string(), z.unknown()).optional(), resolvedAt:z.string().optional()});
export const packHealthSchema = z.object({packId:z.string(), mods:z.number().int(), installed:z.number().int(), pendingErrors:z.number().int(), pendingWarnings:z.number().int(), healthy:z.boolean()});
export const projectSchema = z.object({id:z.string(), slug:z.string().optional(), name:z.string(), summary:z.string().optional(), iconUrl:z.string().optional(), downloads:z.number().optional()});
export const modSearchSchema = z.object({items:z.array(projectSchema), nextCursor:z.string().optional(), total:z.number().int()});
export type Project = z.infer<typeof projectSchema>;
export const modVersionSchema = z.object({id:z.string(), projectId:z.string().optional(), name:z.string().optional(), versionNumber:z.string().optional(), gameVersions:z.array(z.string()).optional(), loaders:z.array(z.string()).optional()});
export type ModVersion = z.infer<typeof modVersionSchema>;
export const searchAllItemSchema = projectSchema.extend({provider:z.string()});
export type SearchAllItem = z.infer<typeof searchAllItemSchema>;
export const modSearchAllSchema = z.object({items:z.array(searchAllItemSchema), errors:z.record(z.string(), z.string()).optional(), total:z.number().int()});

export const listPacks = () => get('/api/packs', page(packSchema)).then(v => v.items);
export const getPack = (id:string) => get(`/api/packs/${encodeURIComponent(id)}`, packSchema);
export const updatePack = (id:string, body:unknown) => patch(`/api/packs/${encodeURIComponent(id)}`, body, packSchema);
export const deletePack = (id:string) => del(`/api/packs/${encodeURIComponent(id)}`);
export const listMods = (packId:string) => get(`/api/packs/${encodeURIComponent(packId)}/mods`, page(modSchema)).then(v => v.items);
export const searchMods = (packId:string, query:Record<string,string|number|undefined>) => {
  const qs = new URLSearchParams(); Object.entries(query).forEach(([k,v]) => v !== undefined && qs.set(k, String(v)));
  return get(`/api/packs/${encodeURIComponent(packId)}/mod-search?${qs}`, modSearchSchema);
};
export const listModVersions = (packId:string, provider:string, projectId:string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/mod-versions?provider=${encodeURIComponent(provider)}&projectId=${encodeURIComponent(projectId)}`, z.object({items:z.array(modVersionSchema)})).then(v => v.items);
export const searchAllMods = (packId:string, query:Record<string,string|number|undefined>) => {
  const qs = new URLSearchParams(); Object.entries(query).forEach(([k,v]) => v !== undefined && qs.set(k, String(v)));
  return get(`/api/packs/${encodeURIComponent(packId)}/mod-search?${qs}`, modSearchAllSchema);
};
export const addMod = (packId:string, body:unknown) => post(`/api/packs/${encodeURIComponent(packId)}/mods`, body, modSchema);
export const updateMod = (packId:string, modId:string, body:unknown) => patch(`/api/packs/${encodeURIComponent(packId)}/mods/${encodeURIComponent(modId)}`, body, modSchema);
export const removeMod = (packId:string, modId:string) => del(`/api/packs/${encodeURIComponent(packId)}/mods/${encodeURIComponent(modId)}`);
export const resolvePack = (packId:string) => post(`/api/packs/${encodeURIComponent(packId)}/resolve`, {}, z.object({lock:z.unknown(), status:z.string()}));
export const listLocks = (packId:string) => get(`/api/packs/${encodeURIComponent(packId)}/locks`, page(lockSchema)).then(v => v.items);
export const listConflicts = (packId:string) => get(`/api/packs/${encodeURIComponent(packId)}/conflicts`, page(conflictSchema)).then(v => v.items);
export const packHealth = (packId:string) => get(`/api/packs/${encodeURIComponent(packId)}/health`, packHealthSchema);
