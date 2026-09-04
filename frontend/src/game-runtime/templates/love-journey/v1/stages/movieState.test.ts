import { describe, expect, it } from 'vitest'

import {
  calculateMovieDragProgress,
  calculateMovieDragRatio,
  calculateMovieProgress,
  cancelPlayerApproach,
  completePartnerResponse,
  createMovieState,
  isSuccessfulMovieDrag,
  MOVIE_DRAG_SUCCESS_RATIO,
  MOVIE_PLAYER_PROGRESS_PER_ROUND,
  startPartnerResponse,
  startPlayerApproach,
} from './movieState'

describe('movie game', () => {
  it('starts at round zero while waiting', () => {
    expect(createMovieState()).toEqual({ phase: 'waiting', round: 0 })
  })

  it('counts a round only after player movement and partner response', () => {
    const initial = createMovieState()
    const playerMoving = startPlayerApproach(initial)
    const partnerMoving = startPartnerResponse(playerMoving)

    expect(playerMoving).toEqual({ phase: 'player-moving', round: 0 })
    expect(partnerMoving).toEqual({ phase: 'partner-moving', round: 0 })
    expect(completePartnerResponse(partnerMoving)).toEqual({ phase: 'waiting', round: 1 })
  })

  it('ignores repeated and out-of-order actions during animation phases', () => {
    const initial = createMovieState()
    const playerMoving = startPlayerApproach(initial)
    const partnerMoving = startPartnerResponse(playerMoving)

    expect(startPlayerApproach(playerMoving)).toBe(playerMoving)
    expect(startPartnerResponse(initial)).toBe(initial)
    expect(completePartnerResponse(playerMoving)).toBe(playerMoving)
    expect(startPartnerResponse(partnerMoving)).toBe(partnerMoving)
  })

  it('returns to the round starting point when a drag is cancelled', () => {
    const initial = createMovieState()
    const playerMoving = startPlayerApproach(initial)

    expect(cancelPlayerApproach(playerMoving)).toEqual(initial)
    expect(cancelPlayerApproach(initial)).toBe(initial)
  })

  it('completes after exactly three rounds and cannot start a fourth', () => {
    let state = createMovieState()
    for (let round = 0; round < 3; round += 1) {
      state = startPlayerApproach(state)
      state = startPartnerResponse(state)
      state = completePartnerResponse(state)
    }

    expect(state).toEqual({ phase: 'completed', round: 3 })
    expect(startPlayerApproach(state)).toBe(state)
    expect(completePartnerResponse(state)).toBe(state)
  })

  it('meets at a seventy-thirty split after round three', () => {
    expect(calculateMovieProgress(3)).toEqual({
      maleProgress: 70,
      femaleProgress: 30,
      combined: 100,
    })
  })

  it('caps every drag at the fixed player distance for one round', () => {
    expect(calculateMovieDragRatio(-10, 100)).toBe(0)
    expect(calculateMovieDragRatio(50, 100)).toBe(0.5)
    expect(calculateMovieDragRatio(150, 100)).toBe(1)

    expect(calculateMovieDragProgress(0, 1)).toBeCloseTo(MOVIE_PLAYER_PROGRESS_PER_ROUND)
    expect(calculateMovieDragProgress(1, 1)).toBeCloseTo(MOVIE_PLAYER_PROGRESS_PER_ROUND * 2)
    expect(calculateMovieDragProgress(2, 2)).toBeCloseTo(70)
  })

  it('only accepts a drag that reaches the completion threshold', () => {
    expect(isSuccessfulMovieDrag(MOVIE_DRAG_SUCCESS_RATIO - 0.01)).toBe(false)
    expect(isSuccessfulMovieDrag(MOVIE_DRAG_SUCCESS_RATIO)).toBe(true)
    expect(isSuccessfulMovieDrag(1)).toBe(true)
  })
})
