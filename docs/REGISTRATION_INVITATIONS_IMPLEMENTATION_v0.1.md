# 一次性注册邀请码实施记录

## 文档信息

- 版本：v0.1
- 状态：已实现，等待完整发布验收
- 更新日期：2026-08-19
- 邀请码格式：`XXXX-XXXX`，例如 `7KDM-N4PX`

## 1. 能力边界

- 制作方注册必须提交管理员生成的邀请码。
- 每个邀请码只能成功注册一个账号。
- 邀请码没有自动过期时间；在使用或管理员撤销前保持有效。
- 已使用的邀请码不能再次使用或撤销。
- 完整邀请码只在管理员创建成功响应和当前页面中显示一次。

## 2. 安全设计

邀请码包含 8 个随机字符，中间使用连字符分组。字符集为 `23456789ABCDEFGHJKLMNPQRSTUVWXYZ`，排除 `0`、`1`、`I` 和 `O`。随机值由 `crypto/rand` 生成。

数据库只保存：

- SHA-256 邀请码哈希；
- 末四位脱敏标识；
- 创建管理员、创建时间、使用者、使用时间和撤销时间。

完整邀请码不写入数据库、应用日志、Analytics、管理员列表或审计日志。管理员列表显示 `••••-N4PX` 形式的标识。

## 3. 原子核销

注册事务执行：

1. 根据邀请码哈希查询未使用且未撤销的记录，并使用 `FOR UPDATE` 加行锁。
2. 创建用户。
3. 写入邀请码的 `used_by_user_id` 和 `used_at`。
4. 提交事务。

因此，同一个邀请码的并发注册最多成功一个。用户名冲突、数据库错误或提交失败会回滚整个事务，不会消耗邀请码。

## 4. 管理接口

```text
POST   /api/v1/admin/invitation-codes
GET    /api/v1/admin/invitation-codes?limit=50
DELETE /api/v1/admin/invitation-codes/{invitationId}
```

三个接口均要求管理员会话；创建和撤销还要求可信 Origin 与有效管理员 CSRF Token。创建和撤销写入 `admin_audit_logs`，审计元数据只包含邀请码末四位。

## 5. 注册接口

```json
{
  "invitationCode": "7KDM-N4PX",
  "userId": "creator_01",
  "password": "password-123",
  "nickname": "Creator"
}
```

无效格式、不存在、已使用或已撤销统一返回：

```text
INVITATION_CODE_INVALID_OR_USED
```

## 6. 关键文件

- `backend/db/migrations/000004_registration_invites.*.sql`
- `backend/internal/invitations/`
- `backend/internal/auth/repository.go`
- `backend/internal/auth/handler.go`
- `frontend/src/views/RegisterView.vue`
- `frontend/src/views/AdminInvitationsView.vue`
- `frontend/src/api/invitations.ts`
- `contracts/openapi/openapi.yaml`
