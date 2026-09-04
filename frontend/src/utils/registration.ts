function characterCount(value: string) {
  return Array.from(value).length
}

export function validateInvitationCode(value: string): string {
  return value.trim() ? '' : '请输入邀请码'
}

export function validateNickname(value: string): string {
  if (characterCount(value.trim()) > 64) return '昵称不能超过 64 个字符'
  return ''
}

export function validatePassword(value: string): string {
  if (!value) return '请输入密码'
  const length = characterCount(value)
  if (length < 8 || length > 128) return '密码长度应为 8–128 个字符'
  return ''
}

export function validatePasswordConfirmation(password: string, confirmation: string): string {
  if (!confirmation) return '请再次输入密码'
  if (password !== confirmation) return '两次输入的密码不一致'
  return ''
}
