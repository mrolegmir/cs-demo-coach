# Runs the share-code bot (bot/bot.js) on this machine, restarting it if it
# exits. Settings come from .env.bot in the repository root (gitignored):
#
#   CSDC_API_URL=https://csdc.example.com
#   CSDC_WORKER_TOKEN=...   (the same token the worker uses)
#
# Once: cd bot; npm ci --ignore-scripts; node bot.js login
# Start at logon (once; -User lets a non-elevated shell register it):
#   Register-ScheduledTask -TaskName csdc-bot `
#     -Trigger (New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME") `
#     -Settings (New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0) `
#     -Action (New-ScheduledTaskAction -Execute powershell.exe `
#       -Argument "-NoProfile -WindowStyle Hidden -File `"$PWD\scripts\bot.ps1`"")
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Get-Content (Join-Path $root '.env.bot') | ForEach-Object {
    if ($_ -match '^\s*([A-Z_]+)\s*=\s*(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
    }
}

$log = Join-Path $root 'demos\out\bot.log'
New-Item -ItemType Directory -Force (Split-Path $log) | Out-Null
$bot = Join-Path $root 'bot\bot.js'
while ($true) {
    cmd /c "node `"$bot`" run >> `"$log`" 2>&1"
    # Exit code 2: the saved session is missing or was rejected, and only
    # "node bot.js login" fixes that, so retry rarely.
    if ($LASTEXITCODE -eq 2) { Start-Sleep -Seconds 3600 } else { Start-Sleep -Seconds 60 }
}
