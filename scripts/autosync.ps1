[CmdletBinding()]
param(
    # 跟随的分支，默认取当前 HEAD 所在分支。
    [string]$Branch,
    # 轮询间隔（秒）。fetch 很轻，30s 足够灵敏又不至于刷屏。
    [int]$IntervalSec = 30,
    # 只跑一轮就退出（用于手工验证 / 计划任务的一次性触发）。
    [switch]$Once,
    # 即使只有前端源码变化也强制重启服务。
    [switch]$ForceRestart,
    # 启动时若服务没在跑，是否顺手拉起（默认拉，让它成为完整守护进程）。
    [switch]$NoStartServices
)

# 远端 → 本机的单向同步：轮询远端新提交，ff 拉下来，按改动路径决定重建什么、要不要重启。
#
# 定位：本机（PC）是**消费端**，提交只从另一台机器推。所以：
#   - 工作区有未提交改动时**拒绝拉取**并记日志，绝不覆盖本地改动；
#   - 只用 --ff-only，不做 merge，避免产生奇怪的合并提交。
#
# 为什么不无脑全量重建：三端的生效方式不一样（详见 windows-iteration.md）
#   apps/web3/src/**           → vite HMR，保存即生效，什么都不用做
#   apps/server/**             → go run 的是编译后的进程，必须重启
#   launcherCore/**            → 得 cargo build 再重启
#   apps/web3/package*.json    → 得先 npm ci
#
# 用法：
#   pwsh scripts/autosync.ps1                       # 常驻轮询（服务没起会先拉起）
#   pwsh scripts/autosync.ps1 -Once                 # 只跑一轮（手工验证）
#   pwsh scripts/autosync.ps1 -IntervalSec 15       # 调间隔
. (Join-Path $PSScriptRoot 'common.ps1')

if (-not $Branch) {
    $Branch = (& git -C $script:RepoRoot rev-parse --abbrev-ref HEAD 2>$null).Trim()
}
if (-not $Branch -or $Branch -eq 'HEAD') {
    throw "无法确定分支，请显式传 -Branch。"
}

$logDir = Join-Path $script:RepoRoot '.tmp/autosync'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$logFile = Join-Path $logDir 'autosync.log'

function Write-Log {
    param(
        [Parameter(Mandatory)][string]$Level,
        [Parameter(Mandatory)][string]$Message
    )
    $line = "{0} [{1}] {2}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $Level, $Message
    Write-Host $line
    Add-Content -LiteralPath $logFile -Value $line -Encoding UTF8
}

function Get-Short {
    param([string]$Sha)
    if ($Sha -and $Sha.Length -ge 8) { return $Sha.Substring(0, 8) }
    return $Sha
}

function Test-ServiceUp {
    foreach ($port in @($script:ServerPort, $script:WebPort)) {
        if (-not (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)) {
            return $false
        }
    }
    return $true
}

function Wait-ServiceUp {
    param([int]$TimeoutSec = 90)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-ServiceUp) { return $true }
        Start-Sleep -Seconds 2
    }
    return $false
}

# 为什么不用 `& pwsh -File ...`：
# dev.ps1 用 Start-Process 拉起的 go / node 常驻服务会继承调用方的输出句柄，
# 于是「dev.ps1 进程早已退出」但调用方永远等不到 EOF —— 实测 & 调用 120s 不返回。
# 改成 Start-Process + 带超时的 WaitForExit，把这件事兜住；脚本输出落盘便于排查。
function Invoke-RepoScript {
    param(
        [Parameter(Mandatory)][string]$Name,
        # dev.ps1 / dev-stop.ps1 秒级收尾；cargo build 首次可能几分钟，单独放宽。
        [int]$TimeoutSec = 120,
        # 透传给被调脚本的额外参数（例如 dev-stop.ps1 -ByPort）。
        [string[]]$ExtraArgs = @()
    )
    $stem = [IO.Path]::GetFileNameWithoutExtension($Name)
    $outFile = Join-Path $logDir "${stem}.out.log"
    $errFile = Join-Path $logDir "${stem}.err.log"
    $psArgs = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $PSScriptRoot $Name)) + $ExtraArgs

    $proc = Start-Process -FilePath 'pwsh' -ArgumentList $psArgs -WorkingDirectory $script:RepoRoot `
        -RedirectStandardOutput $outFile -RedirectStandardError $errFile `
        -WindowStyle Hidden -PassThru
    if (-not $proc.WaitForExit($TimeoutSec * 1000)) {
        # 注意 -f 必须在括号内先把字符串拼好，不能写成 `Write-Log 'WARN' (...) -f ...`：
        # 那个位置会被解析成 Write-Log 的参数名，直接抛参数绑定异常（本文件犯过两次）。
        $msg = "{0} 在 {1}s 内未退出；不断定失败，继续按端口判断。输出见 .tmp/autosync/{2}.out.log" -f $Name, $TimeoutSec, $stem
        Write-Log 'WARN' $msg
        return $false
    }
    return $true
}

# 取当前真正持有两个 dev 端口的进程 PID（去重排序）。
# 用途不是"生死判定"而是"是否换过"：端口在监听 ≠ 服务重启过。
function Get-ListenerPids {
    $pids = @()
    foreach ($port in @($script:ServerPort, $script:WebPort)) {
        $pids += @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty OwningProcess)
    }
    return @($pids | Sort-Object -Unique)
}

function Start-Services {
    Write-Log 'INFO' '启动 dev 服务（后端 18872 / 前端 5271）'
    $null = Invoke-RepoScript 'dev.ps1'
    if (Wait-ServiceUp -TimeoutSec 90) {
        Write-Log 'INFO' ("服务已就绪（双端口均在监听，pid={0}）" -f ((Get-ListenerPids) -join ','))
    } else {
        Write-Log 'WARN' '90s 内未见到双端口监听，检查 .tmp/dev/*.log 与 .tmp/autosync/dev.out.log'
    }
}

function Restart-Services {
    $before = (Get-ListenerPids) -join ','

    # 必须带 -ByPort。只按 PID 文件停是不够的：PID 文件可能缺失或过期
    # （例如服务是在另一个会话里手工起的，或 `go run` 父进程先退出、真正
    # 持有端口的 server.exe 变成孤儿）。那种情况下旧进程停不掉，随后
    # dev.ps1 的 Assert-PortFree 会直接抛错，而端口上仍挂着旧进程——
    # 于是 Wait-ServiceUp 看到"端口在监听"，日志打出"服务已就绪"，
    # 实际后端根本没重启。这正是最不该发生的静默失败。
    $null = Invoke-RepoScript 'dev-stop.ps1' -ExtraArgs @('-ByPort')

    $deadline = (Get-Date).AddSeconds(20)
    while ((Get-Date) -lt $deadline -and (Test-ServiceUp)) { Start-Sleep -Milliseconds 500 }
    if (Test-ServiceUp) {
        Write-Log 'WARN' '停止后 20s 端口仍被占用；dev.ps1 会因 Assert-PortFree 失败，本次重启大概率无效。'
    }

    Start-Services

    $after = (Get-ListenerPids) -join ','
    if ($after -and $after -eq $before) {
        $msg = ("重启后监听 PID 未变（{0}）—— 服务很可能没真正重启，" +
            "请查 .tmp/autosync/dev.out.log / dev.err.log。") -f $after
        Write-Log 'WARN' $msg
    }
}

function Invoke-Sync {
    & git -C $script:RepoRoot fetch origin $Branch --quiet 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Log 'WARN' 'git fetch 失败（网络或代理？），本轮跳过。'
        return
    }

    $localSha = (& git -C $script:RepoRoot rev-parse HEAD).Trim()
    $remoteSha = (& git -C $script:RepoRoot rev-parse "origin/$Branch").Trim()
    if ($localSha -eq $remoteSha) {
        # -ForceRestart 必须在"无新提交"时也能生效。原先它在下面那个重建判断里，
        # 而这里已经 return 了，于是「没有新提交但想强制重启」永远打不到 ——
        # 恰恰是最需要它的场景（手工验证、改了环境变量、想重置进程状态）。
        if ($ForceRestart) {
            $msg = "无新提交（{0}），-ForceRestart 生效 → 重启服务" -f (Get-Short $localSha)
            Write-Log 'INFO' $msg
            Restart-Services
        }
        return
    }

    # 拒绝在有本地改动时拉取：宁可停住也不要覆盖本地改动。
    $dirty = & git -C $script:RepoRoot status --porcelain
    if ($dirty) {
        $count = ($dirty | Measure-Object).Count
        $msg = ("远端有新提交 {0}，但工作区有 {1} 项未提交改动 —— 跳过。" +
            "确认无用后执行：git checkout -- . ; git clean -fd") -f (Get-Short $remoteSha), $count
        Write-Log 'WARN' $msg
        return
    }

    $changed = @(& git -C $script:RepoRoot diff --name-only $localSha $remoteSha)
    Write-Log 'INFO' (("发现新提交 {0} → {1}（{2} 个文件）") -f (Get-Short $localSha), (Get-Short $remoteSha), $changed.Count)

    & git -C $script:RepoRoot merge --ff-only "origin/$Branch" --quiet
    if ($LASTEXITCODE -ne 0) {
        Write-Log 'ERROR' 'fast-forward 合并失败（本地已偏离远端），需要人工介入。'
        return
    }

    $needNpm = @($changed | Where-Object { $_ -eq 'apps/web3/package.json' -or $_ -eq 'apps/web3/package-lock.json' })
    $needRust = @($changed | Where-Object { $_ -like 'launcherCore/*' })
    $needGo = @($changed | Where-Object { $_ -like 'apps/server/*' })
    $needMigration = @($changed | Where-Object { $_ -like 'apps/server/internal/store/migrations/*' })

    if ($needMigration.Count -gt 0) {
        $msg = "本次含数据库迁移（{0} 个）。下次启动会自动应用；若想从干净库开始，跑 scripts/dev-reset.ps1" -f $needMigration.Count
        Write-Log 'WARN' $msg
    }

    if ($needGo.Count -gt 0) { Write-Log 'INFO' ("后端改动 {0} 个文件" -f $needGo.Count) }
    if ($needRust.Count -gt 0) { Write-Log 'INFO' ("内核改动 {0} 个文件" -f $needRust.Count) }

    if ($needNpm.Count -gt 0) {
        Write-Log 'INFO' 'apps/web3 依赖清单有变 → npm ci'
        Push-Location (Join-Path $script:RepoRoot 'apps/web3')
        try {
            & npm ci --no-audit --no-fund
            if ($LASTEXITCODE -ne 0) { Write-Log 'ERROR' "npm ci 失败（exit $LASTEXITCODE）" }
        } finally {
            Pop-Location
        }
    }

    if ($needRust.Count -gt 0) {
        Write-Log 'INFO' 'launcherCore 有变 → cargo build'
        if (-not (Invoke-RepoScript 'build-launcher.ps1' -TimeoutSec 900)) {
            Write-Log 'WARN' 'cargo build 未在 900s 内收尾，看 .tmp/autosync/build-launcher.out.log'
        }
    }

    if ($needGo.Count -gt 0 -or $needRust.Count -gt 0 -or $needNpm.Count -gt 0 -or $ForceRestart) {
        Restart-Services
    } else {
        Write-Log 'INFO' '只有前端源码变化，vite HMR 自行生效，不重启服务'
    }
}

# 启动时先把服务带起来：这样「远端推完 → 本机随时可用」成立，
# 而不是要等到第一次检测到提交才想起服务没跑。
if (-not $NoStartServices -and -not (Test-ServiceUp)) {
    Start-Services
}

if ($Once) {
    Invoke-Sync
    exit 0
}

Write-Log 'INFO' ("autosync 启动：branch={0}  interval={1}s  repo={2}" -f $Branch, $IntervalSec, $script:RepoRoot)
while ($true) {
    try {
        Invoke-Sync
    } catch {
        Write-Log 'ERROR' ("同步异常：{0}" -f $_.Exception.Message)
    }
    Start-Sleep -Seconds $IntervalSec
}
