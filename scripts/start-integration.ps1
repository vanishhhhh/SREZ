$ErrorActionPreference = 'Stop'
$srezIntegrationRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$srezPreviousDirectory = Get-Location
try {
  Set-Location -LiteralPath $srezIntegrationRoot
  & node .\frontend\scripts\prepare-local.mjs
  if ($LASTEXITCODE -ne 0) { throw 'Local configuration is invalid; nothing restarted.' }
  & docker compose -p srez-integration -f compose.integration.yaml up -d --build --wait --wait-timeout 120
  if ($LASTEXITCODE -ne 0) { throw 'Integration API failed to start. Existing project is untouched.' }
  $srezAPIReady = $false
  for ($srezAttempt = 0; $srezAttempt -lt 20; $srezAttempt++) {
    try {
      if ((Invoke-RestMethod -Uri 'http://127.0.0.1:8081/health' -TimeoutSec 2).status -eq 'ok') {
        $srezAPIReady = $true
        break
      }
    } catch { }
    Start-Sleep -Milliseconds 500
  }
  if (-not $srezAPIReady) { throw 'Integration API is not healthy on port 8081. Data and containers were retained.' }
  $srezUIReady = $false
  for ($srezAttempt = 0; $srezAttempt -lt 40; $srezAttempt++) {
    try {
      if ((Invoke-WebRequest -UseBasicParsing -Uri 'http://localhost:5174' -TimeoutSec 3).StatusCode -eq 200 -and (Invoke-RestMethod -Uri 'http://localhost:5174/api/health' -TimeoutSec 3).status -eq 'ok') { $srezUIReady = $true; break }
    } catch { }
    Start-Sleep -Milliseconds 500
  }
  if (-not $srezUIReady) { throw 'Frontend is not healthy on port 5174. Inspect the integration frontend container; data was retained.' }
  Write-Output 'SREZ integration: http://localhost:5174'
  Write-Output 'Mailpit inbox: http://127.0.0.1:8025'
  Write-Output 'Original project remains separate at http://127.0.0.1:5173'
} finally {
  Set-Location -LiteralPath $srezPreviousDirectory
}
