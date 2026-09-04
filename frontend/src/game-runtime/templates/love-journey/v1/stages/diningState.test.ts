import { describe, expect, it } from 'vitest'

import { createDiningState, eatFood } from './diningState'

describe('dining game', () => {
  it('eats the five illustrated dishes only once and then completes', () => {
    let state = createDiningState()
    expect(state.foods.map((food) => food.id)).toEqual([
      'meal',
      'sushi',
      'shrimp',
      'salad',
      'cake',
    ])

    for (const food of state.foods) state = eatFood(state, food.id)
    state = eatFood(state, 'cake')

    expect(state.status).toBe('completed')
    expect(state.eatenCount).toBe(5)
  })
})
