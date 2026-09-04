export type MoviePhase = 'waiting' | 'player-moving' | 'partner-moving' | 'completed'

export type MovieRound = 0 | 1 | 2 | 3

export interface MovieState {
  phase: MoviePhase
  round: MovieRound
}

export interface MovieProgress {
  maleProgress: number
  femaleProgress: number
  combined: number
}

export const MOVIE_ROUND_COUNT = 3
export const MOVIE_PLAYER_TOTAL_PROGRESS = 70
export const MOVIE_PARTNER_TOTAL_PROGRESS = 30
export const MOVIE_PLAYER_PROGRESS_PER_ROUND = MOVIE_PLAYER_TOTAL_PROGRESS / MOVIE_ROUND_COUNT
export const MOVIE_DRAG_SUCCESS_RATIO = 0.8

export function createMovieState(): MovieState {
  return { phase: 'waiting', round: 0 }
}

export function startPlayerApproach(state: MovieState): MovieState {
  if (state.phase !== 'waiting' || state.round >= MOVIE_ROUND_COUNT) return state
  return { ...state, phase: 'player-moving' }
}

export function cancelPlayerApproach(state: MovieState): MovieState {
  if (state.phase !== 'player-moving') return state
  return { ...state, phase: 'waiting' }
}

export function startPartnerResponse(state: MovieState): MovieState {
  if (state.phase !== 'player-moving') return state
  return { ...state, phase: 'partner-moving' }
}

export function completePartnerResponse(state: MovieState): MovieState {
  if (state.phase !== 'partner-moving') return state
  const round = Math.min(MOVIE_ROUND_COUNT, state.round + 1) as MovieRound
  return { phase: round === MOVIE_ROUND_COUNT ? 'completed' : 'waiting', round }
}

export function calculateMovieProgress(round: MovieRound): MovieProgress {
  return {
    maleProgress: (round * MOVIE_PLAYER_TOTAL_PROGRESS) / MOVIE_ROUND_COUNT,
    femaleProgress: (round * MOVIE_PARTNER_TOTAL_PROGRESS) / MOVIE_ROUND_COUNT,
    combined: (round * 100) / MOVIE_ROUND_COUNT,
  }
}

export function calculateMovieDragRatio(distance: number, maximumDistance: number): number {
  if (maximumDistance <= 0) return 0
  return Math.min(1, Math.max(0, distance / maximumDistance))
}

export function calculateMovieDragProgress(round: MovieRound, dragRatio: number): number {
  const settledProgress = calculateMovieProgress(round).maleProgress
  const boundedRatio = Math.min(1, Math.max(0, dragRatio))
  return settledProgress + (MOVIE_PLAYER_PROGRESS_PER_ROUND * boundedRatio)
}

export function isSuccessfulMovieDrag(dragRatio: number): boolean {
  return dragRatio >= MOVIE_DRAG_SUCCESS_RATIO
}
