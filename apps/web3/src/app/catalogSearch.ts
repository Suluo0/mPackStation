import type {CatalogItem} from '../api/catalog';

/* 目录搜索（纯函数，多语言 + 命名空间 + 相关度排序）。
   玩家不会记全名：搜 "ae" / "ae2" 要能命中 ae2 命名空间，搜英文要能命中 en_us 名。
   排序权重（小在前）：
     0 命名空间命中（ae → ae2:* 全部）  1 ID 前缀  2 当前语言名前缀
     3 任意语言名包含（en_us "Applied Energistics 2"）  4 ID 包含  5 当前语言名包含
   同分按 ID 字典序。无查询时保持目录原序（注册表序）。 */
export type ItemMatch = {item: CatalogItem; score: number};

export function searchCatalogItems(items: CatalogItem[], needle: string, ns: string | null): ItemMatch[] {
  const q = needle.trim().toLowerCase();
  if (!q) {
    const out: ItemMatch[] = [];
    for (const item of items) {
      if (ns && !item.id.startsWith(`${ns}:`)) continue;
      out.push({item, score: 9});
    }
    return out.sort((a, b) => a.item.id.localeCompare(b.item.id));
  }
  const out: ItemMatch[] = [];
  for (const item of items) {
    if (ns && !item.id.startsWith(`${ns}:`)) continue;
    const nsPrefix = item.id.split(':')[0] ?? '';
    const id = item.id.toLowerCase();
    const dn = item.displayName.toLowerCase();
    let score = -1;
    if (nsPrefix.includes(q)) score = 0;
    else if (id.startsWith(q)) score = 1;
    else if (dn.startsWith(q)) score = 2;
    else if ((item.names ?? []).some(n => n.name.toLowerCase().includes(q))) score = 3;
    else if (id.includes(q)) score = 4;
    else if (dn.includes(q)) score = 5;
    if (score >= 0) out.push({item, score});
  }
  return out.sort((a, b) => a.score - b.score || a.item.id.localeCompare(b.item.id));
}

/** 交叉来源提示用：忽略 ns 筛选后，命中最多的命名空间（没有则 null）。 */
export function matchNamespace(items: CatalogItem[], needle: string): {ns: string; count: number} | null {
  const q = needle.trim().toLowerCase();
  if (!q) return null;
  const counter = new Map<string, number>();
  for (const it of items) {
    const nsp = it.id.split(':')[0] ?? '';
    if (nsp.includes(q)) counter.set(nsp, (counter.get(nsp) ?? 0) + 1);
  }
  let best: {ns: string; count: number} | null = null;
  for (const [ns, count] of counter) {
    if (!best || count > best.count) best = {ns, count};
  }
  return best;
}

/* ── 标签搜索（# 前缀）────────────────────────────────────
   玩家用概念词搜索（#tools → 锄头斧子镐），平台标签体系没有这个标签，
   靠一张概念表把概念词展开成具体标签集合；其余 # 查询按标签 ID 匹配。
   与模组搜索别名表同一模式：人工核实的种子，长尾靠直接 #<tag>。 */
export const TAG_CONCEPTS: Record<string, string[]> = {
  tools: ['minecraft:pickaxes', 'minecraft:axes', 'minecraft:hoes', 'minecraft:swords', 'minecraft:shovels'],
  工具: ['minecraft:pickaxes', 'minecraft:axes', 'minecraft:hoes', 'minecraft:swords', 'minecraft:shovels'],
  swords: ['minecraft:swords'],
  axes: ['minecraft:axes'],
  pickaxes: ['minecraft:pickaxes'],
  镐: ['minecraft:pickaxes'],
  斧: ['minecraft:axes'],
  剑: ['minecraft:swords'],
  armor: ['minecraft:head_armor', 'minecraft:chest_armor', 'minecraft:leg_armor', 'minecraft:foot_armor'],
  盔甲: ['minecraft:head_armor', 'minecraft:chest_armor', 'minecraft:leg_armor', 'minecraft:foot_armor'],
};

/** 编辑距离（≤2 的短串够用；早退优化不做了，调用方量小）。 */
export function levenshtein(a: string, b: string): number {
  const m = a.length;
  const n = b.length;
  const dp: number[] = Array.from({length: n + 1}, (_, j) => j);
  for (let i = 1; i <= m; i++) {
    let prev = dp[0];
    dp[0] = i;
    for (let j = 1; j <= n; j++) {
      const tmp = dp[j];
      dp[j] = Math.min(dp[j] + 1, dp[j - 1] + 1, prev + (a[i - 1] === b[j - 1] ? 0 : 1));
      prev = tmp;
    }
  }
  return dp[n];
}

export type TagMatch = {item: CatalogItem; matchedTag: string; score: number};

/** # 标签搜索：概念表展开（含编辑距离 ≤1 容错，aer→ae2 同款思路），
    否则按标签 ID 匹配（末段精确 > 末段前缀 > 整 ID 包含）。 */
export function searchCatalogByTag(items: CatalogItem[], tagQuery: string): TagMatch[] {
  const q = tagQuery.trim().toLowerCase();
  if (!q) return [];
  let conceptTags: Set<string> | null = null;
  if (TAG_CONCEPTS[q]) {
    conceptTags = new Set(TAG_CONCEPTS[q]);
  } else if (q.length >= 3) {
    for (const key of Object.keys(TAG_CONCEPTS)) {
      if (levenshtein(q, key) <= 1) { conceptTags = new Set(TAG_CONCEPTS[key]); break; }
    }
  }
  const out: TagMatch[] = [];
  for (const item of items) {
    let best: {tag: string; score: number} | null = null;
    for (const tag of item.tags ?? []) {
      const id = tag.toLowerCase();
      const seg = id.split(':').pop() ?? id;
      let score = -1;
      if (conceptTags?.has(tag)) score = 0;
      else if (seg === q) score = 1;
      else if (seg.startsWith(q)) score = 2;
      else if (id.includes(q)) score = 3;
      if (score >= 0 && (!best || score < best.score)) best = {tag, score};
    }
    if (best) out.push({item, matchedTag: best.tag, score: best.score});
  }
  return out.sort((a, b) => a.score - b.score || a.item.id.localeCompare(b.item.id));
}

/** 空态推荐：标签 ID 含查询词的标签（按成员数降序），供点击精确化。 */
export function suggestTags(tags: {id: string; members: string[]}[], tagQuery: string, limit = 6): {id: string; count: number}[] {
  const q = tagQuery.trim().toLowerCase();
  if (!q) return [];
  return tags
    .filter(t => t.id.toLowerCase().includes(q) && t.members.length > 0)
    .sort((a, b) => b.members.length - a.members.length)
    .slice(0, limit)
    .map(t => ({id: t.id, count: t.members.length}));
}

/* ── 复杂过滤器（第七轮定稿：JEI 式前缀，无类型选择器）────────
   条件 = 值 + 启用位；值的前缀决定匹配模式：# 标签、@ 来源（模组）、裸词文本。
   条件间连接词全局（全部满足 AND / 任一满足 OR）。
   URL 契约：?f=value:on,value:on（value 可含冒号）+ ?fmode=and|or。 */
export type FilterKind = 'tag' | 'ns' | 'text';
export type FilterCond = {value: string; on: boolean};
export type FilterMode = 'and' | 'or';

/** 值的前缀决定匹配模式：# 标签、@ 来源、裸词文本（JEI 习惯）。 */
export function condKind(value: string): FilterKind {
  const v = value.trim();
  if (v.startsWith('#')) return 'tag';
  if (v.startsWith('@')) return 'ns';
  return 'text';
}

/** 去掉前缀后的实际查询词。 */
export function condQuery(value: string): string {
  const v = value.trim();
  return v.startsWith('#') || v.startsWith('@') ? v.slice(1) : v;
}

export function parseFilterSpec(raw: string | null, modeRaw: string | null): {conds: FilterCond[]; mode: FilterMode} {
  const mode: FilterMode = modeRaw === 'or' ? 'or' : 'and';
  if (!raw) return {conds: [], mode};
  const conds: FilterCond[] = [];
  for (const part of raw.split(',')) {
    const seg = part.split(':');
    if (seg.length < 2) continue;
    const on = seg[seg.length - 1] === '1';
    const value = seg.slice(0, -1).join(':');
    // 有前缀的值去前缀后必须非空（裸 "#" / "@" 不算条件）
    if (value && condQuery(value)) conds.push({value, on});
  }
  return {conds, mode};
}

export function encodeFilterSpec(conds: FilterCond[]): string {
  return conds.map(c => `${c.value}:${c.on ? 1 : 0}`).join(',');
}

/** 文本谓词：ID / 当前语言名 / 任意语言名 命中即真（与 searchCatalogItems 的文本口径一致）。 */
export function textMatchItem(item: CatalogItem, q: string): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  // /pattern → 正则匹配（JEI 常见搜索方式）
  if (needle.startsWith('/')) {
    try {
      const re = new RegExp(needle.slice(1), 'i');
      return re.test(item.id) || re.test(item.displayName)
        || (item.names ?? []).some(n => re.test(n.name));
    } catch {
      return false; // 非法正则当无匹配
    }
  }
  return item.id.toLowerCase().includes(needle)
    || item.displayName.toLowerCase().includes(needle)
    || (item.names ?? []).some(n => n.name.toLowerCase().includes(needle));
}

/** 标签谓词：概念表展开（含容错）或标签 ID 匹配。概念展开按查询词记忆，避免逐物品重算。 */
const conceptMemo = new Map<string, Set<string> | null>();
function conceptSetFor(q: string): Set<string> | null {
  if (conceptMemo.has(q)) return conceptMemo.get(q) ?? null;
  let set: Set<string> | null = null;
  if (TAG_CONCEPTS[q]) set = new Set(TAG_CONCEPTS[q]);
  else if (q.length >= 3) {
    for (const key of Object.keys(TAG_CONCEPTS)) {
      if (levenshtein(q, key) <= 1) { set = new Set(TAG_CONCEPTS[key]); break; }
    }
  }
  conceptMemo.set(q, set);
  return set;
}

export function tagMatchItem(item: CatalogItem, q: string): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  const concept = conceptSetFor(needle);
  for (const tag of item.tags ?? []) {
    const id = tag.toLowerCase();
    const seg = id.split(':').pop() ?? id;
    if (concept?.has(tag)) return true;
    if (seg === needle || seg.startsWith(needle) || id.includes(needle)) return true;
  }
  return false;
}

/** 单条件的谓词（前缀分派；前缀后为空 = 不命中任何东西）。 */
export function condMatchItem(item: CatalogItem, value: string): boolean {
  const kind = condKind(value);
  const q = condQuery(value);
  if (!q) return false;
  if (kind === 'tag') return tagMatchItem(item, q);
  if (kind === 'ns') return (item.id.split(':')[0] ?? '').toLowerCase().startsWith(q.toLowerCase());
  return textMatchItem(item, q);
}

/** 应用过滤：启用的条件按连接词组合；未启用条件不参与。 */
export function applyFilter(items: CatalogItem[], conds: FilterCond[], mode: FilterMode): CatalogItem[] {
  const active = conds.filter(c => c.on);
  if (active.length === 0) return items;
  const pred = (it: CatalogItem) => mode === 'and'
    ? active.every(c => condMatchItem(it, c.value))
    : active.some(c => condMatchItem(it, c.value));
  return items.filter(pred);
}
