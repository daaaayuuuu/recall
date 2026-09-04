# 爱的旅程统一资料提交与 AI 情书润色实施记录

## 文档信息

- 版本：v0.1
- 状态：统一资料表单、DeepSeek 情书润色和双旅行照片独立上传已实现并完成本地验证
- 更新日期：2026-08-23
- 当前模板：`love-journey@1.1.0`
- 当前输入协议：`inputSchemaVersion = 3`
- 历史实施记录：[LOVE_JOURNEY_MATERIAL_INPUT_IMPLEMENTATION_v0.1.md](./LOVE_JOURNEY_MATERIAL_INPUT_IMPLEMENTATION_v0.1.md)

本文记录统一资料提交、文本模型配置和情书润色能力。旧实施记录描述的是 `love-journey@1.0.0`、`inputSchemaVersion = 2` 的五场景录入方式，只用于理解历史实现；当前新建和重置后的数据以 `love-journey@1.1.0` 的 v3 协议为准，不把旧数据兼容列为产品要求。

## 1. 本轮结论

本轮完成了以下闭环：

1. 创建游戏时不再要求用户按初见、吃饭、看电影、旅行和今天五个章节分别提交资料。
2. 新建游戏改为一次提交双人合照、旅行照片、情书、密码提示和拆信密码。
3. 情书文本框增加“AI 润色”按钮，已接入 DeepSeek `deepseek-v4-flash`。
4. 润色结果成功返回后自动回填原情书文本框，用户确认后再创建游戏。
5. 两张旅行照片改为“旅行照片 1”和“旅行照片 2”两个独立单文件入口，可以分别选择、替换和移除。
6. 文生图配置已移除；图生图已经由 Worker 实际调用，并把规范化 PNG 保存到 S3-compatible 对象存储。情书润色仍是用户主动触发的独立文本能力。
7. v2 旧模板定义仍留在代码中作为历史实现，但当前产品与测试基线不承诺旧草稿迁移或恢复。

## 2. 当前统一资料协议

### 2.1 图片资料

| 显示名称 | 槽位键 | 是否必填 | 数量 | 当前交互 |
|---|---|---:|---:|---|
| 双人自拍正脸合照 | `cover` | 否 | 0–1 | 单独选择，可替换或移除 |
| 旅行照片 | `travelPhotos` | 否 | 0–2 | 照片 1、照片 2 分开选择，按编号顺序上传和展示 |

创建页使用固定位置保存旅行照片索引。即使用户只选择“旅行照片 2”，前端也不会把它错误地压缩为第 1 张。提交时跳过空位置，并将实际位置作为 `sortOrder` 发送到素材上传接口。

创建与修改共用 `HomeView`。从“我的游戏”卡片进入修改页后，可以为草稿版本补传、替换、排序或删除素材；项目不再提供游戏详情页。

### 2.2 文字资料

| 显示名称 | 字段键 | 是否必填 | 约束 | 用途 |
|---|---|---:|---|---|
| 写给对方的情书 | `loveLetter` | 是 | 1–1000 个 Unicode 字符 | 最终情书正文，可调用 AI 润色 |
| 密码提示 | `passwordHint` | 否 | 最多 100 个 Unicode 字符 | 给接收方提供不直接泄露密码的提示 |
| 拆信密码 | `letterPassword` | 是 | 必须为 4 位数字 | 接收方拆开情书礼物时输入 |

后端仍以模板注册表为最终校验来源，拒绝未知字段、缺失必填字段、超长文本和非 4 位数字密码。生成配置解码同样只接受 4 位密码，文字输入按游戏版本加密后保存。

### 2.3 当前数据策略

- 当前 `love-journey@1.1.0` v3 模板将所有输入归入单一 `materials` 资料组。
- `love-journey@1.0.0` 的 v2 定义仍在注册表中，但只视为历史实现。
- 当前允许清空开发数据并从 v3 重新开始，不为 v2 提供隐式迁移、恢复入口或兼容验收。
- 新建游戏使用 v3，不再要求旧协议中的伴侣单人照、看电影照片、场景描述和 3 张必传旅行照片。

## 3. AI 情书润色

### 3.1 产品交互

情书输入框右下角提供“AI 润色”按钮：

1. 情书为空时按钮禁用，前端也会阻止空请求。
2. 点击按钮后，当前情书正文发送给后端。
3. 后端调用 DeepSeek Chat Completions API。
4. 成功时将 `polishedText` 回填情书文本框。
5. 用户仍可继续编辑，AI 返回内容不会自动创建游戏或覆盖数据库中的现有版本。
6. 页面明确提示用户：点击润色会把当前文字发送给 DeepSeek。

创建页与从“我的游戏”进入的修改页共用相同润色流程。

### 3.2 应用接口

```http
POST /api/v1/ai/love-letter/polish
Content-Type: application/json
X-CSRF-Token: <creator-csrf-token>

{
  "text": "需要润色的情书正文"
}
```

成功响应的数据部分：

```json
{
  "polishedText": "润色后的情书正文",
  "skipped": false
}
```

接口位于制作方登录和 CSRF 保护边界内。输入为空或超过 1000 个字符时返回字段校验错误；模型未配置时返回原文和 `skipped: true`，不阻断创建；模型超时返回 `AI_POLISH_TIMEOUT`；供应商失败或返回内容无效时返回 `AI_POLISH_UNAVAILABLE`。

### 3.3 DeepSeek 调用参数

| 参数 | 当前值 |
|---|---|
| Provider | `deepseek` |
| Base URL | `https://api.deepseek.com` |
| Endpoint | `/chat/completions` |
| Model | `deepseek-v4-flash` |
| Thinking | `disabled` |
| Temperature | `0.6` |
| Stream | `false` |
| 请求超时 | `30s` |
| 最大输出 Token | `2000` |
| 最终输出长度 | 不超过 1000 个 Unicode 字符 |

关闭 Thinking 是为了让短文本润色直接返回正文，减少额外推理延迟和非正文输出。

### 3.4 系统提示词约束

系统提示词约 200 字，核心规则如下：

- 让表达更自然、真诚、温柔，并适度优化画面感和情绪递进；
- 保留姓名、日期、称呼、人称、共同经历、事实和核心情感；
- 不虚构故事、承诺或细节；
- 不改成油腻、夸张或模板化表达；
- 保持原文语言和写信人的语气；
- 只返回可直接回填的正文，不返回标题、解释、点评或 Markdown；
- 输出不超过 1000 个字符。

提示词定义在 `backend/internal/textai/deepseek.go`，不放在环境变量中，便于代码审查和测试固定行为。后续如需让运营人员动态调整，应新增版本化提示词配置和回归评测，不建议直接把提示词改成无版本的数据库文本。

## 4. 模型环境变量

### 4.1 当前已启用的文生文配置

```dotenv
AI_TEXT_PROVIDER=deepseek
AI_TEXT_BASE_URL=https://api.deepseek.com
DEEPSEEK_API_KEY=
AI_TEXT_MODEL=deepseek-v4-flash
AI_TEXT_TIMEOUT=30s
AI_TEXT_MAX_OUTPUT_TOKENS=2000
```

`DEEPSEEK_API_KEY` 必须只写入本地 `.env` 或部署平台 Secret，不能提交到 Git、Markdown、截图或聊天记录。管理员也可以在“AI 配置”页面测试并发布文本模型配置；动态版本优先于环境变量，API Key 使用内容加密密钥加密保存且永不回显。两处都未配置密钥时项目仍可正常启动，润色接口保留原文并通过 `skipped: true` 明确提示已跳过。

### 4.2 当前图生图配置

```dotenv
AI_IMAGE_TO_IMAGE_PROVIDER=
AI_IMAGE_TO_IMAGE_BASE_URL=
AI_IMAGE_TO_IMAGE_API_KEY=
AI_IMAGE_TO_IMAGE_MODEL=
AI_IMAGE_TO_IMAGE_QUALITY=medium
AI_IMAGE_TO_IMAGE_TIMEOUT=3m
AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES=26214400
AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES=26214400
```

图片模型已经接入统一适配器。API Key 为空时 Worker 复用规范化源图，并仍将产物保存到 S3 作为 `game_render`；填写 Key 时还必须同时配置 `openai-compatible` Provider、Base URL 和 Model，Worker 才会调用真实 `/images/edits` 服务。

环境变量示例已同步到：

- 根目录 `.env.example`；
- `config/config.example.yaml`；
- `deploy/railway/.env.example`；
- `deploy/compose/prod/.env.example`；
- `deploy/compose/prod/compose.yaml`。

## 5. 安全与隐私处理

本轮实现包含以下约束：

1. API Key 只从服务端环境变量或加密的动态配置读取，不进入前端构建产物，也不通过管理接口回显。
2. 本地 `.env` 已被 `.gitignore` 排除。
3. 后端日志不记录用户情书、系统提示词、供应商响应正文或 API Key。
4. 供应商非 2xx 响应只保留 HTTP 状态，不把响应正文返回给前端或写入错误信息。
5. DeepSeek 响应读取上限为 1 MiB。
6. 返回正文必须是有效 UTF-8、非空且不超过 1000 个字符。
7. 润色接口要求已登录制作方会话和有效 CSRF Token。
8. 页面在按钮旁说明情书会发送给 DeepSeek，避免用户误以为润色完全在本地执行。

当前尚未实现独立的 AI 调用次数配额、成本预算或每用户速率限制，应在对外发布前补齐。

## 6. 错误处理

| 场景 | HTTP 状态 | 错误代码 | 用户结果 |
|---|---:|---|---|
| 请求 JSON 无效 | 400 | `INVALID_REQUEST` | 提示请求内容无效 |
| 情书为空或超过 1000 字 | 422 | `VALIDATION_ERROR` | 在情书字段提示修正 |
| 文本模型未配置 | 200 | — | 返回原文和 `skipped: true`，继续创建 |
| DeepSeek 调用超时 | 504 | `AI_POLISH_TIMEOUT` | 提示稍后重试 |
| DeepSeek 拒绝、限流、不可用或响应无效 | 502 | `AI_POLISH_UNAVAILABLE` | 提示服务暂时不可用 |

失败时前端保留用户原始情书，不清空输入框。

## 7. 关键实现文件

| 文件 | 作用 |
|---|---|
| `backend/internal/gametemplates/registry.go` | v3 统一资料字段、旅行照片数量及密码校验定义 |
| `backend/internal/games/handler.go` | 润色 API、输入校验、错误映射及创建资料处理 |
| `backend/internal/textai/deepseek.go` | DeepSeek 客户端、系统提示词、响应校验和错误类型 |
| `backend/internal/textai/deepseek_test.go` | DeepSeek 请求参数、鉴权头、失败和无效响应测试 |
| `backend/internal/platform/config/config.go` | 用户主动润色文本、上传图片审核和图生图配置加载及校验 |
| `backend/internal/platform/config/config_test.go` | AI 默认值、环境变量覆盖和可选配置测试 |
| `frontend/src/api/games.ts` | 润色接口及模板输入字段类型 |
| `frontend/src/views/HomeView.vue` | 统一创建表单、AI 回填、双旅行照片固定位置上传 |
| `frontend/src/views/HomeView.vue` | 创建与修改共用资料表单、新版本编辑、AI 回填和单张补传 |
| `frontend/src/styles/main.css` | 统一资料表单、润色按钮及独立照片位置样式 |
| `frontend/src/game-runtime/templates/registry.ts` | v3 模板运行时注册与兼容映射 |

## 8. 验证结果

### 8.1 自动化验证

- 后端完整 `go test ./...` 通过。
- 系统提示词更新后，`go test ./internal/textai ./internal/games ./internal/platform/config` 通过。
- 双旅行照片调整后，`go test ./internal/gametemplates ./internal/games` 通过。
- 前端 `npm run typecheck` 通过。
- 前端 `npm run lint` 通过，零 warning。
- 前端 Vitest 共 19 个测试文件、45 个测试通过。

### 8.2 真实 DeepSeek 联调

本地 API 使用 `.env` 中的私密密钥重启后，已在真实创建页面完成以下验证：

1. 输入一段虚构测试情书。
2. 点击“AI 润色”。
3. `deepseek-v4-flash` 成功返回润色正文。
4. 页面自动将返回内容回填到情书文本框。
5. 没有创建游戏或写入测试草稿。
6. API、MySQL、MinIO 和前端健康状态正常。

验证过程未输出、截图或记录 API Key。

### 8.3 双旅行照片界面验证

真实浏览器检查确认：

- 页面显示“旅行照片 1”和“旅行照片 2”；
- 旅行照片区域包含两个独立 `input[type=file]`；
- 两个文件输入均未启用 `multiple`；
- 两个位置分别显示选择、替换和移除操作；
- 帮助文本为“照片 1 和照片 2 分开选择，并按编号顺序展示。”

## 9. 当前未完成事项

1. 图生图已具备 OpenAI-compatible 适配器和 S3 持久化；仍需在选定生产供应商后完成真实质量、成本、限流和稳定性 E2E。
2. Worker 已将版本生成资源作为运行时资源列表返回，最终拆信揭晓会消费这些照片；中间的旅行关卡仍使用模板自带插画，尚未建立按槽位消费个性化照片的映射。
3. 双人合照与两张旅行照片在五段流程中的进一步分工仍需产品确认。
4. AI 润色尚未增加每用户限流、每日配额、成本统计和供应商请求 ID 追踪。
5. 尚未建立提示词版本、离线评测集和供应商模型升级回归流程。
6. OpenAPI 已同步润色接口、v3 创建/版本输入、模板清单、素材排序和返回字段；后续新增模板字段时仍需同步更新。
7. 对外发布前应补充隐私政策中的第三方模型处理说明及用户同意策略。

## 10. 建议的后续顺序

1. 冻结 v3 资料协议和运行时素材映射，明确 `cover`、`travelPhotos[0]`、`travelPhotos[1]` 在各关卡中的用途。
2. 让 Worker 生成包含真实素材引用和情书终局内容的模板配置。
3. 在制作方试玩中验证双旅行照片、拆信密码和情书终局的完整链路。
4. 为 AI 润色增加限流、额度、成本和无正文内容的可观测指标。
5. 建立情书润色评测样例，评估事实保持、语气保持、虚构率、长度和延迟。
6. 选择生产图生图模型并完成质量评测；文生图不在当前范围内，图生图与用户主动润色文本保持配置和失败隔离。
7. 继续保持 OpenAPI、部署说明和 MVP 实现状态与当前代码同步。
