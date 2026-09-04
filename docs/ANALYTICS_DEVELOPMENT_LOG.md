# Analytics 开发日志

> 当前基线更新（2026-08-23）：游戏详情页与 `game-detail` 页面名已经移除；制作方页面白名单以 `create`、`games`、`game-edit`、`game-preview`、`game-share`、`generation-progress`、`settings` 为准。下文早期验收记录中的 `game-detail` 只描述当时发生过的测试，不再代表现行路由或契约。

2026-08-23 重新验收：隔离真实 E2E 已按当前 `love-journey@1.1.0` 流程通过，覆盖制作方页面、成功与最终失败生成、分享打开、固定五段游玩、4 位密码、照片/情书揭晓、重玩、管理员邀请码和 14 类事件；脚本不再依赖已删除的游戏详情页或 `ANALYTICS_E2E_GAME_ID`，电影与旅行步骤使用当前实际交互。

## 文档信息

- 状态：持续追加
- 创建日期：2026-08-16
- 契约基线：[ANALYTICS_EVENT_CONTRACT_v0.1.md](./ANALYTICS_EVENT_CONTRACT_v0.1.md)

本文件只记录已经发生的阶段工作和测试事实。尚未实现的能力不得写成已完成；失败阶段必须保留失败证据和阻塞原因。

## 记录模板

```text
阶段：
负责人/Agent：
提交 SHA：
改动摘要：
任务级测试：PASS/FAIL + 命令与结果
阶段回归：PASS/FAIL + 命令与结果
文档同步：
遗留问题：
```

## 基线 G0

- 阶段：T0 / G0
- 负责人/Agent：Codex Baseline Agent
- 基线 SHA：`9dddee7a6214d0854e4eebc4a80ef7fb302fff50`
- 结果：PASS
- 摘要：从空 development 数据卷验证当前 `000001` 可建立 `login_id` 用户结构；`make lint`、`make test`、`make build`、`make compose-check` 和 Git 状态检查通过。
- 遗留问题：无

## T1：事件与 API 契约评审

- 阶段：T1 / G1
- 当前状态：PASS
- 负责人/Agent：Codex Contract Agent
- 提交 SHA：`0d0a8ad5c7481d6a1002e91c5d66f30001ccea2d`
- 改动摘要：创建事件契约与开发日志；同步 PRD 的管理员内部行为记录和产品边界；同步技术设计的 Analytics 模块、数据模型、API、Surface、保留与测试候选基线。
- 文档同步：`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`、`PRD_MVP_v0.1.md`、`TECHNICAL_DESIGN_MVP_v0.1.md`
- 评审依据：当前认证、Games、Generation、Sharing Repository/Session 结构；用户确认的产品与保留规则

### 首轮内部检查

- 文档一致性和阶段回归当时通过，但 Contract Agent 在独立评审完成前错误地把整体 `G1 评审` 写成 PASS。
- 该 PASS 不代表已发生的独立评审事实，现已撤销；当前 G1 不得视为完成。

### 独立 Review Agent 结果

- 结果：BLOCKING
- 问题 1：认证 API 的 `userId` 表示登录 ID，管理员路由的 `userId` 表示内部 ULID，与契约禁止混用的要求冲突。
- 问题 2：`resolveShare` 与 `createPlaySession` 都复用 Secret 校验，原契约未禁止在共享校验函数中重复记录 `share.opened`。
- 问题 3：独立评审前已提前写入 G1 PASS，不符合开发日志只记录已发生事实的规则。

### 本轮修复

- 身份语义：产品术语统一为“登录 ID”；候选 API 规范统一使用 `loginId` 表示登录标识、`creatorId` 表示内部 ULID。当前代码/OpenAPI 的历史 `userId` 差异被明确记录为尚未实现的同步项，Analytics 不接受该别名。
- `share.opened`：只允许 `resolveShare` 成功响应边界记录；共享校验函数无 Analytics 副作用；`createPlaySession` 只记录 `play.started`。
- 评审状态：删除提前写入的 G1 PASS，契约改为修订草案，等待独立复查。
- 修复后文档检查：PASS；只修改四个获准 docs 文件，14 个候选事件与 `bleak_todolist.md` 一致且无重复，身份命名、`share.opened` 唯一触发边界、保留、隐私、Surface 和分页交叉检查通过
- `git diff --check`：PASS
- 阶段回归：PASS；`make lint`、`make test`、`make build`
- G1 评审：首轮 BLOCKING；修复后独立复查 PASS
- 遗留问题：生产保留期限、自动清理、用户数据删除工作流与备份删除传播尚未实现，属于上线前限制；身份字段的代码/OpenAPI 同步未在 T1 修改，须由后续获准范围实施并接受评审
- 下一任务是否已满足前置条件：是；允许开始 T2

## T2：数据库迁移与数据模型

- 阶段：T2 / G2
- 当前状态：PASS
- 负责人/Agent：Codex Database Agent
- 提交 SHA：`b5d44e2dfbafc10f28df7171d2adc9ab0b3abb91`
- 改动摘要：新增 `000003_behavior_events` Up/Down 迁移；实现 16 个契约字段、可空全局唯一 `client_event_id`、五组查询索引、`source`/`actor_type`/JSON 对象 CHECK，以及唯一的 `user_id -> users.id ON DELETE SET NULL` 外键。Game、Version、Run、Share、User Session 和 Play Session ID 均保存为无外键快照。合并模板素材槽位功能后，行为事件迁移由原 `000002` 调整为 `000003`，避免重复版本号。
- 首次迁移尝试：FAIL；MySQL 8.4 返回 Error 3814，CHECK 约束不允许 `JSON_COMPACT`。失败的 `CREATE TABLE` 未创建对象；按合并后的迁移序列，相当于将失败留下的 `schema_migrations` 从 `3/dirty` 恢复为 `2/clean`，移除数据库层不受支持的紧凑 JSON 字节计算，保留 JSON 对象 CHECK。精确 4096 UTF-8 字节限制仍按契约由 T3 应用校验负责。
- 审查前任务级测试：PASS；`git diff --check`、`make migrate-up`、`make migrate-status`，以及真实 MySQL 8.4 SQL 断言。该结果是已发生的测试事实，不表示最终 G2 PASS。
- 任务级覆盖：确认新版 `users.login_id` 且不存在旧 `email`；字段、索引、唯一键和外键删除规则正确；多个空 `client_event_id` 可写、重复非空值被拒绝；非法来源、主体和非对象 JSON 被拒绝；可空关联可写；删除 Game/Share/Play Session 后快照事件保留；删除 User 后 `user_id` 置空。
- 审查前阶段回归：PASS；`make dev-infra-up`、`make migrate-up`、`make migrate-status`、`make migrate-down-one`、`make migrate-up`、`make migrate-status`、`make lint`、`make test`、`make build`、`git status --short`。该结果须在修复和独立复查后按 G2 重新执行。
- 审查前迁移循环事实：迁移版本依次验证为 `2 (dirty=false)`、回滚后 `1 (dirty=false)`、重新升级后 `2 (dirty=false)`；后端全部 Go 测试通过，前端 4 个测试文件共 6 项测试通过，lint、typecheck 和前后端 build 通过。最终 G2 尚未执行。
- 独立 Review Agent 结果：BLOCKING；T2 文档在独立审查与最终门禁发生前提前宣称当前状态和 G2 为 PASS，违反“实现 → 测试 → 文档同步 → 独立审查 → 修复 → 复查 → 全量门禁”的阶段时序，也把审查前测试事实误写为最终门禁结论。
- Worker 修复：PASS；只修正两份 Analytics 文档的阶段状态和测试时序，未修改迁移 SQL。
- 独立 Review Agent 复查：PASS；先前唯一 BLOCKING 已解决，迁移 SQL 无新增正确性、安全、隐私或可逆性问题。
- 最终 G2 重跑：PASS；事前确认 `behavior_events` 为 0 行，依序执行 `make dev-infra-up`、`make migrate-up`、`make migrate-status`、`make migrate-down-one`、`make migrate-up`、`make migrate-status`、`make lint`、`make test`、`make build`、`git diff --check`、`git status --short`，最终迁移版本为 `2 (dirty=false)`。
- 最终真实 MySQL 8.4 断言：PASS；16 个字段、当前 `users.login_id` 且无旧 `email`、非空 `client_event_id` 唯一和多个 `NULL`、三项 CHECK、删除业务记录后快照保留、删除 User 后 `user_id` 置空均通过；测试事务已回滚，最终事件表仍为 0 行。
- 最终 G2 结果：PASS；Go vet、前端 ESLint/typecheck、后端全包测试、前端 4 个测试文件共 6 项测试、Go build 和 Vite build 全部通过。
- 文档同步：`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`
- 遗留问题：生产保留与删除工作流仍属于已记录的上线前限制；不阻塞 T2。
- 下一任务是否已满足前置条件：是；允许开始 T3。

## T3：Analytics 核心模块

- 阶段：T3 / G3
- 当前状态：PASS
- 负责人/Agent：Codex Backend Core Worker
- 提交 SHA：`6ea5441bfa8a98496bdc21bf4243bd713aaade63`
- 改动摘要：新增与 HTTP 无关的 Analytics 核心模块；实现 14 个冻结事件的严格校验、最小 Recorder/no-op、服务端 ULID 写入、`clientEventId` 幂等冲突语义、管理员参数化筛选、登录 ID 联查、稳定游标分页和数据库属性二次白名单校验。
- 任务级测试：PASS；`cd backend && go test ./internal/analytics -count=1`、`go test -race ./internal/analytics -count=1`、`go vet ./internal/analytics`、`git diff --check`。
- 首轮任务级覆盖：14 个合法事件；未知事件/属性、错误来源/主体/关联、重复 JSON 键、属性类型/值域/4096 字节边界、UUID v4、ULID、幂等成功与冲突、唯一键竞态、管理员筛选 SQL、登录 ID 参数化联查、默认/最大 limit、单页游标、空切片和数据库未知属性拒绝。
- 文档同步：`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`
- 独立 Test Agent：FAIL；命令均通过，但缺少真实两页同时间游标无重无漏、属性字符串长度边界、Fake Recorder 和删除用户后空关联历史行测试。
- 独立 Review Agent：BLOCKING；`sameSemantics` 错误地把不受信的 `occurredAt` 纳入冲突维度，且确认上述强制测试覆盖缺口；`requestId` 当前正确排除但也需回归测试。
- Worker 修复：PASS；幂等比较现仅使用事件名、规范化属性和七类可信关联，明确忽略 `requestId`/`occurredAt`；新增线程安全 Fake Recorder、两页同时间游标无重无漏、所有筛选参数顺序和值、字符串长度边界、删除用户后空 Creator/登录 ID 历史行，以及属性/各可信关联冲突测试。
- 修复后任务级测试：PASS；`go test ./internal/analytics -count=1`、`go test -race ./internal/analytics -count=1`、`go vet ./internal/analytics`、`git diff --check`。
- 独立 Test Agent 复测：PASS；普通/race/vet/gofmt/diff-check、20 次 shuffle 和 87.5% 覆盖率检查通过，前次覆盖缺口均已解除。
- 独立 Review Agent 复查：PASS；前次五个 BLOCKING 均已修复，未发现新的正确性、SQL 注入、游标、隐私或文档阻塞。
- 阶段回归：PASS；`make lint`、`make test`、`make build`、`git diff --check`、`git status --short`。
- G3 结果：PASS；Go 全仓 vet/测试/build、前端 ESLint/typecheck/Vitest/build 全部通过；Analytics 包普通/race 测试、20 次 shuffle 和 87.5% 覆盖率检查通过。
- 遗留问题：无。
- 下一任务是否已满足前置条件：是；允许开始 T4。

## T4：HTTP API、身份边界与 OpenAPI

- 阶段：T4 / G4
- 当前状态：PASS
- 负责人/Agent：Codex Backend API Worker
- 提交 SHA：`503033bc584d9346ee801c24d1902ecc4925ba52`
- 改动摘要：新增制作方前端事件上报、公开游玩事件上报和管理员行为查询；从已认证 Creator/Play Session 推导内部关联；实现严格 JSON、APP/PLAY Origin、CSRF、公开限流、幂等状态码、显式管理员 DTO、Surface 装配和 OpenAPI 契约。
- 任务级测试：PASS；`go test ./cmd/api ./internal/analytics ./internal/auth ./internal/sharing -count=1`、`go test -race ./cmd/api ./internal/analytics ./internal/auth ./internal/sharing -count=1`、`make lint`、OpenAPI YAML/Schema/隐私检查、`git diff --check`。
- 任务级覆盖：制作方认证/CSRF/APP Origin、公开会话/PLAY Origin/可信关联、严格请求字段、事件/属性/时间校验、201/200/409、管理员鉴权/筛选/游标、管理员 DTO 不含 `clientEventId`、`app|play|all` 路由计划及错误响应/日志脱敏。
- 文档同步：`contracts/openapi/openapi.yaml`、`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`
- 独立 Test Agent：FAIL；现有普通/race/lint/test/build、OpenAPI 解析和真实进程黑盒 Surface 矩阵均通过，但仓内缺少真实装配/Auth/CSRF/连续幂等/公开限流覆盖，且 OpenAPI 漏列实际 429。
- 独立 Review Agent：BLOCKING；确认实际实现的可信关联、Origin、公开 Session、DTO/日志脱敏和包依赖无新增缺陷，但必须补齐上述可重复自动化证据和 OpenAPI 429。
- Worker 修复：PASS；main 实际使用惰性可注入的 Surface 装配，仓内验证最终 route/method 矩阵与 `play` 不调用私有依赖；使用真实 Auth Handler/Repository 验证 Cookie/Session/CSRF/身份上下文；新增连续 HTTP 幂等单条存储、全部客户端身份伪造字段、公开 429、管理员默认/最大 limit 测试，并同步 OpenAPI 429。
- 修复后任务级测试：PASS；规定普通/race 测试、`make lint`、OpenAPI YAML/Schema/隐私检查和 `git diff --check`。
- 独立 Test Agent 复测：PASS；规定普通/race 与全仓 lint/test/build、OpenAPI 全引用/operationId/错误码/隐私检查、实际进程 `app|play|all` 黑盒矩阵均通过。
- 独立 Review Agent 复查：PASS；前次五项 BLOCKING 均已关闭，未发现新的正确性、安全、隐私、Surface、包循环、OpenAPI 或文档阻塞。
- 阶段回归：PASS；规定目标包普通/race 测试、`make lint`、`make test`、`make build`、`git diff --check`、`git status --short`。
- G4 结果：PASS；三端点认证/授权、身份防伪、幂等、限流、参数校验、显式 DTO、OpenAPI 和 `app|play|all` 实际路由矩阵均通过自动化及黑盒复核。
- 遗留问题：无。
- 下一任务是否已满足前置条件：是；允许开始 T5A/T5B。

## T5A：后端可信业务事件埋点

- 阶段：T5A / G5A
- 当前状态：PASS
- 负责人/Agent：Codex Backend Instrumentation Worker
- 提交 SHA：`068af3c0b502a4e9d75b22778cfdcfe8e9cccacf`
- 改动摘要：API 为 Auth、Games、Generation 和 Sharing 注入最小 `analytics.Recorder`；Worker 独立创建持久化 Recorder 并注入 Processor；在主业务成功边界 best-effort 记录 11 个服务端可信事件，失败 warning 只含白名单元数据。
- 初次任务级测试：PASS；规定目标包普通/race 测试及全仓 lint/test/build 均通过。
- 独立 Test Agent：FAIL；生产代码未发现明确错误，但初版测试主要直接调用记录辅助函数，未证明真实 Handler 成功/失败边界、生成幂等、Sharing 唯一触发、Surface Recorder 装配和完整 Worker 最终状态。
- 独立 Review Agent：BLOCKING；确认同一测试证据缺口，并要求补齐最终状态持久化失败、取消竞态、未耗尽租约和精确属性断言。
- Worker 修复：PASS；新增最小可测试性 seam，并直接驱动真实注册/登录、创建游戏/版本、真实 PNG 上传、同幂等键生成提交、创建/解析分享和创建 Play Session；覆盖校验、数据库、对象存储、Secret、过期、Recorder 失败，以及 Worker 成功、最终失败、租约耗尽、取消、重试和持久化失败。
- 独立 Review Agent 复查：PASS；前次五类阻塞全部关闭，seam 均绑定原生产依赖，未改变事务顺序、Surface 或失败处理。
- 独立 Test Agent 复测：PASS；规定 6 包普通测试、指定 race 测试、`make lint`、`make test`、`make build` 和 `git diff --check` 全部通过。
- G5A：PASS；11 个服务端事件成功恰一次、失败零误记、Analytics 非阻断、幂等属性、Surface 隔离、Worker 独立 Recorder 与日志脱敏均有自动化证据。
- 遗留问题：无。

## T5B：前端埋点客户端与交互接入

- 阶段：T5B / G5B
- 当前状态：PASS
- 负责人/Agent：Codex Frontend Tracking Worker
- 提交 SHA：`e695288db86a54077b5a5e776dc40610693f763f`
- 改动摘要：新增类型化 Analytics API 与 best-effort tracker；使用 `crypto.randomUUID()`、同源 Cookie 和现有 Creator CSRF；在稳定制作方路由和公开游戏完成/重玩边界记录事件，不发送可信身份关联或 URL 信息。
- 初次任务级测试：PASS；前端 lint/typecheck/test/build、根 `make test` 和 `git diff --check` 均通过。
- 独立 Review Agent：BLOCKING；实现符合契约，但缺少组件级证据证明上报失败不改变 PlayView phase 且不产生用户错误提示。
- Worker 修复：PASS；使用真实 tracker 和失败的底层 API 补充 PlayView 组件测试，验证完成/重玩 phase、快速双击各一次及 `ElMessage.error` 不调用。
- 独立 Review Agent 复查：PASS；唯一阻塞关闭，未发现新增回归。
- 独立 Test Agent 复测：PASS；`npm run lint`、`npm run typecheck`、`npm run test`（8 个文件、14 项测试）、`npm run build`、根 `make test` 和 `git diff --check` 全部通过。
- G5B：PASS；正确端点、Cookie、CSRF、UUID、无身份字段、路由排除、无 URL/Secret、失败静默、phase 与快速点击均有自动化证据。
- 遗留问题：无。

## T5 汇合

- 阶段：G5
- 当前状态：PASS
- 负责人/Agent：Codex Coordination Agent
- T5A 提交：`068af3c0b502a4e9d75b22778cfdcfe8e9cccacf`
- T5B 提交：`e695288db86a54077b5a5e776dc40610693f763f`
- 文档同步：`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`
- 独立 Review Agent：PASS；OpenAPI、后端 14 个事件、前端 3 个可上报事件、请求字段、Surface、API/Worker Recorder、隐私边界和文档 SHA 均一致，且审查前未提前宣称 G5 PASS。
- G5 全量门禁：PASS；`make lint`、`make test`、`make build`、`make compose-check`、`git diff --check` 和 Git 状态检查全部通过。
- G5 结果：PASS；T5A/T5B 独立提交保留，汇合无冲突、无测试丢失，API 与 Worker 均使用各自数据库连接持久化事件。
- 遗留问题：无。
- 下一任务是否已满足前置条件：是；允许开始 T6。

## T6：管理员行为记录页面

- 阶段：T6 / G6
- 当前状态：PASS
- 负责人/Agent：Codex Admin UI Worker
- 提交 SHA：`4dfac9930999d6b31098d354aa8f8fbfd4e52ebd`
- 改动摘要：扩展前端 Analytics 管理查询类型与客户端；新增 `/admin/behavior-events` 管理员页面、双导航、7 类筛选、默认 50 条、cursor 加载更多、事件 ID 去重、筛选代次隔离、加载/空/错误/重试，以及事件级 properties 正向白名单展示。
- 任务级测试：PASS；前端 lint/typecheck/test（9 个文件、22 项测试）/build、根 `make test` 和 `git diff --check` 全部通过。
- 任务级覆盖：未登录管理员重定向、默认参数、全部筛选、重置、加载更多去重、新筛选清 cursor、迟到响应、空/错误/重试、顶层 `loginId` 和禁止 properties 字段不渲染。
- 独立 Review Agent：PASS；路由守卫、T5B afterEach 隔离、OpenAPI 参数、cursor 状态、隐私白名单、响应式 CSS 与可访问性未发现 BLOCKING。
- 独立 Test Agent：PASS；规定前端门禁与根级回归通过，全部 G6 强制自动化项有证据。
- 视觉检查：PASS；通过独立 `ego-browser` 任务空间和浏览器内 mock Session/事件响应实测 320×800 与 1440×900。两档均满足 `documentElement.scrollWidth === clientWidth`、无越界元素；导航、筛选、事件、Creator ID、顶层登录 ID、Request ID 和白名单属性可读，320px 长 ULID 正常换行。任务空间已关闭，未读取凭据或写数据库。
- 文档同步：`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`
- 文档独立复查：PASS；实现、测试、视觉事实和隐私措辞一致，且最终门禁前未提前宣称 G6 PASS。
- 最终 G6：PASS；前端 lint/typecheck/test（9 个文件、22 项测试）/build、根 `make test`、`git diff --check` 和 Git 状态检查全部通过。
- 遗留问题：无。
- 下一任务是否已满足前置条件：是；允许开始 T7。

## T7：端到端、异常与隐私验收

- 阶段：T7 / G7
- 当前状态：PASS
- 负责人/Agent：Codex QA Worker / Coordination Agent
- 隐私修复提交：`74cda928145e64470d1330716f497f34e5a09309`
- Sharing 有效期修复提交：`ce7695c4e98881d78b2c945b4482234929a14e19`
- QA 验收提交：`170fc7bcb37084f903176396afa224577e1d9952`
- 改动摘要：重复 JSON 属性键错误改为固定文案，避免反射客户端键名；Play Session 查询同时要求 Share 严格未过期；新增默认安全跳过、显式 opt-in 的隔离 MySQL/MinIO/API/Worker/Vite/`ego-browser` 真实验收与异常清理脚本。
- 真实链路覆盖：PASS；真实制作方登录与页面导航、游戏/版本/PNG 资产/成功及最终失败生成、分享创建/打开、Play Session、完成/重玩、管理员登录与行为页面，最终 14 类事件均出现。浏览器前后数据库增量分别证明 `create`、`games`、`game-detail` 页面事件及 `play.completed`、`play.replayed` 确由 UI 产生；管理员页面只在事件卡片内核验 14 类事件。
- 边界覆盖：PASS；相同 `clientEventId` 返回 201→200 且数据库仅一行；制作方与公开端点逐字段拒绝伪造 Creator/登录/Game/Share/Play Session；已建 Session 后 Share 过期、撤销或游戏删除中均拒绝继续上报且事件数不变；未知事件、未知/超量属性、超过 1 MiB 请求和重复键均被拒绝；同时间戳 cursor 无重无漏，`from` 含等值、`to` 排除等值。
- 非阻断与删除覆盖：PASS；临时使隔离 Analytics 表不可用时主业务仍成功且只输出脱敏错误码；删除游戏后行为快照仍保留；成功、最终失败、租约耗尽和取消/重试边界由真实 Worker 与既有任务测试共同覆盖。
- 安全与隐私：PASS；数据库、管理员/公开 API、API/Worker/Vite/browser 日志和管理员 DOM 均扫描用户原文、密码及哈希、Cookie、CSRF、分享 Secret、文件名/图片内容、Bucket/object key/签名 URL 等 canary；`loginId` 只允许管理员 DTO 顶层联查，`clientEventId` 不进入管理员 API/UI。Vite、browser、API 和 Worker 子进程使用最小白名单环境。
- 进程与数据安全：PASS；随机 loopback API/Vite 端口、独立 schema、精确 QA 对象前缀与独立 browser task-space；正常、失败和活跃浏览器期间外部 TERM 均通过进程组登记、杀前完整后代快照和兜底清理。独立复测后 E2E 进程、临时目录、schema、QA 对象和 task-space 均为 0，用户既有 5173/8080 进程 PID 前后不变。
- 任务级测试：PASS；真实隔离 E2E 多轮及最终独立复测、integration 普通/race、Go vet/gofmt、Bash 语法、前端 lint/typecheck/test/build、相关后端普通/race 测试和 diff-check 全部通过。
- 独立 Review Agent：首轮 BLOCKING；发现进程异常清理、真实前端、失效 Session、隐私 canary 与逐字段防伪证据缺口。修复版复查仍 BLOCKING；发现下拉选项可造成 14 事件假阳性、缺少 UI 前后数据库增量，以及 TERM 重挂窗口。全部修复后二次复查 PASS。
- 独立 Test Agent：PASS；最终真实 E2E、UI 数据库增量、卡片级 14 事件、active-browser TERM 生命周期、环境最小化、普通/race/static 与残留审计均通过。
- 文档同步：`ANALYTICS_EVENT_CONTRACT_v0.1.md`、`ANALYTICS_DEVELOPMENT_LOG.md`
- 阶段回归：PASS；`make lint`、`make test`、`make build`、`make compose-check`、`make migrate-status`、`git diff --check` 和 Git 状态检查全部通过；迁移版本为 `2 (dirty=false)`，后端全包与前端 9 个文件 22 项测试通过。
- 最终 G7：PASS。
- 遗留问题：生产保留期限、自动清理、用户数据删除工作流与备份删除传播仍属于已记录的上线前限制；不阻塞 T7。
- 下一任务是否已满足前置条件：是；允许开始 T8。

## T8：文档同步与最终交付

- 阶段：T8 / G8
- 当前状态：PASS
- 负责人/Agent：Codex Documentation Worker / Coordination Agent
- 本阶段提交：见最终本地提交列表；当前提交不能在自身内容中自引用 SHA，不进行历史重写。
- 改动摘要：更新项目入口、开发启动与迁移识别、MVP 完成状态、Analytics 最终实现事实、专项 E2E 和 Railway Surface 验证步骤；明确用户行为记录是 best-effort 内部观察能力，不是计费、安全审计、完整运营后台或 BI 系统。
- 开发与迁移说明：记录当前 `000001` 使用 `users.login_id`、完整迁移为版本 3 clean；旧数据库不会重放已经登记的 `000001`，后续素材槽位和行为事件迁移也不会把旧 `email` 结构修成 `login_id`。需要保留数据时必须停止并设计迁移；只在明确确认开发数据可删除后执行 dry-run 和交互式 `make dev-reset`。本阶段没有执行 reset。
- 运行与验收说明：同步 `SERVICE_SURFACE=app|play|all` 和 Worker 的 Analytics 装配矩阵、管理员 `/admin/behavior-events` 本地验收、默认安全跳过与显式 opt-in E2E 命令，以及 Railway 401/404 路由断言；Railway 服务、变量和启动配置没有变化。
- 文档边界：`bleak_todolist.md` 是用户个人未跟踪文件，按本轮更高优先级安全要求始终只读，不修改、不暂存、不提交；因此 T8 不在该文件内写完成记录，阶段事实只追加到本开发日志。
- 任务级自测：PASS；六份允许文档的本地 Markdown 链接存在，所列 Make target、脚本、迁移 Up/Down、OpenAPI 路由和前端管理路由均存在；Bash 脚本语法通过；文档中的只读用户结构检查命令在当前迁移版本实际只输出 `login_id`；T8 状态防提前 PASS 检查和 `git diff --check` 通过。
- 独立 Review Agent：PASS；实现、OpenAPI、Surface、迁移、Make target、E2E 与限制表述一致；迁移诊断和 reset 安全边界明确；Railway 仅补验证指南且未夸称已经部署；无提前 G8 PASS。
- 独立 Test Agent：首轮 FAIL；MVP 状态把已经完成的第一版行为记录误写成“部分完成”，且文末阶段 6 措辞陈旧。Worker 修正为“第一版已完成、阶段 6 整体仍部分完成”后复测 PASS；链接、命令目标、脚本语法、迁移 Up/Down、OpenAPI/前端路由、限制 guard、diff-check 与 todo SHA-256 不变均通过。
- 最终 G8：PASS；`make lint`、`make test`、`make build`、`make compose-check`、`git diff --check` 和 Git 状态检查全部通过；后端全包测试、前端 9 个文件 22 项测试及前后端构建均通过。
- 人工确认：PASS；README 命令与实际 target/脚本一致；OpenAPI、事件契约、后端常量和前端类型一致；`000003_behavior_events` 同时包含 Up/Down；三个新 API 均有认证/授权测试；管理员可通过 `/admin/behavior-events` 查询且无需直接访问数据库；工作树没有新增 `.env`、测试图片或 Secret。
- 遗留限制：完整运营后台、BI/导出、访问通知、生产保留与自动清理、用户删除和备份删除传播、多副本共享限流、监控/备份恢复演练、安全扫描及全产品/生产 E2E 尚未完成。
