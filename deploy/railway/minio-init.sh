#!/bin/sh
set -eu

: "${MINIO_ROOT_USER:?MINIO_ROOT_USER is required}"
: "${MINIO_ROOT_PASSWORD:?MINIO_ROOT_PASSWORD is required}"
: "${MINIO_ACCESS_KEY:?MINIO_ACCESS_KEY is required}"
: "${MINIO_SECRET_KEY:?MINIO_SECRET_KEY is required}"

minio_endpoint="${MINIO_ENDPOINT_URL:-http://minio.railway.internal:9000}"
attempt=1
until mc alias set gamegen "${minio_endpoint}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" >/dev/null 2>&1 \
  && mc ready gamegen >/dev/null 2>&1; do
  if [ "${attempt}" -ge 30 ]; then
    echo "MinIO did not become ready at ${minio_endpoint}" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 2
done

for bucket in gamegen-user-assets gamegen-source-assets gamegen-render-assets gamegen-artifacts; do
  mc mb --ignore-existing "gamegen/${bucket}"
  mc anonymous set none "gamegen/${bucket}"
done

if ! mc admin user info gamegen "${MINIO_ACCESS_KEY}" >/dev/null 2>&1; then
  mc admin user add gamegen "${MINIO_ACCESS_KEY}" "${MINIO_SECRET_KEY}"
fi
if ! mc admin policy info gamegen gamegen-app >/dev/null 2>&1; then
  mc admin policy create gamegen gamegen-app /config/minio-app-policy.json
fi
mc admin policy attach gamegen gamegen-app --user "${MINIO_ACCESS_KEY}"

echo "RECALL MinIO buckets and application account are ready."
