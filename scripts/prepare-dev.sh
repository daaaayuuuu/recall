#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${PROJECT_ROOT}/deploy/compose/dev/compose.yaml"
DEV_ENV_FILE="${GAME_GEN_DEV_ENV_FILE:-${PROJECT_ROOT}/deploy/compose/dev/.env}"
APP_ENV_FILE="${GAME_GEN_ENV_FILE:-${PROJECT_ROOT}/.env}"
PULL_IMAGES=false

usage() {
  cat <<'EOF'
用法：./scripts/prepare-dev.sh [--pull]

准备本地开发所需环境：
  1. 检查 Docker Engine 和本地配置
  2. 启动并等待 MySQL、MinIO 健康
  3. 创建项目所需的私有 MinIO Bucket
  4. 执行尚未应用的数据库迁移

选项：
  --pull     先拉取 Compose 中锁定版本的开发镜像
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

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pull)
      PULL_IMAGES=true
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

command -v docker >/dev/null 2>&1 || fail "未找到 Docker，请先安装 Docker Desktop。"
docker compose version >/dev/null 2>&1 || fail "未找到支持 docker compose 命令的 Compose 插件。"
command -v go >/dev/null 2>&1 || fail "未找到 Go，无法执行数据库迁移。"
[[ -f "${COMPOSE_FILE}" ]] || fail "开发 Compose 文件不存在：${COMPOSE_FILE}"
[[ -f "${DEV_ENV_FILE}" ]] || fail "开发 Compose 配置不存在：${DEV_ENV_FILE}。请先复制 deploy/compose/dev/.env.example。"
[[ -f "${APP_ENV_FILE}" ]] || fail "应用配置不存在：${APP_ENV_FILE}。请先复制根目录 .env.example。"

set -a
# shellcheck disable=SC1090
source "${APP_ENV_FILE}"
set +a

[[ "${APP_ENVIRONMENT:-}" == "development" ]] || fail "本地准备脚本只允许 APP_ENVIRONMENT=development。"

if ! docker info >/dev/null 2>&1; then
  fail "Docker Engine 未就绪。请启动 Docker Desktop，等待引擎启动完成后重试。"
fi

compose=(docker compose --env-file "${DEV_ENV_FILE}" -f "${COMPOSE_FILE}")

if [[ "${PULL_IMAGES}" == true ]]; then
  echo "[1/4] 拉取项目锁定版本的开发镜像"
  "${compose[@]}" --profile setup pull
  step_offset=1
else
  step_offset=0
fi

echo "[$((step_offset + 1))/$((step_offset + 3))] 启动 MySQL 和 MinIO"
"${compose[@]}" up -d --wait mysql minio

echo "[$((step_offset + 2))/$((step_offset + 3))] 创建 MinIO 私有 Bucket"
"${compose[@]}" --profile setup run --rm minio-init

echo "[$((step_offset + 3))/$((step_offset + 3))] 应用数据库迁移"
(
  cd "${PROJECT_ROOT}/backend"
  go run ./cmd/migrate -command up
)

echo "本地开发依赖已就绪。"
