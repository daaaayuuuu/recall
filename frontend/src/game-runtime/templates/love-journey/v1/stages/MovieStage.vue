<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'

import type { GameStageResult } from '@/game-runtime/types'

import {
  calculateMovieDragProgress,
  calculateMovieDragRatio,
  calculateMovieProgress,
  cancelPlayerApproach,
  completePartnerResponse,
  createMovieState,
  isSuccessfulMovieDrag,
  MOVIE_PLAYER_PROGRESS_PER_ROUND,
  MOVIE_ROUND_COUNT,
  startPartnerResponse,
  startPlayerApproach,
  type MovieRound,
} from './movieState'

type MoviePointerEvent = InstanceType<typeof globalThis.PointerEvent>
type MovieHTMLElement = InstanceType<typeof globalThis.HTMLElement>

const MOVIE_COMPLETION_REVEAL_MS = 900

const props = defineProps<{ active: boolean }>()
const emit = defineEmits<{ complete: [result: GameStageResult] }>()

const state = ref(createMovieState())
const trackElement = ref<MovieHTMLElement>()
const dragPointerId = ref<number | null>(null)
const dragRatio = ref(0)
const dragFeedback = ref<'idle' | 'retry'>('idle')
const completionEmitted = ref(false)
let playerTimer: number | undefined
let partnerTimer: number | undefined
let completionTimer: number | undefined
let dragStartClientX = 0
let dragMaximumDistance = 0

const completedProgress = computed(() => calculateMovieProgress(state.value.round))
const maleProgress = computed(() => {
  if (state.value.phase === 'player-moving') {
    return calculateMovieDragProgress(state.value.round, dragRatio.value)
  }
  const round = state.value.round + (state.value.phase === 'partner-moving' ? 1 : 0)
  return calculateMovieProgress(round as MovieRound).maleProgress
})
const femaleProgress = computed(() => {
  const round = state.value.round + (state.value.phase === 'partner-moving' ? 1 : 0)
  return calculateMovieProgress(round as MovieRound).femaleProgress
})
const handsVisualState = computed<'open' | 'touching' | 'touched'>(() => {
  if (state.value.phase === 'completed') return 'touched'
  if (state.value.round >= 2) return 'touching'
  return 'open'
})
const openHandsApproachRatio = computed(() => {
  const progressBeforeTouching = (MOVIE_ROUND_COUNT - 1) * (100 / MOVIE_ROUND_COUNT)
  const combinedProgress = maleProgress.value + femaleProgress.value
  return Math.min(1, combinedProgress / progressBeforeTouching)
})
const playerHandShift = computed(() => openHandsApproachRatio.value * 175)
const partnerHandShift = computed(() => openHandsApproachRatio.value * -75)
const dragRoundProgress = computed(() => state.value.round + dragRatio.value)
const dragValueText = computed(() => `已完成 ${state.value.round} / ${MOVIE_ROUND_COUNT} 轮靠近`)
const statusText = computed(() => {
  if (state.value.phase === 'completed') return '这一次，你们牵住了彼此。'
  if (state.value.phase === 'player-moving') {
    return dragPointerId.value === null ? '就快碰到了……' : '按住不放，向她靠近……'
  }
  if (state.value.phase === 'partner-moving') return '她也在向你靠近。'
  if (dragFeedback.value === 'retry') return '还差一点，再向她靠近一些。'
  if (state.value.round === 2) return '最后一次，牵住她的手。'
  if (state.value.round === 1) return '再靠近一点。按住「你」继续向右拖动。'
  return '按住「你」，向她拖动。'
})

function prefersReducedMotion() {
  return globalThis.window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

function settleSuccessfulDrag() {
  if (!props.active || state.value.phase !== 'waiting' || playerTimer !== undefined) return
  state.value = startPlayerApproach(state.value)
  dragRatio.value = 1
  dragFeedback.value = 'idle'
  startPartnerApproach()
}

function startPartnerApproach() {
  const reducedMotion = prefersReducedMotion()
  playerTimer = globalThis.window.setTimeout(() => {
    playerTimer = undefined
    state.value = startPartnerResponse(state.value)
    partnerTimer = globalThis.window.setTimeout(() => {
      partnerTimer = undefined
      state.value = completePartnerResponse(state.value)
      dragRatio.value = 0
      if (state.value.phase === 'completed' && !completionEmitted.value) finish()
    }, reducedMotion ? 1 : 280)
  }, reducedMotion ? 1 : 180)
}

function startDrag(event: MoviePointerEvent) {
  if (!props.active || state.value.phase !== 'waiting' || playerTimer !== undefined) return
  if (event.pointerType === 'mouse' && event.button !== 0) return

  const trackWidth = trackElement.value?.getBoundingClientRect().width ?? 0
  dragMaximumDistance = trackWidth * (MOVIE_PLAYER_PROGRESS_PER_ROUND / 100)
  if (dragMaximumDistance <= 0) return

  event.preventDefault()
  state.value = startPlayerApproach(state.value)
  dragFeedback.value = 'idle'
  dragRatio.value = 0
  dragStartClientX = event.clientX
  dragPointerId.value = event.pointerId
  const handle = event.currentTarget as MovieHTMLElement
  handle.setPointerCapture(event.pointerId)
}

function moveDrag(event: MoviePointerEvent) {
  if (event.pointerId !== dragPointerId.value || state.value.phase !== 'player-moving') return
  event.preventDefault()
  dragRatio.value = calculateMovieDragRatio(
    event.clientX - dragStartClientX,
    dragMaximumDistance,
  )
}

function endDrag(event: MoviePointerEvent, cancelled = false) {
  if (event.pointerId !== dragPointerId.value || state.value.phase !== 'player-moving') return

  if (!cancelled) {
    dragRatio.value = calculateMovieDragRatio(
      event.clientX - dragStartClientX,
      dragMaximumDistance,
    )
  }
  const pointerId = dragPointerId.value
  dragPointerId.value = null
  const handle = event.currentTarget as MovieHTMLElement
  if (pointerId !== null && handle.hasPointerCapture(pointerId)) {
    handle.releasePointerCapture(pointerId)
  }

  if (!cancelled && isSuccessfulMovieDrag(dragRatio.value)) {
    dragRatio.value = 1
    startPartnerApproach()
    return
  }

  state.value = cancelPlayerApproach(state.value)
  dragRatio.value = 0
  dragFeedback.value = 'retry'
}

function approachWithKeyboard() {
  settleSuccessfulDrag()
}

function finish() {
  completionEmitted.value = true
  globalThis.navigator.vibrate?.([18, 30, 18])
  completionTimer = globalThis.window.setTimeout(() => {
    completionTimer = undefined
    emit('complete', {
      stageId: 'movie',
      completedAt: Date.now(),
      actionCount: MOVIE_ROUND_COUNT,
      metadata: { approachRatio: '70:30', rounds: MOVIE_ROUND_COUNT },
    })
  }, MOVIE_COMPLETION_REVEAL_MS)
}

onBeforeUnmount(() => {
  if (playerTimer !== undefined) globalThis.window.clearTimeout(playerTimer)
  if (partnerTimer !== undefined) globalThis.window.clearTimeout(partnerTimer)
  if (completionTimer !== undefined) globalThis.window.clearTimeout(completionTimer)
})
</script>

<template>
  <section
    class="journey-scene movie-scene"
    aria-labelledby="movie-title"
  >
    <header class="journey-scene__header">
      <p>场景 3</p>
      <h1 id="movie-title">
        看电影
      </h1>
    </header>

    <div
      class="movie-cinema"
      aria-label="昏暗电影院里的座椅"
    >
      <img
        class="movie-cinema__seats"
        src="/assets/love-journey/movie/seats.png"
        alt=""
      >
      <span
        class="movie-cinema__screen-glow"
        aria-hidden="true"
      />
    </div>

    <div
      ref="trackElement"
      class="movie-hands-stage"
      :class="{ 'movie-hands-stage--completed': state.phase === 'completed' }"
      :data-hand-state="handsVisualState"
    >
      <img
        class="movie-hands__background"
        src="/assets/love-journey/movie/hands-bg.jpg"
        alt=""
        aria-hidden="true"
      >
      <span class="movie-hands__label movie-hands__label--player">你</span>
      <span class="movie-hands__label movie-hands__label--partner">她</span>

      <div
        class="movie-hands__open"
        :class="{ 'movie-hands__sprite--active': handsVisualState === 'open' }"
        :style="{
          '--player-hand-shift': `${playerHandShift}%`,
          '--partner-hand-shift': `${partnerHandShift}%`,
        }"
        aria-hidden="true"
      >
        <span
          class="movie-hands__half movie-hands__half--player"
        >
          <img
            src="/assets/love-journey/movie/hands-open.png"
            alt=""
          >
        </span>
        <span
          class="movie-hands__half movie-hands__half--partner"
        >
          <img
            src="/assets/love-journey/movie/hands-open.png"
            alt=""
          >
        </span>
      </div>

      <img
        class="movie-hands__sprite movie-hands__sprite--touching"
        :class="{ 'movie-hands__sprite--active': handsVisualState === 'touching' }"
        src="/assets/love-journey/movie/hands-touching.png"
        alt=""
        aria-hidden="true"
      >
      <img
        class="movie-hands__sprite movie-hands__sprite--touched"
        :class="{ 'movie-hands__sprite--active': handsVisualState === 'touched' }"
        src="/assets/love-journey/movie/hands-touched.png"
        alt=""
        aria-hidden="true"
      >

      <span
        class="movie-hands__drag-target"
        :class="{
          'movie-hands__drag-target--dragging': dragPointerId !== null,
          'movie-hands__drag-target--disabled': !active || state.phase === 'partner-moving' || state.phase === 'completed',
        }"
        role="slider"
        aria-label="拖动你的手向她靠近"
        aria-describedby="movie-drag-instructions"
        aria-valuemin="0"
        :aria-valuemax="MOVIE_ROUND_COUNT"
        :aria-valuenow="dragRoundProgress"
        :aria-valuetext="dragValueText"
        :aria-disabled="!active || state.phase === 'partner-moving' || state.phase === 'completed'"
        :tabindex="active && state.phase !== 'completed' ? 0 : -1"
        @pointerdown="startDrag"
        @pointermove="moveDrag"
        @pointerup="endDrag"
        @pointercancel="endDrag($event, true)"
        @lostpointercapture="endDrag($event, true)"
        @keydown.right.prevent="approachWithKeyboard"
        @keydown.space.prevent="approachWithKeyboard"
        @keydown.enter.prevent="approachWithKeyboard"
      >
        <span
          v-if="state.phase !== 'completed'"
          class="movie-hands__drag-cue"
          aria-hidden="true"
        >→</span>
      </span>
    </div>

    <p
      v-if="state.phase === 'completed'"
      class="movie-scene__memory"
    >
      那天的电影已经模糊，牵手的瞬间却很清楚。
    </p>

    <div
      class="movie-distance__rounds"
      aria-hidden="true"
    >
      <span
        v-for="round in MOVIE_ROUND_COUNT"
        :key="round"
        :class="{ filled: round <= state.round }"
      />
    </div>

    <p
      class="movie-scene__status"
      role="status"
      aria-live="polite"
    >
      {{ statusText }}
    </p>

    <span
      id="movie-drag-instructions"
      class="movie-scene__progress-sr"
    >
      按住左侧红色袖口的手向右拖动。每轮需要接近终点后松手；使用键盘时，按向右方向键完成一轮。
    </span>
    <span class="movie-scene__progress-sr">
      总靠近进度 {{ Math.round(completedProgress.combined) }}%
    </span>
  </section>
</template>

<style scoped>
.movie-scene {
  gap: 12px;
}

.movie-cinema {
  position: relative;
  min-height: 190px;
  flex: 1;
  overflow: hidden;
  border: 2px solid #091663;
  background: #090d37;
}

.movie-cinema__seats {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center 55%;
}

.movie-cinema__screen-glow {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgb(115 139 255 / 10%), transparent 35%),
    radial-gradient(circle at 50% 0, rgb(255 255 255 / 10%), transparent 48%);
  pointer-events: none;
}

.movie-hands-stage {
  position: relative;
  min-height: 175px;
  flex: 0 0 auto;
  aspect-ratio: 1.7;
  overflow: hidden;
  border: 2px solid #0b2185;
  background: #641f24;
  user-select: none;
}

.movie-hands__background {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
  transition: filter 280ms ease;
}

.movie-hands-stage--completed .movie-hands__background {
  filter: brightness(0.86) saturate(0.9);
}

.movie-hands__label {
  position: absolute;
  z-index: 3;
  top: 8px;
  min-width: 28px;
  padding: 3px 7px;
  border: 1px solid rgb(255 255 255 / 70%);
  border-radius: 999px;
  color: #fff;
  background: rgb(5 14 70 / 76%);
  font-size: 11px;
  font-weight: 800;
  text-align: center;
}

.movie-hands__label--player {
  left: 10px;
}

.movie-hands__label--partner {
  right: 10px;
}

.movie-hands__open {
  position: absolute;
  z-index: 2;
  inset: 0;
  opacity: 0;
  pointer-events: none;
  transition: opacity 220ms ease;
}

.movie-hands__open.movie-hands__sprite--active {
  opacity: 1;
}

.movie-hands__sprite {
  position: absolute;
  z-index: 2;
  top: 50%;
  left: 50%;
  width: min(58%, 220px);
  aspect-ratio: 2 / 3;
  opacity: 0;
  pointer-events: none;
  transform: translate(-50%, -48%) scale(0.96);
  transition: opacity 220ms ease, transform 280ms ease;
}

.movie-hands__sprite.movie-hands__sprite--active {
  opacity: 1;
  transform: translate(-50%, -50%) scale(1);
}

.movie-hands__half {
  position: absolute;
  top: -25%;
  bottom: -25%;
  width: 28%;
  overflow: hidden;
  transition: transform 180ms ease-out;
}

.movie-hands__half img {
  position: absolute;
  top: 0;
  width: 200%;
  max-width: none;
  height: 100%;
}

.movie-hands__half--player {
  left: -1%;
  transform: translateX(var(--player-hand-shift));
}

.movie-hands__half--player img {
  left: 0;
}

.movie-hands__half--partner {
  right: -1%;
  transform: translateX(var(--partner-hand-shift));
}

.movie-hands__half--partner img {
  right: 0;
}

.movie-hands__sprite {
  object-fit: contain;
}

.movie-hands__sprite--touching {
  width: min(60%, 228px);
}

.movie-hands__sprite--touched {
  width: min(56%, 212px);
}

.movie-hands__drag-target {
  position: absolute;
  z-index: 4;
  inset: 0 38% 0 0;
  display: block;
  cursor: grab;
  touch-action: none;
}

.movie-hands__drag-target--dragging {
  cursor: grabbing;
}

.movie-hands__drag-target--dragging .movie-hands__drag-cue {
  transform: translateX(6px);
}

.movie-hands__drag-target--disabled {
  cursor: default;
}

.movie-hands__drag-target:focus-visible {
  border-radius: 4px;
  outline: 3px solid #fff;
  outline-offset: -6px;
}

.movie-hands__drag-cue {
  position: absolute;
  right: 4px;
  bottom: 10px;
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border: 2px solid #102b9d;
  border-radius: 50%;
  color: #102b9d;
  background: rgb(255 255 255 / 92%);
  box-shadow: 0 3px 0 rgb(5 14 70 / 32%);
  font-size: 20px;
  font-weight: 900;
  animation: movie-drag-cue 1.4s ease-in-out infinite;
}

@keyframes movie-drag-cue {
  50% {
    transform: translateX(6px);
  }
}

.movie-scene__memory {
  margin: -4px 0 0;
  color: #0b2185;
  text-align: center;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.5;
}

.movie-distance__rounds {
  display: flex;
  justify-content: center;
  gap: 8px;
}

.movie-distance__rounds span {
  width: 9px;
  height: 9px;
  border: 1px solid #0b2185;
  border-radius: 50%;
  transition: background 180ms ease, transform 180ms ease;
}

.movie-distance__rounds span.filled {
  background: #f06b5a;
  transform: scale(1.12);
}

.movie-scene__status {
  min-height: 22px;
  margin: 0;
  color: #0b2185;
  text-align: center;
  font-size: 13px;
  font-weight: 700;
}

.movie-scene__progress-sr {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  white-space: nowrap;
}

@media (max-height: 650px) {
  .movie-scene {
    gap: 7px;
  }

  .movie-cinema {
    min-height: 135px;
  }

  .movie-hands-stage {
    min-height: 145px;
    aspect-ratio: auto;
  }

  .movie-hands__sprite {
    width: min(48%, 170px);
  }

  .movie-hands__sprite--touching {
    width: min(51%, 180px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .movie-hands__background,
  .movie-hands__open,
  .movie-hands__sprite,
  .movie-hands__half,
  .movie-distance__rounds span {
    transition: none;
  }

  .movie-hands__drag-cue {
    animation: none;
  }
}
</style>
