#!/usr/bin/env bash
# mPackStation 端到端能力基线：逐段验证「建包 → 加模组 → 锁依赖 → 目录/配方 → 任务书 → 构建 .mrpack → 启动器」
# 用法：BASE=<后端地址> TOKEN=<写令牌> bash scripts/e2e-baseline.sh
set -uo pipefail

BASE="${BASE:-http://127.0.0.1:18899}"
TOKEN="${TOKEN:-e2e-token-123}"
OUT="${OUT:-/tmp/e2e-out}"
DATA="${DATA:-/tmp/mpack-e2e}"
mkdir -p "$OUT"
PASS=0; FAIL=0; SKIP=0

say() { printf '\n### %s\n' "$*"; }
# step <名称> <期望码> <curl 参数...>
step() {
  local name="$1" want="$2"; shift 2
  local log="$OUT/${name// /_}.log"
  local code
  code=$(curl -s -o "$log" -w '%{http_code}' "$@")
  if [[ "$code" == "$want" ]]; then
    PASS=$((PASS+1)); printf 'PASS  %-46s HTTP %s\n' "$name" "$code"
  else
    FAIL=$((FAIL+1)); printf 'FAIL  %-46s HTTP %s (want %s) body=%s\n' "$name" "$code" "$want" "$(head -c 300 "$log")"
  fi
}
jqp() { python3 -c "
import json,sys
d=json.load(open(sys.argv[1]))
print(json.dumps(d,ensure_ascii=False)[:400])
" "$1" 2>/dev/null || echo "(non-json)"; }
pick() { python3 -c "
import json,sys
d=json.load(open(sys.argv[1]))
path=sys.argv[2].split('.')
cur=d
for p in path:
    if isinstance(cur,list): cur=cur[int(p)]
    elif isinstance(cur,dict): cur=cur.get(p)
    else: cur=None; break
print(cur if cur is not None else '')
" "$1" "$2" 2>/dev/null || echo ''; }

W=(-H "X-MPack-Token: $TOKEN" -H 'Content-Type: application/json')
MCVER="${MCVER:-1.21.1}"
LOADER="${LOADER:-fabric}"
LOADER_VERSION="${LOADER_VERSION:-0.16.9}"
RUN="E2E Baseline $(date +%m%d-%H%M%S)"

say '0 环境'
step 'health' 200 "$BASE/api/health"
printf '     %s\n' "$(cat "$OUT/health.log")"
df -h "$DATA" | tail -1 | awk '{printf "     数据盘可用 %s (%s)\n", $4, $6}'

say '1 新建整合包'
step 'create pack' 201 -X POST "${W[@]}" -d "{\"name\":\"$RUN\",\"mcVersion\":\"$MCVER\",\"loader\":\"$LOADER\",\"loaderVersion\":\"$LOADER_VERSION\",\"description\":\"auto e2e\"}" "$BASE/api/packs"
PACK=$(pick "$OUT/create_pack.log" id)
printf '     packId=%s\n' "$PACK"
[[ -z "$PACK" ]] && { echo '无法建包，后续步骤跳过'; exit 1; }

say '2 面向包搜索模组（双平台并发扇出）'
for q in jei "roughly enough items" "cloth config"; do
  f="$OUT/search_$(echo "$q" | tr ' ' '_').log"
  code=$(curl -s -o "$f" -w '%{http_code}' -G --data-urlencode "q=$q" --data-urlencode "mcVersion=1.21.1" --data-urlencode "loader=fabric" "$BASE/api/packs/$PACK/mod-search")
  n=$(python3 -c "
import json,sys
try:
    d=json.load(open(sys.argv[1])); items=d.get('items') or d.get('results') or []
    print(len(items), items[0].get('provider','?'), items[0].get('projectId','?'), (items[0].get('displayName') or items[0].get('name','?')) if items else '')
except Exception as e: print('parse-error', e)
" "$f" 2>/dev/null)
  printf '     HTTP %s  hits=%s\n' "$code" "$n"
  [[ "$code" == 200 ]] && PASS=$((PASS+1)) || FAIL=$((FAIL+1))
done

say '3 添加模组（搜索条目无 versionId，必须再查 /mod-versions 取兼容版本）'
ADDED=()
for q in jei "roughly enough items" "cloth config"; do
  f="$OUT/search_$(echo "$q" | tr ' ' '_').log"
  line=$(python3 -c "
import json,sys
try:
    d=json.load(open(sys.argv[1])); items=d.get('items') or []
    if not items: print('',''); sys.exit()
    m=items[0]
    print(m.get('provider') or 'modrinth', m.get('id') or m.get('projectId') or '')
except Exception: print('','')
" "$f" 2>/dev/null)
  read -r PROV PID <<<"$line"
  if [[ -z "$PID" ]]; then printf '     SKIP %s（搜索无结果）\n' "$q"; SKIP=$((SKIP+1)); continue; fi
  vf="$OUT/mod_versions_${PID}.log"
  vcode=$(curl -s -o "$vf" -w '%{http_code}' -G --data-urlencode "provider=$PROV" --data-urlencode "projectId=$PID" --data-urlencode "mcVersion=$MCVER" --data-urlencode "loader=$LOADER" "$BASE/api/packs/$PACK/mod-versions")
  VID=$(python3 -c "
import json,sys
mc,ld=sys.argv[2],sys.argv[3]
try:
    items=json.load(open(sys.argv[1])).get('items') or []
except Exception: items=[]
for v in items:  # 服务端按新→旧返回，取首个兼容条目
    if mc in (v.get('gameVersions') or []) and ld in (v.get('loaders') or []):
        print(v.get('id') or ''); break
else:
    print('')
" "$vf" "$MCVER" "$LOADER" 2>/dev/null)
  if [[ -z "$VID" ]]; then
    printf '     SKIP add %s（HTTP %s，%s %s 无兼容版本，共 %s 条）\n' "$PROV/$PID" "$vcode" "$MCVER" "$LOADER" "$(python3 -c "import json;print(len(json.load(open('$vf')).get('items') or []))" 2>/dev/null)"
    SKIP=$((SKIP+1)); continue
  fi
  printf '     版本解析 %s/%s -> %s（候选 %s 条）\n' "$PROV" "$PID" "$VID" "$(python3 -c "import json;print(len(json.load(open('$vf')).get('items') or []))" 2>/dev/null)"
  step "add mod $PROV-$PID" 201 -X POST "${W[@]}" -d "{\"provider\":\"$PROV\",\"projectId\":\"$PID\",\"versionId\":\"$VID\"}" "$BASE/api/packs/$PACK/mods"
  ADDED+=("$(pick "$OUT/add_mod_${PROV}-${PID}.log" id)")
done

say '4 包内模组清单'
step 'list pack mods' 200 "$BASE/api/packs/$PACK/mods"
python3 -c "
import json
d=json.load(open('$OUT/list_pack_mods.log')); items=d.get('items') or []
print('     已装 %d 个模组：%s' % (len(items), ', '.join((i.get('displayName') or '?') for i in items)))
for i in items:
    print('       -', i.get('status'), i.get('source'), (i.get('fileName') or '')[:48], '| mirror=', i.get('mirrorSource'))
" 2>/dev/null || head -c 200 "$OUT/list_pack_mods.log"

say '5 依赖解析与冲突'
step 'resolve dependencies' 202 -X POST "${W[@]}" -d '{}' "$BASE/api/packs/$PACK/resolve"
step 'list locks' 200 "$BASE/api/packs/$PACK/locks"
step 'list conflicts' 200 "$BASE/api/packs/$PACK/conflicts"
python3 -c "
import json
lk=json.load(open('$OUT/list_locks.log')).get('items') or []
cf=json.load(open('$OUT/list_conflicts.log')).get('items') or []
print('     锁定快照 %d 份，待处理冲突 %d 条' % (len(lk), len(cf)))
for c in cf[:5]: print('       -', c.get('kind'), c.get('status'), (c.get('detail') or '')[:60])
" 2>/dev/null || true

say '6 包内容目录（物品/方块/配方/标签）重建'
step 'catalog rebuild submit' 202 -X POST "${W[@]}" -d '{}' "$BASE/api/packs/$PACK/catalog/rebuild"
TASK=$(pick "$OUT/catalog_rebuild_submit.log" taskId)
printf '     taskId=%s，轮询中…\n' "$TASK"
for i in $(seq 1 60); do
  curl -s -o "$OUT/catalog_status.log" "$BASE/api/packs/$PACK/catalog/status"
  ST=$(pick "$OUT/catalog_status.log" status)
  [[ "$ST" == "succeeded" || "$ST" == "failed" ]] && break
  sleep 5
done
printf '     目录状态=%s（轮询 %ds）\n' "$ST" "$((i*5))"
printf '     %s\n' "$(head -c 400 "$OUT/catalog_status.log")"
step 'catalog items page' 200 "$BASE/api/packs/$PACK/catalog?limit=5"
head -c 500 "$OUT/catalog_items_page.log"; echo

say '7 抽查具体物品与配方'
ITEM=$(python3 -c "
import json
items=json.load(open('$OUT/catalog_items_page.log')).get('items') or []
print(items[0]['id'] if items else '')
" 2>/dev/null)
if [[ -n "$ITEM" ]]; then
  step "item detail $ITEM" 200 "$BASE/api/packs/$PACK/catalog/items/$ITEM"
  python3 - <<PY 2>/dev/null || head -c 300 "$OUT/item_detail_$ITEM.log"
import json
d=json.load(open("$OUT/item_detail_$ITEM.log".replace(' ','%20')))
print('     名称=',d.get('displayName'),'| 语言数=',len(d.get('names') or []),'| 图标=',d.get('iconStatus'),'| 标签=',d.get('tags'),'| 字段=',sorted(d.keys()))
PY
else
  printf '     SKIP 无物品可抽查\n'; SKIP=$((SKIP+1))
fi

say '8 模组内容解析（JEI 风格配方）'
MOD=${ADDED[0]}
if [[ -n "${MOD:-}" ]]; then
  step 'parse mod content' 202 -X POST "${W[@]}" -d '{}' "$BASE/api/packs/$PACK/mods/$MOD/content/parse"
  PARSE_TASK=$(pick "$OUT/parse_mod_content.log" taskId)
  printf '     parse taskId=%s，轮询至终态…\n' "$PARSE_TASK"
  for i in $(seq 1 40); do
    curl -s -o "$OUT/parse_task.log" "$BASE/api/tasks/$PARSE_TASK"
    PST=$(pick "$OUT/parse_task.log" status)
    case "$PST" in success|failed|cancelled) break;; esac
    sleep 5
  done
  printf '     解析任务终态=%s（%ds）\n' "$PST" "$((i*5))"
  step 'list parsed content' 200 "$BASE/api/packs/$PACK/mods/$MOD/content?limit=5"
  python3 -c "
import json
d=json.load(open('$OUT/list_parsed_content.log')); items=d.get('items') or []
from collections import Counter
run=d.get('run') or {}
print('     列表页 %d 条 / run.parsedCount=%s status=%s，按类型：'% (len(items), run.get('parsedCount'), run.get('status')), dict(Counter(i.get('kind') for i in items)))
for i in items[:3]: print('       -', i.get('kind'), i.get('contentId') or i.get('id'))
" 2>/dev/null || head -c 250 "$OUT/list_parsed_content.log"
else
  printf '     SKIP 没有可用模组\n'; SKIP=$((SKIP+1))
fi

say '8b 解析完成后再重建目录（验证目录/解析时序耦合）'
step 'catalog rebuild after parse' 202 -X POST "${W[@]}" -d '{}' "$BASE/api/packs/$PACK/catalog/rebuild"
TASK2=$(pick "$OUT/catalog_rebuild_after_parse.log" taskId)
for i in $(seq 1 60); do
  curl -s -o "$OUT/catalog_status2.log" "$BASE/api/packs/$PACK/catalog/status"
  ST2=$(pick "$OUT/catalog_status2.log" status)
  [[ "$ST2" == "succeeded" || "$ST2" == "failed" ]] && break
  sleep 5
done
printf '     目录状态=%s（轮询 %ds）\n' "$ST2" "$((i*5))"
python3 -c "
import json
d=json.load(open('$OUT/catalog_status2.log'))
print('     warnings:', json.dumps(d.get('warnings') or [], ensure_ascii=False))
" 2>/dev/null || head -c 300 "$OUT/catalog_status2.log"

say '9 任务书闭环（草稿 → 校验 → 应用）'
step 'get quest' 200 "$BASE/api/packs/$PACK/quests"
GET_Q="$OUT/get_quest.log"
REV=$(python3 -c "
import json
d=json.load(open('$GET_Q'))
r=d.get('revision') or {}
print(r.get('revision') if r.get('revision') is not None else '')
" 2>/dev/null)
DRAFT=$(python3 -c "
import json
draft={
 'book':{'title':'E2E 任务书','icon':'minecraft:book','progressionMode':'flexible'},
 'chapters':[{'id':'ch1','title':'开始','description':'','coverColor':'#C9783B','icon':'minecraft:book','position':0}],
 'nodes':[
  {'id':'n1','chapterId':'ch1','title':'入门','subtitle':'','description':'','icon':'minecraft:wooden_pickaxe','x':48,'y':48,'shape':'circle','size':1,'optional':False,'invisible':False,'dependencyRequirement':'all_completed','minRequiredDependencies':0,'prerequisites':[],'tasks':[{'id':'n1-t1','type':'item','itemId':'minecraft:wooden_pickaxe','count':1}],'rewards':[{'kind':'experience','experience':10}],'modRefs':[],'position':0},
  {'id':'n2','chapterId':'ch1','title':'进阶','subtitle':'','description':'','icon':'minecraft:iron_ingot','x':240,'y':48,'shape':'circle','size':1,'optional':False,'invisible':False,'dependencyRequirement':'all_completed','minRequiredDependencies':0,'prerequisites':['n1'],'tasks':[{'id':'n2-t1','type':'item','itemId':'minecraft:iron_ingot','count':1}],'rewards':[],'modRefs':[],'position':1}],
 'edges':[{'id':'e1','fromNodeId':'n1','toNodeId':'n2'}]}
print(json.dumps(draft,ensure_ascii=False))
" 2>/dev/null)
MATCH=${REV:-0}
[[ "$MATCH" == 0 ]] && printf '     新包无任务书 revision（GET /quests 返回 404），按 revision 0 保存\n'
step 'save quest draft' 200 -X PUT "${W[@]}" -H "If-Match: \"$MATCH\"" -d "$DRAFT" "$BASE/api/packs/$PACK/quests/draft" || true
if [[ -s "$OUT/save_quest_draft.log" ]] && grep -q 'revision' "$OUT/save_quest_draft.log"; then
  REV2=$(python3 -c "
import json
d=json.load(open('$OUT/save_quest_draft.log')); print((d.get('revision') or {}).get('revision') if isinstance(d.get('revision'),dict) else d.get('revision',''))
" 2>/dev/null)
  step 'validate quest' 200 -X POST "${W[@]}" -d '{}' "$BASE/api/packs/$PACK/quests/validate"
  printf '     validate: %s\n' "$(head -c 300 "$OUT/validate_quest.log")"
  step 'apply quest' 200 -X POST "${W[@]}" -d '{}' "$BASE/api/packs/$PACK/quests/apply"
  printf '     apply: %s\n' "$(head -c 240 "$OUT/apply_quest.log")"
  step 'quest preview' 200 "$BASE/api/packs/$PACK/quests/preview"
  step 'quest history' 200 "$BASE/api/packs/$PACK/quests/history"
fi

say '10 构建 .mrpack'
step 'register export dir' 201 -X POST "${W[@]}" -d "{\"name\":\"e2e\",\"directory\":\"$DATA/export\"}" "$BASE/api/export-dirs"
VNUM="0.1.$(date +%H%M%S)"
step 'create pack version' 201 -X POST "${W[@]}" -d "{\"version\":\"$VNUM\",\"channel\":\"release\",\"changelog\":\"e2e baseline\",\"source\":\"manual\"}" "$BASE/api/packs/$PACK/versions"
VER=$(pick "$OUT/create_pack_version.log" id)
printf '     packVersionId=%s version=%s\n' "$VER" "$VNUM"
step 'duplicate version (D1: 期望 409 冲突)' 409 -X POST "${W[@]}" -d "{\"version\":\"$VNUM\",\"channel\":\"release\",\"changelog\":\"dup\",\"source\":\"manual\"}" "$BASE/api/packs/$PACK/versions"
step 'build mrpack' 201 -X POST "${W[@]}" -d "{\"packVersionId\":\"$VER\",\"exportDirName\":\"e2e\"}" "$BASE/api/packs/$PACK/build"
printf '     契约形调用(files 省略): %s\n' "$(head -c 300 "$OUT/build_mrpack.log")"
LOCK_SNAP=$(python3 -c "
import json
lk=json.load(open('$OUT/list_locks.log')).get('items') or []
print(lk[0]['snapshot'] if lk and lk[0].get('snapshot') else '{\"mods\":[]}')
" 2>/dev/null)
MANIFEST_B64=$(python3 -c "
import base64,json,sys
m={'formatVersion':1,'game':'minecraft','versionId':'$(pick "$OUT/create_pack_version.log" version)','name':'$PACK','dependencies':{},'generatedBy':'e2e-baseline'}
print(base64.b64encode(json.dumps(m).encode()).decode())
" 2>/dev/null)
BUILD_BODY="{\"packVersionId\":\"$VER\",\"exportDirName\":\"e2e\",\"files\":[{\"path\":\"modrinth.index.json\",\"content\":\"$MANIFEST_B64\"}],\"lockSnapshot\":$LOCK_SNAP}"
step 'build with files' 201 -X POST "${W[@]}" -d "$BUILD_BODY" "$BASE/api/packs/$PACK/build"
printf '     前端形调用(files 非空): %s\n' "$(head -c 400 "$OUT/build_with_files.log")"
BUILD_TASK=$(pick "$OUT/build_with_files.log" taskId)
if [[ -n "$BUILD_TASK" ]]; then
  for i in $(seq 1 36); do
    curl -s -o "$OUT/build_task.log" "$BASE/api/tasks/$BUILD_TASK"
    TS=$(pick "$OUT/build_task.log" status)
    case "$TS" in success|failed|cancelled) break;; esac
    sleep 5
  done
  printf '     任务终态=%s（等待 %ds）\n' "$TS" "$((i*5))"
  head -c 400 "$OUT/build_task.log"; echo
fi
ART=$(pick "$OUT/build_with_files.log" artifact.fileName)
printf '     产物名=%s\n' "${ART:-（响应无 artifact）}"
mkdir -p "$DATA/export"
echo "     导出目录实际内容："
find "$DATA/export" -type f \( -name '*.mrpack' -o -name '*.zip' \) -exec ls -lh {} \; 2>/dev/null | awk '{printf "       %s %s\n", $5, $9}'
for z in $(find "$DATA/export" -name '*.zip' -o -name '*.mrpack' 2>/dev/null | head -2); do
  printf '     %s 内部清单：\n' "$z"
  unzip -l "$z" 2>/dev/null | tail -8 | sed 's/^/       /'
  unzip -p "$z" modrinth.index.json 2>/dev/null | head -c 200 | sed 's/^/       /'; echo
done

say '11 启动台（launcher）'
step 'launcher install' 202 -X POST "${W[@]}" -d '{"packId":"'"$PACK"'"}' "$BASE/api/launcher/install" || true
BUILD_TASK2=$(pick "$OUT/launcher_install.log" taskId)
[[ -n "$BUILD_TASK2" ]] && curl -s "$BASE/api/tasks/$BUILD_TASK2" -o "$OUT/launcher_task.log" && printf '     安装任务终态: %s\n' "$(head -c 200 "$OUT/launcher_task.log")"
printf '     install 响应: %s\n' "$(head -c 400 "$OUT/launcher_install.log")"

say '汇总'
printf 'PASS=%d FAIL=%d SKIP=%d\n' "$PASS" "$FAIL" "$SKIP"
printf 'packId=%s\n证据目录=%s\n' "$PACK" "$OUT"
