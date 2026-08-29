[CmdletBinding()]
param(
    [int]$WebPort = 5273,
    [int]$ServerPort = 18871,
    [string]$DataDir = 'data'
)

. (Join-Path $PSScriptRoot 'common.ps1')
$go = Get-GoCommand
$npm = Get-NpmCommand
$dataPath = (Resolve-Path (Join-Path $script:RepoRoot $DataDir) -ErrorAction SilentlyContinue)
if (-not $dataPath) {
    $dataPath = (New-Item -ItemType Directory -Force -Path (Join-Path $script:RepoRoot $DataDir))
}
$dataPath = $dataPath.Path

Assert-PortFree $WebPort
Assert-PortFree $ServerPort
$logDir = Join-Path $script:RepoRoot '.tmp/dev'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

$serverLog = Join-Path $logDir 'server.log'
$serverError = Join-Path $logDir 'server.error.log'
$webLog = Join-Path $logDir 'web.log'
$webError = Join-Path $logDir 'web.error.log'
$serverArgs = @('run', './cmd/server', '-addr', "127.0.0.1:$ServerPort", '-data', $dataPath)
$webArgs = @('run', 'dev', '--', '--host', '127.0.0.1', '--port', "$WebPort")

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

