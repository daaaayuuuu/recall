import { afterEach, describe, expect, it, vi } from 'vitest'

import { avatarURL, register, uploadAvatar } from './auth'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('avatar API', () => {
  it('builds a cache-busted authenticated avatar URL', () => {
    expect(avatarURL('avatar id')).toBe('/api/v1/me/avatar?v=avatar%20id')
  })

  it('uploads the image as multipart data with CSRF protection', async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          data: {
            id: 'user-1', userId: 'creator_01', nickname: null, avatarAssetId: 'asset-1',
            createdAt: '2026-08-13T00:00:00Z', updatedAt: '2026-08-13T00:00:00Z',
          },
          requestId: 'request-1',
        }),
        { status: 201, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    vi.stubGlobal('fetch', fetcher)

    const file = new File(['avatar'], 'avatar.png', { type: 'image/png' })
    await expect(uploadAvatar(file, 'csrf-token')).resolves.toMatchObject({ avatarAssetId: 'asset-1' })

    const [path, request] = fetcher.mock.calls[0]
    expect(path).toBe('/api/v1/me/avatar')
    expect(request.method).toBe('POST')
    expect(request.credentials).toBe('include')
    expect(request.headers.get('X-CSRF-Token')).toBe('csrf-token')
    expect(request.body).toBeInstanceOf(FormData)
    expect(request.body.get('file')).toBe(file)
  })
})

describe('registration API', () => {
  it('submits the required one-time invitation code', async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({
        data: {
          user: {
            id: 'user-1', userId: 'creator_01', nickname: null, avatarAssetId: null,
            createdAt: '2026-08-19T00:00:00Z', updatedAt: '2026-08-19T00:00:00Z',
          },
          message: '注册成功',
        },
        requestId: 'request-1',
      }), { status: 201, headers: { 'Content-Type': 'application/json' } }),
    )
    vi.stubGlobal('fetch', fetcher)

    await register({
      invitationCode: '7KDM-N4PX',
      userId: 'creator_01',
      password: 'password-123',
      nickname: '',
    })

    const [path, request] = fetcher.mock.calls[0]
    expect(path).toBe('/api/v1/auth/register')
    expect(JSON.parse(String(request.body))).toEqual({
      invitationCode: '7KDM-N4PX',
      userId: 'creator_01',
      password: 'password-123',
      nickname: '',
    })
  })
})
