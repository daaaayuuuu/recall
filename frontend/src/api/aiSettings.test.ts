import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  getAdminAISettings,
  testAdminAIConnection,
  updateAdminAISettings,
  type AISettingsSnapshot,
  type APIKeyMutations,
} from './aiSettings'

const settings: AISettingsSnapshot = {
  text: { enabled: false, provider: 'deepseek', baseUrl: 'https://api.deepseek.com', model: 'deepseek-v4-flash', timeout: '30s', maxOutputTokens: 2000 },
  imageModeration: { enabled: false, provider: '', baseUrl: '', model: '', timeout: '20s', maxOutputTokens: 300 },
  imageToImage: { enabled: false, provider: '', baseUrl: '', model: '', quality: 'medium', timeout: '3m', maxInputBytes: 26214400, maxOutputBytes: 26214400 },
}
const apiKeys: APIKeyMutations = {
  text: { value: '', clear: false },
  imageModeration: { value: '', clear: false },
  imageToImage: { value: '', clear: false },
}

afterEach(() => vi.unstubAllGlobals())

describe('administrator AI settings API', () => {
  it('loads, updates, and tests settings with CSRF protection', async () => {
    const fetcher = vi.fn(async (path: string, request: RequestInit = {}) => {
      void request
      return new Response(JSON.stringify({
        data: path.endsWith('/test')
          ? { capability: 'text', latencyMs: 12 }
          : { dynamicEnabled: true, version: 1, source: 'admin', settings, apiKeys: {} },
        requestId: 'request-1',
      }), { status: 200 })
    })
    vi.stubGlobal('fetch', fetcher)

    await getAdminAISettings()
    await updateAdminAISettings(0, settings, apiKeys, 'csrf-admin')
    await testAdminAIConnection('text', settings, apiKeys, 'csrf-admin')

    expect(fetcher.mock.calls[0][0]).toBe('/api/v1/admin/ai-settings')
    expect(fetcher.mock.calls[1][1].method).toBe('PUT')
    expect(fetcher.mock.calls[2][1].method).toBe('POST')
    for (const index of [1, 2]) {
      expect(fetcher.mock.calls[index][1].headers.get('X-CSRF-Token')).toBe('csrf-admin')
      expect(fetcher.mock.calls[index][1].credentials).toBe('include')
    }
    expect(JSON.parse(String(fetcher.mock.calls[1][1].body))).toMatchObject({ expectedVersion: 0, settings })
  })
})
