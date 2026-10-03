#!/usr/bin/env python3
"""流水线终局验证驱动：真实 Rust 内核把「构建出的 .mrpack」装成可启动实例。

与 chain-test.py 的分工：那边用协议桩证明「请求→校验→入队→入库→fork/exec→终态」，
这里用 cargo build 出来的真二进制证明「真下载→真落盘→真启动」：
  建包 → 面向包搜索 → 加模组 → resolve → 建版本 → 构建 .mrpack
      → POST /api/launcher/install {artifact_id} → 磁盘上的 jar 逐个按 manifest 的 sha1 复核
      → (--launch) POST /api/launcher/launch → 任务终态 + 游戏进程真的活着

判定标准是磁盘与数据库，不是 HTTP 200。任何一步不满足就退出码 1。
用法见 scripts/verify-terminal-chain.sh（它负责编译内核、起第四套隔离后端）。
"""
from __future__ import annotations

import argparse
import hashlib
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

BASE = os.environ.get('TERM_BASE', 'http://127.0.0.1:18874')
TOKEN = os.environ.get('MPACK_TOKEN', 'terminal-token-20260930')
DATA = os.environ.get('TERM_DATA', '/tmp/mpack-terminal')
DB_PATH = os.environ.get('TERM_DB', DATA + '/mpackstation.db')

STEP: list[tuple[bool, str, str]] = []


def say(msg: str) -> None:
    print(f'\n\033[1m▸ {msg}\033[0m', flush=True)


def ok(name: str, detail: str = '') -> None:
    STEP.append((True, name, detail))
    print(f'  PASS  {name}  {detail[:140]}', flush=True)


def bad(name: str, detail: str) -> None:
    STEP.append((False, name, detail))
    print(f'  FAIL  {name}  {detail[:240]}', flush=True)


def check(cond: bool, name: str, fail_detail: str, ok_detail: str = '') -> bool:
    if cond:
        ok(name, ok_detail)
    else:
        bad(name, fail_detail)
    return bool(cond)


def call(method: str, path: str, body=None, timeout: int = 120):
    data = None
    hdrs: dict[str, str] = {}
    if body is not None:
        data = json.dumps(body).encode()
        hdrs['content-type'] = 'application/json'
    if method != 'GET':
        hdrs['X-MPack-Token'] = TOKEN
    req = urllib.request.Request(BASE + path, data=data, headers=hdrs, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, r.read().decode('utf-8', 'replace')
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode('utf-8', 'replace')
    except Exception as e:  # noqa: BLE001
        return 0, f'__transport_error__ {type(e).__name__}: {e}'


def jload(txt: str):
    try:
        return json.loads(txt)
    except Exception:  # noqa: BLE001
        return None


def q(sql: str, args=()):
    con = sqlite3.connect(DB_PATH)
    try:
        return con.execute(sql, args).fetchall()
    finally:
        con.close()


def api(method: str, path: str, body=None, want=(200, 201, 202), what: str = '',
        retries: int = 0, retry_on=(502, 503)):
    """retries>0 只用于外网依赖的调用：Modrinth 偶发 502 provider_unavailable，
    重试几次比让整条链路的断言失去前提（模组没进包，后面的 409/落盘检查全空转）划算。"""
    for attempt in range(retries + 1):
        st, txt = call(method, path, body)
        if st in want or st not in retry_on or attempt == retries:
            break
        print(f'  … {what or path} 返回 {st}，第 {attempt + 1} 次重试', flush=True)
        time.sleep(5)
    if st not in want:
        bad(what or f'{method} {path}', f'期望 {list(want)} 实得 {st} {txt[:200]}')
        return None
    ok(what or f'{method} {path}', f'{st}')
    return jload(txt)


def maven_path(name: str) -> str:
    """com.mojong:block:1.2 → com/mojong/block/1.2/block-1.2.jar（不含 classifier）。"""
    parts = str(name).split(':')
    if len(parts) < 3:
        return ''
    group, artifact, version = parts[0], parts[1], parts[2]
    jar = f'{artifact}-{version}.jar'
    return '/'.join([group.replace('.', '/'), artifact, version, jar])


def missing_libraries(mcdir: str, version_json: str) -> list:
    """版本 JSON 里声明的库一个都不能少（O16）。

    加载器（fabric/quilt）的库在 JSON 里只有 name + url、没有 downloads.artifact，
    下载层以前整批跳过它们，装完缺一堆 jar，游戏起来就 ClassNotFoundException。
    """
    with open(version_json, encoding='utf-8') as fh:
        doc = json.load(fh)
    absent = []
    for lib in doc.get('libraries') or []:
        downloads = lib.get('downloads') or {}
        artifact = downloads.get('artifact') or {}
        rel = str(artifact.get('path') or '') or maven_path(str(lib.get('name') or ''))
        if not rel:
            continue
        if not os.path.isfile(os.path.join(mcdir, 'libraries', rel)):
            absent.append(rel)
    return absent


def sha1_of(path: str) -> str:
    h = hashlib.sha1()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def dir_bytes(path: str) -> int:
    total = 0
    for root, _, files in os.walk(path):
        for name in files:
            try:
                total += os.path.getsize(os.path.join(root, name))
            except OSError:
                pass
    return total


def task_log_messages(task_id: str) -> str:
    """TaskView 契约里没有 message 字段，进度/结果消息在 /api/tasks/{id}/log
    （ndjson，每行一个 EventView，消息在 message）。返回拼接后的全部消息。"""
    st, txt = call('GET', f'/api/tasks/{task_id}/log')
    if st != 200 or not txt:
        return ''
    out = []
    for line in str(txt).splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        m = ev.get('message')
        if m:
            out.append(str(m))
    return '\n'.join(out)


def wait_task(task_id: str, timeout: int, label: str):
    """任务终态词汇按契约是 success（不是 succeeded）。"""
    end = time.time() + timeout
    last = ''
    msg = ''
    while time.time() < end:
        st, txt = call('GET', f'/api/tasks/{task_id}')
        j = jload(txt) or {}
        last = j.get('status', '')
        msg = str(j.get('message') or j.get('error') or '')[:200]
        if last in ('success', 'failed', 'cancelled'):
            if last == 'success':
                ok(f'{label}任务终态', msg)
            else:
                bad(f'{label}任务终态', f'status={last} {msg}')
            return last, j
        time.sleep(3)
    bad(f'{label}任务终态', f'{timeout}s 内未收敛，最后状态={last} {msg}')
    return last, {}


def read_game_boot_log(path: str) -> str:
    """游戏日志按追加写，只取最后一次启动那一段——上一轮的崩溃记录混进来会误判。"""
    if not os.path.isfile(path):
        return ''
    with open(path, encoding='utf-8', errors='replace') as fh:
        fh.seek(0, os.SEEK_END)
        fh.seek(max(0, fh.tell() - 200_000))
        txt = fh.read()
    cut = txt.rfind('Loading Minecraft')
    return txt[cut:] if cut >= 0 else txt


def verify_launch(submit: dict, mcdir: str) -> None:
    """真启动的判据：任务日志里的 pid 还活着，启动段有 Fabric 横幅和渲染线程建号，
    且没有 Mod resolution failed。验完默认收掉游戏进程（TERM_KEEP=1 保留）。"""
    task_id = str(submit.get('taskId') or '')
    st, _ = wait_task(task_id, 420, '启动')
    # pid 在任务日志里，契约的 TaskView 没有 message 字段
    message = task_log_messages(task_id)
    found = re.search(r'pid (\d+)', message)
    if st != 'success':
        return
    if not found:
        bad('游戏进程 pid', f'任务成功但日志里没有 pid: {message[:160]}')
        return
    pid = int(found.group(1))
    time.sleep(6)
    boot = read_game_boot_log(os.path.join(mcdir, 'launcher-launch.log'))
    alive = os.path.exists(f'/proc/{pid}') or os.system(f'ps -p {pid} > /dev/null') == 0
    check(alive, '游戏进程活着', f'pid {pid} 已退出（窗口可能崩了）。日志尾：{boot[-400:]}',
          f'pid={pid}')
    check('Loading Minecraft' in boot and 'Setting user' in boot,
          '游戏真起来了（Fabric 横幅 + 渲染线程建号）',
          f'本轮日志没有启动痕迹: {boot[-400:]}', f'{len(boot)} 字符日志')
    check('Mod resolution failed' not in boot, '模组依赖解析通过（没有 Fabric 报错界面）',
          f'启动段里有 Mod resolution failed: {boot[:300]}')
    if alive and os.environ.get('TERM_KEEP') != '1':
        os.kill(pid, 15)
        print(f'  已终止验证用的游戏进程 pid={pid}（TERM_KEEP=1 可保留）', flush=True)


def main() -> int:  # noqa: C901, PLR0912, PLR0915 - 一次性线性验证脚本
    ap = argparse.ArgumentParser()
    ap.add_argument('--mc', default=os.environ.get('TERM_MC', '1.21.1'))
    ap.add_argument('--loader', default=os.environ.get('TERM_LOADER', 'fabric'))
    ap.add_argument('--loader-version', default=os.environ.get('TERM_LOADER_VER', '0.16.14'))
    ap.add_argument('--project', default=os.environ.get('TERM_PROJECT', 'u6dRKJwZ'),
                    help='Modrinth project id（默认 JEI）')
    ap.add_argument('--deps', default=os.environ.get('TERM_DEPS', 'P7dR8mSH,7tEfOcA7'),
                    help='逗号分隔的必需依赖 project id（默认 fabric-api,mezzconfig），补齐后闸门才放行')
    ap.add_argument('--launch', action='store_true', help='额外真启动 Minecraft')
    ap.add_argument('--install-timeout', type=int, default=int(os.environ.get('TERM_INSTALL_TIMEOUT', '1500')))
    args = ap.parse_args()

    run = time.strftime('%m%d-%H%M%S')
    mcdir = os.path.join(DATA, 'minecraft')
    export = os.path.join(DATA, 'export')
    os.makedirs(mcdir, exist_ok=True)
    os.makedirs(export, exist_ok=True)

    say('0 后端健康 + 内核二进制')
    j = api('GET', '/api/health', what='GET /api/health')
    if not j or j.get('db') is not True:
        bad('后端未就绪', str(j)[:200])
        return 1
    st, txt = call('GET', '/api/system/status')
    print(f'  启动器内核状态: {str(txt)[:220]}')

    say('1 注册导出目录')
    # absolute_path 上有 UNIQUE：同一个文件夹第二次批准（换了名字也算）在 O15
    # 之前会撞约束变成 500，修好后是 409 export_dir_conflict。重跑就复用已批准的
    # 名字，如同界面列出已批准目录让用户挑，而不是每次凭空造一个别名。
    # 服务端存的是 canonical 路径（macOS 上 /tmp 会解析成 /private/tmp），两种写法都查。
    real = os.path.realpath(export)
    have = q('select name from allowed_export_dirs where absolute_path in (?,?) limit 1', (export, real))
    if have:
        export_name = have[0][0]
        ok('导出目录已批准过，复用', f'{export_name} → {real}')
    else:
        export_name = f'term-{run}'
        api('POST', '/api/export-dirs', {'name': export_name, 'directory': export},
            want=(200, 201), what='注册导出目录')
    if not q('select count(*) from allowed_export_dirs')[0][0]:
        bad('导出目录未入库', 'allowed_export_dirs 为空')
        return 1

    say('2 建包（MC ' + args.mc + ' / ' + args.loader + ' ' + args.loader_version + '）')
    j = api('POST', '/api/packs', {'name': f'Terminal {run}', 'mcVersion': args.mc,
                                   'loader': args.loader, 'loaderVersion': args.loader_version,
                                   'description': 'terminal chain'}, want=(201,), what='建包')
    if not j:
        return 1
    pack = j.get('id', '')

    say('3 面向包搜索 → 兼容版本 → 添加模组（真下载 jar）')

    def add_mod(project_id: str, label: str) -> bool:
        j = api('GET', f'/api/packs/{pack}/mod-versions?provider=modrinth'
                       f'&projectId={urllib.parse.quote(project_id)}'
                       f'&mcVersion={args.mc}&loader={args.loader}',
                   what=f'取兼容版本 {label}', retries=2)
        vid = ''
        for v in (j or {}).get('items') or []:
            gv, ld = v.get('gameVersions') or [], v.get('loaders') or []
            if args.mc in gv and (not ld or args.loader in ld):
                vid = v.get('id', '')
                break
        if not check(bool(vid), f'{label} 找到兼容版本', '没有匹配 MC/加载器的版本'):
            return False
        api('POST', f'/api/packs/{pack}/mods', {'provider': 'modrinth', 'projectId': project_id,
                                               'versionId': vid, 'required': True},
            want=(201,), what=f'添加模组 {label}', retries=2)
        return True

    if not add_mod(args.project, args.project):
        return 1
    row = q("select file_name,display_name from pack_mods where pack_id=? and status='installed'",
            (pack,))
    if not check(bool(row), '模组已入包内清单', 'pack_mods 无 installed 行',
                 f'{[r[0] for r in row]}'):
        return 1

    say('4 锁依赖 / 建版本')
    api('POST', f'/api/packs/{pack}/resolve', {}, want=(200, 201, 202), what='resolve')
    j = api('POST', f'/api/packs/{pack}/versions', {'version': f'1.0.{run}', 'channel': 'release',
                                                    'changelog': 'terminal', 'source': 'manual'},
            want=(201,), what='建版本')
    pver = (j or {}).get('id', '') or \
        q('select id from pack_versions where pack_id=? order by created_at desc limit 1',
          (pack,))[0][0]

    def pending_deps(where: str = "status='pending'"):
        return q(f"select severity, summary from conflicts where pack_id=? and kind='dependency' and {where}",
                 (pack,))

    def dep_error_count() -> int:
        return sum(1 for sev, _ in pending_deps() if sev == 'error')

    # 反例（O20）：JEI 声明了必需依赖（fabric-api、mezzconfig），此时还没补，
    # 构建必须被闸门拦住。以前这里一路 201，产物装进游戏就是 Fabric 的
    # "Mod resolution failed"，流水线走完却开不了游戏。
    st, txt = call('POST', f'/api/packs/{pack}/build',
                   {'packVersionId': pver, 'exportDirName': export_name})
    j = jload(txt) or {}
    code = str(((j.get('error') or {}).get('code')) or j.get('code') or '')
    blocked = dep_error_count()
    check(st == 409 and code == 'build_unresolved_conflicts',
          '缺必需依赖时构建被拒（409）', f'实得 {st} {str(txt)[:200]}',
          f'{blocked} 条 error 级依赖冲突被拦，闸门消息带回摘要')

    say('4b 按冲突提示补齐必需依赖后重新 resolve')
    for dep_id in [x.strip() for x in str(args.deps).split(',') if x.strip()]:
        if not add_mod(dep_id, dep_id):
            return 1
    api('POST', f'/api/packs/{pack}/resolve', {}, want=(200, 201, 202), what='resolve(补依赖)')
    # O21：新一轮 resolve 没再检出的冲突必须自动结案，否则闸门永远拦着。
    still_error = dep_error_count()
    resolved_rows = q("select count(*) from conflicts where pack_id=? and kind='dependency' "
                      "and status='resolved'", (pack,))[0][0]
    check(still_error == 0, '补齐依赖后无 error 级依赖冲突', f'仍 pending: {pending_deps()}',
          f'0 条（上一轮 {blocked} 条）')
    check(int(resolved_rows) > 0, '过期冲突自动结案（O21）',
          '库里没有 resolved 的依赖冲突，说明只增不删', f'{resolved_rows} 条已结案')
    # O24：可选依赖（BOHUKqOz）只出 warning，不参与构建闸门。
    warn = sum(1 for sev, _ in pending_deps() if sev == 'warning')
    print(f'  信息  warning 级依赖提示 {warn} 条（可选依赖，不拦构建）', flush=True)

    say('5 构建 .mrpack（服务端按包内权威清单装配）')
    # 第二轮 resolve 把新锁绑到了当前版本上，契约要求构建必须原样带上 lockSnapshot。
    j = api('GET', f'/api/packs/{pack}/locks', what='取锁快照')
    items = (j or {}).get('items') or []
    lock_snap = json.loads(str(items[0]['snapshot'])) if items and items[0].get('snapshot') else {}
    j = api('POST', f'/api/packs/{pack}/build',
            {'packVersionId': pver, 'exportDirName': export_name, 'lockSnapshot': lock_snap},
            want=(201,), what='构建')
    if not j:
        return 1
    art = (j.get('artifact') or {}).get('id', '') or \
        q("select id from artifacts where pack_id=? and kind='mrpack' order by created_at desc limit 1",
          (pack,))[0][0]
    arow = q('select path,size_bytes,kind from artifacts where id=?', (art,))
    if not arow:
        bad('产物未入库', f'artifacts 里没有 {art}')
        return 1
    path, size, kind = arow[0]
    check(kind == 'mrpack' and os.path.isfile(str(path)) and int(size) > 400,
          '产物落盘', f'kind={kind} path={path} size={size}', f'{path} ({size} B)')
    with zipfile.ZipFile(str(path)) as z:
        manifest = json.loads(z.read('modrinth.index.json').decode())
    deps = manifest.get('dependencies') or {}
    files = manifest.get('files') or []
    check(manifest.get('formatVersion') == 1, 'manifest formatVersion=1',
          f'实得 {manifest.get("formatVersion")}')
    check(deps.get('minecraft') == args.mc, 'manifest dependencies.minecraft',
          f'实得 {deps}', str(deps))
    lkey = f'{args.loader}-loader'
    check(deps.get(lkey) == args.loader_version, f'manifest dependencies[{lkey}]',
          f'实得 {deps.get(lkey)}，缺它会把包当原版装、模组全不加载', str(deps.get(lkey)))
    check(len(files) >= 1, 'manifest files[] 非空', '装配结果为空（基线 D1 空壳）', f'{len(files)} 条')
    bad_hash = [f.get('path') for f in files
                if len(str((f.get('hashes') or {}).get('sha1') or '')) != 40
                or len(str((f.get('hashes') or {}).get('sha512') or '')) != 128]
    check(not bad_hash, '每条 files[] 都带 sha1+sha512', f'哈希不全: {bad_hash}',
          f'{len(files)} 条')

    say('6 真实内核安装（只给 artifact_id，版本来自 manifest）')
    j = api('POST', '/api/launcher/install', {'artifact_id': art, 'pack_id': pack,
                                              'minecraft_dir': mcdir, 'mirror': '', 'java_path': ''},
            want=(202,), what='按产物安装')
    if not j:
        return 1
    st, _ = wait_task(j.get('taskId', ''), args.install_timeout, '安装')
    if st != 'success':
        bad('终局链路在安装处断开', '真实内核安装未成功，后续磁盘复核无意义')
        return summarize()
    ins = q('select version_id from launcher_installs where pack_id=? '
            'order by installed_at desc limit 1', (pack,))
    if not check(bool(ins), 'launcher_installs 有记录', '安装成功但库里没有记录'):
        return 1
    inst_vid = str(ins[0][0])
    check(inst_vid.startswith(f'{args.loader}-loader-'), '安装记录 version_id 形状',
          f'version_id={inst_vid!r}，应为 {args.loader}-loader-<lv>-<mc>')

    say('7 磁盘复核：manifest 每个 jar 都在实例目录里，且字节对得上 sha1')
    inst_dir = os.path.join(mcdir, 'versions', inst_vid)
    check(os.path.isdir(inst_dir), '实例目录存在', f'没有 {inst_dir}')
    vjson = os.path.join(inst_dir, f'{inst_vid}.json')
    check(os.path.isfile(vjson), '版本 json 已生成', f'缺 {vjson}')
    missing, mismatched = [], []
    for f in files:
        dest = os.path.join(inst_dir, str(f.get('path')))
        if not os.path.isfile(dest):
            missing.append(f.get('path'))
            continue
        want = str((f.get('hashes') or {}).get('sha1') or '')
        if want and sha1_of(dest) != want:
            mismatched.append(f.get('path'))
    check(not missing, '全部模组 jar 落盘', f'缺失: {missing}', f'{len(files)} 条')
    check(not mismatched, 'jar 字节与 manifest sha1 一致', f'校验失败: {mismatched}')
    libs = os.path.join(mcdir, 'libraries')
    assets = os.path.join(mcdir, 'assets')
    check(dir_bytes(libs) > 1024 * 1024, 'libraries 真下载了', f'{libs} 只有 {dir_bytes(libs)} B',
          f'{dir_bytes(libs) // 1024} KiB')
    # O16：加载器自己的库（fabric-loader/intermediary/mixin）在版本 JSON 里只有
    # name+url 没有 downloads，下载层以前整批跳过它们，装上就 ClassNotFoundException。
    loader_group = {'fabric': 'net/fabricmc', 'quilt': 'net/fabricmc',
                    'forge': 'net/minecraftforge', 'neoforge': 'net/neoforged'}.get(args.loader, '')
    loader_libs = os.path.join(libs, loader_group) if loader_group else libs
    absent = missing_libraries(mcdir, vjson) if os.path.isfile(vjson) else ['<版本 JSON 缺失>']
    check(not absent, f'{args.loader} 加载器库全部落盘（O16 守护）',
          f'版本 JSON 声明的库缺 {len(absent)} 个: {absent[:6]}（游戏会 ClassNotFoundException）',
          f'{dir_bytes(loader_libs) // 1024} KiB，JSON 声明的库全部在位')
    check(dir_bytes(assets) > 1024 * 1024, 'assets 真下载了', f'{assets} 只有 {dir_bytes(assets)} B',
          f'{dir_bytes(assets) // (1024 * 1024)} MiB')

    if args.launch:
        say('8 真启动 Minecraft（离线账号）')
        j = api('POST', '/api/launcher/launch', {'version': '', 'username': 'TermTester',
                                                 'minecraft_dir': mcdir, 'pack_id': pack,
                                                 'xmx_mb': 1024, 'java_path': ''},
                want=(202,), what='启动')
        if j:
            verify_launch(j, mcdir)
    else:
        say('8 未加 --launch：跳过真启动（安装链路已验证到磁盘）')

    return summarize()


def summarize() -> int:
    npass = sum(1 for s in STEP if s[0])
    nfail = [s for s in STEP if not s[0]]
    say(f'合计 断言 {len(STEP)}  PASS {npass}  FAIL {len(nfail)}')
    for _, name, why in nfail:
        print(f'  ✗ {name}: {why[:200]}')
    return 0 if not nfail else 1


if __name__ == '__main__':
    sys.exit(main())
