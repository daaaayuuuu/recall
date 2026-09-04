import { createApp, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DiningStage from './DiningStage.vue'

describe('DiningStage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('uses the frame-matched dining art and completes after all five dishes', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(DiningStage, { active: true, onComplete })
    app.mount(host)

    expect(host.querySelector<HTMLImageElement>('.dining-artboard__background')?.src)
      .toContain('/assets/love-journey/dining/background.png')
    expect(host.querySelector<HTMLImageElement>('.dining-person--left')?.src)
      .toContain('/assets/love-journey/dining/left-person.png')
    expect(host.querySelector<HTMLImageElement>('.dining-person--right')?.src)
      .toContain('/assets/love-journey/dining/right-person.png')
    expect(host.querySelectorAll('[data-food-image]')).toHaveLength(5)
    expect(host.querySelector('[data-food-selection]')).toBeNull()

    for (const foodId of ['meal', 'sushi', 'shrimp', 'salad', 'cake']) {
      const choice = host.querySelector<HTMLButtonElement>(`[data-food="${foodId}"]`)!
      choice.click()
      await nextTick()

      expect(choice.getAttribute('aria-pressed')).toBe('true')
      expect(host.querySelector('[data-food-selection]')?.getAttribute('data-food-selection'))
        .toBe(foodId)
      expect(host.textContent).toContain('正在品尝')
      expect(onComplete).not.toHaveBeenCalled()

      vi.advanceTimersByTime(460)
      await nextTick()

      expect(host.querySelector('[data-food-selection]')).toBeNull()
      expect(host.querySelector(`[data-food-image="${foodId}"]`)?.classList)
        .toContain('dining-food--eaten')
    }

    expect(host.querySelector<HTMLElement>('.dining-scene')?.dataset.status)
      .toBe('completed')
    expect(onComplete).toHaveBeenCalledOnce()
    expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({
      stageId: 'dining',
      actionCount: 5,
      metadata: { lastFoodId: 'cake' },
    }))

    app.unmount()
  })
})
