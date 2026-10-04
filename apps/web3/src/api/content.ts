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
  code: z.string().default(''),
  severity: z.string().default('info'),
  path: z.string().default(''),
  message: z.string().default(''),
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

/** 任务书校验/应用出参与后端一致：{status, issues, revisionId} */
export const questValidationSchema = z.object({
  status: z.string().default(''),
  issues: z.array(validationIssueSchema).default([]),
  revisionId: z.string().nullable().optional().default(null),
});
export type QuestValidation = z.infer<typeof questValidationSchema>;

const listEnvelope = <T extends z.ZodTypeAny>(item: T) =>
  z.object({items: z.array(item), next_cursor: z.string().nullable(), total: z.number().int()});

export const listContent = (packId: string, kind?: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/content${kind ? `?kind=${encodeURIComponent(kind)}` : ''}`, listEnvelope(contentDocumentSchema)).then(v => v.items);
export const createContent = (packId: string, body: {kind: string; slug: string; title: string}) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content`, body, contentDocumentSchema);
export const getContent = (packId: string, documentId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}`, z.object({document: contentDocumentSchema, revision: revisionSchema.nullable()}));
export const contentHistory = (packId: string, documentId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/history`, listEnvelope(revisionSchema)).then(v => v.items);
export const saveContentDraft = (packId: string, documentId: string, ifMatch: number, payload: unknown) =>
  put(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/draft`, {payload}, revisionSchema, {headers: {'If-Match': `"${ifMatch}"`}});
export const validateContent = (packId: string, documentId: string, revisionId?: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/validate${revisionId ? `?revisionId=${encodeURIComponent(revisionId)}` : ''}`, {}, validationSchema);
/* apply 出参按契约携带应用后的修订。 */
export const applyContent = (packId: string, documentId: string, revisionId?: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/apply${revisionId ? `?revisionId=${encodeURIComponent(revisionId)}` : ''}`, {}, z.object({status: z.string(), revision: revisionSchema.optional()}));
export const rollbackContent = (packId: string, documentId: string, revisionId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/content/${encodeURIComponent(documentId)}/rollback`, {revisionId}, revisionSchema);

/* ---- 任务书(同一 pack 下的特殊内容,后端是 QuestBookView) ----
   P0 FTB 扩展字段全部 optional，旧 draft 无字段时前端默认值。 */

export const questBookMetaSchema = z.object({
  title: z.string().optional(),
  icon: z.string().optional(),
  progressionMode: z.string().optional(),
}).optional();

export const questChapterSchema = z.object({
  id: z.string(),
  title: z.string().default(''),
  description: z.string().default(''),
  coverColor: z.string().default(''),
  position: z.number().int().default(0),
  icon: z.string().optional(),
});
export const questNodeSchema = z.object({
  id: z.string(),
  chapterId: z.string(),
  title: z.string().default(''),
  description: z.string().default(''),
  icon: z.string().default(''),
  x: z.number().default(0),
  y: z.number().default(0),
  /* M1：保存时由 draft.edges 同步为指向该节点的 fromNodeId 字符串列表。 */
  prerequisites: z.array(z.unknown()).default([]),
  rewards: z.array(z.unknown()).default([]),
  modRefs: z.array(z.unknown()).default([]),
  position: z.number().int().default(0),
  /* FTB P0 optional extensions */
  subtitle: z.string().optional(),
  shape: z.string().optional(),
  size: z.number().optional(),
  optional: z.boolean().optional(),
  invisible: z.boolean().optional(),
  dependencyRequirement: z.string().optional(),
  minRequiredDependencies: z.number().int().optional(),
  tasks: z.array(z.record(z.string(), z.unknown())).optional(),
});
/* draft.edges 是任务图权威源；后端 quest_edges + 环检测已就绪。 */
export const questEdgeSchema = z.object({
  id: z.string(),
  fromNodeId: z.string(),
  toNodeId: z.string(),
});
export const questDraftSchema = z.object({
  book: questBookMetaSchema,
  chapters: z.array(questChapterSchema).default([]),
  nodes: z.array(questNodeSchema).default([]),
  edges: z.array(questEdgeSchema).default([]),
});
export const questRevisionSchema = z.object({
  id: z.string(),
  questBookId: z.string(),
  state: z.string(),
  revision: z.number().int(),
  createdAt: z.string(),
  draft: questDraftSchema,
});
export const questBookSchema = z.object({
  id: z.string(),
  packId: z.string(),
  activeRevisionId: z.string().nullable().default(null),
  revision: questRevisionSchema,
});
export type QuestBook = z.infer<typeof questBookSchema>;
export type QuestChapter = z.infer<typeof questChapterSchema>;
export type QuestNode = z.infer<typeof questNodeSchema>;
export type QuestEdge = z.infer<typeof questEdgeSchema>;
export type QuestDraft = z.infer<typeof questDraftSchema>;

export const getQuest = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/quests`, questBookSchema);
/* PUT /quests/draft 的响应信封是 {revision, issues} 而不是裸的 revision ——
   以前这里按裸 revision 解析，zod 每次都在校验阶段失败，于是「草稿其实已经存进库了，
   界面却认为失败」，保存按钮永远等不到回执、后续按钮一直 disabled。 */
export const saveQuestDraft = (packId: string, ifMatch: number, body: unknown) =>
  put(`/api/packs/${encodeURIComponent(packId)}/quests/draft`, body,
    z.object({revision: questRevisionSchema, issues: z.array(validationIssueSchema).default([])}),
    {headers: {'If-Match': `"${ifMatch}"`}}).then(r => r.revision);
export const validateQuest = (packId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/quests/validate`, {}, questValidationSchema);
export const applyQuest = (packId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/quests/apply`, {}, questValidationSchema);
export const rollbackQuest = (packId: string, revisionId: string) =>
  post(`/api/packs/${encodeURIComponent(packId)}/quests/rollback`, {revisionId}, questRevisionSchema);
export const questHistory = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/quests/history`, listEnvelope(questRevisionSchema)).then(v => v.items);
