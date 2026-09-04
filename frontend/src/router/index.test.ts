import { createPinia, setActivePinia } from 'pinia'
import { beforeAll, describe, expect, it, vi } from 'vitest'

import { useAdminAuthStore, useAuthStore } from '@/stores/auth'

import { trackCreatorEvent } from '@/analytics/tracker'

vi.mock('@/analytics/tracker', () => ({ trackCreatorEvent: vi.fn() }))

describe('creator page tracking', () => {
  beforeAll(() => {
    setActivePinia(createPinia())
  })

  it('tracks each successful creator route once and excludes login, admin, and public routes', async () => {
    const authStore = useAuthStore()
    authStore.initialized = true
    authStore.user = {
      id: '01K00000000000000000000000',
      userId: 'creator_01',
      nickname: null,
      avatarAssetId: null,
      createdAt: '2026-08-16T00:00:00Z',
      updatedAt: '2026-08-16T00:00:00Z',
    }
    const adminStore = useAdminAuthStore()
    adminStore.initialized = true
    adminStore.admin = { username: 'admin' }

    const { default: router } = await import('./index')

    await router.push('/app/create')
    await router.push('/app/games?sort=recent#private-fragment')
    await router.push('/app/games/game-1/generation/run-1')
    await router.push('/app/games/game-1/edit')
    await router.push('/app/games/game-1/share')
    await router.push('/play/public-id#t=share-secret')
    await router.push('/admin')
    await router.push('/admin/behavior-events')

    authStore.user = null
    await router.push('/auth/login')

    expect(trackCreatorEvent).toHaveBeenCalledTimes(5)
    expect(trackCreatorEvent).toHaveBeenNthCalledWith(1, 'create')
    expect(trackCreatorEvent).toHaveBeenNthCalledWith(2, 'games')
    expect(trackCreatorEvent).toHaveBeenNthCalledWith(3, 'generation-progress')
    expect(trackCreatorEvent).toHaveBeenNthCalledWith(4, 'game-edit')
    expect(trackCreatorEvent).toHaveBeenNthCalledWith(5, 'game-share')
    expect(JSON.stringify(vi.mocked(trackCreatorEvent).mock.calls)).not.toContain('sort=recent')
    expect(JSON.stringify(vi.mocked(trackCreatorEvent).mock.calls)).not.toContain('private-fragment')
    expect(JSON.stringify(vi.mocked(trackCreatorEvent).mock.calls)).not.toContain('share-secret')
  })

  it('redirects an unauthenticated administrator away from behavior events', async () => {
    const adminStore = useAdminAuthStore()
    adminStore.initialized = true
    adminStore.admin = null
    const { default: router } = await import('./index')

    await router.push('/admin/behavior-events')

    expect(router.currentRoute.value.name).toBe('admin-login')
    expect(router.currentRoute.value.query.redirect).toBe('/admin/behavior-events')
  })
})
