export const USER_ID_HELP_TEXT = '3–32 位，以小写字母开头，只能包含小写字母、数字、下划线（_）或连字符（-）'

const reservedUserIds = new Set(['admin', 'administrator', 'root', 'support', 'system'])

export function validateUserId(value: string): string {
  const userId = value.trim()
  if (!userId) return '请输入用户ID'
  if (userId.length < 3 || userId.length > 32) return '用户ID长度应为 3–32 位'
  if (!/^[a-z]/.test(userId)) return '用户ID必须以小写字母开头'
  if (!/^[a-z0-9_-]+$/.test(userId)) {
    return '用户ID只能包含小写字母、数字、下划线（_）或连字符（-）'
  }
  if (reservedUserIds.has(userId)) return '该用户ID不可用，请更换一个'
  return ''
}
