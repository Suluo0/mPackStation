# Shared, side-effect-light helpers for repository PowerShell entrypoints.
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$script:ServerDir = Join-Path $script:RepoRoot 'apps/server'
# 唯一前端入口 = apps/web3（IDE 式单页）。apps/web、apps/web2 是已退役的旧前端，
# 端口/鉴权口径都已作废，不要再指过去（2026-10-05 对齐 AGENTS.md）。
$script:WebDir = Join-Path $script:RepoRoot 'apps/web3'
$script:LauncherDir = Join-Path $script:RepoRoot 'launcherCore'
$script:DistRoot = Join-Path $script:RepoRoot 'dist'

# 唯一开发数据目录（对应 macOS/Linux 的 /tmp/mpack-data）。
# 空库可直接跑，迎新流程会引导建包；要重置换一个目录或删掉即可。
function Get-DevDataDir {
    if ($env:MPACK_DEV_DATA) { return $env:MPACK_DEV_DATA }
    return (Join-Path $env:TEMP 'mpack-data')
}

# 后端 / 前端 标准端口（AGENTS.md 定稿，勿散落成字面量）。
$script:ServerPort = 18872
$script:WebPort = 5271

# 启动器内核的分发位置。注意后端自己的查找口径是 `<数据目录的父目录>/.tools/launcher/`
# （service/api.go 的 workbenchRoot = Dir(dataDir)），数据目录落在 %TEMP% 时那个推导
# 会指向 %TEMP%\.tools，很不直观。所以这里固定放仓库内，并由 dev 脚本通过
# MPACK_LAUNCHER_BIN 显式指给后端（该环境变量优先级最高）。
function Get-LauncherExePath {
    return (Join-Path $script:RepoRoot '.tools/launcher/mpack-launcher.exe')
}

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
