param(
  [ValidateRange(1024, 65535)][int]$ApiPort = 8080,
  [string]$PublicOrigin = 'http://127.0.0.1:5173'
)
$ErrorActionPreference = 'Stop'
if ($PublicOrigin -notmatch '^http://127\.0\.0\.1:\d{4,5}$') { throw 'Для тестов разрешён только локальный Origin.' }
$srezAuthProject = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$srezAuthRun = [Guid]::NewGuid().ToString('N').Substring(0, 12)
$srezAuthNetwork = 'srez-auth-test-' + $srezAuthRun
$srezAuthDBName = 'srez-auth-db-' + $srezAuthRun
$srezAuthAppName = 'srez-auth-api-' + $srezAuthRun
$srezAuthVolume = 'srez-auth-data-' + $srezAuthRun
$srezAuthPassword = [Guid]::NewGuid().ToString('N')
$srezAuthDB = $null
$srezAuthApp = $null
function New-SrezAuthTestKey {
  $srezAuthBytes = New-Object byte[] 32
  $srezAuthRng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  try { $srezAuthRng.GetBytes($srezAuthBytes) } finally { $srezAuthRng.Dispose() }
  [Convert]::ToBase64String($srezAuthBytes)
}
try {
  if (Get-NetTCPConnection -LocalPort $ApiPort -State Listen -ErrorAction SilentlyContinue) { throw "Порт $ApiPort занят. Существующий сервер не будет остановлен." }
  & docker build --progress=plain -t srez-auth-test-api:local $srezAuthProject
  if ($LASTEXITCODE -ne 0) { throw 'Не удалось собрать тестовый API.' }
  & docker network create $srezAuthNetwork | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Не удалось создать тестовую сеть.' }
  & docker volume create $srezAuthVolume | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Не удалось создать отдельный тестовый volume.' }
  $srezAuthDB = (& docker run -d --name $srezAuthDBName --network $srezAuthNetwork --mount "type=volume,source=$srezAuthVolume,target=/var/lib/postgresql/data" -e POSTGRES_USER=auth_test -e "POSTGRES_PASSWORD=$srezAuthPassword" -e POSTGRES_DB=auth_test postgres:16-alpine | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $srezAuthDB -notmatch '^[0-9a-f]{64}$') { $srezAuthDB = $null; throw 'Не удалось запустить тестовую БД.' }
  $srezAuthDBReady = $false
  for ($srezAuthAttempt = 0; $srezAuthAttempt -lt 60; $srezAuthAttempt++) {
    & docker exec $srezAuthDB pg_isready -h 127.0.0.1 -U auth_test -d auth_test 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { $srezAuthDBReady = $true; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $srezAuthDBReady) { throw 'Тестовая БД не стала доступна.' }
  $srezAuthJWT = New-SrezAuthTestKey
  $srezAuthCSRF = New-SrezAuthTestKey
$srezAuthWishlist = New-SrezAuthTestKey
  $srezAuthApp = (& docker run -d --name $srezAuthAppName --network $srezAuthNetwork -p "127.0.0.1:${ApiPort}:8080" -e "DATABASE_URL=postgres://auth_test:${srezAuthPassword}@${srezAuthDBName}:5432/auth_test?sslmode=disable" -e "SREZ_JWT_KEY=$srezAuthJWT" -e "SREZ_CSRF_KEY=$srezAuthCSRF" -e "SREZ_WISHLIST_KEY=$srezAuthWishlist" -e SREZ_JWT_ISSUER=srez -e SREZ_JWT_AUDIENCE=srez-api -e "SREZ_PUBLIC_ORIGIN=$PublicOrigin" -e SREZ_AUTH_LOCAL_HTTP=true -e SREZ_AUTH_ATTEMPTS=100 srez-auth-test-api:local | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or $srezAuthApp -notmatch '^[0-9a-f]{64}$') { $srezAuthApp = $null; throw 'Не удалось запустить тестовый API.' }
  $srezAuthReady = $false
  for ($srezAuthAttempt = 0; $srezAuthAttempt -lt 20; $srezAuthAttempt++) {
    try { if ((Invoke-RestMethod "http://127.0.0.1:$ApiPort/health" -TimeoutSec 1).status -eq 'ok') { $srezAuthReady = $true; break } } catch { }
    Start-Sleep -Milliseconds 250
  }
  if (-not $srezAuthReady) { throw 'Тестовый API не стал доступен.' }
  Write-Output "READY: отдельные Go API и PostgreSQL для тестов на 127.0.0.1:$ApiPort, Origin $PublicOrigin."
  Write-Output "Тестовые контейнеры: $srezAuthAppName / $srezAuthDBName. Volume: $srezAuthVolume."
  $null = Read-Host 'После тестов нажмите Enter: будут остановлены только эти два контейнера; данные и volumes сохранятся'
} finally {
  if ($srezAuthApp) { & docker stop --time 5 $srezAuthApp | Out-Null }
  if ($srezAuthDB) { & docker stop --time 5 $srezAuthDB | Out-Null }
  Write-Output 'Volumes, данные, контейнеры и сеть не удалялись.'
}
