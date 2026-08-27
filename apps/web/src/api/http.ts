import type {ZodType} from 'zod';

/* fetch + zod 校验的极简封装。契约不符时立刻抛错，不让脏数据进组件。 */

async function parseResponse<T>(response: Response, schema: ZodType<T>): Promise<T> {
  if (!response.ok) throw new Error(`HTTP ${response.status} ${response.statusText}`);
  const body: unknown = await response.json();
  const result = schema.safeParse(body);
  if (!result.success) {
    throw new Error(`接口数据结构不符合约定：${result.error.issues.map(i => i.message).join('；')}`);
  }
  return result.data;
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

export async function post<T>(url: string, body: unknown, schema: ZodType<T>): Promise<T> {
  return parseResponse(await fetch(url, {
    method: 'POST',
    headers: {'content-type': 'application/json'},
    body: JSON.stringify(body),
  }), schema);
}
