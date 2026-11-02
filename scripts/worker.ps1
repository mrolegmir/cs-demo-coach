# Runs a worker on this machine against a remote API, restarting it if it
# exits. Settings come from .env.worker in the repository root (gitignored):
#
#   CSDC_API_URL=https://csdc.example.com
#   CSDC_WORKER_TOKEN=...
#   CSDC_LLM=codex
#
# Build first: go build -o bin/worker.exe ./cmd/worker
# Start at logon (once; -User lets a non-elevated shell register it):
#   Register-ScheduledTask -TaskName csdc-worker `
#     -Trigger (New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME") `
#     -Settings (New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0) `
#     -Action (New-ScheduledTaskAction -Execute powershell.exe `
#       -Argument "-NoProfile -WindowStyle Hidden -File `"$PWD\scripts\worker.ps1`"")
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Get-Content (Join-Path $root '.env.worker') | ForEach-Object {
    if ($_ -match '^\s*([A-Z_]+)\s*=\s*(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
    }
}

$log = Join-Path $root 'demos\out\home-worker.log'
New-Item -ItemType Directory -Force (Split-Path $log) | Out-Null
$exe = Join-Path $root 'bin\worker.exe'
while ($true) {
    # Through cmd: Windows PowerShell turns a native command's stderr (the
    # worker logs there) into errors, which 'Stop' would make fatal.
    cmd /c "`"$exe`" >> `"$log`" 2>&1"
    Start-Sleep -Seconds 10
}
