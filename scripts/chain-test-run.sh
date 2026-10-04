#!/usr/bin/env bash
# 一键跑「前端 → 后端调用链路测试」:重置隔离环境 → 起隔离后端 → 打用例。
# 只碰 /tmp/mpack-chain 这套隔离数据,绝不触碰开发库(/tmp/mpack-data)与端口 18765/18766/5173。
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
DATA=${CHAIN_DATA:-/tmp/mpack-chain}
PORT=${CHAIN_PORT:-18872}
WEB_PORT=${CHAIN_WEB_PORT:-5273}
# 第二个隔离实例:故意不给启动器二进制,用来验证「没装 mpack-launcher 时同步 503」这条反向用例。
DATA2=/tmp/mpack-chain-nobin
PORT2=${CHAIN_PORT_NOBIN:-18873}
TOK=${MPACK_TOKEN:-chain-token-20260930}
BIN=/tmp/mpack-chain-server
LOG=${1:-/tmp/chain-run.log}
STUB=$DATA/tools/mpack-launcher

say() { printf '\n\033[1m▸ %s\033[0m\n' "$*"; }
die() { printf '\n\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

# 本机探测一律绕过 HTTP 代理。环境里常驻 HTTP_PROXY/HTTPS_PROXY，若不回环
# 请求走代理，curl 连不上时会「成功」返回一段纯文本错误（退出码 0），
# 于是 port_state 把代理错误当成实例应答，闸门既误报又漏报。
# --noproxy '*' 让连不上时的退出码回到 7（真·无实例）。
CURL_LOCAL="curl -s -m 2 --noproxy *"

# 端口身份闸（issue-chain-test-port-collision）：
# 本脚本默认 PORT 与唯一开发后端 18872 同口。撞口时脚本自己的后端 bind 失败，
# 而 wait_up 只看 /api/health 是否 200 —— 开发后端会答这个 200，于是 173 个用例
# （含建包/删包）会打在 /tmp/mpack-data 上。修法是按响应体里的 dataDir 认人，
# 不认「健康就绪」这个既不唯一也不安全的信号。
#
# port_state <port> 分三态，让调用方能区分「端口没人」与「有人但读不到标记」：
#   free     curl 连不上，端口确实无人应答
#   unknown  有人应答，但 /api/health/identity 不可用（旧二进制没有实例标记）
#   <path>   实例真实服务的 dataDir
# unknown 必须当危险处理：读不到标记就假定端口干净，正是本缺陷当初的成因。
port_state() {
  local body rc
  body=$($CURL_LOCAL "http://127.0.0.1:$1/api/health/identity" 2>/dev/null); rc=$?
  if [ $rc -ne 0 ] || [ -z "$body" ]; then echo free; return 0; fi
  printf '%s' "$body" | python3 -c 'import json,sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("unknown"); raise SystemExit
# 后端活着但回的是 404 信封（无 identity 端点）= 旧二进制，标记不可读。
print("unknown" if ("error" in d and "dataDir" not in d) else (d.get("dataDir") or "unknown"))' 2>/dev/null || echo unknown
}

# assert_port_free_or_ours <port> <期望 dataDir> <人类可读标签>
# 起服务前调用：端口无人、或已有的正是上一轮 ours，都放行；
# 端口上是别人的实例（尤其是开发库）或身份不可读，则硬失败。
assert_port_free_or_ours() { # <port> <期望 dataDir> <标签>
  _port="$1"; _want="$2"; _label="$3"
  _state=$(port_state "$_port")
  [ "$_state" = free ] && return 0                  # 端口无人应答 → 可以起
  if [ "$_state" = "$_want" ]; then
    say "端口 ${_port} 上是本脚本上一轮的实例(${_label})，继续"
    return 0
  fi
  if [ "$_state" = unknown ]; then
    cat >&2 <<EOF
[chain] 拒绝执行：端口 ${_port} 上有实例在应答，但它没有 /api/health/identity
       （后端二进制比本脚本预期的旧），读不到 dataDir，无法确认它服务的是不是隔离目录。
       把读不到标记当成「端口干净」正是本缺陷当初的成因，所以这里硬失败。

  请停掉占用者：scripts/dev-stop.sh
  或换端口：     CHAIN_PORT=<空闲端口> scripts/chain-test-run.sh
EOF
    exit 1
  fi
  cat >&2 <<EOF
[chain] 拒绝执行：端口 ${_port} 已被另一个 mPackStation 实例占用。

  端口上实例的数据目录：${_state}
  本脚本要用的数据目录：${_want}

  这正是 issue-chain-test-port-collision 描述的自伤路径：
  继续跑会把 ${_state} 里的数据当成隔离环境重置（建包/删包/清目录），
  而 ${_state} 很可能就是你的开发库。

  三种解法（任选其一）：
    1) 停掉占用者：scripts/dev-stop.sh
    2) 换端口：     CHAIN_PORT=<空闲端口> scripts/chain-test-run.sh
    3) 真的想复用同一个实例 —— 那就不该用这个脚本，直接对开发库手工验证
EOF
  exit 1
}

# wait_up <port> <期望 dataDir>：不仅要 200，还要确认应答的实例确实是我们要的
# 那个数据目录。开发后端答 200 但 dataDir 不同的情况必须判失败。
wait_up() { # <port> <期望 dataDir>
  for _ in $(seq 1 30); do
    if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 --noproxy '*' "http://127.0.0.1:$1/api/health")" = 200 ]; then
      [ "$(port_state "$1")" = "$2" ] && return 0
      return 1   # 应答者不是目标数据目录 → 立刻失败，不等 30 秒
    fi
    sleep 1
  done
  return 1
}

say '端口身份闸：确认 '"$PORT"'/'"$PORT2"' 上没有别的实例'
assert_port_free_or_ours "$PORT"  "$DATA"  '主隔离实例'
assert_port_free_or_ours "$PORT2" "$DATA2" '无启动器备用实例'

say '停掉上一轮隔离后端(只按 -data 路径匹配,不杀别的进程)'
pkill -f "mpack-chain-server -addr 127.0.0.1:${PORT}" 2>/dev/null
pkill -f "mpack-chain-server -addr 127.0.0.1:${PORT2}" 2>/dev/null
sleep 1

# pkill 之后再确认一次：上面只按二进制名匹配，杀不掉 dev.sh 起的 go run。
for p in "$PORT" "$PORT2"; do
  case "$p" in "$PORT") w="$DATA";; *) w="$DATA2";; esac
  [ "$(port_state "$p")" = "$w" ] || assert_port_free_or_ours "$p" "$w" '停后复核'
done

say '清空隔离数据目录'
rm -rf "$DATA" "$DATA2"
mkdir -p "$DATA/export" "$DATA/minecraft" "$DATA/tools" "$DATA2"

say '构建隔离后端二进制'
( cd "$ROOT/apps/server" && go build -o "$BIN" ./cmd/server ) || { echo '构建失败'; exit 1; }

# 启动器桩:链路测试需要 mpack-launcher 真的可执行,但默认不能靠真实内核 ——
# 真实安装要下几十上百 MB 的 libraries/assets,链路测试要的是「每轮都能快速重跑」。
# 桩只按 launcherCore/src/protocol.rs 的 JSON Lines 形状回 phase/result 事件,
# 对非法版本号回 failure,并按 cli.rs 的旗标定义校验 argv;不下载文件、不真起游戏。
# 它证明的是「入队→入库→worker→fork/exec→终态」这条链路,不等于真实安装/启动成功。
# 真实内核验证:本机 cargo 可用(brew rustup,toolchain stable-aarch64-apple-darwin),
# 用 CHAIN_LAUNCHER_BIN 指向 cargo build 出来的二进制即可让这批用例走真内核,
# 见 scripts/verify-terminal-chain.sh。
say '写启动器桩(协议替身;真实内核验证走 CHAIN_LAUNCHER_BIN)'
cat > "$STUB" <<'STUBEOF'
#!/usr/bin/env bash
# mpack-launcher 测试桩 —— 只复现 JSON Lines 协议,不联网、不起游戏。
emit() { printf '%s\n' "$1"; }
fail() { emit "{\"type\":\"result\",\"success\":false,\"error\":\"$1\",\"message\":\"stub: $2\"}"; exit "${3:-1}"; }
cmd="${1:-}"; shift || true
# 按 launcherCore/src/cli.rs 的 clap 定义校验 argv:
# install 必填 --mc（没有 --version），launch 必填 --version；未知旗标一律退出码 2。
# 桩只认协议不认旗标时,Go 侧发错旗标也能全绿 —— 这正是 D9 漏掉的原因。
version=""; dir=""; username=""; mc=""; loader=""; loader_version=""; xmx=""; mrpack=""
bad=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version)  version="${2:-}"; shift 2 ;;
    --mc)       mc="${2:-}"; shift 2 ;;
    --mrpack)   mrpack="${2:-}"; shift 2 ;;
    --dir)      dir="${2:-}"; shift 2 ;;
    --username) username="${2:-}"; shift 2 ;;
    --loader)   loader="${2:-}"; shift 2 ;;
    --mirror|--java|--account-type|--loader-version)
      [ "$1" = "--loader-version" ] && loader_version="${2:-}"
      shift 2 ;;
    --xmx)      xmx="${2:-}"; shift 2 ;;
    *) bad="$bad $1" ;;
  esac
done
case "$cmd" in
  install|launch) ;;
  *) fail bad_args "unknown command $cmd" 2 ;;
esac
[ -n "$bad" ] && fail bad_args "unexpected argument$bad" 2
case "$cmd" in
  install)
    # 与 cli.rs 同形:--mc 与 --mrpack 二选一(--mrpack 时版本由 manifest 决定)。
    # Go 侧若把 mrpack 安装仍发成 --mc,这里会缺 manifest 校验而假绿。
    if [ -n "$mrpack" ]; then
      [ -f "$mrpack" ] || fail mrpack_not_found "no such .mrpack: $mrpack"
      # 真读一遍 manifest:字段名/依赖必须与 build_mrpack.go 产物一致,
      # 桩自造版本名的话,Go 侧与内核的字段约定错了也测不出来。
      parsed=$(python3 - "$mrpack" <<'PYEOF'
import json, sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as z:
    m = json.loads(z.read('modrinth.index.json').decode())
deps = m.get('dependencies') or {}
ldr = next((k for k in deps if k != 'minecraft'), '')
print('\t'.join([deps.get('minecraft', ''), ldr, deps.get(ldr, ''),
                  str(len(m.get('files') or [])), m.get('versionId') or m.get('version') or '']))
PYEOF
) || fail mrpack_invalid "manifest 解析失败"
      IFS=$'\t' read -r mc loader loader_version modcount packvid <<< "$parsed"
      [ -n "$mc" ] || fail mrpack_invalid "manifest 缺 dependencies.minecraft"
      version="$mc"
      emit "{\"type\":\"phase\",\"phase\":\"preparing\",\"message\":\"stub mrpack $packvid mods=$modcount\"}"
    else
      [ -z "$mc" ] && fail bad_args "missing required argument --mc" 2
      version="$mc"
    fi
    ;;
  launch)
    [ -z "$version" ] && fail bad_args "missing required argument --version" 2
    [ -z "$username" ] && fail bad_args "launch needs --username" 2
    ;;
esac
# 版本号可接受两种形状：裸 Minecraft 版本（1.21.1）与加载器版本目录 ID
# （fabric-loader-0.16.14-1.21.1）。后者是 launcherCore 真实装出来的目录名
# （src/loader/fabric.rs:43-45），O2 之后 Go 侧启动时传的就是安装记录里的那个 ID,
# 桩若仍只认裸版本号,就会把正确行为误判成 version_not_found。
if ! printf '%s' "$version" | grep -Eq '^([0-9]+\.[0-9]+(\.[0-9]+)?|[a-z]+-loader-[0-9a-zA-Z.]+-[0-9.]+)$'; then
  fail version_not_found "no such version $version"
fi
if [ "$cmd" = launch ] && [ -n "$xmx" ] && ! printf '%s' "$xmx" | grep -Eq '^[0-9]+$'; then
  fail bad_args "--xmx 需要纯数字 MB,收到 $xmx"
fi
emit '{"type":"phase","phase":"preparing","message":"stub preparing"}'
emit '{"type":"phase","phase":"resolving_version","message":"stub resolving"}'
if [ "$cmd" = install ]; then
  emit '{"type":"phase","phase":"downloading_libraries","message":"stub libs"}'
  emit '{"type":"phase","phase":"downloading_assets","message":"stub assets"}'
  emit '{"type":"phase","phase":"installing_loader","message":"stub loader"}'
  emit '{"type":"phase","phase":"verifying","message":"stub verify"}'
  # 真实内核 install.rs:74-77 回的是 version_id（带加载器时形如 fabric-loader-<ver>-<mc>）,
  # 安装记录与后续启动都以此为准；桩必须回同样的字段名,否则 O2 的链路测试是绿的假象。
  vid="$version"
  case "$loader" in
    # --loader 传的是简称(fabric)，--mrpack 从 manifest 读到的是依赖全名(fabric-loader)；
    # 两者都要得出与真实内核同一形状的目录 ID：fabric-loader-<加载器版本>-<mc>
    # (launcherCore/src/loader/fabric.rs:43-45)。
    fabric|quilt) vid="${loader}-loader-${loader_version:-latest}-${version}" ;;
    fabric-loader|quilt-loader) vid="${loader}-${loader_version:-latest}-${version}" ;;
    forge|neoforge) vid="${loader}-${version}-${loader_version:-latest}" ;;
  esac
  # --mrpack 分支额外回 packVersion/mods/overrides,与 main.rs run_install 的
  # mrpack 载荷同形;Go 侧只认 version_id,其余字段留给安装记录的可追溯性。
  extra=""
  if [ -n "$mrpack" ]; then
    extra=",\"pack_version\":\"$packvid\",\"mods\":$modcount,\"overrides\":0"
  fi
  emit "{\"type\":\"result\",\"success\":true,\"data\":{\"version\":\"$version\",\"version_id\":\"$vid\",\"dir\":\"$dir\",\"loader\":\"${loader:-vanilla}\",\"loaderVersion\":\"$loader_version\",\"stub\":true$extra}}"
else
  emit '{"type":"phase","phase":"launching","message":"stub launching"}'
  emit "{\"type\":\"result\",\"success\":true,\"data\":{\"pid\":$$,\"version\":\"$version\",\"username\":\"$username\",\"stub\":true}}"
fi
STUBEOF
chmod +x "$STUB"

# 默认用协议桩(快速、可重复);CHAIN_LAUNCHER_BIN 指向 cargo build 的真实二进制时,
# 这批用例就改走真内核 —— scripts/verify-terminal-chain.sh 用的正是这个入口。
LAUNCHER=${CHAIN_LAUNCHER_BIN:-$STUB}
say '启动隔离后端(带启动器二进制:'"$LAUNCHER"') :'
echo "  http://127.0.0.1:${PORT}  -data ${DATA}  launcher=${LAUNCHER}"
( MPACK_TOKEN="$TOK" MPACK_LAUNCHER_BIN="$LAUNCHER" \
    "$BIN" -addr "127.0.0.1:${PORT}" -data "$DATA" > "$DATA/server.log" 2>&1 & )
wait_up "$PORT" "$DATA" || { echo "隔离后端 :${PORT} 起不来,或应答者不是 ${DATA}"; tail -20 "$DATA/server.log" 2>/dev/null; exit 1; }

say '启动第二个隔离实例(无启动器二进制,仅供反向用例) :'
echo "  http://127.0.0.1:${PORT2}  -data ${DATA2}"
( MPACK_TOKEN="$TOK" MPACK_LAUNCHER_BIN="" \
    "$BIN" -addr "127.0.0.1:${PORT2}" -data "$DATA2" > "$DATA2/server.log" 2>&1 & )
if wait_up "$PORT2" "$DATA2"; then NOBIN="http://127.0.0.1:${PORT2}"; else NOBIN=''; echo '  备用实例起不来,该反向用例将 SKIP'; fi

say '确认前端 dev 代理在 :'
echo "  http://127.0.0.1:${WEB_PORT}"
if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 3 --noproxy '*' "http://127.0.0.1:${WEB_PORT}/api/health")" != 200 ]; then
  cat >&2 <<EOF
前端代理 :${WEB_PORT}/api 不可用。请在 apps/web 下用隔离后端起 dev:
  cd ${ROOT}/apps/web
  VITE_API_TARGET=http://127.0.0.1:${PORT} VITE_MPACK_TOKEN=${TOK} npx vite --port ${WEB_PORT}
EOF
  exit 1
fi

say '打链路用例(结果同时写 '"$LOG"')'
MPACK_TOKEN="$TOK" CHAIN_DB="$DATA/mpackstation.db" CHAIN_EXPORT="$DATA/export" \
  CHAIN_DIRECT="http://127.0.0.1:${PORT}" CHAIN_NOBIN="$NOBIN" \
  python3 -u "$ROOT/scripts/chain-test.py" 2>&1 | tee "$LOG"
RC=${PIPESTATUS[0]}

say '收掉备用实例(主实例保留,便于截图复核)'
pkill -f "mpack-chain-server -addr 127.0.0.1:${PORT2}" 2>/dev/null

say '环境保留在运行中:后端 '"${PORT}"' / 前端 '"${WEB_PORT}"' / 数据 '"${DATA}"
exit "$RC"
