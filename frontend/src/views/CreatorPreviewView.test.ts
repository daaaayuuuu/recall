/* eslint-disable vue/component-definition-name-casing, vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { getGamePreview } from '@/api/games'

import CreatorPreviewView from './CreatorPreviewView.vue'

vi.mock('@/api/games', () => ({ getGamePreview: vi.fn() }))

const RouterLinkStub = defineComponent({
  setup(_props, { attrs, slots }) {
    return () => h('a', { ...attrs, href: '#' }, slots.default?.())
  },
})

const ButtonStub = defineComponent({
  emits: ['click'],
  setup(_props, { attrs, emit, slots }) {
    return () => h('button', { ...attrs, onClick: () => emit('click') }, slots.default?.())
  },
})

async function mountPreview() {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(CreatorPreviewView, { gameId: 'game-id', versionId: 'version-id' })
  app.component('RouterLink', RouterLinkStub)
  app.component('el-button', ButtonStub)
  app.component('el-alert', defineComponent({ setup: () => () => null }))
  app.component('el-tag', defineComponent({ setup: (_props, { slots }) => () => h('span', slots.default?.()) }))
  app.mount(host)
  await Promise.resolve()
  await nextTick()
  return { app, host }
}

beforeEach(() => {
  vi.mocked(getGamePreview).mockResolvedValue({
    templateId: 'memory-game',
    templateVersion: '1.0.0',
    configVersion: 1,
    config: { openingTitle: '试玩版本', rounds: [] },
    assets: [],
    game: { id: 'game-id', title: '测试游戏' },
    version: { id: 'version-id', versionNumber: 1 },
  })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('creator preview fullscreen mode', () => {
  it('covers the product shell while playing and restores it after completion', async () => {
    const { app, host } = await mountPreview()

    expect(host.querySelector('.preview-stage--game')).not.toBeNull()
    expect(host.querySelector('.preview-stage__controls')).not.toBeNull()
    expect(host.querySelector('[aria-label="跳过当前场景"]')).not.toBeNull()
    expect(host.querySelector('[aria-label="退出试玩"]')).not.toBeNull()

    host.querySelector<HTMLButtonElement>('[aria-label="跳过当前场景"]')?.click()
    await nextTick()

    expect(host.querySelector('.preview-stage--game')).toBeNull()
    expect(host.querySelector('[aria-label="跳过当前场景"]')).toBeNull()
    expect(host.querySelector('[aria-label="退出试玩"]')).toBeNull()
    expect(host.textContent).toContain('试玩完成')

    app.unmount()
  })
})
