/* global HTMLCanvasElement, HTMLElement */
import html2canvas from 'html2canvas'

const INVALID_FILENAME_CHARACTERS = /[<>:"/\\|?*]/g

export function createGameScreenshotFilename(title: string, capturedAt = new Date()) {
  const safeTitle = title
    .trim()
    .split('')
    .filter((character) => character.charCodeAt(0) >= 32)
    .join('')
    .replace(INVALID_FILENAME_CHARACTERS, '-')
    .replace(/\s+/g, ' ')
    .slice(0, 60) || '游戏截图'
  const timestamp = [
    capturedAt.getFullYear(),
    String(capturedAt.getMonth() + 1).padStart(2, '0'),
    String(capturedAt.getDate()).padStart(2, '0'),
    '-',
    String(capturedAt.getHours()).padStart(2, '0'),
    String(capturedAt.getMinutes()).padStart(2, '0'),
    String(capturedAt.getSeconds()).padStart(2, '0'),
  ].join('')

  return `${safeTitle}-${timestamp}.png`
}

function canvasHasVisibleContent(canvas: HTMLCanvasElement) {
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context || canvas.width === 0 || canvas.height === 0) return false

  const { data } = context.getImageData(0, 0, canvas.width, canvas.height)
  for (let index = 0; index < data.length; index += 4) {
    const alpha = data[index + 3] ?? 0
    const red = data[index] ?? 255
    const green = data[index + 1] ?? 255
    const blue = data[index + 2] ?? 255
    if (alpha > 0 && (red < 245 || green < 245 || blue < 245)) return true
  }

  return false
}

function canvasToPngBlob(canvas: HTMLCanvasElement) {
  return new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, 'image/png')
  })
}

export async function downloadGameScreenshot(
  gameElement: HTMLElement,
  filename: string,
) {
  const bounds = gameElement.getBoundingClientRect()
  if (bounds.width === 0 || bounds.height === 0) {
    throw new Error('Game screenshot target has no visible size')
  }

  const pixelRatio = Math.min(2, Math.max(1, globalThis.window.devicePixelRatio || 1))
  const canvas = await html2canvas(gameElement, {
    backgroundColor: '#fff',
    width: Math.ceil(bounds.width),
    height: Math.ceil(bounds.height),
    scale: pixelRatio,
    logging: false,
    removeContainer: true,
    useCORS: true,
    ignoreElements: (element) => (
      element instanceof HTMLElement && element.dataset.screenshotExclude === 'true'
    ),
  })

  if (!canvasHasVisibleContent(canvas)) {
    throw new Error('Game screenshot generation returned a blank image')
  }

  const blob = await canvasToPngBlob(canvas)
  if (!blob) throw new Error('Game screenshot generation returned no image')

  const objectUrl = globalThis.URL.createObjectURL(blob)
  const downloadLink = globalThis.document.createElement('a')
  downloadLink.href = objectUrl
  downloadLink.download = filename.endsWith('.png') ? filename : `${filename}.png`
  downloadLink.hidden = true
  globalThis.document.body.append(downloadLink)
  downloadLink.click()
  downloadLink.remove()
  globalThis.window.setTimeout(() => globalThis.URL.revokeObjectURL(objectUrl), 1000)
}
