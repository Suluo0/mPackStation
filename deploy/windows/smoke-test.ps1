[CmdletBinding()]
param(
    [string]$PackageDir = 'dist/package',
    [int]$Port = 18879,
    [int]$TimeoutSeconds = 30
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$packagePath = Join-Path $repoRoot $PackageDir
$binary = Join-Path $packagePath 'mpackstation.exe'
if (-not (Test-Path -LiteralPath $binary)) {
    throw "Smoke package is missing: $binary. Run scripts/package.ps1 first."
}

$existing = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($existing) { throw "Smoke port $Port is already in use; no existing process was stopped." }

$id = [Guid]::NewGuid().ToString('N')
$smokeRoot = Join-Path ([IO.Path]::GetTempPath()) "mpackstation-smoke-$id"
$dataPath = Join-Path $smokeRoot 'data'
$logPath = Join-Path $smokeRoot 'server.log'
$errorLogPath = Join-Path $smokeRoot 'server.error.log'
New-Item -ItemType Directory -Force -Path $dataPath | Out-Null
$process = $null
try {
    $process = Start-Process -FilePath $binary -ArgumentList @('-addr', "127.0.0.1:$Port", '-data', $dataPath) -WorkingDirectory $packagePath -RedirectStandardOutput $logPath -RedirectStandardError $errorLogPath -WindowStyle Hidden -PassThru
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $ready = $false
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($process.HasExited) { throw "Server exited before readiness. See $logPath" }
        try {
            $health = Invoke-RestMethod "http://127.0.0.1:$Port/api/healthz" -TimeoutSec 2
            $readyState = Invoke-RestMethod "http://127.0.0.1:$Port/api/readyz" -TimeoutSec 2
            if ($health.status -eq 'ok' -and $readyState.status -eq 'ready' -and $readyState.db -eq $true) { $ready = $true; break }
        } catch { }
        Start-Sleep -Milliseconds 250
    }
    if (-not $ready) { throw "Server did not become ready within $TimeoutSeconds seconds. See $logPath" }
    Write-Host "PASS DEP-001..DEP-003: packaged server started and probes are ready."
} finally {
    if ($process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit(5000) | Out-Null
    }
    if (Test-Path -LiteralPath $smokeRoot) { Remove-Item -LiteralPath $smokeRoot -Recurse -Force }
}
Write-Host 'PASS DEP-004..DEP-005: own process stopped and temporary data was cleaned.'
