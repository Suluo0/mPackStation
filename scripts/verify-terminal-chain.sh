#!/usr/bin/env bash
# 流水线终局验证：用 cargo 真编译出来的 launcherCore 二进制，跑完
# 建包 → 加模组 → 锁依赖 → 构建 .mrpack → 安装 → (可选) 启动 Minecraft，
# 并对磁盘上的 jar 逐个复核 manifest 的 sha1。
#
# 第四套隔离环境：后端 :18874 / 数据 /tmp/mpack-terminal。
# 绝不触碰开发后端(18871, 数据 /tmp/mpack-data)、链路测试(18872/18873)、前端代理(5273)。
#
# 用法:
#   scripts/verify-terminal-chain.sh                # 装到磁盘为止
#   scripts/verify-terminal-chain.sh --launch       # 再真启动一次 Minecraft（会弹窗）
#   TERM_KEEP=1 ...                                 # 验完不杀后端与游戏进程
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
DATA=${TERM_DATA:-/tmp/mpack-terminal}
PORT=${TERM_PORT:-18874}
TOK=${MPACK_TOKEN:-terminal-token-20260930}
BIN=/tmp/mpack-terminal-server
TARGET=${TERM_TARGET:-/tmp/mpack-launcher-target}
PROFILE=${TERM_RELEASE:+release}
PROFILE=${PROFILE:-debug}
LOG=${TERM_LOG:-/tmp/terminal-run.log}

say() { printf '\n\033[1m▸ %s\033[0m\n' "$*"; }

# 本机探测一律绕过 HTTP 代理：环境里常驻 HTTP_PROXY，不绕过时 curl 连不上
# 会「成功」返回一段纯文本错误（退出码 0），端口探测因此既误报又漏报。
CURL_LOCAL="curl -s -m 2 --noproxy *"

# 与 chain-test-run.sh 同源的三态端口探测：free / unknown / <dataDir>。
# unknown = 端口上有人应答但没有 identity 端点（旧后端二进制），读不到实例
# 身份就必须当危险处理，不能当成「端口干净」。
port_state() {
  local body rc
  body=$($CURL_LOCAL "http://127.0.0.1:$1/api/health/identity" 2>/dev/null); rc=$?
  if [ $rc -ne 0 ] || [ -z "$body" ]; then echo free; return 0; fi
  printf '%s' "$body" | python3 -c 'import json,sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("unknown"); raise SystemExit
print("unknown" if ("error" in d and "dataDir" not in d) else (d.get("dataDir") or "unknown"))' 2>/dev/null || echo unknown
}

# 身份闸：默认 18874 与开发实例不同口，但 TERM_PORT 可改，且下面会 rm -rf。
# 端口上若已有别人的实例（改了 TERM_PORT 撞上 18872 等），必须硬失败 ——
# 否则 TERM_FRESH=1 的清库会打在开发数据上。见 issue-chain-test-port-collision。
_state=$(port_state "$PORT")
if [ -n "$_state" ] && [ "$_state" != free ] && [ "$_state" != "$DATA" ]; then
  if [ "$_state" = unknown ]; then
    _shown='(读不到：端口上的后端没有 /api/health/identity，是旧版二进制)'
  else
    _shown="$_state"
  fi
  cat >&2 <<EOF
[terminal] 拒绝执行：端口 ${PORT} 上已有实例，但不是本脚本的隔离实例。
  端口上实例的数据目录：${_shown}
  本脚本要清空的数据目录：${DATA}
  继续跑会 rm -rf 掉不属于本脚本的数据。改 TERM_PORT=<空闲端口>，或先 scripts/dev-stop.sh。
EOF
  exit 1
fi

# wait_up <port>：200 之外还要确认应答者就是目标数据目录
wait_up() { for _ in $(seq 1 40); do
    if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 --noproxy '*' "http://127.0.0.1:$1/api/health")" = 200 ]; then
      [ "$(port_state "$1")" = "$DATA" ] && return 0
      return 1
    fi
    sleep 1; done; return 1; }

# ---- 1. Rust 内核 -------------------------------------------------------
# 本机 cargo 由 brew rustup 提供，工具链装在 ~/.rustup/toolchains/<triplet>/bin，
# 默认不在 PATH 里（~/.cargo/bin 是空的），所以这里显式取一次路径。
if ! command -v cargo >/dev/null 2>&1; then
  RB=$(dirname "$(rustup which cargo 2>/dev/null)" 2>/dev/null || true)
  [ -x "$RB/cargo" ] || RB="$HOME/.cargo/bin"
  [ -x "$RB/cargo" ] || RB=$(echo "$HOME"/.rustup/toolchains/stable-*/bin 2>/dev/null | awk '{print $1}')
  PATH="$RB:$PATH"
fi
if ! command -v cargo >/dev/null 2>&1; then
  echo '找不到 cargo：brew install rustup-init && rustup toolchain install stable' >&2; exit 1
fi
say '构建 launcherCore（cargo，target 在本地盘 '$TARGET'，不放 SMB 上）'
mkdir -p "$TARGET"
( cd "$ROOT/launcherCore" && CARGO_TARGET_DIR="$TARGET" cargo build ${TERM_RELEASE:+--release} ) \
  || { echo 'launcherCore 构建失败'; exit 1; }
LAUNCHER_BIN="$TARGET/$PROFILE/mpack-launcher"
[ -x "$LAUNCHER_BIN" ] || { echo "没有可执行内核: $LAUNCHER_BIN"; exit 1; }
say '内核就绪: '"$LAUNCHER_BIN"'  ('$(/usr/bin/du -h "$LAUNCHER_BIN" | cut -f1)')'
"$LAUNCHER_BIN" install --help 2>&1 | grep -q -- '--mrpack' \
  || { echo '内核不认识 --mrpack：launcherCore 源码不是当前版本'; exit 1; }

# ---- 2. Go 后端 --------------------------------------------------------
say '构建隔离后端'
( cd "$ROOT/apps/server" && go build -o "$BIN" ./cmd/server ) || { echo 'Go 构建失败'; exit 1; }

say '准备隔离数据目录（只动 '"$DATA"'）'
pkill -f "mpack-terminal-server -addr 127.0.0.1:${PORT}" 2>/dev/null
sleep 1
# TERM_FRESH=0 保留上一轮的 libraries/assets：内核按 sha1「已存在即跳过」，
# 复跑启动验证时不必再下一遍几百 MB。
if [ "${TERM_FRESH:-1}" = "1" ]; then rm -rf "$DATA"; fi
mkdir -p "$DATA/export" "$DATA/minecraft"

say '起后端 :'"${PORT}"'  launcher='"$LAUNCHER_BIN"'（真实内核，不是协议桩）'
( MPACK_TOKEN="$TOK" MPACK_LAUNCHER_BIN="$LAUNCHER_BIN" \
    "$BIN" -addr "127.0.0.1:${PORT}" -data "$DATA" > "$DATA/server.log" 2>&1 & )
wait_up "$PORT" || { echo '后端起不来'; tail -20 "$DATA/server.log"; exit 1; }

# ---- 3. 验证驱动 -------------------------------------------------------
say '跑终局验证（结果同时写 '"$LOG"'）'
TERM_BASE="http://127.0.0.1:${PORT}" MPACK_TOKEN="$TOK" TERM_DATA="$DATA" \
  TERM_DB="$DATA/mpackstation.db" \
  python3 -u "$ROOT/scripts/verify-terminal-chain.py" "$@" 2>&1 | tee "$LOG"
RC=${PIPESTATUS[0]}

if [ "${TERM_KEEP:-0}" != "1" ]; then
  say '收掉隔离后端（数据目录保留在 '"$DATA"' 供复核）'
  pkill -f "mpack-terminal-server -addr 127.0.0.1:${PORT}" 2>/dev/null
fi
exit "$RC"
