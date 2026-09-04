# 图生图与 S3 产物实现说明

## 文档信息

- 版本：v0.1
- 日期：2026-08-23
- 状态：已实现
- 范围：游戏创建 Worker 的图生图调用、结果校验、对象存储和任务状态

## 1. 实现边界

游戏创建任务只执行图生图，不执行文生图，也不使用 AI 自动生成游戏文案。游戏文本来自用户输入或模板固定内容；情书润色是用户主动触发的独立功能，不属于本链路。

核心边界为 `backend/internal/imagegeneration.Transformer`。Worker 只依赖统一输入和图片字节输出，供应商协议、鉴权和响应解码留在适配器内部。目前实现：

- `openai-compatible`：调用 `{baseURL}/images/edits`，使用 multipart 上传一张源图，要求 `data[0].b64_json` 返回图片。
- 无 Key 透传：`AI_IMAGE_TO_IMAGE_API_KEY` 为空时直接返回源图片，不调用外部模型；Worker 后续仍会规范化并保存 `game_render`。

## 2. 创建流程

```text
gamegen-source-assets 中已审核源图
  → Worker 读取（受最大输入字节限制）
  → ImageTransformer + 模板固定风格提示词
  → 校验返回字节可解码为受支持图片
  → 重新编码为无元数据 PNG
  → 写入 gamegen-render-assets
  → 记录生成图校验和、尺寸、槽位、顺序和 S3 对象键
  → 生成成功事务写入 assets(kind=game_render)
  → game_version_assets(role=render)
  → 游戏版本标记 ready
```

模型返回内容不会以远程 URL 直接进入配置。所有可玩图片必须先由 Worker 解码、规范化并保存到平台自己的 S3-compatible 对象存储。

版本化 JSON 游戏配置仍写入 `gamegen-artifacts`。若图生图、图片解码、S3 读写或最终数据库事务失败，本次已经写入的配置和生成图片会被清理。

## 3. 状态、租约与取消

后端已移除固定五阶段等待、模拟失败开关和内部模拟百分比。状态只反映真实工作：

```text
queued → transforming_images → saving_results → completed
```

`progress` 字段暂时保留用于 API/数据库兼容，处理中为 `0`，成功或最终失败时为 `100`。前端继续显示进度条，但按真实 `queued/transforming_images/saving_results/completed` 阶段硬编码映射为 8%/68%/88%/100%；这些百分比只是产品呈现，不代表供应商实时进度。

图生图调用可能长于单次任务租约，因此 Worker 在调用期间按租约的约三分之一间隔续租，最长 20 秒一次。心跳同时检查取消请求；取消会终止模型 HTTP 请求，并使用不继承取消信号的短时清理上下文删除已写对象。

数据库迁移 `000005_replace_simulated_generation_stages` 会把升级时仍在排队或运行的旧任务归一到新状态。

## 4. 配置

真实图生图服务配置（可选）：

```dotenv
AI_IMAGE_TO_IMAGE_PROVIDER=openai-compatible
AI_IMAGE_TO_IMAGE_BASE_URL=https://provider.example/v1
AI_IMAGE_TO_IMAGE_API_KEY=replace-with-secret
AI_IMAGE_TO_IMAGE_MODEL=replace-with-image-edit-model
AI_IMAGE_TO_IMAGE_QUALITY=medium
AI_IMAGE_TO_IMAGE_TIMEOUT=3m
AI_IMAGE_TO_IMAGE_MAX_INPUT_BYTES=26214400
AI_IMAGE_TO_IMAGE_MAX_OUTPUT_BYTES=26214400
```

`AI_IMAGE_TO_IMAGE_QUALITY` 只接受 `auto`、`low`、`medium` 或 `high`。Key 为空时 Worker 使用无 Key 透传，创建流程仍可完成，产物是重新规范化并保存到 S3 的源图副本。Key 非空时，Provider、Base URL 和 Model 必须完整；生产环境的模型地址必须使用 HTTPS，配置不完整或供应商类型不支持会拒绝启动。

文生图相关配置已经删除。`GENERATION_STEP_DELAY` 和 `MOCK_GENERATOR_FAIL_STAGE` 也已删除。

## 5. 失败分类

| 场景 | 错误代码 | 可重试 |
| --- | --- | --- |
| 源图超过输入限制 | `INPUT_VALIDATION_FAILED` | 否 |
| 模型超时、不可用或返回无效/超大图片 | `PROVIDER_UNAVAILABLE` | 是 |
| S3 读取或写入失败 | `STORAGE_WRITE_FAILED` | 是 |
| 配置构建等非预期内部错误 | `INTERNAL_ERROR` | 否 |

管理员诊断只保存稳定错误类型、生成器版本、Trace ID 和重试属性，不保存源图片、模型返回图片、提示词、供应商响应正文、文件名或对象地址。

## 6. 验证覆盖

- 图片编辑适配器请求路径、鉴权、multipart 字段和 base64 响应解码。
- 输入/输出上限与无效响应。
- 生成图片重新编码后的 MIME、尺寸、校验和和 `gamegen-render-assets` 写入。
- 第二张图片生成失败时清理配置和已生成图片。
- 成功、取消、租约错误、失败持久化和 Analytics 最终事件边界。
- 配置环境变量、无 Key 透传和已配置供应商的严格 Worker 启动校验。
