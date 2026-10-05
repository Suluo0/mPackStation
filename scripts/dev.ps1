[CmdletBinding()]
param(
    [int]$WebPort = 5271,
    [int]$ServerPort = 18872,
    [string]$DataDir,
    [string]$Proxy
)

# mPackStation dev（Windows）：一键启动唯一标准服务（后端 18872 + 前端 5271，AGENTS.md 定稿）。
# 与 scripts/dev.sh 对齐：同一套端口、同一条数据目录口径、同样的代理存活检测。
. (Join-Path $PSScriptRoot 'common.ps1')
$go = Get-GoCommand
$npm = Get-NpmCommand

if (-not $DataDir) { $DataDir = Get-DevDataDir }
if (-not (Test-Path -LiteralPath $DataDir)) {
    New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
}
$dataPath = (Resolve-Path -LiteralPath $DataDir).Path

Assert-PortFree $WebPort
Assert-PortFree $ServerPort
$logDir = Join-Path $script:RepoRoot '.tmp/dev'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

# ── 代理透传（带存活检测）──────────────────────────────────────────────
# 后端要访问 Modrinth / CurseForge 才能搜索、解析依赖。两头的坑都踩过：
#   1) 不透传 —— 后端环境里没有 proxy 变量，平台全不可达，看着像「代码改坏了」；
#   2) 透传了一个已经死掉的代理 —— Go 的 transport 会一直走它，症状与断网一致。
# 所以先探测代理是否真的活着，死的直接剥离。
$sysProxy = $Proxy
if (-not $sysProxy) { $sysProxy = $env:HTTPS_PROXY }
if (-not $sysProxy) { $sysProxy = $env:HTTP_PROXY }
if ($sysProxy) {
    $alive = $false
    try {
        $null = Invoke-WebRequest -Uri 'https://api.modrinth.com' -Proxy $sysProxy -TimeoutSec 5 -UseBasicParsing
        $alive = $true
    } catch {
        # 4xx/5xx 也算代理活着（说明链路通）
        if ($_.Exception.Response) { $alive = $true }
    }
    if ($alive) {
        $env:HTTP_PROXY = $sysProxy
        $env:HTTPS_PROXY = $sysProxy
        if (-not $env:NO_PROXY) { $env:NO_PROXY = '127.0.0.1,localhost' }
        Write-Host "[dev] proxy: $sysProxy  (NO_PROXY=$($env:NO_PROXY))"
    } else {
        Remove-Item Env:HTTP_PROXY, Env:HTTPS_PROXY -ErrorAction SilentlyContinue
        Write-Host "[dev] WARNING: 检测到代理 $sysProxy 但它没有响应，已剥离代理环境变量再启动后端" -ForegroundColor Yellow
    }
}

$serverLog = Join-Path $logDir 'server.log'
$serverError = Join-Path $logDir 'server.error.log'
$webLog = Join-Path $logDir 'web.log'
$webError = Join-Path $logDir 'web.error.log'

# 启动器内核：仓库内 .tools/launcher/mpack-launcher.exe。找到就显式传给后端
# （MPACK_LAUNCHER_BIN 优先级最高）；找不到只影响启动器任务，其余能力照常。
$launcherExe = Get-LauncherExePath
if (Test-Path -LiteralPath $launcherExe) {
    $env:MPACK_LAUNCHER_BIN = $launcherExe
    Write-Host "[dev] launcher: $launcherExe"
} else {
    Write-Host "[dev] WARNING: 未找到启动器内核 $launcherExe —— 安装/启动任务会返回 503。" -ForegroundColor Yellow
    Write-Host "[dev]          编译它: pwsh scripts/build-launcher.ps1" -ForegroundColor Yellow
}

Write-Host "[dev] starting backend  (go run, 127.0.0.1:$ServerPort, data $dataPath)"
$serverArgs = @('run', './cmd/server', '-addr', "127.0.0.1:$ServerPort", '-data', $dataPath)

# 无鉴权模式（2026-10-03 定调）：本机单用户 IDE，前后端都只监听回环。
# 不传 --host：交给 vite.config.ts 的 server.host，命令行会覆盖它。
Write-Host "[dev] starting frontend (vite, 127.0.0.1:$WebPort)"
$webArgs = @('run', 'dev', '--', '--port', "$WebPort")

$server = Start-Process -FilePath $go -ArgumentList $serverArgs -WorkingDirectory $script:ServerDir -RedirectStandardOutput $serverLog -RedirectStandardError $serverError -WindowStyle Hidden -PassThru
$web = Start-Process -FilePath $npm -ArgumentList $webArgs -WorkingDirectory $script:WebDir -RedirectStandardOutput $webLog -RedirectStandardError $webError -WindowStyle Hidden -PassThru

Set-Content -LiteralPath (Join-Path $logDir 'server.pid') -Value $server.Id -NoNewline
Set-Content -LiteralPath (Join-Path $logDir 'web.pid') -Value $web.Id -NoNewline

# Wait for real readiness instead of declaring success at spawn time.
$serverReady = $false
$webReady = $false
$deadline = (Get-Date).AddSeconds(90)
while ((Get-Date) -lt $deadline -and -not ($serverReady -and $webReady)) {
    if (-not $serverReady) {
        try {
            $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$ServerPort/api/health" -TimeoutSec 2 -UseBasicParsing
            if ($resp.StatusCode -eq 200) { $serverReady = $true }
        } catch { }
    }
    if (-not $webReady) {
        $webReady = [bool](Get-NetTCPConnection -LocalPort $WebPort -State Listen -ErrorAction SilentlyContinue)
    }
    if (-not ($serverReady -and $webReady)) { Start-Sleep -Seconds 2 }
}

Write-Host "Started mPackStation development processes."
Write-Host "  server pid=$($server.Id)  http://127.0.0.1:$ServerPort  ready=$serverReady"
Write-Host "  web    pid=$($web.Id)  http://127.0.0.1:$WebPort  ready=$webReady"
Write-Host "Logs: $logDir"
Write-Host "Stop with: scripts/dev-stop.ps1"
if (-not ($serverReady -and $webReady)) {
    Write-Host "WARNING: not all processes became ready within 90s; check logs above." -ForegroundColor Yellow
    exit 1
}
