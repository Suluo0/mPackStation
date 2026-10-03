import {z} from 'zod';
import {get, post} from './http';

const nameSchema = z.object({locale:z.string(),name:z.string(),key:z.string(),source:z.string()});
export const catalogItemSchema = z.object({id:z.string(),displayName:z.string(),resolvedLocale:z.string(),evidence:z.string(),modelPath:z.string(),iconStatus:z.string(),iconReason:z.string(),names:z.array(nameSchema),tags:z.array(z.string())});
export const catalogBlockSchema = z.object({id:z.string(),displayName:z.string(),resolvedLocale:z.string(),evidence:z.string(),blockstatePath:z.string(),names:z.array(nameSchema),tags:z.array(z.string()),itemIds:z.array(z.string())});
export const catalogTagSchema = z.object({registry:z.enum(['item','block']),id:z.string(),displayName:z.string(),resolvedLocale:z.string(),status:z.string(),labelSource:z.string(),diagnostics:z.array(z.string()),members:z.array(z.string())});
const recipeRefSchema=z.object({role:z.enum(['input','output']),kind:z.enum(['item','item_tag']),id:z.string(),slot:z.number().int(),alternative:z.number().int(),count:z.number().int()});
export const catalogRecipeSchema=z.object({id:z.string(),type:z.string(),status:z.string(),payload:z.unknown(),diagnostics:z.array(z.string()),refs:z.array(recipeRefSchema)});
export const itemCatalogSchema=z.object({revision:z.number().int(),builtAt:z.number().int(),locale:z.string(),availableLocales:z.array(z.string()),warnings:z.array(z.string()),items:z.array(catalogItemSchema),blocks:z.array(catalogBlockSchema),tags:z.array(catalogTagSchema),recipes:z.array(catalogRecipeSchema)});
export const catalogStatusSchema=z.object({sourceRevision:z.number().int(),builtRevision:z.number().int(),status:z.enum(['pending','running','succeeded','failed']),builtAt:z.number().int(),lastError:z.string(),warnings:z.array(z.string()),stale:z.boolean()});
export type ItemCatalog=z.infer<typeof itemCatalogSchema>;
export type CatalogItem=z.infer<typeof catalogItemSchema>;
export type CatalogTag=z.infer<typeof catalogTagSchema>;
export type CatalogRecipe=z.infer<typeof catalogRecipeSchema>;
export type CatalogRecipeRef=z.infer<typeof recipeRefSchema>;

const base=(packId:string)=>`/api/packs/${encodeURIComponent(packId)}/catalog`;
export const getItemCatalog=(packId:string,locale='zh_cn')=>get(`${base(packId)}?locale=${encodeURIComponent(locale)}`,itemCatalogSchema);
export const getCatalogStatus=(packId:string)=>get(`${base(packId)}/status`,catalogStatusSchema);
export const rebuildItemCatalog=(packId:string,locale='zh_cn')=>post(`${base(packId)}/rebuild?locale=${encodeURIComponent(locale)}`,{},z.object({taskId:z.string(),status:z.string()}));
