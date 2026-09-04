import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as analyticsAPI from '@/api/analytics'
import { useAuthStore } from '@/stores/auth'

import { trackCreatorEvent, trackPlayEvent } from './tracker'

vi.mock('@/api/analytics', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/api/analytics')>()
  return {
    ...original,
    recordCreatorEvent: vi.fn(),
    recordPlayEvent: vi.fn(),
  }
})

const clientEventId = '2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2'

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(clientEventId)
})

describe('analytics tracker', () => {
  it('gets creator CSRF and sends one stable-page event without identity fields', async () => {
    const authStore = useAuthStore()
    authStore.csrfToken = 'csrf-token'

    await expect(trackCreatorEvent('games')).resolves.toBeUndefined()

    expect(analyticsAPI.recordCreatorEvent).toHaveBeenCalledTimes(1)
    const [request, csrfToken] = vi.mocked(analyticsAPI.recordCreatorEvent).mock.calls[0]
    expect(csrfToken).toBe('csrf-token')
    expect(request).toMatchObject({
      eventName: 'creator.page_viewed',
      clientEventId,
      properties: { page: 'games' },
    })
    expect(request.occurredAt).toEqual(expect.any(String))
    expect(request).not.toHaveProperty('creatorId')
    expect(request).not.toHaveProperty('loginId')
    expect(globalThis.crypto.randomUUID).toHaveBeenCalledTimes(1)
  })

  it('sends each public event once with a fresh UUID', async () => {
    vi.spyOn(globalThis.crypto, 'randomUUID')
      .mockReturnValueOnce(clientEventId)
      .mockReturnValueOnce('e41f0cc9-2261-45f0-9b82-7bf4dfa1cd72')

    await trackPlayEvent('play.completed')
    await trackPlayEvent('play.replayed')

    expect(analyticsAPI.recordPlayEvent).toHaveBeenCalledTimes(2)
    expect(analyticsAPI.recordPlayEvent).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ eventName: 'play.completed', clientEventId }),
    )
    expect(analyticsAPI.recordPlayEvent).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        eventName: 'play.replayed',
        clientEventId: 'e41f0cc9-2261-45f0-9b82-7bf4dfa1cd72',
      }),
    )
  })

  it('swallows CSRF, creator request, and public request failures without retrying', async () => {
    const authStore = useAuthStore()
    vi.spyOn(authStore, 'ensureCSRF')
      .mockRejectedValueOnce(new Error('csrf unavailable'))
      .mockResolvedValue('csrf-token')
    vi.mocked(analyticsAPI.recordCreatorEvent).mockRejectedValueOnce(new Error('network unavailable'))
    vi.mocked(analyticsAPI.recordPlayEvent).mockRejectedValueOnce(new Error('network unavailable'))

    await expect(trackCreatorEvent('create')).resolves.toBeUndefined()
    await expect(trackCreatorEvent('games')).resolves.toBeUndefined()
    await expect(trackPlayEvent('play.completed')).resolves.toBeUndefined()

    expect(analyticsAPI.recordCreatorEvent).toHaveBeenCalledTimes(1)
    expect(analyticsAPI.recordPlayEvent).toHaveBeenCalledTimes(1)
  })
})
