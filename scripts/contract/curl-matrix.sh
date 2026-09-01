#!/usr/bin/env bash
# mPackStation 契约 curl 矩阵(41 项断言, 2026-08-30/31 两轮矩阵合并超集)。
# 通常由 scripts/verify-contract.sh 调用;也可独立运行:
#   BASE=http://127.0.0.1:18899 TOKEN=<runtime-token> TMPDIR=<dir> bash scripts/contract/curl-matrix.sh
# 断言: 状态码、错误码、信封形状、null 不省略、Task DTO、内容修订链、导入两阶段与幂等(含 idempotency_conflict 回归)。
set -u
B=${BASE:?需要 BASE}
T=${TOKEN:?需要 TOKEN}
TMP=${TMPDIR:-.}
RUNID=${RUNID:-r$RANDOM}
mkdir -p "$TMP" 2>/dev/null || true
# TMP 必须是原生 Windows 路径(verify-contract.sh 已 cygpath 转换);
# 独立运行时若传入 MSYS 路径, 这里兜底转换, 供 curl/python 等原生程序使用
if [[ "$TMP" == /* ]]; then TMP=$(cygpath -w "$TMP"); fi
rm -f "$TMP"/c-*.json
PASS=0; FAIL=0
ck() { local name="$1" got="$2" want="$3"
  if [[ "$got" == *"$want"* ]]; then PASS=$((PASS+1)); echo "PASS $name";
  else FAIL=$((FAIL+1)); echo "FAIL $name"; echo "  got: ${got:0:300}"; echo "  want~: $want"; fi
}

# --- 鉴权/结构 ---
ck "POST无token→401" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/packs -H 'content-type: application/json' -d '{}')" "401"
ck "无content-type→415" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/packs -H "X-MPack-Token: $T" -d '{}')" "415"
ck "坏JSON→400 invalid_argument" "$(curl -s -X POST $B/api/packs -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{bad')" '"code":"invalid_argument"'
ck "recent=101→400(不静默clamp)" "$(curl -s -o /dev/null -w '%{http_code}' "$B/api/tasks?recent=101")" "400"
ck "大body→413 payload_too_large" "$(python -c "
import json,urllib.request,http.client
body=json.dumps({'name':'x','mcVersion':'1.20.1','loader':'fabric','description':'d'*9_000_000}).encode()
code='ERR'
for _ in range(3):
    # 服务端恒回 413;但连接可能在响应送达前被重置(9MB 在途),重试取首个可读状态码
    req=urllib.request.Request('$B/api/packs',data=body,headers={'content-type':'application/json','X-MPack-Token':'$T'})
    try:
        urllib.request.urlopen(req); code='200'; break
    except urllib.error.HTTPError as e:
        code=str(e.code); break
    except Exception:
        continue
print(code)
")" "413"

# --- 包 CRUD ---
ck "创建包→201" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/packs -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"name\":\"alpha-$RUNID\",\"mcVersion\":\"1.20.1\",\"loader\":\"fabric\",\"loaderVersion\":\"0.15\"}")" "201"
ck "创建重名→422 pack_name_duplicate" "$(curl -s -X POST $B/api/packs -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"name\":\"alpha-$RUNID\",\"mcVersion\":\"1.20.1\",\"loader\":\"fabric\"}")" '"code":"pack_name_duplicate"'
ck "MC版本不支持→422" "$(curl -s -X POST $B/api/packs -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"name\":\"bad-mc-$RUNID\",\"mcVersion\":\"1.12.2\",\"loader\":\"fabric\"}")" '"code":"pack_unsupported_mc_version"'
curl -s -o /dev/null -X POST $B/api/packs -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"name\":\"beta-$RUNID\",\"mcVersion\":\"1.20.1\",\"loader\":\"fabric\"}"
PID=$(curl -s $B/api/packs -o $TMP/c-packs.json && python -c "import json; d=json.load(open(r'$TMP/c-packs.json',encoding='utf-8')); print([p['id'] for p in d['items'] if p['name']=='beta-$RUNID'][0])")
ck "包列表信封" "$(python -c "import json; print(sorted(json.load(open(r'$TMP/c-packs.json',encoding='utf-8')).keys()))")" "['items', 'next_cursor', 'total']"
ck "包DTO空字段为null" "$(curl -s $B/api/packs/$PID)" '"iconUrl":null'
ck "PATCH重名→422 pack_name_duplicate" "$(curl -s -X PATCH $B/api/packs/$PID -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"name\":\"alpha-$RUNID\"}")" '"code":"pack_name_duplicate"'
ck "不存在包→404 pack_not_found" "$(curl -s $B/api/packs/nope)" '"code":"pack_not_found"'
ck "duplicate路由已删→404" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/packs/$PID/duplicate -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{}')" "404"

# --- 任务/活动信封 ---
ck "tasks信封键" "$(curl -s "$B/api/tasks?recent=5" -o $TMP/c-tasks0.json && python -c "import json; print(sorted(json.load(open(r'$TMP/c-tasks0.json',encoding='utf-8')).keys()))")" "['items', 'next_cursor', 'total']"
ck "activities信封键" "$(curl -s "$B/api/activities?limit=5" -o $TMP/c-act.json && python -c "import json; print(sorted(json.load(open(r'$TMP/c-act.json',encoding='utf-8')).keys()))")" "['items', 'next_cursor', 'total']"

# --- onboarding ---
ck "写prismAccount→422 readonly" "$(curl -s -X PUT $B/api/onboarding -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"steps":{"prismAccount":true}}')" '"code":"onboarding_step_readonly"'
ck "未知步骤→422 unknown_step" "$(curl -s -X PUT $B/api/onboarding -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"steps":{"nope":true}}')" '"code":"onboarding_unknown_step"'
ck "正常步骤→200" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/api/onboarding -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"steps":{"curseforgeKey":true}}')" "200"

# --- 内容修订链 ---
curl -s -X POST $B/api/packs/$PID/content -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"kind":"recipe","slug":"r1","title":"recipe one","payload":{"schema_version":1,"type":"minecraft:crafting_shaped","input":{"item":"minecraft:stone"},"output":{"item":"minecraft:stone"}}}' -o $TMP/c-content.json
CID=$(python -c "import json; print(json.load(open(r'$TMP/c-content.json',encoding='utf-8'))['document']['id'])")
ck "内容创建201" "$(head -c 60 $TMP/c-content.json)" '"document"'
ck "缺If-Match→400" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/api/packs/$PID/content/$CID/draft -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"payload":{}}')" "400"
ck "stale If-Match→412 revision_conflict" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/api/packs/$PID/content/$CID/draft -H "X-MPack-Token: $T" -H 'content-type: application/json' -H 'If-Match: "99"' -d '{"payload":{"schema_version":1,"type":"minecraft:crafting_shaped","input":{"item":"minecraft:stone"},"output":{"item":"minecraft:dirt"}}}')" "412"
ck "正常draft→200 revision=2" "$(curl -s -X PUT $B/api/packs/$PID/content/$CID/draft -H "X-MPack-Token: $T" -H 'content-type: application/json' -H 'If-Match: "1"' -d '{"payload":{"schema_version":1,"type":"minecraft:crafting_shaped","input":{"item":"minecraft:stone"},"output":{"item":"minecraft:dirt"}}}')" '"revision":2'
ck "内容slug重复→422 content_duplicate_slug" "$(curl -s -X POST $B/api/packs/$PID/content -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"kind":"recipe","slug":"r1","title":"dup","payload":{}}')" '"code":"content_duplicate_slug"'
ck "apply出参带revision" "$(curl -s -X POST "$B/api/packs/$PID/content/$CID/apply" -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{}')" '"status":"applied"'
ck "apply不存在文档→404 content_not_found" "$(curl -s -X POST "$B/api/packs/$PID/content/nope/apply" -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{}')" '"code":"content_not_found"'

# --- 导入两阶段(含 idempotency_conflict 回归) ---
RUNID="$RUNID" TMP="$TMP" python - <<'PYEOF'
import zipfile, io, base64, json, os
buf = io.BytesIO()
with zipfile.ZipFile(buf, 'w') as z:
    z.writestr('manifest.json', '{"name":"curl-import-%s","version":"1.0"}' % os.environ['RUNID'])
body = {"source":"local_zip","content": base64.b64encode(buf.getvalue()).decode()}
open(os.path.join(os.environ['TMP'], 'c-inspect-body.json'),'w').write(json.dumps(body))
PYEOF
curl -s -X POST $B/api/packs/import/inspect -H "X-MPack-Token: $T" -H 'content-type: application/json' -d @$TMP/c-inspect-body.json -o $TMP/c-preview.json
PVID=$(python -c "import json; print(json.load(open(r'$TMP/c-preview.json',encoding='utf-8'))['id'])")
PTOK=$(python -c "import json; print(json.load(open(r'$TMP/c-preview.json',encoding='utf-8'))['token'])")
PHASH=$(python -c "import json; print(json.load(open(r'$TMP/c-preview.json',encoding='utf-8'))['inputHash'])")
ck "预览生成" "$PVID" "import-"
ck "confirm缺幂等键→400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/packs/import -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"previewId\":\"$PVID\",\"token\":\"$PTOK\",\"inputHash\":\"$PHASH\"}")" "400"
ck "confirm token错→400 invalid_argument" "$(curl -s -X POST $B/api/packs/import -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"previewId\":\"$PVID\",\"token\":\"wrong\",\"inputHash\":\"$PHASH\",\"idempotencyKey\":\"k-$RUNID-1\"}")" '"code":"invalid_argument"'
ck "confirm hash错→422 import_input_mismatch" "$(curl -s -X POST $B/api/packs/import -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"previewId\":\"$PVID\",\"token\":\"$PTOK\",\"inputHash\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"idempotencyKey\":\"k-$RUNID-1\"}")" '"code":"import_input_mismatch"'
curl -s -X POST $B/api/packs/import -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"previewId\":\"$PVID\",\"token\":\"$PTOK\",\"inputHash\":\"$PHASH\",\"idempotencyKey\":\"k-$RUNID-1\"}" -o $TMP/c-confirm1.json
ck "confirm出参无内嵌task" "$(python -c "import json; print(sorted(json.load(open(r'$TMP/c-confirm1.json',encoding='utf-8')).keys()))")" "['importId', 'packId', 'reused', 'taskId']"
TID=$(python -c "import json; print(json.load(open(r'$TMP/c-confirm1.json',encoding='utf-8'))['taskId'])")
ck "重复confirm同输入→返原任务" "$(curl -s -X POST $B/api/packs/import -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"previewId\":\"$PVID\",\"token\":\"$PTOK\",\"inputHash\":\"$PHASH\",\"idempotencyKey\":\"k-$RUNID-2\"}" -o $TMP/c-confirm2.json && python -c "import json; d=json.load(open(r'$TMP/c-confirm2.json',encoding='utf-8')); print(d['reused'], d['taskId']=='$TID')")" "True True"
# 回归: 同幂等键不同输入 → 422 idempotency_conflict (2026-08-31 E2E 抓到的 500 bug)
RUNID="$RUNID" TMP="$TMP" python - <<'PYEOF'
import zipfile, io, base64, json, os
buf = io.BytesIO()
with zipfile.ZipFile(buf, 'w') as z:
    z.writestr('manifest.json', '{"name":"curl-import-b-%s","version":"2.0"}' % os.environ['RUNID'])
body = {"source":"local_zip","content": base64.b64encode(buf.getvalue()).decode()}
open(os.path.join(os.environ['TMP'], 'c-inspect-body-b.json'),'w').write(json.dumps(body))
PYEOF
curl -s -X POST $B/api/packs/import/inspect -H "X-MPack-Token: $T" -H 'content-type: application/json' -d @$TMP/c-inspect-body-b.json -o $TMP/c-preview-b.json
PVID2=$(python -c "import json; print(json.load(open(r'$TMP/c-preview-b.json',encoding='utf-8'))['id'])")
PTOK2=$(python -c "import json; print(json.load(open(r'$TMP/c-preview-b.json',encoding='utf-8'))['token'])")
PHASH2=$(python -c "import json; print(json.load(open(r'$TMP/c-preview-b.json',encoding='utf-8'))['inputHash'])")
ck "同键不同输入→422 idempotency_conflict" "$(curl -s -X POST $B/api/packs/import -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"previewId\":\"$PVID2\",\"token\":\"$PTOK2\",\"inputHash\":\"$PHASH2\",\"idempotencyKey\":\"k-$RUNID-1\"}")" '"code":"idempotency_conflict"'
curl -s $B/api/tasks/$TID -o $TMP/c-task.json
ck "任务详情=契约Task键" "$(python -c "import json; print(sorted(json.load(open(r'$TMP/c-task.json',encoding='utf-8')).keys()))")" "['error', 'finishedAt', 'id', 'packId', 'packName', 'progress', 'startedAt', 'status', 'title', 'type']"
ck "任务type=import-pack" "$(python -c "import json; print(json.load(open(r'$TMP/c-task.json',encoding='utf-8'))['type'])")" "import-pack"
ck "任务progress为int 0-100" "$(python -c "import json; p=json.load(open(r'$TMP/c-task.json',encoding='utf-8'))['progress']; print(isinstance(p,int) and 0<=p<=100)")" "True"
sleep 2
ck "tasks信封含新任务" "$(curl -s "$B/api/tasks?recent=5" -o $TMP/c-tasks.json && python -c "import json; d=json.load(open(r'$TMP/c-tasks.json',encoding='utf-8')); print(d['total']>=1, any(t['id']=='$TID' for t in d['items']))")" "True True"

# --- 导入来源校验 ---
ck "URL非https→400" "$(curl -s -X POST $B/api/packs/import/inspect -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"source":"modrinth_url","url":"http://modrinth.com/mod/x"}')" '"code":"invalid_argument"'
ck "URL域名不符→422 import_invalid_source" "$(curl -s -X POST $B/api/packs/import/inspect -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"source":"modrinth_url","url":"https://evil.example.com/x.zip"}')" '"code":"import_invalid_source"'
ck "source不在枚举→400" "$(curl -s -X POST $B/api/packs/import/inspect -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"source":"bogus"}')" '"code":"invalid_argument"'

# --- mod-versions 必填 ---
ck "mod-versions缺参→400" "$(curl -s -o /dev/null -w '%{http_code}' "$B/api/packs/$PID/mod-versions")" "400"

# --- 兼容知识库推荐(beta 包是 1.20.1 fabric: Polymorph 普遍适用必出现, Sinytra 限 forge/neoforge 必缺席) ---
curl -s "$B/api/packs/$PID/mod-recommendations" -o $TMP/c-recs.json
ck "推荐含Polymorph" "$(python -c "import json; d=json.load(open(r'$TMP/c-recs.json',encoding='utf-8')); print([i['name'] for i in d['items']])" | grep -c Polymorph)" "1"
ck "推荐不含Sinytra(loader不符)" "$(python -c "import json; d=json.load(open(r'$TMP/c-recs.json',encoding='utf-8')); print([i['name'] for i in d['items']])" | grep -c Sinytra)" "0"
ck "推荐项走modrinth" "$(python -c "import json; d=json.load(open(r'$TMP/c-recs.json',encoding='utf-8')); print([i['provider'] for i in d['items'] if i['name']=='Polymorph'][0])")" "modrinth"

# --- 发布异步出参 ---
ck "publish async缺包→404 pack_not_found" "$(curl -s -X POST $B/api/packs/nope/publish/modrinth/async -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"packVersionId":"v","artifactId":"a","projectId":"p"}')" '"code":"pack_not_found"'

# --- CurseForge key 管理(需环境有真 key 才能跑验证调用) ---
if [ -n "${CURSEFORGE_API_KEY:-}" ]; then
  ck "CF坏key→400" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/api/system/providers/curseforge/key -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"key":"invalid-key-for-matrix-test"}')" 400
  ck "CF坏key错误码" "$(curl -s -X PUT $B/api/system/providers/curseforge/key -H "X-MPack-Token: $T" -H 'content-type: application/json' -d '{"key":"invalid-key-for-matrix-test"}')" "invalid_argument"
  ck "CF真key→204" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/api/system/providers/curseforge/key -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"key\":\"$CURSEFORGE_API_KEY\"}")" 204
  ck "保存后已配置" "$(curl -s $B/api/system/health)" '"curseforgeKeyConfigured":true'
  ck "清除→204" "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE $B/api/system/providers/curseforge/key -H "X-MPack-Token: $T")" 204
  ck "清除后未配置" "$(curl -s $B/api/system/health)" '"curseforgeKeyConfigured":false'
  ck "恢复真key→204" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/api/system/providers/curseforge/key -H "X-MPack-Token: $T" -H 'content-type: application/json' -d "{\"key\":\"$CURSEFORGE_API_KEY\"}")" 204
  # --- 双平台合并(JEI 在内置身份知识库, 双平台可达时必合并) ---
  curl -s "$B/api/packs/$PID/mod-search?q=jei&limit=10" -o $TMP/c-merge.json
  ck "JEI合并为一张卡" "$(python -c "import json; d=json.load(open(r'$TMP/c-merge.json',encoding='utf-8')); print(len([i for i in d['items'] if 'justenoughitems' in i['name'].lower().replace(' ','')]))")" "1"
  ck "合并卡带mirror双平台" "$(python -c "import json; d=json.load(open(r'$TMP/c-merge.json',encoding='utf-8')); i=[x for x in d['items'] if 'justenoughitems' in x['name'].lower().replace(' ','')][0]; print(sorted([i['provider'], i.get('mirror',{}).get('provider','')]))")" "['curseforge', 'modrinth']"
else
  echo "SKIP: CF key 管理 7 项 + 双平台合并 2 项(环境无 CURSEFORGE_API_KEY)"
fi

echo "================================"
echo "curl-matrix: PASS=$PASS FAIL=$FAIL"
exit $((FAIL > 0))
