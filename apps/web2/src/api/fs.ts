import {z} from 'zod';
import {get, post, tokenHeaders} from './http';

/* 本机目录浏览与导出目录注册：启动台/发布页用后端枚举目录，
   浏览器 webkitdirectory 无法提供服务端可用的绝对路径。 */

export const fsDirSchema = z.object({
  name: z.string(),
  path: z.string(),
});

export const fsBrowseSchema = z.object({
  path: z.string(),
  parent: z.string(),
  directories: z.array(fsDirSchema),
  suggested: z.array(fsDirSchema),
});
export type FsBrowse = z.infer<typeof fsBrowseSchema>;

/* 后端把目录浏览列为"需令牌读取"(它会暴露服务器绝对路径),所以这里带 X-MPack-Token。 */
export const browseDirectories = (path?: string) =>
  get(`/api/fs/browse${path ? `?path=${encodeURIComponent(path)}` : ''}`, fsBrowseSchema, {headers: tokenHeaders()});

export const registerExportDir = (name: string, directory: string) =>
  post('/api/export-dirs', {name, directory}, z.object({name: z.string(), status: z.string()}));
