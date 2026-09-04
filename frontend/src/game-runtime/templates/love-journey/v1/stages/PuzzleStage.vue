<script setup lang="ts">
/* global HTMLElement, PointerEvent, KeyboardEvent */
import { computed, ref } from 'vue'

import { createPuzzleState, placePuzzlePiece } from '@/game-runtime/engines/puzzle/puzzleState'
import type { GameStageResult } from '@/game-runtime/types'

interface PieceLayout {
  id: string
  x: number
  y: number
  rotation: number
  path: string
}

interface PiecePosition {
  x: number
  y: number
}

interface DragSession {
  pieceId: string
  pointerId: number
  offsetX: number
  offsetY: number
}

interface ScatterPosition {
  x: number
  y: number
  rotation: number
}

interface IllustratedBoundary {
  outwardFromLeft: boolean
  start: number
  end: number
}

const travelPieceAssets = [
  '/assets/love-journey/travel-puzzle-piece-left.png',
  '/assets/love-journey/travel-puzzle-piece-right.png',
] as const

const scatterPositions: Record<number, ScatterPosition[]> = {
  1: [{ x: 50, y: 74, rotation: -5 }],
  2: [
    { x: 28, y: 74, rotation: -7 },
    { x: 72, y: 76, rotation: 6 },
  ],
  3: [
    { x: 20, y: 68, rotation: -7 },
    { x: 50, y: 84, rotation: 5 },
    { x: 80, y: 68, rotation: -4 },
  ],
  4: [
    { x: 17, y: 64, rotation: -7 },
    { x: 40, y: 82, rotation: 5 },
    { x: 64, y: 63, rotation: -4 },
    { x: 84, y: 82, rotation: 7 },
  ],
  5: [
    { x: 14, y: 65, rotation: -7 },
    { x: 36, y: 57, rotation: 5 },
    { x: 63, y: 68, rotation: -4 },
    { x: 86, y: 59, rotation: 7 },
    { x: 49, y: 87, rotation: -6 },
  ],
}

const meetingBoundaries: IllustratedBoundary[] = [
  { outwardFromLeft: true, start: 22, end: 50 },
  { outwardFromLeft: false, start: 30, end: 58 },
  { outwardFromLeft: true, start: 16, end: 44 },
  { outwardFromLeft: false, start: 25, end: 53 },
]

const diningBoundaries: IllustratedBoundary[] = [
  { outwardFromLeft: true, start: 20, end: 48 },
  { outwardFromLeft: false, start: 28, end: 56 },
  { outwardFromLeft: true, start: 16, end: 44 },
]

function illustratedBoundary(index: number, pieceCount: number): IllustratedBoundary {
  if (pieceCount === 5) return meetingBoundaries[index]!
  if (pieceCount === 4) return diningBoundaries[index]!
  return { outwardFromLeft: true, start: 22, end: 50 }
}

function createIllustratedPiecePath(index: number, pieceCount: number) {
  let path = 'M 0 0 H 100'
  if (index === pieceCount - 1) {
    path += ' V 72'
  } else {
    const rightBoundary = illustratedBoundary(index, pieceCount)
    const controlX = rightBoundary.outwardFromLeft ? 124 : 76
    path += ` V ${rightBoundary.start} C ${controlX} ${rightBoundary.start}`
      + ` ${controlX} ${rightBoundary.end} 100 ${rightBoundary.end} V 72`
  }

  path += ' H 0'
  if (index === 0) return `${path} Z`
  const leftBoundary = illustratedBoundary(index - 1, pieceCount)
  const controlX = leftBoundary.outwardFromLeft ? 24 : -24
  return `${path} V ${leftBoundary.end} C ${controlX} ${leftBoundary.end}`
    + ` ${controlX} ${leftBoundary.start} 0 ${leftBoundary.start} V 0 Z`
}

function createPiecePath(index: number, pieceCount: number) {
  let path = 'M 0 0 H 100'
  if (index === pieceCount - 1) {
    path += ' V 150'
  } else if (index % 2 === 0) {
    path += ' V 42 C 76 42 76 82 100 82 V 150'
  } else {
    path += ' V 42 C 124 42 124 82 100 82 V 150'
  }

  path += ' H 0'
  if (index === 0) return `${path} Z`
  if ((index - 1) % 2 === 0) return `${path} V 82 C -24 82 -24 42 0 42 V 0 Z`
  return `${path} V 82 C 24 82 24 42 0 42 V 0 Z`
}

function createPieceLayouts(pieceCount: number): PieceLayout[] {
  const normalizedCount = Math.min(5, Math.max(1, Math.trunc(pieceCount)))
  const positions = scatterPositions[normalizedCount] ?? scatterPositions[5]!
  return positions.map((position, index) => ({
    id: `piece-${index + 1}`,
    ...position,
    path: createPiecePath(index, normalizedCount),
  }))
}

const props = defineProps<{
  active: boolean
  pieceCount: number
  experienceLabel: string
  experienceTitle: string
}>()
const emit = defineEmits<{ complete: [result: GameStageResult] }>()

const isTravelTransition = computed(() =>
  props.pieceCount === 2 && props.experienceTitle === '旅行',
)
const isMeetingTransition = computed(() =>
  props.pieceCount === 5 && props.experienceTitle === '初见',
)
const isDiningTransition = computed(() =>
  props.pieceCount === 4 && props.experienceTitle === '吃饭',
)
const isCinemaTransition = computed(() =>
  props.pieceCount === 3 && props.experienceTitle === '看电影',
)
const isIllustratedTransition = computed(() =>
  isTravelTransition.value
    || isMeetingTransition.value
    || isDiningTransition.value
    || isCinemaTransition.value,
)
const puzzleLabel = computed(() => {
  if (isTravelTransition.value) return '旅行拼图'
  if (isMeetingTransition.value) return '初见拼图'
  if (isDiningTransition.value) return '饭后拼图'
  if (isCinemaTransition.value) return '影院拼图'
  return '收尾拼图'
})
const puzzleInstruction = computed(() => {
  if (isTravelTransition.value) return '把两块旅程拼好，打开下一封回忆'
  if (isMeetingTransition.value) return '把五块初见记忆拼好，打开下一段故事'
  if (isDiningTransition.value) return '把四块饭后记忆拼好，一起走向下一场电影'
  if (isCinemaTransition.value) return '把三块影院记忆拼好，继续下一段旅程'
  return `完成这一幕前，将 ${props.pieceCount} 块拼图拖入正确位置`
})
const pieceLayouts = computed(() => {
  const layouts = createPieceLayouts(props.pieceCount)
  if (isTravelTransition.value) {
    return layouts.map((layout, index) => ({
      ...layout,
      x: index === 0 ? 28 : 72,
      y: index === 0 ? 44 : 46,
      rotation: index === 0 ? -5 : 5,
    }))
  }
  if (isCinemaTransition.value) {
    return layouts.map((layout, index) => ({
      ...layout,
      x: [25, 73, 48][index] ?? layout.x,
      y: [29, 55, 77][index] ?? layout.y,
      rotation: [-6, 5, -3][index] ?? layout.rotation,
      path: createIllustratedPiecePath(index, layouts.length),
    }))
  }
  if (isMeetingTransition.value) {
    return layouts.map((layout, index) => ({
      ...layout,
      x: [17, 49, 82, 20, 80][index] ?? layout.x,
      y: [13, 15, 12, 63, 64][index] ?? layout.y,
      rotation: [-6, 4, -3, 5, -4][index] ?? layout.rotation,
      path: createIllustratedPiecePath(index, layouts.length),
    }))
  }
  if (isDiningTransition.value) {
    return layouts.map((layout, index) => ({
      ...layout,
      x: [13, 38, 63, 88][index] ?? layout.x,
      y: 92.5,
      rotation: [-5, 3, -3, 5][index] ?? layout.rotation,
      path: createIllustratedPiecePath(index, layouts.length),
    }))
  }
  return layouts
})

const board = ref<HTMLElement | null>(null)
const target = ref<HTMLElement | null>(null)
const state = ref(createPuzzleState(pieceLayouts.value.map((piece) => piece.id)))
const positions = ref<Record<string, PiecePosition>>(
  Object.fromEntries(pieceLayouts.value.map((piece) => [piece.id, { x: piece.x, y: piece.y }])),
)
const dragSession = ref<DragSession | null>(null)
const completionEmitted = ref(false)

const unplacedPieces = computed(() =>
  pieceLayouts.value.filter((layout) =>
    state.value.pieces.some((piece) => piece.id === layout.id && piece.slotIndex === null),
  ),
)

function pieceInSlot(slotIndex: number) {
  const piece = state.value.pieces.find((candidate) => candidate.slotIndex === slotIndex)
  return piece ? pieceLayouts.value.find((layout) => layout.id === piece.id) : undefined
}

function travelPieceAsset(index: number) {
  return travelPieceAssets[index] ?? travelPieceAssets[0]
}

function pieceViewBox() {
  return isMeetingTransition.value || isDiningTransition.value || isCinemaTransition.value
    ? '0 0 100 72'
    : '0 0 100 150'
}

function positionStyle(piece: PieceLayout) {
  const position = positions.value[piece.id] ?? { x: piece.x, y: piece.y }
  return {
    left: `${position.x}%`,
    top: `${position.y}%`,
    transform: `translate(-50%, -50%) rotate(${piece.rotation}deg)`,
  }
}

function resetPosition(pieceId: string) {
  const layout = pieceLayouts.value.find((piece) => piece.id === pieceId)
  if (!layout) return
  positions.value = {
    ...positions.value,
    [pieceId]: { x: layout.x, y: layout.y },
  }
}

function emitCompletion() {
  if (state.value.status !== 'completed' || completionEmitted.value) return
  completionEmitted.value = true
  globalThis.navigator.vibrate?.([18, 30, 18])
  emit('complete', {
    stageId: 'puzzle',
    completedAt: Date.now(),
    actionCount: state.value.actionCount,
    metadata: { pieceCount: state.value.slotCount },
  })
}

function tryPlacement(pieceId: string, slotIndex: number) {
  const nextState = placePuzzlePiece(state.value, pieceId, slotIndex)
  if (nextState === state.value) {
    resetPosition(pieceId)
    return false
  }

  state.value = nextState
  globalThis.navigator.vibrate?.(12)
  emitCompletion()
  return true
}

function startDragging(event: PointerEvent, pieceId: string) {
  if (!props.active || state.value.status === 'completed' || !board.value) return
  const element = event.currentTarget as HTMLElement
  const pieceBounds = element.getBoundingClientRect()
  event.preventDefault()
  element.setPointerCapture(event.pointerId)
  dragSession.value = {
    pieceId,
    pointerId: event.pointerId,
    offsetX: event.clientX - (pieceBounds.left + pieceBounds.width / 2),
    offsetY: event.clientY - (pieceBounds.top + pieceBounds.height / 2),
  }
}

function continueDragging(event: PointerEvent) {
  const session = dragSession.value
  if (!session || session.pointerId !== event.pointerId || !board.value) return
  const bounds = board.value.getBoundingClientRect()
  if (bounds.width === 0 || bounds.height === 0) return
  event.preventDefault()
  const x = ((event.clientX - session.offsetX - bounds.left) / bounds.width) * 100
  const y = ((event.clientY - session.offsetY - bounds.top) / bounds.height) * 100
  positions.value = {
    ...positions.value,
    [session.pieceId]: {
      x: Math.min(92, Math.max(8, x)),
      y: Math.min(92, Math.max(8, y)),
    },
  }
}

function finishDragging(event: PointerEvent) {
  const session = dragSession.value
  if (!session || session.pointerId !== event.pointerId) return
  const element = event.currentTarget as HTMLElement
  if (element.hasPointerCapture(event.pointerId)) element.releasePointerCapture(event.pointerId)
  dragSession.value = null

  if (!target.value) {
    resetPosition(session.pieceId)
    return
  }

  const targetBounds = target.value.getBoundingClientRect()
  const centerX = event.clientX - session.offsetX
  const centerY = event.clientY - session.offsetY
  const insideTarget = centerX >= targetBounds.left
    && centerX <= targetBounds.right
    && centerY >= targetBounds.top
    && centerY <= targetBounds.bottom

  if (!insideTarget || targetBounds.width === 0) {
    resetPosition(session.pieceId)
    return
  }

  const slotIndex = Math.min(
    state.value.slotCount - 1,
    Math.floor(((centerX - targetBounds.left) / targetBounds.width) * state.value.slotCount),
  )
  tryPlacement(session.pieceId, slotIndex)
}

function cancelDragging(event: PointerEvent) {
  const session = dragSession.value
  if (!session || session.pointerId !== event.pointerId) return
  const element = event.currentTarget as HTMLElement
  if (element.hasPointerCapture(event.pointerId)) element.releasePointerCapture(event.pointerId)
  dragSession.value = null
  resetPosition(session.pieceId)
}

function handlePieceKeydown(event: KeyboardEvent, pieceId: string) {
  if (event.key !== 'Enter' && event.key !== ' ') return
  event.preventDefault()
  const piece = state.value.pieces.find((candidate) => candidate.id === pieceId)
  if (!piece) return
  tryPlacement(pieceId, piece.targetSlotIndex)
}
</script>

<template>
  <section
    class="journey-scene puzzle-scene"
    :class="{
      'puzzle-scene--travel': isTravelTransition,
      'puzzle-scene--meeting': isMeetingTransition,
      'puzzle-scene--dining': isDiningTransition,
      'puzzle-scene--cinema': isCinemaTransition,
    }"
    aria-labelledby="puzzle-title"
  >
    <header class="journey-scene__header">
      <p>{{ experienceLabel }} · {{ puzzleLabel }}</p>
      <h1 id="puzzle-title">
        {{ experienceTitle }}
      </h1>
    </header>

    <p
      id="puzzle-instruction"
      class="puzzle-scene__instruction"
    >
      {{ puzzleInstruction }}
    </p>

    <div
      ref="board"
      class="puzzle-board"
      :class="{
        'puzzle-board--complete': state.status === 'completed',
        'puzzle-board--travel': isTravelTransition,
        'puzzle-board--meeting': isMeetingTransition,
        'puzzle-board--dining': isDiningTransition,
        'puzzle-board--cinema': isCinemaTransition,
      }"
      :data-piece-count="state.slotCount"
    >
      <template v-if="isTravelTransition">
        <img
          class="puzzle-board__travel-art"
          src="/assets/love-journey/travel-puzzle-board.png"
          alt=""
          draggable="false"
        >
        <img
          class="puzzle-board__character puzzle-board__character--left"
          src="/assets/love-journey/travel-puzzle-character-left.png"
          alt=""
          draggable="false"
        >
        <img
          class="puzzle-board__character puzzle-board__character--right"
          src="/assets/love-journey/travel-puzzle-character-right.png"
          alt=""
          draggable="false"
        >
      </template>
      <img
        v-else-if="isMeetingTransition"
        class="puzzle-board__meeting-art"
        src="/assets/love-journey/first-meeting-puzzle-board.png"
        alt=""
        draggable="false"
      >
      <img
        v-else-if="isDiningTransition"
        class="puzzle-board__dining-art puzzle-board__dining-art--first"
        src="/assets/love-journey/dining-puzzle-scene-1.png"
        alt=""
        draggable="false"
      >
      <img
        v-if="isDiningTransition"
        class="puzzle-board__dining-art puzzle-board__dining-art--second"
        src="/assets/love-journey/dining-puzzle-scene-2.png"
        alt=""
        draggable="false"
      >
      <img
        v-else-if="isCinemaTransition"
        class="puzzle-board__cinema-art"
        src="/assets/love-journey/cinema-puzzle-board.png"
        alt=""
        draggable="false"
      >

      <div
        ref="target"
        class="puzzle-target"
        :class="{
          'puzzle-target--travel': isTravelTransition,
          'puzzle-target--meeting': isMeetingTransition,
          'puzzle-target--dining': isDiningTransition,
          'puzzle-target--cinema': isCinemaTransition,
        }"
        :style="{ gridTemplateColumns: `repeat(${state.slotCount}, minmax(0, 1fr))` }"
        :aria-label="`拼图目标区域，已放入 ${state.placedCount} 块，共 ${state.slotCount} 块`"
      >
        <div
          v-for="slotIndex in state.slotCount"
          :key="slotIndex"
          class="puzzle-target__slot"
        >
          <svg
            v-if="!isTravelTransition"
            class="puzzle-target__guide"
            :viewBox="pieceViewBox()"
            aria-hidden="true"
          >
            <path :d="pieceLayouts[slotIndex - 1]?.path" />
          </svg>
          <button
            v-if="pieceInSlot(slotIndex - 1)"
            class="puzzle-piece puzzle-piece--placed"
            type="button"
            tabindex="-1"
            :aria-label="`拼图块 ${slotIndex} 已放好`"
          >
            <img
              v-if="isTravelTransition"
              class="puzzle-piece__travel-art"
              :src="travelPieceAsset(slotIndex - 1)"
              alt=""
              draggable="false"
            >
            <svg
              v-else
              class="puzzle-piece__shape"
              :class="{
                'puzzle-piece__shape--illustrated': isMeetingTransition
                  || isDiningTransition
                  || isCinemaTransition,
              }"
              :viewBox="pieceViewBox()"
              aria-hidden="true"
            >
              <path :d="pieceLayouts[slotIndex - 1]?.path" />
            </svg>
          </button>
        </div>
      </div>

      <p
        v-if="state.status !== 'completed' && !isIllustratedTransition"
        class="puzzle-board__target-label"
        aria-hidden="true"
      >
        目标区域
      </p>

      <button
        v-for="piece in unplacedPieces"
        :key="piece.id"
        class="puzzle-piece puzzle-piece--loose"
        :class="{ 'puzzle-piece--dragging': dragSession?.pieceId === piece.id }"
        :style="positionStyle(piece)"
        type="button"
        :aria-label="`拼图块 ${pieceLayouts.indexOf(piece) + 1}，共 ${state.slotCount} 块；拖动到目标区域，也可按回车键自动放置`"
        aria-describedby="puzzle-instruction"
        @pointerdown="startDragging($event, piece.id)"
        @pointermove="continueDragging"
        @pointerup="finishDragging"
        @pointercancel="cancelDragging"
        @keydown="handlePieceKeydown($event, piece.id)"
        @contextmenu.prevent
      >
        <img
          v-if="isTravelTransition"
          class="puzzle-piece__travel-art"
          :src="travelPieceAsset(pieceLayouts.indexOf(piece))"
          alt=""
          draggable="false"
        >
        <svg
          v-else
          class="puzzle-piece__shape"
          :class="{
            'puzzle-piece__shape--illustrated': isMeetingTransition
              || isDiningTransition
              || isCinemaTransition,
          }"
          :viewBox="pieceViewBox()"
          aria-hidden="true"
        >
          <path :d="piece.path" />
        </svg>
      </button>

      <div
        v-if="isIllustratedTransition"
        class="puzzle-scene__status puzzle-scene__status--illustrated"
        role="status"
        aria-live="polite"
      >
        <span>{{ state.status === 'completed'
          ? `${experienceTitle}记忆已拼好`
          : '轻触或拖动蓝色拼图' }}</span>
        <strong>{{ state.placedCount }} / {{ state.slotCount }}</strong>
      </div>
    </div>

    <div
      v-if="!isIllustratedTransition"
      class="puzzle-scene__status"
      role="status"
      aria-live="polite"
    >
      <span>{{ state.status === 'completed' ? `${experienceTitle}收尾完成` : '收尾拼图进度' }}</span>
      <strong>{{ state.placedCount }} / {{ state.slotCount }}</strong>
    </div>
  </section>
</template>

<style scoped>
.puzzle-scene__instruction {
  min-height: 54px;
  margin: 0;
  padding-top: 18px;
  text-align: center;
  font-size: 14px;
}

.puzzle-board {
  position: relative;
  min-height: 360px;
  flex: 1;
  overflow: hidden;
  border-top: 1px dashed #000;
}

.puzzle-target {
  position: absolute;
  top: 8%;
  left: 50%;
  display: grid;
  width: min(86vw, 320px);
  aspect-ratio: 10 / 3;
  transform: translateX(-50%);
  border: 3px dashed #000;
  background: #fff;
}

.puzzle-target__slot {
  position: relative;
  min-width: 0;
  overflow: visible;
}

.puzzle-target__guide {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

.puzzle-target__guide path {
  fill: transparent;
  stroke: #888;
  stroke-dasharray: 7 6;
  stroke-linejoin: round;
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}

.puzzle-board__target-label {
  position: absolute;
  top: calc(8% + min(27.6vw, 96px) + 9px);
  left: 50%;
  margin: 0;
  transform: translateX(-50%);
  color: #444;
  font-size: 11px;
  letter-spacing: 0.14em;
  white-space: nowrap;
}

.puzzle-piece {
  display: block;
  height: 100%;
  padding: 0;
  border: 0;
  border-radius: 0;
  color: #000;
  background: transparent;
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.puzzle-piece__shape {
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
}

.puzzle-piece__shape path {
  fill: #fff;
  stroke: #000;
  stroke-linejoin: round;
  stroke-width: 3;
  vector-effect: non-scaling-stroke;
}

.puzzle-piece--loose {
  position: absolute;
  z-index: 2;
  width: min(17.2vw, 64px);
  height: min(25.8vw, 96px);
  filter: drop-shadow(3px 3px 0 #000);
  transition: left 180ms ease, top 180ms ease, transform 180ms ease, filter 180ms ease;
}

.puzzle-board[data-piece-count="4"] .puzzle-piece--loose {
  width: min(19vw, 70px);
  height: min(28.5vw, 105px);
}

.puzzle-board[data-piece-count="3"] .puzzle-piece--loose {
  width: min(21vw, 78px);
  height: min(31.5vw, 117px);
}

.puzzle-board[data-piece-count="2"] .puzzle-piece--loose {
  width: min(24vw, 88px);
  height: min(36vw, 132px);
}

.puzzle-piece--dragging {
  z-index: 5;
  cursor: grabbing;
  filter: drop-shadow(6px 6px 0 #000);
  transition: none;
}

.puzzle-piece--loose:focus-visible {
  outline: 3px double #000;
  outline-offset: 4px;
}

.puzzle-piece--placed {
  position: absolute;
  inset: 0;
  z-index: 1;
  width: 100%;
  cursor: default;
  pointer-events: none;
}

.puzzle-board--complete .puzzle-target {
  border-color: transparent;
  box-shadow: 6px 6px 0 #000;
}

.puzzle-board--complete .puzzle-target__guide {
  opacity: 0;
}

.puzzle-scene__status {
  display: flex;
  min-height: 46px;
  align-items: center;
  justify-content: space-between;
  border-top: 2px solid #000;
  font-size: 13px;
}

.puzzle-scene__status strong {
  font-size: 16px;
}

.puzzle-scene--travel,
.puzzle-scene--meeting,
.puzzle-scene--dining,
.puzzle-scene--cinema {
  overflow: hidden;
  background: #faf8f5;
}

.puzzle-scene--travel .puzzle-scene__instruction,
.puzzle-scene--meeting .puzzle-scene__instruction,
.puzzle-scene--dining .puzzle-scene__instruction,
.puzzle-scene--cinema .puzzle-scene__instruction {
  min-height: 46px;
  padding-top: 13px;
  color: #2c6598;
  font-family: "STKaiti", "KaiTi", ui-serif, serif;
  font-size: clamp(14px, 4vw, 16px);
  font-weight: 700;
  letter-spacing: 0.05em;
}

.puzzle-board--travel {
  width: min(100%, 360px, calc((100dvh - 145px) * 0.5625));
  min-height: 0;
  aspect-ratio: 9 / 16;
  margin: 0 auto;
  flex: none;
  overflow: hidden;
  border: 0;
  background: #faf8f5;
  isolation: isolate;
}

.puzzle-board__travel-art {
  position: absolute;
  z-index: 0;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.puzzle-board--meeting {
  width: min(100%, 360px);
  min-height: 0;
  aspect-ratio: 9 / 19;
  margin: 0 auto;
  flex: none;
  overflow: hidden;
  border: 0;
  background: #eaf7fc;
  isolation: isolate;
}

.puzzle-board__meeting-art {
  position: absolute;
  z-index: 0;
  top: 0;
  left: 0;
  display: block;
  width: 100%;
  height: auto;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.puzzle-board--dining {
  width: min(100%, 360px);
  min-height: 0;
  aspect-ratio: 36 / 151;
  margin: 0 auto;
  flex: none;
  overflow: hidden;
  border: 0;
  background: #eaf5ff;
  isolation: isolate;
}

.puzzle-board__dining-art {
  position: absolute;
  z-index: 0;
  left: 0;
  display: block;
  width: 100%;
  height: auto;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.puzzle-board__dining-art--first {
  top: 0;
}

.puzzle-board__dining-art--second {
  top: 42.36%;
}

.puzzle-board--cinema {
  width: min(100%, 360px);
  min-height: 0;
  aspect-ratio: 845 / 1862;
  margin: 0 auto;
  flex: none;
  overflow: hidden;
  border: 0;
  background: #faf8f5;
  isolation: isolate;
}

.puzzle-scene--meeting,
.puzzle-scene--dining,
.puzzle-scene--cinema {
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior-y: contain;
  scrollbar-width: thin;
  scrollbar-color: rgb(44 101 152 / 35%) transparent;
  -webkit-overflow-scrolling: touch;
}

.puzzle-scene--meeting::-webkit-scrollbar,
.puzzle-scene--dining::-webkit-scrollbar,
.puzzle-scene--cinema::-webkit-scrollbar {
  width: 4px;
}

.puzzle-scene--meeting::-webkit-scrollbar-thumb,
.puzzle-scene--dining::-webkit-scrollbar-thumb,
.puzzle-scene--cinema::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgb(44 101 152 / 35%);
}

.puzzle-board__cinema-art {
  position: absolute;
  z-index: 0;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.puzzle-board__character {
  position: absolute;
  z-index: 1;
  display: block;
  height: auto;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
  filter: drop-shadow(0 5px 3px rgb(18 61 128 / 10%));
  transition: transform 420ms ease;
}

.puzzle-board__character--left {
  top: 20%;
  left: 13%;
  width: 32%;
  transform: rotate(-1deg);
}

.puzzle-board__character--right {
  top: 19%;
  right: 11%;
  width: 36%;
  transform: rotate(1deg);
}

.puzzle-target--travel {
  z-index: 3;
  top: 69.2%;
  left: 50%;
  width: 68%;
  height: 12.4%;
  aspect-ratio: auto;
  border: 0;
  background: transparent;
}

.puzzle-target--cinema {
  z-index: 3;
  top: 87.2%;
  left: 50%;
  width: 76%;
  height: 6.6%;
  aspect-ratio: auto;
  border: 0;
  background: rgb(255 255 255 / 24%);
}

.puzzle-target--meeting {
  z-index: 3;
  top: 86.2%;
  left: 50%;
  width: 86%;
  height: 5.7%;
  aspect-ratio: auto;
  border: 0;
  background: rgb(255 255 255 / 48%);
}

.puzzle-target--dining {
  z-index: 3;
  top: 85.6%;
  left: 50%;
  width: 86%;
  height: 3.7%;
  aspect-ratio: auto;
  border: 0;
  background: rgb(255 255 255 / 48%);
}

.puzzle-target--meeting::before,
.puzzle-target--dining::before,
.puzzle-target--cinema::before {
  position: absolute;
  z-index: -1;
  inset: -10px -13px;
  border: 2px dashed #58b4ea;
  border-radius: 13px;
  content: "";
}

.puzzle-board--meeting .puzzle-target__slot,
.puzzle-board--dining .puzzle-target__slot,
.puzzle-board--cinema .puzzle-target__slot {
  overflow: visible;
}

.puzzle-board--meeting .puzzle-target__guide path,
.puzzle-board--dining .puzzle-target__guide path,
.puzzle-board--cinema .puzzle-target__guide path {
  stroke: #58b4ea;
  stroke-dasharray: 6 5;
  stroke-width: 2.2;
}

.puzzle-board--cinema[data-piece-count="3"] .puzzle-piece--loose {
  z-index: 4;
  width: 25.33%;
  height: 6.6%;
  aspect-ratio: auto;
  filter: drop-shadow(0 7px 5px rgb(11 61 130 / 22%));
}

.puzzle-board--meeting[data-piece-count="5"] .puzzle-piece--loose {
  z-index: 4;
  width: 17.2%;
  height: 5.7%;
  aspect-ratio: auto;
  filter: drop-shadow(0 7px 5px rgb(11 61 130 / 22%));
}

.puzzle-board--dining[data-piece-count="4"] .puzzle-piece--loose {
  z-index: 4;
  width: 21.5%;
  height: 3.7%;
  aspect-ratio: auto;
  filter: drop-shadow(0 7px 5px rgb(11 61 130 / 22%));
}

.puzzle-piece__shape--illustrated path {
  fill: #8fd5f4;
  stroke: #183ba5;
  stroke-width: 2.4;
}

.puzzle-board--meeting .puzzle-piece--dragging,
.puzzle-board--dining .puzzle-piece--dragging,
.puzzle-board--cinema .puzzle-piece--dragging {
  z-index: 6;
  filter: drop-shadow(0 11px 8px rgb(11 61 130 / 28%));
  transform: translate(-50%, -50%) scale(1.035) !important;
}

.puzzle-board--meeting .puzzle-piece--loose:focus-visible,
.puzzle-board--dining .puzzle-piece--loose:focus-visible,
.puzzle-board--cinema .puzzle-piece--loose:focus-visible {
  border-radius: 7px;
  outline: 3px solid #ef735f;
  outline-offset: 5px;
}

.puzzle-board--meeting.puzzle-board--complete .puzzle-target,
.puzzle-board--dining.puzzle-board--complete .puzzle-target,
.puzzle-board--cinema.puzzle-board--complete .puzzle-target {
  border-color: transparent;
  box-shadow: none;
  animation: travel-puzzle-complete 560ms ease both;
}

.puzzle-board--meeting .puzzle-scene__status--illustrated,
.puzzle-board--cinema .puzzle-scene__status--illustrated {
  bottom: 0.7%;
}

.puzzle-board--dining .puzzle-scene__status--illustrated {
  bottom: 0.5%;
}

.puzzle-board--travel .puzzle-target__slot {
  overflow: visible;
}

.puzzle-board--travel[data-piece-count="2"] .puzzle-piece--loose {
  z-index: 4;
  width: 34%;
  height: 12.4%;
  aspect-ratio: auto;
  border-radius: 4px;
  filter: drop-shadow(0 7px 5px rgb(11 61 130 / 20%));
}

.puzzle-piece__travel-art {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: fill;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.puzzle-board--travel .puzzle-piece--dragging {
  z-index: 6;
  filter: drop-shadow(0 11px 8px rgb(11 61 130 / 28%));
  transform: translate(-50%, -50%) scale(1.035) !important;
}

.puzzle-board--travel .puzzle-piece--loose:focus-visible {
  border-radius: 7px;
  outline: 3px solid #ef735f;
  outline-offset: 5px;
}

.puzzle-board--travel.puzzle-board--complete .puzzle-target {
  border-color: transparent;
  box-shadow: none;
  animation: travel-puzzle-complete 560ms ease both;
}

.puzzle-board--travel.puzzle-board--complete .puzzle-board__character--left {
  transform: translateX(3%) rotate(0deg);
}

.puzzle-board--travel.puzzle-board--complete .puzzle-board__character--right {
  transform: translateX(-3%) rotate(0deg);
}

.puzzle-scene__status--illustrated {
  position: absolute;
  z-index: 7;
  right: 8%;
  bottom: 3.6%;
  left: 8%;
  min-height: 38px;
  padding: 0 15px;
  border: 2px solid #174f8c;
  border-radius: 999px;
  color: #174f8c;
  background: rgb(255 250 237 / 93%);
  box-shadow: 0 4px 0 rgb(23 79 140 / 14%);
  font-size: 11px;
  letter-spacing: 0.04em;
}

.puzzle-scene__status--illustrated strong {
  color: #ef735f;
  font-size: 15px;
}

@keyframes travel-puzzle-complete {
  50% { filter: drop-shadow(0 0 10px rgb(91 178 226 / 55%)); }
}

@media (max-height: 650px) {
  .puzzle-scene__instruction {
    min-height: 40px;
    padding-top: 10px;
  }

  .puzzle-board {
    min-height: 280px;
  }

  .puzzle-target {
    top: 5%;
    width: min(76vw, 280px);
  }

  .puzzle-board__target-label {
    top: calc(5% + min(22.8vw, 84px) + 6px);
  }

  .puzzle-piece--loose {
    width: min(15.2vw, 56px);
    height: min(22.8vw, 84px);
  }

  .puzzle-scene--travel .puzzle-scene__instruction {
    min-height: 36px;
    padding-top: 8px;
    font-size: 13px;
  }

  .puzzle-board--travel {
    width: min(100%, 300px, calc((100dvh - 125px) * 0.5625));
  }

  .puzzle-target--travel {
    top: 69.2%;
    width: 68%;
    height: 12.4%;
  }

  .puzzle-board--travel[data-piece-count="2"] .puzzle-piece--loose {
    width: 34%;
    height: 12.4%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .puzzle-piece--loose {
    transition: none;
  }

  .puzzle-board__character,
  .puzzle-board--travel.puzzle-board--complete .puzzle-target {
    animation: none;
    transition: none;
  }
}
</style>
