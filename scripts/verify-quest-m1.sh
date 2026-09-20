#!/usr/bin/env bash
# M1 quest graph: API round-trip — create A→B, save with prerequisites sync, GET edges non-empty.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA="${MPACK_M1_DATA:-/tmp/mpack-m1-quest-$$}"
ADDR="127.0.0.1:18907"
TOKEN="m1-quest-token"
LOG_DIR="$ROOT/docs/project-state/history"
LOG="$LOG_DIR/quest-m1-verify-$(date +%Y%m%d-%H%M%S).log"
mkdir -p "$LOG_DIR" "$DATA"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$DATA"
}
trap cleanup EXIT

exec > >(tee "$LOG") 2>&1

echo "=== M1 quest graph verification ==="
echo "date: $(date -Iseconds)"
echo "data: $DATA"
echo "log: $LOG"

echo "--- pure graph assertions ---"
cd "$ROOT/apps/web"
node --experimental-strip-types scripts/questGraph.check.ts

echo "--- tsc -b ---"
npx tsc -b
echo "tsc -b: OK"

echo "--- go quest tests ---"
cd "$ROOT/apps/server"
go test ./internal/service/ -run 'TestP6Quest' -count=1
go test ./internal/httpapi/ -run 'TestP6HTTPQuest' -count=1
echo "go quest tests: OK"

echo "--- start server ---"
cd "$ROOT/apps/server"
BIN="$DATA/mpack-server"
go build -o "$BIN" ./cmd/server
MPACK_TOKEN="$TOKEN" "$BIN" -addr "$ADDR" -data "$DATA" &
SERVER_PID=$!
for i in $(seq 1 60); do
  if curl -sf "http://$ADDR/api/health" >/dev/null 2>&1; then
    echo "server up (pid=$SERVER_PID)"
    break
  fi
  sleep 0.3
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "server died"; exit 1
  fi
  if [[ "$i" -eq 60 ]]; then echo "server health timeout"; exit 1; fi
done

echo "--- create pack ---"
PACK=$(curl -sf -X POST "http://$ADDR/api/packs" \
  -H "content-type: application/json" \
  -H "X-MPack-Token: $TOKEN" \
  -d '{"name":"M1 Quest Pack","mcVersion":"1.20.1","loader":"fabric","loaderVersion":"0.15"}')
echo "$PACK"
PACK_ID=$(node -e "const p=JSON.parse(process.argv[1]); if(!p.id) process.exit(2); process.stdout.write(p.id)" "$PACK")
echo "pack_id=$PACK_ID"

echo "--- UI-equivalent save payload (edges authority + prerequisites sync) ---"
cd "$ROOT/apps/web"
SAVE_BODY=$(node --experimental-strip-types scripts/questM1SaveBody.ts)
echo "$SAVE_BODY"

echo "--- PUT quests/draft ---"
PUT=$(curl -sS -X PUT "http://$ADDR/api/packs/$PACK_ID/quests/draft" \
  -H "content-type: application/json" \
  -H "X-MPack-Token: $TOKEN" \
  -H 'If-Match: "0"' \
  -d "$SAVE_BODY")
echo "$PUT"
echo "$PUT" | node -e "let s='';process.stdin.on('data',d=>s+=d);process.stdin.on('end',()=>{const j=JSON.parse(s); if(!j.revision||j.revision.revision!==1){console.error('PUT fail',s);process.exit(1)}; console.log('PUT revision=1 OK')})"

echo "--- GET quests ---"
GET=$(curl -sf "http://$ADDR/api/packs/$PACK_ID/quests")
echo "$GET"

node -e '
const get = JSON.parse(process.argv[1]);
const draft = get.revision && get.revision.draft;
if (!draft) { console.error("no draft"); process.exit(1); }
const edges = draft.edges || [];
const nodes = draft.nodes || [];
console.log("edges:", JSON.stringify(edges));
console.log("nodes.x/y/prereq:", JSON.stringify(nodes.map(n => ({id:n.id,x:n.x,y:n.y,prerequisites:n.prerequisites}))));
if (!Array.isArray(edges) || edges.length < 1) {
  console.error("FAIL: GET edges empty");
  process.exit(1);
}
const ab = edges.find(e => e.fromNodeId === "A" && e.toNodeId === "B");
if (!ab) { console.error("FAIL: A→B edge missing after GET"); process.exit(1); }
const b = nodes.find(n => n.id === "B");
const pre = Array.isArray(b && b.prerequisites) ? b.prerequisites : [];
if (!pre.includes("A")) {
  console.error("FAIL: B.prerequisites missing A after GET", pre);
  process.exit(1);
}
const a = nodes.find(n => n.id === "A");
const bNode = nodes.find(n => n.id === "B");
if (!a || !bNode || !(a.x > 0 && bNode.x > a.x)) {
  console.error("FAIL: x/y not persisted as canvas coords", a, bNode);
  process.exit(1);
}
console.log("API edges non-empty + prerequisites synced + coords persisted: PASS");
' "$GET"

echo "=== M1 verification ALL PASS ==="
