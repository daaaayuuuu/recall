/* eslint-disable vue/component-definition-name-casing, vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { recordPlayEvent } from '@/api/analytics'
import { getGameConfig } from '@/api/sharing'

import PlayView from './PlayView.vue'

vi.mock('@/api/analytics', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/api/analytics')>()
  return { ...original, recordPlayEvent: vi.fn() }
})
vi.mock('@/api/sharing', () => ({
  createPlaySession: vi.fn(),
  getGameConfig: vi.fn(),
  resolvePublicShare: vi.fn(),
}))

const ButtonStub = defineComponent({
  emits: ['click'],
  setup(_props, { attrs, emit, slots }) {
    return () => h('button', { ...attrs, onClick: () => emit('click') }, slots.default?.())
  },
})

async function flushView() {
  await Promise.resolve()
  await nextTick()
}

async function mountPlayView() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(PlayView, { publicId: 'public-id' })
  app.component('el-button', ButtonStub)
  app.component('el-alert', defineComponent({ setup: () => () => null }))
  app.mount(host)
  await flushView()
  return { app, host }
}

beforeEach(() => {
  globalThis.history.replaceState(null, '', '/play/public-id')
  vi.mocked(getGameConfig).mockResolvedValue({
    templateId: 'memory-game',
    templateVersion: '1.0.0',
    configVersion: 1,
    config: { openingTitle: '测试回忆', rounds: [] },
    assets: [],
    playSessionExpiresAt: '2026-08-16T01:00:00Z',
  })
  vi.mocked(recordPlayEvent).mockResolvedValue({
    eventId: '01K00000000000000000000000',
    duplicate: false,
  })
})

afterEach(() => {
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

describe('public play tracking', () => {
  it('tracks completion and replay once each while preserving the phase transitions', async () => {
    const { app, host } = await mountPlayView()

    expect(host.textContent).toContain('完成这一局')
    expect(host.querySelector('.play-card--game')).not.toBeNull()
    const completeButton = host.querySelector('button')
    expect(completeButton).not.toBeNull()
    completeButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    completeButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect(host.textContent).toContain('游戏完成')
    expect(host.querySelector('.play-card--game')).toBeNull()
    expect(recordPlayEvent).toHaveBeenCalledTimes(1)
    expect(recordPlayEvent).toHaveBeenLastCalledWith(
      expect.objectContaining({ eventName: 'play.completed', properties: { mode: 'public' } }),
    )

    const replayButton = host.querySelector('button')
    replayButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    replayButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect(host.textContent).toContain('完成这一局')
    expect(host.querySelector('.play-card--game')).not.toBeNull()
    expect(recordPlayEvent).toHaveBeenCalledTimes(2)
    expect(recordPlayEvent).toHaveBeenLastCalledWith(
      expect.objectContaining({ eventName: 'play.replayed', properties: { mode: 'public' } }),
    )

    app.unmount()
  })

  it('keeps completion and replay transitions silent when event writes fail', async () => {
    vi.mocked(recordPlayEvent).mockRejectedValue(new Error('analytics unavailable'))
    const messageSpy = vi.spyOn(ElMessage, 'error')
    const { app, host } = await mountPlayView()

    const completeButton = host.querySelector('button')
    completeButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    completeButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushView()

    expect(host.textContent).toContain('游戏完成')
    expect(recordPlayEvent).toHaveBeenCalledTimes(1)
    expect(recordPlayEvent).toHaveBeenLastCalledWith(
      expect.objectContaining({ eventName: 'play.completed', properties: { mode: 'public' } }),
    )
    expect(messageSpy).not.toHaveBeenCalled()

    const replayButton = host.querySelector('button')
    replayButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    replayButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushView()

    expect(host.textContent).toContain('完成这一局')
    expect(recordPlayEvent).toHaveBeenCalledTimes(2)
    expect(recordPlayEvent).toHaveBeenLastCalledWith(
      expect.objectContaining({ eventName: 'play.replayed', properties: { mode: 'public' } }),
    )
    expect(messageSpy).not.toHaveBeenCalled()

    app.unmount()
  })
})
