#!/bin/sh
set -eu
# The mini env-credential bootstrap logs its access key at info level. Supply
# an explicit private config instead; do not echo credentials or enable xtrace.
: "${AWS_ACCESS_KEY_ID:?S3 access key required}"
: "${AWS_SECRET_ACCESS_KEY:?S3 secret key required}"
# Supports generated hex/base64 keys; reject JSON control/escape characters.
case "$AWS_ACCESS_KEY_ID$AWS_SECRET_ACCESS_KEY" in
  *[!a-zA-Z0-9/+=_.-]*) echo 'Invalid S3 credential characters' >&2; exit 1 ;;
esac
umask 077
# Official entrypoint drops to the seaweed user. Its private config must be
# readable after privilege drop, without putting it on the persistent volume.
mkdir -p /tmp/srez-config
printf '{"identities":[{"name":"srez-backend","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","Write","List","Tagging"]}]}' \
  "$AWS_ACCESS_KEY_ID" "$AWS_SECRET_ACCESS_KEY" > /tmp/srez-config/s3.json
if [ "$(id -u)" = 0 ]; then
  chown -R seaweed:seaweed /tmp/srez-config
fi
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY
exec /entrypoint.sh "$@" -s3.config=/tmp/srez-config/s3.json
