# 用户行为事件契约

## 文档信息

- 版本：v0.1
- 状态：G1–G8 PASS
- 更新日期：2026-08-23
- 适用范围：MVP 用户行为事件、上报 API、管理员查询、隐私和保留边界
- 产品边界依据：[PRD_MVP_v0.1.md](./PRD_MVP_v0.1.md)
- 技术基线依据：[TECHNICAL_DESIGN_MVP_v0.1.md](./TECHNICAL_DESIGN_MVP_v0.1.md)

本文是 Analytics 实现的冻结字段与行为基线。G1 独立复查已通过；后续阶段如发生实际设计变化，必须重新评审，且 OpenAPI、后端常量、前端类型和测试必须与本文保持一致。

## 1. 范围与非目标

第一版记录关键制作、生成、分享和公开游玩事件，并向管理员提供脱敏查询。记录用于内部观察和故障排查，不属于 `admin_audit_logs` 管理员安全审计，也不向制作方提供分享访问统计或“对方已打开”通知。

第一版不接入第三方分析平台，不做漏斗、留存、BI 大屏、热力图、屏幕录像或任意点击采集。Analytics 失败不得阻断注册、登录、游戏创建、生成、分享或游玩主流程。

## 2. 术语与身份

| 术语 | 含义 |
|---|---|
| Creator ID / `creatorId` | `users.id`，服务端生成的 26 位 ULID；数据库列名为 `user_id` |
| 登录 ID / `loginId` | 用户自选、规范化后的 `users.login_id`；不是内部主键 |
| User Session ID | `user_sessions.id`，只用于内部关联，不是 Cookie 中的原始凭据 |
| Play Session ID | `play_sessions.id`，只用于内部关联，不是 Cookie 中的原始凭据 |
| `createdAt` | 服务端接收并写入事件的 UTC 时间，也是默认排序和时间筛选字段 |
| `occurredAt` | 前端可选提供的客户端发生时间，仅供展示，不参与认证、授权、有效期或排序判断 |

API、数据库和管理页面不得用含糊的 `userId` 同时表示 Creator ID 与登录 ID。事件表只保存内部 `user_id`，不保存 `login_id` 副本。管理员查询如返回 `loginId`，必须实时从 `users` 表参数化联查，并仅放在管理员 DTO 的顶层白名单字段中。

规范命名为：认证请求和制作方资料 DTO 使用 `loginId`，管理员用户资源路径参数和内部用户筛选使用 `creatorId`。当前代码/OpenAPI 中认证 `userId` 表示登录 ID、管理员 `{userId}` 表示内部 ULID，是已识别但尚未由 T1 修改的历史差异；后续获准的 API 同步必须迁移为上述规范名。Analytics 请求、响应、筛选和页面一律不接受或返回 `userId` 别名。

客户端不得声明 Creator ID、登录 ID、游戏、版本、生成任务、分享或 Session 关联。所有关联均由服务端认证上下文、已校验业务记录或 Worker 当前任务推导。

## 3. 通用事件模型

### 3.1 服务端持久化字段

| 字段 | 必填 | 规则 |
|---|---:|---|
| `id` | 是 | 服务端生成的 26 位 ULID |
| `eventName` | 是 | 第 5 节候选字典中的唯一 `domain.action` 名称，最大 64 个 ASCII 字符 |
| `source` | 是 | `frontend`、`api`、`worker` 之一；由记录入口设置，客户端不可覆盖 |
| `actorType` | 是 | `creator`、`receiver`、`system` 之一；由事件定义设置 |
| `creatorId` / `user_id` | 依事件 | 内部 Creator ULID；不得使用登录 ID 替代 |
| `userSessionId` | 依事件 | 已验证制作方 Session 的内部 ULID；不得保存 Cookie 或 Token |
| `gameId` | 依事件 | 服务端可信业务记录中的 ULID |
| `gameVersionId` | 依事件 | 服务端可信业务记录中的 ULID |
| `generationRunId` | 依事件 | 服务端可信业务记录中的 ULID |
| `shareId` | 依事件 | `share_links.id`，不是 `public_id` 或分享 Secret |
| `playSessionId` | 依事件 | `play_sessions.id`，不是 Cookie 或 Token |
| `requestId` | 否 | 当前 HTTP 请求的服务端请求 ULID；Worker 事件没有 HTTP 请求时为空 |
| `clientEventId` | 前端事件必填 | 第 3.2 节定义的 UUID；服务端可信业务事件为空 |
| `properties` | 是 | JSON 对象；只接受第 5 节对应事件的字段，最大 4096 UTF-8 字节 |
| `occurredAt` | 否 | 前端事件可提供的 RFC 3339 时间，写入前规范化为 UTC |
| `createdAt` | 是 | 服务端写入时间，UTC `DATETIME(6)`；客户端不可提供或覆盖 |

关联字段缺失时不得让客户端补充受信身份。实现应先从可信上下文取得；如果某一可选关联无法安全获得，则留空并在契约中同步说明。第 5 节标为必填的关联无法取得时，不得伪造或降级为客户端值。

### 3.2 `clientEventId` 与幂等

- 仅前端事件使用，且请求中必填。
- 格式为 `crypto.randomUUID()` 产生的规范小写 UUID v4：`xxxxxxxx-xxxx-4xxx-[89ab]xxx-xxxxxxxxxxxx`。
- 固定长度 36 个 ASCII 字符，数据库中非空值全局唯一；不接受大写、无连字符或其他自定义 ID。
- 首次接受时创建事件。相同 `clientEventId`、相同事件名、相同白名单属性且可信关联一致的重试返回原事件，`duplicate=true`，不得插入第二条记录。
- 同一 `clientEventId` 被用于不同事件名、属性或可信关联时返回 `409 CLIENT_EVENT_ID_CONFLICT`，不得覆盖原事件或插入新记录。
- 幂等结果不使 `clientEventId` 成为身份凭据；每次请求仍需重新完成 Session、Origin、CSRF 或公开会话校验。

### 3.3 `properties` 编码与校验

- 必须是 JSON 对象；没有属性的事件使用空对象 `{}`，不接受数组、标量或 `null`。
- 按无额外空白的 UTF-8 JSON 编码计算大小，最多 4096 字节；超过限制返回校验错误。
- 每个事件拒绝未知属性、重复 JSON 键、错误类型、越界数值和不符合格式的字符串。
- 不允许任意嵌套对象或数组。第一版所有允许值都是字符串、整数或布尔值。
- 关联 ID、身份字段和请求 ID 是顶层服务端字段，不得复制进 `properties`。

### 3.4 来源与行为主体

| 事件类别 | `source` | `actorType` |
|---|---|---|
| 制作方页面、公开完成和重玩 | `frontend` | 制作方页面为 `creator`；公开游玩为 `receiver` |
| API 成功边界 | `api` | 制作业务为 `creator`；公开打开/开始为 `receiver` |
| Worker 最终结果 | `worker` | `system` |

## 4. 属性类型和值域

| 属性 | 类型与约束 |
|---|---|
| `page` | 必填字符串；仅 `create`、`games`、`game-edit`、`game-preview`、`game-share`、`generation-progress`、`settings` |
| `templateId` | 必填字符串；1–64 个字符，匹配 `^[a-z][a-z0-9-]{0,63}$` |
| `versionNumber` | 必填整数；1–4294967295 |
| `kind` | 必填字符串；`game_source` 或 `game_cover` |
| `mimeType` | 必填字符串；服务端识别的实际媒体类型，1–128 个可打印 ASCII 字符，不接受参数或用户文件名 |
| `sizeBytes` | 必填整数；0–9223372036854775807 |
| `attemptNumber` | 必填整数；1–4294967295 |
| `deduplicated` | 必填布尔值；幂等命中既有生成任务时为 `true` |
| `executionCount` | 必填整数；1–4294967295 |
| `errorCode` | 必填字符串；仅第 4.1 节白名单 |
| `retryable` | 必填布尔值 |
| `lifetimeDays` | 必填整数；1–90；按分享创建时间到到期时间的持续时长向上取整到完整 24 小时 |
| `mode` | 必填字符串；第一版仅 `public` |

`mimeType` 来自服务端内容识别结果而不是浏览器提交值。字符串属性不得包含换行或控制字符。

### 4.1 `generation.failed.errorCode` 白名单

```text
INPUT_VALIDATION_FAILED
ASSET_READ_FAILED
GENERATION_TIMEOUT
PROVIDER_RATE_LIMITED
PROVIDER_UNAVAILABLE
CONTENT_REJECTED
ASSET_GENERATION_FAILED
GAME_CONFIG_ASSEMBLY_FAILED
GAME_CONFIG_INVALID
TEMPLATE_COMPATIBILITY_FAILED
GAME_VALIDATION_FAILED
STORAGE_WRITE_FAILED
STORAGE_CAPACITY_UNAVAILABLE
TASK_LEASE_EXHAUSTED
INTERNAL_ERROR
```

只记录稳定错误代码，不记录用户素材、提示词、模型响应、堆栈或管理员说明正文。

## 5. 事件字典

表中的“必填关联”均由服务端推导；“允许属性”中的字段全部必填，未列出的属性一律拒绝。

| 事件名 | 触发时机 | 记录端 | `source` / `actorType` | 必填关联 | 允许属性 |
|---|---|---|---|---|---|
| `creator.page_viewed` | 已登录制作方完成一次有效制作端路由跳转 | 前端上报 | `frontend` / `creator` | Creator、User Session | `page` |
| `creator.registered` | 用户账号成功创建并提交后 | API best-effort | `api` / `creator` | Creator | 无 |
| `creator.logged_in` | 用户 Session 成功创建并提交后 | API best-effort | `api` / `creator` | Creator、User Session | 无 |
| `game.created` | 游戏草稿和首版本事务成功提交后 | API best-effort | `api` / `creator` | Creator、Game、Version | `templateId` |
| `game.version_created` | 新版本事务成功提交后 | API best-effort | `api` / `creator` | Creator、Game、Version | `versionNumber`、`templateId` |
| `asset.uploaded` | 素材对象和数据库元数据均保存成功后 | API best-effort | `api` / `creator` | Creator、Game、Version | `kind`、`mimeType`、`sizeBytes` |
| `generation.submitted` | 创建任务成功，或幂等请求返回既有任务后 | API best-effort | `api` / `creator` | Creator、Game、Version、Run | `attemptNumber`、`deduplicated` |
| `generation.succeeded` | Worker 成功提交任务最终状态后 | Worker best-effort | `worker` / `system` | Creator、Game、Version、Run | `executionCount` |
| `generation.failed` | Worker 成功提交最终失败状态后；临时重试不得记录 | Worker best-effort | `worker` / `system` | Creator、Game、Version、Run | `errorCode`、`retryable`、`executionCount` |
| `share.created` | 分享链接事务成功提交后 | API best-effort | `api` / `creator` | Creator、Game、Version、Share | `lifetimeDays` |
| `share.opened` | `POST /public/shares/{publicId}/resolve` 成功校验 Secret 并准备返回公开信息后；每个成功 resolve 请求一次 | API best-effort | `api` / `receiver` | Game、Version、Share | 无 |
| `play.started` | 公开游玩 Session 成功创建并提交后 | API best-effort | `api` / `receiver` | Game、Version、Share、Play Session | 无 |
| `play.completed` | 接收方完成当前公开游戏 | 前端上报 | `frontend` / `receiver` | Game、Version、Share、Play Session | `mode` |
| `play.replayed` | 接收方在完成页点击再次游玩 | 前端上报 | `frontend` / `receiver` | Game、Version、Share、Play Session | `mode` |

`generation.submitted` 对每个成功请求记录一次：新建任务时 `deduplicated=false`，幂等返回既有任务时 `deduplicated=true`。业务请求本身失败时不记录成功事件。

### 5.1 `share.opened` 唯一触发边界

- 只允许 `resolveShare` Handler 在 `validatePublicShare` 成功返回之后、发送成功公开信息响应的边界记录一次。
- `createPlaySession` 即使复用 `validatePublicShare` 再次校验 Secret，也不得记录 `share.opened`；它只在游玩 Session 成功创建后记录一次 `play.started`。
- `validatePublicShare` 等共享校验函数必须保持无 Analytics 副作用，避免一次“打开并开始”流程因两次 Secret 校验产生重复 `share.opened`。
- 接收方主动重新发起一次成功 resolve 属于新的打开请求，可以产生新的 `share.opened`；同一请求内部不得重复记录。

## 6. 可信字段来源

| 事件 | 可信来源 |
|---|---|
| `creator.page_viewed` | `RequireCreatorSession` 中的 `UserSession.ID` 与 `User.ID`；请求体只提供事件名、客户端事件 ID、时间和 `page` |
| `creator.registered` | 成功创建的 `users.id` |
| `creator.logged_in` | 成功创建的 `user_sessions.id` 及其 `users.id` |
| `game.created`、`game.version_created` | 已提交的 Games Repository 返回值和认证 Creator |
| `asset.uploaded` | 完成对象存储和元数据保存后的 Asset、Game、Version 与认证 Creator |
| `generation.submitted` | Generation Repository 返回的 Run、关联 Game/Version 与认证 Creator |
| `generation.succeeded`、`generation.failed` | Worker 当前持久化 Run 及其 Repository 联查得到的 Creator/Game/Version |
| `share.created` | Sharing Repository 返回的 Share 与认证 Creator |
| `share.opened` | 仅 `resolveShare` 成功响应边界中的已校验 Share；共享校验函数和 `createPlaySession` 不触发 |
| `play.started` | 新建 Play Session 及其 Share/Game/Version 联查结果 |
| `play.completed`、`play.replayed` | 当前公开 Session Cookie 哈希查询到的有效 Play Session 及其 Share/Game/Version 联查结果 |

当前实现已从认证 Session、Games、Generation 和 Sharing Repository 取得上述关联，并通过安全读取辅助函数导出当前 Session ID；没有信任前端身份，也没有扩大现有业务表职责。

## 7. 上报 API

T4 已实现本节两个上报端点与第 8 节管理员查询端点，并按第 9 节 Surface 矩阵完成装配；以下契约字段由严格请求 DTO、可信服务端关联和显式管理员响应 DTO 强制执行。首轮独立审查发现的仓内自动化证据与 OpenAPI 429 缺口均已修复，G4 已通过。

### 7.1 请求体

两个前端上报端点共用以下形状：

```json
{
  "eventName": "creator.page_viewed",
  "clientEventId": "9b2ce6ac-8d0f-4afb-8c47-a331707861ea",
  "occurredAt": "2026-08-16T02:35:01.123Z",
  "properties": {
    "page": "game-edit"
  }
}
```

- JSON Body 拒绝未知顶层字段。
- `occurredAt` 可省略；若提供必须是 RFC 3339 时间。
- 请求体不得包含 `userId`、`creatorId`、`loginId`、`userSessionId`、`gameId`、`gameVersionId`、`generationRunId`、`shareId`、`playSessionId`、`requestId`、`source` 或 `actorType`。

### 7.2 制作方上报

```text
POST /api/v1/analytics/events
```

- 只在 `SERVICE_SURFACE=app|all` 挂载。
- 必须有有效制作方 Session、受信 `APP_BASE_URL` Origin 和有效 CSRF Header。
- 第一版只接受 `creator.page_viewed`。
- Creator 与 User Session 关联从认证上下文取得。

### 7.3 公开游玩上报

```text
POST /api/v1/public/play-sessions/current/events
```

- 只在 `SERVICE_SURFACE=play|all` 挂载，并经 `sharing.Handler.MountPublic` 接入。
- 必须有未过期、未撤销且所属游戏仍有效的公开游玩 Session。
- 必须通过 `PLAY_BASE_URL` Origin 校验和公开端点限流。
- 第一版只接受 `play.completed`、`play.replayed`。
- Game、Version、Share 和 Play Session 从当前 Cookie 对应的服务端记录取得；Cookie 原值不进入事件。

### 7.4 成功与错误语义

新事件返回 `201 Created`；合法幂等重试返回 `200 OK`：

```json
{
  "data": {
    "eventId": "01K...",
    "duplicate": false
  },
  "requestId": "01K..."
}
```

认证、Origin、CSRF、限流和 JSON 校验沿用项目稳定错误响应。无效事件、未知属性、越界值或无效时间返回 `400` 或 `422`；身份失效返回 `401`；`clientEventId` 冲突返回 `409`。事件上报失败由前端静默忽略，不弹出用户提示、不无限重试，也不改变路由或游戏状态。

## 8. 管理员查询契约

```text
GET /api/v1/admin/behavior-events
```

- 只在 `SERVICE_SURFACE=app|all` 挂载，要求有效管理员 Session。
- 默认按 `(createdAt DESC, id DESC)` 排序，不能按 `occurredAt` 排序。
- 查询参数均可选：`eventName`、`creatorId`、`loginId`、`gameId`、`source`、`from`、`to`、`cursor`、`limit`。
- `creatorId` 与 `gameId` 必须是 26 位 ULID；`loginId` 按账号规则规范化后通过 `users` 参数化联查进行精确匹配，不查询 `properties`。
- `source` 只接受 `frontend`、`api`、`worker`。
- `from` 为 `createdAt >= from` 的 RFC 3339 下界；`to` 为 `createdAt < to` 的 RFC 3339 上界；两者同时存在时必须满足 `from < to`。
- `limit` 默认 50，允许 1–100；越界返回校验错误，不静默扩大。
- 空结果返回 `items: []` 和 `nextCursor: null`，不是错误。

### 8.1 游标格式

游标是无填充 Base64URL 编码的 UTF-8 JSON：

```json
{"v":1,"createdAt":"2026-08-16T02:35:01.123456Z","id":"01K..."}
```

服务端把游标视为不透明输入，严格校验版本、字段集合、UTC RFC 3339 时间和 26 位 ULID。下一页只返回严格早于该 `(createdAt, id)` 的记录。游标不包含筛选条件；前端改变任一筛选条件时必须丢弃旧游标和旧列表。无更多结果时 `nextCursor` 为 `null`。

### 8.2 管理员 DTO

```json
{
  "data": {
    "items": [
      {
        "id": "01K...",
        "eventName": "game.created",
        "source": "api",
        "actorType": "creator",
        "creatorId": "01K...",
        "loginId": "creator_01",
        "userSessionId": null,
        "gameId": "01K...",
        "gameVersionId": "01K...",
        "generationRunId": null,
        "shareId": null,
        "playSessionId": null,
        "requestId": "01K...",
        "properties": {"templateId": "memory-game"},
        "occurredAt": null,
        "createdAt": "2026-08-16T02:35:01.123456Z"
      }
    ],
    "nextCursor": null
  },
  "requestId": "01K..."
}
```

所有可空关联以 JSON `null` 返回。`properties` 必须再次按事件白名单构造或校验，不能直接透传未知数据库内容。DTO 不返回密码、哈希、Cookie、Token、CSRF、Secret、完整 URL、Fragment、IP、完整 User-Agent、Bucket、对象路径、签名地址、用户输入或图片内容。

## 9. Surface 与进程装配

| `SERVICE_SURFACE` / 进程 | 制作方上报 | 管理员查询 | 公开游玩上报 | 可信业务记录 |
|---|---:|---:|---:|---|
| `app` API | 是 | 是 | 否 | 只记录制作方私有 API 成功事件 |
| `play` API | 否 | 否 | 是 | 只记录 `share.opened`、`play.started` 和公开前端事件 |
| `all` API | 是 | 是 | 是 | 同时记录两类事件，不重复挂载 |
| Worker | 不适用 | 不适用 | 不适用 | 独立创建 Recorder，记录生成最终结果 |

Analytics Repository/Recorder 在 API 的 Surface 分支之前创建，但只注入当前 Surface 实际启动的 Handler。`play` 不得因此初始化制作方认证、内容加密或管理员依赖；`app` 不得挂载公开路由。Worker 使用自己的数据库连接和 Recorder，不依赖 API 进程内存。

T4 的 API 入口已在 Surface 分支前创建共享 Repository；`app` 挂载制作方上报和管理员查询，`play` 只向 Public Sharing Handler 注入 Recorder，`all` 各挂载一次。制作方端点在现有 Session/CSRF 之外再限制为 APP Origin；公开端点继续复用 Sharing 的 PLAY Origin、限流和有效 Play Session 校验。管理员响应使用不含 `clientEventId` 的显式白名单 DTO。

T5A 已把同一个最小 `analytics.Recorder` 注入 Auth、Games、Generation 和 Sharing 的成功边界；API 与 Worker 分别创建数据库持久化 Recorder，不共享进程内对象。服务端现记录 `creator.registered`、`creator.logged_in`、`game.created`、`game.version_created`、`asset.uploaded`、`generation.submitted`、`generation.succeeded`、`generation.failed`、`share.created`、`share.opened` 和 `play.started`。`share.opened` 只在公开解析成功响应边界产生，共享 Secret 校验函数没有埋点副作用；Worker 取消、未到最终状态的重试和最终状态持久化失败均不记录成功或失败事件。

T5B 已提供统一前端 tracker：每个页面、完成或重玩事件生成一个小写 UUID v4 `clientEventId`，制作方请求复用现有 CSRF，公开请求复用同源 Play Cookie；请求不携带 Creator、登录 ID、Game、Share、Play Session 或 URL 身份信息。制作方页面事件只使用稳定路由名白名单，登录、管理员和公开页面均不触发；完成与重玩使用同步 phase 守卫避免快速重复点击产生重复事件。前端上报每次最多请求一次，失败被统一吞掉，不改变路由或游戏 phase，也不触发用户错误提示。

T6 已实现管理员行为记录页面 `/admin/behavior-events`，并保留原 `/admin` 创建任务诊断页。页面使用管理员身份守卫，按事件、Creator ID、登录 ID、Game ID、来源和服务端接收时间查询，默认 50 条；加载更多使用后端 cursor、按事件 ID 去重，筛选变化会清空旧列表和 cursor，并以请求代次拒绝迟到响应。`loginId` 只读取管理员 DTO 顶层字段；详情按事件名从 `properties` 正向选择冻结白名单键，不直接渲染未知数据库内容。320×800 使用单列卡片，1440×900 使用紧凑多列布局，两种真实浏览器视口均无横向溢出。

## 10. 写入失败与日志

- 服务端可信业务事件必须在主业务事务成功后 best-effort 写入；Analytics 写入不得加入主业务事务。
- Analytics 写入失败只记录结构化 warning，主业务响应、事务结果、Worker 最终状态和游戏状态转换保持不变。
- warning 只允许包含 `eventName`、`source`、稳定错误代码、可用的 `requestId` 或 `generationRunId`；不得输出完整 `properties`、登录 ID、用户原文、文件名、对象地址、Secret 或凭据。
- 前端上报 Promise 必须在统一 tracker 中吞掉失败，不触发 Element Plus 错误消息，不阻止页面跳转、完成状态或再次游玩。
- 第一版每个前端事件最多发起一次常规请求；浏览器或调用方因网络不确定性重试时必须复用同一 `clientEventId`，不得无限重试。

## 11. 数据保留与删除

### 11.1 当前 MVP 课程开发环境规则

- 行为事件保留到 game-gen development 环境被明确重置。
- 删除游戏、分享链接或游玩 Session 后，事件行继续保留，且不得阻止现有删除流程。
- `game_id`、`game_version_id`、`generation_run_id`、`share_link_id`、`play_session_id` 和 `user_session_id` 保存为无外键的 ULID 快照，因此业务记录删除后仍可用于历史定位。
- `user_id` 只关联内部 `users.id`，使用可空外键和 `ON DELETE SET NULL`；不复制 `login_id`。当前没有用户自助删除账号功能。
- 行为事件不保留游戏标题、用户昵称、登录 ID、用户输入、图片、分享 Secret 或其他敏感正文快照。

### 11.2 T2 数据库实现

以下为已完成的实现与最终 G2 验证事实；T2 独立复查与迁移回归均已通过。

- `000003_behavior_events` 已按第 3.1 节新增全部 16 个字段；所有 ULID、请求 ID 和客户端 UUID 列使用 ASCII 二进制排序。
- 数据库只为 `user_id` 建立指向 `users.id` 的可空外键，并使用 `ON DELETE SET NULL`。其余业务关联均为无外键快照。
- `client_event_id` 使用可空全局唯一键；MySQL 允许多个 `NULL`，非空重复值由唯一键拒绝。UUID v4 格式和幂等内容一致性由 T3 应用校验实现。
- 数据库 CHECK 约束限制 `source`、`actor_type`，并要求 `properties` 为 JSON 对象；事件级白名单和紧凑 JSON 4096 字节上限由 T3 应用校验实现。
- 查询索引覆盖 `(created_at, id)`、`(event_name, created_at, id)`、`(user_id, created_at, id)`、`(game_id, created_at, id)` 和 `(play_session_id, created_at, id)`。
- 真实 MySQL 8.4 迁移验证确认：可空关联可写；重复非空 `client_event_id` 被拒绝；删除 Game、Share 和 Play Session 不阻断且事件快照保留；删除 User 后 `user_id` 置空。

### 11.3 T3 核心模块实现

- `backend/internal/analytics` 已定义 14 个冻结事件常量、`Event`、`RecordInput`、`ListFilter`、版本化游标、最小 `Recorder` 和 no-op 实现，不依赖 HTTP 或认证模块。
- 写入前按事件严格校验 `source`、`actorType`、必填且唯一允许的可信关联、规范小写 UUID v4、ULID、客户端发生时间和逐事件属性白名单；拒绝重复 JSON 键、嵌套值、未知字段、非整数数值和紧凑编码超过 4096 字节的属性。
- `RecordEvent` 使用服务端 ULID 和 UTC 微秒时间写入；相同 `clientEventId` 的同语义重试返回原事件，不新增第二行，不同事件名、白名单属性或可信关联返回冲突。
- 管理员查询仅使用参数化普通列和 `LEFT JOIN users` 联查顶层 `loginId`；支持事件、Creator、登录 ID、游戏、来源和服务端接收时间筛选，默认 50、最大 100，并以 `(created_at DESC, id DESC)` 和无填充 Base64URL v1 游标稳定分页。
- 数据库读出的 `properties` 在返回前再次按事件白名单校验和紧凑化；未知属性不会透传。用户删除导致历史事件 `user_id` 置空时，查询不会重新强制原始写入关联完整性。

以上实现已通过任务级普通/race 测试、独立测试、独立复查和完整 G3；允许进入 HTTP/API 装配阶段。

### 11.4 上线前限制

自动过期清理、用户数据删除工作流、生产级保留期限以及备份中的删除传播规则均未实现，也不属于本轮。上线前必须由产品、技术和合规负责人确认并实现相应策略；本文不得被理解为已经具备自动清理或完整用户删除能力。

## 12. 隐私禁止项

行为事件、`properties`、公开/制作方 API 响应和 Analytics warning 日志均不得包含：

- `loginId` / `login_id`（管理员 DTO 顶层联查字段除外）。
- 用户输入原文、`memoryText`、生成文本正文、提示词或模型原始响应。
- 密码、密码哈希、Cookie、Session Token、CSRF Token、Authorization Header。
- 分享 Secret、完整分享链接、查询参数、URL Fragment 或完整 URL。
- IP、完整 User-Agent、原始图片、图片正文、原始文件名。
- MinIO Bucket、object key、内部对象地址或预签名 URL。

页面事件只能发送稳定路由名，不得发送 `path`、`fullPath`、query、hash 或 referrer。公开事件不得发送 `publicId`；服务端只保存内部 `shareId` 快照。

## 13. 测试与变更控制

后续实现至少覆盖：事件逐项合法/非法属性、409 冲突与幂等成功、4 KB 边界、可信身份推导、权限/Origin/CSRF/限流、Surface 路由隔离、稳定游标、空数组、删除后保留、best-effort 失败、前端非阻断，以及数据库/API/日志/UI 隐私搜索。

T7 已新增显式 opt-in 的隔离端到端验收：使用独立 MySQL schema、真实 MinIO、`SERVICE_SURFACE=all` 主链 API、成功与最终失败 Worker、随机端口 Vite 和独立 `ego-browser` task-space 完成真实制作方、公开游玩和管理员 UI 链；另实际启动 `app|play|all` 验证 Surface 路由矩阵。最终数据库与管理员事件卡片都核验 14 类事件；浏览器前后数据库增量进一步证明 `create`、`games` 页面事件和 `play.completed`、`play.replayed` 两个公开事件来自真实 UI，且关联与白名单属性正确。删除详情页后，`game-edit`、`game-preview`、`game-share` 和 `generation-progress` 继续作为合法制作方页面名由路由统一上报。

同一验收还覆盖幂等、防伪、已建 Session 后分享过期/撤销/游戏删除中失效、稳定分页、删除后快照、Analytics 故障非阻断和全部隐私禁止项。测试默认安全跳过，仅在 `ANALYTICS_E2E=1` 时运行；Vite、browser、API 和 Worker 子进程使用最小白名单环境。正常退出、失败或活跃浏览器期间收到外部 TERM 时，脚本都会在发信号前快照完整后代并结合已登记进程组进行清理，同时移除隔离 schema、精确 QA 对象前缀和浏览器 task-space；最终独立复测确认这些残留计数均为 0，且既有开发端口进程未变化。

端到端验收发现并修复两个边界：重复 JSON 属性键的校验错误使用固定文案，不再反射任意客户端键名；已有 Play Session 仅在其 Share 仍严格未过期时有效，Share 到期、撤销或游戏进入删除状态后公开事件均被拒绝且不产生 Analytics 写入。上述行为已由核心、HTTP、Repository、真实进程与浏览器测试共同覆盖。

新增或修改事件必须同时更新本文、数据库/后端常量、OpenAPI、前端类型与测试，并重新评审身份来源和隐私字段。不得创建近义事件名或在不同端使用不同字段名。G1 独立复查已通过；后续契约变化仍须同步实现与测试，并重新完成对应阶段评审。
