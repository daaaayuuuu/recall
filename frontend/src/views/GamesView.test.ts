/* eslint-disable vue/component-definition-name-casing, vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { deleteGame, listGames } from '@/api/games'

import GamesView from './GamesView.vue'

vi.mock('@/api/games', () => ({
  deleteGame: vi.fn(),
  listGames: vi.fn(),
}))

const { authMocks, messageMocks, messageBoxMocks } = vi.hoisted(() => ({
  authMocks: { ensureCSRF: vi.fn() },
  messageMocks: { error: vi.fn(), success: vi.fn() },
  messageBoxMocks: { confirm: vi.fn() },
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authMocks }))
vi.mock('element-plus', () => ({ ElMessage: messageMocks, ElMessageBox: messageBoxMocks }))

async function flushView() {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

beforeEach(() => {
  authMocks.ensureCSRF.mockResolvedValue('csrf-token')
  vi.mocked(deleteGame).mockResolvedValue({ deletionJobId: 'job-1', message: '删除中' })
  messageBoxMocks.confirm.mockResolvedValue('confirm')
})

describe('games list generation recovery', () => {
  it('renders two games as two separate cards', async () => {
    vi.mocked(listGames).mockResolvedValue({
      items: [
        {
          id: 'game-one',
          title: '第一段故事',
          description: null,
          coverAssetId: null,
          coverPreviewUrl: null,
          status: 'ready',
          currentVersionId: 'version-one',
          assetCount: 2,
          createdAt: '2026-08-23T00:00:00Z',
          updatedAt: '2026-08-23T00:01:00Z',
        },
        {
          id: 'game-two',
          title: '第二段故事',
          description: null,
          coverAssetId: null,
          coverPreviewUrl: null,
          status: 'ready',
          currentVersionId: 'version-two',
          assetCount: 3,
          createdAt: '2026-08-23T00:00:00Z',
          updatedAt: '2026-08-23T00:02:00Z',
        },
      ],
    })
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(GamesView)
    app.component('RouterLink', defineComponent({
      props: { to: { type: [String, Object], required: true } },
      setup(props, { attrs, slots }) {
        return () => h('a', { ...attrs, 'data-to': JSON.stringify(props.to) }, slots.default?.())
      },
    }))
    app.component('el-tag', defineComponent({ setup: (_props, { slots }) => () => h('span', slots.default?.()) }))
    app.component('el-empty', defineComponent({ setup: () => () => null }))
    app.mount(host)
    await flushView()

    const cards = host.querySelectorAll('.game-grid > .game-card')
    expect(cards).toHaveLength(2)
    expect(cards[0]?.textContent).toContain('第一段故事')
    expect(cards[1]?.textContent).toContain('第二段故事')

    app.unmount()
  })

  it('orders trial, edit, share, and delete actions for a completed game', async () => {
    vi.mocked(listGames).mockResolvedValue({
      items: [{
        id: 'game-ready',
        title: '我们的故事',
        description: '值得收藏的回忆',
        coverAssetId: 'cover-ready',
        coverPreviewUrl: '/cover-ready.png',
        status: 'ready',
        currentVersionId: 'version-ready',
        assetCount: 5,
        createdAt: '2026-08-23T00:00:00Z',
        updatedAt: '2026-08-23T00:01:00Z',
      }],
    })
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(GamesView)
    app.component('RouterLink', defineComponent({
      props: { to: { type: [String, Object], required: true } },
      setup(props, { attrs, slots }) {
        return () => h('a', { ...attrs, 'data-to': JSON.stringify(props.to) }, slots.default?.())
      },
    }))
    app.component('el-tag', defineComponent({ setup: (_props, { slots }) => () => h('span', slots.default?.()) }))
    app.component('el-empty', defineComponent({ setup: () => () => null }))
    app.mount(host)
    await flushView()

    const actions = host.querySelector('.game-card__actions')!
    expect([...actions.children].map((item) => item.textContent?.trim())).toEqual([
      '试玩',
      '修改',
      '',
      '',
    ])
    const targets = [...actions.querySelectorAll<HTMLAnchorElement>('a')].map((link) => link.dataset.to ?? '')
    expect(targets[0]).toContain('game-preview')
    expect(targets[1]).toContain('game-edit')
    expect(targets[2]).toContain('game-share')
    expect(host.querySelector('.game-card__cover')).toBeNull()
    expect(host.querySelector('img')).toBeNull()
    expect(host.querySelector('.game-card__content')?.tagName).toBe('DIV')
    expect(host.querySelector('.game-card > a')).toBeNull()
    expect(host.querySelector('[aria-label="分享我们的故事"]')).not.toBeNull()
    expect(host.querySelector('[aria-label="删除我们的故事"]')).not.toBeNull()

    app.unmount()
  })

  it('requires confirmation before deleting and removes the card after success', async () => {
    vi.mocked(listGames).mockResolvedValue({
      items: [{
        id: 'game-delete',
        title: '准备删除的故事',
        description: null,
        coverAssetId: null,
        coverPreviewUrl: null,
        status: 'ready',
        currentVersionId: 'version-ready',
        assetCount: 1,
        createdAt: '2026-08-23T00:00:00Z',
        updatedAt: '2026-08-23T00:01:00Z',
      }],
    })
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(GamesView)
    app.component('RouterLink', defineComponent({
      props: { to: { type: [String, Object], required: true } },
      setup(props, { attrs, slots }) {
        return () => h('a', { ...attrs, 'data-to': JSON.stringify(props.to) }, slots.default?.())
      },
    }))
    app.component('el-tag', defineComponent({ setup: (_props, { slots }) => () => h('span', slots.default?.()) }))
    app.component('el-empty', defineComponent({ setup: () => () => null }))
    app.mount(host)
    await flushView()

    host.querySelector<HTMLButtonElement>('[aria-label="删除准备删除的故事"]')?.click()
    await flushView()

    expect(messageBoxMocks.confirm).toHaveBeenCalledWith(
      expect.stringContaining('准备删除的故事'),
      '确认删除这个游戏？',
      expect.objectContaining({
        confirmButtonText: '确认删除',
        cancelButtonText: '取消',
        customClass: 'recall-message-box',
        confirmButtonClass: 'recall-message-box__danger',
        closeOnClickModal: false,
      }),
    )
    expect(deleteGame).toHaveBeenCalledWith('game-delete', 'csrf-token')
    expect(host.querySelector('.game-card')).toBeNull()
    expect(messageMocks.success).toHaveBeenCalledWith('游戏已删除')

    app.unmount()
  })

  it('does not delete when the confirmation is cancelled', async () => {
    messageBoxMocks.confirm.mockRejectedValueOnce('cancel')
    vi.mocked(listGames).mockResolvedValue({
      items: [{
        id: 'game-keep',
        title: '保留的故事',
        description: null,
        coverAssetId: null,
        coverPreviewUrl: null,
        status: 'failed',
        currentVersionId: 'version-failed',
        assetCount: 0,
        createdAt: '2026-08-23T00:00:00Z',
        updatedAt: '2026-08-23T00:01:00Z',
      }],
    })
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(GamesView)
    app.component('RouterLink', defineComponent({
      props: { to: { type: [String, Object], required: true } },
      setup(props, { attrs, slots }) {
        return () => h('a', { ...attrs, 'data-to': JSON.stringify(props.to) }, slots.default?.())
      },
    }))
    app.component('el-tag', defineComponent({ setup: (_props, { slots }) => () => h('span', slots.default?.()) }))
    app.component('el-empty', defineComponent({ setup: () => () => null }))
    app.mount(host)
    await flushView()

    host.querySelector<HTMLButtonElement>('[aria-label="删除保留的故事"]')?.click()
    await flushView()

    expect(deleteGame).not.toHaveBeenCalled()
    expect(host.querySelector('.game-card')).not.toBeNull()

    app.unmount()
  })
})
