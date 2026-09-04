import { describe, expect, it } from 'vitest'

import {
  createEnvelopeRevealState,
  nextEnvelopeItem,
  revealNextEnvelopeItem,
} from './envelopeState'

describe('envelope reveal', () => {
  it('reveals every photo before the letter', () => {
    let state = createEnvelopeRevealState()

    expect(nextEnvelopeItem(state, 3)).toBe('photo')
    state = revealNextEnvelopeItem(state, 3)
    state = revealNextEnvelopeItem(state, 3)
    state = revealNextEnvelopeItem(state, 3)
    expect(state.revealedPhotoCount).toBe(3)
    expect(nextEnvelopeItem(state, 3)).toBe('letter')

    state = revealNextEnvelopeItem(state, 3)
    expect(state.letterRevealed).toBe(true)
    expect(nextEnvelopeItem(state, 3)).toBe('completed')
    expect(revealNextEnvelopeItem(state, 3)).toBe(state)
  })

  it('reveals the letter first when the user uploaded no photos', () => {
    const state = revealNextEnvelopeItem(createEnvelopeRevealState(), 0)

    expect(state.revealedPhotoCount).toBe(0)
    expect(state.letterRevealed).toBe(true)
  })
})
