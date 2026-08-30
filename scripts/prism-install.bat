@echo off
rem prism-install.bat - Auto-install Prism Launcher portable into .tools\prism (cmd / double-click, Windows only)
rem No system/registry/UAC changes; delete .tools\prism to uninstall.
rem Proxy policy: if a Clash-family process is running, probe common ports and use it; otherwise direct.
setlocal EnableDelayedExpansion

pushd "%~dp0.." && set "ROOT=!CD!" && popd
set "DEST=%ROOT%\.tools\prism"
set "TMPD=%ROOT%\.tmp\prism-install"
set "API=https://api.github.com/repos/PrismLauncher/PrismLauncher/releases/latest"
set "TAREXE=%SystemRoot%\System32\tar.exe"

if exist "%DEST%\prismlauncher.exe" (
  echo [prism-install] already installed: %DEST%\prismlauncher.exe
  "%DEST%\prismlauncher.exe" --version
  exit /b 0
)

rem --- proxy detection ---
set "PROXY="
tasklist 2>nul | findstr /i "clash mihomo verge v2ray sing-box nekoray" >nul
if not errorlevel 1 (
  echo [prism-install] proxy process detected, probing ports...
  for %%P in (7897 7890 10809 10808) do (
    if not defined PROXY (
      curl.exe -fsS -m 5 -x http://127.0.0.1:%%P https://www.gstatic.com/generate_204 >nul 2>nul
      if not errorlevel 1 set "PROXY=http://127.0.0.1:%%P"
    )
  )
  if defined PROXY (echo [prism-install] proxy usable: !PROXY!) else (echo [prism-install] proxy process found but no working port, using direct)
) else (
  echo [prism-install] no proxy process, using direct connection
)

echo [prism-install] resolving latest PrismLauncher release...
if not exist "%TMPD%" mkdir "%TMPD%"
if defined PROXY (set "PX=-x !PROXY!") else (set "PX=")
set "PINNED_TAG=11.0.3"
if exist "%TMPD%\release.json" del /q "%TMPD%\release.json"
rem api.github.com is usually reachable directly; fallback to proxy; then pinned version
curl.exe -fsSL -m 15 -o "%TMPD%\release.json" "%API%" 2>nul
if errorlevel 1 if defined PROXY curl.exe -fsSL -m 30 !PX! -o "%TMPD%\release.json" "%API%" 2>nul

set "URL="
set "TAG="
if exist "%TMPD%\release.json" (
  for /f "usebackq delims=" %%L in (`findstr /c:"browser_download_url" "%TMPD%\release.json"`) do (
    echo %%L | findstr /c:"Windows-MSVC" | findstr /c:".zip" >nul && if not defined URL (
      set "LINE=%%L"
      set "LINE=!LINE:*"browser_download_url": "=!"
      for /f "tokens=1 delims=," %%U in ("!LINE!") do set "URL=%%U"
    )
  )
  for /f "usebackq delims=" %%L in (`findstr /c:"tag_name" "%TMPD%\release.json"`) do if not defined TAG (
    set "LINE=%%L"
    set "LINE=!LINE:*"tag_name": "=!"
    for /f "tokens=1 delims=," %%U in ("!LINE!") do set "TAG=%%U"
  )
  set "URL=!URL:"=!"
  set "TAG=!TAG:"=!"
)
if not defined URL (
  echo [prism-install] GitHub API unreachable, falling back to pinned version %PINNED_TAG%
  set "TAG=%PINNED_TAG%"
  set "URL=https://github.com/PrismLauncher/PrismLauncher/releases/download/%PINNED_TAG%/PrismLauncher-Windows-MSVC-%PINNED_TAG%.zip"
)
echo [prism-install] release: !TAG!
echo [prism-install] asset: !URL!

rem download priority: proxy (if detected) ^> GitCode CN mirror ^> gh-proxy ^> direct
set "ZIP=%TMPD%\prism.zip"
if exist "!ZIP!" del /q "!ZIP!"
set "DONE="
if defined PROXY (
  echo [prism-install] downloading via proxy !PROXY! ...
  curl.exe -fL -m 600 --retry 1 !PX! -o "!ZIP!" "!URL!" 2>nul && set "DONE=1"
)
if not defined DONE (
  echo [prism-install] trying GitCode mirror...
  curl.exe -fL -m 600 --retry 1 -o "!ZIP!" "https://gitcode.com/PrismLauncher/PrismLauncher/releases/download/!TAG!/PrismLauncher-Windows-MSVC-!TAG!.zip" 2>nul && set "DONE=1"
)
if not defined DONE (
  echo [prism-install] trying gh-proxy...
  curl.exe -fL -m 600 --retry 1 -o "!ZIP!" "https://gh-proxy.com/!URL!" 2>nul && set "DONE=1"
)
if not defined DONE (
  echo [prism-install] trying direct GitHub...
  curl.exe -fL -m 600 --retry 1 -o "!ZIP!" "!URL!" 2>nul && set "DONE=1"
)
if not defined DONE (echo [prism-install] ERROR: all download sources failed & exit /b 1)

echo [prism-install] extracting to %DEST% ...
if exist "%DEST%" rmdir /s /q "%DEST%"
mkdir "%DEST%"
"%TAREXE%" -xf "!ZIP!" -C "%TMPD%" || (echo [prism-install] ERROR: extract failed & exit /b 1)

set "SRC="
for /d %%D in ("%TMPD%\*") do if exist "%%D\prismlauncher.exe" set "SRC=%%D"
if not defined SRC if exist "%TMPD%\prismlauncher.exe" set "SRC=%TMPD%"
if not defined SRC (echo [prism-install] ERROR: prismlauncher.exe not found after extract & exit /b 1)
xcopy /e /i /q /y "!SRC!" "%DEST%" >nul

rmdir /s /q "%TMPD%"
if not exist "%DEST%\prismlauncher.exe" (echo [prism-install] ERROR: install incomplete & exit /b 1)
echo [prism-install] installed: %DEST%\prismlauncher.exe
"%DEST%\prismlauncher.exe" --version
echo [prism-install] done. launch CLI example: prismlauncher.exe -d DATA_DIR -I pack.mrpack
endlocal
