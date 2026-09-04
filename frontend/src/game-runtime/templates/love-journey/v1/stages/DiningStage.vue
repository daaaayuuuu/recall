<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'

import type { GameStageResult } from '@/game-runtime/types'

import {
  createDiningState,
  eatFood,
  type DiningFoodId,
  type FoodItem,
} from './diningState'

interface DiningFoodArtwork {
  id: DiningFoodId
  label: string
  image: string
  selectionImage: string
}

const foodArtwork: DiningFoodArtwork[] = [
  {
    id: 'meal',
    label: '什锦餐盘',
    image: '/assets/love-journey/dining/meal.png',
    selectionImage: '/assets/love-journey/dining/meal-click-area.png',
  },
  {
    id: 'sushi',
    label: '寿司拼盘',
    image: '/assets/love-journey/dining/sushi.png',
    selectionImage: '/assets/love-journey/dining/sushi-click-area.png',
  },
  {
    id: 'shrimp',
    label: '鲜虾拼盘',
    image: '/assets/love-journey/dining/shrimp.png',
    selectionImage: '/assets/love-journey/dining/shrimp-click-area.png',
  },
  {
    id: 'salad',
    label: '蔬菜沙拉',
    image: '/assets/love-journey/dining/salad.png',
    selectionImage: '/assets/love-journey/dining/salad-click-area.png',
  },
  {
    id: 'cake',
    label: '莓果蛋糕',
    image: '/assets/love-journey/dining/cake.png',
    selectionImage: '/assets/love-journey/dining/cake-click-area.png',
  },
]
const selectionDisplayMilliseconds = 460

const props = defineProps<{ active: boolean }>()
const emit = defineEmits<{ complete: [result: GameStageResult] }>()

const state = ref(createDiningState())
const selectedFoodId = ref<DiningFoodId | null>(null)
const completionEmitted = ref(false)
let selectionTimer: number | undefined

const remaining = computed(() => state.value.foods.length - state.value.eatenCount)
const selectedArtwork = computed(() => (
  foodArtwork.find((artwork) => artwork.id === selectedFoodId.value) ?? null
))
const interactionLocked = computed(() => (
  !props.active || state.value.status !== 'playing' || selectedFoodId.value !== null
))
const statusText = computed(() => {
  if (state.value.status === 'completed') return '五道菜已经全部吃完，吃饭完成'
  if (selectedArtwork.value) return `正在品尝${selectedArtwork.value.label}`
  return `还剩 ${remaining.value} 道菜`
})

function selectFood(food: FoodItem) {
  if (interactionLocked.value || food.eaten || selectionTimer !== undefined) return

  selectedFoodId.value = food.id
  selectionTimer = globalThis.window.setTimeout(() => {
    selectionTimer = undefined
    if (!props.active || state.value.status !== 'playing') {
      selectedFoodId.value = null
      return
    }

    state.value = eatFood(state.value, food.id)
    selectedFoodId.value = null
    if (state.value.status === 'completed') finish(food.id)
  }, selectionDisplayMilliseconds)
}

function finish(lastFoodId: DiningFoodId) {
  if (completionEmitted.value || state.value.status !== 'completed') return
  completionEmitted.value = true
  globalThis.navigator.vibrate?.([18, 30, 18])
  emit('complete', {
    stageId: 'dining',
    completedAt: Date.now(),
    actionCount: state.value.eatenCount,
    metadata: { lastFoodId },
  })
}

onBeforeUnmount(() => {
  if (selectionTimer !== undefined) globalThis.window.clearTimeout(selectionTimer)
})
</script>

<template>
  <section
    class="journey-scene dining-scene"
    aria-labelledby="dining-title"
    :data-status="state.status"
  >
    <h1
      id="dining-title"
      class="dining-visually-hidden"
    >
      场景 2：吃饭
    </h1>

    <div
      class="dining-artboard"
      aria-label="两个人一起吃饭，点击餐桌上的五道菜"
    >
      <img
        class="dining-artboard__background"
        src="/assets/love-journey/dining/background.png"
        alt=""
        aria-hidden="true"
      >
      <img
        class="dining-person dining-person--left"
        src="/assets/love-journey/dining/left-person.png"
        alt=""
        aria-hidden="true"
      >
      <img
        class="dining-person dining-person--right"
        src="/assets/love-journey/dining/right-person.png"
        alt=""
        aria-hidden="true"
      >

      <img
        v-if="selectedArtwork"
        :key="selectedArtwork.id"
        class="dining-selection"
        :class="`dining-selection--${selectedArtwork.id}`"
        :src="selectedArtwork.selectionImage"
        alt=""
        aria-hidden="true"
        :data-food-selection="selectedArtwork.id"
      >

      <template
        v-for="(food, index) in state.foods"
        :key="food.id"
      >
        <img
          class="dining-food"
          :class="[
            `dining-food--${food.id}`,
            { 'dining-food--eaten': food.eaten },
          ]"
          :src="foodArtwork[index].image"
          alt=""
          aria-hidden="true"
          :data-food-image="food.id"
        >
        <button
          class="dining-food-button"
          :class="`dining-food-button--${food.id}`"
          type="button"
          :aria-label="`吃掉${foodArtwork[index].label}`"
          :aria-pressed="selectedFoodId === food.id"
          :disabled="interactionLocked || food.eaten"
          :data-food="food.id"
          @click="selectFood(food)"
        >
          <span class="dining-visually-hidden">{{ foodArtwork[index].label }}</span>
        </button>
      </template>

      <p
        class="dining-visually-hidden"
        role="status"
        aria-live="polite"
      >
        {{ statusText }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.dining-scene {
  --dining-focus: #ffd43b;

  position: relative;
  display: block;
  min-height: min(726px, calc(100dvh - 34px));
  padding: 0;
  overflow: hidden;
  background: #fdfcfb;
  container-type: size;
  isolation: isolate;
}

.dining-artboard {
  position: absolute;
  z-index: 0;
  top: 50%;
  left: 50%;
  width: 100%;
  width: max(100cqw, calc(100cqh * 0.562799));
  height: auto;
  overflow: hidden;
  aspect-ratio: 941 / 1672;
  transform: translate(-50%, -50%);
}

.dining-artboard__background {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: fill;
  pointer-events: none;
  user-select: none;
}

.dining-person,
.dining-food,
.dining-selection {
  position: absolute;
  display: block;
  pointer-events: none;
  user-select: none;
}

.dining-person {
  z-index: 1;
  height: auto;
}

.dining-person--left {
  top: 20.76%;
  left: 5.46%;
  width: 42.98%;
}

.dining-person--right {
  top: 15.05%;
  left: 45.87%;
  width: 53.13%;
}

.dining-food {
  z-index: 2;
  height: auto;
  transform-origin: center;
  transition: opacity 210ms ease-out, transform 240ms cubic-bezier(0.3, 0.8, 0.3, 1);
}

.dining-food--meal {
  top: 62.08%;
  left: 15.49%;
  width: 29.56%;
}

.dining-food--sushi {
  top: 61.18%;
  left: 53.5%;
  width: 28.05%;
}

.dining-food--shrimp {
  top: 77.18%;
  left: 8.16%;
  width: 25.02%;
}

.dining-food--salad {
  top: 77.45%;
  left: 39.21%;
  width: 28.45%;
}

.dining-food--cake {
  top: 77.75%;
  left: 74.52%;
  width: 16.51%;
}

.dining-food--eaten {
  opacity: 0;
  transform: translateY(3%) scale(0.78);
}

.dining-selection {
  z-index: 3;
  object-fit: fill;
  animation: dining-selection-pop 180ms cubic-bezier(0.2, 0.85, 0.3, 1.2);
}

.dining-selection--meal {
  top: 58.85%;
  left: 10.84%;
  width: 39.11%;
  height: 16.39%;
}

.dining-selection--sushi {
  top: 58.85%;
  left: 49.52%;
  width: 36.77%;
  height: 16.87%;
}

.dining-selection--shrimp {
  top: 73.68%;
  left: 0.43%;
  width: 37.94%;
  height: 19.02%;
}

.dining-selection--salad {
  top: 73.56%;
  left: 34.64%;
  width: 39.11%;
  height: 19.02%;
}

.dining-selection--cake {
  top: 75.36%;
  left: 69.08%;
  width: 29.54%;
  height: 16.63%;
}

@keyframes dining-selection-pop {
  from {
    opacity: 0;
    transform: scale(0.84);
  }
}

.dining-food-button {
  position: absolute;
  z-index: 4;
  padding: 0;
  border: 0;
  border-radius: 50%;
  outline: 0;
  background: transparent;
  cursor: pointer;
  touch-action: manipulation;
  -webkit-tap-highlight-color: transparent;
}

.dining-food-button--meal {
  top: 60.6%;
  left: 11.4%;
  width: 37.5%;
  height: 15.2%;
}

.dining-food-button--sushi {
  top: 60.2%;
  left: 49.6%;
  width: 35.2%;
  height: 15.5%;
}

.dining-food-button--shrimp {
  top: 75.4%;
  left: 2.2%;
  width: 35.5%;
  height: 17.2%;
}

.dining-food-button--salad {
  top: 75.5%;
  left: 35.6%;
  width: 36.4%;
  height: 17.2%;
}

.dining-food-button--cake {
  top: 76.3%;
  left: 68.7%;
  width: 29.1%;
  height: 16.2%;
}

.dining-food-button:focus-visible {
  outline: 4px solid var(--dining-focus);
  outline-offset: -8px;
}

.dining-food-button:disabled {
  cursor: wait;
}

.dining-visually-hidden {
  position: absolute !important;
  width: 1px !important;
  height: 1px !important;
  padding: 0 !important;
  overflow: hidden !important;
  clip: rect(0, 0, 0, 0) !important;
  white-space: nowrap !important;
  border: 0 !important;
}

@media (prefers-reduced-motion: reduce) {
  .dining-food {
    transition: none;
  }

  .dining-selection {
    animation: none;
  }
}
</style>
