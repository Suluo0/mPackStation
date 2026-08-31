import type {ZodType} from 'zod';

/* fetch + zod 校验的极简封装。契约不符时立刻抛错，不让脏数据进组件。 */

/** ApiError 保留后端错误信封的 status 与 code,界面可按 code 分支
    (如 revision_conflict → "内容已被他人修改,刷新后重试")。 */
export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function parseResponse<T>(response: Response, schema: ZodType<T>): Promise<T> {
  if (!response.ok) throw await readError(response);
  const body: unknown = await response.json();
  const result = schema.safeParse(body);
  if (!result.success) {
    throw new Error(`接口数据结构不符合约定：${result.error.issues.map(i => i.message).join('；')}`);
  }
  return result.data;
}

/* 后端统一错误信封是 {"error": {"code": ..., "message": ...}}，但也可能直接是纯文本。 */
async function readError(response: Response): Promise<ApiError> {
  const fallback = `HTTP ${response.status} ${response.statusText}`;
  try {
    const body: unknown = await response.json();
    if (body && typeof body === 'object' && 'error' in body) {
      const err = (body as {error: unknown}).error;
      if (err && typeof err === 'object') {
        const e = err as {message?: unknown; code?: unknown};
        const message = typeof e.message === 'string' && e.message ? e.message : fallback;
        const code = typeof e.code === 'string' && e.code ? e.code : 'unknown';
        return new ApiError(message, response.status, code);
      }
      if (typeof err === 'string') return new ApiError(err || fallback, response.status, 'unknown');
    }
    if (body && typeof body === 'object' && 'message' in body) {
      const m = (body as {message: unknown}).message;
      return new ApiError(typeof m === 'string' && m ? m : fallback, response.status, 'unknown');
    }
  } catch {
    return new ApiError(fallback, response.status, 'unknown');
  }
  return new ApiError(fallback, response.status, 'unknown');
}

export async function get<T>(url: string, schema: ZodType<T>): Promise<T> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 8000);
  try {
    return await parseResponse(await fetch(url, {signal: controller.signal}), schema);
  } finally {
    window.clearTimeout(timeout);
  }
}

/* 非 GET 请求需要 X-MPack-Token。令牌由 vite.config.ts 在构建期注入
   （VITE_MPACK_TOKEN 环境变量优先，否则读后端 data/runtime-token)。
   不存在硬编码兜底；注入为空时写请求会被后端 401，错误会如实抛给界面。 */
const WRITE_TOKEN: string = typeof __MPACK_WRITE_TOKEN__ === 'string' ? __MPACK_WRITE_TOKEN__ : '';

function writeHeaders(): Record<string, string> {
  return {'content-type': 'application/json', 'X-MPack-Token': WRITE_TOKEN};
}

export type RequestOptions = {headers?: Record<string, string>};

async function send<T>(method: 'POST' | 'PUT' | 'PATCH', url: string, body: unknown, schema: ZodType<T>, options: RequestOptions = {}): Promise<T> {
  const response = await fetch(url, {
    method,
    headers: {...writeHeaders(), ...options.headers},
    body: JSON.stringify(body),
  });
  return parseResponse(response, schema);
}

export function post<T>(url: string, body: unknown, schema: ZodType<T>, options?: RequestOptions): Promise<T> {
  return send('POST', url, body, schema, options);
}

export function put<T>(url: string, body: unknown, schema: ZodType<T>, options?: RequestOptions): Promise<T> {
  return send('PUT', url, body, schema, options);
}

export function patch<T>(url: string, body: unknown, schema: ZodType<T>, options?: RequestOptions): Promise<T> {
  return send('PATCH', url, body, schema, options);
}

/* DELETE 返回 204 No Content，没有响应体，不能走 parseResponse。 */
export async function del(url: string): Promise<void> {
  const response = await fetch(url, {method: 'DELETE', headers: writeHeaders()});
  if (!response.ok) throw await readError(response);
}

/* PUT 204 No Content 变体(如设置类端点,无响应体)。 */
export async function putVoid(url: string, body: unknown): Promise<void> {
  const response = await fetch(url, {method: 'PUT', headers: writeHeaders(), body: JSON.stringify(body)});
  if (!response.ok) throw await readError(response);
}
