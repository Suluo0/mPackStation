#!/usr/bin/env bash
# mPackStation dev：一键启动唯一标准服务（后端 18872 + 前端 5271，AGENTS.md 定稿）。
# macOS / Linux；Windows 用 scripts/dev.ps1（同端口、同数据目录口径，2026-10-05 对齐）。
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA=/tmp/mpack-data          # 唯一开发数据目录（空库可直接跑，迎新流程会引导建包）
LOGDIR="$ROOT/.tmp/dev"
mkdir -p "$LOGDIR"

port_busy() { lsof -t -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }

if port_busy 18872; then echo "[dev] 18872 已占用，先 scripts/dev-stop.sh"; exit 1; fi
if port_busy 5271;  then echo "[dev] 5271 已占用，先 scripts/dev-stop.sh"; exit 1; fi

# ── 代理透传（带存活检测） ──────────────────────────────────────────────
# 后端要访问 Modrinth / CurseForge 才能搜索、解析依赖。曾经后端进程环境里
# 没有任何 proxy 变量（脚本没透传），结果两个平台全不可达 → 搜索一律返回
# total=0、降级区也没了，看着像「代码改坏了」，实际是出不了网。
# 这里显式透传系统代理，并保证回环地址不走代理。
#
# 反过来也一样坑（2026-10-04 排查确认）：shell 里挂着一个已经死掉的代理
# （端口没人监听），Go 的 HTTP transport 默认遵循代理 env，于是所有平台请求
# 经由死代理全部失败 —— 症状与「完全断网」一模一样。所以透传前先探测代理
# 是否真的活着，死代理一律剥离并提示。
sys_proxy="${HTTPS_PROXY:-${https_proxy:-${HTTP_PROXY:-${http_proxy:-}}}}"
if [ -n "$sys_proxy" ]; then
  if curl -s -m 3 -o /dev/null -x "$sys_proxy" https://api.modrinth.com 2>/dev/null; then
    export HTTP_PROXY="${HTTP_PROXY:-${http_proxy:-$sys_proxy}}"
    export HTTPS_PROXY="${HTTPS_PROXY:-${https_proxy:-$sys_proxy}}"
    export http_proxy="${http_proxy:-$HTTP_PROXY}"
    export https_proxy="${https_proxy:-$HTTPS_PROXY}"
    export NO_PROXY="${NO_PROXY:-127.0.0.1,localhost}"
    export no_proxy="${no_proxy:-$NO_PROXY}"
    echo "[dev] proxy: $HTTPS_PROXY  (NO_PROXY=$NO_PROXY)"
  else
    unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy
    echo "[dev] WARNING: 检测到代理 $sys_proxy 但它没有响应，已剥离代理环境变量再启动后端" \
         "（直连平台；如果你确实需要走代理，请先把代理拉起来）"
  fi
fi

echo "[dev] starting backend  (go run, 127.0.0.1:18872, data $DATA)"
(cd "$ROOT/apps/server" && go run ./cmd/server -addr 127.0.0.1:18872 -data "$DATA" \
  > "$LOGDIR/server.log" 2>&1) &

# 无鉴权模式（用户 2026-10-03 定调）：本机单用户 IDE，前后端都只监听回环，
# 不再需要写令牌 —— 曾经那条「前端怎么拿到令牌」的链路本身就是 401 的根源。
echo "[dev] starting frontend (vite, 127.0.0.1:5271)"
# 唯一前端入口 = apps/web3（IDE 式单页，docs/active/design/web3-ide-shell-v3.md）。
# 不传 --host：交给 vite.config.ts 的 server.host（回环），命令行会覆盖它。
(cd "$ROOT/apps/web3" && npm run dev -- --port 5271 \
  > "$LOGDIR/web.log" 2>&1) &

for i in $(seq 1 45); do
  srv="$(curl -s -m 2 -o /dev/null -w '%{http_code}' --noproxy '*' http://127.0.0.1:18872/api/health 2>/dev/null)"
  if [ "$srv" = "200" ] && port_busy 5271; then
    echo "[dev] backend  ready  http://127.0.0.1:18872"
    echo "[dev] frontend ready  http://127.0.0.1:5271"
    # ── 外部访问（EasyTier mesh）────────────────────────────────────────
    # 人在外面时，mesh 内的设备（手机/其他机器）直接开 http://<虚拟IP>:5271。
    # 应用本体仍只监听 127.0.0.1：这里只把 mesh 虚拟 IP 上的 5271 转发到回环，
    # 不开 0.0.0.0（vite.config.ts 头注的安全约束不破）。EasyTier 不在线就跳过。
    et_ip=""
    if command -v easytier-cli >/dev/null 2>&1; then
      et_ip="$(easytier-cli node 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
    fi
    if [ -n "$et_ip" ]; then
      EXT_HOST="$et_ip" node "$ROOT/scripts/ext-proxy.mjs" > "$LOGDIR/ext-proxy.log" 2>&1 &
      echo "[dev] 外部访问: http://$et_ip:5271  （EasyTier mesh 内可用；日志 $LOGDIR/ext-proxy.log）"
    else
      echo "[dev] EasyTier 不在线，跳过外部访问代理（scripts/ext-proxy.mjs 可手动起）"
    fi
    echo "[dev] logs: $LOGDIR"
    echo "[dev] stop: scripts/dev-stop.sh"
    exit 0
  fi
  sleep 2
done
echo "[dev] WARNING: 90s 未就绪，查 $LOGDIR/server.log 与 web.log"
exit 1
