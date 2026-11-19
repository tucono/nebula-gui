# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Checks what the Windows installer left on this machine. CI runs it, as
# an administrator, after a silent install and after each silent
# uninstall.
#
#   check-install.ps1 -State Installed   the service runs and answers
#   check-install.ps1 -State Removed     uninstalled; settings and group kept
#   check-install.ps1 -State Purged      uninstalled with /REMOVESETTINGS

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('Installed', 'Removed', 'Purged')]
    [string]$State
)
$ErrorActionPreference = 'Stop'

$app = Join-Path $env:ProgramFiles 'nebula-gui'
$stateDir = Join-Path $env:ProgramData 'nebula-gui'
$failures = @()
function Check([bool]$ok, [string]$what) {
    if ($ok) { Write-Output "ok:   $what" }
    else { Write-Output "FAIL: $what"; $script:failures += $what }
}

$service = Get-CimInstance Win32_Service -Filter "Name='nebula-guid'"
$rule = Get-NetFirewallRule -DisplayName 'nebula-gui' -ErrorAction SilentlyContinue
$group = Get-LocalGroup -Name 'nebula-gui' -ErrorAction SilentlyContinue

# statusOverPipe sends GET /v1/status to the service's pipe, which
# Administrators may use, and returns the status line.
function statusOverPipe {
    $pipe = New-Object System.IO.Pipes.NamedPipeClientStream('.', 'nebula-guid', [System.IO.Pipes.PipeDirection]::InOut)
    try {
        $pipe.Connect(2000)
        $req = [System.Text.Encoding]::ASCII.GetBytes("GET /v1/status HTTP/1.0`r`nHost: nebula-guid`r`n`r`n")
        $pipe.Write($req, 0, $req.Length)
        $pipe.Flush()
        return (New-Object System.IO.StreamReader($pipe)).ReadLine()
    } finally {
        $pipe.Dispose()
    }
}

switch ($State) {
    'Installed' {
        foreach ($f in 'nebula-gui.exe', 'nebula-guid.exe', 'nebula-gui.ico',
            'dist\windows\wintun\bin\amd64\wintun.dll', 'dist\windows\wintun\LICENSE.txt') {
            Check (Test-Path (Join-Path $app $f)) "$f installed"
        }
        $ver = & (Join-Path $app 'nebula-guid.exe') --version
        Check ($LASTEXITCODE -eq 0 -and $ver) "nebula-guid --version: $ver"
        # The version details from packaging/windows/*.winres.json.
        foreach ($exe in 'nebula-gui', 'nebula-guid') {
            $info = (Get-Item (Join-Path $app "$exe.exe")).VersionInfo
            Check ($info.OriginalFilename -eq "$exe.exe" -and $info.ProductVersion) "$exe.exe details: $($info.FileDescription), $($info.ProductVersion)"
        }
        Check (Test-Path (Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\nebula-gui.lnk')) 'Start Menu shortcut'

        Check ($null -ne $service) 'service registered'
        if ($service) {
            Write-Output "      command line: $($service.PathName)"
            Check ($service.StartMode -eq 'Auto') "starts automatically (StartMode $($service.StartMode))"
            Check ($service.StartName -eq 'LocalSystem') "runs as LocalSystem ($($service.StartName))"
            Check ($service.PathName -like "*$app\nebula-guid.exe*run --socket-group nebula-gui*") 'runs the installed binary with the group'
        }

        Check ($null -ne $group) 'nebula-gui group exists'
        if ($group) {
            $members = (Get-LocalGroupMember -Name 'nebula-gui' | ForEach-Object Name) -join ', '
            Write-Output "      members: $members"
        }

        Check ($null -ne $rule) 'firewall rule exists'
        if ($rule) {
            $prog = ($rule | Get-NetFirewallApplicationFilter).Program
            $proto = ($rule | Get-NetFirewallPortFilter).Protocol
            Check ($rule.Direction -eq 'Inbound' -and $rule.Action -eq 'Allow' -and $rule.Enabled -eq 'True') 'rule allows inbound'
            Check ($proto -eq 'UDP') "rule is for UDP ($proto)"
            Check ($prog -eq (Join-Path $app 'nebula-guid.exe')) "rule is for nebula-guid.exe ($prog)"
        }

        # The installer starts the service; give it a moment to listen.
        $line = $null
        for ($i = 0; $i -lt 20 -and -not $line; $i++) {
            try { $line = statusOverPipe } catch { Start-Sleep -Milliseconds 500 }
        }
        Check ((Get-Service nebula-guid).Status -eq 'Running') 'service running'
        Check ($line -match '^HTTP/1\.[01] 200') "API answers over the pipe ($line)"
        Check (Test-Path $stateDir) 'state directory created'
    }
    'Removed' {
        Check ($null -eq $service) 'service removed'
        Check ($null -eq $rule) 'firewall rule removed'
        Check (-not (Test-Path (Join-Path $app 'nebula-guid.exe'))) 'program files removed'
        Check (-not (Get-Process nebula-guid -ErrorAction SilentlyContinue)) 'nebula-guid not running'
        Check (Test-Path $stateDir) 'state directory kept'
        Check ($null -ne $group) 'nebula-gui group kept'
    }
    'Purged' {
        Check ($null -eq $service) 'service removed'
        Check ($null -eq $rule) 'firewall rule removed'
        Check (-not (Test-Path (Join-Path $app 'nebula-guid.exe'))) 'program files removed'
        Check (-not (Test-Path $stateDir)) 'state directory removed'
        Check ($null -eq $group) 'nebula-gui group removed'
    }
}

if ($failures.Count -gt 0) {
    Write-Output "$($failures.Count) check(s) failed"
    exit 1
}
Write-Output "all checks passed ($State)"
