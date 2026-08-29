#!/usr/bin/env bash
# mPackStation dev environment: one-click stop (frees 18871 + 5273)
set -u

killed=0
for port in 18871 5273; do
  pids=$(netstat -ano | grep -E ":$port\s" | grep LISTENING | awk '{print $NF}' | sort -u)
  for pid in $pids; do
    echo "[stop] port $port held by pid=$pid, killing tree"
    MSYS_NO_PATHCONV=1 taskkill /PID "$pid" /T /F >/dev/null 2>&1
    killed=1
  done
done

# parents (go run / npm) exit on their own once the child holding the port dies
sleep 2

still=""
for port in 18871 5273; do
  netstat -ano | grep -E ":$port\s" | grep -q LISTENING && still="$still $port"
done
if [ -n "$still" ]; then
  echo "[stop] WARNING: ports still listening:$still"
  exit 1
fi
if [ "$killed" = 1 ]; then
  echo "[stop] dev environment stopped, ports 18871/5273 free"
else
  echo "[stop] nothing was running"
fi
exit 0
