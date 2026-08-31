import {z} from 'zod';
import {del, get, post, putVoid} from './http';

/* 系统域:平台健康/状态、MC 版本候选、Prism 便携工具。 */

const providerProbe = z.enum(['unknown', 'ok', 'unavailable']).optional();

export const systemHealthSchema = z.object({
  curseforgeKeyConfigured: z.boolean(),
  modrinthReachable: z.boolean(),
  curseforgeReachable: z.boolean(),
  modrinthStatus: providerProbe,
  curseforgeStatus: providerProbe,
  storageWritable: z.boolean(),
  storageFreeBytes: z.number(),
});
export type SystemHealth = z.infer<typeof systemHealthSchema>;

export const systemStatusSchema = z.object({
  modrinthReachable: z.boolean(),
  curseforgeReachable: z.boolean(),
  modrinthStatus: providerProbe,
  curseforgeStatus: providerProbe,
  cacheSizeBytes: z.number(),
  storageFreeBytes: z.number(),
});
export type SystemStatus = z.infer<typeof systemStatusSchema>;

export function fetchHealth(): Promise<SystemHealth> {
  return get('/api/system/health', systemHealthSchema);
}

export function fetchStatus(): Promise<SystemStatus> {
  return get('/api/system/status', systemStatusSchema);
}

export function fetchMcVersions(): Promise<string[]> {
  return get('/api/meta/mc-versions', z.array(z.string()));
}

/* 后台异步把 Prism 便携安装提交为任务;成败以任务日志为准。 */
export function installPrism(): Promise<{started: boolean; taskId?: string}> {
  return post('/api/tools/prism/install', {}, z.object({started: z.boolean(), taskId: z.string().optional()}));
}

/* 唤起 Prism GUI(便携 -d 目录)让用户登录微软账号;登录完成由后端检测 accounts.json 自动打勾。 */
export function launchPrismLogin(): Promise<{launched: boolean}> {
  return post('/api/tools/prism/login', {}, z.object({launched: z.boolean()}));
}

/* CurseForge key 管理:保存前后端会用真实平台调用验证 key 有效性,
   保存/清除立即生效无需重启。key 值任何读取接口都不会回传。 */
export function saveCurseForgeKey(key: string): Promise<void> {
  return putVoid('/api/system/providers/curseforge/key', {key});
}

export function clearCurseForgeKey(): Promise<void> {
  return del('/api/system/providers/curseforge/key');
}
