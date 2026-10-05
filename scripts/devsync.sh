#!/usr/bin/env bash
# Mac 侧反向守护：把任意推送方（PC / 其他机器）的提交自动拉到 Mac。
#
# 与 scripts/autosync.ps1 对称 —— 那个解决「Mac 推完，PC 自动跟上」，
# 这个解决「PC 推完，Mac 自动跟上」。两条合起来，两端才真正对等。
#
# 设计原则与 autosync 一致，三条都是「宁可不动手，也不破坏」：
#   1. 只用 --ff-only，永不产生 merge commit
#   2. 工作区脏就跳过 —— 绝不覆盖正在编辑的内容
#   3. 本地有未推送提交（已偏离远端）就跳过，交给人处理
#
# 这里不做任何重建/重启：Mac 是开发端，vite/go 由使用者自己起。
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BRANCH="${DEVSYNC_BRANCH:-DEV_2610-WK1}"
LOG_DIR="$REPO_ROOT/.tmp/devsync"
LOG="$LOG_DIR/devsync.log"

mkdir -p "$LOG_DIR"

log() { printf '%s [%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" "$2" >> "$LOG"; }
short() { git -C "$REPO_ROOT" rev-parse --short "${1:-HEAD}" 2>/dev/null || echo '?'; }

# 1) 取远端。失败不报错退出（网络抖动是常态），只记一条。
if ! git -C "$REPO_ROOT" fetch origin "$BRANCH" --quiet 2>/dev/null; then
    log WARN 'git fetch 失败（网络或代理？），本轮跳过。'
    exit 0
fi

LOCAL="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null)" || exit 0
REMOTE="$(git -C "$REPO_ROOT" rev-parse "origin/$BRANCH" 2>/dev/null)" || exit 0

# 2) 无新提交 —— 最常见的路径，直接走。
[ "$LOCAL" = "$REMOTE" ] && exit 0

# 3) 工作区脏：这是 Mac 侧的常态（正在写代码），跳过是保护而非失败。
if [ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]; then
    log WARN "远端有新提交 $(short "$REMOTE")，但工作区有未提交改动 —— 跳过，不覆盖你的编辑。"
    exit 0
fi

# 4) 本地是否领先远端？领先就不能 ff —— 说明 Mac 有没推的提交，别自作主张。
COUNTS="$(git -C "$REPO_ROOT" rev-list --left-right --count "origin/$BRANCH...HEAD" 2>/dev/null)"
BEHIND="${COUNTS%%[[:space:]]*}"
AHEAD="${COUNTS##*[[:space:]]}"
if [ "${AHEAD:-0}" != "0" ]; then
    log WARN "本地有 ${AHEAD} 个未推送提交、远端领先 ${BEHIND} 个 —— 已偏离，跳过（需人工 rebase/merge）。"
    exit 0
fi

# 5) 干净且只落后 —— 这才是该动手的情况。
if git -C "$REPO_ROOT" merge --ff-only "origin/$BRANCH" --quiet 2>/dev/null; then
    log INFO "已拉取 $(short "$LOCAL") → $(short "$REMOTE")（${BEHIND} 个提交）"
else
    log ERROR 'fast-forward 失败，需要人工介入。'
fi

exit 0
