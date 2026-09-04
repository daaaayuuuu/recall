# 个性化游戏图生图模型选型与成本测算

## 文档信息

- 版本：v0.3（范围收敛修订）
- 更新日期：2026-08-20
- 创建任务 AI 范围：用户图片卡通化（图生图）
- 明确排除：文生图、AI 自动生成游戏文案
- 独立能力：情书润色仅在用户主动点击时调用文本模型，不属于创建任务

> 供应商价格和模型能力会变化，上线前必须重新核对官方资料和真实账单。

## 1. 当前结论

创建任务只需要图生图：Worker 将通过安全审核的用户源图连同模板固定风格提示词发送给图片编辑模型，要求保留人物身份、数量、关系、姿态和关键外观特征。模型输出必须是图片字节；Worker 会再次解码并统一编码为 PNG，随后保存到 S3-compatible 的 `gamegen-render-assets`，并以 `game_render` 资源关联到游戏版本。

当前代码使用低耦合 `ImageTransformer` 接口，并实现 OpenAI-compatible `/images/edits` 协议。生产环境通过以下配置选择实际模型：

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

模型配置可选：`AI_IMAGE_TO_IMAGE_API_KEY` 为空时，所有环境都使用无 Key 透传，不调用 AI，但仍把规范化源图保存为版本专属 `game_render`；Key 非空时才启用真实模型并严格校验其余供应商配置。

## 2. 评测约束

候选模型必须同时满足：

1. 接受单张参考图片和文本编辑指令。
2. 返回可直接解码的图片；当前适配器要求响应包含 `data[0].b64_json`。
3. 能稳定保留人物数量、身份、关系、肤色、发型、服装、姿态和主要构图。
4. 不擅自加入文字、Logo、水印、对话框或额外人物。
5. 支持请求超时取消，并提供稳定的错误状态；最好返回请求 ID 便于脱敏排障。
6. 供应商的数据保留、训练使用、内容安全和地域合规条款满足产品要求。

## 3. 成本基准

在没有生产日志前，可使用以下可替换参数：

| 参数 | 示例值 |
| --- | ---: |
| 日均游戏创建量 `D` | 1,000 局 |
| 每局源图数量 `N` | 4 张 |
| 重试与重生成系数 `G` | 1.20 |
| 月计费天数 `M` | 30 日 |

```text
月图生图调用数 = D × N × G × M
               = 1,000 × 4 × 1.20 × 30
               = 144,000 次

月成本 = 月调用数 × 单次图生图实际账单成本
```

图生图成本通常同时受输入图尺寸、输出图尺寸、质量档位和重试率影响，不应直接套用纯文生图的单张标价。评测需要记录每次调用的输入、输出、失败原因、实际账单和最终是否验收合格。

## 4. 最小评测集

- 30 组经过授权和脱敏的参考照片，每个模型每组生成 2 张。
- 覆盖单人、双人、多人、复杂背景、低清图片、不同肤色和年龄。
- 使用与代码一致的固定模板风格提示词，不加入用户隐私文本。
- 盲评时优先比较人物保真、人数正确、画风一致、构图可用、缺陷率、延迟和每个合格结果的有效成本。

建议权重：

| 指标 | 权重 |
| --- | ---: |
| 人物身份与人数保真 | 35% |
| 画风及游戏可用性 | 20% |
| 构图、比例与裁切 | 15% |
| 画面缺陷 | 10% |
| 安全误拒与失败率 | 10% |
| 延迟 | 5% |
| 合格结果有效成本 | 5% |

```text
每个合格结果的有效成本 = 总调用成本 / 验收合格图片数
```

## 5. 不在当前范围内

- 不根据纯文字生成背景、封面或场景图片。
- 不在创建任务中使用 LLM 生成标题、剧情、提示或结尾文案。
- 不让模型生成 HTML、CSS、JavaScript、游戏规则或可执行代码。
- 不把图片模型返回的远程 URL 直接写入游戏配置；生成图片必须由 Worker 校验并保存到平台 S3。

## 6. 参考

- [OpenAI GPT Image 2](https://developers.openai.com/api/docs/models/gpt-image-2)
- [OpenAI Images API reference](https://developers.openai.com/api/reference/resources/images)
