param(
  [Parameter(Mandatory = $true)][string]$Companion,
  [Parameter(Mandatory = $true)][string]$Replacement,
  [Parameter(Mandatory = $true)][string]$CodexBinary,
  [Parameter(Mandatory = $true)][string]$BoxHost,
  [Parameter(Mandatory = $true)][int]$MacPort,
  [Parameter(Mandatory = $true)][int]$PhonePort,
  [Parameter(Mandatory = $true)][string]$PinnedKey,
  [Parameter(Mandatory = $true)][string]$RelaySecret,
  [Parameter(Mandatory = $true)][string]$ProjectDirectory
)

$ErrorActionPreference = "Stop"
foreach ($Path in @($Companion, $Replacement, $CodexBinary)) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "companion smoke: a required local file is missing"
  }
}
if (-not (Test-Path -LiteralPath $ProjectDirectory -PathType Container)) {
  throw "companion smoke: project directory is missing"
}

$SmokeHome = Join-Path ([System.IO.Path]::GetTempPath()) ("codex-launcher-smoke-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $SmokeHome | Out-Null
$env:USERPROFILE = $SmokeHome
$env:LOCALAPPDATA = Join-Path $SmokeHome "AppData\Local"
$env:APPDATA = Join-Path $SmokeHome "AppData\Roaming"
$env:XDG_CONFIG_HOME = Join-Path $SmokeHome ".config"
$InstalledCompanion = Join-Path $env:APPDATA "codex-launcher\bin\codex-launcher.exe"
$MaintenanceReceipt = Join-Path $env:LOCALAPPDATA "codex-launcher\maintenance\last-result.json"
$Installed = $false

function Wait-Maintenance([string]$Operation) {
  for ($Attempt = 0; $Attempt -lt 300; $Attempt++) {
    if (Test-Path -LiteralPath $MaintenanceReceipt -PathType Leaf) {
      $Result = Get-Content -LiteralPath $MaintenanceReceipt -Raw | ConvertFrom-Json
      if ($Result.operation -eq $Operation) {
        if (-not $Result.ok) { throw "companion smoke: deferred maintenance failed" }
        return
      }
    }
    Start-Sleep -Milliseconds 100
  }
  throw "companion smoke: deferred maintenance timed out"
}

try {
  & $Companion version
  & $Companion setup `
    --computer-name "Codex Launcher smoke" `
    --box-host $BoxHost `
    --mac-port $MacPort `
    --phone-port $PhonePort `
    --pinned-key $PinnedKey `
    --relay-secret $RelaySecret `
    --codex-binary $CodexBinary `
    --project-id smoke `
    --project-name "Smoke project" `
    --project-path $ProjectDirectory
	& $Companion install
	$Installed = $true
	& $InstalledCompanion status
	& $InstalledCompanion doctor
	& $InstalledCompanion install --replace $Replacement
	Wait-Maintenance "replace"
	& $InstalledCompanion status
	& $InstalledCompanion doctor
	& $InstalledCompanion rollback
	Wait-Maintenance "rollback"
	& $InstalledCompanion status
	& $InstalledCompanion doctor
	& $InstalledCompanion uninstall
	Wait-Maintenance "uninstall"
  $Installed = $false
  Write-Output "companion smoke: install, replace, rollback, and uninstall passed"
}
finally {
	if ($Installed) {
		try { & $InstalledCompanion uninstall | Out-Null } catch { }
  }
  Remove-Item -LiteralPath $SmokeHome -Recurse -Force -ErrorAction SilentlyContinue
}
