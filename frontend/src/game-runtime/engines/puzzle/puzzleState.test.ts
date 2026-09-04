import { describe, expect, it } from 'vitest'

import { createPuzzleState, placePuzzlePiece } from './puzzleState'

describe('puzzle state', () => {
  it('places a piece into an empty target slot', () => {
    const state = placePuzzlePiece(createPuzzleState(['a', 'b']), 'a', 0)

    expect(state.pieces[0]?.slotIndex).toBe(0)
    expect(state.placedCount).toBe(1)
    expect(state.actionCount).toBe(1)
    expect(state.status).toBe('playing')
  })

  it('ignores occupied, invalid, and repeated placements', () => {
    const initial = createPuzzleState(['a', 'b'])
    const placed = placePuzzlePiece(initial, 'a', 0)

    expect(placePuzzlePiece(placed, 'b', 0)).toBe(placed)
    expect(placePuzzlePiece(placed, 'a', 1)).toBe(placed)
    expect(placePuzzlePiece(placed, 'missing', 1)).toBe(placed)
    expect(placePuzzlePiece(placed, 'b', 2)).toBe(placed)
  })

  it.each([5, 4, 3, 2])('completes a %i-piece puzzle only after every slot is filled', (pieceCount) => {
    let state = createPuzzleState(
      Array.from({ length: pieceCount }, (_, index) => `piece-${index + 1}`),
    )
    for (let index = 0; index < pieceCount; index += 1) {
      state = placePuzzlePiece(state, state.pieces[index]!.id, index)
    }

    expect(state.placedCount).toBe(pieceCount)
    expect(state.actionCount).toBe(pieceCount)
    expect(state.status).toBe('completed')
  })
})
