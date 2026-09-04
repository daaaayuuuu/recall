import { apiRequest } from './client'

export type InvitationStatus = 'unused' | 'used' | 'revoked'

export interface AdminInvitation {
  id: string
  codeHint: string
  status: InvitationStatus
  createdByAdmin: string
  usedByCreatorId: string | null
  usedByLoginId: string | null
  usedAt: string | null
  revokedAt: string | null
  createdAt: string
}

export interface CreatedInvitation extends AdminInvitation {
  code: string
}

export function listAdminInvitations(limit = 50) {
  return apiRequest<{ items: AdminInvitation[] }>(`/admin/invitation-codes?limit=${limit}`)
}

export function createAdminInvitation(csrfToken: string) {
  return apiRequest<CreatedInvitation>('/admin/invitation-codes', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}

export function revokeAdminInvitation(invitationId: string, csrfToken: string) {
  return apiRequest<AdminInvitation>(`/admin/invitation-codes/${encodeURIComponent(invitationId)}`, {
    method: 'DELETE',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}
