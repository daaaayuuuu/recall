# Recall

个性化游戏生成与分享平台 MVP。

当前仓库已经贯通阶段 1–5 的核心产品链路，包括认证与个人设置、游戏与素材、真实图生图任务、`love-journey@1.1.0` 五段玩法、制作方试玩、分享链接和公开游玩会话；阶段 6 也已完成动态 AI 配置、第一版用户行为记录与管理员脱敏查询。完整运营后台、生产供应商质量验收、监控备份和上线加固仍未完成，因此暂不能标记为可直接生产上线。

产品和技术基线位于 `docs/`；独立且平台中立的 AI 候选、成本公式和评测方案见 [`docs/AI_MODEL_SELECTION_v0.2.md`](docs/AI_MODEL_SELECTION_v0.2.md)，模板独立代码、共用基础引擎和平台运行时之间的接口见 [`docs/GAME_TEMPLATE_RUNTIME_CONTRACT_v0.1.md`](docs/GAME_TEMPLATE_RUNTIME_CONTRACT_v0.1.md)，当前完成度与文档差异见 [`docs/MVP_IMPLEMENTATION_STATUS.md`](docs/MVP_IMPLEMENTATION_STATUS.md)。团队共用的本地基础环境位于 `deploy/compose/dev/`。

首次参与开发请直接阅读 [`README_DEVELOPMENT.md`](README_DEVELOPMENT.md)，其中包含必装环境、首次启动步骤、必需配置、模块说明和常见问题排查。

## 快速启动开发环境

需要预先安装 Docker Desktop 或兼容的 Docker Engine 与 Compose 插件。

首次拉取并启动 Docker Desktop 后，在项目根目录执行：

```bash
make dev-setup
```

该命令会校验工具版本、创建缺失的本地配置、安装锁定依赖、拉取开发镜像、启动 MySQL/MinIO、创建 Bucket 并完成数据库迁移，且不会覆盖已有 `.env`。完成后打开两个终端，分别运行：

```bash
./scripts/start-backend.sh
./scripts/start-frontend.sh
```

后端脚本会自动启动 MySQL、MinIO，创建 Bucket、执行数据库迁移，再启动 API 和 Worker。需要主动拉取项目锁定版本的开发镜像时运行 `make dev-env-update`。

启动后可使用：

- MySQL：`127.0.0.1:3306`
- MinIO Console：`http://127.0.0.1:9001`

完整配置、连接参数、Bucket 和重置方式见 [`deploy/compose/dev/README.md`](deploy/compose/dev/README.md)。

> `.env.example` 中的密码仅供本地开发。共享和生产环境必须使用独立 Secret。

### 完全重置本地开发数据

需要删除所有本地账号、游戏、分享、任务和对象文件并重新开始时，运行：

```bash
bash scripts/reset-dev.sh
```

脚本会要求输入当前开发 Compose 项目名进行二次确认，然后删除 MySQL 和 MinIO 的开发命名卷，重新启动基础设施、创建私有 Bucket，并从零执行全部数据库迁移。重置完成后再运行前后端启动脚本即可。源码、`.env`、依赖目录和生产 Compose 不会被删除。

若只想核对操作范围，可先运行 `bash scripts/reset-dev.sh --dry-run`。只有确认本地 MySQL/MinIO 数据都可删除后，才运行上面的交互式重置；日常本地恢复不要跳过确认。该脚本只接受 `APP_ENVIRONMENT=development` 且 Compose 项目名包含 `dev` 的配置。

也可以绕过启动脚本，分别在三个终端启动进程。此时需要先运行 `make dev-prepare`：

```bash
make dev-prepare
make api
make worker
make frontend
```

正常开发推荐使用启动脚本，将后台 API 与 Worker 合并到一个终端运行：

```bash
./scripts/start-backend.sh
./scripts/start-frontend.sh
```

后台脚本还支持仅启动单个进程：

```bash
./scripts/start-backend.sh api
./scripts/start-backend.sh worker
```

开发入口：

- 前端：`http://127.0.0.1:5173`
- API 存活检查：`http://127.0.0.1:8080/health/live`
- API 就绪检查：`http://127.0.0.1:8080/health/ready`
- Worker 就绪检查：`http://127.0.0.1:8081/health/ready`

## 阶段 2：认证与个人设置

当前已经支持制作方使用管理员生成的一次性邀请码和自选用户 ID 注册，以及登录、退出、昵称、头像与已登录密码修改。邀请码格式为 `XXXX-XXXX`，每个邀请码只能成功注册一个账号；完整邀请码只在管理员生成成功时显示一次。MVP 不提供忘记密码自助找回，请妥善保存用户 ID 和密码。

管理员密码必须配置为 Argon2id 哈希。生成本地哈希：

```bash
make hash-password PASSWORD='change-me-in-local-env'
```

将输出结果写入本地 `.env` 的 `ADMIN_PASSWORD_HASH` 后重启后台。哈希值包含 `$`，请使用单引号包裹整个值，例如：

```dotenv
ADMIN_PASSWORD_HASH='$argon2id$...'
```

前端入口：

- 注册：`http://127.0.0.1:5173/auth/register`
- 登录：`http://127.0.0.1:5173/auth/login`
- 个人设置：`http://127.0.0.1:5173/app/settings`
- 管理员登录：`http://127.0.0.1:5173/admin/login`
- 管理员邀请码：`http://127.0.0.1:5173/admin/invitation-codes`
- 管理员行为记录：`http://127.0.0.1:5173/admin/behavior-events`
- 管理员 AI 配置：`http://127.0.0.1:5173/admin/ai-settings`

## 阶段 3：游戏、版本与图片素材

登录后，可以创建游戏草稿、维护名称与描述、为同一个游戏新建版本，以及给草稿版本上传或删除图片。创建页同时承担已有游戏的修改流程；“我的游戏”不提供恢复或详情中转页，每张卡片固定只有“试玩、修改、分享、删除”四种操作，并依据游戏状态禁用暂不可用的操作。

前端入口：

- 创建游戏：`http://127.0.0.1:5173/app/create`
- 游戏列表：`http://127.0.0.1:5173/app/games`
- 修改游戏：从“我的游戏”卡片的“修改”进入

后端为每个版本加密保存用户输入文本；创建游戏时只把用户输入和模板固定内容组装进配置，不调用 AI 自动生成游戏文案。情书润色仍是用户主动点击后才调用的独立文本能力；没有配置文本模型 Key 时接口会保留原文并提示已跳过，不影响后续创建。图片上传支持 JPEG、PNG 与 WebP，解码后统一重新编码为 PNG，以清理原文件元数据。配置图片审核 Key 后，游戏素材在写入对象存储前还会生成最长边 768 像素的临时 JPEG 预览，并交给可替换的视觉 LLM 做同步安全审核；未通过或已配置的审核服务异常时不会保存图片。头像会居中裁剪为最大 512×512，并保存在私有 `gamegen-user-assets` Bucket；游戏原始素材保存在 `gamegen-source-assets`，浏览器拿到的都是短期预签名预览地址。删除整个游戏时，API 会先将游戏移出列表，再由 Worker 清理对象存储和数据库记录。

审核配置是可选的。`AI_IMAGE_MODERATION_API_KEY` 为空时会明确跳过审核，使本地、测试和尚未接入 AI 的部署仍可完成上传；这会降低内容安全保护，正式开放上传前建议配置支持图片输入及 OpenAI-compatible `/chat/completions` 格式的视觉 LLM：

```dotenv
AI_IMAGE_MODERATION_PROVIDER=openai-compatible
AI_IMAGE_MODERATION_BASE_URL=https://your-provider.example/v1
AI_IMAGE_MODERATION_API_KEY=replace-with-secret
AI_IMAGE_MODERATION_MODEL=replace-with-vision-model
AI_IMAGE_MODERATION_TIMEOUT=20s
AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS=300
```

审核模块的边界、策略和复用方式见 [`docs/IMAGE_LLM_MODERATION_IMPLEMENTATION_v0.1.md`](docs/IMAGE_LLM_MODERATION_IMPLEMENTATION_v0.1.md)。

三类 AI 配置也可以由管理员在线校验、测试并发布；API Key 加密保存且永不回显。动态配置的启动密钥、版本语义和安全边界见 [`docs/DYNAMIC_AI_SETTINGS_IMPLEMENTATION_v0.1.md`](docs/DYNAMIC_AI_SETTINGS_IMPLEMENTATION_v0.1.md)。

本地 `.env` 中的 `CONTENT_ENCRYPTION_KEY_V1` 必须是 Base64 编码的 32 字节密钥。仓库示例值仅供开发，部署到共享环境前必须替换。

接口定义见 [`contracts/openapi/openapi.yaml`](contracts/openapi/openapi.yaml)。

## 阶段 4：创建任务框架

创建或修改页可以把当前版本提交到 MySQL 任务队列。API 使用 `Idempotency-Key` 防止重复点击创建多条任务；Worker 使用租约领取任务，并在真实图生图调用期间续租和检查取消请求。后端只保存 `queued/running/succeeded/failed/cancelled` 等真实状态以及兼容性的 0/100 进度值；前端按真实阶段固定映射为 8%、68%、88% 和 100% 的展示进度，不把该百分比当作模型实时完成度。生成页不会从“我的游戏”恢复旧运行，用户只会在本次提交后进入对应运行路由。

Worker 会读取版本源图，逐张调用低耦合的图生图适配器，把返回结果重新解码并规范化为 PNG，再写入 S3-compatible 的私有 `gamegen-render-assets` Bucket。生成图片的校验和、尺寸、槽位和对象位置会在生成成功事务中以 `game_render` 资源关联到对应游戏版本；版本化 JSON 配置写入 `gamegen-artifacts`。任一步失败、取消或数据库完成事务失败时，会清理本次已写入的配置和生成图片。

图生图配置也是可选的。`AI_IMAGE_TO_IMAGE_API_KEY` 为空时，Worker 会复用规范化后的源图，仍将它作为版本专属 `game_render` 保存到 S3，保证创建、试玩和分享链路可用；配置 Key 后则调用支持 OpenAI-compatible `/images/edits` 的真实图生图服务：

```dotenv
GENERATION_LEASE_DURATION=60s
GENERATION_MAX_EXECUTIONS=3
AI_IMAGE_TO_IMAGE_PROVIDER=openai-compatible
AI_IMAGE_TO_IMAGE_BASE_URL=https://your-provider.example/v1
AI_IMAGE_TO_IMAGE_API_KEY=replace-with-secret
AI_IMAGE_TO_IMAGE_MODEL=replace-with-image-edit-model
AI_IMAGE_TO_IMAGE_QUALITY=medium
AI_IMAGE_TO_IMAGE_TIMEOUT=3m
AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES=26214400
AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES=26214400
```

管理员首页能够查看稳定错误代码、Trace ID 和白名单诊断字段，但不会返回用户输入文本、图片内容、模型响应、文件名或对象地址。项目没有文生图配置或自动游戏文案生成步骤。

完整链路、清理语义和配置边界见 [`docs/IMAGE_TO_IMAGE_GENERATION_IMPLEMENTATION_v0.1.md`](docs/IMAGE_TO_IMAGE_GENERATION_IMPLEMENTATION_v0.1.md)。

## 阶段 5：分享与公开游玩

制作方可以从“我的游戏”卡片直接试玩当前已完成版本。该入口需要登录，只读取当前账号拥有的版本，不会自动创建分享链接或公开游玩会话；试玩资源使用最长 15 分钟的临时地址。

游戏创建完成后，可从卡片的“分享”进入独立页面，生成 1 天、7 天、30 天或自定义截止时间的分享链接，并复制链接或下载二维码。当前页面只处理本次新建的分享结果，不提供历史链接恢复或管理入口。链接形式为：

```text
http://127.0.0.1:5173/play/{publicId}#t={secret}
```

公开页会从 URL Fragment 读取 Secret，并立即从地址栏移除。接收方无需注册即可查看分享者昵称、开始最长 30 分钟的一局游戏、加载经过 [`contracts/game-config/v1.schema.json`](contracts/game-config/v1.schema.json) 约束的版本配置，以及在当前会话内再次游玩。

分享自然到期后，已经开始的一局可以继续到自身会话过期；制作方主动停止分享或永久删除游戏时，相关游玩会话立即失效。公开接口不会返回制作方用户 ID、内部用户主键、用户输入原文、原始图片或 MinIO 对象地址；生成文本素材仅作为已校验游戏配置的一部分，在有效预览或游玩边界内返回。

当前新建游戏固定使用 `love-journey@1.1.0`。五段回忆固定为初见、吃饭、看电影、旅行和拆信，不从配置中的 `rounds` 数量推导；拆信密码固定为 4 位数字，当前实现不承担旧数据兼容。生成任务会产出版本专属图生图素材，并把情书、密码、密码提示和资源绑定写入可玩配置。`memory-game@1.0.0` 与旧模板定义只作为仓库历史实现存在，不是新建流程入口。

## 响应式前端

制作端、认证页、公开游玩页和管理页同时支持电脑与手机浏览器，并以手机端体验为优先：

- `320px–700px` 使用单列卡片、全宽主要操作和制作端底部导航，并适配 iPhone 安全区域。
- `701px–960px` 使用平板过渡布局；创建/修改、设置和分享页保持单列，列表按两列展示。
- `961px` 以上使用桌面多列布局。
- 交互控件的移动端最小触控高度为 44px；游戏卡片四操作区和创建表单不得引起页面横向溢出。

本地修改页面后，应至少使用 `320×800` 和 `1440×900` 两种视口检查登录、创建/修改、列表、生成进度、分享、制作方试玩、设置、公开游玩和管理页面。

## 用户行为记录（阶段 6 部分完成）

系统以 best-effort 方式记录 14 类冻结事件，覆盖制作方注册/登录/页面访问、游戏与版本创建、素材上传、生成提交与最终结果、分享创建/打开，以及公开游玩的开始/完成/重玩。Analytics 写入失败不会回滚或阻断主业务；这些记录不能作为计费、合规审计或强一致业务事实来源。

管理员可在 `http://127.0.0.1:5173/admin/behavior-events` 查询脱敏事件。页面支持事件名、Creator ID、登录 ID、Game ID、来源、开始时间和结束时间 7 类筛选，默认读取 50 条，并使用 cursor 继续加载。关联身份由服务端可信上下文推导，事件属性和管理页面都按事件白名单展示，不包含 Cookie、Token、分享 Secret、用户输入、图片内容或 MinIO 对象地址。

该能力是内部观察和故障排查入口，不是完整运营后台、管理员安全审计日志或 BI 系统；当前没有漏斗、留存、聚合指标、导出、访问通知、生产保留期限或自动清理。

## 阶段 6 与 MVP 收口

阶段 6 已部分完成管理员认证、动态 AI 配置、创建任务脱敏诊断和用户行为记录查询。当前真实生成、五段模板运行时以及制作方/接收方核心游玩链路已经落地；剩余工作主要是完整管理员运营能力、生产供应商质量与成本验收、监控备份、安全扫描及生产环境端到端验收。隔离 E2E 已覆盖真实 MySQL、MinIO、API、Worker、Vite 和浏览器主链，但不能替代生产环境验收。具体清单和完成判定见 [`docs/MVP_IMPLEMENTATION_STATUS.md`](docs/MVP_IMPLEMENTATION_STATUS.md)。

运行全部静态检查、测试与构建：

```bash
make lint
make test
make build
```

## 单机发布与部署

生产部署采用一个统一业务镜像。该镜像同时包含 API、Worker、数据库迁移程序和 Vue 静态文件，并在 Compose 中以三个独立容器运行；MySQL、MinIO 与 Caddy 也运行在同一台 Linux 服务器上。

```bash
make image IMAGE=game-gen:0.1.0-rc.1 APP_VERSION=0.1.0-rc.1
cp deploy/compose/prod/.env.example deploy/compose/prod/.env
# 编辑生产域名、凭据和加密密钥
make prod-init
make prod-migrate
make prod-up
```

完整的首次安装、发版、回滚和安全说明见 [`deploy/compose/prod/README.md`](deploy/compose/prod/README.md)。当前生产供应商验收与上线加固仍未完成，因此该方案现阶段用于内测和候选发布环境。

## Railway 部署

Railway 部署复用同一个业务镜像，但分别运行制作端 API、公开游玩 API 和 Worker；MySQL 与 MinIO 继续使用项目固定版本的官方 Docker 镜像和独立 Volume，不使用 Railway MySQL 模板或 Railway Bucket。应用会自动读取 Railway 注入的 `PORT`，并通过 `SERVICE_SURFACE=app|play` 在代码层隔离两个公网服务。

完整的服务创建顺序、变量、MinIO 初始化、数据库迁移、域名与健康检查配置见 [`deploy/railway/README.md`](deploy/railway/README.md)。
