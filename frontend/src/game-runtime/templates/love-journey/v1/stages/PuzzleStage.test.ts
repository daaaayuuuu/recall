/* eslint-disable vue/one-component-per-file -- createApp mounts the component in separate test cases. */
import { createApp, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import PuzzleStage from './PuzzleStage.vue'

describe('PuzzleStage', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('uses the dedicated travel artwork without changing the two-piece interaction', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(PuzzleStage, {
      active: true,
      pieceCount: 2,
      experienceLabel: '场景 4',
      experienceTitle: '旅行',
      onComplete,
    })
    app.mount(host)

    expect(host.querySelector('.puzzle-scene--travel')).not.toBeNull()
    expect(host.querySelector<HTMLImageElement>('.puzzle-board__travel-art')?.src)
      .toContain('/assets/love-journey/travel-puzzle-board.png')
    expect(host.querySelectorAll('.puzzle-board__character')).toHaveLength(2)
    expect(host.querySelectorAll('.puzzle-piece--loose .puzzle-piece__travel-art')).toHaveLength(2)
    expect(host.textContent).toContain('把两块旅程拼好，打开下一封回忆')

    host.querySelector<HTMLButtonElement>('.puzzle-piece--loose')
      ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(host.textContent).toContain('1 / 2')

    host.querySelector<HTMLButtonElement>('.puzzle-piece--loose')
      ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(onComplete).toHaveBeenCalledOnce()
    expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({
      stageId: 'puzzle',
      actionCount: 2,
      metadata: { pieceCount: 2 },
    }))

    app.unmount()
  })

  it('uses the dedicated cinema artwork without changing the three-piece interaction', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(PuzzleStage, {
      active: true,
      pieceCount: 3,
      experienceLabel: '场景 3',
      experienceTitle: '看电影',
    })
    app.mount(host)

    expect(host.querySelector('.puzzle-scene--cinema')).not.toBeNull()
    expect(host.querySelector<HTMLImageElement>('.puzzle-board__cinema-art')?.src)
      .toContain('/assets/love-journey/cinema-puzzle-board.png')
    expect(host.querySelectorAll('.puzzle-piece--loose .puzzle-piece__shape--illustrated'))
      .toHaveLength(3)
    expect(host.textContent).toContain('把三块影院记忆拼好，继续下一段旅程')

    for (let index = 0; index < 3; index += 1) {
      host.querySelector<HTMLButtonElement>('.puzzle-piece--loose')
        ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
      await nextTick()
    }

    expect(host.textContent).toContain('看电影记忆已拼好')

    app.unmount()
  })

  it('uses the dedicated first-meeting artwork above the five-piece puzzle', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(PuzzleStage, {
      active: true,
      pieceCount: 5,
      experienceLabel: '场景 1',
      experienceTitle: '初见',
      onComplete,
    })
    app.mount(host)

    expect(host.querySelector('.puzzle-scene--meeting')).not.toBeNull()
    expect(host.querySelector<HTMLImageElement>('.puzzle-board__meeting-art')?.src)
      .toContain('/assets/love-journey/first-meeting-puzzle-board.png')
    expect(host.querySelectorAll('.puzzle-piece--loose .puzzle-piece__shape--illustrated'))
      .toHaveLength(5)
    const piecePaths = [...host.querySelectorAll('.puzzle-piece--loose path')]
      .map((path) => path.getAttribute('d'))
    expect(new Set(piecePaths)).toHaveLength(5)
    expect(host.textContent).toContain('把五块初见记忆拼好，打开下一段故事')

    for (let index = 0; index < 5; index += 1) {
      host.querySelector<HTMLButtonElement>('.puzzle-piece--loose')
        ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
      await nextTick()
    }

    expect(onComplete).toHaveBeenCalledOnce()
    expect(host.textContent).toContain('初见记忆已拼好')

    app.unmount()
  })

  it('uses two dining transition scenes above the four-piece puzzle', async () => {
    const host = document.createElement('div')
    document.body.append(host)
    const onComplete = vi.fn()
    const app = createApp(PuzzleStage, {
      active: true,
      pieceCount: 4,
      experienceLabel: '场景 2',
      experienceTitle: '吃饭',
      onComplete,
    })
    app.mount(host)

    expect(host.querySelector('.puzzle-scene--dining')).not.toBeNull()
    const sceneImages = [...host.querySelectorAll<HTMLImageElement>('.puzzle-board__dining-art')]
      .map((image) => image.src)
    expect(sceneImages).toEqual(expect.arrayContaining([
      expect.stringContaining('/assets/love-journey/dining-puzzle-scene-1.png'),
      expect.stringContaining('/assets/love-journey/dining-puzzle-scene-2.png'),
    ]))
    expect(host.querySelectorAll('.puzzle-piece--loose .puzzle-piece__shape--illustrated'))
      .toHaveLength(4)
    const piecePaths = [...host.querySelectorAll('.puzzle-piece--loose path')]
      .map((path) => path.getAttribute('d'))
    expect(new Set(piecePaths)).toHaveLength(4)
    expect(host.textContent).toContain('把四块饭后记忆拼好，一起走向下一场电影')

    for (let index = 0; index < 4; index += 1) {
      host.querySelector<HTMLButtonElement>('.puzzle-piece--loose')
        ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
      await nextTick()
    }

    expect(onComplete).toHaveBeenCalledOnce()
    expect(host.textContent).toContain('吃饭记忆已拼好')

    app.unmount()
  })

  it('keeps the generic artwork for other closing puzzles', () => {
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(PuzzleStage, {
      active: true,
      pieceCount: 4,
      experienceLabel: '场景 2',
      experienceTitle: '纪念日',
    })
    app.mount(host)

    expect(host.querySelector('.puzzle-scene--travel')).toBeNull()
    expect(host.querySelector('.puzzle-scene--cinema')).toBeNull()
    expect(host.querySelector('.puzzle-board__cinema-art')).toBeNull()
    expect(host.querySelector('.puzzle-scene--meeting')).toBeNull()
    expect(host.querySelector('.puzzle-scene--dining')).toBeNull()
    expect(host.querySelectorAll('.puzzle-piece--loose .puzzle-piece__shape')).toHaveLength(4)

    app.unmount()
  })
})
