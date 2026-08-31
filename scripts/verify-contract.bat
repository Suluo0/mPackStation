@echo off
REM mPackStation 契约验收入口(调用 git-bash 执行 scripts\verify-contract.sh)
where bash >nul 2>nul
if errorlevel 1 (
  echo [X] not found git-bash in PATH
  exit /b 1
)
bash "%~dp0verify-contract.sh" %*
