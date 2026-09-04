import { afterEach, describe, expect, it, vi } from 'vitest'

import { APIError, apiRequest } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('apiRequest', () => {
  it('uses the versioned API base and includes browser credentials', async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ data: { ready: true }, requestId: 'request-1' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetcher)

    await expect(apiRequest<{ ready: boolean }>('/test')).resolves.toEqual({ ready: true })
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/test',
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('turns API error envelopes into APIError values', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            error: { code: 'VALIDATION_ERROR', message: '请检查输入内容', fields: { userId: '无效' } },
            requestId: 'request-2',
          }),
          { status: 422, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )

    const error = await apiRequest('/test').catch((reason: unknown) => reason)
    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({ status: 422, code: 'VALIDATION_ERROR', fields: { userId: '无效' } })
  })
})
