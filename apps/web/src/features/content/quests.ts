import {z} from 'zod'; import {get,post,put} from '../../api/http';
export const questBookSchema=z.unknown();
export const getQuest=(packId:string)=>get(`/api/packs/${encodeURIComponent(packId)}/quests`,questBookSchema);
export const saveQuestDraft=(packId:string,ifMatch:number,body:unknown)=>put(`/api/packs/${encodeURIComponent(packId)}/quests/draft`,body, z.unknown(), {headers:{'If-Match':`"${ifMatch}"`}});
export const validateQuest=(packId:string)=>post(`/api/packs/${encodeURIComponent(packId)}/quests/validate`,{},z.unknown());
export const applyQuest=(packId:string)=>post(`/api/packs/${encodeURIComponent(packId)}/quests/apply`,{},z.object({status:z.string()}));
export const rollbackQuest=(packId:string,revisionId:string)=>post(`/api/packs/${encodeURIComponent(packId)}/quests/rollback`,{revisionId},z.unknown());
export const questHistory=(packId:string)=>get(`/api/packs/${encodeURIComponent(packId)}/quests/history`,z.object({items:z.array(z.unknown()),next_cursor:z.string().nullable().optional(),total:z.number().int().optional()})).then(v=>v.items);
