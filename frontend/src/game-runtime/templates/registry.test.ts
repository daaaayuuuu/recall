import { describe, expect, it } from 'vitest'

import { findTemplate, listRegisteredTemplates } from './registry'

describe('game template registry', () => {
  it('loads both supported love journey config versions', async () => {
    const legacyDefinition = findTemplate('love-journey', '1.0.0')
    const definition = findTemplate('love-journey', '1.1.0')

    expect(definition?.displayName).toBe('爱的旅程')
    expect(legacyDefinition?.displayName).toBe('爱的旅程')
    expect(findTemplate('love-journey', '2.0.0')).toBeUndefined()
    expect(listRegisteredTemplates()).toHaveLength(2)
    await expect(definition?.load()).resolves.toHaveProperty('default')
  })
})
