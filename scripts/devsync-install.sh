#!/usr/bin/env bash
# 安装 / 更新 Mac 侧 devsync 的 launchd 定时任务。
#
# 为什么是「运行时生成 plist」而不是「仓库里放一份 plist」：
#   launchd 的 plist 必须写绝对路径，而绝对路径属于机器级信息 ——
#   按项目约定（见 AGENTS.md 的信息分层规则）机器相关路径不入仓、不入提交历史。
#   所以仓库里只放这个生成器；plist 在安装时按当前仓库实际位置生成。
#
# 为什么还要在本地盘放一个入口脚本（wrapper）：
#   macOS 26 实测，launchd 在 spawn 阶段打不开 /Volumes/Evo 上的文件 ——
#   把 StandardOutPath/StandardErrorPath 指向 Evo 时，job 每 30s 触发一次、
#   每次都 `last exit code = 78: EX_CONFIG`，而且连那个日志文件都不会被创建
#   （`runs` 一直涨，说明调度没问题，是 spawn 本身失败）。
#   对照组（本机两个正常工作的第三方 agent）恰好都把日志放在本地盘：
#     com.xunsu.dufs-nas  → ~/Library/Logs/dufs-nas.log        （Evo 只作运行参数）
#     io.lifeos.collector → ~/.lifeos/collector.log            （全本地）
#   所以 launchd 的入口和它要打开的日志一律落本地盘；
#   Evo 上的 devsync.sh 由 wrapper 在**运行时**调用（运行时访问外置卷是允许的，
#   dufs 就是这样读 /Volumes/Evo/nas 的）。
#
# 用法：
#   bash scripts/devsync-install.sh      # 安装或更新
#   bash scripts/devsync-install.sh -u   # 卸载
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LABEL="com.mpack.devsync"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
POLLER="$SCRIPT_DIR/devsync.sh"
LOG_DIR="$REPO_ROOT/.tmp/devsync"          # devsync.sh 自己写的运行日志（仓库内，没问题）
SUPPORT_DIR="$HOME/Library/Application Support/mPackStation"
WRAPPER="$SUPPORT_DIR/devsync-launchd.sh"  # launchd 入口：必须在本地盘
OUT_LOG="$HOME/Library/Logs/mpack-devsync.out.log"
ERR_LOG="$HOME/Library/Logs/mpack-devsync.err.log"

if [ "${1:-}" = "-u" ] || [ "${1:-}" = "--uninstall" ]; then
    launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
    rm -f "$PLIST" "$WRAPPER"
    echo "已卸载 $LABEL（并删除 $PLIST、$WRAPPER）"
    exit 0
fi

[ -f "$POLLER" ] || { echo "找不到 $POLLER" >&2; exit 1; }

mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs" "$SUPPORT_DIR" "$LOG_DIR"
chmod +x "$POLLER"

# 本地盘入口：launchd 只碰这个文件，再由它去调仓库里的真身。
cat > "$WRAPPER" <<WRAPPER_EOF
#!/bin/bash
# 由 scripts/devsync-install.sh 生成，勿手工编辑。
# 存在意义见该脚本头部注释：macOS 26 的 launchd 打不开 /Volumes/Evo 上的文件，
# 所以入口本身（以及它的 stdout/stderr）必须落在本地盘。
exec /bin/bash "$POLLER"
WRAPPER_EOF
chmod +x "$WRAPPER"

cat > "$PLIST" <<PLIST_EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>$WRAPPER</string>
  </array>
  <key>StartInterval</key><integer>30</integer>
  <key>RunAtLoad</key><true/>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>$OUT_LOG</string>
  <key>StandardErrorPath</key><string>$ERR_LOG</string>
</dict>
</plist>
PLIST_EOF

# 先卸旧的再装，避免重复加载报 "already bootstrapped"。
launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
if ! launchctl bootstrap "gui/$UID" "$PLIST"; then
    cat >&2 <<'MSG'

bootstrap 失败。若报 "5: Input/output error"，说明当前会话无权向 launchd 注册
（受控 / agent 会话常见，与 plist 内容无关）。请在**自己的 Terminal 窗口**里重跑本脚本；
或注销后重新登录 —— ~/Library/LaunchAgents/ 下的 plist 会在登录时被自动加载。
MSG
    exit 1
fi
launchctl enable "gui/$UID/$LABEL"

echo "已安装 $LABEL"
echo "  plist    : $PLIST"
echo "  入口     : $WRAPPER   (本地盘)"
echo "  脚本     : $POLLER"
echo "  运行日志 : $LOG_DIR/devsync.log   (仓库内，devsync.sh 自己写)"
echo "  launchd  : $OUT_LOG / $ERR_LOG   (本地盘，launchd 重定向)"
launchctl print "gui/$UID/$LABEL" 2>/dev/null |
    grep -iE "^\s*(state|program =|run interval|runs|last exit)" || true
