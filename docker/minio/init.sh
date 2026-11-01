#!/bin/sh
set -eu
umask 077

: "${MINIO_ROOT_USER:?Required}"
: "${MINIO_ROOT_PASSWORD:?Required}"
: "${MINIO_APP_ACCESS_KEY:?Required}"
: "${MINIO_APP_SECRET_KEY:?Required}"
: "${MINIO_BUCKET:?Required}"
# Empty prefix means all objects in this single private bucket.
MINIO_PREFIX=${MINIO_PREFIX-}
case "$MINIO_BUCKET" in ''|*[!a-z0-9.-]*) echo 'Invalid bucket name' >&2; exit 1;; esac
case "$MINIO_PREFIX" in *[!a-zA-Z0-9/_-]*) echo 'Invalid prefix' >&2; exit 1;; esac

mc() { /usr/local/bin/mc --config-dir /tmp/mc "$@"; }
mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc mb --ignore-existing "local/$MINIO_BUCKET" >/dev/null
mc anonymous set none "local/$MINIO_BUCKET" >/dev/null

object_resource="arn:aws:s3:::$MINIO_BUCKET/*"
if [ -n "$MINIO_PREFIX" ]; then
    object_resource="arn:aws:s3:::$MINIO_BUCKET/$MINIO_PREFIX/*"
fi
cat > /tmp/app-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
    "Resource": ["arn:aws:s3:::$MINIO_BUCKET"]
  }, {
    "Effect": "Allow",
    "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
    "Resource": ["$object_resource"]
  }]
}
EOF
mc admin user add local "$MINIO_APP_ACCESS_KEY" "$MINIO_APP_SECRET_KEY" >/dev/null
mc admin policy create local absensi-photo-storage /tmp/app-policy.json >/dev/null
mc admin policy attach local absensi-photo-storage --user "$MINIO_APP_ACCESS_KEY" >/dev/null
printf 'MinIO ready: private bucket and scoped application credentials configured.\n'
