import {z} from 'zod';
import {get, post, put} from './http';

/* 内容域:内容文档(配方/结构等)与任务书,共用一个域文件。
   后端已修 D-4(B6): activeRevisionId/sourceRevisionId 恒在、无值发 null。 */

export const contentDocumentSchema = z.object({
  id: z.string(),
  packId: z.string(),
  kind: z.string(),
  slug: z.string(),
  title: z.string(),
  activeRevisionId: z.string().nullable(),
  createdAt: z.iso.datetime(),
  updatedAt: z.iso.datetime(),
});
export type ContentDocument = z.infer<typeof contentDocumentSchema>;

export const revisionSchema = z.object({
  id: z.string(),
  documentId: z.string(),
  state: z.string(),
  sourceRevisionId: z.string().nullable(),
  revision: z.number().int(),
  /* payload 结构随 kind 变化(配方/结构/矿脉各不相同),由编辑器按 kind 解释。 */
  payload: z.unknown(),
  createdAt: z.iso.datetime(),
});
export type ContentRevision = z.infer<typeof revisionSchema>;

export const validationIssueSchema = z.object({
  code: z.string(),
  severity: z.string(),
  path: z.string(),
  message: z.string(),
  details: z.record(z.string(), z.unknown()).optional(),
});

export const validationSchema = z.object({
  id: z.string(),
  revisionId: z.string(),
  status: z.string(),
  issues: z.array(validationIssueSchema),
  affectedMods: z.array(z.string()),
  createdAt: z.iso.datetime(),
});
export type ContentValidation = z.infer<typeof validationSchema>;

const listEnvelope = <T extends z.ZodTypeAny>(item: T) =>
  z.object({items: z.array(item), next_cursor: z.string().nullable(), total: z.number().int()});

export const listContent = (packId: string, kind?: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/content${kind ? `?kind=${encodeURIComponent(kind)}` : ''}`, listEnvelope(contentDocumentSchema)).then(v => v.items);
export const getContent = (packId: string, documentId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}`, z.object({document: contentDocumentSchema, revision: revisionSchema}));
export const saveContentDraft = (packId: string, documentId: string, ifMatch: number, payload: unknown) =>
  put(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/draft`, {payload}, revisionSchema, {headers: {'If-Match': `"${ifMatch}"`}});
export const validateContent = (packId: string, documentId: string, revisionId?: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/validate${revisionId ? `?revisionId=${encodeURIComponent(revisionId)}` : ''}`, {}, validationSchema);
/* apply 出参按契约携带应用后的修订。 */
export const applyContent = (packId: string, documentId: string, revisionId?: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/apply${revisionId ? `?revisionId=${encodeURIComponent(revisionId)}` : ''}`, {}, z.object({status: z.string(), revision: revisionSchema.optional()}));
export const rollbackContent = (packId: string, documentId: string, revisionId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/rollback`, {revisionId}, revisionSchema);

/* ---- 任务书(同一 pack 下的特殊内容,后端是 QuestBookView) ---- */

export const questChapterSchema = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string(),
  coverColor: z.string(),
  position: z.number().int(),
});
export const questNodeSchema = z.object({
  id: z.string(),
  chapterId: z.string(),
  title: z.string(),
  description: z.string(),
  icon: z.string(),
  x: z.number(),
  y: z.number(),
  prerequisites: z.array(z.unknown()),
  rewards: z.array(z.unknown()),
  modRefs: z.array(z.unknown()),
  position: z.number().int(),
});
export const questEdgeSchema = z.object({
  id: z.string(),
  fromNodeId: z.string(),
  toNodeId: z.string(),
});
export const questDraftSchema = z.object({
  chapters: z.array(questChapterSchema),
  nodes: z.array(questNodeSchema),
  edges: z.array(questEdgeSchema),
});
export const questRevisionSchema = z.object({
  id: z.string(),
  questBookId: z.string(),
  state: z.string(),
  revision: z.number().int(),
  createdAt: z.iso.datetime(),
  draft: questDraftSchema,
});
export const questBookSchema = z.object({
  id: z.string(),
  packId: z.string(),
  activeRevisionId: z.string().nullable(),
  revision: questRevisionSchema,
});
export type QuestBook = z.infer<typeof questBookSchema>;

export const getQuest = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/quests`, questBookSchema);
export const saveQuestDraft = (packId: string, ifMatch: number, body: unknown) =>
  put(`/api/packs/${encodeURIComponent(packId)}/quests/draft`, body, questRevisionSchema, {headers: {'If-Match': `"${ifMatch}"`}});
export const validateQuest = (packId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/quests/validate`, {}, validationSchema);
export const applyQuest = (packId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/quests/apply`, {}, z.object({status: z.string()}));
export const rollbackQuest = (packId: string, revisionId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/quests/rollback`, {revisionId}, questRevisionSchema);
export const questHistory = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/quests/history`, listEnvelope(questRevisionSchema)).then(v => v.items);
