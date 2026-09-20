/* FTB dependency evaluation matrix — pure function unit tests.
   Run: node --experimental-strip-types apps/web/scripts/test-quest-deps.ts
   Matrix (design §5.6):
     all_completed + 2 全完成 → 解锁
     one_completed + 1/2 完成 → 解锁
     all_completed + 1/2 完成 → 锁定
     min_required=1 + 2 前置完成 1 → 解锁
*/
import {
  depPayloadToUi,
  depUiToPayload,
  evaluateDependencies,
  isNodeAvailable,
} from '../src/features/quest/questDeps.ts';

function assert(cond: boolean, msg: string) {
  if (!cond) {
    console.error('FAIL:', msg);
    process.exit(1);
  }
  console.log('ok -', msg);
}

const S = (...ids: string[]) => new Set(ids);

// all_completed
{
  const r = evaluateDependencies(['a', 'b'], 'all_completed', 0, S('a', 'b'));
  assert(r.unlocked, 'all_completed + 2/2 completed → unlocked');
  const r1 = evaluateDependencies(['a', 'b'], 'all_completed', 0, S('a'));
  assert(!r1.unlocked, 'all_completed + 1/2 completed → locked');
  assert(JSON.stringify(r1.missing) === JSON.stringify(['b']), 'all_completed missing lists b');
  const r0 = evaluateDependencies(['a', 'b'], 'all_completed', 0, S());
  assert(!r0.unlocked, 'all_completed + 0/2 completed → locked');
}

// one_completed
{
  const r = evaluateDependencies(['a', 'b'], 'one_completed', 0, S('a'));
  assert(r.unlocked, 'one_completed + 1/2 completed → unlocked');
  const r0 = evaluateDependencies(['a', 'b'], 'one_completed', 0, S());
  assert(!r0.unlocked, 'one_completed + 0/2 completed → locked');
  assert(JSON.stringify(r0.missing) === JSON.stringify(['a', 'b']), 'one_completed missing lists all');
}

// min_required (AND_N)
{
  const r = evaluateDependencies(['a', 'b'], 'all_completed', 1, S('a'));
  assert(r.unlocked, 'min_required=1 + 1/2 completed → unlocked');
  const r2 = evaluateDependencies(['a', 'b', 'c'], 'all_completed', 2, S('a', 'b'));
  assert(r2.unlocked, 'min_required=2 + 2/3 completed → unlocked');
  const rFail = evaluateDependencies(['a', 'b'], 'all_completed', 2, S('a'));
  assert(!rFail.unlocked, 'min_required=2 + 1/2 completed → locked');
}

// empty prereqs always unlocked
{
  const r = evaluateDependencies([], 'all_completed', 5, S());
  assert(r.unlocked, 'no prerequisites → unlocked regardless of requirement');
}

// all_started / one_started with explicit started set
{
  const r = evaluateDependencies(['a', 'b'], 'all_started', 0, S('a'), S('a', 'b'));
  assert(r.unlocked, 'all_started + both started (one completed) → unlocked');
  const r1 = evaluateDependencies(['a', 'b'], 'one_started', 0, S(), S('b'));
  assert(r1.unlocked, 'one_started + 1 started → unlocked');
  const r0 = evaluateDependencies(['a', 'b'], 'all_started', 0, S(), S('a'));
  assert(!r0.unlocked, 'all_started + 1/2 started → locked');
}

// default requirement when undefined = all_completed
{
  const r = evaluateDependencies(['a'], undefined, undefined, S('a'));
  assert(r.unlocked, 'undefined requirement + completed prereq → unlocked');
  const r0 = evaluateDependencies(['a'], undefined, undefined, S());
  assert(!r0.unlocked, 'undefined requirement + incomplete prereq → locked');
}

// isNodeAvailable
{
  assert(isNodeAvailable(['x'], 'all_completed', 0, S('x')) === true, 'isNodeAvailable true when met');
  assert(isNodeAvailable(['x'], 'all_completed', 0, S()) === false, 'isNodeAvailable false when locked');
}

// UI ↔ payload mapping (验收 7)
{
  const p1 = depUiToPayload('all_completed', 0);
  assert(p1.dependencyRequirement === 'all_completed' && p1.minRequiredDependencies === 0, 'UI 全部完成 → all_completed');
  const p2 = depUiToPayload('one_completed', 0);
  assert(p2.dependencyRequirement === 'one_completed' && p2.minRequiredDependencies === 0, 'UI 任一完成 → one_completed');
  const p3 = depUiToPayload('min_n', 3);
  assert(p3.dependencyRequirement === 'all_completed' && p3.minRequiredDependencies === 3, 'UI 至少N → minRequiredDependencies=N');

  const u1 = depPayloadToUi('one_completed', 0);
  assert(u1.mode === 'one_completed' && u1.minN === 0, 'payload one_completed → UI 任一完成');
  const u2 = depPayloadToUi('all_completed', 2);
  assert(u2.mode === 'min_n' && u2.minN === 2, 'payload min=2 → UI 至少 N');
  const u3 = depPayloadToUi(undefined, undefined);
  assert(u3.mode === 'all_completed', 'payload empty → UI 全部完成');
}

// round-trip UI→payload→UI
{
  for (const mode of ['all_completed', 'one_completed', 'all_started', 'one_started'] as const) {
    const p = depUiToPayload(mode, 0);
    const u = depPayloadToUi(p.dependencyRequirement, p.minRequiredDependencies);
    assert(u.mode === mode, `round-trip ${mode}`);
  }
  const p = depUiToPayload('min_n', 4);
  const u = depPayloadToUi(p.dependencyRequirement, p.minRequiredDependencies);
  assert(u.mode === 'min_n' && u.minN === 4, 'round-trip min_n=4');
}

console.log('\nquest-deps matrix assertions: ALL PASS');
