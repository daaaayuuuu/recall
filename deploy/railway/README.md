# Railway 部署

标准方案在一个 Railway Project 中运行 6 个服务。MySQL 和 MinIO 使用本项目指定的官方 Docker 镜像，不使用 Railway MySQL 模板或 Railway Bucket；持久数据仍必须挂载 Railway Volume。

Railway 试用额度可能限制可创建的服务数量。此时可以只保留 `mysql`、`minio`、`game-gen-app`、`game-gen-play` 四个长期服务：`minio-init` 初始化完成后删除，并把 Worker 临时合并到 `game-gen-app` 容器。下文仍先描述正式环境推荐的独立 Worker 方案，并在业务服务章节给出试用环境配置。

Railway Public Networking 负责公网域名、TLS 和转发，因此这里不部署 Caddy。现有 Caddy/Compose 文件继续只服务于单机 Linux 部署。

| 服务 | 来源 / 启动命令 | 公网 | 持久化 |
| --- | --- | --- | --- |
| `mysql` | `mysql:8.4.10` | 无 | `/var/lib/mysql` |
| `minio` | `minio/minio:RELEASE.2025-09-07T16-13-09Z` | 仅 9000 资源域名 | `/data` |
| `game-gen-app` | 当前仓库，`/app/api` | 制作端域名 | 无 |
| `game-gen-play` | 当前仓库，`/app/api` | 游玩端域名 | 无 |
| `game-gen-worker` | 当前仓库，`/app/worker` | 无 | 无 |
| `minio-init` | 当前仓库的初始化镜像 | 无 | 无 |

`game-gen-app` 与 `game-gen-play` 使用同一个业务镜像，但分别设置 `SERVICE_SURFACE=app` 和 `SERVICE_SURFACE=play`。前者不会挂载公开游玩 API/页面，后者不会挂载认证、制作、管理和私有分享 API/页面。`all` 只用于本地开发及现有单机 Compose。

## 1. 创建 MySQL

创建一个空服务并使用 Docker Image `mysql:8.4.10`，服务名必须设为 `mysql`。添加 Volume，挂载路径为 `/var/lib/mysql`，不要生成公网域名。

设置变量：

```dotenv
MYSQL_ROOT_PASSWORD=replace-with-a-long-random-root-password
MYSQL_DATABASE=gamegen
MYSQL_USER=gamegen
MYSQL_PASSWORD=replace-with-a-different-long-random-password
TZ=UTC
```

不要把 3306 暴露到公网。其他服务通过 `mysql.railway.internal:3306` 访问。

## 2. 创建 MinIO

创建一个空服务并使用 Docker Image `minio/minio:RELEASE.2025-09-07T16-13-09Z`，服务名必须设为 `minio`。添加 Volume，挂载路径为 `/data`。

启动命令（Railway 的自定义命令会替换镜像入口点，因此必须包含 `minio` 可执行文件）：

```text
minio server /data --address :9000 --console-address :9001
```

设置变量：

```dotenv
MINIO_ROOT_USER=gamegen-root
MINIO_ROOT_PASSWORD=replace-with-a-long-random-root-secret
MINIO_REGION=us-east-1
RAILWAY_RUN_UID=0
TZ=UTC
```

Railway Volume 初始由 root 拥有，而固定版本的 MinIO 镜像可能以非 root 用户运行；`RAILWAY_RUN_UID=0` 用于确保它能够初始化 `/data`。为 MinIO 生成一个公网域名或绑定 `assets.example.com`，目标端口选择 `9000`。不要为 9001 Console 创建公网入口。四个 Bucket 都保持私有，浏览器只使用短期预签名 URL。

## 3. 初始化 MinIO

从同一个 GitHub 仓库创建临时服务 `minio-init`，将 Railway Config File 设为：

```text
deploy/railway/minio-init.railway.toml
```

该配置文件会让初始化服务使用：

```text
deploy/railway/minio-init.Dockerfile
```

设置以下变量；这里的应用账号与 MinIO root 账号必须不同：

```dotenv
MINIO_ENDPOINT_URL=http://minio.railway.internal:9000
MINIO_ROOT_USER=${{minio.MINIO_ROOT_USER}}
MINIO_ROOT_PASSWORD=${{minio.MINIO_ROOT_PASSWORD}}
MINIO_ACCESS_KEY=gamegen-app
MINIO_SECRET_KEY=replace-with-a-long-random-application-secret
```

在 MinIO 已经启动后部署该服务。日志出现 `RECALL MinIO buckets and application account are ready.` 即完成；进程随后正常退出。脚本可以安全重复运行。确认成功后可以移除或停用这个临时服务，但要保存应用账号 Secret。

## 4. 创建三个业务服务

从同一个 GitHub 仓库分别创建 `game-gen-app`、`game-gen-play`、`game-gen-worker`。根目录 [`railway.toml`](../../railway.toml) 只固定使用 Dockerfile 构建器；三个服务都必须增加下面的构建变量，以使用统一业务镜像：

```dotenv
RAILWAY_DOCKERFILE_PATH=deploy/docker/app.Dockerfile
```

先给三个服务设置以下共享变量。也可以从 [`.env.example`](.env.example) 复制后替换占位值：

```dotenv
APP_ENVIRONMENT=production
APP_BASE_URL=https://app.example.com
PLAY_BASE_URL=https://play.example.com
LOG_LEVEL=info
MYSQL_DSN=gamegen:${{mysql.MYSQL_PASSWORD}}@tcp(mysql.railway.internal:3306)/gamegen?parseTime=true&loc=UTC&charset=utf8mb4&multiStatements=true
MINIO_ENDPOINT=minio.railway.internal:9000
MINIO_PUBLIC_ENDPOINT=assets.example.com
MINIO_ACCESS_KEY=gamegen-app
MINIO_SECRET_KEY=replace-with-the-minio-application-secret
MINIO_REGION=us-east-1
MINIO_USE_SSL=false
MINIO_PUBLIC_USE_SSL=true
```

`MINIO_PUBLIC_ENDPOINT` 只写主机名，不带 `https://`。不要设置 `PORT`、`HTTP_ADDRESS` 或 `WORKER_HEALTH_ADDRESS`；Railway 注入的 `PORT` 会自动成为当前进程的监听端口。

### game-gen-app

```text
Start Command:      /app/api
Pre-deploy Command: /app/migrate -command up -path /app/migrations
Healthcheck Path:   /health/ready
Replicas:           1
```

额外变量：

```dotenv
SERVICE_SURFACE=app
TRUST_PROXY_HEADERS=true
ADMIN_USERNAME=admin
ADMIN_PASSWORD_HASH=$argon2id$replace-with-generated-hash
CONTENT_ENCRYPTION_KEY_V1=replace-with-base64-32-byte-key
SHARE_ENCRYPTION_KEY_V1=replace-with-another-base64-32-byte-key
DYNAMIC_AI_CONFIG_ENABLED=true
AI_CONFIG_ENCRYPTION_KEY_V1=replace-with-a-third-base64-32-byte-key
AI_CONFIG_REFRESH_INTERVAL=2s
AI_IMAGE_MODERATION_PROVIDER=openai-compatible
AI_IMAGE_MODERATION_BASE_URL=https://your-provider.example/v1
AI_IMAGE_MODERATION_API_KEY=replace-with-vision-provider-secret
AI_IMAGE_MODERATION_MODEL=replace-with-vision-model
AI_IMAGE_MODERATION_TIMEOUT=20s
AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS=300
```

生成管理员密码哈希和三个相互独立的加密密钥：

```bash
make hash-password PASSWORD='replace-this-password'
openssl rand -base64 32
openssl rand -base64 32
openssl rand -base64 32
```

三条加密密钥分别用于内容、分享和动态 AI 配置，在产生数据后不得更换、复用或丢失。`AI_CONFIG_ENCRYPTION_KEY_V1` 必须在 App 与 Worker 中完全一致，Play 服务无需使用。图片审核配置可选：Key 为空时上传会跳过审核，正式开放上传前建议配置支持图片输入及 OpenAI-compatible `/chat/completions` 格式的视觉模型；Key 非空时其余供应商配置会被严格校验。审核供应商密钥只配置在 `game-gen-app`，不要复制到公开游玩服务或 Worker。将制作端域名绑定到此服务。

### game-gen-play

```text
Start Command:    /app/api
Healthcheck Path: /health/ready
Replicas:         1
```

额外变量：

```dotenv
SERVICE_SURFACE=play
TRUST_PROXY_HEADERS=true
```

该服务不需要管理员、内容加密、分享加密或图片审核 Secret。将游玩端域名绑定到此服务。

### game-gen-worker

```text
Start Command:    /app/worker
Healthcheck Path: /health/ready
Replicas:         1
```

额外变量：

```dotenv
WORKER_POLL_INTERVAL=5s
GENERATION_LEASE_DURATION=60s
GENERATION_MAX_EXECUTIONS=3
CONTENT_ENCRYPTION_KEY_V1=replace-with-the-same-base64-32-byte-key-as-app
DYNAMIC_AI_CONFIG_ENABLED=true
AI_CONFIG_ENCRYPTION_KEY_V1=replace-with-the-same-third-base64-32-byte-key-as-app
AI_CONFIG_REFRESH_INTERVAL=2s
```

不要给 Worker 生成公网域名。Worker 必须配置 `CONTENT_ENCRYPTION_KEY_V1`；启用动态配置时还必须使用与 App 相同的 `AI_CONFIG_ENCRYPTION_KEY_V1`。`AI_IMAGE_TO_IMAGE_*` 是动态配置尚未发布时的 ENV 基线，Key 为空会复用规范化源图。Worker 不需要 `SERVICE_SURFACE`、代理、管理员或分享加密变量。

### 试用环境：将 Worker 合并到 game-gen-app

如果试用额度不允许再创建 `game-gen-worker`，将 `game-gen-app` 的 Start Command 改为：

```text
/bin/sh -c '/app/worker & exec /app/api'
```

并给 `game-gen-app` 增加：

```dotenv
WORKER_HEALTH_ADDRESS=127.0.0.1:9091
WORKER_POLL_INTERVAL=5s
GENERATION_LEASE_DURATION=60s
GENERATION_MAX_EXECUTIONS=3
```

`WORKER_HEALTH_ADDRESS` 必须使用独立的本地端口，避免与 API 使用的 Railway `PORT` 冲突。该方式适合试用和内测；正式环境仍建议拆分 Worker，使 API 与后台任务能够独立重启和扩缩容。

## 5. 部署顺序和验证

首次部署按以下顺序进行：

1. 部署 `mysql` 并确认日志显示可以接受连接。
2. 部署 `minio`，确认 `/minio/health/live` 正常。
3. 运行一次 `minio-init`。
4. 部署 `game-gen-app`；它的 Pre-deploy Command 会执行数据库迁移。
5. 部署 `game-gen-play` 和 `game-gen-worker`。
6. 配置三个公网域名，并把最终 HTTPS 地址写回 `APP_BASE_URL`、`PLAY_BASE_URL` 和 `MINIO_PUBLIC_ENDPOINT`，然后重新部署业务服务。

验证：

```bash
curl --fail https://app.example.com/health/ready
curl --fail https://app.example.com/
curl --fail https://play.example.com/health/ready
curl --fail https://play.example.com/play/not-found
curl --fail https://assets.example.com/minio/health/live
```

还应确认服务边界：

```bash
curl --fail-with-body https://app.example.com/api/v1/public/unknown
curl --fail-with-body https://play.example.com/api/v1/auth/session
curl --fail-with-body https://play.example.com/app/games
```

后三个请求预期返回 `404`，因此命令会以非零状态退出。

### Analytics Surface 验证

`000003_behavior_events` 沿用 `game-gen-app` 现有 Pre-deploy Command 执行，不需要新增 Railway 服务、变量或启动命令。部署完成后，可用以下不携带 Session 的请求验证 Analytics 路由边界；每条 `test` 成功都表示实际状态码与右侧预期一致：

```bash
# app：管理员查询已挂载但未认证为 401；公开上报未挂载为 404
test "$(curl -sS -o /dev/null -w '%{http_code}' https://app.example.com/api/v1/admin/behavior-events)" = 401
test "$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H 'Origin: https://play.example.com' -d '{}' https://app.example.com/api/v1/public/play-sessions/current/events)" = 404

# play：制作方与管理员路由未挂载为 404；公开上报已挂载但无 Session 为 401
test "$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H 'Origin: https://app.example.com' -d '{}' https://play.example.com/api/v1/analytics/events)" = 404
test "$(curl -sS -o /dev/null -w '%{http_code}' https://play.example.com/api/v1/admin/behavior-events)" = 404
test "$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H 'Origin: https://play.example.com' -d '{}' https://play.example.com/api/v1/public/play-sessions/current/events)" = 401
```

这些命令只验证代码层 Surface 隔离，不表示当前环境已经完成生产部署、数据保留、备份或完整 E2E 验收。

## 6. 当前限制

- 当前限流器保存在单个进程内，所以三个业务服务先各运行一个副本。以后增加副本前，应把限流状态迁移到共享存储。
- Railway 部署不会自动提供 MySQL/MinIO 备份。必须为两个 Volume 安排外部备份和恢复演练。
- MinIO 的资源域名暴露的是 S3 API，不是 Console。Bucket 必须保持私有，root 凭据不得提供给业务服务。
- Worker 已接入真实图生图协议、S3 生成图片持久化和 `love-journey@1.1.0` 情书/4 位密码/资源配置；在生产供应商质量评测、中间场景素材映射和全产品生产 E2E 完成前，环境仍只适合作为内测或候选发布环境。

Railway 相关行为可参考官方文档：[Dockerfile](https://docs.railway.com/builds/dockerfiles)、[Pre-deploy Command](https://docs.railway.com/guides/pre-deploy-command)、[Private Networking](https://docs.railway.com/networking/private-networking)、[Volumes](https://docs.railway.com/reference/volumes)、[Outbound Networking](https://docs.railway.com/networking/outbound-networking)。
