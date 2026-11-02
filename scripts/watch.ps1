# Runs csdc watch on this machine, restarting it if it exits, so demos
# downloaded in CS2 are uploaded on their own. Settings come from .env.watch
# in the repository root (gitignored):
#
#   CSDC_API_URL=https://csdc.example.com
#   CSDC_UPLOAD_TOKEN=...   (Auto-upload page on the site)
#
# Build first: go build -o bin/csdc.exe ./cmd/csdc
# Start at logon (once; -User lets a non-elevated shell register it):
#   Register-ScheduledTask -TaskName csdc-watch `
#     -Trigger (New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME") `
#     -Settings (New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0) `
#     -Action (New-ScheduledTaskAction -Execute powershell.exe `
#       -Argument "-NoProfile -WindowStyle Hidden -File `"$PWD\scripts\watch.ps1`"")
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Get-Content (Join-Path $root '.env.watch') | ForEach-Object {
    if ($_ -match '^\s*([A-Z_]+)\s*=\s*(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
    }
}

$log = Join-Path $root 'demos\out\watch.log'
New-Item -ItemType Directory -Force (Split-Path $log) | Out-Null
$exe = Join-Path $root 'bin\csdc.exe'
while ($true) {
    # Through cmd: Windows PowerShell turns a native command's stderr (csdc
    # logs there) into errors, which 'Stop' would make fatal.
    cmd /c "`"$exe`" watch >> `"$log`" 2>&1"
    Start-Sleep -Seconds 60
}
