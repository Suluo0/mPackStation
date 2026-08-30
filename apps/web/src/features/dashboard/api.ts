import {del, get, post, put} from '../../api/http';
import {z} from 'zod';
import {
  dashboardSchema, taskListSchema, activityListSchema,
  systemHealthSchema, systemStatusSchema, onboardingSchema,
  mcVersionListSchema, packSchema,
  importPreviewSchema, importConfirmSchema,
  type DashboardData, type DashboardTask, type DashboardActivity,
  type SystemHealth, type SystemStatus, type Onboarding, type CreatedPack,
  type ImportPreview, type ImportConfirm,
} from './types';

/* 看板适配层 —— 组件只认这里的函数与 zod 类型，永远不直接碰 fetch/原始响应。
   全部直连后端；响应不符合契约会在 api/http.ts 里立刻抛错，不会静默降级成假数据。 */

export function fetchDashboard(): Promise<DashboardData> {
  return get('/api/dashboard', dashboardSchema);
}

export function fetchTasks(): Promise<DashboardTask[]> {
  return get('/api/tasks?recent=20', taskListSchema);
}

export function fetchActivities(): Promise<DashboardActivity[]> {
  return get('/api/activities?limit=10', activityListSchema);
}

export function fetchHealth(): Promise<SystemHealth> {
  return get('/api/system/health', systemHealthSchema);
}

export function fetchStatus(): Promise<SystemStatus> {
  return get('/api/system/status', systemStatusSchema);
}

export function fetchOnboarding(): Promise<Onboarding> {
  return get('/api/onboarding', onboardingSchema);
}

export function fetchMcVersions(): Promise<string[]> {
  return get('/api/meta/mc-versions', mcVersionListSchema);
}

/* 迎新步骤打勾：后端接受 {"steps": {"<key>": true}}，回填最新的 onboarding 状态。 */
export function acknowledgeOnboarding(steps: Record<string, boolean>): Promise<Onboarding> {
  return put('/api/onboarding', {steps}, onboardingSchema);
}

export type CreatePackInput = {
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion?: string;
  description?: string;
};

export function createPack(input: CreatePackInput): Promise<CreatedPack> {
  return post('/api/packs', {
    name: input.name,
    mcVersion: input.mcVersion,
    loader: input.loader,
    loaderVersion: input.loaderVersion ?? '',
    description: input.description ?? '',
  }, packSchema);
}

/* 删除返回 204 No Content，成功后由调用方刷新列表。 */
export function deletePack(id: string): Promise<void> {
  return del(`/api/packs/${encodeURIComponent(id)}`);
}

/* ---- 导入：后端是两阶段（先解析出预览，再确认入队） ---- */

export type ImportSource = 'curseforge' | 'modrinth' | 'local';

/* 后端的来源常量是 curseforge_url / modrinth_url / local_zip，
   虽然它也接受 curseforge / modrinth / local 这类别名，这里显式用规范值。 */
const sourcePayload: Record<ImportSource, string> = {
  curseforge: 'curseforge_url',
  modrinth: 'modrinth_url',
  local: 'local_zip',
};

export type InspectImportInput = {
  source: ImportSource;
  url?: string;
  /** 本地 zip 的 base64 内容（不含 data: 前缀） */
  contentBase64?: string;
};

export function inspectImport(input: InspectImportInput): Promise<ImportPreview> {
  return post('/api/packs/import/inspect', {
    source: sourcePayload[input.source],
    url: input.url ?? '',
    content: input.contentBase64 ?? '',
  }, importPreviewSchema);
}

export function confirmImport(preview: ImportPreview, idempotencyKey: string): Promise<ImportConfirm> {
  return post('/api/packs/import', {
    previewId: preview.id,
    token: preview.token,
    inputHash: preview.inputHash,
    idempotencyKey,
  }, importConfirmSchema);
}

/* ---- 任务控制 ----
   这些接口返回的是 task.TaskView（结构与看板列表用的 service.Task 不同），
   所以这里只发请求、不解析响应体，由调用方重新拉取任务列表。 */

function taskAction(taskId: string, action: 'pause' | 'resume' | 'cancel' | 'retry'): Promise<unknown> {
  return post(`/api/tasks/${encodeURIComponent(taskId)}/${action}`, {}, z.unknown());
}

export function pauseTask(taskId: string): Promise<unknown> { return taskAction(taskId, 'pause'); }
export function resumeTask(taskId: string): Promise<unknown> { return taskAction(taskId, 'resume'); }
export function cancelTask(taskId: string): Promise<unknown> { return taskAction(taskId, 'cancel'); }
export function retryTask(taskId: string): Promise<unknown> { return taskAction(taskId, 'retry'); }
