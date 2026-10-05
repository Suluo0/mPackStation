@echo off
rem mPackStation dev environment: one-click start (backend 18872 + frontend 5271)
rem Aligned with scripts/dev.sh / AGENTS.md: web3 is the only frontend entry.
setlocal EnableDelayedExpansion

set "ROOT=%~dp0.."
pushd "%ROOT%"
set "ROOT=%CD%"
popd

set "GO=%ROOT%\.tools\go\bin\go.exe"
if not exist "%GO%" set "GO=go"
set "LOGDIR=%ROOT%\.tmp\dev"
if not exist "%LOGDIR%" mkdir "%LOGDIR%"

set "DATA=%TEMP%\mpack-data"
if not exist "%DATA%" mkdir "%DATA%"

netstat -ano | findstr ":18872 " | findstr LISTENING >nul
if not errorlevel 1 (echo [dev] port 18872 already in use, stop it first: scripts\dev-stop.bat & exit /b 1)
netstat -ano | findstr ":5271 " | findstr LISTENING >nul
if not errorlevel 1 (echo [dev] port 5271 already in use, stop it first: scripts\dev-stop.bat & exit /b 1)

echo [dev] starting backend  (go run, 127.0.0.1:18872, data %DATA%)
start "mpack-server" /min cmd /c "cd /d "%ROOT%\apps\server" && "%GO%" run ./cmd/server -addr 127.0.0.1:18872 -data "%DATA%" > "%LOGDIR%\server.log" 2>&1"

echo [dev] starting frontend (vite, 127.0.0.1:5271)
start "mpack-web" /min cmd /c "cd /d "%ROOT%\apps\web3" && npm run dev -- --port 5271 > "%LOGDIR%\web.log" 2>&1"

rem wait for real readiness (max 90s)
set /a tries=0
:waitloop
set /a tries+=1
if %tries% gtr 45 goto timeout
set "SRV="
for /f %%s in ('curl -s -m 2 -o nul -w "%%{http_code}" http://127.0.0.1:18872/api/health 2^>nul') do set "SRV=%%s"
netstat -ano | findstr ":5271 " | findstr LISTENING >nul
set "WEB=0"
if not errorlevel 1 set "WEB=1"
if "%SRV%"=="200" if "%WEB%"=="1" goto ready
"%SystemRoot%\System32\timeout.exe" /t 2 /nobreak >nul
goto waitloop

:timeout
echo [dev] WARNING: not ready within 90s, check %LOGDIR%\server.log and web.log
exit /b 1

:ready
echo [dev] backend  ready  http://127.0.0.1:18872
echo [dev] frontend ready  http://127.0.0.1:5271
echo [dev] logs: %LOGDIR%
echo [dev] stop: scripts\dev-stop.bat
exit /b 0
