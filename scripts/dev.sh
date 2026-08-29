#!/usr/bin/env bash
# mPackStation dev environment: one-click start (backend 18871 + frontend 5273)
# Works in git-bash / MSYS on Windows.
set -u

ROOT_MSYS="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$ROOT_MSYS" && pwd -W)"
GO="$ROOT/.tools/go/bin/go.exe"
[ -f "$GO" ] || GO=go
LOGDIR="$ROOT/.tmp/dev"
mkdir -p "$LOGDIR"

port_busy() {
  netstat -ano | grep -E ":$1\s" | grep -q LISTENING
}

if port_busy 18871; then echo "[dev] port 18871 already in use, stop it first: scripts/dev-stop.sh"; exit 1; fi
if port_busy 5273;  then echo "[dev] port 5273 already in use, stop it first: scripts/dev-stop.sh";  exit 1; fi

echo "[dev] starting backend  (go run, 127.0.0.1:18871)"
(cd "$ROOT/apps/server" && "$GO" run ./cmd/server -addr 127.0.0.1:18871 -data "$ROOT/data" \
  > "$LOGDIR/server.log" 2>&1) &

echo "[dev] starting frontend (vite, 127.0.0.1:5273)"
(cd "$ROOT/apps/web" && npm run dev -- --host 127.0.0.1 --port 5273 \
  > "$LOGDIR/web.log" 2>&1) &

for i in $(seq 1 45); do
  srv="$(curl -s -m 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:18871/api/health 2>/dev/null)"
  if [ "$srv" = "200" ] && port_busy 5273; then
    echo "[dev] backend  ready  http://127.0.0.1:18871"
    echo "[dev] frontend ready  http://127.0.0.1:5273"
    echo "[dev] logs: $LOGDIR"
    echo "[dev] stop: scripts/dev-stop.sh"
    exit 0
  fi
  sleep 2
done

echo "[dev] WARNING: not ready within 90s, check $LOGDIR/server.log and web.log"
exit 1
