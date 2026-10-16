# skill-manager agent bootstrap for Windows (PowerShell 5.1+)
# Downloads the binary to ~\.skill-manager\ and pushes every local agent skill
# directory it can find to the hub. Safe to re-run (idempotent).
param(
    [Parameter(Mandatory = $true)][string]$Server,
    [Parameter(Mandatory = $true)][string]$Token,
    [string]$Hostname = "",
    [string[]]$Paths = @()
)
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"  # PS5.1 progress rendering makes big downloads crawl

$HubDir = Join-Path $env:USERPROFILE ".skill-manager"
New-Item -ItemType Directory -Force -Path $HubDir | Out-Null
$Exe = Join-Path $HubDir "skill-manager.exe"
Write-Host "==> download skill-manager.exe -> $Exe"
Invoke-WebRequest -Uri "$Server/dl/skill-manager.exe" -OutFile $Exe -UseBasicParsing

if (-not $Hostname) { $Hostname = $env:COMPUTERNAME }

$candidates = @(
    (Join-Path $env:USERPROFILE ".agents\skills"),
    (Join-Path $env:USERPROFILE ".claude\skills"),
    (Join-Path $env:USERPROFILE ".claude\plugins"),
    (Join-Path $env:USERPROFILE ".zcode\cli\plugins\cache")
)
$auto = @($candidates | Where-Object { Test-Path $_ })
$extra = @($Paths | Where-Object { Test-Path $_ })
$all = @($auto) + @($extra)
if ($all.Count -eq 0) {
    Write-Host "no skill directories found; pass them via -Paths"
    exit 1
}

$fail = 0
foreach ($d in $all) {
    Write-Host "==> push $d (hostname: $Hostname)"
    & $Exe push --server $Server --token $Token --path $d --hostname $Hostname
    if ($LASTEXITCODE -ne 0) { $fail = 1 }
}

Write-Host "==> done. binary kept at $HubDir (safe to re-run)"
$dest = Join-Path $env:USERPROFILE ".agents\skills"
Write-Host ("    pull merged skills back: {0} pull --server {1} --token {2} --dest {3}" -f $Exe, $Server, $Token, $dest)
exit $fail
