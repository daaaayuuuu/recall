/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { getGame } from '@/api/games'
import { createShareLink } from '@/api/sharing'
import QRCode from 'qrcode'

import QuickShareView from './QuickShareView.vue'

const { ensureCSRF, message } = vi.hoisted(() => ({
  ensureCSRF: vi.fn(),
  message: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}))

vi.mock('@/api/games', () => ({ getGame: vi.fn() }))
vi.mock('@/api/sharing', () => ({ createShareLink: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ ensureCSRF }) }))
vi.mock('element-plus', () => ({ ElMessage: message }))
vi.mock('qrcode', () => ({ default: { toDataURL: vi.fn() } }))

const RouterLinkStub = defineComponent({
  props: { to: { type: [String, Object], required: true } },
  setup(props, { attrs, slots }) {
    return () => h('a', { ...attrs, 'data-to': JSON.stringify(props.to) }, slots.default?.())
  },
})

async function flushView() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

async function mountView() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(QuickShareView, { gameId: 'game-ready' })
  app.component('RouterLink', RouterLinkStub)
  app.mount(host)
  await flushView()
  return { app, host }
}

beforeEach(() => {
  vi.mocked(getGame).mockResolvedValue({
    id: 'game-ready',
    title: '我们的故事',
    description: '值得收藏的回忆',
    coverAssetId: null,
    coverPreviewUrl: null,
    status: 'ready',
    currentVersionId: 'version-ready',
    assetCount: 5,
    createdAt: '2026-08-23T00:00:00Z',
    updatedAt: '2026-08-23T00:01:00Z',
  })
  vi.mocked(createShareLink).mockResolvedValue({
    id: 'share-1',
    gameId: 'game-ready',
    gameVersionId: 'version-ready',
    publicId: 'LK0820',
    url: 'https://recall.love/g/LK0820#t=secret',
    status: 'active',
    expiresAt: '2026-08-30T00:00:00Z',
    revokedAt: null,
    createdAt: '2026-08-23T00:00:00Z',
  })
  vi.mocked(QRCode.toDataURL).mockResolvedValue('data:image/png;base64,cXJjb2Rl')
  ensureCSRF.mockResolvedValue('csrf-token')
  Object.defineProperty(globalThis.navigator, 'clipboard', {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  })
})

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('quick share page', () => {
  it('creates, copies, and renders a QR code for the same private link', async () => {
    const downloads: Array<{ href: string; filename: string }> = []
    vi.spyOn(globalThis.HTMLAnchorElement.prototype, 'click').mockImplementation(function () {
      downloads.push({ href: this.href, filename: this.download })
    })
    const { app, host } = await mountView()

    expect(host.textContent).toContain('把完成的游戏发给朋友')
    expect(host.querySelector<HTMLSelectElement>('[aria-label="分享有效期"]')?.value).toBe('7')

    host.querySelector<HTMLButtonElement>('.quick-share-create-button')?.click()
    await flushView()

    expect(ensureCSRF).toHaveBeenCalledOnce()
    expect(createShareLink).toHaveBeenCalledOnce()
    expect(createShareLink).toHaveBeenCalledWith('game-ready', expect.any(String), 'csrf-token')
    expect(QRCode.toDataURL).toHaveBeenCalledWith(
      'https://recall.love/g/LK0820#t=secret',
      expect.objectContaining({ width: 640, errorCorrectionLevel: 'M' }),
    )
    expect(globalThis.navigator.clipboard.writeText).toHaveBeenCalledWith(
      'https://recall.love/g/LK0820#t=secret',
    )
    expect(host.textContent).toContain('把故事交给 TA')
    expect(host.textContent).toContain('https://recall.love/g/LK0820#t=secret')
    expect(host.querySelector<HTMLImageElement>('[alt="我们的故事的分享二维码"]')?.src)
      .toBe('data:image/png;base64,cXJjb2Rl')

    const saveButton = [...host.querySelectorAll<HTMLButtonElement>('.quick-share-button')]
      .find((button) => button.textContent?.includes('保存二维码'))
    saveButton?.click()
    expect(downloads).toEqual([{
      href: 'data:image/png;base64,cXJjb2Rl',
      filename: '我们的故事-分享二维码.png',
    }])

    app.unmount()
  })
})
