#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
FRONTEND_DIR="${PROJECT_ROOT}/frontend"

if ! command -v npm >/dev/null 2>&1; then
  echo "错误：未找到 npm，请先安装 Node.js 和 npm。" >&2
  exit 1
fi

if [[ ! -d "${FRONTEND_DIR}/node_modules" ]]; then
  echo "错误：前端依赖尚未安装。请先在项目根目录运行 make bootstrap。" >&2
  exit 1
fi

echo "正在启动前端服务：http://127.0.0.1:5173"
cd "${FRONTEND_DIR}"
exec npm run dev -- "$@"
