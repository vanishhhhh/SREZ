$ErrorActionPreference = 'Stop'
$srezRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Install Go 1.27.2+ and reopen PowerShell.' }
$srezEnvPath = Join-Path $srezRoot '.env'
if (-not (Test-Path -LiteralPath $srezEnvPath)) { throw 'Run node frontend/scripts/prepare-local.mjs first.' }
# Treat .env strictly as data, never Invoke-Expression or shell source.
foreach ($srezLine in (Get-Content -LiteralPath $srezEnvPath)) {
    $srezMatch = [regex]::Match($srezLine, '^([A-Z][A-Z0-9_]*)=(.*)$')
    if ($srezMatch.Success) {
        $srezValue = $srezMatch.Groups[2].Value.Trim()
        if ($srezValue.Length -ge 2 -and (($srezValue.StartsWith('"') -and $srezValue.EndsWith('"')) -or ($srezValue.StartsWith("'") -and $srezValue.EndsWith("'")))) {
            $srezValue = $srezValue.Substring(1, $srezValue.Length - 2)
        }
        [Environment]::SetEnvironmentVariable($srezMatch.Groups[1].Value, $srezValue, 'Process')
    }
}
# Explicit integration ports prevent accidentally using the original DB:5432.
$srezPassword = [Uri]::EscapeDataString($env:POSTGRES_PASSWORD)
$env:DATABASE_URL = "postgres://srez:${srezPassword}@127.0.0.1:15432/srez?sslmode=disable"
$env:HTTP_ADDR = '127.0.0.1:8081'
$env:SREZ_S3_ENDPOINT = 'http://127.0.0.1:18333'
$env:SREZ_S3_PUBLIC_ENDPOINT = 'http://127.0.0.1:18333'
$env:SREZ_PUBLIC_ORIGIN = 'http://localhost:5174'
$env:SREZ_FRONTEND_BASE_URL = 'http://localhost:5174'
$env:SREZ_AUTH_LOCAL_HTTP = 'true'
$env:SREZ_SMTP_HOST = '127.0.0.1'
$env:SREZ_SMTP_PORT = '1025'
$env:SREZ_SMTP_TLS = 'none'
$srezPreviousDirectory = Get-Location
try {
    Set-Location -LiteralPath $srezRoot
    & go run ./cmd/app
    if ($LASTEXITCODE -ne 0) { throw 'Backend stopped with an error. Inspect the safe application log.' }
} finally { Set-Location -LiteralPath $srezPreviousDirectory }
