/* eslint-disable vue/one-component-per-file -- createApp receives one component in separate test cases. */
import { createApp, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import TravelStage from './TravelStage.vue'

describe('TravelStage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('uses the illustrated travel assets and packs all four items', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(TravelStage, { active: true })
    app.mount(host)

    expect(host.querySelector<HTMLImageElement>('.travel-artboard__background')?.getAttribute('src'))
      .toBe('/assets/love-journey/travel/background.png')
    expect(host.querySelector<HTMLImageElement>('.travel-suitcase__body')?.getAttribute('src'))
      .toBe('/assets/love-journey/travel/suitcase-body-open.png')
    expect(host.querySelectorAll<HTMLButtonElement>('[data-travel-item]')).toHaveLength(4)
    expect(host.querySelector('.travel-scene__instruction')).toBeNull()
    expect(host.querySelector('.travel-scene__status')).toBeNull()
    expect(host.textContent).not.toContain('把旅行要用的东西拖进行李箱')

    for (const item of ['camera', 'hat', 'ticket', 'charger']) {
      host.querySelector<HTMLButtonElement>(`[data-travel-item="${item}"]`)?.click()
      await nextTick()
    }

    expect(host.querySelector<HTMLElement>('.travel-scene')?.dataset.phase).toBe('ready-to-close')
    expect(host.textContent).not.toContain('4 件行李已收好，等待合上箱盖')
    expect(host.textContent).toContain('上滑合箱')
    expect(host.querySelector<HTMLButtonElement>('.travel-suitcase')?.disabled).toBe(false)

    app.unmount()
  })

  it('shows the closed suitcase before completing the stage', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(TravelStage, { active: true, onComplete })
    app.mount(host)

    for (const item of ['camera', 'hat', 'ticket', 'charger']) {
      host.querySelector<HTMLButtonElement>(`[data-travel-item="${item}"]`)?.click()
      await nextTick()
    }

    host.querySelector<HTMLButtonElement>('.travel-suitcase')?.click()
    await nextTick()

    expect(host.querySelector<HTMLElement>('.travel-scene')?.dataset.phase).toBe('closing')
    expect(host.querySelector('.travel-workspace')?.classList.contains('travel-workspace--closing')).toBe(true)
    expect(host.querySelector<HTMLImageElement>('.travel-suitcase__closed')?.getAttribute('src'))
      .toBe('/assets/love-journey/travel/suitcase-closed.png')
    expect(onComplete).not.toHaveBeenCalled()

    vi.advanceTimersByTime(719)
    await nextTick()
    expect(onComplete).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    await nextTick()

    expect(host.querySelector<HTMLElement>('.travel-scene')?.dataset.phase).toBe('completed')
    expect(host.textContent).not.toContain('行李箱已合上')
    expect(onComplete).toHaveBeenCalledOnce()
    expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({
      stageId: 'travel',
      actionCount: 5,
      metadata: { packedItems: 4, suitcaseClosed: true },
    }))

    app.unmount()
  })
})
