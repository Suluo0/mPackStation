[CmdletBinding()]
param(
    # Also kill whatever is listening on the dev ports, even without PID files.
    [switch]$ByPort
)

. (Join-Path $PSScriptRoot 'common.ps1')

$logDir = Join-Path $script:RepoRoot '.tmp/dev'
$stopped = @()

function Stop-RecordedProcess {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$PidFile
    )
    if (-not (Test-Path -LiteralPath $PidFile)) {
        Write-Host "$Name : no PID file, skipped"
        return
    }
    $pidValue = [int](Get-Content -LiteralPath $PidFile -Raw).Trim()
    $proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
    if (-not $proc) {
        Write-Host "$Name : pid=$pidValue already exited"
    } else {
        # Kill the whole tree: `go run` and `npm.cmd` spawn child processes
        # that actually hold the ports.
        & taskkill /PID $pidValue /T /F | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "$Name : pid=$pidValue stopped (tree)"
        } else {
            Write-Host "$Name : pid=$pidValue failed to stop (taskkill exit $LASTEXITCODE)" -ForegroundColor Yellow
        }
    }
    Remove-Item -LiteralPath $PidFile -Force -ErrorAction SilentlyContinue
}

Stop-RecordedProcess -Name 'server' -PidFile (Join-Path $logDir 'server.pid')
Stop-RecordedProcess -Name 'web'    -PidFile (Join-Path $logDir 'web.pid')

if ($ByPort) {
    foreach ($port in @(5273, 18871)) {
        $owners = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty OwningProcess -Unique
        foreach ($ownerPid in $owners) {
            $p = Get-Process -Id $ownerPid -ErrorAction SilentlyContinue
            Write-Host "port $port held by pid=$ownerPid ($($p.ProcessName)); killing tree"
            & taskkill /PID $ownerPid /T /F | Out-Null
        }
    }
}

$stillUp = @()
foreach ($port in @(5273, 18871)) {
    if (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue) {
        $stillUp += $port
    }
}
if ($stillUp) {
    Write-Host "WARNING: ports still listening: $($stillUp -join ', '). Re-run with -ByPort to force." -ForegroundColor Yellow
    exit 1
}
Write-Host "mPackStation dev environment stopped. Ports 5273/18871 are free."
