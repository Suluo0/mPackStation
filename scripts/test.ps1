[CmdletBinding()]
param([switch]$SkipFrontend)

. (Join-Path $PSScriptRoot 'common.ps1')
$go = Get-GoCommand
$fmt = Get-GoFmtCommand
$npm = Get-NpmCommand

Push-Location $script:ServerDir
try {
    $unformatted = (& $fmt '-l' '.') -join "`n"
    if ($unformatted) { throw "Unformatted files:`n$unformatted" }
    Invoke-Checked $go @('test', './...') $script:ServerDir
    Invoke-Checked $go @('vet', './...') $script:ServerDir
} finally { Pop-Location }

if (-not $SkipFrontend) {
    Push-Location $script:WebDir
    try { Invoke-Checked $npm @('run', 'test') $script:WebDir } finally { Pop-Location }
}
Write-Host 'Tests complete.'

