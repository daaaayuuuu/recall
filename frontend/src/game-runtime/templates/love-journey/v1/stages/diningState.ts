export const diningFoodIds = ['meal', 'sushi', 'shrimp', 'salad', 'cake'] as const

export type DiningFoodId = (typeof diningFoodIds)[number]

export interface FoodItem {
  id: DiningFoodId
  eaten: boolean
}

export interface DiningState {
  status: 'playing' | 'completed'
  foods: FoodItem[]
  eatenCount: number
}

export function createDiningState(): DiningState {
  return {
    status: 'playing',
    foods: diningFoodIds.map((id) => ({ id, eaten: false })),
    eatenCount: 0,
  }
}

export function eatFood(state: DiningState, foodId: DiningFoodId): DiningState {
  if (state.status !== 'playing') return state
  const target = state.foods.find((food) => food.id === foodId)
  if (!target || target.eaten) return state

  const foods = state.foods.map((food) =>
    food.id === foodId ? { ...food, eaten: true } : food,
  )
  const eatenCount = state.eatenCount + 1

  return {
    status: foods.every((food) => food.eaten) ? 'completed' : 'playing',
    foods,
    eatenCount,
  }
}
