#!/usr/bin/env bash
# 安装 / 更新 Mac 侧 devsync 的 launchd 定时任务。
#
# 为什么是「运行时生成 plist」而不是「仓库里放一份 plist」：
#   launchd 的 plist 必须写绝对路径，而绝对路径属于机器级信息 ——
#   按项目约定（见 AGENTS.md 的信息分层规则）机器相关路径不入仓、不入提交历史。
#   所以仓库里只放这个生成器；plist 在安装时按当前仓库实际位置生成。
#
# 用法：
#   bash scripts/devsync-install.sh      # 安装或更新
#   bash scripts/devsync-install.sh -u   # 卸载
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LABEL="com.mpack.devsync"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG_DIR="$REPO_ROOT/.tmp/devsync"
POLLER="$SCRIPT_DIR/devsync.sh"

if [ "${1:-}" = "-u" ] || [ "${1:-}" = "--uninstall" ]; then
    launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
    rm -f "$PLIST"
    echo "已卸载 $LABEL（并删除 $PLIST）"
    exit 0
fi

[ -f "$POLLER" ] || { echo "找不到 $POLLER" >&2; exit 1; }

mkdir -p "$HOME/Library/LaunchAgents" "$LOG_DIR"
chmod +x "$POLLER"

cat > "$PLIST" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>$POLLER</string>
  </array>
  <key>StartInterval</key><integer>30</integer>
  <key>RunAtLoad</key><true/>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>$LOG_DIR/launchd.out.log</string>
  <key>StandardErrorPath</key><string>$LOG_DIR/launchd.err.log</string>
</dict>
</plist>
PLIST_EOF

# 先卸旧的再装，避免重复加载报 "already bootstrapped"。
launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$UID" "$PLIST"
launchctl enable "gui/$UID/$LABEL"

echo "已安装 $LABEL"
echo "  plist : $PLIST"
echo "  脚本  : $POLLER"
echo "  日志  : $LOG"
launchctl print "gui/$UID/$LABEL" 2>/dev/null | grep -iE "^\s*(state|program|run interval)" || true
