[CmdletBinding()]
param(
    [string]$DataDir,
    [string]$ProjectsDir,
    [switch]$KeepData,
    [switch]$KeepProjects
)

# 一键回到「干净测试起点」：停服 → 归档并清空开发库 → 归档导入产生的项目目录。
#
# 为什么要脚本化：导入测试会在数据目录里留下包记录/模组台账/任务日志，在项目根
# 目录下留下 <包名>-<短id>\ 的实例目录（一个 132 模组的包 ≈ 1.2GB）。手工清理容
# 易只删一半（例如删了目录但库里还留着包记录），下次导入就分不清哪条是脏数据。
# 这里的口径与 2026-10-05 的手工清理一致：**归档而不是直接删**，随时可回退。
#
# 用法：
#   pwsh scripts/dev-reset.ps1                 # 全部重置（数据 + 项目目录）
#   pwsh scripts/dev-reset.ps1 -KeepData       # 只清项目目录
#   pwsh scripts/dev-reset.ps1 -KeepProjects   # 只重置数据库
. (Join-Path $PSScriptRoot 'common.ps1')

if (-not $DataDir) { $DataDir = Get-DevDataDir }
if (-not $ProjectsDir) { $ProjectsDir = Join-Path $env:USERPROFILE 'Documents\mPackStation Projects' }
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'

# ── 1. 停服（先停，避免文件被占用 / WAL 处于半写状态）────────────────
# 这里不调 dev-stop.ps1：它在端口没释放时会 exit 1，而脚本内 & 调用的 exit
# 会把整个会话一起结束，重置流程会半途而废。
Write-Host "[reset] 停止开发服务…"
foreach ($port in @($script:ServerPort, $script:WebPort)) {
    $owners = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty OwningProcess -Unique
    foreach ($ownerPid in $owners) {
        & taskkill /PID $ownerPid /T /F 2>$null | Out-Null
        Write-Host "  port $port held by pid=$ownerPid, killed"
    }
}
Start-Sleep -Seconds 2

# ── 2. 数据库 ────────────────────────────────────────────────────────
if (-not $KeepData) {
    if (-not (Test-Path -LiteralPath $DataDir)) {
        Write-Host "[reset] 数据目录不存在，跳过：$DataDir"
    } else {
        $backup = Join-Path $DataDir "backup-$stamp"
        New-Item -ItemType Directory -Force -Path $backup | Out-Null
        $moved = 0
        foreach ($pattern in @('mpackstation.db*', 'mpack.db*', 'runtime-token', 'server.lock')) {
            Get-ChildItem -LiteralPath $DataDir -Filter $pattern -File -ErrorAction SilentlyContinue | ForEach-Object {
                Move-Item -LiteralPath $_.FullName -Destination $backup -Force
                $moved++
            }
        }
        # 导入暂存（mpack-import-*）也是可再生的
        $tmpDir = Join-Path $DataDir 'tmp'
        if (Test-Path -LiteralPath $tmpDir) {
            Get-ChildItem -LiteralPath $tmpDir -ErrorAction SilentlyContinue | ForEach-Object {
                Move-Item -LiteralPath $_.FullName -Destination $backup -Force
                $moved++
            }
        }
        Write-Host "[reset] 数据库已归档 $moved 项 → $backup"
        Write-Host "[reset] 下次启动 dev 会新建空库（迎新流程会引导建包）"
    }
} else {
    Write-Host "[reset] -KeepData：数据库保留"
}

# ── 3. 项目目录（导入/安装产生的实例）────────────────────────────────
if (-not $KeepProjects) {
    if (-not (Test-Path -LiteralPath $ProjectsDir)) {
        Write-Host "[reset] 项目目录不存在，跳过：$ProjectsDir"
    } else {
        $dirs = Get-ChildItem -LiteralPath $ProjectsDir -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -notlike 'backup-*' }
        if (-not $dirs) {
            Write-Host "[reset] 项目目录已空：$ProjectsDir"
        } else {
            $sizeMb = [math]::Round((($dirs | ForEach-Object {
                (Get-ChildItem -LiteralPath $_.FullName -Recurse -File -ErrorAction SilentlyContinue |
                    Measure-Object -Property Length -Sum).Sum
            }) | Measure-Object -Sum).Sum / 1MB, 1)
            Write-Host "[reset] 归档 $($dirs.Count) 个项目目录（约 $sizeMb MB）…"
            $archive = Join-Path $env:TEMP "mpack-projects-$stamp"
            New-Item -ItemType Directory -Force -Path $archive | Out-Null
            foreach ($d in $dirs) {
                Move-Item -LiteralPath $d.FullName -Destination $archive -Force
            }
            Write-Host "[reset] 项目目录已归档 → $archive"
            Write-Host "[reset] 确认无误后可直接删除该归档目录释放空间"
        }
    }
} else {
    Write-Host "[reset] -KeepProjects：项目目录保留"
}

Write-Host "[reset] 完成。重新启动：pwsh scripts/dev.ps1"
