@echo off
rem mPackStation dev environment: one-click stop (frees 18871 + 5273)
setlocal EnableDelayedExpansion

set "KILLED=0"
for %%p in (18871 5273) do (
    for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":%%p " ^| findstr LISTENING') do (
        echo [stop] port %%p held by pid=%%a, killing tree
        taskkill /PID %%a /T /F >nul 2>&1
        set "KILLED=1"
    )
)

rem parents (go run / npm) exit on their own once the child holding the port dies
"%SystemRoot%\System32\timeout.exe" /t 2 /nobreak >nul

set "STILL="
for %%p in (18871 5273) do (
    netstat -ano | findstr ":%%p " | findstr LISTENING >nul
    if not errorlevel 1 set "STILL=!STILL! %%p"
)
if defined STILL (
    echo [stop] WARNING: ports still listening:%STILL%
    exit /b 1
)
if "%KILLED%"=="1" (echo [stop] dev environment stopped, ports 18871/5273 free) else (echo [stop] nothing was running)
exit /b 0
