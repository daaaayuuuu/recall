# 本地开发基础环境

该 Compose 环境提供 `game-gen` 开发所需的基础依赖：

- MySQL 8.4 LTS
- MinIO 与四个默认私有 Bucket

前端、API 和 Worker 仍由本机开发进程运行。对应工程和 Dockerfile 建立后，再按需加入 Compose profile。

## 启动

```bash
cd deploy/compose/dev
cp .env.example .env
docker compose pull
docker compose up -d --wait
docker compose --profile setup run --rm minio-init
docker compose ps
```

在项目根目录推荐运行：

```bash
make dev-setup    # 全新 clone：配置、依赖、镜像、基础设施和迁移
make dev-prepare
```

该命令会完成健康等待、Bucket 初始化和数据库迁移。若还要先拉取 Compose 中锁定版本的全部开发镜像，运行 `make dev-env-update`。`make dev-infra-up` 仅启动基础设施和初始化 Bucket，不执行迁移。

`.env` 只用于本地开发且不会被 Git 跟踪。请勿把示例密码用于共享或生产环境。

## 本机连接信息

| 服务 | 地址 | 凭据 |
|---|---|---|
| MySQL | `127.0.0.1:3306` | 数据库、用户名和密码见 `.env` |
| MinIO API | `http://127.0.0.1:9000` | 用户名和密码见 `.env` |
| MinIO Console | `http://127.0.0.1:9001` | 用户名和密码见 `.env` |

API 在宿主机运行时，可使用：

```text
MYSQL_DSN=gamegen:gamegen_dev_only@tcp(127.0.0.1:3306)/gamegen?parseTime=true&loc=UTC&charset=utf8mb4&multiStatements=true
MINIO_ENDPOINT=127.0.0.1:9000
```

API 在同一个 Compose 网络运行时，将主机名分别改为 `mysql` 和 `minio`。

MinIO 初始化任务会建立以下 Bucket，并保持匿名访问关闭：

```text
gamegen-user-assets
gamegen-source-assets
gamegen-render-assets
gamegen-artifacts
```

## 常用命令

```bash
docker compose logs -f
docker compose stop
docker compose start
docker compose down
```

如需彻底重置开发数据，可运行以下命令。该操作会永久删除本环境中的数据库和对象：

```bash
docker compose down --volumes
```

若要一次性删除全部开发数据并自动重建基础设施、Bucket 和数据库表，推荐从项目根目录运行：

```bash
bash scripts/reset-dev.sh
```

这会永久清空 MySQL 和 MinIO 的开发命名卷。使用 `--dry-run` 可以先查看操作计划，使用 `--yes` 可以在明确知晓后果时跳过交互确认。脚本只处理 `deploy/compose/dev/compose.yaml`，不会操作生产 Compose。
