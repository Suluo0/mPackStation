#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if command -v pwsh >/dev/null 2>&1; then
  exec pwsh -NoProfile -File "$SCRIPT_DIR/test.ps1" "$@"
fi
if command -v powershell >/dev/null 2>&1; then
  exec powershell -NoProfile -File "$SCRIPT_DIR/test.ps1" "$@"
fi
echo "PowerShell 7 (pwsh) is required for this entrypoint." >&2
exit 1
