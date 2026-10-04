import type {CatalogItem, CatalogRecipe, CatalogRecipeRef, CatalogTag} from '../api/catalog';

/* ── 配方归一化 ────────────────────────────────────────────────
   把后端已经算好的输入输出边（CatalogRecipe.refs）翻译成界面能画的形状。
   这里只做翻译，不重新解析 payload、不发明边 —— 读不出就是读不出。

   后端契约（apps/server/internal/service/item_catalog.go · addRecipe）：
   - crafting_shaped 的 slot = row*3 + column（行主序，固定 3 宽）
   - crafting_shapeless 的 slot = 原料下标（0..n-1）
   - 熔炼 / 高炉 / 烟熏 / 营火 / 切石机：input slot = 0
   - smithing_transform / smithing_trim：0 = 模板，1 = 基底，2 = 附加
   - 一个槽位可以有多条 ref（alternative 0..n-1）＝「任一即可」
   - kind='item_tag' 时 id 是标签 id，成员要用 tagById 展开（后端不展开）
   - 后端不认识配方类型时 refs 为空、status='unsupported' —— 这不是「没有配方」，
     是「有配方但读不出边」，界面必须区分（见 unparsed）。
*/

export type SlotKind = 'item' | 'tag' | 'any';

export type IngredientSlot = {
  /** 槽位在配方内的键（s0..s8 / i0..i2 / out），用于选中与候选游标 */
  key: string;
  kind: SlotKind;
  /** 代表物品 id；标签槽可能为空（标签没解析出成员） */
  id: string;
  /** 可接受的具体物品；item 槽恒为 1 个，tag/any 槽是候选表 */
  candidates: string[];
  /** 一份需要几个 */
  count: number;
  /** kind==='tag' 时来源标签 id */
  tagId: string | null;
};

export type RecipeKind = 'grid' | 'single' | 'smithing' | 'rule' | 'unknown';

/** 卡片上的「任意 X」汇总：同一个标签占了这个配方的几个槽、共几个候选。
    木桶 = 木板 ×6 + 木台阶 ×2 —— 这才是「通用公式」在人眼里的样子。 */
export type TagNote = {
  tagId: string;
  /** 标签的可读名（木板 / 木台阶 …）；推不出就用 id 末段 */
  label: string;
  slots: number;
  candidates: number;
};

export type RecipeView = {
  id: string;
  type: string;
  /** 机器名（工作台 / 切石机 / 熔炉 / 锻造台 …） */
  machine: string;
  kind: RecipeKind;
  /** 网格行数 / 列数；非网格为 0 */
  rows: number;
  cols: number;
  /** 长度 rows*cols 的槽位表，索引 = r*3 + c（与后端的 slot 同口径） */
  grid: (IngredientSlot | null)[];
  /** 单输入机器与锻造台的输入序列 */
  inputs: IngredientSlot[];
  output: IngredientSlot | null;
  /** 读得出类型但读不出产物时的说明（例如锻造纹饰沿用基底装备） */
  outputNote: string | null;
  meta: {time?: number; xp?: number};
  /** 标签：有序 / 无序 / 产出 ×N / 规则 / 未结构化 … */
  flags: string[];
  notes: string[];
  /** 可读的「任意 X」汇总（见 TagNote）；没有标签槽就是空 */
  tagNotes: TagNote[];
  /** status !== 'parsed'：后端没结构化这个类型，边为空 */
  unparsed: boolean;
};

export type RecipeCtx = {
  itemById: Map<string, CatalogItem>;
  tagById: Map<string, CatalogTag>;
};

/** 一组名字里最有代表性的公共后缀。
    原版标签的成员名并不总是整齐：「橡木木板 … 竹板」里有一个异类，
    严格求交会把「木板」一路降级成「板」；所以按「覆盖大多数成员」来选，
    基准取最长的那个名字（拿最短名当基准会提前卡住后缀长度）。 */
function commonSuffix(names: string[]): string {
  if (names.length < 2) return '';
  const base = names.reduce((a, b) => (b.length > a.length ? b : a), names[0]);
  const need = Math.ceil(names.length * 0.8);
  let best = '';
  for (let len = 1; len <= base.length; len++) {
    const cand = base.slice(base.length - len);
    if (names.filter(n => n.endsWith(cand)).length >= need) best = cand; else break;
  }
  return best;
}

/** 标签的可读名。
    后端只在标签恰好一个成员时才给出真译名（labelSource='single_member'），
    多成员标签的 displayName 是 `#ns:path` 形式 —— 对用户等于没有名字，
    「任意木板」就只能显示成「#minecraft:planks」。
    这里做展示层的启发式补足：拿成员的显示名求最长公共后缀，推不出（<2 字）就用 id 末段。
    不改数据、不编造具体物品，只是把已有的名字折叠成一个类目词。 */
export function tagLabel(tag: CatalogTag | undefined, tagId: string, itemById: Map<string, CatalogItem>): string {
  if (tag?.displayName && !tag.displayName.startsWith('#')) return tag.displayName;
  const names = (tag?.members ?? []).map(m => itemById.get(m)?.displayName).filter((n): n is string => !!n);
  const suffix = commonSuffix(names).trim();
  if (suffix.length >= 2) return suffix;
  const seg = tagId.replace(/^#/, '').split(':').pop() ?? tagId;
  return seg.replace(/_/g, ' ');
}

const MACHINE: Record<string, string> = {
  'minecraft:crafting_shaped': '工作台',
  'minecraft:crafting_shapeless': '工作台',
  'minecraft:crafting_transmute': '工作台',
  'minecraft:stonecutting': '切石机',
  'minecraft:smelting': '熔炉',
  'minecraft:blasting': '高炉',
  'minecraft:smoking': '烟熏炉',
  'minecraft:campfire_cooking': '营火',
  'minecraft:smithing_transform': '锻造台',
  'minecraft:smithing_trim': '锻造台',
};

/* 原版「特殊配方」的类型只有规则、没有网格（后端 status='unsupported'）。
   它们是游戏内的判定规则而不是槽位图，所以展示成规则卡才是它们的真实形态。
   这里的文案是游戏规则说明，不是从目录里读出来的数据。 */
export const RULE_NOTES: Record<string, string> = {
  'minecraft:crafting_special_armordye': '皮革盔甲的任意部位 + 任意染料，可混色；颜色不影响属性。',
  'minecraft:crafting_special_bannerduplicate': '同款旗帜两份叠放，图案一并复制（不改颜色）。',
  'minecraft:crafting_special_bookcloning': '书与笔 + 已写成的书，按原书份数递增；每次复制消耗墨水。',
  'minecraft:crafting_special_firework_rocket': '纸 + 火药（数量决定飞行时间）+ 可选烟火之星（决定爆炸效果）。',
  'minecraft:crafting_special_firework_star': '火药 + 任意染料；可叠加形状材料与附加效果材料。',
  'minecraft:crafting_special_firework_star_fade': '烟火之星 + 任意染料，追加褪色渐变。',
  'minecraft:crafting_special_mapcloning': '已探索地图 + 空地图 → 两份相同地图。',
  'minecraft:crafting_special_mapextending': '地图 + 8 张纸，扩大一档比例尺。',
  'minecraft:crafting_special_repairitem': '两件同种可损耗物品，耐久合并并附带少量额外耐久。',
  'minecraft:crafting_special_shielddecoration': '盾 + 任意旗帜，图案取自该旗帜。',
  'minecraft:crafting_special_shulkerboxcoloring': '潜影盒 + 任意染料；盒内物品保留。',
  'minecraft:crafting_special_suspiciousstew': '碗 + 红蘑菇 + 棕蘑菇 + 可选任意花；效果由所加的花决定。',
  'minecraft:crafting_special_tippedarrow': '8 支箭 + 滞留药水 → 8 支药箭，写入药水效果与时长。',
  'minecraft:crafting_decorated_pot': '四个陶片四面环绕 + 红砖；图案由所用陶片决定。',
};

const SPECIAL_PREFIX = 'minecraft:crafting_special_';

export function isRuleType(type: string): boolean {
  return type === 'minecraft:crafting_decorated_pot' || type.startsWith(SPECIAL_PREFIX);
}

export function machineOf(type: string): string {
  if (isRuleType(type)) return type === 'minecraft:crafting_decorated_pot' ? '陶罐' : '工作台';
  return MACHINE[type] ?? type.replace(/^[^:]+:/, '');
}

/* ── payload 的最小读取（payload 是 unknown，只取看得懂的字段） ────── */
function payloadObject(r: CatalogRecipe): Record<string, unknown> | null {
  const p = r.payload;
  return p && typeof p === 'object' && !Array.isArray(p) ? p as Record<string, unknown> : null;
}

/** 有序配方的真实图案尺寸：行数/列数上限 3（后端按 3 宽编 slot）。 */
function shapedDims(r: CatalogRecipe): {rows: number; cols: number} | null {
  const pattern = payloadObject(r)?.pattern;
  if (!Array.isArray(pattern) || pattern.some(x => typeof x !== 'string')) return null;
  if (pattern.length === 0) return null;
  let longest = 1;
  for (const line of pattern as string[]) longest = Math.max(longest, line.length);
  return {rows: Math.min(3, pattern.length), cols: Math.min(3, longest)};
}

/** 一条关系 + 它是怎么被命中的（直连，还是经过某个标签）。 */
export type RecipeHit = {rec: CatalogRecipe; viaTag: string | null};

/** 物品的「作为产物 / 作为原料」配方集合。
    后端的 refs 里，标签原料只有一条 ref、id 是标签本身，不展开成员 ——
    所以按物品 id 直接索引会漏掉「经标签」命中的那条：金合欢木板配方的输入 ref 是
    #minecraft:acacia_logs，拿 acacia_log 去查什么也查不到，而 JEI 的 U 是查得到的。
    这里把焦点物品自己的标签也当提及键查一遍，并把「经哪个标签」一起带出来。 */
export function collectRecipes(
  direct: CatalogRecipe[] | undefined,
  by: Map<string, CatalogRecipe[]>,
  tagKeys: string[],
): RecipeHit[] {
  const seen = new Map<string, RecipeHit>();
  for (const rec of direct ?? []) seen.set(rec.id, {rec, viaTag: null});
  for (const tag of tagKeys) {
    for (const rec of by.get(tag) ?? []) if (!seen.has(rec.id)) seen.set(rec.id, {rec, viaTag: tag});
  }
  return [...seen.values()];
}

/* ── refs → 槽位 ─────────────────────────────────────────────── */

/** 同一槽位的多条 ref ＝ 并列候选（alternative 升序）。 */
function slotFrom(key: string, refs: CatalogRecipeRef[], ctx: RecipeCtx): IngredientSlot | null {
  const sorted = [...refs].sort((a, b) => a.alternative - b.alternative);
  const candidates: string[] = [];
  const tags: string[] = [];
  for (const ref of sorted) {
    if (ref.kind === 'item_tag') {
      if (!tags.includes(ref.id)) tags.push(ref.id);
      for (const member of ctx.tagById.get(ref.id)?.members ?? []) {
        if (ctx.itemById.has(member)) candidates.push(member);
      }
    } else {
      candidates.push(ref.id);
    }
  }
  const uniq = [...new Set(candidates)];
  const count = Math.max(1, sorted[0]?.count ?? 1);

  if (sorted.length === 1 && sorted[0].kind === 'item_tag') {
    return {key, kind: 'tag', id: uniq[0] ?? '', candidates: uniq, count, tagId: sorted[0].id};
  }
  if (uniq.length === 0) {
    return tags.length
      ? {key, kind: 'tag', id: '', candidates: [], count, tagId: tags[0]}
      : null;
  }
  return {
    key,
    kind: sorted.length > 1 ? 'any' : 'item',
    id: uniq[0],
    candidates: uniq,
    count,
    /* 多条 ref 但都来自同一个标签时，这也是「任意 X」——保留 tagId，
       否则这类槽（并列候选写法）在卡片上就没有类目词可显示。 */
    tagId: tags.length === 1 && sorted.every(r => r.kind === 'item_tag') ? tags[0] : null,
  };
}

function groupBySlot(refs: CatalogRecipeRef[]): Map<number, CatalogRecipeRef[]> {
  const out = new Map<number, CatalogRecipeRef[]>();
  for (const ref of refs) {
    const arr = out.get(ref.slot);
    if (arr) arr.push(ref); else out.set(ref.slot, [ref]);
  }
  return out;
}

/** 把一条目录配方翻成可渲染的视图模型。逐条独立，可安全缓存。 */
export function buildRecipeView(r: CatalogRecipe, ctx: RecipeCtx): RecipeView {
  const machine = machineOf(r.type);
  const inputSlots = groupBySlot(r.refs.filter(x => x.role === 'input'));
  const outputRefs = r.refs.filter(x => x.role === 'output');
  /* 槽位键带上配方 id：候选游标是按槽位记的，前缀配方 id 就不会在换配方时串味。 */
  const key = (local: string) => `${r.id}#${local}`;
  const output = outputRefs.length ? slotFrom(key('out'), outputRefs, ctx) : null;
  const unparsed = r.status !== 'parsed';

  const base: RecipeView = {
    id: r.id, type: r.type, machine, kind: 'unknown',
    rows: 0, cols: 0, grid: [], inputs: [], output,
    outputNote: null, meta: {}, flags: [], notes: [...r.diagnostics], tagNotes: [], unparsed,
  };

  if (isRuleType(r.type)) {
    base.kind = 'rule';
    base.flags.push('规则');
    /* 这类配方的类型本来就不在后端结构化范围内 —— 那是预期，不是异常。
       所以卡面上只放规则说明，不把后端那句「尚未结构化此配方类型」摆上去。 */
    const note = RULE_NOTES[r.type];
    base.notes = note ? [note] : [...r.diagnostics];
    return base;
  }

  if (unparsed) {
    /* 类型不在后端结构化范围内：保留类型名，明确标记读不出边。 */
    base.kind = 'unknown';
    return base;
  }

  const highestInput = inputSlots.size ? Math.max(...inputSlots.keys()) : -1;

  if (r.type === 'minecraft:crafting_shaped') {
    const dims = shapedDims(r) ?? {rows: Math.min(3, Math.max(1, Math.ceil((highestInput + 1) / 3))), cols: 3};
    base.kind = 'grid';
    base.rows = dims.rows;
    base.cols = dims.cols;
    base.grid = new Array(dims.rows * dims.cols).fill(null);
    for (const [slot, refs] of inputSlots) {
      /* 后端 slot 是 3 宽行主序（row*3+column），显示栅格却是 dims.cols 宽
         （2×2 的图案就画 2 列），所以要在这个 3 宽坐标→显示下标之间换算。
         column >= dims.cols 属于越界（图案行比最长行短），丢弃。 */
      const row = Math.floor(slot / 3);
      const column = slot % 3;
      if (row < 0 || row >= dims.rows || column >= dims.cols) continue;
      const index = row * dims.cols + column;
      base.grid[index] = slotFrom(key(`s${index}`), refs, ctx);
    }
    base.flags.push('有序');
    if (dims.rows < 3 || dims.cols < 3) base.flags.push(`${dims.rows}×${dims.cols}`);
  } else if (r.type === 'minecraft:crafting_shapeless' || r.type === 'ae2:transform') {
    const n = Math.max(1, highestInput + 1);
    const cols = Math.min(3, n);
    const rows = Math.ceil(n / cols);
    base.kind = 'grid';
    base.rows = rows;
    base.cols = cols;
    base.grid = new Array(rows * cols).fill(null);
    for (const [slot, refs] of inputSlots) {
      if (slot < 0 || slot >= base.grid.length) continue;
      base.grid[slot] = slotFrom(key(`s${slot}`), refs, ctx);
    }
    base.flags.push('无序');
    if (n > 9) base.flags.push(`原料 ${n} 种`);
  } else if (r.type === 'minecraft:smithing_transform' || r.type === 'minecraft:smithing_trim') {
    base.kind = 'smithing';
    const labels = ['模板', '基底', '附加'];
    for (const slot of [0, 1, 2]) {
      const refs = inputSlots.get(slot);
      if (!refs) continue;
      const s = slotFrom(key(`i${slot}`), refs, ctx);
      if (s) base.inputs.push(s);
    }
    if (!output) {
      base.outputNote = r.type === 'minecraft:smithing_trim'
        ? '产物是「基底」那件装备本身，纹饰不改变物品 id'
        : '目录里没有登记固定产物';
    }
    base.notes.push(`输入顺序：${labels.slice(0, base.inputs.length).join(' · ')}`);
  } else if (inputSlots.size) {
    base.kind = 'single';
    for (const slot of [...inputSlots.keys()].sort((a, b) => a - b)) {
      const s = slotFrom(key(`i${slot}`), inputSlots.get(slot)!, ctx);
      if (s) base.inputs.push(s);
    }
    const payload = payloadObject(r);
    const time = payload?.cookingtime ?? payload?.cookingTime;
    const xp = payload?.experience;
    if (typeof time === 'number') base.meta.time = time;
    if (typeof xp === 'number') base.meta.xp = xp;
  } else {
    base.kind = 'unknown';
    return base;
  }

  if (output && output.count > 1) base.flags.push(`产出 ×${output.count}`);

  /* 把标签槽折叠成「任意 X」：木桶的 6 个 #minecraft:planks 槽 → 木板 ×6。
     这是通用公式的可读形态 —— 8 个孤立的 1/11 角标说的是同一件事，却要人自己拼。 */
  const allSlots = base.kind === 'grid'
    ? base.grid.filter((s): s is IngredientSlot => s !== null)
    : base.inputs;
  const agg = new Map<string, TagNote>();
  for (const s of allSlots) {
    if (!s.tagId) continue;
    const hit = agg.get(s.tagId);
    if (hit) {
      hit.slots += 1;
      hit.candidates = Math.max(hit.candidates, s.candidates.length);
    } else {
      agg.set(s.tagId, {
        tagId: s.tagId,
        label: tagLabel(ctx.tagById.get(s.tagId), s.tagId, ctx.itemById),
        slots: 1,
        candidates: s.candidates.length,
      });
    }
  }
  base.tagNotes = [...agg.values()];
  return base;
}

/** 配方卡的视图模型缓存：同一份目录下同一 id 只算一次。 */
export function makeRecipeViewCache(ctx: RecipeCtx) {
  const cache = new Map<string, RecipeView>();
  return (r: CatalogRecipe): RecipeView => {
    const hit = cache.get(r.id);
    if (hit) return hit;
    const view = buildRecipeView(r, ctx);
    cache.set(r.id, view);
    return view;
  };
}
