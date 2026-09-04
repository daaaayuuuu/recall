#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
APP_ENV_FILE="${GAME_GEN_ENV_FILE:-${PROJECT_ROOT}/.env}"
APP_ENV_EXAMPLE="${PROJECT_ROOT}/.env.example"
DEV_ENV_FILE="${GAME_GEN_DEV_ENV_FILE:-${PROJECT_ROOT}/deploy/compose/dev/.env}"
DEV_ENV_EXAMPLE="${PROJECT_ROOT}/deploy/compose/dev/.env.example"

fail() {
  echo "错误：$*" >&2
  exit 1
}

require_command() {
  local command_name="$1"
  local install_hint="$2"

  command -v "${command_name}" >/dev/null 2>&1 || fail "未找到 ${command_name}。${install_hint}"
}

ensure_config() {
  local example_file="$1"
  local target_file="$2"

  [[ -f "${example_file}" ]] || fail "配置模板不存在：${example_file}"
  if [[ -f "${target_file}" ]]; then
    echo "  保留已有配置：${target_file}"
    return
  fi

  cp "${example_file}" "${target_file}"
  echo "  已创建本地配置：${target_file}"
}

echo "[1/4] 检查本机开发工具"
require_command git "请先安装 Git。"
require_command go "请安装 Go 1.26.x。"
require_command node "请安装 Node.js 24.x。"
require_command npm "请安装随 Node.js 提供的 npm 11.x。"
require_command docker "请安装并启动 Docker Desktop。"

go_version="$(go env GOVERSION)"
case "${go_version}" in
  go1.26.*) ;;
  *) fail "当前 Go 版本为 ${go_version}，项目要求 Go 1.26.x（推荐 1.26.5）。" ;;
esac

node_version="$(node --version)"
node_major="$(node -p 'process.versions.node.split(".")[0]')"
[[ "${node_major}" == "24" ]] || fail "当前 Node.js 版本为 ${node_version}，项目要求 Node.js 24.x（推荐 24.18.0）。"

npm_version="$(npm --version)"
npm_major="${npm_version%%.*}"
[[ "${npm_major}" == "11" ]] || fail "当前 npm 版本为 ${npm_version}，项目要求 npm 11.x。"

docker compose version >/dev/null 2>&1 || fail "未找到支持 docker compose 命令的 Compose 插件。"
docker info >/dev/null 2>&1 || fail "Docker Engine 未就绪。请启动 Docker Desktop，等待引擎启动完成后重试。"

echo "  Go ${go_version#go} / Node.js ${node_version#v} / npm ${npm_version}"

echo "[2/4] 创建缺失的本地配置"
ensure_config "${APP_ENV_EXAMPLE}" "${APP_ENV_FILE}"
ensure_config "${DEV_ENV_EXAMPLE}" "${DEV_ENV_FILE}"

echo "[3/4] 安装锁定版本的项目依赖"
(
  cd "${PROJECT_ROOT}/backend"
  go mod download
)
(
  cd "${PROJECT_ROOT}/frontend"
  npm ci
)

echo "[4/4] 更新并准备 Docker 开发环境"
bash "${SCRIPT_DIR}/prepare-dev.sh" --pull

echo
echo "开发环境初始化完成。接下来打开两个终端运行："
echo "  ./scripts/start-backend.sh"
echo "  ./scripts/start-frontend.sh"
echo
echo "如需使用管理后台，请按 README_DEVELOPMENT.md 生成本地管理员密码哈希。"
