import {z} from 'zod';
import {get, post} from './http';

/* 启动器域：调用自研 Rust 启动器内核 mPackLauncher 安装/启动 Minecraft。
   安装/启动经 /api/launcher/* 入队为后台任务（202 {taskId}），
   进度与日志从任务域（fetchTasks / 任务日志端点）轮询获取。 */

export type LauncherInstallInput = {
  version: string;
  loader?: string;
  // 加载器版本（如 0.16.14）；留空由启动器取 latest。
  loaderVersion?: string;
  mirror?: string;
  javaPath?: string;
  minecraftDir: string;
  packId?: string;
};

export type LauncherLaunchInput = {
  // 留空即由后端解析该目录实际装出来的版本目录 ID（GET /api/launcher/installs 同源）。
  // 带加载器时它是 fabric-loader-<ver>-<mc>，不等于包的 mcVersion。
  version?: string;
  username: string;
  minecraftDir: string;
  javaPath?: string;
  xmxMb?: number;
  packId?: string;
};

export const launcherInstallSchema = z.object({
  id: z.string(),
  minecraftDir: z.string(),
  versionId: z.string(),
  loader: z.string(),
  mcVersion: z.string(),
  packId: z.string(),
  taskId: z.string(),
  installedAt: z.iso.datetime(),
});
export type LauncherInstall = z.infer<typeof launcherInstallSchema>;

const taskRefSchema = z.object({taskId: z.string()});
const installsSchema = z.object({installs: z.array(launcherInstallSchema)});

export function installLauncher(input: LauncherInstallInput): Promise<{taskId: string}> {
  return post('/api/launcher/install', {
    version: input.version,
    loader: input.loader ?? '',
    loader_version: input.loaderVersion ?? '',
    mirror: input.mirror ?? '',
    java_path: input.javaPath ?? '',
    minecraft_dir: input.minecraftDir,
    pack_id: input.packId ?? '',
  }, taskRefSchema);
}

/** 本机已装出来的版本目录列表：启动入参必须来自它，而不是包的 mcVersion。 */
export function listLauncherInstalls(packId?: string, minecraftDir?: string): Promise<LauncherInstall[]> {
  const params = new URLSearchParams();
  if (packId) params.set('packId', packId);
  if (minecraftDir) params.set('minecraftDir', minecraftDir);
  const qs = params.toString();
  return get(`/api/launcher/installs${qs ? `?${qs}` : ''}`, installsSchema).then(r => r.installs);
}

export function launchLauncher(input: LauncherLaunchInput): Promise<{taskId: string}> {
  return post('/api/launcher/launch', {
    version: input.version ?? '',
    username: input.username,
    minecraft_dir: input.minecraftDir,
    java_path: input.javaPath ?? '',
    xmx_mb: input.xmxMb ?? 0,
    pack_id: input.packId ?? '',
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
