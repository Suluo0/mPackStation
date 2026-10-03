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

wait_up() { # wait_up <port>
  for _ in $(seq 1 30); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 "http://127.0.0.1:$1/api/health")" = 200 ] && return 0
    sleep 1
  done
  return 1
}

say '停掉上一轮隔离后端(只按 -data 路径匹配,不杀别的进程)'
pkill -f "mpack-chain-server -addr 127.0.0.1:${PORT}" 2>/dev/null
pkill -f "mpack-chain-server -addr 127.0.0.1:${PORT2}" 2>/dev/null
sleep 1

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
wait_up "$PORT" || { echo "隔离后端 :${PORT} 起不来"; tail -20 "$DATA/server.log"; exit 1; }

say '启动第二个隔离实例(无启动器二进制,仅供反向用例) :'
echo "  http://127.0.0.1:${PORT2}  -data ${DATA2}"
( MPACK_TOKEN="$TOK" MPACK_LAUNCHER_BIN="" \
    "$BIN" -addr "127.0.0.1:${PORT2}" -data "$DATA2" > "$DATA2/server.log" 2>&1 & )
if wait_up "$PORT2"; then NOBIN="http://127.0.0.1:${PORT2}"; else NOBIN=''; echo '  备用实例起不来,该反向用例将 SKIP'; fi

say '确认前端 dev 代理在 :'
echo "  http://127.0.0.1:${WEB_PORT}"
if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 3 "http://127.0.0.1:${WEB_PORT}/api/health")" != 200 ]; then
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
