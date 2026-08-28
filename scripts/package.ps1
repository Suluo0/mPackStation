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

New-Item -ItemType Directory -Force -Path $script:DistRoot | Out-Null
if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
Compress-Archive -LiteralPath (Join-Path $packageDir '*') -DestinationPath $zipPath
Write-Host "Package complete: $zipPath"
