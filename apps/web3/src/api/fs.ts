import {z} from 'zod';
import {get, post} from './http';

/* 本机目录浏览与导出目录注册：启动台/发布页用后端枚举目录，
   浏览器 webkitdirectory 无法提供服务端可用的绝对路径。
   目录浏览会暴露服务器绝对路径，但后端只监听回环、局域网够不到，
   所以不再需要令牌（见 api/http.ts 头注）。 */

export const fsDirSchema = z.object({
  name: z.string(),
  path: z.string(),
});

export const fsFileSchema = z.object({
  name: z.string(),
  path: z.string(),
  size: z.number(),
});

export const fsBrowseSchema = z.object({
  path: z.string(),
  parent: z.string(),
  directories: z.array(fsDirSchema),
  /* 常规文件（导入文件选择：路径直接回传后端读盘，2026-10-05）。 */
  files: z.array(fsFileSchema).default([]),
  suggested: z.array(fsDirSchema),
});
export type FsBrowse = z.infer<typeof fsBrowseSchema>;

export const browseDirectories = (path?: string) =>
  get(`/api/fs/browse${path ? `?path=${encodeURIComponent(path)}` : ''}`, fsBrowseSchema);

export const registerExportDir = (name: string, directory: string) =>
  post('/api/export-dirs', {name, directory}, z.object({name: z.string(), status: z.string()}));

/* 已批准的产物输出目录。构建必须指定其中之一（后端只往带标记的目录里写文件），
   所以构建界面得能把已有的列出来选，而不是让用户每次重敲路径。 */
export const exportDirSchema = z.object({
  name: z.string(),
  directory: z.string(),
  verifiedAt: z.number(),
  createdAt: z.number(),
});
export type ExportDir = z.infer<typeof exportDirSchema>;

export const listExportDirs = () =>
  get('/api/export-dirs', z.object({
    items: z.array(exportDirSchema),
    next_cursor: z.string().nullable(),
    total: z.number().int(),
  })).then(v => v.items);

