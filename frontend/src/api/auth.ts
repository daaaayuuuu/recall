import { apiRequest, apiResourceURL } from './client'

export interface CreatorUser {
  id: string
  userId: string
  nickname: string | null
  avatarAssetId: string | null
  createdAt: string
  updatedAt: string
}

export interface AdminIdentity {
  username: string
}

export function register(input: { invitationCode: string; userId: string; password: string; nickname: string }) {
  return apiRequest<{ user: CreatorUser; message: string }>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function login(input: { userId: string; password: string }) {
  return apiRequest<{ user: CreatorUser }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function getSession() {
  return apiRequest<{ user: CreatorUser }>('/auth/session')
}

export function getCSRFToken() {
  return apiRequest<{ csrfToken: string }>('/auth/csrf', { method: 'POST' })
}

export function logout(csrfToken: string) {
  return apiRequest<{ message: string }>('/auth/logout', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}

export function getMe() {
  return apiRequest<CreatorUser>('/me')
}

export function updateMe(nickname: string, csrfToken: string) {
  return apiRequest<CreatorUser>('/me', {
    method: 'PATCH',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ nickname }),
  })
}

export function avatarURL(avatarAssetId: string) {
  return apiResourceURL(`/me/avatar?v=${encodeURIComponent(avatarAssetId)}`)
}

export function uploadAvatar(file: File, csrfToken: string) {
  const body = new FormData()
  body.append('file', file)
  return apiRequest<CreatorUser>('/me/avatar', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body,
  })
}

export function deleteAvatar(csrfToken: string) {
  return apiRequest<CreatorUser>('/me/avatar', {
    method: 'DELETE',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}

export function changePassword(currentPassword: string, newPassword: string, csrfToken: string) {
  return apiRequest<{ message: string }>('/me/password', {
    method: 'PUT',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}

export function adminLogin(input: { username: string; password: string }) {
  return apiRequest<{ admin: AdminIdentity }>('/admin/auth/login', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function getAdminSession() {
  return apiRequest<{ admin: AdminIdentity }>('/admin/auth/session')
}

export function getAdminCSRFToken() {
  return apiRequest<{ csrfToken: string }>('/admin/auth/csrf', { method: 'POST' })
}

export function adminLogout(csrfToken: string) {
  return apiRequest<{ message: string }>('/admin/auth/logout', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}
