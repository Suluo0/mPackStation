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

echo "[dev] starting backend  (go run, 127.0.0.1:18872, data $DATA)"
(cd "$ROOT/apps/server" && go run ./cmd/server -addr 127.0.0.1:18872 -data "$DATA" \
  > "$LOGDIR/server.log" 2>&1) &

echo "[dev] starting frontend (vite, 0.0.0.0:5271)"
# 唯一前端入口 = apps/web3（IDE 式单页，docs/design/web3-ide-shell-v3.md）。
(cd "$ROOT/apps/web3" && npm run dev -- --host 0.0.0.0 --port 5271 \
  > "$LOGDIR/web.log" 2>&1) &

for i in $(seq 1 45); do
  srv="$(curl -s -m 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:18872/api/health 2>/dev/null)"
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
