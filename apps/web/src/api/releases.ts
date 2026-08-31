import {z} from 'zod';
import {get, post} from './http';

/* 发布域:交付检查、版本、产物、发布记录、构建/发布动作。 */

export const deliveryCheckSchema = z.object({
  kind: z.string(),
  status: z.string(),
  detail: z.string(),
});
export type DeliveryCheck = z.infer<typeof deliveryCheckSchema>;

export const artifactSchema = z.object({
  id: z.string(),
  packId: z.string(),
  packVersionId: z.string(),
  fileName: z.string(),
  sha256: z.string(),
  sourceFingerprint: z.string(),
  status: z.string(),
  kind: z.string(),
  sizeBytes: z.number(),
  createdAt: z.iso.datetime(),
});
export type Artifact = z.infer<typeof artifactSchema>;

export const versionSchema = z.object({
  id: z.string(),
  packId: z.string(),
  version: z.string(),
  channel: z.string(),
  changelog: z.string(),
  source: z.string(),
  lockId: z.string(),
  createdAt: z.iso.datetime(),
  updatedAt: z.iso.datetime(),
});
export type PackVersion = z.infer<typeof versionSchema>;

export const releaseSchema = z.object({
  id: z.string(),
  packId: z.string(),
  packVersionId: z.string(),
  provider: z.string(),
  status: z.string(),
  remoteId: z.string(),
  idempotencyKey: z.string(),
  remoteState: z.string(),
  artifactId: z.string(),
  errorCode: z.string(),
  errorMessage: z.string(),
  createdAt: z.iso.datetime(),
  updatedAt: z.iso.datetime(),
});
export type Release = z.infer<typeof releaseSchema>;

const page = <T extends z.ZodTypeAny>(item: T) => z.object({
  items: z.array(item),
  next_cursor: z.string().nullable(),
  total: z.number().int(),
});

export const listDeliveryChecks = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/delivery-checks`, page(deliveryCheckSchema));
export const runDeliveryChecks = (packId: string, body: unknown) =>
  post(`/api/packs/${encodeURIComponent(packId)}/delivery-checks/run`, body, z.object({items: z.array(deliveryCheckSchema)}));
export const listVersions = (packId: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/versions`, page(versionSchema)).then(v => v.items);
export const listArtifacts = (packId: string, versionId?: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/artifacts${versionId ? `?packVersionId=${encodeURIComponent(versionId)}` : ''}`, page(artifactSchema)).then(v => v.items);
export const listReleases = (packId: string, versionId?: string) =>
  get(`/api/packs/${encodeURIComponent(packId)}/releases${versionId ? `?packVersionId=${encodeURIComponent(versionId)}` : ''}`, page(releaseSchema)).then(v => v.items);

export const buildPack = (packId: string, body: unknown) =>
  post(`/api/packs/${encodeURIComponent(packId)}/build`, body, z.object({artifact: artifactSchema, sourceFingerprint: z.string()}));
export const publishPack = (packId: string, provider: string, body: unknown) =>
  post(`/api/packs/${encodeURIComponent(packId)}/publish/${encodeURIComponent(provider)}`, body, releaseSchema);
export const pollRelease = (id: string) =>
  post(`/api/releases/${encodeURIComponent(id)}/poll`, {}, releaseSchema);
export const retryRelease = (id: string, body: unknown) =>
  post(`/api/releases/${encodeURIComponent(id)}/retry`, body, releaseSchema);
