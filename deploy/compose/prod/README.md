# 单机生产部署

该目录用于在一台 Linux 服务器上运行完整的 `game-gen`：一个统一业务镜像分别启动 API、Worker 和一次性迁移任务，MySQL、MinIO 与 Caddy 使用各自的官方镜像。除 80/443 外，服务端口只存在于 Docker 内部网络。

## 1. 前置条件

- Linux 服务器已安装 Docker Engine 与 Compose 插件。
- `app`、`play`、`assets` 三个域名的 A/AAAA 记录均指向服务器。
- 防火墙只对公网开放 80、443 和受限的 SSH 管理端口。
- 服务器时间同步正常；应用、MySQL 和 MinIO统一使用 UTC。
- 已规划异机备份位置。服务器本地备份不能防止整机或磁盘故障。

示例域名：

```text
app.example.com       制作端和管理端
play.example.com      公开游玩页
assets.example.com    MinIO 签名资源
```

## 2. 准备配置

```bash
cd deploy/compose/prod
cp .env.example .env
chmod 600 .env
```

编辑 `.env`，至少替换所有 `replace-with-*` 内容。生产文件不会被 Git 跟踪。

生成三个相互独立的 32 字节加密密钥：

```bash
openssl rand -base64 32
openssl rand -base64 32
openssl rand -base64 32
```

分别写入 `CONTENT_ENCRYPTION_KEY_V1`、`SHARE_ENCRYPTION_KEY_V1` 和 `AI_CONFIG_ENCRYPTION_KEY_V1`。产生生产数据后不得重新生成或丢失这些值；三种用途不能复用同一密钥。

管理员密码必须使用 Argon2id 哈希：

```bash
make hash-password PASSWORD='replace-this-password'
```

如果哈希包含 `$`，在 `.env` 中使用单引号包裹完整值。

验证配置：

```bash
make prod-config-check
```

## 3. 构建或载入统一业务镜像

可以直接在服务器构建：

```bash
make image \
  IMAGE=game-gen:0.1.0-rc.1 \
  APP_VERSION=0.1.0-rc.1 \
  VCS_REF=your-git-commit \
  BUILD_DATE=2026-08-14T00:00:00Z
```

然后把 `.env` 中的 `GAMEGEN_IMAGE` 设置为：

```dotenv
GAMEGEN_IMAGE=game-gen:0.1.0-rc.1
```

也可以在 CI 中构建后使用镜像仓库，或者通过以下方式离线传输：

```bash
docker save game-gen:0.1.0-rc.1 | gzip > game-gen-0.1.0-rc.1.tar.gz
gzip -dc game-gen-0.1.0-rc.1.tar.gz | docker load
```

不要在生产配置中使用可被覆盖的 `latest` 标签。

## 4. 首次安装

在项目根目录执行：

```bash
make prod-init
make prod-migrate
make prod-up
make prod-ps
```

这些命令依次完成：

1. 启动并等待 MySQL、MinIO。
2. 创建四个私有 Bucket 和最小权限应用账号。
3. 执行数据库迁移。
4. 启动 API、Worker 和 Caddy。
5. 由 Caddy自动申请并续期 HTTPS 证书。

验证：

```bash
curl --fail https://app.example.com/health/ready
curl --fail https://app.example.com/
curl --fail https://play.example.com/play/not-found
```

随后完成一次人工冒烟测试：使用用户 ID 注册、登录、上传图片、创建任务、创建分享链接，并在 `play` 域名打开分享。

## 5. 日常发版

每个版本构建新的不可变标签，然后：

1. 完成 MySQL 与 MinIO 异机备份。
2. 修改 `.env` 中的 `GAMEGEN_IMAGE`。
3. 验证 Compose 配置。
4. 执行迁移。
5. 更新应用容器。

```bash
make prod-config-check
make prod-migrate
make prod-up
make prod-ps
```

查看应用日志：

```bash
make prod-logs
```

迁移必须保持向后兼容。推荐先增加新表或新列，再部署应用；删除旧字段应放到后续独立版本。

## 6. 回滚

把 `.env` 中的 `GAMEGEN_IMAGE` 改回上一个已验证版本，然后执行：

```bash
make prod-up
make prod-ps
```

生产回滚默认只回滚应用镜像，不自动执行数据库 `down-one`。如果新版本执行了不兼容迁移，应优先向前修复；从备份恢复数据库只作为最后手段。

## 7. 数据和安全边界

- `mysql-data`、`minio-data`、`caddy-data` 和 `caddy-config` 是持久化 Volume。
- 不要在生产执行 `docker compose down --volumes`。
- 不要把 `.env`、数据库转储、MinIO 内容或证书目录复制进业务镜像。
- MySQL 3306、MinIO 9000/9001、API 8080 和 Worker 8081 不映射到宿主机公网。
- `play` 域名仅允许 `/api/v1/public/*`；其他 API 在 Caddy 层返回 404。
- MinIO Bucket 保持私有，浏览器只通过短期预签名 URL 访问资源。
- 每日备份 MySQL 和 MinIO，并定期在另一台机器上执行恢复演练。

## 8. 当前产品限制

该部署方案解决单机发布和运行问题。Worker 已接入真实图生图协议，并把生成 PNG 保存到 S3-compatible 对象存储；`love-journey@1.1.0` 已消费情书、4 位密码和最终揭晓资源。在生产模型质量评测、中间场景素材映射、全产品生产 E2E、监控告警和恢复演练完成前，环境仍应视为内测或候选发布环境。
