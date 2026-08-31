#!/usr/bin/env bash
# mPackStation 契约验收四层入口(重构行为不变性证明)。
#   层1 静态门禁: go vet / gofmt / go test / 前端 tsc
#   层2 curl 契约矩阵(41 项)            scripts/contract/curl-matrix.sh
#   层3 E2E 直连(22 项)                 scripts/contract/e2e-harness.ts 打包自真实前端 API 层
#   层4 E2E 经 vite 代理(22 项)         同上, base 换 vite dev 端口
# 用法: bash scripts/verify-contract.sh [--skip-proxy]
# 退出码: 0=全绿。测试实例端口 18899 / vite 5274, 数据在 scripts/contract/.tmp(每次清空),
# 不影响日常 dev 实例(18871)。
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="$ROOT/.tools/go/bin/go.exe"
GOFMT="$ROOT/.tools/go/bin/gofmt.exe"
SRV="$ROOT/apps/server"
WEB="$ROOT/apps/web"
PORT=18899
BASE="http://127.0.0.1:$PORT"
VITE_PORT=5274
TMP="$ROOT/scripts/contract/.tmp"
RUNID="v$(date +%s)"
SKIP_PROXY=0
[ "${1:-}" = "--skip-proxy" ] && SKIP_PROXY=1
FAILED=""

say() { echo; echo "=== $1 ==="; }
kill_port() { # 按端口杀监听进程(Windows: git-bash kill 杀不掉 native 子进程, 用 powershell)
  powershell -NoProfile -Command "\$p = Get-NetTCPConnection -LocalPort $1 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty OwningProcess; if (\$p) { Stop-Process -Id \$p -Force }" >/dev/null 2>&1
}
wait_health() { # wait_health <url> <秒>; 注意 curl -o /dev/null 在 Windows 上恒 exit 23, 必须用 shell 重定向
  local i
  for i in $(seq 1 "$2"); do
    curl -s "$1" >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

mkdir -p "$TMP"

say "层1 静态门禁"
(cd "$SRV" && "$GO" vet ./...) || FAILED="$FAILED vet"
UNFMT=$(cd "$SRV" && "$GOFMT" -l cmd internal | sed 's#\\#/#g')
# 历史遗留: 一批老文件以 CRLF 存库, gofmt 恒报; 基线外新增才算失败
NEWUNFMT=$(comm -13 <(sort "$ROOT/scripts/contract/gofmt-baseline.txt") <(echo "$UNFMT" | sed '/^$/d' | sort))
[ -n "$NEWUNFMT" ] && { echo "gofmt 新增未通过: $NEWUNFMT"; FAILED="$FAILED gofmt"; }
(cd "$SRV" && "$GO" test ./... -count=1 -timeout=180s > "$TMP/go-test.log" 2>&1) || FAILED="$FAILED go-test"
grep -v "^ok\|no test files" "$TMP/go-test.log" || true
(cd "$WEB" && npx tsc -p tsconfig.json --noEmit) || FAILED="$FAILED tsc"
echo "层1 完成 ${FAILED:+[已有失败:$FAILED]}"

say "层2/3 准备: 构建并启动隔离实例 $BASE"
TMP_WIN=$(cygpath -w "$TMP")
(cd "$SRV" && "$GO" build -o "$TMP_WIN\\mpack-server-verify.exe" ./cmd/server) || { echo "构建失败"; exit 1; }
kill_port $PORT
rm -rf "$TMP/data"
"$TMP/mpack-server-verify.exe" -addr "127.0.0.1:$PORT" -data "$TMP_WIN\\data" >"$TMP/server.log" 2>&1 &
SRVPID=$!
wait_health "$BASE/api/system/health" 15 || { echo "测试实例未就绪, 见 $TMP/server.log"; kill_port $PORT; exit 1; }
TOKEN=$(tr -d '\r\n' < "$TMP/data/runtime-token")
echo "实例就绪 (pid=$SRVPID)"

say "层2 curl 契约矩阵"
BASE="$BASE" TOKEN="$TOKEN" TMPDIR="$TMP_WIN" RUNID="$RUNID" bash "$ROOT/scripts/contract/curl-matrix.sh" || FAILED="$FAILED curl-matrix"

say "层3 E2E 直连 $BASE"
ROOT_WIN=$(cygpath -w "$ROOT")
ZIP_B64=$(python -c "import zipfile,io,base64; b=io.BytesIO(); z=zipfile.ZipFile(b,'w'); z.writestr('manifest.json','{\"name\":\"e2e-$RUNID\",\"version\":\"1.0\"}'); z.close(); print(base64.b64encode(b.getvalue()).decode())")
(cd "$WEB" && npx esbuild "$ROOT_WIN\\scripts\\contract\\e2e-harness.ts" --bundle --platform=node --format=cjs --target=node18 --define:__MPACK_WRITE_TOKEN__=\"$TOKEN\" --outfile="$TMP_WIN\\e2e-harness.cjs" --log-level=warning) || { echo "harness 打包失败"; FAILED="$FAILED harness-build"; }
if [ -f "$TMP/e2e-harness.cjs" ]; then
  MPACK_E2E_BASE="$BASE" MPACK_E2E_ZIP_B64="$ZIP_B64" MPACK_E2E_RUN="$RUNID" node "$TMP_WIN\\e2e-harness.cjs" || FAILED="$FAILED e2e-direct"
fi

if [ "$SKIP_PROXY" = "0" ]; then
  say "层4 E2E 经 vite 代理 127.0.0.1:$VITE_PORT"
  kill_port $VITE_PORT
  (cd "$WEB" && VITE_API_TARGET="$BASE" VITE_MPACK_TOKEN="$TOKEN" npx vite --port $VITE_PORT --strictPort >"$TMP/vite.log" 2>&1) &
  wait_health "http://127.0.0.1:$VITE_PORT/api/system/health" 30 || { echo "vite 代理未就绪, 见 $TMP/vite.log"; FAILED="$FAILED vite-boot"; }
  if [ -f "$TMP/e2e-harness.cjs" ]; then
    MPACK_E2E_BASE="http://127.0.0.1:$VITE_PORT" MPACK_E2E_ZIP_B64="$ZIP_B64" MPACK_E2E_RUN="${RUNID}p" node "$TMP_WIN\\e2e-harness.cjs" || FAILED="$FAILED e2e-proxy"
  fi
  kill_port $VITE_PORT
fi

say "清理"
kill_port $PORT

echo
if [ -z "$FAILED" ]; then
  echo "== 契约验收: 全绿 =="
  exit 0
else
  echo "== 契约验收: 失败项:$FAILED =="
  exit 1
fi
