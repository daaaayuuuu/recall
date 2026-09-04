# game-gen 开发者本地启动指南

本文面向第一次参与 `game-gen` 开发的工程师。按“首次启动”一节操作，可以在本地运行前端、API、Worker，以及 MySQL 和 MinIO。

> 默认开发方式：MySQL 和 MinIO 由 Docker Compose 运行；前端、API 和 Worker 在本机运行，便于修改代码和断点调试。

## 1. 必须安装的环境

| 工具 | 项目使用版本 | 用途 |
|---|---:|---|
| Git | 2.x | 拉取和管理代码 |
| Go | 1.26.0 或更高的 1.26.x；CI 使用 1.26.5 | 编译和运行 API、Worker、迁移工具 |
| Node.js | 24.x；CI 使用 24.18.0 | 运行和构建 Vue 前端 |
| npm | 随 Node.js 安装；CI 使用 11.x | 安装前端依赖 |
| Docker Desktop | 建议使用当前稳定版 | 运行本地基础设施 |
| Docker Compose | 支持 `docker compose` 命令的插件，通常包含在 Docker Desktop 中 | 管理 MySQL 和 MinIO |
| Make | macOS/Linux 通常可直接安装 | 执行项目统一命令 |
| Bash | 3.2 或更高版本 | 运行 `scripts/` 中的开发脚本；macOS 系统自带版本即可 |

macOS 开发者建议直接安装 Docker Desktop。Windows 开发者建议使用 Docker Desktop + WSL2，并在 WSL2 中运行项目命令。

安装后先确认工具可用：

```bash
git --version
go version
node --version
npm --version
docker --version
docker compose version
make --version
```

执行后还需要启动 Docker Desktop，并等待其显示 Docker Engine 已就绪。
仓库同时提供 `.go-version` 和 `.nvmrc`，使用 asdf、mise、goenv 或 nvm 等版本管理器时，可以直接选择与 CI 一致的 Go 1.26.5 和 Node.js 24.18.0。

## 2. 首次拉取后的快速启动

以下命令都应在 `game-gen` 项目根目录执行。

### 第一步：拉取代码

```bash
git clone <仓库地址>
cd game-gen
```

### 第二步：一键初始化开发环境

```bash
make dev-setup
```

该命令可安全重复执行，并自动完成：

- 校验 Go 1.26.x、Node.js 24.x、npm 11.x、Docker 和 Compose。
- 仅在文件不存在时，从示例创建两份本地配置，不覆盖已有设置。
- 下载 Go 模块并使用 `npm ci` 安装锁定版本的前端依赖。
- 拉取项目锁定版本的 MySQL、MinIO 和 MinIO Client 镜像。
- 启动基础设施、创建 Bucket，并执行数据库迁移。

首次运行可能需要几分钟。两份本地配置用途不同：

- 根目录 `.env`：API 和 Worker 使用的应用配置。
- `deploy/compose/dev/.env`：MySQL 和 MinIO 使用的 Docker Compose 配置。

默认示例值可以直接用于个人本地开发，但不得用于共享环境或生产环境。

### 第三步：按需配置本地管理员密码

生成 Argon2id 密码哈希：

```bash
make hash-password PASSWORD='你准备使用的本地管理员密码'
```

复制命令输出，将根目录 `.env` 中的 `ADMIN_PASSWORD_HASH` 替换为完整哈希。哈希包含 `$`，在 `.env` 中必须使用单引号包裹：

```dotenv
ADMIN_USERNAME=admin
ADMIN_PASSWORD_HASH='$argon2id$...'
```

如果暂时不需要管理员登录，可以稍后配置，但不要使用示例中的占位值尝试登录。

### 第四步：启动后端

```bash
./scripts/start-backend.sh
```

后端脚本会自动完成以下准备工作，然后启动 API 和 Worker：

- 检查 Docker Engine 是否已经就绪。
- 启动并等待 MySQL、MinIO 健康。
- 创建项目所需的四个私有 MinIO Bucket。
- 执行尚未应用的数据库迁移。
- 构建并同时启动 API、Worker。

首次运行时 Docker 会自动下载项目锁定版本的镜像，因此可能需要等待几分钟。后台脚本支持只运行一个进程：

```bash
./scripts/start-backend.sh api
./scripts/start-backend.sh worker
```

如果基础设施和迁移已经由 IDE 或其他方式管理，可以显式跳过准备步骤：

```bash
./scripts/start-backend.sh api --skip-prepare
```

脚本准备的本地服务为：

- MySQL：`127.0.0.1:3306`
- MinIO API：`http://127.0.0.1:9000`
- MinIO Console：`http://127.0.0.1:9001`

如需主动拉取 Compose 中锁定版本的开发镜像并准备环境，可以单独运行：

```bash
make dev-env-update
```

后端启动和上述环境准备命令都会自动执行数据库迁移。可以单独检查迁移状态：

```bash
make migrate-status
```

当前仓库的全新数据库会从 `000001_initial_schema` 建立基础表，并依次增加模板素材槽位、行为事件、一次性邀请码、真实生成阶段和动态 AI 配置版本；全部迁移后状态应为版本 `6` 且 `dirty=false`。如果本地数据库创建时间较早，可只检查用户身份列名而不输出账号或凭据：

```bash
docker compose --env-file deploy/compose/dev/.env -f deploy/compose/dev/compose.yaml exec -T mysql sh -eu -c '
  export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
  mysql --user=root --database="$MYSQL_DATABASE" --batch --skip-column-names \
    -e "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '\''users'\'' AND COLUMN_NAME IN ('\''email'\'', '\''login_id'\'') ORDER BY COLUMN_NAME"
'
```

预期只输出 `login_id`。如果数据库已经登记过旧版 `000001`，但查询只出现旧 `email` 或缺少 `login_id`，不要继续运行应用并假设后续迁移会自动修复：迁移工具不会重放已经登记的 `000001`，素材槽位和行为事件迁移也不会改写旧用户结构。需要保留数据时应停止并单独设计、评审数据迁移；只有明确确认现有 MySQL/MinIO 开发数据都可删除后，才先预览再执行交互式重置：

```bash
bash scripts/reset-dev.sh --dry-run
make dev-reset
```

不要为跳过判断而使用非交互重置。

### 第五步：启动前端

保持后端终端运行，打开第二个终端：

```bash
./scripts/start-frontend.sh
```

两个脚本都使用 `Ctrl+C` 停止。停止后台脚本不会删除数据库或对象文件，也不会自动停止 Docker 基础设施，方便下次快速启动。需要停止基础设施时运行 `make dev-infra-down`。

### 第六步：确认服务正常

打开或请求以下地址：

| 服务 | 地址 | 正常表现 |
|---|---|---|
| 前端 | `http://127.0.0.1:5173` | 显示登录或应用页面 |
| API 存活检查 | `http://127.0.0.1:8080/health/live` | 返回成功状态 |
| API 就绪检查 | `http://127.0.0.1:8080/health/ready` | MySQL、MinIO 均正常 |
| Worker 就绪检查 | `http://127.0.0.1:8081/health/ready` | Worker 依赖均正常 |

## 3. 必须理解和检查的配置

### 3.1 根目录 `.env`

| 配置 | 是否必须 | 说明 |
|---|---|---|
| `MYSQL_DSN` | 是 | 必须和 Compose 中的数据库名、用户名及密码一致 |
| `MINIO_ENDPOINT` | 是 | API/Worker 连接 MinIO 的地址；本机默认 `127.0.0.1:9000` |
| `MINIO_PUBLIC_ENDPOINT` | 是 | 浏览器访问预签名素材的地址；本机保持 `127.0.0.1:9000` |
| `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | 是 | 必须和 `deploy/compose/dev/.env` 中的 MinIO 凭据一致 |
| `CONTENT_ENCRYPTION_KEY_V1` | 是 | Base64 编码的 32 字节密钥，用于加密游戏输入内容 |
| `SHARE_ENCRYPTION_KEY_V1` | 是 | 另一条 Base64 编码的 32 字节密钥，用于加密分享 Secret |
| `ADMIN_PASSWORD_HASH` | 管理后台必须 | 必须是通过项目命令生成的 Argon2id 哈希，不能填写明文密码 |
| `SERVICE_SURFACE` | 是 | 本地保持 `all`；Railway API 服务分别使用 `app` 和 `play` |
| `HTTP_ADDRESS` | 是 | API 监听地址，默认 `:8080` |
| `WORKER_HEALTH_ADDRESS` | 是 | Worker 健康检查地址，默认 `:8081` |
| `TRUST_PROXY_HEADERS` | 是 | 本地保持 `false`；仅在 Railway/Caddy 等可信代理后设为 `true` |
| `APP_BASE_URL` / `PLAY_BASE_URL` | 是 | 制作端与分享端的前端地址，本地默认 `http://127.0.0.1:5173` |
| `DYNAMIC_AI_CONFIG_ENABLED` | 推荐 | 为 `true` 时允许管理员发布运行时 AI 配置；紧急回退时可设为 `false` |
| `AI_CONFIG_ENCRYPTION_KEY_V1` | 动态配置必须 | 独立的 Base64 32 字节密钥，用于加密数据库中的 AI API Key |
| `AI_CONFIG_REFRESH_INTERVAL` | 否 | API/Worker 检查新配置的短缓存周期，默认 `2s` |

其余运行参数按用途分组如下；未写入 `.env` 时使用 `config/config.local.yaml` 或代码默认值，环境变量优先级最高：

| 分组 | 环境变量 | 说明 |
|---|---|---|
| 配置与静态文件 | `GAMEGEN_CONFIG`、`WEB_STATIC_DIR`、`PORT` | 指定 YAML 文件、统一镜像静态目录；Railway 的 `PORT` 只在未显式设置监听地址时覆盖 API/Worker 健康端口 |
| 基础运行 | `APP_ENVIRONMENT`、`LOG_LEVEL`、`WORKER_POLL_INTERVAL` | 环境只能是 `development/test/production`；控制日志和 Worker 轮询 |
| MinIO | `MINIO_REGION`、`MINIO_USE_SSL`、`MINIO_PUBLIC_USE_SSL` | 服务端连接与浏览器公开签名地址的区域及 TLS 开关 |
| 生成队列 | `GENERATION_LEASE_DURATION`、`GENERATION_MAX_EXECUTIONS` | Worker 租约与最大领取次数 |
| 文本润色 ENV 基线 | `AI_TEXT_PROVIDER`、`AI_TEXT_BASE_URL`、`DEEPSEEK_API_KEY`、`AI_TEXT_MODEL`、`AI_TEXT_TIMEOUT`、`AI_TEXT_MAX_OUTPUT_TOKENS` | 管理员尚未发布动态版本或动态配置关闭时使用；空 Key 保留原文 |
| 图片审核 ENV 基线 | `AI_IMAGE_MODERATION_PROVIDER`、`AI_IMAGE_MODERATION_BASE_URL`、`AI_IMAGE_MODERATION_API_KEY`、`AI_IMAGE_MODERATION_MODEL`、`AI_IMAGE_MODERATION_TIMEOUT`、`AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS` | 空 Key 跳过上传审核；非空 Key 时严格校验并失败关闭 |
| 图生图 ENV 基线 | `AI_IMAGE_TO_IMAGE_PROVIDER`、`AI_IMAGE_TO_IMAGE_BASE_URL`、`AI_IMAGE_TO_IMAGE_API_KEY`、`AI_IMAGE_TO_IMAGE_MODEL`、`AI_IMAGE_TO_IMAGE_QUALITY`、`AI_IMAGE_TO_IMAGE_TIMEOUT`、`AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES`、`AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES` | 空 Key 复用规范化源图；非空 Key 时 Worker 调用图片编辑服务 |

管理员页面只管理上述三类 AI 基线对应的运行时配置。数据库、MinIO、域名、监听地址、管理员凭据、内容/分享/AI 配置加密密钥、队列和日志仍必须通过 ENV/YAML/部署平台设置。

仓库提供的三条加密密钥只允许用于个人本地开发。如果需要生成独立密钥，应分别执行三次并写入三种不同用途：

```bash
openssl rand -base64 32
```

不要为已有数据的环境随意更换加密密钥，否则之前保存的游戏内容、分享信息或 AI API Key 将无法解密。三种用途不能使用同一条密钥。

AI 配置加密密钥同样不能与上述两条密钥复用。管理员页面、版本存储和紧急回退说明见 [`docs/DYNAMIC_AI_SETTINGS_IMPLEMENTATION_v0.1.md`](docs/DYNAMIC_AI_SETTINGS_IMPLEMENTATION_v0.1.md)。

### 3.2 `deploy/compose/dev/.env`

这个文件控制 Docker 中的服务和宿主机端口。修改数据库或 MinIO 凭据时，必须同步修改根目录 `.env` 中的对应连接配置。

默认会占用以下端口：

```text
3306  MySQL
9000  MinIO API
9001  MinIO Console
```

如果本机已有服务占用这些端口，可以在该文件中更换端口，然后同步更新根目录 `.env`。

### 3.3 前端 API 地址

本地默认不需要创建前端环境变量文件。Vite 会把 `/api` 和 `/api/v1` 转发到 `http://127.0.0.1:8080`。

如果修改了 API 端口，需要同时修改根目录 `.env` 的 `HTTP_ADDRESS`，以及 `frontend/vite.config.ts` 中两个代理项的 `target`。保持浏览器通过 Vite 同源代理访问 API，可以避免本地跨域问题。

### 3.4 Analytics Surface 装配

API 通过 `SERVICE_SURFACE=app|play|all` 选择以下装配；该写法表示三个可选值，不是环境变量的字面值。

| 进程 / `SERVICE_SURFACE` | Analytics 路由与记录职责 |
|---|---|
| API `app` | 挂载制作方上报和管理员查询；记录制作方私有 API 成功事件；不挂载公开上报 |
| API `play` | 挂载公开游玩上报；记录分享打开、开始游玩和公开前端事件；不挂载制作方或管理员路由 |
| API `all` | 本地开发使用，同时挂载 app/play 两侧且各一次 |
| Worker | 不使用 HTTP Surface；独立连接数据库并记录生成最终成功或失败 |

Analytics Recorder 在 API/Worker 中分别创建。`play` Surface 不需要为 Analytics 初始化管理员或制作方私有依赖，`app` Surface 也不会暴露公开游玩路由。

## 4. 项目模块说明

| 目录或模块 | 简短说明 |
|---|---|
| `frontend/` | Vue 3 + TypeScript + Vite 前端，包含认证、游戏管理、个人设置、分享游玩和管理页面 |
| `frontend/src/api/` | 前端 API 请求与数据类型 |
| `frontend/src/views/` | 页面级 Vue 组件 |
| `frontend/src/stores/` | Pinia 状态管理 |
| `backend/cmd/api/` | HTTP API 进程入口，负责认证、游戏、素材、任务、分享和公开游玩接口 |
| `backend/cmd/worker/` | 异步 Worker 入口，执行真实图生图、配置产物持久化和延迟删除任务 |
| `backend/cmd/migrate/` | 数据库迁移命令入口 |
| `backend/cmd/hash-password/` | 本地生成管理员 Argon2id 密码哈希的工具 |
| `backend/internal/auth/` | 用户 ID 注册、登录、会话、密码和头像 |
| `backend/internal/games/` | 游戏、版本、输入内容和图片素材 |
| `backend/internal/generation/` | 游戏生成任务、进度、租约、取消和重试 |
| `backend/internal/sharing/` | 分享链接、公开访问和游玩会话 |
| `backend/internal/worker/` | Worker 轮询和任务调度循环 |
| `backend/internal/platform/` | 配置、数据库、对象存储、日志、安全和通用基础能力 |
| `backend/db/migrations/` | 按版本执行的 MySQL 数据库结构变更 |
| `contracts/openapi/` | 前后端共享的 HTTP API 契约 |
| `contracts/game-config/` | 生成游戏配置的 JSON Schema |
| `deploy/compose/dev/` | 本地 MySQL 和 MinIO 环境 |
| `deploy/compose/prod/` | 单机生产部署 Compose 配置 |
| `deploy/docker/` | 统一业务镜像的 Dockerfile |
| `scripts/` | 本地前端和后端启动脚本 |
| `docs/` | PRD、技术设计、技术选型和 MVP 完成状态 |

本地运行时的数据流可以简化理解为：

```text
浏览器 → Vite 前端（5173）→ Go API（8080）
                                ├─ MySQL（业务数据与任务队列）
                                └─ MinIO（头像、素材和生成结果）

Go Worker（健康检查 8081）→ MySQL 领取任务 → MinIO 写入或清理对象
```

当前前端基线需要特别注意：新建游戏使用 `love-journey@1.1.0`，拆信密码必须是 4 位数字，回忆流程固定为 5 段；“我的游戏”卡片固定只有试玩、修改、分享、删除四种操作，不存在详情页或从列表恢复生成运行的入口。创建和修改共用 `HomeView`，提交后进入独立生成路由；生成界面的 8%/68%/88%/100% 是按真实任务阶段硬编码的展示映射，不是后端模型实时百分比。

## 5. 常用开发命令

```bash
# 安装或恢复全部依赖
make bootstrap

# 全新 clone 后的一键初始化
make dev-setup

# 启动、停止本地基础设施
make dev-prepare
make dev-env-update
make dev-infra-up
make dev-infra-down

# 破坏性重置；仅在确认全部本地数据可删除并先 dry-run 后使用
make dev-reset

# 数据库迁移
make migrate-up
make migrate-status

# 分别启动进程
make api
make worker
make frontend

# 格式化、静态检查、测试和构建
make fmt
make lint
make test
make build

# 校验开发和生产 Compose 配置
make compose-check
```

提交代码前至少执行：

```bash
make lint
make test
make build
```

前端页面还应至少检查 `320×800` 手机视口和 `1440×900` 桌面视口。

### Analytics 本地验收

使用 `SERVICE_SURFACE=all` 启动本地 API、Worker 和前端，配置可用管理员账号后，可从制作方页面完成创建/生成/分享，再从公开链接完成开始、完成和重玩，最后访问：

```text
http://127.0.0.1:5173/admin/behavior-events
```

确认页面能按事件名、Creator ID、登录 ID、Game ID、来源、开始时间和结束时间筛选，默认显示 50 条并可继续加载；详情只能显示事件契约允许的属性。普通 `make test` 会安全跳过需要真实基础设施和浏览器的专项 E2E。已安装并可使用 `ego-browser` 时，可显式执行隔离验收：

```bash
ANALYTICS_E2E=1 ./scripts/test-analytics-e2e.sh
```

脚本使用随机 loopback 端口、一次性 MySQL schema、精确 QA MinIO 前缀和独立浏览器 task-space，并在正常退出、失败或外部 TERM 时清理本轮资源；它不会调用 `dev-reset`。执行前仍应保存自己的未提交工作，并保持现有开发服务由各自终端管理。

## 6. 本地调试

后端调试入口分别是：

- API：`backend/cmd/api/main.go`
- Worker：`backend/cmd/worker/main.go`

在 GoLand、VS Code 等 IDE 中调试时，将工作目录设为 `backend/`，程序入口选择对应的 `cmd`，并加载项目根目录 `.env` 中的环境变量。API 和 Worker 需要使用两个独立的调试进程。

启动 IDE 调试进程前先在项目根目录运行：

```bash
make dev-prepare
```

该命令会准备基础设施并执行迁移，但不会占用 API、Worker 的调试端口。

前端使用 Vite 开发服务器，修改 Vue/TypeScript/CSS 后会自动热更新：

```bash
./scripts/start-frontend.sh
```

浏览器请求会通过 Vite 代理访问本机 API，因此正常开发时不需要额外配置跨域。

## 7. 常见问题

### 提示端口已被占用

API 默认使用 `8080`，Worker 默认使用 `8081`，前端默认使用 `5173`。先停止之前启动的进程，或修改对应配置。macOS/Linux 可以查看端口占用：

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
lsof -nP -iTCP:8081 -sTCP:LISTEN
lsof -nP -iTCP:5173 -sTCP:LISTEN
```

### API 就绪检查失败

依次确认：

1. Docker Desktop 已启动。
2. `docker compose --env-file deploy/compose/dev/.env -f deploy/compose/dev/compose.yaml ps` 中服务处于健康状态。
3. 根目录 `.env` 的 MySQL 和 MinIO 配置与 Compose 配置一致。
4. 已执行 `make dev-prepare`，或正常完成过后端启动脚本的准备阶段。

### 图片上传或预览失败

确认 MinIO 健康，并检查是否已通过 `make dev-infra-up` 创建四个 Bucket。浏览器需要能访问根目录 `.env` 中的 `MINIO_PUBLIC_ENDPOINT`。

### 前端提示依赖未安装

在项目根目录重新执行：

```bash
make bootstrap
```

### 需要清空本地数据重新开始

以下命令会永久删除本地 MySQL 数据和 MinIO 对象，然后自动重建基础设施、Bucket 和数据库表：

```bash
bash scripts/reset-dev.sh
```

脚本会要求输入 `RESET game-gen-dev` 二次确认。先用 `bash scripts/reset-dev.sh --dry-run` 查看操作范围；仅在确认不需要保留任何本地数据时执行交互式重置，不要在日常恢复中跳过确认。

## 8. 进一步阅读

- 产品范围：[`docs/PRD_MVP_v0.1.md`](docs/PRD_MVP_v0.1.md)
- 技术设计：[`docs/TECHNICAL_DESIGN_MVP_v0.1.md`](docs/TECHNICAL_DESIGN_MVP_v0.1.md)
- 当前完成情况：[`docs/MVP_IMPLEMENTATION_STATUS.md`](docs/MVP_IMPLEMENTATION_STATUS.md)
- 本地基础设施细节：[`deploy/compose/dev/README.md`](deploy/compose/dev/README.md)
- API 契约：[`contracts/openapi/openapi.yaml`](contracts/openapi/openapi.yaml)
