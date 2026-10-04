/* 逆向链路引擎 · interpret：配方 payload → 边。
   按「键名并集」分类，不按 type 白名单——键名并集缺一个就掉一批配方
   （2026-10-03 实测：Mekanism 不加 main_output 只有 76%，加上 89%）。
   规格出处：docs/design/craft-chain-engine-plan-2026-10-03.md §2。 */

export type Ref = {kind: 'item' | 'item_tag'; id: string; qty: number; unit: 'item' | 'mB'};

export type Interpreted = {
  inputs: Ref[];
  outputs: Ref[];
  /** 非物品输入（能量/魔力/热/流体）：进 totals 与边标注，不进 children。 */
  nonItem: {energy?: number; heat?: number; fluid?: Ref[]; mana?: number};
  ticks?: {value: number; source: 'recipe-field'; field: string};
  /** ok=false → 引擎侧记为 unresolved 断点。 */
  ok: boolean;
  misses: string[];
};

const IN_KEYS = ['input', 'ingredient', 'ingredients', 'item_input', 'item_inputs', 'chemical_input',
  'fluid_input', 'inputs', 'top_input', 'bottom_input', 'left_input', 'right_input',
  'stack', 'catalyst', 'key'];
const OUT_KEYS = ['output', 'outputs', 'result', 'results', 'item_output', 'item_outputs',
  'chemical_output', 'fluid_output', 'main_output', 'secondary_output'];
const TICK_KEYS = ['duration', 'cookingtime', 'time', 'ticks'];

type Json = Record<string, unknown>;

const isObj = (v: unknown): v is Json => v !== null && typeof v === 'object' && !Array.isArray(v);

/** 物品/化学品/流体引用 → Ref。count=物品数量（缺省 1）；
    amount=化学品/流体数量（单位 mB，不取整）。tag 与 item 互斥出现 → item_tag。 */
function toRef(raw: unknown): Ref | null {
  if (typeof raw === 'string') {
    return {kind: 'item', id: raw, qty: 1, unit: 'item'};
  }
  if (!isObj(raw)) {
    return null;
  }
  const idRaw = raw.id ?? raw.item ?? raw.item_id;
  const tagRaw = raw.tag ?? raw.item_tag ?? raw.tag_id;
  const id = typeof idRaw === 'string' ? idRaw : null;
  if (id) {
    const count = typeof raw.count === 'number' ? raw.count : 1;
    return {kind: 'item', id, qty: count, unit: 'item'};
  }
  if (typeof tagRaw === 'string') {
    const count = typeof raw.count === 'number' ? raw.count : 1;
    return {kind: 'item_tag', id: tagRaw, qty: count, unit: 'item'};
  }
  const chem = raw.chemical ?? raw.fluid;
  if (isObj(chem)) {
    const chemId = typeof chem.id === 'string' ? chem.id : typeof raw.id === 'string' ? raw.id : null;
    if (chemId) {
      const amount = typeof chem.amount === 'number' ? chem.amount : typeof raw.amount === 'number' ? raw.amount : 1;
      return {kind: 'item', id: chemId, qty: amount, unit: 'mB'};
    }
  }
  return null;
}

function collectRefs(raw: unknown, out: Ref[], skip: Set<string>): void {
  if (Array.isArray(raw)) {
    for (const entry of raw) {
      collectRefs(entry, out, skip);
    }
    return;
  }
  if (!isObj(raw)) {
    return;
  }
  const ref = toRef(raw);
  if (ref) {
    // secondary_output 为空对象这类"占位输出"不入边（2026-10-03 sawing 特例）
    if (ref.id && ref.id !== 'minecraft:air') {
      out.push(ref);
    }
    return;
  }
  // 嵌套对象（如mekanism的{item:{...}}包裹）继续下钻，但已识别过的键不再重复归边
  for (const [k, v] of Object.entries(raw)) {
    if (!skip.has(k)) {
      collectRefs(v, out, skip);
    }
  }
}

export function interpret(type: string, payload: unknown): Interpreted {
  const miss = (m: string): Interpreted => ({
    inputs: [], outputs: [], nonItem: {}, ok: false, misses: [m],
  });
  if (!isObj(payload)) {
    return miss(`${type}: payload 不是对象`);
  }
  const p = payload as Json;
  const inputs: Ref[] = [];
  const outputs: Ref[] = [];
  const nonItem: Interpreted['nonItem'] = {};

  for (const k of IN_KEYS) {
    if (k in p) {
      collectRefs(p[k], inputs, new Set(Object.keys(p)));
    }
  }
  for (const k of OUT_KEYS) {
    if (k in p) {
      collectRefs(p[k], outputs, new Set(Object.keys(p)));
    }
  }
  // 非物品输入（能量/热/魔力，用户 10-03 硬要求：不进 children，进 totals）
  const energy = p.energy ?? p.power ?? p.fe ?? p.per_tick_energy;
  if (typeof energy === 'number') {
    nonItem.energy = energy;
  }
  const heat = p.heat;
  if (typeof heat === 'number') {
    nonItem.heat = heat;
  }
  const mana = p.mana;
  if (typeof mana === 'number') {
    nonItem.mana = mana;
  }
  const fluidOut: Ref[] = [];
  collectRefs(p.fluid_output ?? p.output_fluid, fluidOut, new Set());
  if (fluidOut.length) {
    nonItem.fluid = fluidOut;
  }

  let ticks: Interpreted['ticks'] | undefined;
  for (const k of TICK_KEYS) {
    const v = p[k];
    if (typeof v === 'number' && v > 0) {
      ticks = {value: v, source: 'recipe-field', field: k};
      break;
    }
  }

  const misses: string[] = [];
  if (inputs.length === 0) {
    misses.push('no-inputs');
  }
  if (outputs.length === 0) {
    misses.push('no-outputs');
  }
  return {inputs, outputs, nonItem, ticks, ok: misses.length === 0, misses};
}
