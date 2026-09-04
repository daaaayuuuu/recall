import { describe, expect, it } from 'vitest'

import {
  closeTravelSuitcase,
  completeTravelSuitcase,
  createTravelState,
  packTravelItem,
} from './travelState'

function packAllItems() {
  return createTravelState().items.reduce(
    (state, item) => packTravelItem(state, item.id),
    createTravelState(),
  )
}

describe('travel game', () => {
  it('starts in packing with four unpacked items', () => {
    const state = createTravelState()

    expect(state.phase).toBe('packing')
    expect(state.items).toHaveLength(4)
    expect(state.items.every((item) => !item.packed)).toBe(true)
  })

  it('packs each known item only once', () => {
    const initial = createTravelState()
    const packed = packTravelItem(initial, 'camera')

    expect(packed.items.filter((item) => item.packed)).toHaveLength(1)
    expect(packTravelItem(packed, 'camera')).toBe(packed)
    expect(packTravelItem(packed, 'unknown')).toBe(packed)
  })

  it('waits for an explicit suitcase close after all four items are packed', () => {
    const ready = packAllItems()

    expect(ready.phase).toBe('ready-to-close')
    expect(closeTravelSuitcase(createTravelState())).toEqual(createTravelState())
    expect(closeTravelSuitcase(ready).phase).toBe('closing')
  })

  it('completes only after the suitcase closing animation', () => {
    const ready = packAllItems()
    const closing = closeTravelSuitcase(ready)

    expect(completeTravelSuitcase(ready)).toBe(ready)
    expect(completeTravelSuitcase(closing).phase).toBe('completed')
  })
})
