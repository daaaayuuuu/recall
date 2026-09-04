import { createApp, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  listAdminBehaviorEvents,
  type AdminBehaviorEvent,
  type AdminBehaviorEventPage,
} from '@/api/analytics'

import AdminBehaviorEventsView from './AdminBehaviorEventsView.vue'

vi.mock('@/api/analytics', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/api/analytics')>()
  return { ...original, listAdminBehaviorEvents: vi.fn() }
})

const baseEvent: AdminBehaviorEvent = {
  id: '01K00000000000000000000001',
  eventName: 'creator.page_viewed',
  source: 'frontend',
  actorType: 'creator',
  creatorId: '01K00000000000000000000002',
  loginId: 'creator_01',
  userSessionId: '01K00000000000000000000003',
  gameId: '01K00000000000000000000004',
  gameVersionId: null,
  generationRunId: null,
  shareId: null,
  playSessionId: null,
  requestId: '01K00000000000000000000005',
  properties: { page: 'games' },
  occurredAt: '2026-08-16T02:30:00Z',
  createdAt: '2026-08-16T02:35:00Z',
}

async function flushView() {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

async function mountView() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(AdminBehaviorEventsView)
  app.mount(host)
  await flushView()
  return { app, host }
}

function setInput(host: HTMLElement, testId: string, value: string) {
  const input = host.querySelector(`[data-testid="${testId}"]`) as HTMLInputElement | HTMLSelectElement
  input.value = value
  input.dispatchEvent(new Event(input instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }))
}

function click(host: HTMLElement, testId: string) {
  const button = host.querySelector(`[data-testid="${testId}"]`) as HTMLButtonElement
  button.click()
}

afterEach(() => {
  document.body.replaceChildren()
  vi.mocked(listAdminBehaviorEvents).mockReset()
  vi.clearAllMocks()
})

describe('admin behavior events view', () => {
  it('loads 50 events by default and resets every filter to that default', async () => {
    vi.mocked(listAdminBehaviorEvents).mockResolvedValue({ items: [], nextCursor: null })
    const { app, host } = await mountView()

    const eventOptions = Array.from(
      (host.querySelector('[data-testid="event-name-filter"]') as HTMLSelectElement).options,
    ).map((option) => option.value).filter(Boolean)
    expect(eventOptions).toHaveLength(14)
    expect(eventOptions).toEqual(expect.arrayContaining([
      'creator.page_viewed',
      'generation.failed',
      'share.opened',
      'play.replayed',
    ]))

    expect(listAdminBehaviorEvents).toHaveBeenNthCalledWith(1, {
      eventName: undefined,
      creatorId: undefined,
      loginId: undefined,
      gameId: undefined,
      source: undefined,
      from: undefined,
      to: undefined,
      cursor: undefined,
      limit: 50,
    })
    expect(host.textContent).toContain('当前没有符合条件的行为记录')

    setInput(host, 'event-name-filter', 'generation.failed')
    setInput(host, 'creator-id-filter', '01K00000000000000000000002')
    setInput(host, 'login-id-filter', 'creator_01')
    setInput(host, 'game-id-filter', '01K00000000000000000000004')
    setInput(host, 'source-filter', 'worker')
    setInput(host, 'from-filter', '2026-08-15T08:00')
    setInput(host, 'to-filter', '2026-08-17T08:00')
    click(host, 'apply-filters')
    await flushView()

    expect(listAdminBehaviorEvents).toHaveBeenLastCalledWith(expect.objectContaining({
      eventName: 'generation.failed',
      creatorId: '01K00000000000000000000002',
      loginId: 'creator_01',
      gameId: '01K00000000000000000000004',
      source: 'worker',
      from: expect.stringMatching(/^2026-08-15T/),
      to: expect.stringMatching(/^2026-08-17T/),
      cursor: undefined,
      limit: 50,
    }))

    click(host, 'reset-filters')
    await flushView()

    for (const id of ['event-name-filter', 'creator-id-filter', 'login-id-filter', 'game-id-filter', 'source-filter', 'from-filter', 'to-filter']) {
      expect((host.querySelector(`[data-testid="${id}"]`) as HTMLInputElement).value).toBe('')
    }
    expect(listAdminBehaviorEvents).toHaveBeenLastCalledWith(expect.objectContaining({
      eventName: undefined,
      creatorId: undefined,
      loginId: undefined,
      gameId: undefined,
      source: undefined,
      from: undefined,
      to: undefined,
      cursor: undefined,
      limit: 50,
    }))

    app.unmount()
  })

  it('appends cursor pages without duplicates and never reuses the cursor after a filter changes', async () => {
    const secondEvent = { ...baseEvent, id: '01K00000000000000000000006', eventName: 'game.created' as const, properties: { templateId: 'memory-game' } }
    vi.mocked(listAdminBehaviorEvents)
      .mockResolvedValueOnce({ items: [baseEvent], nextCursor: 'cursor-page-2' })
      .mockResolvedValueOnce({ items: [baseEvent, secondEvent], nextCursor: null })
      .mockResolvedValueOnce({ items: [], nextCursor: null })
    const { app, host } = await mountView()

    click(host, 'load-more-events')
    await flushView()

    expect(listAdminBehaviorEvents).toHaveBeenNthCalledWith(2, expect.objectContaining({ cursor: 'cursor-page-2', limit: 50 }))
    expect(host.querySelectorAll('.behavior-event-card')).toHaveLength(2)

    setInput(host, 'source-filter', 'api')
    await nextTick()
    expect(host.querySelectorAll('.behavior-event-card')).toHaveLength(0)
    expect(host.querySelector('[data-testid="load-more-events"]')).toBeNull()
    click(host, 'apply-filters')
    await flushView()

    expect(listAdminBehaviorEvents).toHaveBeenNthCalledWith(3, expect.objectContaining({ source: 'api', cursor: undefined }))
    app.unmount()
  })

  it('ignores a late response from a superseded filter request', async () => {
    let resolveFirst!: (page: AdminBehaviorEventPage) => void
    const first = new Promise<AdminBehaviorEventPage>((resolve) => { resolveFirst = resolve })
    const currentEvent = { ...baseEvent, id: '01K00000000000000000000008', eventName: 'share.opened' as const, properties: {} }
    vi.mocked(listAdminBehaviorEvents)
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ items: [currentEvent], nextCursor: null })
    const { app, host } = await mountView()

    setInput(host, 'source-filter', 'api')
    click(host, 'apply-filters')
    await flushView()
    expect(host.querySelector('.behavior-event-card')?.textContent).toContain('share.opened')

    resolveFirst({ items: [baseEvent], nextCursor: 'stale-cursor' })
    await flushView()

    expect(host.querySelector('.behavior-event-card')?.textContent).toContain('share.opened')
    expect(host.querySelector('.behavior-event-card')?.textContent).not.toContain('creator.page_viewed')
    expect(host.querySelector('[data-testid="load-more-events"]')).toBeNull()
    app.unmount()
  })

  it('shows API errors with retry and then renders an empty state', async () => {
    vi.mocked(listAdminBehaviorEvents)
      .mockRejectedValueOnce(new Error('服务暂不可用'))
      .mockResolvedValueOnce({ items: [], nextCursor: null })
    const { app, host } = await mountView()

    expect(host.querySelector('.behavior-events-page')?.getAttribute('aria-busy')).toBe('false')
    expect(host.textContent).toContain('行为记录加载失败')
    expect(host.textContent).toContain('服务暂不可用')
    click(host, 'retry-events')
    await flushView()

    expect(listAdminBehaviorEvents).toHaveBeenCalledTimes(2)
    expect(host.textContent).toContain('当前没有符合条件的行为记录')
    app.unmount()
  })

  it('renders loginId only from the top-level identity and positively whitelists event properties', async () => {
    const event: AdminBehaviorEvent = {
      ...baseEvent,
      eventName: 'generation.failed',
      source: 'worker',
      properties: {
        errorCode: 'INTERNAL_ERROR',
        retryable: false,
        executionCount: 2,
        login_id: 'forged-login',
        Token: 'private-token-value',
        Secret: 'private-secret-value',
        bucket: 'private-bucket-value',
        objectKey: 'private-object-key-value',
        unknown: 'unknown-private-value',
      },
    }
    vi.mocked(listAdminBehaviorEvents).mockResolvedValue({ items: [event], nextCursor: null })
    const { app, host } = await mountView()

    expect(host.textContent).toContain('creator_01')
    expect(host.textContent).toContain('INTERNAL_ERROR')
    expect(host.textContent).toContain('执行次数')
    for (const privateValue of [
      'forged-login',
      'private-token-value',
      'private-secret-value',
      'private-bucket-value',
      'private-object-key-value',
      'unknown-private-value',
    ]) {
      expect(host.textContent).not.toContain(privateValue)
    }
    app.unmount()
  })
})
