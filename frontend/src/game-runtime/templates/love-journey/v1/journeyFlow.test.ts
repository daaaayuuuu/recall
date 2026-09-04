import { describe, expect, it } from 'vitest'

import { loveJourneyManifest, loveJourneyStageOrder } from './manifest'
import {
  loveJourneyPuzzlePieceCount,
  loveJourneyStageNumber,
  nextLoveJourneyStage,
} from './journeyFlow'

describe('love journey flow', () => {
  it('keeps five numbered experiences without a standalone puzzle stage', () => {
    expect(loveJourneyStageOrder).toEqual([
      'first-meeting',
      'dining',
      'movie',
      'travel',
      'password',
    ])
    expect(loveJourneyManifest.stages.map((stage) => stage.id)).toEqual(loveJourneyStageOrder)
    expect(loveJourneyManifest.stages.map((stage) => stage.status)).toEqual([
      'ready',
      'ready',
      'ready',
      'ready',
      'ready',
    ])
    expect(nextLoveJourneyStage('dining')).toBe('movie')
    expect(nextLoveJourneyStage('movie')).toBe('travel')
    expect(nextLoveJourneyStage('travel')).toBe('password')
    expect(nextLoveJourneyStage('password')).toBeUndefined()
    expect(loveJourneyStageNumber('password')).toBe(5)
    expect(loveJourneyManifest.stages.find((stage) => stage.id === 'travel')?.label).toBe('场景 4')
  })

  it('ends the first four experiences with progressively smaller puzzles', () => {
    expect(loveJourneyStageOrder.map(loveJourneyPuzzlePieceCount)).toEqual([5, 4, 3, 2, undefined])
  })
})
