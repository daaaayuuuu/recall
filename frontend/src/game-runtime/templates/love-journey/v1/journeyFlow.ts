import { loveJourneyStageOrder, type LoveJourneyStageId } from './manifest'

const puzzlePieceCountByStage = {
  'first-meeting': 5,
  dining: 4,
  movie: 3,
  travel: 2,
} satisfies Partial<Record<LoveJourneyStageId, number>>

export function nextLoveJourneyStage(stageId: LoveJourneyStageId) {
  const index = loveJourneyStageOrder.indexOf(stageId)
  return loveJourneyStageOrder[index + 1]
}

export function loveJourneyStageNumber(stageId: LoveJourneyStageId) {
  return loveJourneyStageOrder.indexOf(stageId) + 1
}

export function loveJourneyPuzzlePieceCount(stageId: LoveJourneyStageId) {
  return puzzlePieceCountByStage[stageId as keyof typeof puzzlePieceCountByStage]
}
