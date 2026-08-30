#!/usr/bin/env bash
# prism-install.sh - 自动安装 Prism Launcher 便携版到 .tools/prism(git-bash, Windows 专用)
# 不碰系统/注册表/UAC,删除 .tools/prism 即完成卸载。
# 代理策略:先探测再下载 —— 有 Clash 系进程就探测常用端口走代理,否则直连;绝不失败了才换路。
set -euo pipefail

ROOT_MSYS="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$ROOT_MSYS" && pwd -W)"            # Windows 原生路径,喂给 curl.exe/tar.exe
DEST_MSYS="$ROOT_MSYS/.tools/prism"
DEST="$ROOT\\.tools\\prism"
TMPD_MSYS="$ROOT_MSYS/.tmp/prism-install"
TMPD="$ROOT\\.tmp\\prism-install"
API="https://api.github.com/repos/PrismLauncher/PrismLauncher/releases/latest"

log() { echo "[prism-install] $*"; }
fail() { echo "[prism-install] ERROR: $*" >&2; exit 1; }

if [ -x "$DEST_MSYS/prismlauncher.exe" ]; then
  log "already installed: $DEST\\prismlauncher.exe"
  "$DEST_MSYS/prismlauncher.exe" --version || true
  exit 0
fi

# --- 代理检测:有代理进程 → 探测常用端口选能用的;没有 → 直连 ---
PROXY=""
if tasklist 2>/dev/null | grep -qiE 'clash|mihomo|verge|v2ray|sing-box|nekoray'; then
  log "proxy process detected, probing ports 7897/7890/10809/10808..."
  for port in 7897 7890 10809 10808; do
    if curl -fsS -m 5 -x "http://127.0.0.1:$port" "https://www.gstatic.com/generate_204" >/dev/null 2>&1; then
      PROXY="http://127.0.0.1:$port"
      log "proxy usable: $PROXY"
      break
    fi
  done
  [ -z "$PROXY" ] && log "proxy process found but no working port, falling back to direct"
else
  log "no proxy process, using direct connection"
fi

CURL_PROXY=()
[ -n "$PROXY" ] && CURL_PROXY=(-x "$PROXY")

log "resolving latest PrismLauncher release..."
mkdir -p "$TMPD_MSYS" "$DEST_MSYS"
JSON="$TMPD\\release.json"
JSON_MSYS="$TMPD_MSYS/release.json"
# api.github.com 通常可直连(部分代理节点反被 GitHub 403);失败再试代理;再不行用钉住的版本号兜底
PINNED_TAG="11.0.3"
rm -f "$JSON"
curl -fsSL -m 15 -o "$JSON" "$API" 2>/dev/null \
  || { [ -n "$PROXY" ] && curl -fsSL -m 30 "${CURL_PROXY[@]}" -o "$JSON" "$API" 2>/dev/null; } \
  || log "GitHub API unreachable, falling back to pinned version $PINNED_TAG"

if [ -s "$JSON" ]; then
  URL="$(grep -o '"browser_download_url": *"[^"]*Windows-MSVC[^"]*\.zip"' "$JSON_MSYS" | head -1 | sed 's/.*"\(http[^"]*\)"/\1/')"
  TAG="$(grep -o '"tag_name": *"[^"]*"' "$JSON_MSYS" | head -1 | sed 's/.*"\([^"]*\)"/\1/')"
else
  TAG="$PINNED_TAG"
  URL="https://github.com/PrismLauncher/PrismLauncher/releases/download/$TAG/PrismLauncher-Windows-MSVC-$TAG.zip"
fi
[ -n "$URL" ] || fail "no Windows-MSVC zip asset found in latest release"
log "latest release: $TAG"
log "asset: $URL"

ZIP="$TMPD\\prism.zip"
# 下载源优先级:代理(如检测到) > GitCode 国内镜像 > gh-proxy > 直连
# GitCode 是 CSDN 的 GitHub 镜像,与官方 release 同路径;镜像可能滞后于最新 tag,404 即降级
ASSET="PrismLauncher-Windows-MSVC-${TAG}.zip"
dl() { curl -fL -m 600 --retry 1 "$@" -o "$ZIP" 2>/dev/null && [ -s "$ZIP" ]; }
rm -f "$ZIP"
if [ -n "$PROXY" ]; then
  log "downloading via proxy $PROXY ..."
  dl "${CURL_PROXY[@]}" "$URL" || log "proxy download failed, trying mirrors"
fi
if [ ! -s "$ZIP" ] && [ -n "$TAG" ]; then
  log "trying GitCode mirror..."
  dl "https://gitcode.com/PrismLauncher/PrismLauncher/releases/download/$TAG/$ASSET" || log "gitcode miss (tag lag?), next"
fi
if [ ! -s "$ZIP" ]; then
  log "trying gh-proxy..."
  dl "https://gh-proxy.com/$URL" || log "gh-proxy failed, next"
fi
if [ ! -s "$ZIP" ]; then
  log "trying direct GitHub..."
  dl "$URL" || fail "all download sources failed"
fi
[ -s "$ZIP" ] || fail "downloaded file is empty"

log "extracting to $DEST ..."
rm -rf "$DEST_MSYS"
mkdir -p "$DEST_MSYS"
# git-bash 的 /usr/bin/tar 是 GNU tar,不认 zip;zip 必须用系统 bsdtar
BSdtar="${SYSTEMROOT:-C:\\Windows}\\System32\\tar.exe"
"$BSdtar" -xf "$ZIP" -C "$TMPD" || fail "extract failed"
SRC="$(find "$TMPD_MSYS" -name prismlauncher.exe 2>/dev/null | head -1)"
[ -n "$SRC" ] || fail "prismlauncher.exe not found after extract"
cp -r "$(dirname "$SRC")/." "$DEST_MSYS/"

rm -rf "$TMPD_MSYS"
[ -x "$DEST_MSYS/prismlauncher.exe" ] || fail "install incomplete"
log "installed: $DEST\\prismlauncher.exe"
"$DEST_MSYS/prismlauncher.exe" --version || true
log "done. launch CLI example: prismlauncher.exe -d <实例根目录> -I pack.mrpack"
