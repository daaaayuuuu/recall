export type PuzzleStatus = 'playing' | 'completed'

export interface PuzzlePieceState {
  id: string
  targetSlotIndex: number
  slotIndex: number | null
}

export interface PuzzleState {
  pieces: PuzzlePieceState[]
  slotCount: number
  placedCount: number
  actionCount: number
  status: PuzzleStatus
}

export function createPuzzleState(pieceIds: string[]): PuzzleState {
  if (pieceIds.length === 0) throw new Error('Puzzle requires at least one piece')

  return {
    pieces: pieceIds.map((id, targetSlotIndex) => ({ id, targetSlotIndex, slotIndex: null })),
    slotCount: pieceIds.length,
    placedCount: 0,
    actionCount: 0,
    status: 'playing',
  }
}

export function placePuzzlePiece(
  state: PuzzleState,
  pieceId: string,
  slotIndex: number,
): PuzzleState {
  if (state.status === 'completed') return state
  if (!Number.isInteger(slotIndex) || slotIndex < 0 || slotIndex >= state.slotCount) return state

  const piece = state.pieces.find((candidate) => candidate.id === pieceId)
  if (!piece || piece.slotIndex !== null) return state
  if (piece.targetSlotIndex !== slotIndex) return state
  if (state.pieces.some((candidate) => candidate.slotIndex === slotIndex)) return state

  const pieces = state.pieces.map((candidate) =>
    candidate.id === pieceId ? { ...candidate, slotIndex } : candidate,
  )
  const placedCount = state.placedCount + 1

  return {
    ...state,
    pieces,
    placedCount,
    actionCount: state.actionCount + 1,
    status: placedCount === state.slotCount ? 'completed' : 'playing',
  }
}
