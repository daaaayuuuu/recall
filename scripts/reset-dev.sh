#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${PROJECT_ROOT}/deploy/compose/dev/compose.yaml"
DEV_ENV_FILE="${GAME_GEN_DEV_ENV_FILE:-${PROJECT_ROOT}/deploy/compose/dev/.env}"
APP_ENV_FILE="${GAME_GEN_ENV_FILE:-${PROJECT_ROOT}/.env}"
ASSUME_YES=false
DRY_RUN=false

usage() {
  cat <<'EOF'
用法：./scripts/reset-dev.sh [--yes] [--dry-run]

彻底重置本地开发数据，然后重新创建基础设施：
  1. 停止开发环境的 MySQL 和 MinIO
  2. 删除对应 Docker 命名卷（数据库表和对象文件）
  3. 重新启动并等待基础设施健康
  4. 重新创建私有 Bucket
  5. 从零执行全部数据库迁移

选项：
  --yes      跳过交互确认，适用于明确知晓后果的自动化场景
  --dry-run  只显示将执行的命令，不修改任何数据
  -h, --help 显示帮助

可选环境变量：
  GAME_GEN_DEV_ENV_FILE  指定开发 Compose 环境变量文件
  GAME_GEN_ENV_FILE      指定应用环境变量文件
EOF
}

fail() {
  echo "错误：$*" >&2
  exit 1
}

read_dotenv_value() {
  local key="$1"
  local file="$2"
  local value

  value="$(sed -nE "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*(.*)$/\1/p" "${file}" | tail -n 1)"
  value="${value%$'\r'}"
  if [[ "${value}" == \"*\" && "${value}" == *\" ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "${value}" == \'*\' && "${value}" == *\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "${value}"
}

print_command() {
  printf '  '
  printf '%q ' "$@"
  printf '\n'
}

run() {
  print_command "$@"
  if [[ "${DRY_RUN}" == true ]]; then
    return
  fi
  "$@"
}

run_migrations() {
  echo "  使用 ${APP_ENV_FILE} 执行 go run ./cmd/migrate -command up"
  if [[ "${DRY_RUN}" == true ]]; then
    return
  fi
  (
    set -a
    # shellcheck disable=SC1090
    source "${APP_ENV_FILE}"
    set +a
    cd "${PROJECT_ROOT}/backend"
    go run ./cmd/migrate -command up
  )
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes)
      ASSUME_YES=true
      ;;
    --dry-run)
      DRY_RUN=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "错误：未知参数 $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

command -v docker >/dev/null 2>&1 || fail "未找到 Docker，请先安装并启动 Docker Desktop。"
command -v go >/dev/null 2>&1 || fail "未找到 Go，无法重新执行数据库迁移。"
[[ -f "${COMPOSE_FILE}" ]] || fail "开发 Compose 文件不存在：${COMPOSE_FILE}"
[[ -f "${DEV_ENV_FILE}" ]] || fail "开发 Compose 配置不存在：${DEV_ENV_FILE}"
[[ -f "${APP_ENV_FILE}" ]] || fail "应用配置不存在：${APP_ENV_FILE}"

app_environment="$(read_dotenv_value APP_ENVIRONMENT "${APP_ENV_FILE}")"
[[ "${app_environment}" == "development" ]] || fail "只允许重置 APP_ENVIRONMENT=development，当前值为 ${app_environment:-<空>}。"

compose_project="$(read_dotenv_value COMPOSE_PROJECT_NAME "${DEV_ENV_FILE}")"
compose_project="${compose_project:-game-gen-dev}"
[[ "${compose_project}" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || fail "Compose 项目名格式不安全：${compose_project}"
case "${compose_project}" in
  *[dD][eE][vV]*) ;;
  *) fail "只允许删除名称包含 dev 的 Compose 项目，当前为 ${compose_project}。" ;;
esac

compose=(docker compose --env-file "${DEV_ENV_FILE}" -f "${COMPOSE_FILE}")

echo "即将彻底重置本地开发项目：${compose_project}"
echo "将永久删除："
echo "  - MySQL 中的所有表和数据"
echo "  - MinIO 中的所有游戏、头像、素材和生成产物"
echo "不会删除源码、${APP_ENV_FILE}、依赖目录或生产环境数据。"

if [[ "${ASSUME_YES}" != true && "${DRY_RUN}" != true ]]; then
  if [[ ! -t 0 ]]; then
    fail "非交互环境必须显式传入 --yes。"
  fi
  expected="RESET ${compose_project}"
  read -r -p "请输入“${expected}”继续：" confirmation
  [[ "${confirmation}" == "${expected}" ]] || fail "确认内容不匹配，已取消重置。"
fi

echo
echo "[1/4] 删除开发容器、网络和命名卷"
run "${compose[@]}" down --volumes --remove-orphans

echo
echo "[2/4] 从空卷启动 MySQL 和 MinIO"
run "${compose[@]}" up -d --wait

echo
echo "[3/4] 重新创建私有对象存储 Bucket"
run "${compose[@]}" --profile setup run --rm minio-init

echo
echo "[4/4] 从零执行数据库迁移"
run_migrations

if [[ "${DRY_RUN}" == true ]]; then
  echo
  echo "Dry run 完成，未修改任何数据。"
  exit 0
fi

echo
echo "本地开发数据已完全重置，基础设施和空数据库已准备完成。"
echo "旧的制作方、管理员会话和公开游玩会话均已失效。"
echo "接下来可运行："
echo "  bash scripts/start-backend.sh"
echo "  bash scripts/start-frontend.sh"
