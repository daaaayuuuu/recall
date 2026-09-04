#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BACKEND_DIR="${PROJECT_ROOT}/backend"
BIN_DIR="${PROJECT_ROOT}/tmp/dev-bin"
ENV_FILE="${GAME_GEN_ENV_FILE:-${PROJECT_ROOT}/.env}"
MODE="all"
SKIP_PREPARE="${GAME_GEN_SKIP_PREPARE:-false}"

usage() {
  cat <<'EOF'
用法：./scripts/start-backend.sh [all|api|worker] [--skip-prepare]

  all     同时启动 API 和 Worker（默认）
  api     仅启动 API
  worker  仅启动 Worker

默认会先启动 MySQL、MinIO，创建 Bucket 并执行数据库迁移。
使用 --skip-prepare 可跳过该步骤，适用于基础设施已经由其他方式管理的场景。

可通过 GAME_GEN_ENV_FILE、GAME_GEN_DEV_ENV_FILE 指定配置文件，
或通过 GAME_GEN_SKIP_PREPARE=true 跳过环境准备。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    all|api|worker)
      MODE="$1"
      ;;
    --skip-prepare)
      SKIP_PREPARE=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "错误：未知参数 $1。" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if ! command -v go >/dev/null 2>&1; then
  echo "错误：未找到 go，请先安装 Go 工具链。" >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "错误：环境变量文件不存在：${ENV_FILE}" >&2
  echo "请先在项目根目录运行 cp .env.example .env。" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

case "${SKIP_PREPARE}" in
  true|1|yes)
    echo "已跳过本地基础设施准备。"
    ;;
  false|0|no|'')
    bash "${SCRIPT_DIR}/prepare-dev.sh"
    ;;
  *)
    echo "错误：GAME_GEN_SKIP_PREPARE 必须为 true 或 false。" >&2
    exit 2
    ;;
esac

address_port() {
  local address="$1"
  local port="${address##*:}"

  port="${port%]}"
  if [[ ! "${port}" =~ ^[0-9]+$ ]]; then
    echo "错误：无法从监听地址 ${address} 解析端口。" >&2
    exit 1
  fi

  printf '%s' "${port}"
}

check_port_available() {
  local service="$1"
  local address="$2"
  local port
  local listeners

  port="$(address_port "${address}")"

  if ! command -v lsof >/dev/null 2>&1; then
    return
  fi

  listeners="$(lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "${listeners}" ]]; then
    echo "错误：${service} 无法启动，端口 ${port} 已被占用：" >&2
    printf '%s\n' "${listeners}" >&2
    echo "请先停止占用进程，或修改 ${ENV_FILE} 中对应的监听地址。" >&2
    exit 1
  fi
}

build_service() {
  local name="$1"

  mkdir -p "${BIN_DIR}"
  echo "正在构建 ${name}..."
  (
    cd "${BACKEND_DIR}"
    go build -o "${BIN_DIR}/${name}" "./cmd/${name}"
  )
}

start_api() {
  echo "正在启动 API：http://127.0.0.1${HTTP_ADDRESS:-:8080}"
  exec "${BIN_DIR}/api"
}

start_worker() {
  echo "正在启动 Worker 健康检查：http://127.0.0.1${WORKER_HEALTH_ADDRESS:-:8081}"
  exec "${BIN_DIR}/worker"
}

case "${MODE}" in
  api)
    check_port_available "API" "${HTTP_ADDRESS:-:8080}"
    build_service api
    start_api
    ;;
  worker)
    check_port_available "Worker" "${WORKER_HEALTH_ADDRESS:-:8081}"
    build_service worker
    start_worker
    ;;
esac

API_PORT="$(address_port "${HTTP_ADDRESS:-:8080}")"
WORKER_PORT="$(address_port "${WORKER_HEALTH_ADDRESS:-:8081}")"
if [[ "${API_PORT}" == "${WORKER_PORT}" ]]; then
  echo "错误：API 和 Worker 配置了相同的端口 ${API_PORT}。" >&2
  exit 1
fi

check_port_available "API" "${HTTP_ADDRESS:-:8080}"
check_port_available "Worker" "${WORKER_HEALTH_ADDRESS:-:8081}"

build_service api
build_service worker

API_PID=""
WORKER_PID=""

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM

  for pid in "${API_PID}" "${WORKER_PID}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
    fi
  done

  for _ in {1..50}; do
    if ! kill -0 "${API_PID}" 2>/dev/null && ! kill -0 "${WORKER_PID}" 2>/dev/null; then
      break
    fi
    sleep 0.1
  done

  for pid in "${API_PID}" "${WORKER_PID}"; do
    if [[ -n "${pid}" ]]; then
      if kill -0 "${pid}" 2>/dev/null; then
        kill -KILL "${pid}" 2>/dev/null || true
      fi
      wait "${pid}" 2>/dev/null || true
    fi
  done

  exit "${exit_code}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

start_api &
API_PID=$!
start_worker &
WORKER_PID=$!

echo "后台服务已启动。按 Ctrl+C 可同时停止 API 和 Worker。"

while true; do
  if ! kill -0 "${API_PID}" 2>/dev/null; then
    set +e
    wait "${API_PID}"
    exit_code=$?
    set -e
    exit "${exit_code}"
  fi

  if ! kill -0 "${WORKER_PID}" 2>/dev/null; then
    set +e
    wait "${WORKER_PID}"
    exit_code=$?
    set -e
    exit "${exit_code}"
  fi

  sleep 1
done
