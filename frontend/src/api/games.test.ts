import { afterEach, describe, expect, it, vi } from 'vitest'

import { getGamePreview } from './games'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('getGamePreview', () => {
  it('loads the authenticated creator preview for a specific version', async () => {
    const data = {
      game: { id: 'game-1', title: '夏日回忆' },
      version: { id: 'version-2', versionNumber: 2 },
      templateId: 'memory-game',
      templateVersion: '1.0.0',
      configVersion: 1,
      config: { openingTitle: '夏日回忆', rounds: [] },
      assets: [],
    }
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ data, requestId: 'request-preview' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetcher)

    await expect(getGamePreview('game-1', 'version-2')).resolves.toEqual(data)
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/games/game-1/versions/version-2/preview',
      expect.objectContaining({ credentials: 'include' }),
    )
  })
})
