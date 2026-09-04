/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import type { GenerationRun } from '@/api/generation'
import type { CreatorPreview, GameAsset } from '@/api/games'

import GenerationProgressScreen from './GenerationProgressScreen.vue'

const runningRun: GenerationRun = {
  id: 'run-1',
  gameId: 'game-1',
  gameVersionId: 'version-1',
  attemptNumber: 1,
  triggerType: 'initial',
  status: 'running',
  stage: 'transforming_images',
  progress: 0,
  errorCode: null,
  errorMessage: null,
  retryable: false,
  cancelRequested: false,
  createdAt: '2026-08-23T00:00:00Z',
  updatedAt: '2026-08-23T00:00:01Z',
  startedAt: '2026-08-23T00:00:01Z',
  completedAt: null,
}

function mountScreen(run: GenerationRun, preview?: CreatorPreview, sourceAssets: GameAsset[] = []) {
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(GenerationProgressScreen, { run, gameId: 'game-1', preview, sourceAssets })
  app.component('RouterLink', defineComponent({
    setup(_props, { slots }) {
      return () => h('a', slots.default?.())
    },
  }))
  app.mount(host)
  return { app, host }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('generation progress screen', () => {
  it('renders the four-step running layout from the real run stage', () => {
    const { app, host } = mountScreen(runningRun)

    expect(host.textContent).toContain('68%')
    expect(host.textContent).toContain('让回忆慢慢成形')
    expect(host.querySelectorAll('.generation-progress-steps li')).toHaveLength(4)
    expect(host.querySelectorAll('.generation-progress-step--completed')).toHaveLength(1)
    expect(host.querySelectorAll('.generation-progress-step--active')).toHaveLength(1)
    expect(host.textContent).toContain('正在生成回忆场景')

    app.unmount()
  })

  it('shows a persistent completion state and return action', () => {
    const preview: CreatorPreview = {
      game: { id: 'game-1', title: '我们的故事' },
      version: { id: 'version-1', versionNumber: 1 },
      templateId: 'love-journey',
      templateVersion: '1.1.0',
      configVersion: 1,
      config: {
        openingTitle: '我们的故事',
        rounds: [],
        loveLetter: '写给你的信',
      },
      assets: [
        { key: 'render-1', type: 'image', url: '/render-1.png', mimeType: 'image/png', expiresAt: '2026-08-24T00:00:00Z' },
        { key: 'render-2', type: 'image', url: '/render-2.png', mimeType: 'image/png', expiresAt: '2026-08-24T00:00:00Z' },
      ],
    }
    const sourceAssets: GameAsset[] = [
      { id: 'asset-1', role: 'cover', slotKey: 'cover', mimeType: 'image/png', sizeBytes: 1, width: 10, height: 10, sortOrder: 0, previewUrl: '/cover.png', createdAt: '2026-08-23T00:00:00Z' },
      { id: 'asset-2', role: 'source', slotKey: 'travelPhotos', mimeType: 'image/png', sizeBytes: 1, width: 10, height: 10, sortOrder: 0, previewUrl: '/travel-1.png', createdAt: '2026-08-23T00:00:00Z' },
      { id: 'asset-3', role: 'source', slotKey: 'travelPhotos', mimeType: 'image/png', sizeBytes: 1, width: 10, height: 10, sortOrder: 1, previewUrl: '/travel-2.png', createdAt: '2026-08-23T00:00:00Z' },
    ]
    const { app, host } = mountScreen({
      ...runningRun,
      status: 'succeeded',
      stage: 'completed',
      progress: 100,
      completedAt: '2026-08-23T00:01:00Z',
    }, preview, sourceAssets)

    expect(host.textContent).toContain('你们的故事，做好了')
    expect(host.textContent).toContain('5段回忆')
    expect(host.textContent).toContain('2张照片')
    expect(host.textContent).toContain('1封情书')
    expect(host.querySelectorAll('.generation-complete__photo img')).toHaveLength(2)
    expect(host.textContent).toContain('预览我们的故事')
    expect(host.textContent).toContain('返回修改内容')

    app.unmount()
  })
})
