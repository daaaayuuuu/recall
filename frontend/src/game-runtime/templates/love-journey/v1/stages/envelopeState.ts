export interface EnvelopeRevealState {
  revealedPhotoCount: number
  letterRevealed: boolean
}

export type EnvelopeItemKind = 'photo' | 'letter' | 'completed'

export function createEnvelopeRevealState(): EnvelopeRevealState {
  return { revealedPhotoCount: 0, letterRevealed: false }
}

export function nextEnvelopeItem(state: EnvelopeRevealState, photoCount: number): EnvelopeItemKind {
  if (state.letterRevealed) return 'completed'
  return state.revealedPhotoCount < photoCount ? 'photo' : 'letter'
}

export function revealNextEnvelopeItem(
  state: EnvelopeRevealState,
  photoCount: number,
): EnvelopeRevealState {
  const next = nextEnvelopeItem(state, photoCount)
  if (next === 'photo') return { ...state, revealedPhotoCount: state.revealedPhotoCount + 1 }
  if (next === 'letter') return { ...state, letterRevealed: true }
  return state
}
