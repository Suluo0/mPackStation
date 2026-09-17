// 解析模组平台链接并维护到模组库：
// - 支持 curseforge.com / modrinth.com / mcmod.cn 三种模组页链接
// - mod_id 永远由系统分配稳定 mod-N；平台 slug 只用于取名称，绝不充当关联键
// - 链接按 (mod_id, platform) 唯一：已存在则更新 URL，相同则不动
import type { Payload, PayloadRequest } from 'payload'
import { commitTransaction, initTransaction, killTransaction, ValidationError } from 'payload'
import { validModId } from './validate'
import type { SessionUser } from './auth'
import { HttpError } from './auth'
import { nextModId } from './mods'
import { publishPoolEvent } from './collaboration'

export type ModPlatform = 'curseforge' | 'modrinth' | 'mcmod'

export type ParseUrlInput = {
  url: string
  /** 可选：显式指定目标 mod_id（必须已在主表注册）；缺省由系统分配下一个 mod-N */
  modId?: string
}

export type ParseUrlResult = {
  modId: string
  platform: ModPlatform
  url: string
  modCreated: boolean
  candidateCreated: boolean
  linkAction: 'created' | 'updated' | 'unchanged'
  nameSource: 'existing' | 'modrinth-api' | 'curseforge-api' | 'slug'
  nameEn: string
  nameZh: string | null
}

// 各平台的模组页路径规则（slug 捕获组）
const HOST_RULES: { host: string; platform: ModPlatform; pattern: RegExp; example: string }[] = [
  {
    host: 'curseforge.com',
    platform: 'curseforge',
    pattern: /^(?:\/minecraft\/(?:mc-mods|mods)|\/projects)\/([A-Za-z0-9][A-Za-z0-9_-]*)$/,
    example: '/minecraft/mc-mods/<slug>'
  },
  {
    host: 'modrinth.com',
    platform: 'modrinth',
    pattern: /^\/(?:mod|mods)\/([A-Za-z0-9][A-Za-z0-9_-]*)/,
    example: '/mod/<slug>'
  },
  {
    host: 'mcmod.cn',
    platform: 'mcmod',
    pattern: /^\/class\/(\d+)\.html?/,
    example: '/class/<id>.html'
  },
]

function parseUrl(url: string): { platform: ModPlatform; slug: string } {
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    throw new ValidationError({ errors: [{ message: '无法解析 URL：' + url, path: 'url' }] })
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new ValidationError({ errors: [{ message: 'URL 必须使用 http/https', path: 'url' }] })
  }
  const host = parsed.hostname.toLowerCase().replace(/^www\./, '')
  for (const rule of HOST_RULES) {
    if (host !== rule.host) continue
    const m = parsed.pathname.match(rule.pattern)
    if (!m) {
      throw new ValidationError({
        errors: [{ message: '无法解析 ' + rule.platform + ' 链接（期望形如 ' + rule.host + rule.example + '）', path: 'url' }],
      })
    }
    return { platform: rule.platform, slug: m[1] }
  }
  throw new ValidationError({
    errors: [{ message: '不支持的平台：' + host + '（当前仅支持 curseforge.com / modrinth.com / mcmod.cn）', path: 'url' }],
  })
}

function prettifySlug(slug: string): string {
  const out = slug
    .split(/[-_]+/)
    .filter(Boolean)
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join(' ')
  return out || slug
}

// 从平台公共 API 取正式名称；失败（CF 无 key / 网络错误 / 无此模组）返回 null，
// 由调用方回退到 slug 派生名——解析建档不因网络问题失败
async function fetchModName(
  platform: ModPlatform,
  slug: string,
  fetchImpl: typeof fetch,
): Promise<string | null> {
  let target: string
  const init: RequestInit = { signal: AbortSignal.timeout(10_000) }
  if (platform === 'modrinth') {
    target = 'https://api.modrinth.com/v2/project/' + encodeURIComponent(slug)
  } else if (platform === 'curseforge') {
    const key = process.env.CURSEFORGE_API_KEY
    if (!key) return null
    target = 'https://api.curseforge.com/v1/mods/' + encodeURIComponent(slug)
    init.headers = { 'x-api-key': key }
  } else {
    return null // mcmod 无公共 API
  }
  try {
    const res = await fetchImpl(target, init)
    if (!res.ok) return null
    const json = (await res.json()) as {
      title?: unknown
      name?: unknown
      data?: { name?: unknown }
    }
    // modrinth v2：展示名在顶层 title；curseforge v1：name 在 data 下
    const candidate =
      platform === 'modrinth' ? (json.title ?? json.name) : (json.data?.name ?? json.name)
    return typeof candidate === 'string' && candidate.trim() ? candidate.trim() : null
  } catch {
    return null
  }
}

export async function parseModUrl(
  payload: Payload,
  input: ParseUrlInput,
  opts: { fetchImpl?: typeof fetch; actor?: SessionUser } = {},
): Promise<ParseUrlResult> {
  const url = typeof input.url === 'string' ? input.url.trim() : ''
  if (!url) throw new ValidationError({ errors: [{ message: '缺少 url', path: 'url' }] })

  const { platform, slug } = parseUrl(url)
  if (input.modId !== undefined) {
    if (!validModId(input.modId)) {
      throw new ValidationError({ errors: [{ message: 'modId 参数非法', path: 'modId' }] })
    }
  }

  const fetchImpl = opts.fetchImpl ?? fetch
  const fetched = input.modId === undefined && (platform === 'modrinth' || platform === 'curseforge')
    ? await fetchModName(platform, slug, fetchImpl)
    : null
  const req = { payload } as unknown as PayloadRequest
  const started = await initTransaction(req)
  try {
    let target = input.modId
    let modCreated = false
    let nameSource: ParseUrlResult['nameSource'] = 'existing'
    let nameEn = ''
    let nameZh: string | null = null

    const sameUrl = await payload.find({
      collection: 'mod-links',
      where: { url: { equals: url } },
      limit: 1,
      depth: 0,
      req,
    })
    const currentOwner = sameUrl.docs[0]?.mod_id
    if (target && currentOwner && currentOwner !== target) {
      throw new HttpError(409, 'SOURCE_URL_CONFLICT', `该来源链接已关联 ${currentOwner}，不能再关联 ${target}`)
    }
    if (!target) target = currentOwner

    const existing = target
      ? await payload.find({
          collection: 'mods',
          where: { mod_id: { equals: target } },
          limit: 1,
          depth: 0,
          req,
        })
      : null

    if (target && (!existing || existing.docs.length === 0)) {
      throw new ValidationError({
        errors: [{ message: '主表中不存在该 mod_id：' + target, path: 'modId' }],
      })
    }

    if (!target) {
      target = await nextModId(payload, req)
      nameSource = fetched
        ? (platform === 'modrinth' ? 'modrinth-api' : 'curseforge-api')
        : 'slug'
      nameEn = fetched || prettifySlug(slug)
      const created = await payload.create({
        collection: 'mods',
        data: { mod_id: target, name_en: nameEn, revision: 1 },
        depth: 0,
        req,
      })
      modCreated = true
      nameZh = created.name_zh ?? null
    } else {
      const existingDoc = existing?.docs[0]
      nameEn = String(existingDoc?.name_en ?? '')
      nameZh = typeof existingDoc?.name_zh === 'string' ? existingDoc.name_zh : null
    }

    const links = await payload.find({
      collection: 'mod-links',
      where: { mod_id: { equals: target }, platform: { equals: platform } },
      limit: 1,
      depth: 0,
      req,
    })
    let linkAction: ParseUrlResult['linkAction']
    if (links.docs.length === 0) {
      await payload.create({
        collection: 'mod-links',
        data: { mod_id: target, platform, url },
        depth: 0,
        req,
      })
      linkAction = 'created'
    } else if (links.docs[0].url === url) {
      linkAction = 'unchanged'
    } else {
      await payload.update({
        collection: 'mod-links',
        id: links.docs[0].id,
        data: { url },
        depth: 0,
        req,
      })
      linkAction = 'updated'
    }

    const candidates = await payload.find({
      collection: 'candidates',
      where: { mod_id: { equals: target } },
      limit: 1,
      depth: 0,
      req,
    })
    const candidateCreated = candidates.docs.length === 0
    if (candidateCreated) {
      await payload.create({
        collection: 'candidates',
        data: { mod_id: target, category: '未分类', status: 'candidate', revision: 1 },
        depth: 0,
        req,
      })
    }

    await commitTransaction(req)
    publishPoolEvent({
      type: 'mod.changed',
      modId: target,
      actor: opts.actor?.display_name || opts.actor?.email || 'system',
      at: new Date().toISOString(),
    })
    return { modId: target, platform, url, modCreated, candidateCreated, linkAction, nameSource, nameEn, nameZh }
  } catch (error) {
    if (started) await killTransaction(req)
    throw error
  }
}
