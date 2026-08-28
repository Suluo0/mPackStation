[CmdletBinding()]
param(
    [switch]$AllowIncomplete,
    [switch]$SkipInstall,
    [switch]$SkipBuild
)

. (Join-Path $PSScriptRoot 'common.ps1')
$go = Get-GoCommand
$fmt = Get-GoFmtCommand
$npm = Get-NpmCommand
$failures = [System.Collections.Generic.List[string]]::new()
$warnings = [System.Collections.Generic.List[string]]::new()

function Run-Check {
    param([string]$Label, [scriptblock]$Action)
    try { & $Action; Write-Host "PASS  $Label" }
    catch { $failures.Add("${Label}: $($_.Exception.Message)") }
}

Run-Check 'Go formatting' {
    Push-Location $script:ServerDir
    try {
        $unformatted = (& $fmt '-l' '.') -join "`n"
        if ($unformatted) { throw "Unformatted files:`n$unformatted" }
    } finally { Pop-Location }
}
Run-Check 'Go tests' { Push-Location $script:ServerDir; try { Invoke-Checked $go @('test', './...') $script:ServerDir } finally { Pop-Location } }
Run-Check 'Go vet' { Push-Location $script:ServerDir; try { Invoke-Checked $go @('vet', './...') $script:ServerDir } finally { Pop-Location } }

Push-Location $script:WebDir
try {
    if (-not $SkipInstall) { Run-Check 'Frontend lockfile install' { Invoke-Checked $npm @('ci') $script:WebDir } }
    Run-Check 'Frontend test/type check' { Invoke-Checked $npm @('run', 'test') $script:WebDir }
    if (-not $SkipBuild) { Run-Check 'Frontend production build' { Invoke-Checked $npm @('run', 'build') $script:WebDir } }
} finally { Pop-Location }

# The verifier is intentionally explicit about unfinished v7 surface area. It never
# turns a missing endpoint into a false green result.
$httpSource = Join-Path $script:ServerDir 'internal/httpapi'
$expectedRoutes = @(
    'GET /api/dashboard', 'GET /api/packs', 'POST /api/packs',
    'GET /api/tasks', 'GET /api/activities', 'POST /api/packs/import',
    'GET /api/system/status', 'GET /api/onboarding', 'GET /api/meta/mc-versions'
)
$sourceText = if (Test-Path -LiteralPath $httpSource) {
    (Get-ChildItem $httpSource -Filter '*.go' | Where-Object { $_.Name -notlike '*_test.go' } | Get-Content -Raw) -join "`n"
} else { '' }
$missingRoutes = @($expectedRoutes | Where-Object {
    $parts = $_.Split(' ', 2)
    $sourceText -notmatch [Regex]::Escape($parts[0]) -or $sourceText -notmatch [Regex]::Escape($parts[1])
})
if ($missingRoutes.Count -gt 0) {
    $message = "Expected v7 routes not yet present: $($missingRoutes -join ', ')"
    if ($AllowIncomplete) { $warnings.Add($message) } else { $failures.Add($message) }
}

Write-Host "`nVerification summary"
if ($warnings.Count) { $warnings | ForEach-Object { Write-Warning $_ } }
if ($failures.Count) {
    $failures | ForEach-Object { Write-Error $_ }
    exit 1
}
Write-Host 'Verification complete: all applicable checks passed.'
