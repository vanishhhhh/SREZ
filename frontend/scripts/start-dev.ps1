param([ValidateSet(5174,5175)][int]$FrontendPort = 5174)
$ErrorActionPreference = 'Stop'
$srezFrontendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$srezViteEntry = Join-Path $srezFrontendRoot 'node_modules/vite/bin/vite.js'
$srezFrontendURL = 'http://localhost:' + $FrontendPort
$srezListener = Get-NetTCPConnection -LocalPort $FrontendPort -State Listen -ErrorAction SilentlyContinue
if ($srezListener) {
  $srezOwnProcess = $false
  foreach ($srezPID in @($srezListener.OwningProcess | Select-Object -Unique)) {
    $srezProcess = Get-CimInstance Win32_Process -Filter "ProcessId=$srezPID"
    if ($srezProcess.CommandLine -and $srezProcess.CommandLine.Replace('/', '\').Contains($srezViteEntry.Replace('/', '\'))) { $srezOwnProcess = $true }
  }
  if (-not $srezOwnProcess) { throw "Port $FrontendPort belongs to another process. Nothing was stopped or replaced." }
  Write-Output "SREZ integration is already running. Open $srezFrontendURL"
  exit 0
}
$srezNode = (Get-Command node.exe -ErrorAction Stop).Source
if (-not (Test-Path -LiteralPath $srezViteEntry -PathType Leaf)) {
  throw 'Frontend dependencies are missing. Run npm install in the frontend directory first.'
}
$srezLogRoot = Join-Path $srezFrontendRoot ('node_modules/.cache/srez-dev-' + $FrontendPort)
$null = New-Item -ItemType Directory -Path $srezLogRoot -Force
$srezOutputLog = Join-Path $srezLogRoot 'stdout.log'
$srezErrorLog = Join-Path $srezLogRoot 'stderr.log'
# Use only the separate integration API, regardless of the terminal environment.
$srezPreviousTarget = $env:SREZ_API_TARGET
$srezPreviousPort = $env:SREZ_FRONTEND_PORT
try {
  $env:SREZ_API_TARGET = if ($FrontendPort -eq 5175) { 'http://127.0.0.1:8082' } else { 'http://127.0.0.1:8081' }
  $env:SREZ_FRONTEND_PORT = [string]$FrontendPort
  $srezDevProcess = Start-Process -FilePath $srezNode -ArgumentList @('"' + $srezViteEntry + '"') -WorkingDirectory $srezFrontendRoot -WindowStyle Hidden -RedirectStandardOutput $srezOutputLog -RedirectStandardError $srezErrorLog -PassThru
} finally {
  $env:SREZ_API_TARGET = $srezPreviousTarget
  $env:SREZ_FRONTEND_PORT = $srezPreviousPort
}
$srezReady = $false
for ($srezAttempt = 0; $srezAttempt -lt 30; $srezAttempt++) {
  if ($srezDevProcess.HasExited) { break }
  try {
    if ((Invoke-WebRequest -UseBasicParsing -Uri $srezFrontendURL -TimeoutSec 1).StatusCode -eq 200) {
      $srezReady = $true
      break
    }
  } catch { }
  Start-Sleep -Milliseconds 200
}
if (-not $srezReady) { throw "Frontend did not become ready. Logs: $srezLogRoot" }
Write-Output "SREZ frontend is running: $srezFrontendURL"
Write-Output "Background process ID: $($srezDevProcess.Id). Logs: $srezLogRoot"
Write-Output "The separate API must be running: 8081 for port 5174, 8082 for QA port 5175."
