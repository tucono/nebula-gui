# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Checks what the Windows uninstaller does with a user's own files: this
# user's, since a runner has no second user who has used the app. CI runs
# it as an administrator around each silent uninstall.
#
#   check-uninstall.ps1 -State Seed   make the files the app would leave,
#                                     and another app's beside them
#   check-uninstall.ps1 -State Kept   after an uninstall: only the
#                                     start-at-sign-in values are gone
#   check-uninstall.ps1 -State Gone   after /REMOVESETTINGS: all are gone
#
# Another app's files and values must survive both.

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('Seed', 'Kept', 'Gone')]
    [string]$State
)
$ErrorActionPreference = 'Stop'

# The names in internal/userfiles.
$dirs = @(
    (Join-Path $env:APPDATA 'nebula-gui'),
    (Join-Path $env:APPDATA 'fyne\io.github.tucono.nebulagui')
)
$otherFiles = @(
    (Join-Path $env:APPDATA 'other-app\keep.json'),
    (Join-Path $env:APPDATA 'fyne\other-app\keep.json')
)
$run = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$approved = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run'

$failures = @()
function Check([bool]$ok, [string]$what) {
    if ($ok) { Write-Output "ok:   $what" }
    else { Write-Output "FAIL: $what"; $script:failures += $what }
}

function HasValue([string]$key, [string]$name) {
    $k = Get-Item -LiteralPath $key -ErrorAction SilentlyContinue
    return ($null -ne $k) -and ($null -ne $k.GetValue($name))
}

function SetValues([string]$name) {
    foreach ($key in $run, $approved) {
        # New-Item -Force on a key that exists would empty it.
        if (-not (Test-Path -LiteralPath $key)) { New-Item -Path $key -Force | Out-Null }
    }
    New-ItemProperty -LiteralPath $run -Name $name -PropertyType String -Force `
        -Value "`"$env:ProgramFiles\$name\$name.exe`"" | Out-Null
    # Task Manager's "turned off" record: 3, then a timestamp.
    New-ItemProperty -LiteralPath $approved -Name $name -PropertyType Binary -Force `
        -Value ([byte[]](3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)) | Out-Null
}

switch ($State) {
    'Seed' {
        foreach ($d in $dirs) {
            New-Item -ItemType Directory -Path (Join-Path $d 'logs') -Force | Out-Null
            Set-Content -LiteralPath (Join-Path $d 'saved.json') -Value '{}'
            Set-Content -LiteralPath (Join-Path $d 'logs\app.log') -Value 'x'
        }
        foreach ($f in $otherFiles) {
            New-Item -ItemType Directory -Path (Split-Path $f) -Force | Out-Null
            Set-Content -LiteralPath $f -Value '{}'
        }
        SetValues 'nebula-gui'
        SetValues 'other-app'
        Write-Output "seeded $env:USERNAME's files under $env:APPDATA and the Run values"
        exit 0
    }
    'Kept' {
        foreach ($d in $dirs) { Check (Test-Path -LiteralPath (Join-Path $d 'saved.json')) "$d kept" }
    }
    'Gone' {
        foreach ($d in $dirs) { Check (-not (Test-Path -LiteralPath $d)) "$d removed" }
    }
}

Check (-not (HasValue $run 'nebula-gui')) 'Run value removed'
Check (-not (HasValue $approved 'nebula-gui')) 'StartupApproved\Run value removed'
foreach ($f in $otherFiles) { Check (Test-Path -LiteralPath $f) "another app's $f kept" }
Check (HasValue $run 'other-app') "another app's Run value kept"
Check (HasValue $approved 'other-app') "another app's StartupApproved\Run value kept"

if ($failures.Count -gt 0) {
    Write-Output "$($failures.Count) check(s) failed"
    exit 1
}
Write-Output "all checks passed ($State)"
