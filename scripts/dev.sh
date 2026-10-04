#!/usr/bin/env bash
# mPackStation dev：一键启动唯一标准服务（后端 18872 + 前端 5271，AGENTS.md 定稿）。
# macOS / Linux；Windows 侧沿用仓库内历史脚本不再维护。
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA=/tmp/mpack-data          # 唯一开发数据目录（空库可直接跑，迎新流程会引导建包）
LOGDIR="$ROOT/.tmp/dev"
mkdir -p "$LOGDIR"

port_busy() { lsof -t -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

if port_busy 18872; then echo "[dev] 18872 已占用，先 scripts/dev-stop.sh"; exit 1; fi
if port_busy 5271;  then echo "[dev] 5271 已占用，先 scripts/dev-stop.sh"; exit 1; fi

# ── 代理透传 ────────────────────────────────────────────────────────────
# 后端要访问 Modrinth / CurseForge 才能搜索、解析依赖。曾经后端进程环境里
# 没有任何 proxy 变量（脚本没透传），结果两个平台全不可达 → 搜索一律返回
# total=0、降级区也没了，看着像「代码改坏了」，实际是出不了网。
# 这里显式透传系统代理，并保证回环地址不走代理。
sys_proxy="${HTTPS_PROXY:-${https_proxy:-${HTTP_PROXY:-${http_proxy:-}}}}"
if [ -n "$sys_proxy" ]; then
  export HTTP_PROXY="${HTTP_PROXY:-${http_proxy:-$sys_proxy}}"
  export HTTPS_PROXY="${HTTPS_PROXY:-${https_proxy:-$sys_proxy}}"
  export http_proxy="${http_proxy:-$HTTP_PROXY}"
  export https_proxy="${https_proxy:-$HTTPS_PROXY}"
  export NO_PROXY="${NO_PROXY:-127.0.0.1,localhost}"
  export no_proxy="${no_proxy:-$NO_PROXY}"
  echo "[dev] proxy: $HTTPS_PROXY  (NO_PROXY=$NO_PROXY)"
fi

echo "[dev] starting backend  (go run, 127.0.0.1:18872, data $DATA)"
(cd "$ROOT/apps/server" && go run ./cmd/server -addr 127.0.0.1:18872 -data "$DATA" \
  > "$LOGDIR/server.log" 2>&1) &

# 无鉴权模式（用户 2026-10-03 定调）：本机单用户 IDE，前后端都只监听回环，
# 不再需要写令牌 —— 曾经那条「前端怎么拿到令牌」的链路本身就是 401 的根源。
echo "[dev] starting frontend (vite, 127.0.0.1:5271)"
# 唯一前端入口 = apps/web3（IDE 式单页，docs/design/web3-ide-shell-v3.md）。
# 不传 --host：交给 vite.config.ts 的 server.host（回环），命令行会覆盖它。
(cd "$ROOT/apps/web3" && npm run dev -- --port 5271 \
  > "$LOGDIR/web.log" 2>&1) &

for i in $(seq 1 45); do
  srv="$(curl -s -m 2 -o /dev/null -w '%{http_code}' --noproxy '*' http://127.0.0.1:18872/api/health 2>/dev/null)"
  if [ "$srv" = "200" ] && port_busy 5271; then
    echo "[dev] backend  ready  http://127.0.0.1:18872"
    echo "[dev] frontend ready  http://127.0.0.1:5271"
    echo "[dev] logs: $LOGDIR"
    echo "[dev] stop: scripts/dev-stop.sh"
    exit 0
  fi
  sleep 2
done
echo "[dev] WARNING: 90s 未就绪，查 $LOGDIR/server.log 与 web.log"
exit 1
