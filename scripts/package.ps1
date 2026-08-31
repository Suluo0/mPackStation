[CmdletBinding()]
param(
    [string]$Version,
    [switch]$SkipBuild,
    [string]$OutputDir = 'dist/package'
)

. (Join-Path $PSScriptRoot 'common.ps1')
$versionValue = Get-RepoVersion $Version
Assert-VersionSafe $versionValue
$buildDir = Join-Path $script:RepoRoot 'dist/build'
$packageDir = Join-Path $script:RepoRoot $OutputDir
$zipPath = Join-Path $script:DistRoot "mpackstation-$versionValue.zip"

if (-not $SkipBuild) { & (Join-Path $PSScriptRoot 'build.ps1') -Version $versionValue }
if (-not (Test-Path -LiteralPath (Join-Path $buildDir 'mpackstation.exe'))) {
    throw "Build output is missing: $buildDir"
}

if (Test-Path -LiteralPath $packageDir) { Remove-Item -LiteralPath $packageDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $packageDir | Out-Null
Copy-Item -LiteralPath (Join-Path $buildDir 'mpackstation.exe') -Destination $packageDir
Copy-Item -LiteralPath (Join-Path $buildDir 'web') -Destination $packageDir -Recurse
Copy-Item -LiteralPath (Join-Path $buildDir 'VERSION') -Destination $packageDir
Copy-Item -LiteralPath (Join-Path $buildDir 'BUILD.txt') -Destination $packageDir
Copy-Item -LiteralPath (Join-Path $script:RepoRoot 'LICENSE') -Destination $packageDir
$readme = @(
    'mPackStation single-instance distribution',
    '',
    'Run mpackstation.exe -data <absolute-data-directory>.',
    'The distribution never includes user data, API keys, caches, or exports.'
) -join "`n"
Set-Content -LiteralPath (Join-Path $packageDir 'README.txt') -Value $readme -NoNewline

# 双库红线(设计约定, 勿删): 模组身份知识库(mod_identity_baseline.json)通过
# go:embed 编进 exe 随分发; 用户库(整合包/模组/key/任务)只存在于用户指定的
# -data 目录, 永不进包。这里机械断言包内不出现任何数据库文件。
$leaked = Get-ChildItem -LiteralPath $packageDir -Recurse -Include '*.db','*.db-*','*.sqlite*' -File
if ($leaked) { throw "Package must never contain user database files: $($leaked.FullName -join ', ')" }

New-Item -ItemType Directory -Force -Path $script:DistRoot | Out-Null
if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
Compress-Archive -LiteralPath (Join-Path $packageDir '*') -DestinationPath $zipPath
Write-Host "Package complete: $zipPath"
