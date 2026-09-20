/* FTB dependency evaluation — pure functions, no DOM.
   Payload tokens: all_completed | one_completed | all_started | one_started
   + minRequiredDependencies (AND_N when > 0).
   UI labels (设计 §3 / 验收 7):
     全部完成 → all_completed
     任一完成 → one_completed
     全部开始 → all_started
     任一开始 → one_started
     至少 N   → minRequiredDependencies=N（base requirement 仍写 payload token）
*/

export type DependencyRequirement = 'all_completed' | 'one_completed' | 'all_started' | 'one_started';

export const DEPENDENCY_REQUIREMENT_TOKENS: DependencyRequirement[] = [
  'all_completed',
  'one_completed',
  'all_started',
  'one_started',
];

export type DepUiMode = 'all_completed' | 'one_completed' | 'all_started' | 'one_started' | 'min_n';

export const DEP_UI_OPTIONS: {label: string; mode: DepUiMode}[] = [
  {label: '全部完成', mode: 'all_completed'},
  {label: '任一完成', mode: 'one_completed'},
  {label: '全部开始', mode: 'all_started'},
  {label: '任一开始', mode: 'one_started'},
  {label: '至少 N', mode: 'min_n'},
];

/** UI 选择 → payload 字段。 */
export function depUiToPayload(
  mode: DepUiMode,
  minN: number,
): {dependencyRequirement: DependencyRequirement; minRequiredDependencies: number} {
  if (mode === 'min_n') {
    return {
      dependencyRequirement: 'all_completed',
      minRequiredDependencies: Math.max(0, Math.floor(minN) || 1),
    };
  }
  return {dependencyRequirement: mode, minRequiredDependencies: 0};
}

/** payload 字段 → UI 选择。min>0 时映射为「至少 N」。 */
export function depPayloadToUi(
  requirement: string | undefined,
  minRequired: number | undefined,
): {mode: DepUiMode; minN: number} {
  const minN = Math.max(0, Math.floor(minRequired ?? 0));
  if (minN > 0) return {mode: 'min_n', minN};
  const req = (requirement || 'all_completed') as DependencyRequirement;
  if (DEPENDENCY_REQUIREMENT_TOKENS.includes(req)) {
    return {mode: req as DepUiMode, minN: 0};
  }
  return {mode: 'all_completed', minN: 0};
}

export function depUiLabel(mode: DepUiMode, minN: number): string {
  if (mode === 'min_n') return `至少 ${minN || 1}`;
  return DEP_UI_OPTIONS.find(o => o.mode === mode)?.label ?? '全部完成';
}

export type DepEvalResult = {
  unlocked: boolean;
  /** 未满足的前置节点 id（one_* 满足时为 []）。 */
  missing: string[];
};

/**
 * 预览/模拟求值。started 默认视为 completed（无真实存档时的简化）。
 * 矩阵（设计 §5.6）:
 *   all_completed + 2 全完成 → 解锁
 *   all_completed + 1/2 完成 → 锁定
 *   one_completed + 1/2 完成 → 解锁
 *   minRequired=1 + 2 前置完成 1 → 解锁
 */
export function evaluateDependencies(
  prerequisites: readonly string[],
  requirement: string | undefined,
  minRequiredDependencies: number | undefined,
  completed: ReadonlySet<string>,
  started: ReadonlySet<string> = completed,
): DepEvalResult {
  const prereqs = prerequisites ?? [];
  if (prereqs.length === 0) return {unlocked: true, missing: []};

  const req = (requirement || 'all_completed') as DependencyRequirement;
  const minN = Math.max(0, Math.floor(minRequiredDependencies ?? 0));

  const completedMissing = prereqs.filter(id => !completed.has(id));
  const startedMissing = prereqs.filter(id => !started.has(id));

  if (minN > 0) {
    const doneCount = prereqs.length - completedMissing.length;
    return {unlocked: doneCount >= minN, missing: completedMissing};
  }

  switch (req) {
    case 'all_completed':
      return {unlocked: completedMissing.length === 0, missing: completedMissing};
    case 'one_completed':
      return {
        unlocked: completedMissing.length < prereqs.length,
        missing: completedMissing.length < prereqs.length ? [] : completedMissing,
      };
    case 'all_started':
      return {unlocked: startedMissing.length === 0, missing: startedMissing};
    case 'one_started':
      return {
        unlocked: startedMissing.length < prereqs.length,
        missing: startedMissing.length < prereqs.length ? [] : startedMissing,
      };
    default:
      return {unlocked: completedMissing.length === 0, missing: completedMissing};
  }
}

/** 节点是否可标记完成（前置已满足）。无前置即可接。 */
export function isNodeAvailable(
  prerequisites: readonly string[],
  requirement: string | undefined,
  minRequiredDependencies: number | undefined,
  completed: ReadonlySet<string>,
): boolean {
  return evaluateDependencies(prerequisites, requirement, minRequiredDependencies, completed).unlocked;
}
