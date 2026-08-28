[CmdletBinding()]
param(
    [string]$Version,
    [switch]$SkipInstall,
    [string]$OutputDir = 'dist/build'
)

. (Join-Path $PSScriptRoot 'common.ps1')

$go = Get-GoCommand
$npm = Get-NpmCommand
$versionValue = Get-RepoVersion $Version
Assert-VersionSafe $versionValue
$commit = Get-GitCommit
$buildTime = [DateTime]::UtcNow.ToString('o')
$outputPath = Join-Path $script:RepoRoot $OutputDir
$serverOutput = Join-Path $outputPath 'mpackstation.exe'
$webOutput = Join-Path $outputPath 'web'

New-Item -ItemType Directory -Force -Path $outputPath | Out-Null
if (-not $SkipInstall) {
    Push-Location $script:WebDir
    try { Invoke-Checked $npm @('ci') $script:WebDir } finally { Pop-Location }
}

Push-Location $script:WebDir
try { Invoke-Checked $npm @('run', 'build') $script:WebDir } finally { Pop-Location }

Push-Location $script:ServerDir
try {
    Invoke-Checked $go @('mod', 'download') $script:ServerDir
    $ldflags = "-X main.version=$versionValue"
    Invoke-Checked $go @('build', '-trimpath', '-ldflags', $ldflags, '-o', $serverOutput, './cmd/server') $script:ServerDir
} finally { Pop-Location }

if (Test-Path -LiteralPath $webOutput) { Remove-Item -LiteralPath $webOutput -Recurse -Force }
Copy-Item -LiteralPath (Join-Path $script:WebDir 'dist') -Destination $webOutput -Recurse
Set-Content -LiteralPath (Join-Path $outputPath 'VERSION') -Value $versionValue -NoNewline
Set-Content -LiteralPath (Join-Path $outputPath 'BUILD.txt') -Value "version=$versionValue`ncommit=$commit`nbuild_time=$buildTime`n" -NoNewline
Write-Host "Build complete: $outputPath"
