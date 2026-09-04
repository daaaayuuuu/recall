import { afterEach, describe, expect, it, vi } from 'vitest'

import { listAdminBehaviorEvents, recordCreatorEvent, recordPlayEvent } from './analytics'

afterEach(() => {
  vi.unstubAllGlobals()
})

function successfulWriteResponse() {
  return new Response(
    JSON.stringify({ data: { eventId: '01K00000000000000000000000', duplicate: false }, requestId: 'request-1' }),
    { status: 201, headers: { 'Content-Type': 'application/json' } },
  )
}

describe('analytics API', () => {
  it('posts a creator event with browser credentials and CSRF to the private endpoint', async () => {
    const fetcher = vi.fn(async () => successfulWriteResponse())
    vi.stubGlobal('fetch', fetcher)

    await recordCreatorEvent(
      {
        eventName: 'creator.page_viewed',
        clientEventId: '2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2',
        occurredAt: '2026-08-16T02:35:01.123Z',
        properties: { page: 'game-edit' },
      },
      'csrf-token',
    )

    const [path, request] = fetcher.mock.calls[0]
    expect(path).toBe('/api/v1/analytics/events')
    expect(request).toMatchObject({ method: 'POST', credentials: 'include' })
    expect(request.headers.get('X-CSRF-Token')).toBe('csrf-token')
    expect(JSON.parse(String(request.body))).toEqual({
      eventName: 'creator.page_viewed',
      clientEventId: '2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2',
      occurredAt: '2026-08-16T02:35:01.123Z',
      properties: { page: 'game-edit' },
    })
  })

  it('posts a public event with browser credentials and no client-declared identity', async () => {
    const fetcher = vi.fn(async () => successfulWriteResponse())
    vi.stubGlobal('fetch', fetcher)

    await recordPlayEvent({
      eventName: 'play.completed',
      clientEventId: '2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2',
      properties: { mode: 'public' },
    })

    const [path, request] = fetcher.mock.calls[0]
    expect(path).toBe('/api/v1/public/play-sessions/current/events')
    expect(request).toMatchObject({ method: 'POST', credentials: 'include' })
    const body = JSON.parse(String(request.body)) as Record<string, unknown>
    expect(body).toEqual({
      eventName: 'play.completed',
      clientEventId: '2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2',
      properties: { mode: 'public' },
    })
    for (const field of ['creatorId', 'loginId', 'gameId', 'shareId', 'playSessionId']) {
      expect(body).not.toHaveProperty(field)
    }
  })

  it('requests the default admin page with a limit of 50', async () => {
    const fetcher = vi.fn(async () => new Response(
      JSON.stringify({ data: { items: [], nextCursor: null }, requestId: 'request-1' }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    ))
    vi.stubGlobal('fetch', fetcher)

    await listAdminBehaviorEvents()

    const [path, request] = fetcher.mock.calls[0]
    expect(path).toBe('/api/v1/admin/behavior-events?limit=50')
    expect(request).toMatchObject({ credentials: 'include' })
  })

  it('encodes every supported admin filter and the opaque cursor', async () => {
    const fetcher = vi.fn(async () => new Response(
      JSON.stringify({ data: { items: [], nextCursor: null }, requestId: 'request-1' }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    ))
    vi.stubGlobal('fetch', fetcher)

    await listAdminBehaviorEvents({
      eventName: 'generation.failed',
      creatorId: '01K00000000000000000000001',
      loginId: 'creator_01',
      gameId: '01K00000000000000000000002',
      source: 'worker',
      from: '2026-08-15T00:00:00.000Z',
      to: '2026-08-17T00:00:00.000Z',
      cursor: 'eyJ2IjoxfQ',
      limit: 25,
    })

    const [path] = fetcher.mock.calls[0]
    const url = new URL(String(path), 'https://example.test')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      eventName: 'generation.failed',
      creatorId: '01K00000000000000000000001',
      loginId: 'creator_01',
      gameId: '01K00000000000000000000002',
      source: 'worker',
      from: '2026-08-15T00:00:00.000Z',
      to: '2026-08-17T00:00:00.000Z',
      cursor: 'eyJ2IjoxfQ',
      limit: '25',
    })
  })
})
