import {z} from 'zod';
import {del, get, patch, post} from './http';

/* 整合包域:CRUD。对齐后端 service.Pack,无值字段返回 null 不省略键(D-4)。 */

export const loaderEnum = z.enum(['forge', 'neoforge', 'fabric', 'quilt']);
export type Loader = z.infer<typeof loaderEnum>;

/* 加载器显示名。后端存的是小写标识（fabric / neoforge），界面上要按各家官方写法
   显示：NeoForge 和 Forge 是两个词、必须大写中间那个字母，Fabric / Quilt 单词首字母大写。
   不认识的值原样返回 —— 不猜、不吞。 */
const LOADER_LABELS: Record<string, string> = {
  fabric: 'Fabric',
  forge: 'Forge',
  neoforge: 'NeoForge',
  quilt: 'Quilt',
};
export function loaderLabel(loader: string | null | undefined): string {
  const v = (loader ?? '').trim();
  if (!v) return '';
  return LOADER_LABELS[v.toLowerCase()] ?? v;
}

export const packSchema = z.object({
  id: z.string(),
  name: z.string(),
  iconUrl: z.string().nullable(),
  mcVersion: z.string(),
  loader: z.string(),
  loaderVersion: z.string().nullable(),
  description: z.string().nullable(),
  status: z.string(),
  packVersion: z.string(),
  createdAt: z.iso.datetime(),
  updatedAt: z.iso.datetime(),
});
export type Pack = z.infer<typeof packSchema>;

export const packListSchema = z.object({
  items: z.array(packSchema),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
});

export type CreatePackInput = {
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion?: string;
  description?: string;
};

export const listPacks = () => get('/api/packs', packListSchema).then(v => v.items);
export const getPack = (id: string) => get(`/api/packs/${encodeURIComponent(id)}`, packSchema);
export const updatePack = (id: string, body: unknown) => patch(`/api/packs/${encodeURIComponent(id)}`, body, packSchema);

export function createPack(input: CreatePackInput): Promise<Pack> {
  return post('/api/packs', {
    name: input.name,
    mcVersion: input.mcVersion,
    loader: input.loader,
    loaderVersion: input.loaderVersion ?? '',
    description: input.description ?? '',
  }, packSchema);
}

/* 删除返回 204 No Content,成功后由调用方刷新列表。 */
export const deletePack = (id: string) => del(`/api/packs/${encodeURIComponent(id)}`);
