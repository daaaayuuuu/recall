#!/usr/bin/env bash

set -Eeuo pipefail
set +x

required=(
  ANALYTICS_E2E_BROWSER_TASK
  ANALYTICS_E2E_FRONTEND_URL
  ANALYTICS_E2E_LOGIN_ID
  ANALYTICS_E2E_CREATOR_PASSWORD
  ANALYTICS_E2E_SHARE_URL
  ANALYTICS_E2E_ADMIN_USERNAME
  ANALYTICS_E2E_ADMIN_PASSWORD
  ANALYTICS_E2E_BROWSER_FORBIDDEN
  ANALYTICS_E2E_BROWSER_GENERATOR
)
for name in "${required[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    echo "browser E2E is missing required private environment" >&2
    exit 2
  fi
done

BROWSER_TASK_ACTIVE=1
cleanup_task_space() {
  if [[ "${BROWSER_TASK_ACTIVE}" != "1" ]]; then
    return
  fi
  "${ANALYTICS_E2E_BROWSER_GENERATOR}" cleanup >/dev/null 2>&1 || true
}
trap cleanup_task_space EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${ANALYTICS_E2E_BROWSER_GENERATOR}" flow

# Skill policy requires completion to be a separate, final ego-browser invocation.
"${ANALYTICS_E2E_BROWSER_GENERATOR}" cleanup
BROWSER_TASK_ACTIVE=0
