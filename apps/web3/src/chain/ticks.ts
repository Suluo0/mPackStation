/* 逆向链路引擎 · ticks：机器时长四层降级（L1 配方字段 → L2/L3 后端工单 BK-1 ⏸ → L4 规则表）。
   规则：tick:null 一律渲染成「节拍未核」，任何组件不得用 200 兜底。规格出处：方案 §3。 */

export type Tick = {
  mode: 'per-cycle' | 'rate' | 'unknown';
  ticksPerCycle?: number;
  rate?: {inQty: number; inUnit: 'mB' | 'item'; outQty: number; outUnit: 'mB' | 'item'; perTicks: number};
  parallel?: number;
  /** 必填出处：'recipe.duration' | 'javap BASE_TICKS_REQUIRED' | 'GeneralConfig.java:140' | 'JEI 实测 2026-10-03' */
  source: string;
  confidence: 'measured' | 'derived' | 'assumed';
};

export type TickRule =
  | {mode: 'per-cycle'; ticksPerCycle: number; source: string; confidence: 'measured' | 'derived' | 'assumed'}
  | {mode: 'rate'; rate: {inQty: number; inUnit: 'mB' | 'item'; outQty: number; outUnit: 'mB' | 'item'; perTicks: number}; source: string; confidence: 'measured' | 'derived' | 'assumed'};

/** 由Type算机器名（type = "mekanism:enriching" 等）。 */
export function machineOf(type: string): string {
  return type.split(':').slice(1).join(':');
}

/** L4 规则表查询：机器类节拍/速率。chainRules.json 由调用方传入。 */
export function ruleFor(rules: {byType?: Record<string, TickRule & {tick?: number | null}>},
  type: string): Tick | null {
  const byType = rules.byType ?? {};
  const entry = byType[type];
  if (!entry) {
    return null;
  }
  // 兼容 {tick: 200} 简写（等价 per-cycle）
  if ('tick' in entry && typeof (entry as {tick?: number}).tick === 'number') {
    return {mode: 'per-cycle', ticksPerCycle: (entry as {tick: number}).tick, source: entry.source, confidence: entry.confidence};
  }
  if (entry.mode === 'rate') {
    return entry;
  }
  if (entry.mode === 'per-cycle' && typeof entry.ticksPerCycle === 'number') {
    return {mode: 'per-cycle', ticksPerCycle: entry.ticksPerCycle, source: entry.source, confidence: entry.confidence};
  }
  return null;
}

/** L1：配方字段时长（最高优先，来源最硬）。 */
export function tickFromRecipe(payload: Record<string, unknown>): Tick | null {
  for (const k of ['duration', 'cookingtime', 'time', 'ticks']) {
    const v = payload[k];
    if (typeof v === 'number' && v > 0) {
      return {mode: 'per-cycle', ticksPerCycle: v, source: `recipe.${k}`, confidence: 'measured'};
    }
  }
  return null;
}

/** 组装最终 Tick：L1 配方字段 → L4 规则表 → unknown（绝不用固定值兜底）。 */
export function resolveTick(type: string, payload: Record<string, unknown>,
  rules: {byType?: Record<string, TickRule & {tick?: number | null}>}): Tick {
  return tickFromRecipe(payload) ?? ruleFor(rules, type)
    ?? {mode: 'unknown', source: '节拍未核', confidence: 'assumed'};
}
