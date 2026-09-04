import { createApp, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import MovieStage from './MovieStage.vue'

function dispatchPointer(
  target: Element,
  type: string,
  { clientX, pointerId = 1 }: { clientX: number, pointerId?: number },
) {
  const event = new MouseEvent(type, { bubbles: true, button: 0, clientX })
  Object.defineProperties(event, {
    pointerId: { value: pointerId },
    pointerType: { value: 'mouse' },
  })
  target.dispatchEvent(event)
}

describe('MovieStage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('uses the movie artwork and three bounded hand drags to complete the stage', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(MovieStage, { active: true, onComplete })
    app.mount(host)

    const track = host.querySelector<HTMLElement>('.movie-hands-stage')
    const handle = host.querySelector<HTMLElement>('.movie-hands__drag-target')
    expect(track).not.toBeNull()
    expect(handle).not.toBeNull()
    expect(host.querySelector('button')).toBeNull()
    expect(host.querySelector<HTMLImageElement>('.movie-cinema__seats')?.getAttribute('src'))
      .toBe('/assets/love-journey/movie/seats.png')
    expect(host.querySelector<HTMLImageElement>('.movie-hands__background')?.getAttribute('src'))
      .toBe('/assets/love-journey/movie/hands-bg.jpg')
    expect(host.querySelectorAll<HTMLImageElement>('img[src="/assets/love-journey/movie/hands-open.png"]'))
      .toHaveLength(2)
    expect(track?.dataset.handState).toBe('open')

    vi.spyOn(track!, 'getBoundingClientRect').mockReturnValue({
      width: 300,
      height: 24,
      top: 0,
      right: 300,
      bottom: 24,
      left: 0,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    handle!.setPointerCapture = vi.fn()
    handle!.hasPointerCapture = vi.fn(() => true)
    handle!.releasePointerCapture = vi.fn()

    dispatchPointer(handle!, 'pointerdown', { clientX: 0 })
    dispatchPointer(handle!, 'pointermove', { clientX: 20 })
    dispatchPointer(handle!, 'pointerup', { clientX: 20 })
    await nextTick()

    expect(handle?.getAttribute('aria-valuenow')).toBe('0')
    expect(track?.dataset.handState).toBe('open')
    expect(host.textContent).toContain('还差一点，再向她靠近一些。')
    expect(onComplete).not.toHaveBeenCalled()

    for (let round = 1; round <= 3; round += 1) {
      dispatchPointer(handle!, 'pointerdown', { clientX: 0, pointerId: round + 1 })
      dispatchPointer(handle!, 'pointermove', { clientX: 60, pointerId: round + 1 })
      dispatchPointer(handle!, 'pointerup', { clientX: 60, pointerId: round + 1 })
      vi.advanceTimersByTime(460)
      await nextTick()

      expect(host.querySelectorAll('.movie-distance__rounds .filled')).toHaveLength(round)
      expect(track?.dataset.handState).toBe(
        round === 1 ? 'open' : round === 2 ? 'touching' : 'touched',
      )
    }

    expect(handle?.getAttribute('aria-valuenow')).toBe('3')
    expect(host.querySelector<HTMLImageElement>('img.movie-hands__sprite--active')?.getAttribute('src'))
      .toBe('/assets/love-journey/movie/hands-touched.png')
    expect(host.textContent).toContain('这一次，你们牵住了彼此。')
    expect(onComplete).not.toHaveBeenCalled()

    vi.advanceTimersByTime(900)
    await nextTick()

    expect(onComplete).toHaveBeenCalledOnce()
    expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({
      stageId: 'movie',
      actionCount: 3,
      metadata: { approachRatio: '70:30', rounds: 3 },
    }))

    app.unmount()
  })
})
