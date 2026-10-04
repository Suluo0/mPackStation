#!/usr/bin/env bash
# 停唯一标准服务（18872 / 5271）+ 外部访问代理（ext-proxy 也听 5271，只是绑在
# EasyTier 虚拟 IP 上，同端口一并收掉）。
lsof -t -nP -iTCP:18872 -sTCP:LISTEN 2>/dev/null | xargs kill 2>/dev/null
lsof -t -nP -iTCP:5271  -sTCP:LISTEN 2>/dev/null | xargs kill 2>/dev/null
echo "[dev-stop] done"
