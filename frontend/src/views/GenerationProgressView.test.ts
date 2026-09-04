/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { getGenerationRun, type GenerationRun } from '@/api/generation'
import { getGamePreview, listAssets } from '@/api/games'

import GenerationProgressView from './GenerationProgressView.vue'

vi.mock('@/api/generation', () => ({
  getGenerationRun: vi.fn(),
}))

vi.mock('@/api/games', () => ({
  getGamePreview: vi.fn(),
  listAssets: vi.fn(),
}))

const queuedRun: GenerationRun = {
  id: 'run-1', gameId: 'game-1', gameVersionId: 'version-1', attemptNumber: 1,
  triggerType: 'initial', status: 'queued', stage: 'queued', progress: 0,
  errorCode: null, errorMessage: null, retryable: false, cancelRequested: false,
  createdAt: '2026-08-23T00:00:00Z', updatedAt: '2026-08-23T00:00:00Z',
  startedAt: null, completedAt: null,
}

async function flushView() {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.mocked(getGamePreview).mockResolvedValue({
    game: { id: 'game-1', title: '我们的故事' },
    version: { id: 'version-1', versionNumber: 1 },
    templateId: 'love-journey', templateVersion: '1.1.0', configVersion: 1,
    config: { openingTitle: '我们的故事', rounds: [], loveLetter: '情书' },
    assets: [],
  })
  vi.mocked(listAssets).mockResolvedValue({ items: [] })
})

afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('generation progress polling', () => {
  it('polls the submitted run route through completion', async () => {
    vi.mocked(getGenerationRun)
      .mockResolvedValueOnce(queuedRun)
      .mockResolvedValueOnce({
        ...queuedRun,
        status: 'succeeded',
        stage: 'completed',
        progress: 100,
        completedAt: '2026-08-23T00:01:00Z',
      })
    const host = document.createElement('div')
    document.body.append(host)
    const app = createApp(GenerationProgressView, { gameId: 'game-1', runId: 'run-1' })
    app.component('RouterLink', defineComponent({
      setup(_props, { slots }) {
        return () => h('a', slots.default?.())
      },
    }))
    app.mount(host)
    await flushView()

    expect(getGenerationRun).toHaveBeenCalledWith('game-1', 'run-1')
    expect(host.textContent).toContain('8%')

    await vi.advanceTimersByTimeAsync(1200)
    await flushView()

    expect(getGenerationRun).toHaveBeenCalledTimes(2)
    expect(host.textContent).toContain('你们的故事，做好了')
    expect(getGamePreview).toHaveBeenCalledWith('game-1', 'version-1')
    expect(listAssets).toHaveBeenCalledWith('game-1', 'version-1')
    app.unmount()
  })
})
