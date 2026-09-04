import type { GameTemplateDefinition } from '@/game-runtime/types'

const templateDefinitions: GameTemplateDefinition[] = [
  {
    id: 'love-journey',
    version: '1.0.0',
    displayName: '爱的旅程',
    load: () => import('./love-journey/v1'),
  },
  {
    id: 'love-journey',
    version: '1.1.0',
    displayName: '爱的旅程',
    load: () => import('./love-journey/v1'),
  },
]

const templateRegistry = new Map(
  templateDefinitions.map((definition) => [
    `${definition.id}@${definition.version}`,
    definition,
  ]),
)

export function findTemplate(id: string, version: string) {
  return templateRegistry.get(`${id}@${version}`)
}

export function listRegisteredTemplates() {
  return [...templateDefinitions]
}
