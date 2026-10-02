import {z} from 'zod';
import {get, post} from './http';

/* 任务域:任务列表/详情/控制。Task DTO 全局唯一(dto.md Task):
   控制接口与列表返回同一结构;type 是开放字符串,未知 kind 原样返回。 */

export const taskSchema = z.object({
  id: z.string(),
  type: z.string().min(1),
  title: z.string(),
  packId: z.string().nullable(),
  packName: z.string().nullable(),
  status: z.enum(['queued', 'running', 'success', 'failed', 'cancelled', 'paused']),
  progress: z.number().int().min(0).max(100),
  error: z.string().nullable(),
  startedAt: z.iso.datetime().nullable(),
  finishedAt: z.iso.datetime().nullable(),
});
export type Task = z.infer<typeof taskSchema>;

export const taskListSchema = z.object({
  items: z.array(taskSchema),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
});

export function fetchTasks(): Promise<Task[]> {
  return get('/api/tasks?recent=20', taskListSchema).then(v => v.items);
}

function taskAction(taskId: string, action: 'pause' | 'resume' | 'cancel' | 'retry'): Promise<Task> {
  return post(`/api/tasks/${encodeURIComponent(taskId)}/${action}`, {}, taskSchema);
}

export const pauseTask = (taskId: string) => taskAction(taskId, 'pause');
export const resumeTask = (taskId: string) => taskAction(taskId, 'resume');
export const cancelTask = (taskId: string) => taskAction(taskId, 'cancel');
export const retryTask = (taskId: string) => taskAction(taskId, 'retry');
