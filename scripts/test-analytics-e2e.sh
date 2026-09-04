#!/usr/bin/env bash

set -Eeuo pipefail
set +x

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BACKEND_DIR="${PROJECT_ROOT}/backend"
APP_ENV_FILE="${GAME_GEN_ENV_FILE:-${PROJECT_ROOT}/.env}"
COMPOSE_ENV_FILE="${PROJECT_ROOT}/deploy/compose/dev/.env"
COMPOSE_FILE="${PROJECT_ROOT}/deploy/compose/dev/compose.yaml"
BROWSER_E2E_SCRIPT="${PROJECT_ROOT}/scripts/test-analytics-browser-e2e.sh"

if [[ "${ANALYTICS_E2E:-}" != "1" ]]; then
  echo "analytics E2E is opt-in; rerun with ANALYTICS_E2E=1" >&2
  exit 2
fi
if [[ ! -f "${APP_ENV_FILE}" || ! -f "${COMPOSE_ENV_FILE}" ]]; then
  echo "analytics E2E requires local app and development Compose environment files" >&2
  exit 2
fi
if [[ ! -x "${BROWSER_E2E_SCRIPT}" ]]; then
  echo "analytics E2E requires the executable browser harness" >&2
  exit 2
fi

set -a
# shellcheck disable=SC1090
source "${APP_ENV_FILE}"
# shellcheck disable=SC1090
source "${COMPOSE_ENV_FILE}"
set +a

case "${MYSQL_DSN:-}" in
  */*\?*) ;;
  *)
    echo "analytics E2E cannot derive an isolated DSN from MYSQL_DSN" >&2
    exit 2
    ;;
esac

E2E_DATABASE="gamegen_e2e_$(date +%s)_${RANDOM}"
if [[ ! "${E2E_DATABASE}" =~ ^gamegen_e2e_[0-9]+_[0-9]+$ ]]; then
  echo "analytics E2E generated an unsafe database name" >&2
  exit 2
fi
if [[ ! "${MYSQL_USER:-}" =~ ^[A-Za-z0-9_]+$ ]]; then
  echo "analytics E2E requires a simple local MySQL username" >&2
  exit 2
fi
E2E_MYSQL_DSN="${MYSQL_DSN%%/*}/${E2E_DATABASE}?${MYSQL_DSN#*\?}"
E2E_BIN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gamegen-analytics-e2e.XXXXXX")"
chmod 700 "${E2E_BIN_DIR}"
DATABASE_CREATED=0
TEST_PID=""
CHILD_PGID_FILE="${E2E_BIN_DIR}/child-process-groups"
: >"${CHILD_PGID_FILE}"
chmod 600 "${CHILD_PGID_FILE}"
E2E_BROWSER_TASK="analytics-e2e-$(openssl rand -hex 8)"

compose() {
  docker compose --env-file "${COMPOSE_ENV_FILE}" -f "${COMPOSE_FILE}" "$@"
}

mysql_admin() {
  compose exec -T -e E2E_DATABASE="${E2E_DATABASE}" mysql sh -eu -c '
    export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
    exec mysql --user=root --batch --skip-column-names
  '
}

cleanup_minio_user() {
  local user_id="$1"
  if [[ ! "${user_id}" =~ ^[0-7][0-9A-HJKMNP-TV-Z]{25}$ ]]; then
    return
  fi
  compose --profile setup run --rm --no-deps \
    -e E2E_USER_ID="${user_id}" --entrypoint /bin/sh minio-init -eu -c '
      mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
      for bucket in gamegen-user-assets gamegen-source-assets gamegen-render-assets gamegen-artifacts; do
        mc rm --recursive --force "local/$bucket/users/$E2E_USER_ID/" >/dev/null 2>&1 || true
      done
    ' >/dev/null 2>&1 || true
}

snapshot_process_tree() {
  local pid="$1"
  local child=""
  if [[ ! "${pid}" =~ ^[0-9]+$ ]]; then
    return
  fi
  printf '%s\n' "${pid}"
  if command -v pgrep >/dev/null 2>&1; then
    while IFS= read -r child; do
      snapshot_process_tree "${child}"
    done < <(pgrep -P "${pid}" 2>/dev/null || true)
  fi
}

terminate_process_tree() {
  local pid="$1"
  local snapshot_file="${E2E_BIN_DIR}/process-snapshot"
  local target_file="${E2E_BIN_DIR}/process-targets"
  local target=""
  local group=""
  if [[ ! "${pid}" =~ ^[0-9]+$ ]]; then
    return
  fi

  # Resolve the entire current tree and merge every independently-created child
  # process group before sending any signal. The registry survives if the Go test
  # exits first and its API, Vite, Worker, or browser children are reparented.
  snapshot_process_tree "${pid}" >"${snapshot_file}"
  {
    cat "${snapshot_file}"
    cat "${CHILD_PGID_FILE}"
  } | awk '/^[0-9]+$/' | sort -u >"${target_file}"

  while IFS= read -r group; do
    [[ "${group}" =~ ^[0-9]+$ ]] || continue
    kill -TERM -- "-${group}" >/dev/null 2>&1 || true
  done <"${CHILD_PGID_FILE}"
  while IFS= read -r target; do
    [[ "${target}" =~ ^[0-9]+$ ]] || continue
    kill -TERM "${target}" >/dev/null 2>&1 || true
  done <"${target_file}"

  for _ in {1..50}; do
    local alive=0
    while IFS= read -r target; do
      if [[ "${target}" =~ ^[0-9]+$ ]] && kill -0 "${target}" >/dev/null 2>&1; then
        alive=1
        break
      fi
    done <"${target_file}"
    if [[ "${alive}" == "0" ]]; then
      break
    fi
    sleep 0.1
  done
  while IFS= read -r group; do
    [[ "${group}" =~ ^[0-9]+$ ]] || continue
    kill -KILL -- "-${group}" >/dev/null 2>&1 || true
  done <"${CHILD_PGID_FILE}"
  while IFS= read -r target; do
    [[ "${target}" =~ ^[0-9]+$ ]] || continue
    kill -KILL "${target}" >/dev/null 2>&1 || true
  done <"${target_file}"
}

run_browser_cleanup() {
  env -i \
    HOME="${HOME}" PATH="${PATH}" TMPDIR="${TMPDIR:-/tmp}" \
    LANG="${LANG:-}" LC_ALL="${LC_ALL:-}" LC_CTYPE="${LC_CTYPE:-}" \
    ANALYTICS_E2E_BROWSER_TASK="${E2E_BROWSER_TASK}" \
    "${E2E_BIN_DIR}/browser-harness" cleanup
}

cleanup() {
  local exit_code=$?
  local user_ids=""
  trap - EXIT INT TERM
  if [[ -n "${TEST_PID}" ]]; then
    terminate_process_tree "${TEST_PID}"
    wait "${TEST_PID}" >/dev/null 2>&1 || true
    TEST_PID=""
  fi
  if [[ -n "${E2E_BROWSER_TASK}" && -x "${E2E_BIN_DIR}/browser-harness" ]]; then
    run_browser_cleanup >/dev/null 2>&1 || true
  fi
  if [[ "${DATABASE_CREATED}" == "1" ]]; then
    user_ids="$(mysql_admin 2>/dev/null <<SQL || true
SELECT id FROM \`${E2E_DATABASE}\`.users;
SQL
)"
    while IFS= read -r user_id; do
      cleanup_minio_user "${user_id}"
    done <<<"${user_ids}"
    mysql_admin >/dev/null 2>&1 <<SQL || true
REVOKE ALL PRIVILEGES ON \`${E2E_DATABASE}\`.* FROM '${MYSQL_USER}'@'%';
SQL
    mysql_admin >/dev/null 2>&1 <<SQL || true
DROP DATABASE IF EXISTS \`${E2E_DATABASE}\`;
SQL
  fi
  if [[ -d "${E2E_BIN_DIR}" && "${E2E_BIN_DIR}" == "${TMPDIR:-/tmp}"/gamegen-analytics-e2e.* ]]; then
    rm -rf -- "${E2E_BIN_DIR}"
  fi
  exit "${exit_code}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

compose up -d --wait mysql minio
compose --profile setup run --rm minio-init >/dev/null

mysql_admin >/dev/null <<SQL
CREATE DATABASE \`${E2E_DATABASE}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
SQL
DATABASE_CREATED=1
mysql_admin >/dev/null <<SQL
GRANT ALL PRIVILEGES ON \`${E2E_DATABASE}\`.* TO '${MYSQL_USER}'@'%';
SQL

(
  cd "${BACKEND_DIR}"
  MYSQL_DSN="${E2E_MYSQL_DSN}" go run ./cmd/migrate -command up >/dev/null
  go build -o "${E2E_BIN_DIR}/api" ./cmd/api
  go build -o "${E2E_BIN_DIR}/worker" ./cmd/worker
  go build -o "${E2E_BIN_DIR}/hash-password" ./cmd/hash-password
  go build -o "${E2E_BIN_DIR}/browser-harness" ./integration/browser_harness
)

E2E_ADMIN_PASSWORD="$(openssl rand -hex 24)"
E2E_ADMIN_PASSWORD_HASH="$("${E2E_BIN_DIR}/hash-password" "${E2E_ADMIN_PASSWORD}")"
E2E_CREATOR_PASSWORD="$(openssl rand -hex 24)"

export ANALYTICS_E2E_MYSQL_DSN="${E2E_MYSQL_DSN}"
export ANALYTICS_E2E_API_BIN="${E2E_BIN_DIR}/api"
export ANALYTICS_E2E_WORKER_BIN="${E2E_BIN_DIR}/worker"
export ANALYTICS_E2E_ADMIN_USERNAME="analytics_admin"
export ANALYTICS_E2E_ADMIN_PASSWORD="${E2E_ADMIN_PASSWORD}"
export ANALYTICS_E2E_ADMIN_PASSWORD_HASH="${E2E_ADMIN_PASSWORD_HASH}"
export ANALYTICS_E2E_CREATOR_PASSWORD="${E2E_CREATOR_PASSWORD}"
export ANALYTICS_E2E_FRONTEND_DIR="${PROJECT_ROOT}/frontend"
export ANALYTICS_E2E_BROWSER_SCRIPT="${BROWSER_E2E_SCRIPT}"
export ANALYTICS_E2E_BROWSER_GENERATOR="${E2E_BIN_DIR}/browser-harness"
export ANALYTICS_E2E_BROWSER_TASK="${E2E_BROWSER_TASK}"
export ANALYTICS_E2E_CHILD_PGID_FILE="${CHILD_PGID_FILE}"

(
  cd "${BACKEND_DIR}"
  test_environment=(
    "HOME=${HOME}" "PATH=${PATH}" "TMPDIR=${TMPDIR:-/tmp}"
    "LANG=${LANG:-}" "LC_ALL=${LC_ALL:-}" "LC_CTYPE=${LC_CTYPE:-}"
    "ANALYTICS_E2E=1"
    "ANALYTICS_E2E_MYSQL_DSN=${ANALYTICS_E2E_MYSQL_DSN}"
    "ANALYTICS_E2E_API_BIN=${ANALYTICS_E2E_API_BIN}"
    "ANALYTICS_E2E_WORKER_BIN=${ANALYTICS_E2E_WORKER_BIN}"
    "ANALYTICS_E2E_ADMIN_USERNAME=${ANALYTICS_E2E_ADMIN_USERNAME}"
    "ANALYTICS_E2E_ADMIN_PASSWORD=${ANALYTICS_E2E_ADMIN_PASSWORD}"
    "ANALYTICS_E2E_ADMIN_PASSWORD_HASH=${ANALYTICS_E2E_ADMIN_PASSWORD_HASH}"
    "ANALYTICS_E2E_CREATOR_PASSWORD=${ANALYTICS_E2E_CREATOR_PASSWORD}"
    "ANALYTICS_E2E_FRONTEND_DIR=${ANALYTICS_E2E_FRONTEND_DIR}"
    "ANALYTICS_E2E_BROWSER_SCRIPT=${ANALYTICS_E2E_BROWSER_SCRIPT}"
    "ANALYTICS_E2E_BROWSER_GENERATOR=${ANALYTICS_E2E_BROWSER_GENERATOR}"
    "ANALYTICS_E2E_BROWSER_TASK=${ANALYTICS_E2E_BROWSER_TASK}"
    "ANALYTICS_E2E_CHILD_PGID_FILE=${ANALYTICS_E2E_CHILD_PGID_FILE}"
    "MINIO_ENDPOINT=${MINIO_ENDPOINT}"
    "MINIO_PUBLIC_ENDPOINT=${MINIO_PUBLIC_ENDPOINT:-${MINIO_ENDPOINT}}"
    "MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}"
    "MINIO_SECRET_KEY=${MINIO_SECRET_KEY}"
    "MINIO_REGION=${MINIO_REGION:-us-east-1}"
    "MINIO_USE_SSL=${MINIO_USE_SSL:-false}"
    "MINIO_PUBLIC_USE_SSL=${MINIO_PUBLIC_USE_SSL:-false}"
    "CONTENT_ENCRYPTION_KEY_V1=${CONTENT_ENCRYPTION_KEY_V1}"
    "SHARE_ENCRYPTION_KEY_V1=${SHARE_ENCRYPTION_KEY_V1}"
  )
  if [[ -n "${ANALYTICS_E2E_FORCE_FAILURE_AFTER_START:-}" ]]; then
    test_environment+=("ANALYTICS_E2E_FORCE_FAILURE_AFTER_START=${ANALYTICS_E2E_FORCE_FAILURE_AFTER_START}")
  fi
  exec env -i "${test_environment[@]}" go test ./integration -run '^TestAnalyticsE2E$' -count=1 -v
) &
TEST_PID=$!
set +e
wait "${TEST_PID}"
test_status=$?
set -e
TEST_PID=""
if [[ "${test_status}" -ne 0 ]]; then
  exit "${test_status}"
fi

echo "analytics E2E PASS (isolated database removed on exit)"
