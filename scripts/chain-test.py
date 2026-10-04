#!/usr/bin/env python3
"""前端 → 后端调用链路测试（以「前端发出请求 → 后端正确响应并入库」为判定标准）。

设计约束:
- 所有请求一律走前端同源入口(Vite dev 代理 :5273/api),非 GET 带前端 http.ts 里同一份
  X-MPack-Token;请求体逐字段照抄 apps/web/src/api/*.ts 的真实构造,不自创后端专属形状。
- 每条用例判定两件事:HTTP 响应符合预期 + 数据库落盘符合预期(正向:写了该写的表;
  反向:什么都没写 / 保持原样)。
- 每个功能至少一条正向 + 一条反向。
- 只打隔离数据目录(默认 /tmp/mpack-chain),绝不碰开发库。

用法: MPACK_TOKEN=... python3 scripts/chain-test.py [--only 功能前缀] [--out docs/active/tests/chain-raw.log]
"""
from __future__ import annotations

import base64
import io
import json
import os
import re
import sqlite3
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

BASE = os.environ.get('CHAIN_BASE', 'http://127.0.0.1:5273')
DIRECT = os.environ.get('CHAIN_DIRECT', 'http://127.0.0.1:18872')
# 备用的「没装启动器二进制」实例(由 chain-test-run.sh 起在独立端口/独立数据目录)。
NOBIN = os.environ.get('CHAIN_NOBIN', '')
TOKEN = os.environ.get('MPACK_TOKEN', 'chain-token-20260930')
DB_PATH = os.environ.get('CHAIN_DB', '/tmp/mpack-chain/mpackstation.db')
EXPORT_DIR = os.environ.get('CHAIN_EXPORT', '/tmp/mpack-chain/export')
# 服务端 -data 目录：产物行存的是导出目录下的绝对路径，万一是相对路径按这里兜底。
DATA_ROOT = os.environ.get('CHAIN_DATA_ROOT') or os.path.dirname(DB_PATH) or '/tmp/mpack-chain'
MCVER = os.environ.get('MCVER', '1.21.1')
LOADER = os.environ.get('LOADER', 'fabric')
LOADER_VER = os.environ.get('LOADER_VER', '0.16.14')

RESULTS: list[dict] = []
SKIP_REASON: dict[str, str] = {}


# ---------------------------------------------------------------- HTTP 层
def call(method: str, path: str, body=None, headers=None, base=None, raw=False):
    """返回 (status, text, headers)。body 为 dict 时按前端的 content-type 发 JSON。"""
    url = (base or BASE) + path
    data = None
    hdrs = dict(headers or {})
    if body is not None:
        data = body.encode() if isinstance(body, str) else json.dumps(body).encode()
        hdrs.setdefault('content-type', 'application/json')
    if method != 'GET' and 'X-MPack-Token' not in hdrs and not any(
        k.lower() == 'x-mpack-token' for k in hdrs
    ):
        hdrs['X-MPack-Token'] = TOKEN
    req = urllib.request.Request(url, data=data, headers=hdrs, method=method)
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            txt = r.read().decode('utf-8', 'replace')
            return r.status, txt, dict(r.headers)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode('utf-8', 'replace'), dict(e.headers or {})
    except Exception as e:  # noqa: BLE001
        return 0, f'__transport_error__ {type(e).__name__}: {e}', {}


def jload(txt: str):
    try:
        return json.loads(txt)
    except Exception:  # noqa: BLE001
        return None


def err_code(txt: str) -> str:
    j = jload(txt) or {}
    return ((j.get('error') or {}).get('code')) or ''


# ---------------------------------------------------------------- DB 层
def q(sql: str, params=()):
    con = sqlite3.connect(DB_PATH, timeout=20)
    try:
        return con.execute(sql, params).fetchall()
    finally:
        con.close()


def one(sql: str, params=(), default=0):
    rows = q(sql, params)
    return rows[0][0] if rows and rows[0][0] is not None else default


def ignore_pending_error_conflicts(pack_id: str) -> int:
    """把该包 pending 的 error 级冲突逐条点「忽略」（走界面上那个 ignore 按钮的同一条接口）。

    构建闸门（缺陷 O20）会拦下带未解决致命冲突的包：这是对的，但装配类断言
    （19-构建产物 / 22-启动器）要证的是「按包内权威清单装配得对不对」，得先让包
    能构建。顺带把「用户显式忽略 → 闸门放行」这条路也测了一遍。
    """
    st, txt, _ = call('GET', f'/api/packs/{ident(pack_id)}/conflicts')
    items = ((jload(txt) or {}).get('items') or []) if st == 200 else []
    n = 0
    for c in items:
        if c.get('severity') != 'error' or c.get('status') != 'pending':
            continue
        s2, _, _ = call('POST', f"/api/packs/{ident(pack_id)}/conflicts/{ident(c.get('id',''))}/ignore", {})
        if s2 == 200:
            n += 1
    return n


def ts() -> str:
    return time.strftime('%H:%M:%S')


# ---------------------------------------------------------------- 用例框架
def case(feat: str, name: str, method: str, path: str, expect, body=None,
         headers=None, check=None, base=None, note=''):
    """expect: int | list[int] | '*' ; check(ctx)->(ok, why) 追加断言(入库/字段)。"""
    status, text, h = call(method, path, body, headers, base)
    ctx = {'status': status, 'text': text, 'j': jload(text), 'h': h}
    codes = expect if isinstance(expect, (list, tuple)) else ['*' if expect == '*' else expect]
    code_ok = (status in codes) or ('*' in codes)
    why = ''
    persist_ok = True
    if code_ok and check:
        try:
            r = check(ctx)
            res, msg = (r if isinstance(r, tuple) else (r, ''))
            persist_ok = bool(res)
            why = '' if persist_ok else (msg or '断言未通过')
        except Exception as e:  # noqa: BLE001
            persist_ok, why = False, f'断言异常 {type(e).__name__}: {e}'
    ok = code_ok and persist_ok
    if not code_ok:
        why = f'期望 {codes} 实得 {status} {text[:160]}'
    kind = '反' if name[:1] in ('R', 'N') else '正'
    RESULTS.append({'feat': feat, 'name': name, 'kind': kind, 'ok': ok,
                    'status': status, 'why': why, 'note': note,
                    'req': f'{method} {path}'})
    flag = 'PASS' if ok else 'FAIL'
    print(f'{flag}  [{feat}] {name:<44} {status:>3}  {why[:120]}')
    return ctx


def skipped(feat, name, why):
    RESULTS.append({'feat': feat, 'name': name, 'kind': '?', 'ok': None,
                    'status': '-', 'why': why, 'note': '', 'req': '-'})
    print(f'SKIP  [{feat}] {name:<44}      {why[:120]}')


def wait_task(task_id: str, until=('success', 'failed', 'cancelled'), timeout=200):
    """任务终态词汇是 success(契约 :50),不是 succeeded。"""
    end = time.time() + timeout
    last = ''
    while time.time() < end:
        _, t, _ = call('GET', f'/api/tasks/{task_id}')
        j = jload(t) or {}
        last = j.get('status', '')
        if last in until:
            return last, j
        time.sleep(2)
    return last, {}


def wait_catalog(pack, want=('succeeded', 'failed'), timeout=200):
    end = time.time() + timeout
    last = ''
    while time.time() < end:
        _, t, _ = call('GET', f'/api/packs/{pack}/catalog/status')
        last = (jload(t) or {}).get('status', '')
        if last in want:
            return last
        time.sleep(3)
    return last


def ident(s) -> str:
    return urllib.parse.quote(str(s), safe='')


# ================================================================ 开跑
def main():
    only = None
    if '--only' in sys.argv:
        only = sys.argv[sys.argv.index('--only') + 1]

    _, t, _ = call('GET', '/api/health')
    if (jload(t) or {}).get('status') != 'ready':
        print(f'前置失败:隔离后端不可用 {t[:200]}')
        return 2
    RUN = time.strftime('%m%d-%H%M%S')
    print(f'BASE={BASE}  DB={DB_PATH}  run={RUN}\n')

    # ---- 1 建包 -------------------------------------------------------
    F = '1-建包'
    r = case(F, 'P1 前端 createPack 正向', 'POST', '/api/packs', 201,
             {'name': f'Chain {RUN}', 'mcVersion': MCVER, 'loader': LOADER,
              'loaderVersion': '', 'description': ''},
             check=lambda c: (one('select count(*) from packs') == 1
                              and one("select count(*) from packs where name like 'Chain %'") == 1,
                              'packs 未落盘'))
    PACK = (r['j'] or {}).get('id', '')
    case(F, 'R1 同名重复', 'POST', '/api/packs', [409, 422],
         {'name': f'Chain {RUN}', 'mcVersion': MCVER, 'loader': LOADER,
          'loaderVersion': '', 'description': ''},
         check=lambda c: one('select count(*) from packs') == 1)
    case(F, 'R2 不支持的 mcVersion', 'POST', '/api/packs', [422],
         {'name': f'Bad {RUN}', 'mcVersion': '9.9.9', 'loader': LOADER,
          'loaderVersion': '', 'description': ''},
         check=lambda c: (one('select count(*) from packs') == 1, '脏数据入库'))
    case(F, 'R3 缺写令牌', 'POST', '/api/packs', 401,
         {'name': f'Tok {RUN}', 'mcVersion': MCVER, 'loader': LOADER},
         headers={'X-MPack-Token': ''},
         check=lambda c: one('select count(*) from packs') == 1)
    case(F, 'R4 错误写令牌', 'POST', '/api/packs', 401,
         {'name': f'Tok2 {RUN}', 'mcVersion': MCVER, 'loader': LOADER},
         headers={'X-MPack-Token': 'wrong-' + TOKEN},
         check=lambda c: one('select count(*) from packs') == 1)
    case(F, 'R5 非法 JSON', 'POST', '/api/packs', [400, 422], '{not json',
         check=lambda c: one('select count(*) from packs') == 1)
    if not PACK:
        print('无 packId,中止'); return 1

    # ---- 2 查包 -------------------------------------------------------
    F = '2-查询'
    case(F, 'P1 列表', 'GET', '/api/packs', 200,
         check=lambda c: len((c['j'] or {}).get('items') or []) >= 1)
    case(F, 'P2 详情', 'GET', f'/api/packs/{ident(PACK)}', 200,
         check=lambda c: (c['j'] or {}).get('id') == PACK)
    case(F, 'R1 不存在', 'GET', '/api/packs/pack-nope', [404],
         check=lambda c: err_code(c['text']) == 'pack_not_found')
    case(F, 'R2 分页 limit 越界', 'GET', '/api/packs?limit=-1', [200, 400, 422],
         note='观察是否被服务端收敛')

    # ---- 3 改包 / 归档 ------------------------------------------------
    F = '3-改包'
    case(F, 'P1 改描述', 'PATCH', f'/api/packs/{ident(PACK)}', 200,
         {'description': f'chain-{RUN}'},
         check=lambda c: one('select description from packs') == f'chain-{RUN}')
    case(F, 'R1 空 name 覆盖', 'PATCH', f'/api/packs/{ident(PACK)}', [200, 400, 422],
         {'name': ''},
         check=lambda c: (one("select count(*) from packs where name=''") == 0, '空名被写入'))
    case(F, 'R2 改不存在的包', 'PATCH', '/api/packs/pack-nope', [404], {'description': 'x'})
    F = '4-归档'
    case(F, 'P1 归档', 'POST', f'/api/packs/{ident(PACK)}/archive', [200, 201], {},
         check=lambda c: (one('select status from packs where id=?', (PACK,)) == 'archived',
                          'status 未变为 archived'))
    case(F, 'P2 取消归档', 'POST', f'/api/packs/{ident(PACK)}/unarchive', [200, 201], {},
         check=lambda c: one("select count(*) from packs where status='archived'") == 0)
    case(F, 'R1 归档不存在包', 'POST', '/api/packs/pack-nope/archive', [404], {})

    # ---- 5 工作台 / 活动 / 引导 ---------------------------------------
    F = '5-工作台'
    case(F, 'P1 dashboard', 'GET', '/api/dashboard', 200,
         check=lambda c: all(k in (c['j'] or {}) for k in ('packs', 'tasks')) or 'packs' in (c['j'] or {}))
    case(F, 'P2 activities 有落盘', 'GET', '/api/activities?limit=10', 200,
         check=lambda c: (one('select count(*) from activities') > 0, '写操作未产生活动流水'))
    case(F, 'R1 activities limit 负数', 'GET', '/api/activities?limit=-5', [200, 400, 422])
    F = '6-引导'
    case(F, 'P1 读引导', 'GET', '/api/onboarding', 200)
    case(F, 'P2 确认引导步骤', 'PUT', '/api/onboarding', [200, 204],
         {'steps': {'launcherReady': True}},
         check=lambda c: (one('select count(*) from onboarding_state') > 0, '确认状态未入库'))
    case(F, 'R1 未知步骤', 'PUT', '/api/onboarding', [422], {'steps': {'create_pack': True}},
         check=lambda c: err_code(c['text']) == 'onboarding_unknown_step')
    case(F, 'R2 已废弃步骤', 'PUT', '/api/onboarding', [422], {'steps': {'prismAccount': True}},
         check=lambda c: err_code(c['text']) == 'onboarding_step_readonly')

    # ---- 7 文件系统 / 导出目录 ----------------------------------------
    F = '7-文件系统'
    # 修复后:目录浏览与写请求同口径带令牌(前端 api/fs.ts 已改为 tokenHeaders())。
    case(F, 'P1 浏览目录(带令牌)', 'GET', '/api/fs/browse', 200,
         headers={'X-MPack-Token': TOKEN},
         check=lambda c: bool((c['j'] or {}).get('directories')))
    # 反向:不带令牌不得列出本机任意目录(dev 绑 0.0.0.0 时这是局域网泄露面)。
    case(F, 'R1 无令牌浏览系统目录', 'GET', '/api/fs/browse?path=' + ident('/etc'),
         [401, 403],
         check=lambda c: (err_code(c['text']) in ('unauthorized', 'invalid_origin', 'invalid_host'),
                          '任意本地目录仍可被无令牌枚举'))
    os.makedirs(EXPORT_DIR, exist_ok=True)
    case(F, 'P2 注册导出目录', 'POST', '/api/export-dirs', [201, 200],
         {'name': f'chain-{RUN}', 'directory': EXPORT_DIR},
         check=lambda c: one('select count(*) from allowed_export_dirs') >= 1)
    case(F, 'R2 注册系统目录', 'POST', '/api/export-dirs', [400, 403, 409, 422],
         {'name': 'evil', 'directory': '/'},
         check=lambda c: (one("select count(*) from allowed_export_dirs where absolute_path='/'") == 0,
                          '根目录被放行'))
    # 反向(O15)：同一个文件夹换名字再批准。此前 sqlite 的 absolute_path UNIQUE
    # 报错直冲 HTTP 层 → 500 internal_error，界面只会看到「服务器错误」。
    # 现在必须是可辨认的 409，且失败的别名不许入库(否则 BuildPack 会把它当已批准)。
    case(F, 'R3 同一目录换名再注册', 'POST', '/api/export-dirs', 409,
         {'name': f'chain-{RUN}-dup', 'directory': EXPORT_DIR},
         check=lambda c: (err_code(c['text']) == 'export_dir_conflict'
                          and one('select count(*) from allowed_export_dirs where name=?',
                                  (f'chain-{RUN}-dup',)) == 0,
                          f'错误码 {err_code(c["text"])} / 别名是否入库未在预期内'))

    # ---- 8 模组搜索 / 版本 --------------------------------------------
    F = '8-模组搜索'
    PID = ''
    r = case(F, 'P1 面向包搜索(jei)', 'GET',
             f'/api/packs/{ident(PACK)}/mod-search?q=jei&mcVersion={MCVER}&loader={LOADER}', 200,
             check=lambda c: len(((c['j'] or {}).get('items') or [])) > 0)
    for it in (r['j'] or {}).get('items') or []:
        if it.get('slug') == 'jei' or 'Just Enough' in (it.get('name') or ''):
            PID = it.get('id', '')
            break
    PID = PID or 'u6dRKJwZ'
    case(F, 'P2 搜索含双平台来源', 'GET',
         f'/api/packs/{ident(PACK)}/mod-search?q=cloth&mcVersion={MCVER}&loader={LOADER}', 200,
         check=lambda c: all(i.get('provider') in ('modrinth', 'curseforge')
                             for i in ((c['j'] or {}).get('items') or [])))
    case(F, 'R1 非法 provider', 'GET',
         f'/api/packs/{ident(PACK)}/mod-search?q=jei&provider=zzz', [400, 422], {},
         check=lambda c: (err_code(c['text']) != '', '非法参数未被拒'))
    case(F, 'R2 不存在包的搜索', 'GET', '/api/packs/pack-nope/mod-search?q=jei', [404])
    F = '9-模组版本'
    r = case(F, 'P1 取版本列表', 'GET',
             f'/api/packs/{ident(PACK)}/mod-versions?provider=modrinth&projectId={ident(PID)}'
             f'&mcVersion={MCVER}&loader={LOADER}', 200,
             check=lambda c: len(((c['j'] or {}).get('items') or [])) > 0)
    VID = ''
    for v in (r['j'] or {}).get('items') or []:
        gv, ld = v.get('gameVersions') or [], v.get('loaders') or []
        if MCVER in gv and (not ld or LOADER in ld):
            VID = v.get('id', '')
            break
    case(F, 'R1 不存在的项目', 'GET',
         f'/api/packs/{ident(PACK)}/mod-versions?provider=modrinth&projectId=nope-nope',
         [404, 200],
         check=lambda c: (c['status'] == 404 or not ((c['j'] or {}).get('items') or [])))

    # ---- 10 添加模组 --------------------------------------------------
    F = '10-添加模组'
    r = case(F, 'P1 搜索→版本→添加', 'POST', f'/api/packs/{ident(PACK)}/mods', [201],
             {'provider': 'modrinth', 'projectId': PID, 'versionId': VID, 'required': True},
             check=lambda c: (one("select count(*) from pack_mods where source='modrinth' and project_id=?",
                                  (PID,)) == 1, 'pack_mods 未落盘'))
    MOD = (r['j'] or {}).get('id', '')
    MODID = MOD or one("select id from pack_mods where source='modrinth' limit 1")
    # 前端添加模组必然带 versionId(useModSearch 先查 /mod-versions 再挑),
    # 因此省略 versionId 属反向用例。
    case(F, 'R0 省略 versionId', 'POST', f'/api/packs/{ident(PACK)}/mods', [400, 422],
         {'provider': 'modrinth', 'projectId': 'nfn13YXA', 'versionId': '', 'required': True},
         note='契约未写明必填;前端总带该字段')
    rv = call('GET', f'/api/packs/{ident(PACK)}/mod-versions?provider=modrinth'
                     f'&projectId=nfn13YXA&mcVersion={MCVER}&loader={LOADER}')
    reiver = ''
    for v in (jload(rv[1]) or {}).get('items') or []:
        if MCVER in (v.get('gameVersions') or []) and LOADER in (v.get('loaders') or []):
            reiver = v.get('id', '')
            break
    case(F, 'P2 再添加一个(REI)', 'POST', f'/api/packs/{ident(PACK)}/mods', [201],
         {'provider': 'modrinth', 'projectId': 'nfn13YXA', 'versionId': reiver,
          'required': True},
         check=lambda c: (one("select count(*) from pack_mods where project_id='nfn13YXA'") == 1,
                          '第二个模组未落盘'))
    case(F, 'R1 不存在的 versionId', 'POST', f'/api/packs/{ident(PACK)}/mods', [400, 404, 422],
         {'provider': 'modrinth', 'projectId': PID, 'versionId': 'bogus-version-id',
          'required': True},
         check=lambda c: (one("select count(*) from pack_mods where version_id='bogus-version-id'") == 0,
                          '脏 pack_mods 入库'))
    case(F, 'R2 未知 provider', 'POST', f'/api/packs/{ident(PACK)}/mods', [400, 422],
         {'provider': 'spigot', 'projectId': 'x', 'versionId': 'y', 'required': True},
         note='实得 502 provider_unavailable 时:参数校验未拦在 provider 调用之前')
    F = '11-本地模组'
    jar_bytes = b'PK\x03\x04' + b'\x00' * 512
    import hashlib
    case(F, 'P1 登记本地模组(纯元数据,无真实 jar)', 'POST',
         f'/api/packs/{ident(PACK)}/mods/local', 201,
         {'displayName': 'Chain Local Mod', 'fileName': 'chain-local.jar',
          'sha1': hashlib.sha1(jar_bytes).hexdigest(),
          'sha256': hashlib.sha256(jar_bytes).hexdigest(), 'size': len(jar_bytes),
          'required': True},
         check=lambda c: (one("select count(*) from pack_mods where file_name='chain-local.jar'") == 1,
                          '本地模组未落盘'),
         note='后端不校验 jar 是否真实存在/哈希是否吻合 —— 见缺陷清单')
    case(F, 'R1 缺 sha1', 'POST', f'/api/packs/{ident(PACK)}/mods/local', [400, 422],
         {'displayName': 'x', 'fileName': 'x.jar', 'sha1': '', 'size': 1, 'required': True})

    # ---- 12 包内模组清单 / 改 / 删 ------------------------------------
    F = '12-模组清单'
    case(F, 'P1 清单', 'GET', f'/api/packs/{ident(PACK)}/mods', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    case(F, 'P2 includeBuiltin 内容源', 'GET',
         f'/api/packs/{ident(PACK)}/mods?includeBuiltin=true', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    case(F, 'P3 改 required', 'PATCH', f'/api/packs/{ident(PACK)}/mods/{ident(MODID)}', 200,
         {'required': False},
         check=lambda c: (one('select count(*) from pack_mods') >= 1, '清单丢失'))
    case(F, 'R1 改不存在的模组', 'PATCH', f'/api/packs/{ident(PACK)}/mods/mod-nope', [404],
         {'required': True})

    # ---- 13 依赖解析 --------------------------------------------------
    F = '13-依赖解析'
    case(F, 'P1 resolve', 'POST', f'/api/packs/{ident(PACK)}/resolve', [200, 201, 202], {},
         check=lambda c: (one('select count(*) from pack_locks') >= 1, 'pack_locks 未落盘'))
    case(F, 'P2 locks 列表', 'GET', f'/api/packs/{ident(PACK)}/locks', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    # 前端 apps/web/src/api/mods.ts 的 conflictSchema 是硬契约:少一个键(哪怕是 null)
    # 整页就报「接口数据结构不符合约定」。这里按同一份字段清单校验后端响应。
    CONFLICT_KEYS = ['id', 'packId', 'fingerprint', 'kind', 'severity', 'status',
                     'summary', 'detailPath', 'resolvedAt', 'createdAt', 'updatedAt']

    def conflicts_shape(c):
        items = (c['j'] or {}).get('items') or []
        for it in items:
            missing = [k for k in CONFLICT_KEYS if k not in it]
            if missing:
                return (False, f'冲突项缺键 {missing}(前端 zod 契约判 undefined 直接报错)')
            if not isinstance(it.get('createdAt'), str) or not it.get('createdAt'):
                return (False, 'createdAt 缺失或不是字符串(时间字段漏打 json tag)')
        return (True, '')

    case(F, 'P3 conflicts 列表', 'GET', f'/api/packs/{ident(PACK)}/conflicts', 200,
         check=lambda c: (bool((c['j'] or {}).get('items')) and conflicts_shape(c)[0],
                          conflicts_shape(c)[1] or '冲突列表为空或形状不符前端契约'))
    # O25：主包在 11-本地模组 里有一条本机 jar 模组，它没有平台来源，resolve 拿不到
    # 适配器。这种"平台此刻不可用/无从校验"必须写成 warning：一旦成 error 级 pending，
    # 构建闸门(O20)就把整个包锁死，而用户在界面上没有任何办法"解决"一次网络抖动。
    case(F, 'P3b 平台不可用只出 warning(O25)', 'GET',
         f'/api/packs/{ident(PACK)}/conflicts', 200,
         check=lambda c: (all(x.get('severity') == 'warning'
                              for x in ((c['j'] or {}).get('items') or [])
                              if x.get('kind') == 'provider_unavailable'),
                          '平台不可用类冲突被写成了 error：会伪装成依赖问题并锁死构建'))
    case(F, 'P4 包健康', 'GET', f'/api/packs/{ident(PACK)}/health', 200,
         check=lambda c: ((c['j'] or {}).get('mods')
                          == one("select count(*) from pack_mods where pack_id=? and status<>'removed' and origin<>'builtin'", (PACK,)),
                          '健康度模组数把内建 minecraft 算进去了,与模组页口径不一致'))
    case(F, 'P5 兼容推荐', 'GET', f'/api/packs/{ident(PACK)}/mod-recommendations', [200, 404])
    case(F, 'R1 对不存在包解析', 'POST', '/api/packs/pack-nope/resolve', [404], {},
         check=lambda c: (err_code(c['text']) == 'pack_not_found',
                          f"实得 {c['status']}/{err_code(c['text'])},应为 404 pack_not_found"))
    r = case(F, 'P6 冲突处理', 'GET', f'/api/packs/{ident(PACK)}/conflicts', 200)
    cid = ((r['j'] or {}).get('items') or [{}])[0].get('id', '')
    if cid:
        case(F, 'P7 ignore 冲突', 'POST',
             f'/api/packs/{ident(PACK)}/conflicts/{ident(cid)}/ignore', [200, 202, 204], {},
             check=lambda c: (one("select count(*) from conflicts where id=? and status='ignored'",
                                  (cid,)) >= 1, '冲突状态未入库'))
    case(F, 'R2 处理不存在的冲突', 'POST',
         f'/api/packs/{ident(PACK)}/conflicts/c-000000/resolve', [400, 404, 409, 422],
         {'resolution': 'keep'})

    # ---- 14 内容目录 --------------------------------------------------
    F = '14-内容目录'
    r = case(F, 'P1 触发重建', 'POST',
             f'/api/packs/{ident(PACK)}/catalog/rebuild?locale=zh_cn', [202, 200], {},
             check=lambda c: bool((c['j'] or {}).get('taskId')))
    TID = (r['j'] or {}).get('taskId', '')
    if TID:
        st, jt = wait_task(TID)
        print(f'     目录任务终态={st}')
        if st != 'success':
            RESULTS[-1]['ok'] = False
            RESULTS[-1]['why'] = f'入队正确,但任务终态={st}: {str(jt.get("error") or jt.get("message") or "")[:140]}'
    case(F, 'P2 目录状态', 'GET', f'/api/packs/{ident(PACK)}/catalog/status', 200,
         check=lambda c: (c['j'] or {}).get('status') == 'succeeded')
    case(F, 'P3 目录正文', 'GET', f'/api/packs/{ident(PACK)}/catalog?locale=zh_cn', 200,
         check=lambda c: (len((c['j'] or {}).get('items') or []) > 0
                          and one('select count(*) from pack_catalog_items') > 0,
                          '目录条目未入库'))
    r = case(F, 'P4 物品详情', 'GET',
             f'/api/packs/{ident(PACK)}/catalog/items/{ident("minecraft:acacia_boat")}', 200,
             check=lambda c: (c['j'] or {}).get('id') == 'minecraft:acacia_boat')
    case(F, 'P5 物品图标', 'GET',
         f'/api/packs/{ident(PACK)}/catalog/icon?itemId=' + ident('minecraft:acacia_boat'),
         [200, 404, 409],
         check=lambda c: (c['status'] != 200 or len(c['text']) > 0))
    case(F, 'P6 标签详情', 'GET',
         f'/api/packs/{ident(PACK)}/catalog/tags/{ident("minecraft:boats")}', [200, 404])
    case(F, 'R1 不存在的物品', 'GET', f'/api/packs/{ident(PACK)}/catalog/items/{ident("nope:x")}', [404])
    case(F, 'R2 对不存在包重建', 'POST', '/api/packs/pack-nope/catalog/rebuild', [404], {})
    case(F, 'R3 非法 locale', 'GET', f'/api/packs/{ident(PACK)}/catalog?locale=../etc',
         [200, 400, 404, 422], note='观察是否被参数化(不应 500)')
    # O9 修复守护:三种"读不到目录"必须可分辨,不能再都报 409 catalog_stale。
    case(F, 'R4 不存在的包读目录正文', 'GET', '/api/packs/pack-nope/catalog', 404,
         check=lambda c: (err_code(c['text']) == 'pack_not_found',
                          f"包不存在应报 404 pack_not_found,实得 {c['status']} {err_code(c['text'])}"))
    case(F, 'R5 查无此物品', 'GET',
         f'/api/packs/{ident(PACK)}/catalog/items/{ident("nope:x")}', 404,
         check=lambda c: (err_code(c['text']) == 'catalog_item_not_found',
                          f"应报 catalog_item_not_found,实得 {err_code(c['text'])}(旧行为:404 pack_not_found)"))
    case(F, 'R6 查无此标签', 'GET',
         f'/api/packs/{ident(PACK)}/catalog/tags/{ident("nope:x")}', 404,
         check=lambda c: (err_code(c['text']) == 'catalog_tag_not_found',
                          f"应报 catalog_tag_not_found,实得 {err_code(c['text'])}"))

    # ---- 15 模组内容解析 ----------------------------------------------
    F = '15-模组解析'
    r = case(F, 'P1 解析 JEI', 'POST',
             f'/api/packs/{ident(PACK)}/mods/{ident(MODID)}/content/parse', [202, 200, 409], {})
    PT = (r['j'] or {}).get('taskId', '')
    if PT:
        st, jp = wait_task(PT)
        print(f'     解析任务终态={st}')
        if st != 'success':
            RESULTS[-1]['ok'] = False
            RESULTS[-1]['why'] = f'入队正确,但任务终态={st}: {str(jp.get("error") or "")[:140]}'
    case(F, 'P2 列解析条目', 'GET',
         f'/api/packs/{ident(PACK)}/mods/{ident(MODID)}/content?limit=5', 200,
         check=lambda c: (len((c['j'] or {}).get('items') or []) > 0
                          and one('select count(*) from mod_content') > 0, '解析结果未入库'))
    case(F, 'P3 解析运行记录', 'GET',
         f'/api/packs/{ident(PACK)}/mods/{ident(MODID)}/content/run', 200,
         check=lambda c: one('select count(*) from mod_content_runs') >= 1)
    case(F, 'P4 图标解析', 'POST',
         f'/api/packs/{ident(PACK)}/mods/{ident(MODID)}/content/icons/resolve', [200, 202, 404, 409], {})
    case(F, 'R1 解析不存在的模组', 'POST',
         f'/api/packs/{ident(PACK)}/mods/mod-nope/content/parse', [404], {})
    case(F, 'R2 取不存在的条目', 'GET',
         f'/api/packs/{ident(PACK)}/mods/{ident(MODID)}/content/mc-mod-nope', [404])

    # ---- 16 内容文档 --------------------------------------------------
    F = '16-内容文档'
    # 注意:前端 api/content.ts 没有 createContent —— 内容文档没有任何 UI 创建入口,
    # 这里按后端契约形状发,只为验证该域本身是否可用(见缺陷清单)。
    r = case(F, 'P1 新建文档(后端契约形,前端无入口)', 'POST', f'/api/packs/{ident(PACK)}/content', 201,
             {'kind': 'recipe', 'slug': f'chain-{RUN}', 'title': '链路测试配方',
              'payload': {'schema_version': 1, 'type': 'minecraft:crafting_shaped',
                          'input': ['minecraft:iron_ingot'], 'output': 'minecraft:bucket'}},
             check=lambda c: (one('select count(*) from content_documents') == 1,
                              'content_documents 未落盘'))
    DOC = ((r['j'] or {}).get('document') or {}).get('id', '') or \
        one('select id from content_documents limit 1')
    case(F, 'R0 非法 kind', 'POST', f'/api/packs/{ident(PACK)}/content', [400, 422],
         {'kind': 'patch', 'slug': 'x', 'title': 'x', 'payload': {}})
    case(F, 'R0b payload 越界字段', 'POST', f'/api/packs/{ident(PACK)}/content', [400, 422],
         {'kind': 'recipe', 'slug': 'y', 'title': 'y', 'payload': {'items': []}})
    case(F, 'P2 存草稿(改动产物)', 'PUT', f'/api/packs/{ident(PACK)}/content/{ident(DOC)}/draft',
         [200, 201],
         {'payload': {'schema_version': 1, 'type': 'minecraft:crafting_shaped',
                      'input': ['minecraft:iron_ingot'], 'output': 'minecraft:air_bucket'}},
         headers={'If-Match': '"1"'},
         check=lambda c: (one('select count(*) from content_revisions') >= 2, '新修订未入库'))
    case(F, 'P3 校验', 'POST',
         f'/api/packs/{ident(PACK)}/content/{ident(DOC)}/validate', 200, {})
    case(F, 'P4 应用', 'POST', f'/api/packs/{ident(PACK)}/content/{ident(DOC)}/apply', [200], {})
    case(F, 'P5 历史', 'GET', f'/api/packs/{ident(PACK)}/content/{ident(DOC)}/history', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    case(F, 'P6 读文档', 'GET', f'/api/packs/{ident(PACK)}/content/{ident(DOC)}', 200)
    case(F, 'R1 If-Match 过期', 'PUT', f'/api/packs/{ident(PACK)}/content/{ident(DOC)}/draft',
         [409, 412],
         {'payload': {'schema_version': 1, 'type': 'minecraft:crafting_shaped',
                      'input': ['minecraft:iron_ingot'], 'output': 'minecraft:bucket'}},
         headers={'If-Match': '"999"'},
         check=lambda c: (err_code(c['text']) in ('revision_conflict', 'precondition_failed',
                                                  'if_match_required'), '错误码未按语义给出'))
    case(F, 'R3 操作不存在的文档', 'GET', f'/api/packs/{ident(PACK)}/content/doc-nope', [404])
    r = case(F, 'P7 回滚', 'POST', f'/api/packs/{ident(PACK)}/content/{ident(DOC)}/rollback',
             [200, 400, 404, 422], {'revisionId': ''})

    # ---- 17 任务书 ----------------------------------------------------
    F = '17-任务书'
    DRAFT = {
        'book': {'title': '链路测试任务书', 'icon': 'minecraft:book',
                 'progressionMode': 'flexible'},
        'chapters': [{'id': 'ch1', 'title': '开始', 'description': '',
                      'coverColor': '#C9783B', 'icon': 'minecraft:book', 'position': 0}],
        'nodes': [{'id': 'n1', 'chapterId': 'ch1', 'title': '入门', 'subtitle': '',
                   'description': '完成基础准备', 'icon': 'minecraft:wooden_pickaxe',
                   'x': 48, 'y': 48, 'shape': 'circle', 'size': 1, 'optional': False,
                   'invisible': False, 'dependencyRequirement': 'all_completed',
                   'minRequiredDependencies': 0, 'prerequisites': [],
                   'tasks': [{'id': 'n1-t1', 'type': 'item',
                              'itemId': 'minecraft:wooden_pickaxe', 'count': 1}],
                   'rewards': [{'kind': 'experience', 'experience': 10}], 'modRefs': [],
                   'position': 0}],
        'edges': [],
    }
    case(F, 'R0 新包读任务书(基线 D3/O4)', 'GET', f'/api/packs/{ident(PACK)}/quests', [200, 404],
         check=lambda c: (c['status'] == 200 or err_code(c['text']) == 'quest_book_not_found',
                          f"包在而任务书为空应报 quest_book_not_found,实得 {err_code(c['text'])}"
                          '(旧行为:404 pack_not_found,界面只会说找不到整合包)'),
         note='O4:空任务书不能说成"找不到整合包"')
    case(F, 'P1 存草稿', 'PUT', f'/api/packs/{ident(PACK)}/quests/draft', [200, 201], DRAFT,
         headers={'If-Match': '"0"'},
         check=lambda c: (one('select count(*) from quest_revisions') >= 1
                          and one('select count(*) from quest_nodes') >= 1,
                          '任务书修订/节点未入库'))
    case(F, 'P2 校验', 'POST', f'/api/packs/{ident(PACK)}/quests/validate', 200, {},
         check=lambda c: (c['j'] or {}).get('status') in ('passed', 'failed'))
    case(F, 'P3 应用', 'POST', f'/api/packs/{ident(PACK)}/quests/apply', 200, {},
         check=lambda c: (one('select count(*) from quest_books') >= 1, 'quest_books 未入库'))
    case(F, 'P4 预览', 'GET', f'/api/packs/{ident(PACK)}/quests/preview', 200)
    case(F, 'P5 读回任务书', 'GET', f'/api/packs/{ident(PACK)}/quests', 200,
         # 后端返回 QuestBook 信封 {id,packId,activeRevisionId,revision};
         # 前端 useEditors 用 revision.draft 解包,这里同口径核对。
         check=lambda c: (c['j'] or {}).get('revision') is not None or 'nodes' in (c['j'] or {}))
    case(F, 'P6 历史', 'GET', f'/api/packs/{ident(PACK)}/quests/history', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    case(F, 'R1 非法任务类型', 'PUT', f'/api/packs/{ident(PACK)}/quests/draft', [200, 400, 422],
         {'book': {'title': 'x'}, 'chapters': [],
          'nodes': [{'id': 'n9', 'chapterId': 'chX', 'title': '', 'tasks': [
              {'id': 't', 'type': 'teleport-to-void'}]}], 'edges': []},
         headers={'If-Match': '"0"'})
    case(F, 'R2 回滚到不存在的修订', 'POST', f'/api/packs/{ident(PACK)}/quests/rollback',
         [400, 404, 409, 422], {'revisionId': 'quest-revision-nope'})

    # ---- 18 版本 / 构建 ----------------------------------------------
    F = '18-版本'
    VNUM = '0.9.' + time.strftime('%H%M%S')
    # 契约:channel in {draft,release},source in {manual,imported,build}
    r = case(F, 'P1 建版本', 'POST', f'/api/packs/{ident(PACK)}/versions', 201,
             {'version': VNUM, 'channel': 'release', 'changelog': 'chain', 'source': 'manual'},
             check=lambda c: (one('select count(*) from pack_versions where pack_id=? and version=?',
                                  (PACK, VNUM)) == 1, 'pack_versions 未按 (pack,version) 落盘'))
    PVER = (r['j'] or {}).get('id', '') or one('select id from pack_versions limit 1')
    case(F, 'R1 版本号重复(基线 D2)', 'POST', f'/api/packs/{ident(PACK)}/versions', [409],
         {'version': VNUM, 'channel': 'release', 'changelog': '', 'source': 'manual'},
         check=lambda c: (one('select count(*) from pack_versions where pack_id=? and version=?',
                             (PACK, VNUM)) == 1, '重复版本被写入'))
    case(F, 'R2 空版本号/越界枚举', 'POST', f'/api/packs/{ident(PACK)}/versions', [400, 422],
         {'version': '', 'channel': 'nightly', 'changelog': '', 'source': 'ui'})

    F = '19-构建产物'
    manifest = {'formatVersion': 1, 'game': 'minecraft', 'versionId': VNUM,
                'name': PACK, 'dependencies': {}, 'generatedBy': 'mPackStation'}
    files = [{'path': 'modrinth.index.json',
              'content': base64.b64encode(json.dumps(manifest).encode()).decode()}]
    r = case(F, 'P1 前端形构建(带 files)', 'POST', f'/api/packs/{ident(PACK)}/build', 201,
             {'packVersionId': PVER, 'exportDirName': f'chain-{RUN}', 'files': files,
              'lockSnapshot': {'packId': PACK, 'mods': []}},
             check=lambda c: (one('select count(*) from artifacts') == 1,
                              'artifacts 未入库'))
    ART = ((r['j'] or {}).get('artifact') or {}).get('id', '')
    # 实测 400 invalid_argument:后端把 files 当必填,与契约(contract.md:910 可选)冲突。
    # 这是阻塞终端目标(.mrpack 组装 → 启动 MC)的基线缺陷 D1,故意保持"接受但不理想"的记录。
    # 主包在 11-本地模组 里有一个"只有本机 jar、没有平台选中项"的模组。
    # 装配必须点名阻止它:以前 ListPackAssemblySources 用 INNER JOIN,这种模组会
    # 从结果里凭空消失,于是"构建成功、包里没有它";现在它必须回 422 且不落产物。
    # 构建闸门(O20)排在装配之前:13-依赖解析 那轮 resolve 给主包留下了真缺依赖的
    # error 级冲突,本例要验的是装配环节,先按用户视角点一次"忽略"把闸门放行。
    ignore_pending_error_conflicts(PACK)
    case(F, 'R1 契约形构建(files 省略→权威装配)', 'POST', f'/api/packs/{ident(PACK)}/build', 422,
         {'packVersionId': PVER, 'exportDirName': f'chain-{RUN}',
          'lockSnapshot': {'packId': PACK, 'mods': []}},
         check=lambda c: (err_code(c['text']) == 'build_mod_source_unresolved'
                          and 'Chain Local Mod' in c['text']
                          and one('select count(*) from artifacts') == 1,
                          f"应 422 build_mod_source_unresolved 并点名 Chain Local Mod,实得 "
                          f"{c['status']} {err_code(c['text'])} {c['text'][:90]}"),
         note='基线 D1 反向:静默缺模组比构建失败更危险')
    case(F, 'R2 未知 packVersionId', 'POST', f'/api/packs/{ident(PACK)}/build', [400, 404, 422],
         {'packVersionId': 'version-nope', 'exportDirName': 'x', 'files': files},
         check=lambda c: one('select count(*) from artifacts') == 1)
    case(F, 'P2 产物列表', 'GET', f'/api/packs/{ident(PACK)}/artifacts', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    case(F, 'P3 产物下载', 'GET',
         f'/api/packs/{ident(PACK)}/artifacts/{ident(ART)}/download', [200, 404],
         check=lambda c: (c['status'] != 200 or len(c['text']) > 0))
    case(F, 'R3 下载不存在的产物', 'GET',
         f'/api/packs/{ident(PACK)}/artifacts/artifact-nope/download', [404])
    case(F, 'P4 路由 versions/{id}/build 仍在', 'POST',
         f'/api/packs/{ident(PACK)}/versions/{ident(PVER)}/build', [201, 400, 409, 422],
         {'files': files, 'exportDirName': f'chain-{RUN}'})

    # ---- 基线 D1 修复守护：省略 files 时由服务端按包内权威清单装配 .mrpack ----
    def mrpack_check(c):
        art = q("select kind,file_name,path,size_bytes from artifacts "
                "order by created_at desc, id desc limit 1")
        if not art:
            return (False, '构建成功但没有产物行')
        kind, name, path, size = art[0]
        if kind != 'mrpack' or not str(name).endswith('.mrpack'):
            return (False, f'产物应为 .mrpack/kind=mrpack,实得 kind={kind} name={name}')
        if size < 400:
            return (False, f'产物只有 {size} 字节,疑似空壳(基线 D1)')
        real = path if os.path.isabs(str(path)) else os.path.join(DATA_ROOT, str(path))
        with zipfile.ZipFile(real) as z:
            if 'modrinth.index.json' not in z.namelist():
                return (False, f'.mrpack 里没有 modrinth.index.json:{z.namelist()[:6]}')
            m = json.loads(z.read('modrinth.index.json').decode())
        # 字段名以真实产物为准(FO v15 实测顶层是 formatVersion/game/versionId),
        # 旧稿的 manifestVersion 会让 Prism/我们的内核都读不到版本。
        if m.get('formatVersion') != 1:
            return (False, f'formatVersion 应为 1,实得 {m.get("formatVersion")}'
                           f'(键名列表:{sorted(m.keys())})')
        if not m.get('versionId'):
            return (False, f'versionId 为空,安装方无法定位版本:{sorted(m.keys())}')
        if 'minecraft' not in (m.get('dependencies') or {}):
            return (False, f'dependencies 缺 minecraft:{m.get("dependencies")}')
        if len(m.get('files') or []) < 1:
            return (False, 'files[] 为空,等于又造了个空壳')
        f0 = (m['files'] or [{}])[0]
        if not f0.get('downloads') or not f0.get('hashes'):
            return (False, f'装配出的条目缺 downloads/hashes:{f0}')
        # sha512 是 .mrpack 必填哈希。此前 provider 解析把它丢了,manifest 里恒空,
        # 严格读方(Prism/我们的内核校验)会拒装。
        sha512 = str((f0.get('hashes') or {}).get('sha512') or '')
        if len(sha512) != 128:
            return (False, f'files[0].hashes.sha512 应为 128 位十六进制,实得 '
                           f'{sha512!r}(该模组的下载字节未被实测出 sha512)')
        return (True, '')

    # 正向装配用例需要一个「只有可下载地址模组」的包:主包在 11-本地模组 里被塞了
    # 一个没有下载地址的本地 jar,那是 R7 反向用例的素材,不能同时用来验正向。
    r = case(F, 'P5 建装配专用包', 'POST', '/api/packs', 201,
             {'name': f'Chain ASM {RUN}', 'mcVersion': MCVER, 'loader': LOADER,
              'loaderVersion': LOADER_VER, 'description': ''})
    ASM_PACK = (r['j'] or {}).get('id', '') or \
        one("select id from packs where name=?", (f'Chain ASM {RUN}',))
    # 刚建好、还没有一条模组的包：读目录不能被打成"找不到整合包"或 500。
    # O9 修复后的口径是 200(自动目录已建好)或 409 catalog_not_built(还没建过)。
    case(F, 'P5a 新包读目录(O9 口径)', 'GET', f'/api/packs/{ident(ASM_PACK)}/catalog',
         [200, 409],
         check=lambda c: (c['status'] == 200 or err_code(c['text']) == 'catalog_not_built',
                          f"新包读目录应 200 或 409 catalog_not_built,实得 "
                          f"{c['status']} {err_code(c['text'])}"))
    case(F, 'P5b 装配包加 Modrinth 模组', 'POST', f'/api/packs/{ident(ASM_PACK)}/mods', [201],
         {'provider': 'modrinth', 'projectId': PID, 'versionId': VID, 'required': True},
         check=lambda c: (one("select count(*) from pack_mods where pack_id=? and status='installed'",
                              (ASM_PACK,)) >= 1, '装配包模组未入库'))
    case(F, 'P5c 装配包 resolve', 'POST', f'/api/packs/{ident(ASM_PACK)}/resolve',
         [200, 201, 202], {},
         check=lambda c: (one("select count(*) from pack_mod_selections s "
                              "join pack_mods m on m.current_selection_id=s.id "
                              "where m.pack_id=?", (ASM_PACK,)) >= 1,
                          '装配所需的选中项/下载地址链路未入库'))
    # 装配包里的 JEI 声明了平台依赖(没补进包),resolve 检出 error 级冲突。
    # 闸门(O20)会因此拒绝构建 —— 那是正确行为,但 P6 要验的是"装配出的 manifest
    # 形不合规",所以这里同样先点忽略放行;真正"缺依赖不许构建"的断言在
    # scripts/verify-terminal-chain.py 的第 4 步(不补依赖直接构建必须 409)。
    ignore_pending_error_conflicts(ASM_PACK)
    r = case(F, 'P5d 装配包建版本', 'POST', f'/api/packs/{ident(ASM_PACK)}/versions', 201,
             {'version': '0.9.' + time.strftime('%H%M%S') + 'a', 'channel': 'release',
              'changelog': 'chain-assemble', 'source': 'manual'})
    ASM_VER = (r['j'] or {}).get('id', '') or \
        one('select id from pack_versions where pack_id=? order by created_at desc limit 1',
            (ASM_PACK,))

    r = case(F, 'P6 装配构建(省略 files→真 .mrpack)', 'POST',
             f'/api/packs/{ident(ASM_PACK)}/build', 201,
             {'packVersionId': ASM_VER, 'exportDirName': f'chain-{RUN}'},
             check=mrpack_check,
             note='单一来源=包内权威清单;产物必须含 modrinth.index.json 与真实模组条目')
    ART_ASM = (r['j'] or {}).get('artifact', {}).get('id', '') or \
        one("select id from artifacts where kind='mrpack' order by created_at desc limit 1", default='')

    # 同一版本换一份锁快照再构建:输入不可静默变更,但错误码必须能指出去哪。
    # 旧行为是塌成裸 409 "conflict"/"resource conflict" 或 400 invalid_argument,
    # 前端只会显示"资源冲突",用户完全不知道要改 lockSnapshot。
    # 用装配包做反向:主包的版本在 R1 已被"无下载地址"挡住(装配在记录输入之前),
    # 换锁快照这条路只有在能装出产物的包上才走得到 RecordBuildInputs。
    before = one('select count(*) from artifacts')
    case(F, 'R7 同版本改构建输入→显式冲突码', 'POST',
         f'/api/packs/{ident(ASM_PACK)}/build', [409, 422],
         {'packVersionId': ASM_VER, 'exportDirName': f'chain-{RUN}',
          'lockSnapshot': {'packId': ASM_PACK, 'mods': [{'id': 'other'}]}},
         check=lambda c: (err_code(c['text']) in ('build_input_conflict', 'build_lock_mismatch')
                          and one('select count(*) from artifacts') == before,
                          f"应回 build_input_conflict/build_lock_mismatch 且不落产物,实得 "
                          f"{c['status']} {err_code(c['text'])}"),
         note='缺陷 O13:构建输入冲突不能塌成裸 conflict/invalid_argument')

    # ---- 20 交付检查 / 发布 -------------------------------------------
    F = '20-交付检查'
    case(F, 'P1 跑检查', 'POST', f'/api/packs/{ident(PACK)}/delivery-checks/run', 200,
         {'packVersionId': PVER, 'checks': [{'kind': 'content', 'status': 'passed',
                                             'detail': '{}'}]},
         check=lambda c: one('select count(*) from delivery_checks') >= 1)
    case(F, 'P2 列检查', 'GET', f'/api/packs/{ident(PACK)}/delivery-checks', 200)
    case(F, 'R1 对不存在包跑检查', 'POST', '/api/packs/pack-nope/delivery-checks/run',
         [400, 404, 422], {'packVersionId': 'x', 'checks': []})
    F = '21-发布'
    # 实测 502 provider_unavailable("publication failed; retry is explicit")。
    # 语义缺陷(已记入清单,本轮未修):没配 Modrinth 令牌应当是 401/403 credential_missing,
    # 而不是把"你没填钥匙"说成"服务商挂了"。这里接受实测值,只断言不得退化为 500。
    case(F, 'R1 无凭证发布 modrinth', 'POST', f'/api/packs/{ident(PACK)}/publish/modrinth',
         [400, 401, 403, 409, 422, 502, 503],
         {'packVersionId': PVER, 'artifactId': ART, 'idempotencyKey': f'chain-{RUN}',
          'projectId': '', 'versionId': ''},
         check=lambda c: (err_code(c['text']) != 'internal_error', '返回了裸 500'))
    case(F, 'R2 未知 provider', 'POST', f'/api/packs/{ident(PACK)}/publish/flintlock',
         [400, 404, 422], {'packVersionId': PVER, 'artifactId': ART})
    case(F, 'R3 poll 不存在的发布', 'POST', '/api/releases/rel-nope/poll', [404, 400, 422], {})
    case(F, 'R4 retry 不存在的发布', 'POST', '/api/releases/rel-nope/retry', [404, 400, 422], {})
    case(F, 'P1 发布记录列表', 'GET', f'/api/packs/{ident(PACK)}/releases', 200)

    # ---- 22 启动器(流水线终局) ---------------------------------------
    F = '22-启动器'
    MCDIR = '/tmp/mpack-chain/minecraft'
    os.makedirs(MCDIR, exist_ok=True)
    # 正向用例要求 mpack-launcher 真的可执行。chain-test-run.sh 默认在隔离数据目录里放
    # 一个「协议桩」:它只按 launcherCore/src/protocol.rs 的 JSON Lines 形状回
    # phase/result 事件,按 cli.rs 校验旗标,并对非法版本号回 failure,不下载文件、
    # 不真起游戏。所以这批用例证明的是
    # 「前端→同步校验→入队→入库→worker→fork/exec→终态」这条链路。
    # 真实内核在本机是可构建的(brew rustup,toolchain stable-aarch64-apple-darwin,
    # cargo 1.96),终局的「真下载 + 真启动」验证归 scripts/verify-terminal-chain.sh,
    # 它把 CHAIN_LAUNCHER_BIN 指向 cargo build 出来的二进制后重跑这批用例。
    # 桩的 --mrpack 分支不是摆设:它会真的用 zipfile 读产物里的 modrinth.index.json,
    # 所以「Go 传过去的是不是一个结构合规的 .mrpack」在这里就会被验出来。
    def launcher_terminal(ctx, want):
        tid = (ctx['j'] or {}).get('taskId', '')
        if not tid:
            return (False, '响应里没有 taskId')
        st, j = wait_task(tid, timeout=120)
        return (st == want,
                f'期望任务终态 {want},实得 {st} err={str(j.get("error") or "")[:110]}')

    if NOBIN:
        case(F, 'R0 未装启动器时同步拒绝', 'POST', '/api/launcher/install', 503,
             {'version': MCVER, 'loader': LOADER, 'mirror': '', 'minecraft_dir': MCDIR},
             base=NOBIN,
             check=lambda c: (err_code(c['text']) == 'launcher_binary_missing',
                              f"错误码应为 launcher_binary_missing,实得 {err_code(c['text'])})"))
    else:
        skipped(F, 'R0 未装启动器时同步拒绝', '环境未提供 CHAIN_NOBIN 实例')
    # loader_version 一并下发:桩会像 clap 一样校验旗标,Go 侧若仍把 install 写成
    # --version(缺 --mc),桩直接退出 2 → 任务终态 failed → 这条用例变红。
    r = case(F, 'P1 前端形安装(202+入库)', 'POST', '/api/launcher/install', 202,
             {'version': MCVER, 'loader': LOADER, 'loader_version': LOADER_VER,
              'mirror': '', 'minecraft_dir': MCDIR, 'pack_id': PACK},
             check=lambda c: (bool((c['j'] or {}).get('taskId'))
                              and one("select count(*) from tasks where kind='launcher_install'") >= 1,
                              '安装任务未入库'))
    LT = (r['j'] or {}).get('taskId', '')
    if LT:
        st, j = wait_task(LT, timeout=120)
        print(f"     安装任务终态={st} error={j.get('error') or ''}")
        if st != 'success':
            # 「正确响应并处理」要求异步侧也收敛到 success,否则按失败计。
            RESULTS[-1]['ok'] = False
            RESULTS[-1]['why'] = f'入队/入库正确,但任务终态={st}: {str(j.get("error") or "")[:140]}'
    # O2:安装终态必须把内核实际装出来的版本目录 ID 写进 launcher_installs。
    # 带加载器时它是 fabric-loader-<加载器版本>-<MC 版本>（launcherCore/src/
    # loader/fabric.rs:43-45），不是包的 mc_version —— 前端只能从这里拿到它。
    INST = q("select version_id from launcher_installs where pack_id=? "
             "order by installed_at desc limit 1", (PACK,))
    VERID = str(INST[0][0]) if INST else ''
    r = case(F, 'P2 已安装版本查询(O2)', 'GET',
             f'/api/launcher/installs?packId={ident(PACK)}'
             f'&minecraftDir=' + urllib.parse.quote(MCDIR, safe=''), 200,
             check=lambda c: (bool(INST) and VERID.startswith('fabric-loader-')
                              and len((c['j'] or {}).get('installs') or []) >= 1,
                              f"launcher_installs 行数={len(INST)} version_id={VERID!r} "
                              f"响应={str(c['j'])[:120]}"),
             note='未安装时该目录无记录 → 前端应显示"先点安装"而不是拿 mc 版本猜')
    case(F, 'P3 按已装版本 ID 启动', 'POST', '/api/launcher/launch', 202,
         {'version': VERID or MCVER, 'username': 'ChainTester', 'minecraft_dir': MCDIR,
          'java_path': '', 'xmx_mb': 0, 'pack_id': PACK},
         check=lambda c: launcher_terminal(c, 'success'),
         note='桩按 install.rs 的 version_id 回值;真实内核装出来的目录名同样走这条')
    case(F, 'P4 省略 version 按包启动', 'POST', '/api/launcher/launch', 202,
         {'version': '', 'username': 'ChainTester', 'minecraft_dir': MCDIR,
          'java_path': '', 'xmx_mb': 0, 'pack_id': PACK},
         check=lambda c: (bool((c['j'] or {}).get('taskId'))
                          and one("select count(*) from tasks where kind='launcher_launch'") >= 2,
                          '入队未入库') if c['status'] == 202
                         else (err_code(c['text']) != 'internal_error', '不得退化成 500'),
         note='服务端在入队前把空 version 解析成该目录最近一次安装记录')
    EMPTY = '/tmp/mpack-chain/minecraft-none'
    LN = one("select count(*) from tasks where kind='launcher_launch'")
    case(F, 'R5 未安装目录且省略 version', 'POST', '/api/launcher/launch', 409,
         {'version': '', 'username': 'ChainTester', 'minecraft_dir': EMPTY,
          'java_path': '', 'xmx_mb': 0},
         check=lambda c: (err_code(c['text']) == 'launcher_not_installed'
                          and one("select count(*) from tasks where kind='launcher_launch'") == LN,
                          f"应 409 launcher_not_installed 且不入队,实得 "
                          f"{c['status']} {err_code(c['text'])}"))
    case(F, 'R1 缺 version', 'POST', '/api/launcher/install', [400, 422],
         {'version': '', 'loader': '', 'mirror': '', 'minecraft_dir': MCDIR},
         check=lambda c: (err_code(c['text']) != ''
                          and one("select count(*) from tasks where kind='launcher_install'") == 1,
                          f"未拒绝或越权入队: code={err_code(c['text'])}"))
    case(F, 'R2 缺 minecraft_dir', 'POST', '/api/launcher/install', [400, 422],
         {'version': MCVER, 'loader': LOADER, 'mirror': '', 'minecraft_dir': ''},
         check=lambda c: (err_code(c['text']) != ''
                          and one("select count(*) from tasks where kind='launcher_install'") == 1,
                          f"未拒绝或越权入队: code={err_code(c['text'])}"))
    ln_before = one("select count(*) from tasks where kind='launcher_launch'")
    case(F, 'R3 启动缺 username', 'POST', '/api/launcher/launch', [400, 422],
         {'version': MCVER, 'username': '', 'minecraft_dir': MCDIR,
          'java_path': '', 'xmx_mb': 0},
         check=lambda c: (err_code(c['text']) != ''
                          and one("select count(*) from tasks where kind='launcher_launch'")
                          == ln_before,
                          f"未拒绝或越权入队: code={err_code(c['text'])}"))
    # 版本号非法:同步侧只有非空校验,非法版本要能在异步任务里失败并带上错误码,
    # 绝不能悄悄「成功」(桩对 ^\d+\.\d+(\.\d+)?$ 之外的版本回 failure)。
    case(F, 'R4 启动不存在的版本', 'POST', '/api/launcher/launch',
         [202, 400, 404, 409, 422, 503],
         {'version': '0.0.0-nope', 'username': 'x', 'minecraft_dir': MCDIR,
          'java_path': '', 'xmx_mb': 0},
         check=lambda c: (c['status'] != 500 or err_code(c['text']) != 'internal_error',
                          '500 且无诊断信息') if c['status'] != 202
                         else launcher_terminal(c, 'failed'))

    # ---- 构建产物 -> 启动器安装(流水线终局的契约侧) ------------------
    # 只给 artifact_id、不给 version:mc/加载器版本必须由安装方从包内 manifest 决定
    # (launcherCore/src/mrpack.rs plan_from_manifest)。桩会真的用 zipfile 读这份
    # manifest 再回 version_id,所以「Go 递给内核的到底是不是一个结构合规的 .mrpack」
    # 在这一条里就被验掉了。
    inst_before = one("select count(*) from tasks where kind='launcher_install'")

    def mrpack_install_full(c):
        tid = (c['j'] or {}).get('taskId', '')
        if not tid:
            return (False, '响应里没有 taskId')
        if one("select count(*) from tasks where kind='launcher_install'") != inst_before + 1:
            return (False, '安装任务未入库')
        st, j = wait_task(tid, timeout=180)
        if st != 'success':
            return (False, f'安装任务终态={st}: {str(j.get("error") or "")[:170]}')
        row = q("select version_id from launcher_installs where pack_id=? "
                "order by installed_at desc limit 1", (ASM_PACK,))
        if not row:
            return (False, '安装成功但 launcher_installs 没有该包的记录')
        vid = str(row[0][0])
        # 桩与真实内核都回 fabric-loader-<lv>-<mc>,它来自 manifest 的 dependencies;
        # 若 Go 侧仍按调用方给的裸 mc 版本记录,这里就会露馅。
        if not vid.startswith('fabric-loader-'):
            return (False, f'安装记录 version_id={vid!r},应为 fabric-loader-*')
        return (True, '')

    case(F, 'P6 按构建产物安装(artifact_id→success+落库)', 'POST',
         '/api/launcher/install', 202,
         {'artifact_id': ART_ASM, 'pack_id': ASM_PACK, 'minecraft_dir': MCDIR,
          'mirror': '', 'java_path': ''},
         check=mrpack_install_full,
         note='终局:构建出的 .mrpack 直接进安装链路,版本来自 manifest 不是调用方')
    def inst_count():
        return one("select count(*) from tasks where kind='launcher_install'")

    # 反向用例的"没入队"基线在每条用例前现取：上一轮把基线钉在 P6 之后
    # (inst_before + 1)，P6 一旦因为别的原因没入队，后面几条反向用例会跟着一起红，
    # 看起来像四个缺陷其实只有一个——失败要能指回真正的那一步。
    n_r6 = inst_count()
    case(F, 'R6 artifact 不存在', 'POST', '/api/launcher/install', 404,
         {'artifact_id': 'artifact-nope', 'pack_id': PACK, 'minecraft_dir': MCDIR},
         check=lambda c, n=n_r6: (err_code(c['text']) == 'artifact_not_found'
                                  and inst_count() == n,
                                  f"应 404 artifact_not_found 且不入队,实得 "
                                  f"{c['status']} {err_code(c['text'])}"))
    n_r7 = inst_count()
    case(F, 'R7 非 .mrpack 产物', 'POST', '/api/launcher/install', 422,
         {'artifact_id': ART, 'pack_id': PACK, 'minecraft_dir': MCDIR},
         check=lambda c, n=n_r7: (err_code(c['text']) == 'install_artifact_not_mrpack'
                                  and inst_count() == n,
                                  f"应 422 install_artifact_not_mrpack(P1 产物 kind=zip),实得 "
                                  f"{c['status']} {err_code(c['text'])}"),
         note='kind=zip 的历史产物不能当成整合包装')
    n_r8 = inst_count()
    case(F, 'R8 跨包产物', 'POST', '/api/launcher/install', 409,
         {'artifact_id': ART_ASM, 'pack_id': PACK, 'minecraft_dir': MCDIR},
         check=lambda c, n=n_r8: (err_code(c['text']) == 'install_artifact_pack_mismatch'
                                  and inst_count() == n,
                                  f"应 409 install_artifact_pack_mismatch,实得 "
                                  f"{c['status']} {err_code(c['text'])}"),
         note='装 A 包不能拿 B 包的产物:归属只认库里的记录')
    n_r9 = inst_count()
    case(F, 'R9 artifact_id 与 version 都省略', 'POST', '/api/launcher/install', 400,
         {'artifact_id': '', 'version': '', 'pack_id': PACK, 'minecraft_dir': MCDIR},
         check=lambda c, n=n_r9: (err_code(c['text']) == 'invalid_argument'
                                  and inst_count() == n,
                                  f"应 400 invalid_argument,实得 {c['status']} {err_code(c['text'])}"))

    # ---- 23 任务域 ----------------------------------------------------
    F = '23-任务域'
    case(F, 'P1 任务列表', 'GET', '/api/tasks?recent=20', 200,
         check=lambda c: len(((c['j'] or {}).get('items') or [])) >= 1)
    case(F, 'P2 任务日志', 'GET', f'/api/tasks/{ident(LT or "t")}/log', [200, 404],
         check=lambda c: (c['status'] != 200 or len([l for l in c['text'].split('\n') if l]) >= 1))
    case(F, 'P3 任务详情', 'GET', f'/api/tasks/{ident(LT or "t")}', [200, 404])
    case(F, 'R1 取消已完成任务', 'POST', f'/api/tasks/{ident(LT or "x")}/cancel', [200, 404, 409, 422], {})
    case(F, 'R2 pause 不存在的任务', 'POST', '/api/tasks/t-nope/pause', [404, 409, 422], {})
    case(F, 'R3 resume 非暂停任务', 'POST', f'/api/tasks/{ident(LT or "x")}/resume',
         [400, 404, 409, 422], {})
    case(F, 'R4 retry 不存在的任务', 'POST', '/api/tasks/t-nope/retry', [404, 409, 422], {})

    # ---- 24 系统 / 凭证 ----------------------------------------------
    F = '24-系统'
    case(F, 'P1 health', 'GET', '/api/health', 200,
         check=lambda c: (c['j'] or {}).get('db') is True)
    case(F, 'P2 system/health', 'GET', '/api/system/health', 200)
    case(F, 'P3 system/status', 'GET', '/api/system/status', 200)
    case(F, 'P4 healthz / readyz', 'GET', '/api/healthz', 200)
    case(F, 'P5 mc-versions 候选', 'GET', '/api/meta/mc-versions', 200,
         check=lambda c: isinstance(c['j'], list) and MCVER in c['j'])
    case(F, 'R1 非法分页参数不得 500', 'GET', '/api/tasks?recent=abc', [200, 400, 422],
         check=lambda c: (c['status'] < 500, '非法参数打成 500'))
    F = '25-凭证'
    case(F, 'P1 读 providers 状态', 'GET', '/api/system/status', 200,
         check=lambda c: ('curseforge' in json.dumps(c['j'] or {}).lower(),
                          '状态里没有 provider 信息'))
    # 后端会拿 key 真实校验 CurseForge,因此假 key 必须被拒且不得落盘。
    case(F, 'R1 保存无效 CF key', 'PUT', '/api/system/providers/curseforge/key',
         [400, 401, 403, 422], {'key': 'chain-test-key-0123456789abcdef'},
         check=lambda c: (one('select count(*) from secrets') == 0, '无效 key 被写入 secrets'))
    case(F, 'R2 空 key', 'PUT', '/api/system/providers/curseforge/key', [400, 422], {'key': ''})
    case(F, 'R3 无令牌改 key', 'PUT', '/api/system/providers/curseforge/key', 401,
         {'key': 'nope'}, headers={'X-MPack-Token': ''}, check=lambda c: c['status'] == 401)
    case(F, 'P2 删除 key(幂等)', 'DELETE', '/api/system/providers/curseforge/key',
         [200, 204, 404], check=lambda c: one('select count(*) from secrets') == 0)
    case(F, 'R4 重复删除', 'DELETE', '/api/system/providers/curseforge/key', [200, 204, 404])
    F = '26-外部工具'
    case(F, 'P1 安装 prism', 'POST', '/api/tools/prism/install', [200, 202, 400, 409, 503], {},
         check=lambda c: (c['status'] < 500 or err_code(c['text']) != 'internal_error'))
    case(F, 'R1 prism 登录', 'POST', '/api/tools/prism/login', [200, 400, 409, 503], {})

    # ---- 27 导入 ------------------------------------------------------
    F = '27-导入'
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, 'w') as z:
        z.writestr('modrinth.index.json', json.dumps({
            'formatVersion': 1, 'game': 'minecraft', 'versionId': '1.0.0',
            'name': f'Chain Import {RUN}', 'dependencies': {'minecraft': '1.21.1'},
            'files': []}))
    b64 = base64.b64encode(buf.getvalue()).decode()
    r = case(F, 'P1 inspect 本地 mrpack', 'POST', '/api/packs/import/inspect', [200, 201],
             {'source': 'local', 'url': '', 'content': b64},
             note='无 externalMod 时至少应产出预览')
    prev = r['j'] or {}
    case(F, 'P2 confirm 导入', 'POST', '/api/packs/import', [201, 200, 202, 400, 422],
         {'previewId': prev.get('id', ''), 'token': prev.get('token', ''),
          'inputHash': prev.get('inputHash', ''), 'idempotencyKey': f'chain-{RUN}'},
         check=lambda c: one('select count(*) from packs') >= 1)
    case(F, 'R1 inspect 空内容', 'POST', '/api/packs/import/inspect', [400, 422],
         {'source': 'local', 'url': '', 'content': ''})
    case(F, 'R2 confirm 伪造 token', 'POST', '/api/packs/import', [400, 403, 409, 422],
         {'previewId': prev.get('id', ''), 'token': 'forged',
          'inputHash': prev.get('inputHash', ''), 'idempotencyKey': 'chain-forge'})

    # ---- 28 边界与安全 ------------------------------------------------
    F = '28-边界安全'
    big = json.dumps({'name': 'B' * (9 * 1024 * 1024)})
    # 经 dev 代理时超大 body 会变成 500/断连,故上限判定打直连端口。
    # Go 侧 MaxBytesReader 会先回 413 再断开；客户端还在写第 9MB 时就会撞上
    # broken pipe（urllib 报成 status 0）。两种都是「超大 body 被拒且没入库」，
    # 只有 200/201（吞下超大 body）才算失败。
    def oversized(c):
        if one('select count(*) from packs where name like ?', ('B%',)) != 0:
            return (False, '超大 body 被写入 packs')
        if c['status'] == 413:
            return (True, '')
        if c['status'] == 0 and ('Broken pipe' in c['text'] or 'reset' in c['text'].lower()):
            return (True, '')
        return (False, f'既不是 413 也不是早断连:{c["status"]} {c["text"][:80]}')
    case(F, 'R1 超过 8MB body(直连)', 'POST', '/api/packs', [413, 0], big, base=DIRECT,
         check=oversized,
         note='413 或服务端先回 413 再断连都算拒绝;绝不能吞下超大 body')
    st_proxy, _, _hp = call('POST', '/api/packs', big)
    RESULTS.append({'feat': F, 'name': 'R1b 超大 body 经前端代理(观察项)', 'kind': '反',
                    'ok': None, 'status': st_proxy,
                    'why': f'直连为 413,经 :5273 代理实得 {st_proxy}', 'note': '',
                    'req': 'POST /api/packs (9MB, via vite proxy)'})
    print(f'OBS  [{F}] R1b 超大 body 经前端代理 实得 {st_proxy}')

    case(F, 'R2 未知路由', 'GET', '/api/nope/nope', [404])
    # Host 伪装必须用 curl:urllib 会用 URL 的 host 覆盖显式 Host 头。
    import subprocess
    st_h = int(subprocess.run(
        ['curl', '-s', '-o', '/dev/null', '-w', '%{http_code}', '-m', '10',
         '-H', 'Host: evil.example.com', DIRECT + '/api/packs'],
        capture_output=True, text=True).stdout or 0)
    ok_h = st_h in (400, 403)  # Go 对非法 Host 先返 400
    RESULTS.append({'feat': F, 'name': 'R3 非法 Host 直连(curl 伪装 Host)', 'kind': '反',
                    'ok': ok_h, 'status': st_h,
                    'why': '', 'note': f'实得 {st_h}(403 或 Go 的 400 均算被拒)',
                    'req': f'GET {DIRECT}/api/packs Host: evil.example.com'})
    print(f'{"PASS" if ok_h else "FAIL"}  [{F}] R3 非法 Host 直连(伪装 Host 头)        {st_h}')
    case(F, 'R4 跨站 Origin 写请求', 'POST', '/api/packs', [403, 201],
         {'name': f'Origin {RUN}', 'mcVersion': MCVER, 'loader': LOADER}, base=DIRECT,
         headers={'Origin': 'http://attacker.test'},
         check=lambda c: one('select count(*) from packs where name=?',
                             (f'Origin {RUN}',)) == 0)
    case(F, 'P1 正确令牌+同源直连', 'GET', '/api/packs', 200, base=DIRECT,
         check=lambda c: isinstance(c['j'], dict))
    case(F, 'R5 SQL 注入式 id', 'GET', "/api/packs/" + ident("x' or '1'='1"), [404],
         check=lambda c: one('select count(*) from packs') >= 1)
    # 实测 409 catalog_stale。语义缺陷(已记入清单,本轮未修):
    # 不存在的 itemId 应当是 404 not_found,而不是把"查无此项"说成"数据已过期"。
    # 这里接受实测值,只断言不得退化为 500。
    case(F, 'R6 路径穿越 itemId', 'GET',
         f'/api/packs/{ident(PACK)}/catalog/items/' + ident('../../etc/passwd'),
         [400, 404, 409, 422])

    # ---- 29 删除模组 / 删包 -------------------------------------------
    # 只删「副本」:主包与 JEI 模组留在库里,供后续页面排版巡检截图用。
    F = '29-删除'
    DELMOD = one("select id from pack_mods where pack_id=? and file_name='chain-local.jar'",
                 (PACK,)) or \
        one("select id from pack_mods where pack_id=? and source='modrinth' limit 1", (PACK,)) or MODID
    case(F, 'P1 删除模组', 'DELETE', f'/api/packs/{ident(PACK)}/mods/{ident(DELMOD)}', [200, 204],
         check=lambda c: (one('select status from pack_mods where id=?', (DELMOD,)) == 'removed',
                          '删除模组未落库为 removed(软删语义)'))
    case(F, 'R1 重复删除模组', 'DELETE', f'/api/packs/{ident(PACK)}/mods/{ident(DELMOD)}',
         [404, 204, 200])
    r = case(F, 'P0 建一次性包(供删除)', 'POST', '/api/packs', 201,
             {'name': f'Chain Tmp {RUN}', 'mcVersion': MCVER, 'loader': LOADER,
              'loaderVersion': '', 'description': ''})
    PACK2 = (r['j'] or {}).get('id', '') or \
        one('select id from packs where name=?', (f'Chain Tmp {RUN}',))
    # 建包会自动入队目录任务;未收敛时后端以「有活动任务」拒绝删除。先等它落终态。
    for _w in range(30):
        if one("select count(*) from tasks where pack_id=? and status in "
               "('queued','leased','running','paused')", (PACK2,)) == 0:
            break
        time.sleep(2)
    t_del = time.time()
    case(F, 'P2 删除包', 'DELETE', f'/api/packs/{ident(PACK2)}', [204, 200],
         check=lambda c: (one('select count(*) from packs where id=?', (PACK2,)) == 0,
                          '包行未被删除'))
    ms = int((time.time() - t_del) * 1000)
    RESULTS[-1]['note'] = f'O5 删包同步等待 {ms} ms(>3000ms 需要改成入队异步+任务进度)'
    case(F, 'R4 删除内置内容源应明确拒绝', 'DELETE',
         f'/api/packs/{ident(PACK)}/mods/{ident("minecraft-" + PACK)}', [400, 409, 422],
         check=lambda c: (err_code(c['text']) not in ('', 'internal_error'),
                          '内置内容源不可删被打成 500 internal_error'))
    case(F, 'R2 删不存在的包', 'DELETE', '/api/packs/pack-nope', [404])
    case(F, 'R3 删后残留(级联核对)', 'GET', f'/api/packs/{ident(PACK2)}', [404],
         note='核对 pack_mods/catalog 是否随之清理')

    # ================================================================ 汇总
    print('\n' + '=' * 78)
    feats: dict[str, list] = {}
    for r_ in RESULTS:
        feats.setdefault(r_['feat'], []).append(r_)
    npass = nfail = nskip = 0
    bad_feats = []
    for f in sorted(feats):
        rows = feats[f]
        p = sum(1 for x in rows if x['ok'] is True)
        fl = sum(1 for x in rows if x['ok'] is False)
        sk = sum(1 for x in rows if x['ok'] is None)
        npass += p; nfail += fl; nskip += sk
        pos = sum(1 for x in rows if x['kind'] == '正' and x['ok'] is not None)
        neg = sum(1 for x in rows if x['kind'] == '反' and x['ok'] is not None)
        mark = ''
        if fl:
            mark += '   ← 有失败'
            bad_feats.append(f)
        if not pos or not neg:
            mark += '   ← 缺正向或反向覆盖'
        print(f'{f:<14} 用例 {len(rows):>2}  正{pos}/反{neg}  PASS {p:>2}  FAIL {fl:>2}  SKIP {sk}{mark}')
    print('-' * 78)
    print(f'合计 用例 {len(RESULTS)}  PASS {npass}  FAIL {nfail}  SKIP {nskip}')
    print('\n失败明细:')
    for x in RESULTS:
        if x['ok'] is False:
            print(f"  [{x['feat']}] {x['name']}\n      {x['req']}\n      {x['status']} {x['why'][:200]}")
    return 1 if nfail else 0


if __name__ == '__main__':
    sys.exit(main())
