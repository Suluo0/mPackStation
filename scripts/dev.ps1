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

Write-Host "Started mPackStation development processes."
Write-Host "  server pid=$($server.Id)  http://127.0.0.1:$ServerPort"
Write-Host "  web    pid=$($web.Id)  http://127.0.0.1:$WebPort"
Write-Host "Logs: $logDir"
Write-Host "This script does not stop existing or newly started processes; stop these PIDs explicitly when finished."

