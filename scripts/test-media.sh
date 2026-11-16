#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Unique project, random credentials and dynamically assigned loopback ports.
export SREZ_S3_ACCESS_KEY="$(openssl rand -hex 20)"
export SREZ_S3_SECRET_KEY="$(openssl rand -hex 32)"
export POSTGRES_PASSWORD="$(openssl rand -hex 24)"
project="srez-media-test-$(date +%s)-$$"
compose=(docker compose -p "$project" -f compose.media-test.yaml)
# Stop/remove only containers from this isolated project. Volumes are retained.
trap '"${compose[@]}" down' EXIT
check_logs() {
 CONTAINER_ID="$("${compose[@]}" ps -q seaweedfs)" python3 - <<'PYLOG'
import os, subprocess
r = subprocess.run(["docker", "logs", os.environ["CONTAINER_ID"]], capture_output=True)
if r.returncode:
    raise SystemExit("Cannot inspect SeaweedFS logs")
logs = r.stdout + r.stderr
for name in ("SREZ_S3_ACCESS_KEY", "SREZ_S3_SECRET_KEY"):
    if os.environ[name].encode() in logs:
        raise SystemExit("S3 credential detected in SeaweedFS logs")
PYLOG
}
"${compose[@]}" up -d --wait --wait-timeout 180
check_logs
export TEST_S3_ENDPOINT="http://$("${compose[@]}" port seaweedfs 8333)"
export TEST_DATABASE_URL="postgres://srez:${POSTGRES_PASSWORD}@$("${compose[@]}" port db 5432)/srez?sslmode=disable"
go test ./... -count=1
go test ./internal/handler -run 'Test(PostgresMediaSeaweedFlow|MediaSlowPublicTCP|MediaUploadAdmission|MediaConfiguredBucket|MediaGC)' -count=1 -v
go test ./internal/objectstorage -run 'Test(SeaweedBucketPrivacyGuard|PrivacyInspectionFailure|PrivatePolicyPermissionAnalysis)' -count=1 -v
go test ./internal/storage -run TestPersistenceRejectsTruncatedAndCorruptedHTTPBody -count=1 -v
go test -race ./internal/usecase ./internal/handler ./internal/storage ./internal/objectstorage -count=1
export TEST_MEDIA_PERSISTENCE_SCHEMA="media_restart_$(date +%s)_$$"
TEST_S3_WRITE_RESTART=true go test ./internal/storage -run TestSeaweedPersistenceAfterRestart -count=1 -v
"${compose[@]}" restart seaweedfs
# Wait for the restarted container without recreating unrelated services.
"${compose[@]}" up -d --wait --wait-timeout 180 --no-recreate
check_logs
export TEST_S3_ENDPOINT="http://$("${compose[@]}" port seaweedfs 8333)"
export TEST_DATABASE_URL="postgres://srez:${POSTGRES_PASSWORD}@$("${compose[@]}" port db 5432)/srez?sslmode=disable"
TEST_S3_VERIFY_RESTART=true go test ./internal/storage -run TestSeaweedPersistenceAfterRestart -count=1 -v
printf 'Isolated test project: %s (volumes retained)\n' "$project"
