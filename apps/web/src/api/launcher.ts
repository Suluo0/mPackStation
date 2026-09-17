import {z} from 'zod';
import {post} from './http';

/* 启动器域：调用自研 Rust 启动器内核 mPackLauncher 安装/启动 Minecraft。
   安装/启动经 /api/launcher/* 入队为后台任务（202 {taskId}），
   进度与日志从任务域（fetchTasks / 任务日志端点）轮询获取。 */

export type LauncherInstallInput = {
  version: string;
  loader?: string;
  mirror?: string;
  minecraftDir: string;
};

export type LauncherLaunchInput = {
  version: string;
  username: string;
  minecraftDir: string;
  javaPath?: string;
  xmxMb?: number;
};

const taskRefSchema = z.object({taskId: z.string()});

export function installLauncher(input: LauncherInstallInput): Promise<{taskId: string}> {
  return post('/api/launcher/install', {
    version: input.version,
    loader: input.loader ?? '',
    mirror: input.mirror ?? '',
    minecraft_dir: input.minecraftDir,
  }, taskRefSchema);
}

export function launchLauncher(input: LauncherLaunchInput): Promise<{taskId: string}> {
  return post('/api/launcher/launch', {
    version: input.version,
    username: input.username,
    minecraft_dir: input.minecraftDir,
    java_path: input.javaPath ?? '',
    xmx_mb: input.xmxMb ?? 0,
  }, taskRefSchema);
}

/* 任务日志端点返回 NDJSON（每行一个任务事件），fetch 后按行解析。 */
export type TaskLogEvent = {
  id: string;
  taskId: string;
  sequence: number;
  status: string;
  message: string;
  detail: unknown;
  createdAt: string;
};

export async function fetchTaskLog(taskId: string): Promise<TaskLogEvent[]> {
  const res = await fetch(`/api/tasks/${encodeURIComponent(taskId)}/log`);
  if (!res.ok) throw new Error(`任务日志请求失败：HTTP ${res.status}`);
  const text = await res.text();
  return text.split('\n')
    .filter(Boolean)
    .map(line => JSON.parse(line) as TaskLogEvent);
}
