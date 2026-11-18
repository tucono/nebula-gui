# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Downloads the Wintun zip named in wintun.sha256, checks its SHA-256, and
# extracts it into OutDir, so the DLL is at
# OutDir\wintun\bin\amd64\wintun.dll. A zip already in OutDir is checked
# again rather than downloaded again.
#
#   fetch-wintun.ps1 -OutDir dist\windows

param([Parameter(Mandatory = $true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$line = Get-Content (Join-Path $PSScriptRoot 'wintun.sha256') |
    Where-Object { $_ -match '^[0-9a-f]{64}\s+\S+$' } | Select-Object -First 1
if (-not $line) { throw 'wintun.sha256 has no "<sha256>  <file>" line' }
$want, $name = -split $line

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$zip = Join-Path $OutDir $name
if (-not (Test-Path $zip)) {
    Invoke-WebRequest -UseBasicParsing -Uri "https://www.wintun.net/builds/$name" -OutFile $zip
}
$got = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLowerInvariant()
if ($got -ne $want) {
    Remove-Item $zip
    throw "$name has SHA-256 $got, not $want as wintun.sha256 says; removed it"
}

$dest = Join-Path $OutDir 'wintun'
if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
Expand-Archive -Path $zip -DestinationPath $OutDir
Write-Output "wintun: $name verified and extracted to $dest"
