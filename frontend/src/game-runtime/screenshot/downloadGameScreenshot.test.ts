/* global HTMLAnchorElement */
import html2canvas from 'html2canvas'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createGameScreenshotFilename, downloadGameScreenshot } from './downloadGameScreenshot'

vi.mock('html2canvas', () => ({ default: vi.fn() }))

describe('game screenshot download', () => {
  const createObjectUrl = vi.fn(() => 'blob:game-screenshot')
  const revokeObjectUrl = vi.fn()
  let screenshotCanvas: HTMLCanvasElement

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.stubGlobal('URL', {
      ...globalThis.URL,
      createObjectURL: createObjectUrl,
      revokeObjectURL: revokeObjectUrl,
    })
    vi.spyOn(globalThis.window, 'devicePixelRatio', 'get').mockReturnValue(3)
    screenshotCanvas = document.createElement('canvas')
    screenshotCanvas.width = 780
    screenshotCanvas.height = 1688
    vi.spyOn(screenshotCanvas, 'getContext').mockReturnValue({
      getImageData: () => ({ data: new Uint8ClampedArray([0, 0, 0, 255]) }),
    } as unknown as CanvasRenderingContext2D)
    vi.spyOn(screenshotCanvas, 'toBlob').mockImplementation((callback) => {
      callback(new Blob(['png'], { type: 'image/png' }))
    })
    vi.mocked(html2canvas).mockResolvedValue(screenshotCanvas)
  })

  afterEach(() => {
    vi.runAllTimers()
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('creates a filesystem-safe timestamped png name', () => {
    expect(createGameScreenshotFilename(
      ' 我们的/爱的:旅程 ',
      new Date(2026, 7, 17, 9, 8, 7),
    )).toBe('我们的-爱的-旅程-20260817-090807.png')
  })

  it('renders the visible game at up to 2x and directly downloads the png', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const gameElement = document.createElement('div')
    vi.spyOn(gameElement, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 390,
      bottom: 844,
      width: 390,
      height: 844,
      toJSON: () => ({}),
    })

    await downloadGameScreenshot(gameElement, '爱的旅程.png')

    expect(html2canvas).toHaveBeenCalledWith(gameElement, expect.objectContaining({
      backgroundColor: '#fff',
      width: 390,
      height: 844,
      scale: 2,
      logging: false,
      removeContainer: true,
      useCORS: true,
    }))
    const options = vi.mocked(html2canvas).mock.calls[0]?.[1]
    const excluded = document.createElement('button')
    excluded.dataset.screenshotExclude = 'true'
    expect(options?.ignoreElements?.(excluded)).toBe(true)
    expect(createObjectUrl).toHaveBeenCalledOnce()
    expect(click).toHaveBeenCalledOnce()
    expect(document.querySelector('a[download]')).toBeNull()
  })

  it('rejects a blank canvas instead of downloading an empty png', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const gameElement = document.createElement('div')
    vi.spyOn(gameElement, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 390,
      bottom: 844,
      width: 390,
      height: 844,
      toJSON: () => ({}),
    })
    vi.mocked(screenshotCanvas.getContext).mockReturnValue({
      getImageData: () => ({ data: new Uint8ClampedArray([255, 255, 255, 255]) }),
    } as unknown as CanvasRenderingContext2D)

    await expect(downloadGameScreenshot(gameElement, '空白截图.png'))
      .rejects.toThrow('blank image')

    expect(createObjectUrl).not.toHaveBeenCalled()
    expect(click).not.toHaveBeenCalled()
  })
})
