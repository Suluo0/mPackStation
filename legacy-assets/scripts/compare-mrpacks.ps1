param(
  [Parameter(Mandatory = $true)][string]$Reference,
  [Parameter(Mandatory = $true)][string]$Candidate
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem

function Read-Mrpack([string]$path) {
  $resolved = (Resolve-Path -LiteralPath $path).Path
  $zip = [System.IO.Compression.ZipFile]::OpenRead($resolved)
  try {
    $entry = $zip.GetEntry('modrinth.index.json')
    if (-not $entry) { throw "Missing modrinth.index.json in $resolved" }
    $reader = [System.IO.StreamReader]::new($entry.Open())
    try { $index = $reader.ReadToEnd() | ConvertFrom-Json } finally { $reader.Dispose() }

    $overrides = @{}
    foreach ($item in $zip.Entries) {
      $name = $item.FullName.Replace('\', '/')
      if ($name.StartsWith('overrides/') -and -not $name.EndsWith('/')) {
        $stream = $item.Open()
        $sha = [System.Security.Cryptography.SHA256]::Create()
        try { $hash = ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-', '') }
        finally { $stream.Dispose(); $sha.Dispose() }
        $overrides[$name.Substring(10)] = $hash
      }
    }
    return [PSCustomObject]@{ Index = $index; Overrides = $overrides }
  }
  finally { $zip.Dispose() }
}

$left = Read-Mrpack $Reference
$right = Read-Mrpack $Candidate
$differences = [System.Collections.Generic.List[string]]::new()

foreach ($field in @('formatVersion', 'game', 'versionId', 'name', 'summary')) {
  if ($left.Index.$field -ne $right.Index.$field) {
    $differences.Add("metadata.$field differs")
  }
}

foreach ($dependency in @('minecraft', 'forge', 'neoforge', 'fabric-loader', 'quilt-loader')) {
  if ($left.Index.dependencies.$dependency -ne $right.Index.dependencies.$dependency) {
    $differences.Add("dependencies.$dependency differs")
  }
}

$leftFiles = @{}
foreach ($file in $left.Index.files) { $leftFiles[$file.path] = $file }
$rightFiles = @{}
foreach ($file in $right.Index.files) { $rightFiles[$file.path] = $file }

foreach ($path in @($leftFiles.Keys + $rightFiles.Keys | Sort-Object -Unique)) {
  if (-not $leftFiles.ContainsKey($path)) { $differences.Add("added file: $path"); continue }
  if (-not $rightFiles.ContainsKey($path)) { $differences.Add("removed file: $path"); continue }
  $a = $leftFiles[$path]
  $b = $rightFiles[$path]
  if ($a.fileSize -ne $b.fileSize) { $differences.Add("$path fileSize differs") }
  if ($a.hashes.sha1 -ne $b.hashes.sha1) { $differences.Add("$path sha1 differs") }
  if ($a.hashes.sha512 -ne $b.hashes.sha512) { $differences.Add("$path sha512 differs") }
  if (($a.downloads -join '|') -ne ($b.downloads -join '|')) { $differences.Add("$path downloads differ") }
  if ($a.env.client -ne $b.env.client -or $a.env.server -ne $b.env.server) { $differences.Add("$path environment differs") }
}

foreach ($path in @($left.Overrides.Keys + $right.Overrides.Keys | Sort-Object -Unique)) {
  if (-not $left.Overrides.ContainsKey($path)) { $differences.Add("added override: $path"); continue }
  if (-not $right.Overrides.ContainsKey($path)) { $differences.Add("removed override: $path"); continue }
  if ($left.Overrides[$path] -ne $right.Overrides[$path]) { $differences.Add("changed override: $path") }
}

Write-Output "Reference files: $($leftFiles.Count)"
Write-Output "Candidate files: $($rightFiles.Count)"
Write-Output "Reference overrides: $($left.Overrides.Count)"
Write-Output "Candidate overrides: $($right.Overrides.Count)"

if ($differences.Count -gt 0) {
  Write-Output "Differences: $($differences.Count)"
  $differences | ForEach-Object { Write-Output "- $_" }
  exit 1
}

Write-Output 'MRPACKS_EQUIVALENT'
