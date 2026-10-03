#!/usr/bin/env bash
# 停唯一标准服务（18872 / 5271）。
lsof -t -nP -iTCP:18872 -sTCP:LISTEN 2>/dev/null | xargs kill 2>/dev/null
lsof -t -nP -iTCP:5271  -sTCP:LISTEN 2>/dev/null | xargs kill 2>/dev/null
echo "[dev-stop] done"
