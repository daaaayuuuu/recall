/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('./stages/FirstMeetingStage.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      emits: ['complete'],
      setup(_, { emit }) {
        return () => h('button', {
          'data-testid': 'first-meeting-complete',
          onClick: () => emit('complete', {
            stageId: 'first-meeting',
            completedAt: Date.now(),
            actionCount: 3,
          }),
        }, '完成初见体验')
      },
    }),
  }
})

vi.mock('./stages/PuzzleStage.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      emits: ['complete'],
      setup(_, { emit }) {
        return () => h('button', {
          'data-testid': 'puzzle-complete',
          onClick: () => emit('complete', {
            stageId: 'puzzle',
            completedAt: Date.now(),
            actionCount: 5,
          }),
        }, '完成收尾拼图')
      },
    }),
  }
})

import LoveJourneyTemplate from './LoveJourneyTemplate.vue'

describe('LoveJourneyTemplate background music', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('loops across the game, retries playback, and fades in to 35 percent', async () => {
    const play = vi.spyOn(globalThis.HTMLMediaElement.prototype, 'play')
      .mockRejectedValueOnce(new Error('autoplay blocked'))
      .mockResolvedValue(undefined)
    const pause = vi.spyOn(globalThis.HTMLMediaElement.prototype, 'pause')
      .mockImplementation(() => undefined)
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(LoveJourneyTemplate, {
      gameConfig: {
        templateId: 'love-journey',
        templateVersion: '1.1.0',
        configVersion: 3,
        config: {
          openingTitle: '爱的旅程',
          rounds: [],
          letterPassword: '0820',
        },
        assets: [],
      },
      mode: 'public',
    })
    app.mount(host)
    await nextTick()

    const music = host.querySelector<HTMLAudioElement>('audio')
    expect(music?.getAttribute('src')).toBe('/assets/love-journey/background-music.mp3')
    expect(music?.loop).toBe(true)
    expect(music?.preload).toBe('auto')
    expect(play).toHaveBeenCalledOnce()
    expect(music?.volume).toBe(0)

    await Promise.resolve()

    host.querySelector('.love-journey-template')
      ?.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    await Promise.resolve()
    await nextTick()

    expect(play).toHaveBeenCalledTimes(2)
    expect(music?.volume).toBe(0)

    vi.advanceTimersByTime(1_500)
    expect(music?.volume).toBeCloseTo(0.175)

    vi.advanceTimersByTime(1_500)
    expect(music?.volume).toBeCloseTo(0.35)

    app.unmount()
    expect(pause).toHaveBeenCalledOnce()
  })

  it('advances one scene per private-preview skip and completes from the final scene', async () => {
    vi.spyOn(globalThis.HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
    vi.spyOn(globalThis.HTMLMediaElement.prototype, 'pause').mockImplementation(() => undefined)

    const previewSkipRequest = ref(0)
    const complete = vi.fn()
    const PreviewHost = defineComponent({
      setup() {
        return () => h(LoveJourneyTemplate, {
          gameConfig: {
            templateId: 'love-journey',
            templateVersion: '1.1.0',
            configVersion: 3,
            config: {
              openingTitle: '爱的旅程',
              rounds: [],
              letterPassword: '0820',
            },
            assets: [],
          },
          mode: 'creator-preview',
          previewSkipRequest: previewSkipRequest.value,
          onComplete: complete,
        })
      },
    })
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(PreviewHost)
    app.mount(host)
    await nextTick()

    expect(host.querySelector('.journey-progress')?.textContent).toContain('1 / 5')

    for (const stageNumber of [2, 3, 4, 5]) {
      previewSkipRequest.value += 1
      await nextTick()
      expect(host.querySelector('.journey-progress')?.textContent).toContain(`${stageNumber} / 5`)
    }

    previewSkipRequest.value += 1
    await nextTick()
    expect(complete).toHaveBeenCalledOnce()

    app.unmount()
  })

  it('automatically enters the next scene after its closing puzzle without a next button', async () => {
    vi.spyOn(globalThis.HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
    vi.spyOn(globalThis.HTMLMediaElement.prototype, 'pause').mockImplementation(() => undefined)

    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(LoveJourneyTemplate, {
      gameConfig: {
        templateId: 'love-journey',
        templateVersion: '1.1.0',
        configVersion: 3,
        config: {
          openingTitle: '爱的旅程',
          rounds: [],
          letterPassword: '0820',
        },
        assets: [],
      },
      mode: 'public',
    })
    app.mount(host)
    await nextTick()

    expect(host.querySelector('.journey-progress')?.textContent).toContain('1 / 5')
    expect(host.querySelector('[data-testid="journey-next-stage"]')).toBeNull()

    host.querySelector<HTMLButtonElement>('[data-testid="first-meeting-complete"]')?.click()
    await nextTick()
    expect(host.querySelector('[data-testid="puzzle-complete"]')).not.toBeNull()

    host.querySelector<HTMLButtonElement>('[data-testid="puzzle-complete"]')?.click()
    await nextTick()
    expect(host.querySelector('.journey-progress')?.textContent).toContain('1 / 5')
    expect(host.textContent).not.toContain('进入下一幕')

    vi.advanceTimersByTime(699)
    await nextTick()
    expect(host.querySelector('.journey-progress')?.textContent).toContain('1 / 5')

    vi.advanceTimersByTime(1)
    await nextTick()
    expect(host.querySelector('.journey-progress')?.textContent).toContain('2 / 5')
    expect(host.querySelector('[data-testid="journey-next-stage"]')).toBeNull()

    app.unmount()
  })
})
