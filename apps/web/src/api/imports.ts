import {z} from 'zod';
import {post} from './http';

/* 导入域:两阶段(先解析出预览,再确认入队)。真正的导入在后台任务里执行。 */

export const importSourceEnum = z.enum(['curseforge', 'modrinth', 'local']);
export type ImportSource = z.infer<typeof importSourceEnum>;

/* 后端的来源常量是 curseforge_url / modrinth_url / local_zip,
   虽然它也接受 curseforge / modrinth / local 这类别名,这里显式用规范值。 */
const sourcePayload: Record<ImportSource, string> = {
  curseforge: 'curseforge_url',
  modrinth: 'modrinth_url',
  local: 'local_zip',
};

export const importPreviewSchema = z.object({
  id: z.string(),
  token: z.string(),
  inputHash: z.string(),
  source: z.string(),
  expiresAt: z.iso.datetime(),
  entryCount: z.number().int(),
  /* 后端已修 D-4(B6): packName 恒在,URL 来源暂未知名称时为 ""。 */
  packName: z.string(),
});
export type ImportPreview = z.infer<typeof importPreviewSchema>;

/* 确认后立刻入队,返回任务与包标识;契约(2026-08-30 起)不内嵌 task 对象,
   前端拿 taskId 轮询 /api/tasks 即可。 */
export const importConfirmSchema = z.object({
  importId: z.string(),
  taskId: z.string(),
  packId: z.string().nullable(),
  reused: z.boolean(),
});
export type ImportConfirm = z.infer<typeof importConfirmSchema>;

export type InspectImportInput = {
  source: ImportSource;
  url?: string;
  /** 本地 zip 的 base64 内容(不含 data: 前缀) */
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
