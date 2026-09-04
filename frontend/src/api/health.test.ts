import { describe, expect, it, vi } from 'vitest'

import { getReadiness } from './health'

describe('getReadiness', () => {
  it('returns the typed readiness response', async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ status: 'ok', service: 'api', dependencies: { mysql: 'ok' } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(getReadiness(fetcher, 'http://api.test')).resolves.toMatchObject({
      status: 'ok',
      service: 'api',
    })
    expect(fetcher).toHaveBeenCalledWith(
      'http://api.test/health/ready',
      expect.objectContaining({ credentials: 'include' }),
    )
  })
})

