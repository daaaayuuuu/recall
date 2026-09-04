export const loveJourneyStageOrder = [
  'first-meeting',
  'dining',
  'movie',
  'travel',
  'password',
] as const

export type LoveJourneyStageId = (typeof loveJourneyStageOrder)[number]

export interface LoveJourneyStageDefinition {
  id: LoveJourneyStageId
  sequence: number
  label: string
  title: string
  status: 'ready' | 'placeholder'
}

export const loveJourneyManifest = {
  id: 'love-journey',
  version: '1.1.0',
  displayName: '爱的旅程',
  orientation: 'portrait',
  initialStageId: 'first-meeting' as LoveJourneyStageId,
  stages: [
    { id: 'first-meeting', sequence: 1, label: '场景 1', title: '初见', status: 'ready' },
    { id: 'dining', sequence: 2, label: '场景 2', title: '吃饭', status: 'ready' },
    { id: 'movie', sequence: 3, label: '场景 3', title: '看电影', status: 'ready' },
    { id: 'travel', sequence: 4, label: '场景 4', title: '旅行', status: 'ready' },
    { id: 'password', sequence: 5, label: '场景 5', title: '密码', status: 'ready' },
  ] satisfies LoveJourneyStageDefinition[],
}
