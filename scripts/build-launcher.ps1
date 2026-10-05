[CmdletBinding()]
# 注意：参数名不能叫 -Debug。cmdlet 的公共参数已占用 Debug/Verbose/ErrorAction 等名字，
# 带 [CmdletBinding()] 的脚本再声明同名的会直接 MetadataError（2026-10-06 在 Windows 上实测）。
param(
    [switch]$DebugBuild,
    [string]$TargetDir,
    [switch]$SkipCopy
)

# 编译 launcherCore（Rust）→ mpack-launcher.exe，并安装到 <repo>\.tools\launcher\，
# 也就是 dev.ps1 通过 MPACK_LAUNCHER_BIN 交给后端的位置。
#
# 为什么要专门一个脚本：build.ps1 只管 Go exe + 前端 dist，不含内核；
# 而后端「安装整合包 / 启动游戏」这两个能力全部依赖这个二进制，缺了它
# 提交任务会同步返回 503（service/launcher_task.go 的 launcherPreflight）。
#
# TargetDir 建议指向本地 SSD（例如 -TargetDir D:\cargo-target\mpack）。
# cargo 的 target 目录如果落在网络盘或外置卷上，增量编译会慢一个数量级。
. (Join-Path $PSScriptRoot 'common.ps1')

if (-not (Get-Command cargo -ErrorAction SilentlyContinue)) {
    throw "cargo not found. 安装 Rust 工具链：https://rustup.rs （安装后重开终端）"
}

$buildProfile = if ($DebugBuild) { 'debug' } else { 'release' }
$cargoArgs = @('build')
if (-not $DebugBuild) { $cargoArgs += '--release' }

if ($TargetDir) {
    New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
    $env:CARGO_TARGET_DIR = (Resolve-Path -LiteralPath $TargetDir).Path
    Write-Host "[launcher] CARGO_TARGET_DIR=$($env:CARGO_TARGET_DIR)"
}

Write-Host "[launcher] cargo $($cargoArgs -join ' ')  (profile=$buildProfile)"
Push-Location $script:LauncherDir
try {
    & cargo @cargoArgs
    if ($LASTEXITCODE -ne 0) { throw "cargo build failed with exit code $LASTEXITCODE" }
} finally {
    Pop-Location
}

$targetRoot = if ($env:CARGO_TARGET_DIR) { $env:CARGO_TARGET_DIR } else { Join-Path $script:LauncherDir 'target' }
$built = Join-Path $targetRoot "$buildProfile/mpack-launcher.exe"
if (-not (Test-Path -LiteralPath $built)) {
    throw "构建产物未找到：$built"
}

if ($SkipCopy) {
    Write-Host "[launcher] built: $built  (skip copy)"
    exit 0
}

$dest = Get-LauncherExePath
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $dest) | Out-Null
Copy-Item -LiteralPath $built -Destination $dest -Force
Write-Host "[launcher] installed: $dest"
Write-Host "[launcher] 下一次 scripts/dev.ps1 启动时会自动通过 MPACK_LAUNCHER_BIN 使用它。"
