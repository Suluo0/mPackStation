import {z} from 'zod';
import {get, post} from './http';

/* 模组内容提取引擎:触发解析、列出解析结果、查看单条详情、查询解析运行状态。
   API 按模组维度: /api/packs/{packId}/mods/{modId}/content 。 */

export const modContentItemSchema = z.object({
  id: z.string(),
  kind: z.string(),
  path: z.string(),
  key: z.string(),
  payload: z.unknown(),
  isDynamic: z.boolean(),
  parseError: z.string().optional(),
});
export type ModContentItem = z.infer<typeof modContentItemSchema>;

export const modContentRunSchema = z.object({
  status: z.string(),
  parsedCount: z.number().int(),
  dynamicCount: z.number().int(),
  errorCount: z.number().int(),
  totalFiles: z.number().int(),
  parsedAt: z.string().optional(),
  errorMessage: z.string().optional(),
});
export type ModContentRun = z.infer<typeof modContentRunSchema>;

export const modContentListSchema = z.object({
  items: z.array(modContentItemSchema),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
  run: modContentRunSchema.nullable(),
});
export type ModContentList = z.infer<typeof modContentListSchema>;

export const parseResultSchema = z.object({
  taskId: z.string(),
  status: z.string(),
});
export type ParseResult = z.infer<typeof parseResultSchema>;

const base = (packId: string, modId: string) =>
  `/api/packs/${encodeURIComponent(packId)}/mods/${encodeURIComponent(modId)}/content`;

/** 触发异步解析,返回 202 + taskId;同一 sha1 已解析时后端返回 409。 */
export const parseModContent = (packId: string, modId: string) =>
  post(`${base(packId, modId)}/parse`, {}, parseResultSchema);

/** 列出解析结果,可按 kind 过滤、分页。 */
export const listModContent = (
  packId: string,
  modId: string,
  opts: {kind?: string; limit?: number; cursor?: string} = {},
) => {
  const qs = new URLSearchParams();
  if (opts.kind) qs.set('kind', opts.kind);
  if (opts.limit) qs.set('limit', String(opts.limit));
  if (opts.cursor) qs.set('cursor', opts.cursor);
  const q = qs.toString();
  return get(`${base(packId, modId)}${q ? `?${q}` : ''}`, modContentListSchema);
};

/** 查询最近一次解析运行状态。未解析时后端返回 404。 */
export const getModContentRun = (packId: string, modId: string) =>
  get(`${base(packId, modId)}/run`, z.object({run: modContentRunSchema})).then(v => v.run);

/** 按 id 获取单条解析结果详情(含完整 payload)。 */
export const getModContent = (packId: string, modId: string, contentId: string) =>
  get(`${base(packId, modId)}/${encodeURIComponent(contentId)}`, z.object({item: modContentItemSchema})).then(v => v.item);

export const modContentIconsSchema = z.object({
  tagIcons: z.record(z.string(), z.string()),
  items: z.array(modContentItemSchema),
  missing: z.array(z.string()),
  warnings: z.array(z.string()),
  mcVersion: z.string(),
});
/** Resolve fresh icons against the exact vanilla resource version; caches assets locally. */
export const resolveModContentIcons = (packId: string, modId: string) =>
  post(`${base(packId, modId)}/icons/resolve`, {}, modContentIconsSchema);
