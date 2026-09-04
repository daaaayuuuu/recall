import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  createAdminInvitation,
  listAdminInvitations,
  revokeAdminInvitation,
} from './invitations'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('administrator invitation API', () => {
  it('creates and revokes invitations with CSRF protection', async () => {
    const fetcher = vi.fn(async (path: string, request: RequestInit) =>
      new Response(JSON.stringify({
        data: request.method === 'POST' && path.endsWith('/admin/invitation-codes')
          ? {
              id: 'invite-1', code: '7KDM-N4PX', codeHint: '••••-N4PX', status: 'unused',
              createdByAdmin: 'admin', usedByCreatorId: null, usedByLoginId: null,
              usedAt: null, revokedAt: null, createdAt: '2026-08-19T00:00:00Z',
            }
          : {
              id: 'invite-1', codeHint: '••••-N4PX', status: 'revoked',
              createdByAdmin: 'admin', usedByCreatorId: null, usedByLoginId: null,
              usedAt: null, revokedAt: '2026-08-19T01:00:00Z', createdAt: '2026-08-19T00:00:00Z',
            },
        requestId: 'request-1',
      }), { status: request.method === 'POST' ? 201 : 200 }),
    )
    vi.stubGlobal('fetch', fetcher)

    await expect(createAdminInvitation('admin-csrf')).resolves.toMatchObject({ code: '7KDM-N4PX' })
    await expect(revokeAdminInvitation('invite-1', 'admin-csrf')).resolves.toMatchObject({ status: 'revoked' })

    for (const [, request] of fetcher.mock.calls) {
      expect(request.headers.get('X-CSRF-Token')).toBe('admin-csrf')
      expect(request.credentials).toBe('include')
    }
    expect(fetcher.mock.calls[0][1].method).toBe('POST')
    expect(fetcher.mock.calls[1][1].method).toBe('DELETE')
  })

  it('loads only masked invitation records', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({
      data: { items: [{ id: 'invite-1', codeHint: '••••-N4PX', status: 'unused' }] },
      requestId: 'request-1',
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetcher)

    const result = await listAdminInvitations()
    expect(result.items[0]).not.toHaveProperty('code')
    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/admin/invitation-codes?limit=50')
  })
})
