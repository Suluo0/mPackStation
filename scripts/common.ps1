# Shared, side-effect-light helpers for repository PowerShell entrypoints.
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$script:ServerDir = Join-Path $script:RepoRoot 'apps/server'
$script:WebDir = Join-Path $script:RepoRoot 'apps/web'
$script:DistRoot = Join-Path $script:RepoRoot 'dist'

function Require-Command {
    param([Parameter(Mandatory)][string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command '$Name' was not found. Install it or add it to PATH."
    }
}

function Get-GoCommand {
    $bundled = Join-Path $script:RepoRoot '.tools/go/bin/go.exe'
    if (Test-Path -LiteralPath $bundled) { return $bundled }
    Require-Command 'go'
    return (Get-Command 'go').Source
}

function Get-GoFmtCommand {
    $bundled = Join-Path $script:RepoRoot '.tools/go/bin/gofmt.exe'
    if (Test-Path -LiteralPath $bundled) { return $bundled }
    Require-Command 'gofmt'
    return (Get-Command 'gofmt').Source
}

function Get-NpmCommand {
    if (Get-Command 'npm.cmd' -ErrorAction SilentlyContinue) {
        return (Get-Command 'npm.cmd').Source
    }
    Require-Command 'npm'
    return (Get-Command 'npm').Source
}

function Get-RepoVersion {
    param([string]$Requested)
    if ($Requested) { return $Requested }
    if ($env:MPACK_VERSION) { return $env:MPACK_VERSION }
    return '0.1.0-dev'
}

function Assert-VersionSafe {
    param([Parameter(Mandatory)][string]$Value)
    if ($Value -notmatch '^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$') {
        throw "Version must contain only letters, digits, '.', '_' or '-' and be at most 64 characters."
    }
}

function Get-GitCommit {
    try {
        $commit = (& git -C $script:RepoRoot rev-parse HEAD 2>$null).Trim()
        if ($commit) { return $commit }
    } catch { }
    return 'unknown'
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter(Mandatory)][string[]]$ArgumentList,
        [Parameter(Mandatory)][string]$WorkingDirectory
    )
    & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $FilePath $($ArgumentList -join ' ')"
    }
}

function Assert-PortFree {
    param([Parameter(Mandatory)][int]$Port)
    $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    if ($connections) {
        $owners = ($connections | Select-Object -ExpandProperty OwningProcess -Unique) -join ', '
        throw "Port $Port is already in use by process $owners. No existing process was stopped."
    }
}
