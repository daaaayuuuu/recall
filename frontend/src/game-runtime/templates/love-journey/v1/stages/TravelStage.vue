<script setup lang="ts">
/* global HTMLButtonElement, HTMLElement, PointerEvent */
import { computed, onBeforeUnmount, ref } from 'vue'

import type { GameStageResult } from '@/game-runtime/types'

import {
  closeTravelSuitcase,
  completeTravelSuitcase,
  createTravelState,
  packTravelItem,
} from './travelState'

interface ItemDefinition {
  id: string
  label: string
  image: string
  startX: number
  startY: number
  startWidth: number
  packedX: number
  packedY: number
  packedWidth: number
  rotation: number
  packedRotation: number
}

interface ItemDrag {
  itemId: string
  pointerId: number
  startX: number
  startY: number
  centerX: number
  centerY: number
  dx: number
  dy: number
  moved: boolean
}

interface CloseDrag {
  pointerId: number
  startY: number
  startedAt: number
  targetHeight: number
  dy: number
  moved: boolean
}

const itemDefinitions: ItemDefinition[] = [
  {
    id: 'camera',
    label: '相机',
    image: '/assets/love-journey/travel/camera.png',
    startX: 6.6,
    startY: 69.4,
    startWidth: 25,
    packedX: 23,
    packedY: 36,
    packedWidth: 13.5,
    rotation: 0,
    packedRotation: -4,
  },
  {
    id: 'hat',
    label: '帽子',
    image: '/assets/love-journey/travel/hat.png',
    startX: 26.9,
    startY: 68.4,
    startWidth: 29.5,
    packedX: 37,
    packedY: 36,
    packedWidth: 13.5,
    rotation: 0,
    packedRotation: 3,
  },
  {
    id: 'ticket',
    label: '车票',
    image: '/assets/love-journey/travel/tickets.png',
    startX: 49.5,
    startY: 69.5,
    startWidth: 25.6,
    packedX: 51,
    packedY: 39,
    packedWidth: 13.5,
    rotation: 0,
    packedRotation: -6,
  },
  {
    id: 'charger',
    label: '充电器',
    image: '/assets/love-journey/travel/charger.png',
    startX: 66.3,
    startY: 68.5,
    startWidth: 31.5,
    packedX: 65,
    packedY: 38,
    packedWidth: 13.5,
    rotation: 0,
    packedRotation: 5,
  },
]
const closeAnimationMilliseconds = 720

const props = defineProps<{ active: boolean }>()
const emit = defineEmits<{ complete: [result: GameStageResult] }>()

const state = ref(createTravelState())
const suitcaseDrop = ref<HTMLElement | null>(null)
const itemDrag = ref<ItemDrag | null>(null)
const closeDrag = ref<CloseDrag | null>(null)
const completionEmitted = ref(false)
const suppressCloseClick = ref(false)
let closeTimer: number | undefined
let clickSuppressionTimer: number | undefined
let suppressedItemClick: string | undefined

const closeOffset = computed(() => Math.min(0, closeDrag.value?.dy ?? 0))

function prefersReducedMotion() {
  return globalThis.window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

function isPacked(itemId: string) {
  return state.value.items.find((item) => item.id === itemId)?.packed ?? false
}

function itemStyle(item: ItemDefinition) {
  if (isPacked(item.id)) {
    return {
      left: `${item.packedX}%`,
      top: `${item.packedY}%`,
      width: `${item.packedWidth}%`,
      transform: `rotate(${item.packedRotation}deg)`,
    }
  }
  const drag = itemDrag.value?.itemId === item.id ? itemDrag.value : null
  return {
    left: `${item.startX}%`,
    top: `${item.startY}%`,
    width: `${item.startWidth}%`,
    transform: drag
      ? `translate(${drag.dx}px, ${drag.dy}px) rotate(${item.rotation}deg) scale(1.06)`
      : `rotate(${item.rotation}deg)`,
  }
}

function startItemDrag(item: ItemDefinition, event: PointerEvent) {
  if (!props.active || state.value.phase !== 'packing' || isPacked(item.id) || event.button !== 0) return
  const button = event.currentTarget as HTMLButtonElement
  const bounds = button.getBoundingClientRect()
  itemDrag.value = {
    itemId: item.id,
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    centerX: bounds.left + bounds.width / 2,
    centerY: bounds.top + bounds.height / 2,
    dx: 0,
    dy: 0,
    moved: false,
  }
  button.setPointerCapture(event.pointerId)
}

function moveItem(event: PointerEvent) {
  const drag = itemDrag.value
  if (!drag || drag.pointerId !== event.pointerId) return
  event.preventDefault()
  const dx = event.clientX - drag.startX
  const dy = event.clientY - drag.startY
  itemDrag.value = { ...drag, dx, dy, moved: drag.moved || Math.hypot(dx, dy) > 6 }
}

function stopItemDrag(event: PointerEvent, cancelled = false) {
  const drag = itemDrag.value
  if (!drag || drag.pointerId !== event.pointerId) return
  const button = event.currentTarget as HTMLButtonElement
  if (button.hasPointerCapture(event.pointerId)) button.releasePointerCapture(event.pointerId)
  itemDrag.value = null
  if (!drag.moved) return

  suppressItemClick(drag.itemId)
  if (cancelled || !suitcaseDrop.value) return
  const bounds = suitcaseDrop.value.getBoundingClientRect()
  const centerX = drag.centerX + drag.dx
  const centerY = drag.centerY + drag.dy
  if (centerX >= bounds.left && centerX <= bounds.right && centerY >= bounds.top && centerY <= bounds.bottom) {
    storeItem(drag.itemId)
  }
}

function suppressItemClick(itemId: string) {
  if (clickSuppressionTimer !== undefined) globalThis.window.clearTimeout(clickSuppressionTimer)
  suppressedItemClick = itemId
  clickSuppressionTimer = globalThis.window.setTimeout(() => {
    clickSuppressionTimer = undefined
    suppressedItemClick = undefined
  }, 0)
}

function autoPack(itemId: string) {
  if (suppressedItemClick !== itemId) storeItem(itemId)
}

function storeItem(itemId: string) {
  if (props.active) state.value = packTravelItem(state.value, itemId)
}

function startCloseDrag(event: PointerEvent) {
  if (!props.active || state.value.phase !== 'ready-to-close' || event.button !== 0) return
  const button = event.currentTarget as HTMLButtonElement
  closeDrag.value = {
    pointerId: event.pointerId,
    startY: event.clientY,
    startedAt: Date.now(),
    targetHeight: button.getBoundingClientRect().height,
    dy: 0,
    moved: false,
  }
  button.setPointerCapture(event.pointerId)
}

function moveCloseDrag(event: PointerEvent) {
  const drag = closeDrag.value
  if (!drag || drag.pointerId !== event.pointerId) return
  event.preventDefault()
  const dy = event.clientY - drag.startY
  closeDrag.value = { ...drag, dy, moved: drag.moved || Math.abs(dy) > 6 }
}

function stopCloseDrag(event: PointerEvent, cancelled = false) {
  const drag = closeDrag.value
  if (!drag || drag.pointerId !== event.pointerId) return
  const button = event.currentTarget as HTMLButtonElement
  if (button.hasPointerCapture(event.pointerId)) button.releasePointerCapture(event.pointerId)
  closeDrag.value = null
  if (!drag.moved) return

  suppressCloseClick.value = true
  globalThis.window.setTimeout(() => { suppressCloseClick.value = false }, 0)
  const upwardDistance = Math.max(0, -drag.dy)
  const upwardVelocity = upwardDistance / Math.max(1, Date.now() - drag.startedAt)
  if (!cancelled && (
    upwardDistance >= drag.targetHeight * 0.18
    || (upwardDistance >= 45 && upwardVelocity >= 0.4)
  )) animateClose()
}

function autoClose() {
  if (!suppressCloseClick.value) animateClose()
}

function animateClose() {
  if (!props.active || state.value.phase !== 'ready-to-close') return
  state.value = closeTravelSuitcase(state.value)
  closeTimer = globalThis.window.setTimeout(() => {
    closeTimer = undefined
    state.value = completeTravelSuitcase(state.value)
    finish()
  }, prefersReducedMotion() ? 1 : closeAnimationMilliseconds)
}

function finish() {
  if (completionEmitted.value || state.value.phase !== 'completed') return
  completionEmitted.value = true
  globalThis.navigator.vibrate?.([18, 30, 18])
  emit('complete', {
    stageId: 'travel',
    completedAt: Date.now(),
    actionCount: 5,
    metadata: { packedItems: 4, suitcaseClosed: true },
  })
}

onBeforeUnmount(() => {
  if (closeTimer !== undefined) globalThis.window.clearTimeout(closeTimer)
  if (clickSuppressionTimer !== undefined) globalThis.window.clearTimeout(clickSuppressionTimer)
  itemDrag.value = null
  closeDrag.value = null
})
</script>

<template>
  <section
    class="journey-scene travel-scene"
    aria-labelledby="travel-title"
    :data-phase="state.phase"
  >
    <header class="journey-scene__header">
      <p>场景 4</p>
      <h1 id="travel-title">
        旅行
      </h1>
    </header>

    <div class="travel-artboard">
      <img
        class="travel-artboard__background"
        src="/assets/love-journey/travel/background.png"
        alt=""
        aria-hidden="true"
      >
      <img
        class="travel-traveler travel-traveler--left"
        src="/assets/love-journey/travel/traveler-left.png"
        alt=""
        aria-hidden="true"
      >
      <img
        class="travel-traveler travel-traveler--right"
        src="/assets/love-journey/travel/traveler-right.png"
        alt=""
        aria-hidden="true"
      >

      <div
        class="travel-workspace"
        :class="{ 'travel-workspace--closing': state.phase === 'closing' || state.phase === 'completed' }"
      >
        <img
          class="travel-workspace__panel"
          src="/assets/love-journey/travel/packing-panel.png"
          alt=""
          aria-hidden="true"
        >

        <button
          class="travel-suitcase"
          type="button"
          :aria-label="state.phase === 'ready-to-close' ? '向上滑动或点击合上行李箱' : '行李箱'"
          :disabled="state.phase !== 'ready-to-close'"
          :style="{ transform: `translateY(${closeOffset * 0.08}px)` }"
          @click="autoClose"
          @keydown.enter.space.prevent="animateClose"
          @pointerdown="startCloseDrag"
          @pointermove="moveCloseDrag"
          @pointerup="stopCloseDrag"
          @pointercancel="stopCloseDrag($event, true)"
        >
          <img
            class="travel-suitcase__lid"
            src="/assets/love-journey/travel/suitcase-lid-open.png"
            alt=""
            aria-hidden="true"
          >
          <img
            class="travel-suitcase__body"
            src="/assets/love-journey/travel/suitcase-body-open.png"
            alt=""
            aria-hidden="true"
          >
          <img
            class="travel-suitcase__closed"
            src="/assets/love-journey/travel/suitcase-closed.png"
            alt=""
            aria-hidden="true"
          >
          <span
            ref="suitcaseDrop"
            class="travel-suitcase__drop-zone"
            aria-hidden="true"
          />
          <span
            v-if="state.phase === 'ready-to-close'"
            class="travel-suitcase__swipe"
            aria-hidden="true"
          ><b>↑</b> 上滑合箱</span>
        </button>

        <button
          v-for="item in itemDefinitions"
          :key="item.id"
          class="travel-item"
          :class="{
            'travel-item--packed': isPacked(item.id),
            'travel-item--dragging': itemDrag?.itemId === item.id,
          }"
          :style="itemStyle(item)"
          type="button"
          :data-travel-item="item.id"
          :aria-label="isPacked(item.id) ? `${item.label}已收好` : `拖动或点击收好${item.label}`"
          :disabled="isPacked(item.id) || state.phase !== 'packing'"
          @click="autoPack(item.id)"
          @keydown.enter.space.prevent="storeItem(item.id)"
          @pointerdown="startItemDrag(item, $event)"
          @pointermove="moveItem"
          @pointerup="stopItemDrag"
          @pointercancel="stopItemDrag($event, true)"
        >
          <img
            :src="item.image"
            alt=""
            aria-hidden="true"
            draggable="false"
          >
          <span class="travel-visually-hidden">{{ item.label }}</span>
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.travel-scene {
  --travel-blue: #142996;
  --travel-yellow: #ffd23f;

  position: relative;
  display: block;
  min-height: min(726px, calc(100dvh - 34px));
  padding: 0;
  overflow: hidden;
  color: var(--travel-blue);
  background: #edfaff;
  isolation: isolate;
}

.travel-scene > .journey-scene__header {
  position: absolute;
  z-index: 12;
  top: max(12px, env(safe-area-inset-top));
  right: 16px;
  left: 16px;
  min-height: 44px;
  padding: 7px 13px;
  border: 2px solid var(--travel-blue);
  border-radius: 999px;
  background: rgba(255, 253, 248, 0.94);
  box-shadow: 3px 3px 0 rgba(20, 41, 150, 0.16);
  backdrop-filter: blur(5px);
}

.travel-scene > .journey-scene__header h1 {
  padding: 0;
  color: var(--travel-blue);
  font-family: "Kaiti SC", STKaiti, KaiTi, serif;
  font-size: 27px;
}

.travel-scene > .journey-scene__header p {
  color: #5672ad;
  font-size: 11px;
  font-weight: 800;
}

.travel-artboard {
  position: absolute;
  z-index: 0;
  top: 0;
  left: 50%;
  width: auto;
  min-width: 100%;
  max-width: 120%;
  height: 100%;
  overflow: hidden;
  aspect-ratio: 9 / 16;
  transform: translateX(-50%);
}

.travel-artboard__background {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: fill;
}

.travel-traveler {
  position: absolute;
  z-index: 1;
  display: block;
  height: auto;
  filter: drop-shadow(0 8px 10px rgba(68, 126, 166, 0.08));
  pointer-events: none;
  user-select: none;
}

.travel-traveler--left {
  top: 9.5%;
  left: 3.5%;
  width: 54.3%;
}

.travel-traveler--right {
  top: 5.1%;
  right: 1.4%;
  width: 58.7%;
}

.travel-workspace {
  position: absolute;
  z-index: 5;
  top: 48.9%;
  left: 50%;
  width: 102.2%;
  aspect-ratio: 960 / 858;
  transform: translateX(-50%);
}

.travel-workspace__panel {
  position: absolute;
  z-index: 0;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
  user-select: none;
}

.travel-item {
  position: absolute;
  z-index: 5;
  min-width: 52px;
  padding: 0;
  border: 0;
  border-radius: 18px;
  color: var(--travel-blue);
  background: transparent;
  cursor: grab;
  touch-action: none;
  user-select: none;
  transition:
    left 320ms cubic-bezier(0.2, 0.8, 0.2, 1),
    top 320ms cubic-bezier(0.2, 0.8, 0.2, 1),
    width 320ms cubic-bezier(0.2, 0.8, 0.2, 1),
    opacity 240ms ease,
    transform 160ms ease-out;
}

.travel-item img {
  display: block;
  width: 100%;
  height: auto;
  filter: drop-shadow(0 5px 5px rgba(20, 41, 150, 0.13));
  pointer-events: none;
  transition: filter 160ms ease, transform 160ms ease;
}

.travel-item:active:not(:disabled) {
  cursor: grabbing;
}

.travel-item:not(:disabled):hover img,
.travel-item:not(:disabled):focus-visible img {
  filter: drop-shadow(0 7px 5px rgba(20, 41, 150, 0.2));
  transform: translateY(-3px);
}

.travel-item:focus-visible,
.travel-suitcase:focus-visible {
  outline: 3px solid var(--travel-yellow);
  outline-offset: 3px;
}

.travel-item--packed {
  z-index: 4;
  min-width: 44px;
  pointer-events: none;
}

.travel-item--dragging {
  z-index: 10;
  transition: none;
}

.travel-suitcase {
  position: absolute;
  z-index: 3;
  inset: 0;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  color: var(--travel-blue);
  background: transparent;
  cursor: ns-resize;
  touch-action: none;
  transition: transform 120ms ease-out;
}

.travel-suitcase:disabled {
  color: var(--travel-blue);
  cursor: default;
  opacity: 1;
}

.travel-suitcase__lid,
.travel-suitcase__body,
.travel-suitcase__closed {
  position: absolute;
  display: block;
  object-fit: fill;
  pointer-events: none;
  user-select: none;
}

.travel-suitcase__lid {
  z-index: 1;
  top: -3.1%;
  left: 12.5%;
  width: 74.4%;
  height: 43.3%;
  transform-origin: 50% 100%;
  transition: transform 360ms cubic-bezier(0.55, 0.05, 0.5, 0.9), opacity 180ms ease 180ms;
}

.travel-suitcase__body {
  z-index: 2;
  top: 26.7%;
  left: 10.9%;
  width: 78%;
  height: 46.5%;
  transition: transform 220ms ease 180ms, opacity 180ms ease 210ms;
}

.travel-suitcase__closed {
  z-index: 4;
  top: 27%;
  left: 10.9%;
  width: 78%;
  height: 43.3%;
  opacity: 0;
  transform: translateY(14px) scale(0.94);
  transition: transform 220ms ease 190ms, opacity 180ms ease 190ms;
}

.travel-suitcase__drop-zone {
  position: absolute;
  z-index: 3;
  top: 32%;
  left: 18%;
  width: 64%;
  height: 26%;
  border-radius: 18% 18% 28% 28%;
}

.travel-suitcase__swipe {
  position: absolute;
  z-index: 7;
  top: 1.5%;
  left: 50%;
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 6px 10px;
  border: 2px solid var(--travel-blue);
  border-radius: 999px;
  background: rgba(255, 253, 248, 0.96);
  box-shadow: 2px 3px 0 rgba(20, 41, 150, 0.16);
  font-size: 12px;
  font-weight: 900;
  white-space: nowrap;
  transform: translateX(-50%);
  animation: travel-swipe 900ms ease-in-out infinite alternate;
}

.travel-suitcase__swipe b {
  display: inline-grid;
  width: 20px;
  height: 20px;
  place-items: center;
  border-radius: 50%;
  color: #fff;
  background: var(--travel-blue);
  font-size: 14px;
}

.travel-workspace--closing .travel-suitcase__lid {
  opacity: 0;
  transform: translateY(65%) scaleY(0.9);
}

.travel-workspace--closing .travel-suitcase__body {
  opacity: 0;
  transform: translateY(6px) scale(0.98);
}

.travel-workspace--closing .travel-suitcase__closed {
  opacity: 1;
  transform: translateY(0) scale(1);
}

.travel-workspace--closing .travel-item--packed {
  opacity: 0;
}

.travel-visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@keyframes travel-swipe {
  to { transform: translate(-50%, -9px); }
}

@media (max-width: 360px) {
  .travel-item {
    min-width: 46px;
  }
}

@media (max-height: 700px) {
  .travel-scene > .journey-scene__header {
    top: 8px;
    min-height: 38px;
    padding-block: 4px;
  }

  .travel-scene > .journey-scene__header h1 {
    font-size: 23px;
  }

  .travel-traveler--left {
    top: 9.5%;
  }

  .travel-traveler--right {
    top: 5.1%;
  }

  .travel-workspace {
    top: 48.9%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .travel-suitcase__swipe,
  .travel-item,
  .travel-item img,
  .travel-suitcase__lid,
  .travel-suitcase__body,
  .travel-suitcase__closed {
    animation: none;
    transition-duration: 1ms;
  }
}
</style>
