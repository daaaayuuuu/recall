# 图片 LLM 安全审核实现说明

## 文档信息

- 版本：v0.1
- 更新日期：2026-08-23
- 范围：制作方创建或编辑游戏时上传的图片素材
- 结论：同步上传前审核已接入；未配置 Key 时以明确的降级模式跳过

## 1. 目标与边界

游戏图片上传不能直接依赖某一家模型。业务层只依赖 `imagemoderation.Reviewer` 接口，供应商请求格式、提示词、预览图片编码和响应校验都封装在独立模块中。后续头像、举报图片或生成结果复审可以复用同一接口，并通过 `Purpose` 提供受控业务用途。

当前接入范围只包括游戏版本的 `source` 和 `cover` 素材。头像上传与 AI 生成图片暂未接入审核，审核结果也暂未写入独立数据库表。

## 2. 上传处理顺序

```text
multipart 临时文件
  -> 校验 JPEG / PNG / WebP
  -> 安全解码并重新编码为 PNG，清除原图元数据
  -> 已配置 Key：从规范化图片生成最长边 768 px 的临时 JPEG 审核预览
       -> 视觉 LLM 返回结构化审核结论
       -> 通过：写入对象存储和数据库
       -> 拒绝：返回 422，不写入素材
       -> 超时、供应商异常或无效响应：返回 503，不写入素材
  -> 未配置 Key：记录跳过状态并继续写入
```

降级只由“没有配置审核 Key”触发。一旦配置了供应商，审核仍采用失败关闭策略：模型无法给出可信结论时，系统不会用“默认通过”继续上传。传给模型的是重新编码并缩小后的图片，不包含原始文件名和原始图片元数据。未配置 Key 的部署不具备等价的内容安全保护，正式开放上传前应评估并配置审核服务。

## 3. 模块设计

核心接口位于 `backend/internal/imagemoderation/reviewer.go`：

- `Reviewer.Configured()`：报告当前实现是否具备调用条件；
- `Reviewer.Review(context.Context, Input)`：返回统一 `Decision`；
- `Input`：只包含图片读取器、MIME 类型和受控用途；
- `Decision`：包含是否通过、安全类别、内部原因和供应商请求 ID。

当前实现包含：

1. `OpenAICompatibleReviewer`：调用支持图片输入的 `/chat/completions` 端点；
2. `DevelopmentReviewer`：保留给自动化测试使用的确定性放行实现；
3. `UnconfiguredReviewer`：显式报告未配置，由上传业务层记录并执行无 Key 降级。

新增供应商时应实现 `Reviewer`，不要把供应商 SDK、请求 DTO 或错误类型暴露给 `games.Handler`。

## 4. 审核策略

视觉 LLM 只在图片明确包含以下高风险内容时拒绝：

- 明确色情内容；
- 涉及未成年人的性化或剥削内容；
- 血腥暴力或严重可见伤害；
- 正在发生或图形化的自残；
- 仇恨或极端主义宣传、招募；
- 明确展示或指导严重违法活动；
- 可读的身份、财务、医疗或认证文件；
- 同等严重但不属于以上类别的风险。

普通人像、情侣互动、旅行、食物、泳装、艺术作品和日常场景允许上传。模型只能返回约定 JSON 字段和固定类别；未知类别、额外字段、空原因、结论与类别矛盾或附带其他文本都会被视为无效响应并失败关闭。

## 5. 配置

无 Key 降级模式（所有环境均可启动）：

```dotenv
AI_IMAGE_MODERATION_PROVIDER=
AI_IMAGE_MODERATION_BASE_URL=
AI_IMAGE_MODERATION_API_KEY=
AI_IMAGE_MODERATION_MODEL=
AI_IMAGE_MODERATION_TIMEOUT=20s
AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS=300
```

真实审核模式：

```dotenv
AI_IMAGE_MODERATION_PROVIDER=openai-compatible
AI_IMAGE_MODERATION_BASE_URL=https://provider.example/v1
AI_IMAGE_MODERATION_API_KEY=replace-with-secret
AI_IMAGE_MODERATION_MODEL=replace-with-vision-model
AI_IMAGE_MODERATION_TIMEOUT=20s
AI_IMAGE_MODERATION_MAX_OUTPUT_TOKENS=300
```

只有在 `AI_IMAGE_MODERATION_API_KEY` 非空时，启动校验才要求 Provider、Base URL 和 Model 完整有效；生产环境的 Base URL 必须使用 HTTPS。配置了 Key 的供应商错误仍会阻断上传；空 Key 则跳过审核。公开游玩服务不接受上传，因此不需要持有审核模型密钥。

## 6. 接口行为

`POST /api/v1/games/{gameId}/versions/{versionId}/assets` 新增以下错误：

| HTTP | 错误码 | 含义 |
|---:|---|---|
| 422 | `IMAGE_MODERATION_REJECTED` | 图片明确未通过安全审核 |
| 503 | `IMAGE_MODERATION_UNAVAILABLE` | 模型超时、不可用或响应无法可信解析 |

前端创建流程会保留已经创建的草稿，并显示对应图片位置及服务端错误信息；用户可从“我的游戏”卡片进入修改页重新上传。创建与修改共用同一套上传组件和错误反馈，不再存在游戏详情页上传入口。

## 7. 隐私、日志与后续工作

- API 不记录图片内容、Base64 数据、文件名或模型返回的自由文本原因；拒绝日志只记录固定类别、供应商请求 ID 和平台请求 ID。
- 供应商仍会接收缩小后的图片内容。生产选型前必须确认数据留存、训练使用、跨境传输、内容审核和个人信息处理条款。
- 当前没有保存每次审核的独立审计记录。若后续需要申诉、管理员复核或合规留档，应新增与业务主体松耦合的审核记录仓储，避免直接把供应商字段塞入 `assets` 表。
- 头像、AI 生图结果和历史素材补审可以在后续分别接入同一 `Reviewer`；不同用途可在独立策略层决定是否使用相同类别和阈值。
- 需要补充真实供应商的沙箱联调、延迟与成本指标、限流、熔断，以及脱敏评测集上的误拒和漏放回归。
