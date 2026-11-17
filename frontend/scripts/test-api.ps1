$ErrorActionPreference='Stop'
$frontendProject = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$frontendRunID=[Guid]::NewGuid().ToString('N').Substring(0,12)
$frontendTestNetwork='srez-front-test-'+$frontendRunID
$frontendDBName='srez-front-db-'+$frontendRunID
$frontendDBPassword=[Guid]::NewGuid().ToString('N')
$frontendDB=$null
$frontendApp=$null
$frontendNetworkReady=$false
function New-FrontendTestKey {
    $frontendBytes=New-Object byte[] 32
    $frontendRng=[System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {$frontendRng.GetBytes($frontendBytes)} finally {$frontendRng.Dispose()}
    [Convert]::ToBase64String($frontendBytes)
}
try {
    if (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue) { throw 'Порт 8080 занят. Тестовый API не будет останавливать существующий сервер.' }
    & docker build --progress=plain -t srez-frontend-test-api:local $frontendProject
    if ($LASTEXITCODE -ne 0) {throw 'Docker build failed'}
    & docker network create $frontendTestNetwork | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Test network creation failed'}
    $frontendNetworkReady=$true
    $frontendDB=(& docker run --rm -d --name $frontendDBName --network $frontendTestNetwork -e POSTGRES_USER=frontend_test -e "POSTGRES_PASSWORD=$frontendDBPassword" -e POSTGRES_DB=frontend_test postgres:16-alpine | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $frontendDB -notmatch '^[0-9a-f]{64}$') {$frontendDB=$null;throw 'Test PostgreSQL startup failed'}
    $frontendReady=$false
    for ($frontendAttempt=0;$frontendAttempt -lt 60;$frontendAttempt++) {
        & docker exec $frontendDB pg_isready -h 127.0.0.1 -U frontend_test -d frontend_test 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {$frontendReady=$true;break}
        Start-Sleep -Milliseconds 250
    }
    if (-not $frontendReady) {throw 'Test PostgreSQL did not become ready'}
    $frontendJWT=New-FrontendTestKey
    $frontendCSRF=New-FrontendTestKey
$frontendWishlist = New-FrontendTestKey
    $frontendApp=(& docker run --rm -d --network $frontendTestNetwork -p '127.0.0.1:8080:8080' -e "DATABASE_URL=postgres://frontend_test:${frontendDBPassword}@${frontendDBName}:5432/frontend_test?sslmode=disable" -e "SREZ_JWT_KEY=$frontendJWT" -e "SREZ_CSRF_KEY=$frontendCSRF" -e "SREZ_WISHLIST_KEY=$frontendWishlist" -e SREZ_JWT_ISSUER=srez -e SREZ_JWT_AUDIENCE=srez-api -e SREZ_PUBLIC_ORIGIN=http://127.0.0.1:5173 -e SREZ_AUTH_LOCAL_HTTP=true -e SREZ_AUTH_ATTEMPTS=100 srez-frontend-test-api:local | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $frontendApp -notmatch '^[0-9a-f]{64}$') {$frontendApp=$null;throw 'Test API startup failed'}
    Write-Output 'READY: isolated PostgreSQL/API on 127.0.0.1:8080; frontend Origin 127.0.0.1:5173. No working volumes used.'
    $null=Read-Host 'Press Enter to stop this test API and remove only its temporary containers/network'
} finally {
    if ($frontendApp) {& docker stop --time 5 $frontendApp | Out-Null}
    if ($frontendDB) {& docker stop --time 5 $frontendDB | Out-Null}
    if ($frontendNetworkReady) {& docker network rm $frontendTestNetwork | Out-Null}
}
