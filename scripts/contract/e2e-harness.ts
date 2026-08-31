// E2E harness: 调用真实前端 API 层(src/api/*, 与应用同一份 zod schema)打活后端。
// 任何 zod 不匹配都会抛错记 FAIL。相对 '/api/...' 路径由 shim 转到测试 base URL。
// 由 scripts/verify-contract.sh 打包执行; 环境变量:
//   MPACK_E2E_BASE     后端 base URL (默认 http://127.0.0.1:18872)
//   MPACK_E2E_ZIP_B64  base64 的最小 mrpack zip (manifest.json)
//   MPACK_E2E_RUN      运行标识(幂等键唯一化, 默认时间戳)
const BASE = process.env.MPACK_E2E_BASE || 'http://127.0.0.1:18872';
// http.ts get() 用 window.setTimeout(浏览器 API); Node harness 需要 shim。
(globalThis as any).window = globalThis;
const realFetch = globalThis.fetch;
globalThis.fetch = ((input: any, init?: any) => {
  const url = typeof input === 'string' && input.startsWith('/')
    ? BASE + input
    : input;
  return realFetch(url, init);
}) as typeof fetch;

import {fetchDashboard, fetchActivities} from '../../apps/web/src/api/dashboard';
import {fetchHealth, fetchStatus, fetchMcVersions} from '../../apps/web/src/api/system';
import {fetchOnboarding, acknowledgeOnboarding} from '../../apps/web/src/api/onboarding';
import {listPacks, getPack, createPack, updatePack, deletePack} from '../../apps/web/src/api/packs';
import {listMods, listLocks, listConflicts, packHealth} from '../../apps/web/src/api/mods';
import {fetchTasks, pauseTask} from '../../apps/web/src/api/tasks';
import {inspectImport, confirmImport} from '../../apps/web/src/api/imports';
import {ApiError} from '../../apps/web/src/api/http';

let pass = 0, fail = 0;
async function check(name: string, fn: () => Promise<any>, expect?: (v: any) => boolean) {
  try {
    const v = await fn();
    if (expect && !expect(v)) { fail++; console.log(`FAIL ${name}: shape ok but expectation false -> ${JSON.stringify(v).slice(0, 200)}`); return; }
    pass++; console.log(`PASS ${name}`);
  } catch (e: any) {
    fail++;
    const extra = e?.issues ? JSON.stringify(e.issues).slice(0, 300) : (e?.code || e?.message || String(e)).slice(0, 300);
    console.log(`FAIL ${name}: ${e?.constructor?.name} ${extra}`);
  }
}
async function checkErr(name: string, fn: () => Promise<any>, wantStatus: number, wantCode: string) {
  try {
    await fn();
    fail++; console.log(`FAIL ${name}: expected ${wantStatus}/${wantCode}, got success`);
  } catch (e: any) {
    if (e instanceof ApiError && e.status === wantStatus && e.code === wantCode) { pass++; console.log(`PASS ${name} (${wantStatus} ${wantCode})`); }
    else { fail++; console.log(`FAIL ${name}: got ${e?.constructor?.name} status=${e?.status} code=${e?.code}`); }
  }
}

async function main() {
  await check('fetchHealth', fetchHealth);
  await check('fetchStatus', fetchStatus);
  await check('fetchDashboard', fetchDashboard, v => Array.isArray(v.packs ?? v.items ?? v));
  await check('fetchTasks', fetchTasks, v => Array.isArray(v));
  await check('fetchActivities', fetchActivities, v => Array.isArray(v));
  await check('fetchMcVersions', fetchMcVersions, v => Array.isArray(v) && v.length > 0);
  await check('fetchOnboarding', fetchOnboarding);

  // 写链路(http.ts 自动带 X-MPack-Token)
  let packId = '';
  await check('createPack', async () => {
    const p = await createPack({name: `e2e-harness-${process.env.MPACK_E2E_RUN || Date.now()}`, mcVersion: '1.20.1', loader: 'fabric'} as any);
    packId = (p as any).id;
    return p;
  }, v => !!v.id && v.iconUrl === null && typeof v.createdAt === 'string');
  await check('listPacks', listPacks, v => v.some((p: any) => p.id === packId));
  await check('getPack', () => getPack(packId), v => v.id === packId);
  await check('updatePack', () => updatePack(packId, {description: 'e2e'}), v => v.description === 'e2e');
  await checkErr('updatePack dup name -> 422', async () => {
    const p2 = await createPack({name: `e2e-harness-b-${process.env.MPACK_E2E_RUN || Date.now()}`, mcVersion: '1.20.1', loader: 'fabric'} as any);
    try { await updatePack((p2 as any).id, {name: `e2e-harness-${process.env.MPACK_E2E_RUN || Date.now()}`}); }
    finally { await deletePack((p2 as any).id); }
  }, 422, 'pack_name_duplicate');
  await check('listMods', () => listMods(packId), v => Array.isArray(v));
  await check('listLocks', () => listLocks(packId), v => Array.isArray(v));
  await check('listConflicts', () => listConflicts(packId), v => Array.isArray(v));
  await check('packHealth', () => packHealth(packId), v => typeof v.healthy === 'boolean');
  await check('acknowledgeOnboarding', () => acknowledgeOnboarding({firstPack: true}));
  await checkErr('onboarding readonly -> 422', () => acknowledgeOnboarding({prismAccount: true}), 422, 'onboarding_step_readonly');
  await checkErr('pauseTask missing -> 404', () => pauseTask('t-nope'), 404, 'task_not_found');

  // 导入两阶段(zip 字节由外部生成)
  await check('inspect+confirm import', async () => {
    const b64 = process.env.MPACK_E2E_ZIP_B64!;
    const runId = process.env.MPACK_E2E_RUN || String(Date.now());
    const preview = await inspectImport({source: 'local', contentBase64: b64});
    const r1 = await confirmImport(preview, `harness-${runId}-1`);
    if (!(r1 as any).taskId) throw new Error('confirm missing taskId');
    const r2 = await confirmImport(preview, `harness-${runId}-2`);
    if (!(r2 as any).reused || (r2 as any).taskId !== (r1 as any).taskId) throw new Error('replay did not return original task');
    return r2;
  });
  await check('deletePack', () => deletePack(packId));
  await checkErr('getPack after delete -> 404', () => getPack(packId), 404, 'pack_not_found');

  console.log(`\n== E2E via frontend layer: PASS=${pass} FAIL=${fail} (base=${BASE})`);
  process.exit(fail ? 1 : 0);
}
main();
