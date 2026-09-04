# 游戏模板与运行时契约

## 文档信息

- 版本：v0.1
- 状态：设计基线；第一个 `love-journey@1.1.0` 运行时已实现，未落地部分明确标为后续目标
- 更新日期：2026-08-23
- 产品需求依据：[PRD_MVP_v0.1.md](./PRD_MVP_v0.1.md)
- 技术方案依据：[TECHNICAL_DESIGN_MVP_v0.1.md](./TECHNICAL_DESIGN_MVP_v0.1.md)
- 适用范围：受信任静态模板、基础游戏引擎、模板运行时、文件素材与文本槽位、游戏实例、游玩会话与版本兼容
- 不包含：图片生成方式、提示词、生成模型选择、3D 游戏和用户生成可执行代码

## 1. 核心结论

平台采用“模板独立、引擎共用”的结构：

- 每套模板是一份随前端发布、经过审核的独立 Vue 代码，可拥有自己的页面、剧情、关卡顺序、过场、视觉效果和专属交互。
- 一套模板可以组合拼图、翻牌、问答等多个基础游戏模块。
- 多套模板可以使用同一种基础玩法，但不强制共用该玩法的页面组件。
- 拼图切块、完成判断、翻牌匹配、计时等稳定规则由无模板视觉的基础引擎共用。
- 创建任务只生成或组装结构化配置与受控资源，不生成或执行 HTML、CSS、JavaScript、WebAssembly 或其他代码。
- 路由负责创建、预览和游玩等产品流程；模板内部当前关卡由模板运行时和游玩会话控制，不为每个玩法建立正式路由。

概念关系如下：

```text
共享基础游戏引擎
拼图规则、翻牌规则、计时、计分
             ↑ 使用
模板专属关卡页面
生日拼图页、太空拼图页、生日翻牌页
             ↑ 编排
模板
页面、剧情、关卡流程、主题与素材角色
             ↓ 实例化
游戏版本
templateId + templateVersion + 配置 + 素材引用
             ↓ 被游玩
游玩会话
当前关卡、关卡结果、总分与有效期
```

## 2. 术语和身份

### 2.1 基础游戏引擎 `GameEngine`

一个不包含模板视觉的玩法规则实现，例如 `puzzle` 或 `memory-match`。引擎可以使用 Vue Composable、纯 TypeScript 状态机或适合该玩法的 2D 库实现。

### 2.2 模板关卡页面 `StageRenderer`

模板拥有的 Vue 页面组件。它决定布局、视觉、文案、动画和交互反馈，并调用基础游戏引擎完成具体玩法。

### 2.3 模板 `GameTemplate`

一套完整的游戏体验。模板包含模板清单、模板根组件和独立关卡页面，可以组合多个基础游戏引擎。

### 2.4 游戏版本 `GameVersion`

某个模板被用户素材和配置实例化后的不可变版本。已经分享的游戏版本不得被模板升级静默改变。

### 2.5 游玩会话 `PlaySession`

接收方某一次游玩的临时状态，记录当前关卡、关卡结果和整体完成状态。它与可被多次游玩的游戏版本分开保存。

## 3. 职责边界

| 能力 | 平台运行时 | 模板代码 | 基础游戏引擎 |
|---|---|---|---|
| 模板加载和版本解析 | 负责 | 声明入口 | 不负责 |
| 页面和主题 | 提供舞台边界 | 负责 | 不负责 |
| 剧情和关卡顺序 | 建立会话与记录整体完成；关卡内状态当前仅在前端内存中 | 负责定义并在本地推进 | 不负责 |
| 拼图、翻牌等规则 | 不负责具体规则 | 调用 | 负责 |
| 素材授权和 URL 解析 | 负责 | 只消费已解析资源 | 不负责 |
| 进度和会话保存 | 当前保存会话开始/完成；后续才保存关卡检查点 | 当前只上报整体完成 | 只输出可保存状态 |
| 全局音频解锁和管理 | 负责 | 请求播放 | 不直接操作全局音频 |
| 暂停、恢复和销毁 | 统一触发 | 响应 | 提供能力并清理资源 |
| 错误隔离和上报 | 负责 | 提供可理解降级 UI | 返回稳定错误 |
| 计分规则 | 提供聚合能力 | 决定模板语义 | 提供玩法原始结果 |

模板不得绕过运行时直接读取认证状态、访问对象存储内部地址或自行调用进度保存接口。基础引擎不得依赖某个模板的主题、页面组件或剧情文案。

## 4. 模板注册契约

模板通过受信任注册表发布，不从游戏配置中的任意 URL 加载代码。

```ts
export interface TemplateDefinition {
  id: string
  version: string
  manifest: TemplateManifest
  load: () => Promise<{ default: TemplateRootComponent }>
}

export interface TemplateManifest {
  displayName: string
  orientation: 'portrait' | 'landscape'
  assetSlots: AssetSlotDefinition[]
  textSlots: TextSlotDefinition[]
  stages: StageDefinition[]
  initialStageId: string
}
```

注册表必须使用动态导入，避免打开一个游戏时下载所有模板代码：

```ts
export const templateRegistry: Record<string, TemplateDefinition> = {
  'birthday-adventure@1.0.0': {
    id: 'birthday-adventure',
    version: '1.0.0',
    manifest: birthdayAdventureManifest,
    load: () => import('./templates/birthday-adventure/v1'),
  },
}
```

`templateId + templateVersion` 共同确定唯一模板入口。找不到精确版本时必须返回“不支持的模板版本”，不能自动回退到最新版。

同一个模板版本必须同时登记在三个位置：

- 前端受信任注册表：负责动态加载模板代码。
- 后端模板允许列表：负责判断能否创建、发布、预览和游玩。
- `contracts/game-config/templates/`：保存该精确版本的配置与素材 Schema。

CI 必须检查三处 `templateId + templateVersion` 集合一致。模板状态至少区分：

- `active`：允许创建新游戏，也允许历史版本游玩。
- `retired`：不再出现在新建模板列表，但历史游戏仍可精确加载。
- `blocked`：因安全或严重故障暂停游玩；不能把它静默回退到其他版本。

## 5. 模板入口契约

每套模板只暴露一个根入口，平台向其传递已校验的游戏版本和受控运行时能力：

```ts
export interface TemplateProps {
  game: RuntimeGameInstance
  session: RuntimeSessionSnapshot
  runtime: TemplateRuntime
}

export interface TemplateRuntime {
  enterStage(stageId: string): Promise<void>
  completeStage(stageId: string, result: StageResult): Promise<void>
  completeGame(result: GameResult): Promise<void>
  saveCheckpoint(stageId: string, checkpoint: unknown): Promise<void>
  resolveAsset(assetKey: string): RuntimeAsset | undefined
  playSound(soundId: string): void
  pause(): void
  resume(): void
  reportError(error: TemplateRuntimeError): void
}
```

平台运行时拥有会话、资源授权和错误隔离；模板根组件拥有具体页面和关卡编排。模板不得把 `TemplateRuntime` 保存到全局单例。

## 6. 关卡与流程契约

一套模板可以包含开场、剧情、多个游戏关卡和结算页：

```ts
export interface StageDefinition {
  id: string
  type: 'intro' | 'story' | 'game' | 'result'
  renderer: string
  engine?: string
  assetBindings?: Record<string, string | string[]>
  config?: Record<string, unknown>
  nextStageId?: string
}
```

示例：

```json
{
  "initialStageId": "opening",
  "stages": [
    {
      "id": "opening",
      "type": "intro",
      "renderer": "birthday/opening",
      "nextStageId": "portrait-puzzle"
    },
    {
      "id": "portrait-puzzle",
      "type": "game",
      "engine": "puzzle@1",
      "renderer": "birthday/puzzle",
      "assetBindings": {
        "image": "main-character"
      },
      "config": {
        "rows": 3,
        "columns": 3
      },
      "nextStageId": "friends-memory"
    },
    {
      "id": "friends-memory",
      "type": "game",
      "engine": "memory-match@1",
      "renderer": "birthday/memory",
      "assetBindings": {
        "cards": "friends"
      },
      "nextStageId": "ending"
    },
    {
      "id": "ending",
      "type": "result",
      "renderer": "birthday/ending"
    }
  ]
}
```

v0.1 只要求线性流程。模板可以在自己的代码中实现视觉过场，但不能通过修改 URL 绕过 `TemplateRuntime.enterStage`。分支条件、可视化流程编辑器和用户自定义关卡顺序不属于本契约 v0.1。

## 7. 基础游戏引擎契约

基础游戏引擎优先采用“无页面引擎”，向模板暴露状态和动作：

```ts
export interface EngineLifecycle {
  start(): void
  pause(): void
  resume(): void
  reset(): void
  destroy(): void
}

export interface EngineResult {
  success: boolean
  score?: number
  durationMs: number
  metadata?: Record<string, unknown>
}
```

以拼图为例，引擎可以表现为：

```ts
const puzzle = usePuzzleEngine({
  rows: 3,
  columns: 3,
  image: props.assets.image,
})

puzzle.pieces
puzzle.movePiece(from, to)
puzzle.elapsedMs
puzzle.completed
puzzle.pause()
puzzle.destroy()
```

同一拼图引擎可以被 `BirthdayPuzzleStage.vue` 和 `SpacePuzzleStage.vue` 使用，但两者无需共享页面组件。复用顺序为：

1. 视觉差异小：共享页面结构，使用主题变量。
2. 视觉差异中等：使用插槽或有限变体。
3. 页面结构和交互差异大：保留独立关卡页面，只共享引擎。

不得为消除少量 UI 重复而建立包含大量布尔参数和布局参数的“万能玩法组件”。

当前 `frontend/src/game-runtime/engines/puzzle/puzzleState.ts` 已实现无模板视觉的纯状态规则，负责拼图块、目标槽位、合法放置和全部完成判断；`love-journey@1.1.0` 在前四个主体验结尾复用拼图页面，按 5、4、3、2 块逐次降低难度，页面负责互补凸榫/凹口轮廓、目标虚线提示、拖放、触摸、键盘替代操作和黑白线框视觉。拼图不单独占用关卡进度，密码体验后不再追加。该实现尚不包含图片切块、计时以及上述完整生命周期接口，不能视为完整通用拼图引擎已经冻结。

模板必须引用精确的引擎主版本，例如 `puzzle@1`。只修复不改变规则结果的缺陷时可以保持引擎版本；会改变完成判断、计分、可保存状态或交互语义的修改必须发布新引擎版本，并由模板新版本显式采用。历史模板不能因共享引擎升级而静默改变玩法。

## 8. 素材与文本槽位契约

模板清单必须声明图片、音频等文件素材的角色和约束，也必须声明用户配置文本需要填入的槽位。创建器、Worker、API 和前端运行时共同校验：

```ts
export interface AssetSlotDefinition {
  key: string
  type: 'image' | 'image-list' | 'audio'
  required: boolean
  minCount?: number
  maxCount?: number
  aspectRatio?: number
  preferredWidth?: number
  preferredHeight?: number
  allowTransparency?: boolean
}

export interface TextSlotDefinition {
  key: string
  type: 'text' | 'text-list'
  purpose: 'title' | 'intro' | 'story' | 'instruction' | 'result' | 'other'
  required: boolean
  minCount?: number
  maxCount?: number
  minLength?: number
  maxLength: number
}

export interface RuntimeAsset {
  key: string
  type: 'image' | 'audio'
  url: string
  mimeType: string
  expiresAt: string
  width?: number
  height?: number
}
```

约束如下：

- 配置只保存资源 ID 或槽位引用，不保存任意外部 URL。
- 创建任务只允许把用户输入写入 `textSlots` 声明的纯文本字段，不得新增模板未声明的文本槽位；不调用 AI 自动生成文案。
- 配置文本以结构化字段写入该模板版本的 `config`，字段数量、类型和长度必须同时满足模板清单及 JSON Schema。
- 配置文本不得包含 HTML、JavaScript、外部资源地址或其他可执行内容；模板负责最终排版和视觉呈现。
- API 在有效的预览或游玩边界内把资源 ID 转换成短期 URL。
- 模板只能读取清单白名单中的渲染资源，不能读取用户原始上传资源。
- 同一个共享素材可以被多个关卡绑定，不必复制对象。
- 必需素材缺失、数量不符合要求或比例无法安全适配时拒绝发布。
- 单项非关键素材加载失败时允许模板提供占位或降级；核心玩法素材失败时终止当前关卡并显示可理解错误。

## 9. 游戏配置契约

游戏配置使用稳定信封，模板内部配置再由精确的模板版本 Schema 校验：

```ts
export interface GameConfigDocument {
  templateId: string
  templateVersion: string
  configVersion: number
  config: Record<string, unknown>
  assetBindings: Record<string, string | string[]>
}
```

其中 `configVersion` 表示稳定配置信封的结构版本；模板内部字段由 `templateId + templateVersion` 对应的 Schema 决定。三者不能互相替代。

校验顺序：

1. 校验信封字段和未知字段。
2. 用 `templateId + templateVersion` 查找受支持模板。
3. 使用该模板对应的配置 Schema 校验 `config`。
4. 使用模板清单校验文本槽位的类型、数量和长度。
5. 使用模板清单校验素材槽位、类型、数量和引用归属。
6. 验证成功后才能将游戏版本标记为 `ready`。
7. 预览和公开游玩接口读取产物后再次执行兼容性校验。

当前仓库中的 `contracts/game-config/v1.schema.json` 已实现稳定版本信封，并校验 `love-journey@1.1.0` 的情书、4 位数字密码与可选密码提示；运行时资源由预览或公开游玩接口在授权边界内以独立 `assets` 列表返回。当前还没有把各模板的配置拆成单独 Schema 文件，也没有在中间四段按槽位消费个性化素材。`memory-game@1.0.0` 与 `love-journey@1.0.0` 仅作为历史代码存在，不是新建流程基线。

## 10. 会话和进度契约

游戏版本与某一次游玩必须分离：

```ts
export interface RuntimeSessionSnapshot {
  sessionId: string
  gameVersionId: string
  currentStageId: string
  status: 'playing' | 'completed' | 'abandoned'
  stageResults: Record<string, StageResult>
  totalScore?: number
  expiresAt: string
}

export interface StageResult {
  success: boolean
  score?: number
  durationMs: number
  metadata?: Record<string, unknown>
}
```

目标策略与当前差异：

- 当前平台只记录游玩会话的开始、整体完成和重玩事件，不保存关卡检查点。
- 当前五段关卡状态只存在于模板组件内存；刷新后从第一段重新开始。
- 后续如增加关卡保存，模板可以提交关卡边界检查点，但不得假设每一步都已上传服务端。
- “再玩一次”创建新的局内状态，不修改原始游戏版本。
- 分享自然到期后，已开始的一局仍遵循现有 30 分钟会话上限。
- “我的游戏”不显示或恢复任何游玩/生成检查点；生成运行只能在提交后进入对应运行路由。

## 11. 路由契约

正式产品路由保持稳定：

```text
/app/create                       选择模板并开始创建
/app/games                        我的游戏；卡片仅试玩、修改、分享、删除
/app/games/:gameId/edit           修改当前游戏并创建新版本
/app/games/:gameId/generation/:runId  查看本次提交的生成状态
/app/games/:gameId/preview        制作方预览
/app/games/:gameId/share          创建本次分享链接和二维码
/play/:publicId                   接收方开场与游玩
```

拼图、翻牌和模板内部关卡不是正式路由。当前关卡由游玩会话和模板运行时决定。内部开发工具可以通过查询参数直接预览关卡，但该能力不得出现在公开游玩流程中，也不得写入正式会话结果。

进入模板实际游玩状态后，现有预览或公开路由保持不变，运行时建立覆盖整个浏览器可视区域的沉浸式舞台。制作方预览的舞台覆盖制作端导航与页面工具栏，并提供返回“我的游戏”的独立退出控件；公开游玩舞台覆盖开场页卡片，不展示制作方控件。完成、退出或模板卸载时必须移除舞台并恢复原页面滚动状态。

## 12. 生命周期与平台能力

平台必须统一处理：

- 开始按钮触发后的浏览器音频解锁。
- 页面隐藏、系统中断和用户暂停。
- 模板或关卡卸载时的定时器、动画帧、音频、Canvas 和事件监听清理。
- 短期素材 URL 续签。
- 模板级错误边界和稳定错误码。
- 手机安全区域、方向约束和基础舞台尺寸。
- 全视口舞台、背景滚动锁定，以及制作方试玩退出控件的安全区域定位。

模板负责响应平台生命周期，但不能覆盖平台的认证、会话有效期和资源授权判断。

### 12.1 全视口舞台约束

- `playing` 状态的舞台覆盖整个浏览器可视区域，不能继续露出产品导航、外层卡片或页面背景内容。
- 手机端模板占满视口；桌面端竖屏模板可以保持清单规定的最大宽度并居中，但其外部仍属于全屏游戏层。
- 全屏游戏层锁定页面级滚动；模板内部只有明确声明的内容区域可以局部滚动。
- 制作方预览必须提供最小 `44×44px` 的退出目标，并通过 `safe-area-inset-*` 避开刘海、圆角和系统手势区域。
- 退出控件不得遮挡关卡进度、主要操作和完成按钮；公开游玩不显示该制作方控件。
- 本契约中的“全视口”指覆盖浏览器可视区域，不要求调用浏览器 Fullscreen API，也不能依赖用户授予系统级全屏权限。

### 12.2 客户端截图导出约束

模板可以在明确的用户点击后，把受信任的游戏舞台导出为图片并直接下载，但不得默认截取整个系统屏幕或请求不必要的屏幕共享权限。当前 `love-journey@1.1.0` 只在最终密码体验完成后提供 PNG 导出。

- 截图生成在浏览器本地完成，不上传服务器，也不写入游戏版本或游玩会话。
- 保存按钮、生成状态、制作方退出控件等非游戏内容必须从导出画面排除。
- 输出尺寸必须设上限，避免高倍屏或超长页面导致内存耗尽；当前模板最高使用 2 倍像素密度。
- 文件名必须过滤路径分隔符、控制字符和其他非法字符。
- 模板只能截取当前授权渲染的资源；跨域图片必须满足 CORS，短期签名 URL 不能因截图缓存参数而被修改。
- 导出前必须验证画布包含可见内容；透明或全白结果不得作为成功截图下载。
- 生成或下载失败时保留当前游戏状态，显示可理解提示并允许重试。

## 13. 版本与当前数据策略

- 游戏版本创建后固定 `templateId` 和 `templateVersion`。
- 用户更换模板时创建新的游戏版本，不原地修改已经完成或分享的版本。
- 模板的不兼容修改必须发布新版本。
- 已分享游戏不得自动使用新模板版本。
- 当前产品不承诺旧模板或旧数据兼容；开发数据可以通过受保护的重置脚本清空后从 `love-journey@1.1.0` 重新开始。
- 代码中保留的旧模板分支只视为历史实现，不形成“我的游戏”恢复入口或迁移责任。
- 如果未来重新引入兼容期，必须同时覆盖配置 Schema、素材槽位、默认值和会话状态，并单独定义验收范围。

样式微调、文案修正等是否升级模板版本，由其是否改变历史游戏体验或配置兼容性决定；不能仅依靠语义版本号推断兼容，运行时仍必须精确匹配注册项。

## 14. 建议目录

```text
frontend/src/game-runtime/
├── runtime/
│   ├── GameRuntime.vue
│   ├── runtime-context.ts
│   ├── session-store.ts
│   ├── asset-loader.ts
│   └── errors.ts
├── engines/
│   ├── puzzle/
│   └── memory-match/
├── templates/
│   ├── registry.ts
│   ├── birthday-adventure/
│   │   └── v1/
│   │       ├── index.ts
│   │       ├── manifest.ts
│   │       ├── BirthdayTemplate.vue
│   │       └── stages/
│   └── space-adventure/
│       └── v1/
└── devtools/
    └── TemplatePreview.vue

contracts/game-config/
├── envelope/
│   └── v1.schema.json
└── templates/
    ├── birthday-adventure/
    │   └── 1.0.0.schema.json
    └── space-adventure/
        └── 1.0.0.schema.json
```

上述目录是迁移目标，不代表当前代码已经实现。

## 15. 验证和测试

每个基础引擎至少覆盖：

- 正常完成、重置、暂停、恢复和销毁。
- 重复输入、快速点击和页面卸载时不会继续更新状态。
- 计时和完成判断具有确定性。

每套模板至少覆盖：

- 注册表可以按精确版本加载。
- 所有关卡可从入口走到结算。
- 素材绑定符合清单，缺失素材有明确结果。
- 页面刷新按当前实现从第一段重新开始；若未来增加关卡检查点，再补充对应恢复测试。
- 不同手机视口、安全区域和目标方向。
- 模板异常被错误边界隔离。

平台端到端测试至少覆盖：

- 选择模板、创建、预览、分享和公开游玩。
- 同一种引擎被两套视觉不同的模板使用。
- 新建流程只产生当前冻结模板版本；旧版本兼容不纳入当前验收。
- 未知模板、未知版本、非法配置和非法素材引用被拒绝。

## 16. v0.1 实现约束

第一阶段实现保持以下边界：

- 模板由开发者编写并随前端发布，用户不能上传或编辑模板代码。
- 每套模板拥有独立代码目录，可以包含多个基础玩法。
- 基础玩法只共享规则与状态，不要求模板共享关卡页面。
- 模板流程先采用线性关卡。
- 优先支持手机竖屏和基础 2D 交互。
- 在确有连续精灵动画或碰撞需求前，优先使用 Vue、DOM 和 CSS；需要时可由单个引擎引入合适的 2D 库。
- 进度以关卡边界保存为主，不建设通用实时同步引擎。
- 模板按需加载，单个模板故障不能影响制作端或其他模板。
