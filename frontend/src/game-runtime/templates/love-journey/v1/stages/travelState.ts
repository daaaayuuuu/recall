export type TravelPhase = 'packing' | 'ready-to-close' | 'closing' | 'completed'

export interface TravelItem {
  id: string
  packed: boolean
}

export interface TravelState {
  phase: TravelPhase
  items: TravelItem[]
}

const travelItemIds = ['camera', 'hat', 'ticket', 'charger'] as const

export function createTravelState(): TravelState {
  return {
    phase: 'packing',
    items: travelItemIds.map((id) => ({ id, packed: false })),
  }
}

export function packTravelItem(state: TravelState, itemId: string): TravelState {
  if (state.phase !== 'packing') return state
  const target = state.items.find((item) => item.id === itemId)
  if (!target || target.packed) return state

  const items = state.items.map((item) =>
    item.id === itemId ? { ...item, packed: true } : item,
  )
  return {
    phase: items.every((item) => item.packed) ? 'ready-to-close' : 'packing',
    items,
  }
}

export function closeTravelSuitcase(state: TravelState): TravelState {
  if (state.phase !== 'ready-to-close' || state.items.some((item) => !item.packed)) return state
  return { ...state, phase: 'closing' }
}

export function completeTravelSuitcase(state: TravelState): TravelState {
  if (state.phase !== 'closing') return state
  return { ...state, phase: 'completed' }
}
