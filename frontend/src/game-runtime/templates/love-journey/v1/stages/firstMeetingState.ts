export type FirstMeetingEmoji = 'wink' | 'heart' | 'laugh' | 'blush'

export type FirstMeetingPhase = 'selecting' | 'ready-to-send' | 'sent' | 'completed'

export interface FirstMeetingState {
  phase: FirstMeetingPhase
  roundIndex: 0 | 1 | 2
  selectedEmojis: FirstMeetingEmoji[]
  playerMessage: FirstMeetingEmoji[] | null
}

export const firstMeetingRoundSequences = [
  ['wink', 'heart', 'laugh', 'blush'],
  ['heart', 'blush', 'wink', 'laugh'],
  ['blush', 'laugh', 'wink', 'heart'],
] as const satisfies readonly (readonly FirstMeetingEmoji[])[]

export function createFirstMeetingState(): FirstMeetingState {
  return {
    phase: 'selecting',
    roundIndex: 0,
    selectedEmojis: [],
    playerMessage: null,
  }
}

export function getCurrentSequence(
  state: FirstMeetingState,
): (typeof firstMeetingRoundSequences)[number] {
  return firstMeetingRoundSequences[state.roundIndex]
}

export function getExpectedEmoji(state: FirstMeetingState): FirstMeetingEmoji | null {
  if (state.phase !== 'selecting') return null
  return getCurrentSequence(state)[state.selectedEmojis.length] ?? null
}

export function selectEmoji(
  state: FirstMeetingState,
  emoji: FirstMeetingEmoji,
): FirstMeetingState {
  if (getExpectedEmoji(state) !== emoji) return state

  const selectedEmojis = [...state.selectedEmojis, emoji]
  return {
    ...state,
    phase: selectedEmojis.length === getCurrentSequence(state).length
      ? 'ready-to-send'
      : 'selecting',
    selectedEmojis,
  }
}

export function sendCurrentRound(state: FirstMeetingState): FirstMeetingState {
  if (state.phase !== 'ready-to-send') return state
  return {
    ...state,
    phase: 'sent',
    playerMessage: [...state.selectedEmojis],
  }
}

export function advanceAfterDisplay(state: FirstMeetingState): FirstMeetingState {
  if (state.phase !== 'sent') return state
  if (state.roundIndex === 2) return { ...state, phase: 'completed' }

  return {
    phase: 'selecting',
    roundIndex: (state.roundIndex + 1) as 1 | 2,
    selectedEmojis: [],
    playerMessage: null,
  }
}
