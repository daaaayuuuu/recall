import { describe, expect, it } from 'vitest'

import {
  advanceAfterDisplay,
  createFirstMeetingState,
  firstMeetingRoundSequences,
  getCurrentSequence,
  getExpectedEmoji,
  selectEmoji,
  sendCurrentRound,
  type FirstMeetingEmoji,
  type FirstMeetingState,
} from './firstMeetingState'

function selectCurrentRound(state: FirstMeetingState) {
  return getCurrentSequence(state).reduce(selectEmoji, state)
}

describe('first meeting state', () => {
  it('uses the three frozen round sequences', () => {
    expect(firstMeetingRoundSequences).toEqual([
      ['wink', 'heart', 'laugh', 'blush'],
      ['heart', 'blush', 'wink', 'laugh'],
      ['blush', 'laugh', 'wink', 'heart'],
    ])

    for (const sequence of firstMeetingRoundSequences) {
      expect(sequence).toHaveLength(4)
      expect(new Set(sequence)).toEqual(new Set<FirstMeetingEmoji>([
        'wink',
        'heart',
        'laugh',
        'blush',
      ]))
    }
    expect(new Set(firstMeetingRoundSequences.map((sequence) => sequence.join(','))).size).toBe(3)
  })

  it('starts on round one and exposes only its first expected emoji', () => {
    const state = createFirstMeetingState()

    expect(state).toEqual({
      phase: 'selecting',
      roundIndex: 0,
      selectedEmojis: [],
      playerMessage: null,
    })
    expect(getCurrentSequence(state)).toBe(firstMeetingRoundSequences[0])
    expect(getExpectedEmoji(state)).toBe('wink')
  })

  it('ignores a wrong emoji without changing state or reference', () => {
    const state = createFirstMeetingState()
    const wrongSelection = selectEmoji(state, 'heart')

    expect(wrongSelection).toBe(state)
    expect(wrongSelection.selectedEmojis).toEqual([])
    expect(getExpectedEmoji(wrongSelection)).toBe('wink')
  })

  it('accepts only the expected order and becomes ready after four selections', () => {
    let state = createFirstMeetingState()
    for (const [index, emoji] of firstMeetingRoundSequences[0].entries()) {
      state = selectEmoji(state, emoji)
      expect(state.selectedEmojis).toEqual(firstMeetingRoundSequences[0].slice(0, index + 1))
      expect(state.phase).toBe(index === 3 ? 'ready-to-send' : 'selecting')
    }

    expect(getExpectedEmoji(state)).toBeNull()
    expect(selectEmoji(state, 'wink')).toBe(state)
  })

  it('cannot send early and copies the completed selection into the player message', () => {
    const initial = createFirstMeetingState()
    expect(sendCurrentRound(initial)).toBe(initial)

    const ready = selectCurrentRound(initial)
    const sent = sendCurrentRound(ready)

    expect(sent.phase).toBe('sent')
    expect(sent.playerMessage).toEqual(firstMeetingRoundSequences[0])
    expect(sent.playerMessage).not.toBe(sent.selectedEmojis)
    expect(sendCurrentRound(sent)).toBe(sent)
    expect(selectEmoji(sent, 'wink')).toBe(sent)
  })

  it('clears transient state and switches to each next sequence after display', () => {
    const firstSent = sendCurrentRound(selectCurrentRound(createFirstMeetingState()))
    const secondRound = advanceAfterDisplay(firstSent)

    expect(secondRound).toEqual({
      phase: 'selecting',
      roundIndex: 1,
      selectedEmojis: [],
      playerMessage: null,
    })
    expect(getCurrentSequence(secondRound)).toBe(firstMeetingRoundSequences[1])
    expect(getExpectedEmoji(secondRound)).toBe('heart')

    const secondSent = sendCurrentRound(selectCurrentRound(secondRound))
    const thirdRound = advanceAfterDisplay(secondSent)
    expect(thirdRound.roundIndex).toBe(2)
    expect(thirdRound.selectedEmojis).toEqual([])
    expect(thirdRound.playerMessage).toBeNull()
    expect(getCurrentSequence(thirdRound)).toBe(firstMeetingRoundSequences[2])
    expect(getExpectedEmoji(thirdRound)).toBe('blush')
  })

  it('completes only after the third sent message has been displayed', () => {
    let state = createFirstMeetingState()
    for (let roundIndex = 0; roundIndex < firstMeetingRoundSequences.length; roundIndex += 1) {
      state = selectCurrentRound(state)
      state = sendCurrentRound(state)
      expect(state.phase).toBe('sent')
      state = advanceAfterDisplay(state)
    }

    expect(state.phase).toBe('completed')
    expect(state.roundIndex).toBe(2)
    expect(state.playerMessage).toEqual(firstMeetingRoundSequences[2])
    expect(getExpectedEmoji(state)).toBeNull()
    expect(advanceAfterDisplay(state)).toBe(state)
    expect(sendCurrentRound(state)).toBe(state)
    expect(selectEmoji(state, 'blush')).toBe(state)
  })

  it('does not advance from any phase other than sent', () => {
    const selecting = createFirstMeetingState()
    const ready = selectCurrentRound(selecting)

    expect(advanceAfterDisplay(selecting)).toBe(selecting)
    expect(advanceAfterDisplay(ready)).toBe(ready)
  })
})
