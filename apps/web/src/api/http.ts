import type {ZodType} from 'zod';

/* fetch + zod 校验的极简封装。契约不符时立刻抛错，不让脏数据进组件。 */

async function parseResponse<T>(response: Response, schema: ZodType<T>): Promise<T> {
  if (!response.ok) throw new Error(await readError(response));
  const body: unknown = await response.json();
  const result = schema.safeParse(body);
  if (!result.success) {
    throw new Error(`接口数据结构不符合约定：${result.error.issues.map(i => i.message).join('；')}`);
  }
  return result.data;
}

/* 后端统一错误信封是 {"error": {"code": ..., "message": ...}}，但也可能直接是纯文本。 */
async function readError(response: Response): Promise<string> {
  const fallback = `HTTP ${response.status} ${response.statusText}`;
  try {
    const body: unknown = await response.json();
    if (body && typeof body === 'object' && 'error' in body) {
      const err = (body as {error: unknown}).error;
      if (err && typeof err === 'object' && 'message' in err) {
        return String((err as {message: unknown}).message) || fallback;
      }
      if (typeof err === 'string') return err || fallback;
    }
    if (body && typeof body === 'object' && 'message' in body) {
      return String((body as {message: unknown}).message) || fallback;
    }
  } catch {
    return fallback;
  }
  return fallback;
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

/* 非 GET 请求需要 X-MPack-Token。后端读 MPACK_TOKEN，未设置时 dev 兜底为 "test"
   （后端代码里标注为 P2 待办）。生产部署必须同时设置服务端的 MPACK_TOKEN 和
   前端构建期的 VITE_MPACK_TOKEN，两边保持一致。 */
const WRITE_TOKEN: string = import.meta.env.VITE_MPACK_TOKEN ?? (import.meta.env.DEV ? 'test' : '');

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
  const response = await fetch(url, {method: 'DELETE', headers: {'X-MPack-Token': WRITE_TOKEN}});
  if (!response.ok) throw new Error(await readError(response));
}
