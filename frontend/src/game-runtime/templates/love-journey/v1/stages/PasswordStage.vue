<script setup lang="ts">
/* global HTMLButtonElement, HTMLElement, KeyboardEvent, PointerEvent, WheelEvent */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import type { PlayableAsset } from '@/api/gameplay'
import type { GameStageResult } from '@/game-runtime/types'

import {
  createEnvelopeRevealState,
  nextEnvelopeItem,
  revealNextEnvelopeItem,
} from './envelopeState'
import {
  clearPasswordError,
  createPasswordState,
  PASSWORD_LENGTH,
  verifyPassword,
} from './passwordState'

interface PullDrag {
  pointerId: number
  startX: number
  startY: number
  startedAt: number
  cardHeight: number
  dx: number
  dy: number
  moved: boolean
}

interface WheelDrag {
  pointerId: number
  index: number
  startY: number
  handled: boolean
}

interface WheelScrollState {
  accumulatedDelta: number
  locked: boolean
  resetTimer?: number
}

const hintDelayMilliseconds = 30_000
const props = defineProps<{
  active: boolean
  password: string
  passwordHint?: string
  photos: PlayableAsset[]
  loveLetter: string
}>()
const emit = defineEmits<{ complete: [result: GameStageResult] }>()

const state = ref(createPasswordState())
const envelopeState = ref(createEnvelopeRevealState())
const revealPhase = ref<'lockbox' | 'envelope'>('lockbox')
const envelopeOpening = ref(false)
const checking = ref(false)
const errorRevision = ref(0)
const hintVisible = ref(false)
const pullDrag = ref<PullDrag | null>(null)
const wheelDrag = ref<WheelDrag | null>(null)
const itemFlying = ref(false)
const completionEmitted = ref(false)
const suppressPullClick = ref(false)
const photoLoadStates = ref<Record<string, 'loaded' | 'error'>>({})
let checkTimer: number | undefined
let resetTimer: number | undefined
let hintTimer: number | undefined
let pullTimer: number | undefined
let envelopeTransitionTimer: number | undefined

const passwordLength = computed(() => PASSWORD_LENGTH)
const wheelDigits = ref<number[]>(Array.from({ length: passwordLength.value }, () => 0))
const wheelScrollStates: WheelScrollState[] = Array.from(
  { length: passwordLength.value },
  () => ({ accumulatedDelta: 0, locked: false }),
)
const keyboardDigitIndex = ref(0)
const revealedPhotos = computed(() => props.photos.slice(0, envelopeState.value.revealedPhotoCount))
const nextItem = computed(() => nextEnvelopeItem(envelopeState.value, props.photos.length))
const currentPhoto = computed(() => props.photos[envelopeState.value.revealedPhotoCount])
const currentPhotoLoadState = computed<'loading' | 'loaded' | 'error'>(() => {
  if (nextItem.value !== 'photo' || !currentPhoto.value) return 'loaded'
  return photoLoadStates.value[currentPhoto.value.key] ?? 'loading'
})
const currentPhotoLoading = computed(() => currentPhotoLoadState.value === 'loading')
const pullOffset = computed(() => ({
  x: (pullDrag.value?.dx ?? 0) * 0.2,
  y: Math.min(18, pullDrag.value?.dy ?? 0),
}))
const revealStatus = computed(() => {
  if (envelopeState.value.letterRevealed) return '情书已经打开'
  if (nextItem.value === 'letter') return '照片已全部取出，再上滑一次打开情书'
  if (currentPhotoLoadState.value === 'loading') return '照片正在加载，请稍候…'
  if (currentPhotoLoadState.value === 'error') return '照片加载失败，可以继续取出下一份回忆'
  return `已取出 ${envelopeState.value.revealedPhotoCount} / ${props.photos.length} 张照片`
})

const revealInstruction = computed(() => {
  if (envelopeState.value.letterRevealed) return '这封信，终于交到你手里。'
  if (currentPhotoLoadState.value === 'loading') return '正在准备这张照片，请稍候…'
  return '向上滑动，从信封里取出回忆。'
})

function setCurrentPhotoLoadState(loadState: 'loaded' | 'error') {
  const photo = currentPhoto.value
  if (!photo) return
  photoLoadStates.value = { ...photoLoadStates.value, [photo.key]: loadState }
}

function prefersReducedMotion() {
  return globalThis.window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

function clearHintTimer() {
  if (hintTimer === undefined) return
  globalThis.window.clearTimeout(hintTimer)
  hintTimer = undefined
}

function startHintTimer() {
  clearHintTimer()
  hintVisible.value = false
  if (!props.active || !props.passwordHint?.trim() || state.value.status === 'completed') return
  hintTimer = globalThis.window.setTimeout(() => {
    hintTimer = undefined
    if (props.active && state.value.status !== 'completed') hintVisible.value = true
  }, hintDelayMilliseconds)
}

function wheelDigit(index: number, offset = 0) {
  return (wheelDigits.value[index]! + offset + 10) % 10
}

function setWheelDigit(index: number, digit: number) {
  if (!props.active || checking.value || state.value.status !== 'playing') return
  wheelDigits.value = wheelDigits.value.map((value, wheelIndex) => wheelIndex === index ? digit : value)
  state.value = { ...state.value, input: wheelDigits.value.join('') }
}

function rotateWheel(index: number, direction: 1 | -1) {
  setWheelDigit(index, wheelDigit(index, direction))
  keyboardDigitIndex.value = (index + 1) % passwordLength.value
}

function resetWheelScrollAfterGesture(index: number) {
  const scrollState = wheelScrollStates[index]
  if (!scrollState) return
  if (scrollState.resetTimer !== undefined) globalThis.window.clearTimeout(scrollState.resetTimer)
  scrollState.resetTimer = globalThis.window.setTimeout(() => {
    scrollState.accumulatedDelta = 0
    scrollState.locked = false
    scrollState.resetTimer = undefined
  }, 180)
}

function handleWheelScroll(event: WheelEvent, index: number) {
  if (event.deltaY === 0) return
  const scrollState = wheelScrollStates[index]
  if (!scrollState) return
  resetWheelScrollAfterGesture(index)
  if (scrollState.locked) return
  scrollState.accumulatedDelta += event.deltaY
  if (Math.abs(scrollState.accumulatedDelta) < 36) return
  rotateWheel(index, scrollState.accumulatedDelta > 0 ? 1 : -1)
  scrollState.accumulatedDelta = 0
  scrollState.locked = true
}

function startWheelDrag(event: PointerEvent, index: number) {
  if (!props.active || checking.value || state.value.status !== 'playing' || event.button !== 0) return
  const element = event.currentTarget as HTMLElement
  wheelDrag.value = { pointerId: event.pointerId, index, startY: event.clientY, handled: false }
  element.setPointerCapture(event.pointerId)
}

function moveWheelDrag(event: PointerEvent) {
  const drag = wheelDrag.value
  if (!drag || drag.pointerId !== event.pointerId || drag.handled) return
  event.preventDefault()
  const distance = event.clientY - drag.startY
  if (Math.abs(distance) < 38) return
  rotateWheel(drag.index, distance < 0 ? 1 : -1)
  wheelDrag.value = { ...drag, handled: true }
}

function stopWheelDrag(event: PointerEvent) {
  const drag = wheelDrag.value
  if (!drag || drag.pointerId !== event.pointerId) return
  const element = event.currentTarget as HTMLElement
  if (element.hasPointerCapture(event.pointerId)) element.releasePointerCapture(event.pointerId)
  wheelDrag.value = null
}

function submitWheelPassword() {
  if (!props.active || checking.value || state.value.status !== 'playing') return
  state.value = { ...state.value, input: wheelDigits.value.join('') }
  checking.value = true
  checkTimer = globalThis.window.setTimeout(checkPassword, 120)
}

function checkPassword() {
  checkTimer = undefined
  state.value = verifyPassword(state.value, props.password)
  if (state.value.status === 'completed') {
    checking.value = false
    clearHintTimer()
    hintVisible.value = false
    globalThis.navigator.vibrate?.([18, 35, 18])
    return
  }

  errorRevision.value += 1
  globalThis.navigator.vibrate?.([45, 25, 45])
  resetTimer = globalThis.window.setTimeout(() => {
    resetTimer = undefined
    state.value = clearPasswordError(state.value)
    checking.value = false
  }, 550)
}

function openEnvelopeFromLockbox() {
  if (!props.active || envelopeOpening.value || revealPhase.value !== 'lockbox') return
  envelopeOpening.value = true
  globalThis.navigator.vibrate?.(18)
  envelopeTransitionTimer = globalThis.window.setTimeout(() => {
    envelopeTransitionTimer = undefined
    revealPhase.value = 'envelope'
    envelopeOpening.value = false
  }, prefersReducedMotion() ? 1 : 360)
}

function handleKeydown(event: KeyboardEvent) {
  if (!props.active || state.value.status === 'completed') return
  if (/^\d$/.test(event.key)) {
    const index = keyboardDigitIndex.value
    setWheelDigit(index, Number(event.key))
    keyboardDigitIndex.value = (index + 1) % passwordLength.value
    if (index === passwordLength.value - 1) submitWheelPassword()
  }
  if (event.key === 'Backspace' || event.key === 'Delete') {
    const index = (keyboardDigitIndex.value - 1 + passwordLength.value) % passwordLength.value
    setWheelDigit(index, 0)
    keyboardDigitIndex.value = index
  }
  if (event.key === 'Enter') submitWheelPassword()
}

function startPull(event: PointerEvent) {
  if (!props.active || itemFlying.value || currentPhotoLoading.value || nextItem.value === 'completed' || event.button !== 0) return
  const button = event.currentTarget as HTMLButtonElement
  pullDrag.value = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    startedAt: Date.now(),
    cardHeight: button.getBoundingClientRect().height,
    dx: 0,
    dy: 0,
    moved: false,
  }
  button.setPointerCapture(event.pointerId)
}

function movePull(event: PointerEvent) {
  const drag = pullDrag.value
  if (!drag || drag.pointerId !== event.pointerId || itemFlying.value) return
  event.preventDefault()
  const dx = event.clientX - drag.startX
  const dy = event.clientY - drag.startY
  pullDrag.value = { ...drag, dx, dy, moved: drag.moved || Math.hypot(dx, dy) > 6 }
}

function stopPull(event: PointerEvent, cancelled = false) {
  const drag = pullDrag.value
  if (!drag || drag.pointerId !== event.pointerId || itemFlying.value) return
  const button = event.currentTarget as HTMLButtonElement
  if (button.hasPointerCapture(event.pointerId)) button.releasePointerCapture(event.pointerId)
  if (!drag.moved) {
    pullDrag.value = null
    return
  }

  suppressPullClick.value = true
  globalThis.window.setTimeout(() => { suppressPullClick.value = false }, 0)
  const upwardDistance = Math.max(0, -drag.dy)
  const upwardVelocity = upwardDistance / Math.max(1, Date.now() - drag.startedAt)
  const valid = !cancelled && (
    upwardDistance >= drag.cardHeight * 0.25
    || (upwardDistance >= 55 && upwardVelocity >= 0.4)
  )
  if (valid) animateReveal()
  else pullDrag.value = null
}

function autoReveal() {
  if (!suppressPullClick.value) animateReveal()
}

function animateReveal() {
  if (!props.active || itemFlying.value || currentPhotoLoading.value || nextItem.value === 'completed') return
  itemFlying.value = true
  pullTimer = globalThis.window.setTimeout(() => {
    pullTimer = undefined
    envelopeState.value = revealNextEnvelopeItem(envelopeState.value, props.photos.length)
    itemFlying.value = false
    pullDrag.value = null
    if (envelopeState.value.letterRevealed) finish()
  }, prefersReducedMotion() ? 1 : 380)
}

function finish() {
  if (completionEmitted.value) return
  completionEmitted.value = true
  globalThis.navigator.vibrate?.([20, 35, 20])
  emit('complete', {
    stageId: 'password',
    completedAt: Date.now(),
    actionCount: 8 + props.photos.length,
    metadata: { revealedPhotos: props.photos.length, letterRevealed: true },
  })
}

onMounted(() => {
  globalThis.window.addEventListener('keydown', handleKeydown)
  startHintTimer()
})

watch(() => props.active, (active, wasActive) => {
  if (active && !wasActive) startHintTimer()
  if (!active) {
    clearHintTimer()
    hintVisible.value = false
  }
})

onBeforeUnmount(() => {
  globalThis.window.removeEventListener('keydown', handleKeydown)
  if (checkTimer !== undefined) globalThis.window.clearTimeout(checkTimer)
  if (resetTimer !== undefined) globalThis.window.clearTimeout(resetTimer)
  if (pullTimer !== undefined) globalThis.window.clearTimeout(pullTimer)
  if (envelopeTransitionTimer !== undefined) globalThis.window.clearTimeout(envelopeTransitionTimer)
  for (const scrollState of wheelScrollStates) {
    if (scrollState.resetTimer !== undefined) globalThis.window.clearTimeout(scrollState.resetTimer)
  }
  clearHintTimer()
})
</script>

<template>
  <section
    class="journey-scene password-scene"
    aria-labelledby="password-title"
  >
    <header class="journey-scene__header">
      <p>场景 5</p>
      <h1 id="password-title">
        {{ state.status === 'completed' ? '今天' : '密码' }}
      </h1>
    </header>

    <template v-if="state.status !== 'completed'">
      <div
        :key="errorRevision"
        class="password-lockbox password-lockbox--closed"
        :class="{ 'password-lockbox--error': state.status === 'error' }"
      >
        <img
          class="password-lockbox__closed-art"
          src="/assets/love-journey/gift-lockbox-closed.png"
          alt="蓝色水彩旅行密码箱"
          draggable="false"
        >
        <div
          class="password-scene__intro password-lockbox__intro"
        >
          <small>A LITTLE SECRET FOR YOU</small>
          <p>转动密码，打开礼物</p>
          <span>滑动数字轮或轻触上下数字，再按下解锁</span>
        </div>

        <div class="password-lock">
          <div
            class="password-wheels"
            :style="{ '--wheel-count': passwordLength }"
            :aria-label="`${passwordLength} 位转轮密码`"
          >
            <div
              v-for="(_, index) in wheelDigits"
              :key="index"
              class="password-wheel"
            >
              <button
                class="password-wheel__step password-wheel__step--up"
                type="button"
                :aria-label="`第 ${index + 1} 位增加`"
                :disabled="checking"
                @click="rotateWheel(index, 1)"
              >
                <span aria-hidden="true">⌃</span>
              </button>
              <div
                class="password-wheel__window"
                role="spinbutton"
                :aria-label="`密码第 ${index + 1} 位`"
                aria-valuemin="0"
                aria-valuemax="9"
                :aria-valuenow="wheelDigit(index)"
                tabindex="0"
                @wheel.prevent="handleWheelScroll($event, index)"
                @pointerdown="startWheelDrag($event, index)"
                @pointermove="moveWheelDrag"
                @pointerup="stopWheelDrag"
                @pointercancel="stopWheelDrag"
                @keydown.up.prevent="rotateWheel(index, -1)"
                @keydown.down.prevent="rotateWheel(index, 1)"
              >
                <span>{{ wheelDigit(index, -1) }}</span>
                <b>{{ wheelDigit(index) }}</b>
                <span>{{ wheelDigit(index, 1) }}</span>
              </div>
              <button
                class="password-wheel__step password-wheel__step--down"
                type="button"
                :aria-label="`第 ${index + 1} 位减少`"
                :disabled="checking"
                @click="rotateWheel(index, -1)"
              >
                <span aria-hidden="true">⌄</span>
              </button>
            </div>
          </div>
        </div>

        <div class="password-lockbox__actions">
          <p
            class="password-scene__message"
            role="status"
            aria-live="polite"
          >
            {{ state.status === 'error' ? '密码不正确，请再试一次' : '把数字转到记忆中的那一天' }}
          </p>

          <button
            class="password-unlock"
            type="button"
            :disabled="checking"
            @click="submitWheelPassword"
          >
            <span aria-hidden="true">✦</span>
            {{ checking ? '正在解锁…' : '打开密码箱' }}
            <span aria-hidden="true">✦</span>
          </button>

          <p
            v-if="hintVisible && passwordHint?.trim()"
            class="password-scene__hint"
            role="status"
            aria-live="polite"
          >
            提示：{{ passwordHint }}
          </p>
        </div>
      </div>
    </template>

    <div
      v-else-if="revealPhase === 'lockbox'"
      class="password-lockbox-reveal"
    >
      <div class="password-scene__intro password-scene__intro--unlocked">
        <p>密码正确，盒子打开了</p>
        <span>信封就在里面，点击它继续打开这份礼物</span>
      </div>

      <div
        class="password-lockbox password-lockbox--open"
        :class="{ 'password-lockbox--leaving': envelopeOpening }"
      >
        <div
          class="password-lockbox__gift-aura"
          aria-hidden="true"
        />
        <img
          class="password-lockbox__open-art"
          src="/assets/love-journey/gift-lockbox-open.png"
          alt="已经打开的蓝色水彩礼物盒"
          draggable="false"
        >
        <div
          class="password-lockbox__sparkles"
          aria-hidden="true"
        >
          <i
            v-for="sparkle in 6"
            :key="sparkle"
          />
        </div>
        <button
          class="password-lockbox__envelope"
          type="button"
          aria-label="打开盒中的信封"
          :disabled="envelopeOpening"
          @click="openEnvelopeFromLockbox"
        >
          <img
            src="/assets/love-journey/gift-envelope.png"
            alt=""
            draggable="false"
          >
          <b aria-hidden="true">♥</b>
          <em>{{ envelopeOpening ? '正在展开…' : '点击信封' }}</em>
        </button>
      </div>

      <p class="password-lockbox-reveal__instruction">
        {{ envelopeOpening ? '密码盒正在退场，信封即将展开…' : '轻触信封，取出里面珍藏的回忆' }}
      </p>
    </div>

    <div
      v-else
      class="envelope-scene"
    >
      <div
        v-if="revealedPhotos.length"
        class="envelope-photos"
        aria-label="从信封取出的照片"
      >
        <figure
          v-for="(photo, index) in revealedPhotos"
          :key="photo.key"
        >
          <img
            :src="photo.url"
            :alt="`照片 ${index + 1}`"
            draggable="false"
          >
          <figcaption>照片 {{ index + 1 }}</figcaption>
        </figure>
      </div>

      <article
        v-if="envelopeState.letterRevealed"
        class="envelope-letter"
        aria-label="情书"
      >
        <span>给你的一封信</span>
        <p>{{ loveLetter || '谢谢你陪我走完这段旅程，未来也请继续多多指教。' }}</p>
      </article>

      <div
        class="envelope-wrap"
        :class="{ 'envelope-wrap--letter-open': envelopeState.letterRevealed }"
      >
        <button
          v-if="nextItem !== 'completed'"
          class="envelope-pull-card"
          :class="[
            `envelope-pull-card--${nextItem}`,
            { 'envelope-pull-card--flying': itemFlying },
          ]"
          :style="{
            transform: itemFlying
              ? 'translateY(-260px) rotate(-3deg)'
              : `translate(${pullOffset.x}px, ${pullOffset.y}px) rotate(${nextItem === 'photo' ? -2 : 1}deg)`,
          }"
          type="button"
          :aria-label="nextItem === 'photo'
            ? currentPhotoLoading
              ? `照片 ${envelopeState.revealedPhotoCount + 1} 正在加载`
              : `向上滑动或点击取出照片 ${envelopeState.revealedPhotoCount + 1}`
            : '向上滑动或点击取出情书'"
          :disabled="itemFlying || currentPhotoLoading"
          @click="autoReveal"
          @keydown.enter.space.prevent="animateReveal"
          @pointerdown="startPull"
          @pointermove="movePull"
          @pointerup="stopPull"
          @pointercancel="stopPull($event, true)"
        >
          <img
            v-if="nextItem === 'photo' && currentPhoto"
            :class="{ 'envelope-pull-card__photo--ready': currentPhotoLoadState === 'loaded' }"
            :src="currentPhoto.url"
            :alt="`待取出的照片 ${envelopeState.revealedPhotoCount + 1}`"
            draggable="false"
            @load="setCurrentPhotoLoadState('loaded')"
            @error="setCurrentPhotoLoadState('error')"
          >
          <span
            v-if="nextItem === 'photo' && currentPhotoLoading"
            class="envelope-photo-loading"
            role="status"
          >
            <i aria-hidden="true" />
            正在加载照片…
          </span>
          <span
            v-else-if="nextItem === 'photo' && currentPhotoLoadState === 'error'"
            class="envelope-photo-error"
          >照片暂时无法显示</span>
          <span v-else-if="nextItem !== 'photo'">情书<br>LOVE LETTER</span>
        </button>

        <div
          class="envelope-drawing"
          aria-hidden="true"
        >
          <img
            class="envelope-drawing__art"
            src="/assets/love-journey/gift-envelope-open.png"
            alt=""
            draggable="false"
          >
          <b>TO YOU</b>
        </div>
      </div>

      <div class="envelope-instruction">
        <span
          v-if="!envelopeState.letterRevealed && !currentPhotoLoading"
          aria-hidden="true"
        >↑</span>
        <p>{{ revealInstruction }}</p>
      </div>
      <p
        class="envelope-status"
        role="status"
        aria-live="polite"
      >
        {{ revealStatus }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.password-scene__intro {
  display: grid;
  justify-items: center;
  gap: 7px;
  padding: 24px 10px 15px;
  text-align: center;
}

.password-scene__intro p,
.password-scene__intro span,
.password-scene__message,
.password-scene__hint,
.envelope-instruction p,
.envelope-status,
.envelope-letter p {
  margin: 0;
}

.password-scene__intro p {
  color: #123f78;
  font-family: ui-serif, "Songti SC", Georgia, serif;
  font-size: 17px;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.password-scene__intro span {
  color: #62738d;
  font-size: 12px;
  letter-spacing: 0.05em;
}

.password-lockbox {
  position: relative;
  width: min(342px, 100%);
  margin: 0 auto;
  box-sizing: border-box;
  filter: drop-shadow(0 9px 0 rgb(27 83 137 / 13%));
}

.password-lockbox--error {
  animation: password-shake 320ms ease;
}

.password-lockbox__lid {
  position: relative;
  z-index: 2;
  height: 112px;
  border: 4px solid #15569a;
  border-radius: 33px 33px 14px 14px;
  background:
    radial-gradient(circle at 24% 30%, rgb(255 255 255 / 76%) 0 2px, transparent 3px),
    linear-gradient(152deg, #d9f3ff 0%, #9edcf4 46%, #bcecff 100%);
  box-shadow:
    inset 0 0 0 7px #eaf9ff,
    inset 0 0 0 10px #438dc2,
    0 4px 0 #0d477f;
  transform: perspective(320px) rotateX(4deg);
  transform-origin: 50% 100%;
}

.password-lockbox__lid::after {
  position: absolute;
  inset: 17px;
  border: 2px solid rgb(255 255 255 / 78%);
  border-radius: 20px 20px 8px 8px;
  content: "";
  box-shadow: inset 0 0 0 2px rgb(23 94 153 / 22%);
}

.password-lockbox__corner {
  position: absolute;
  z-index: 1;
  top: 12px;
  color: #1767a9;
  font-family: Georgia, serif;
  font-size: 35px;
  line-height: 1;
  text-shadow: 9px 9px 0 #ff9a80;
}

.password-lockbox__corner--top-left { left: 21px; transform: rotate(-21deg); }
.password-lockbox__corner--top-right { right: 21px; transform: scaleX(-1) rotate(-21deg); }

.password-lockbox__lid-label {
  position: absolute;
  z-index: 2;
  right: 58px;
  bottom: 22px;
  left: 58px;
  display: flex;
  align-items: center;
  color: #236394;
  gap: 8px;
}

.password-lockbox__lid-label i {
  height: 1px;
  flex: 1;
  background: #fff;
  box-shadow: 0 1px 0 #2775ad;
}

.password-lockbox__lid-label b {
  font-family: Georgia, serif;
  font-size: 8px;
  letter-spacing: 0.16em;
}

.password-lockbox__body {
  position: relative;
  min-height: 184px;
  margin: -3px 7px 0;
  padding: 31px 21px 21px;
  border: 4px solid #145596;
  border-radius: 8px 8px 20px 20px;
  background:
    radial-gradient(circle at 78% 22%, rgb(255 255 255 / 48%) 0 2px, transparent 3px),
    linear-gradient(135deg, #a8def2 0%, #c9f0fb 48%, #92d3ee 100%);
  box-shadow:
    inset 0 0 0 7px #eafaff,
    inset 0 0 0 10px #65acd2;
}

.password-lockbox__floral-band {
  position: absolute;
  z-index: 3;
  top: -2px;
  right: -4px;
  left: -4px;
  display: flex;
  height: 30px;
  align-items: center;
  justify-content: center;
  border: 3px solid #15569a;
  color: #1767a9;
  background: #9cdaef;
  gap: 8px;
  font-size: 15px;
}

.password-lockbox__floral-band i {
  width: 48px;
  height: 8px;
  background: repeating-linear-gradient(135deg, #1c6aaa 0 5px, transparent 5px 10px);
  opacity: 0.72;
}

.password-lockbox__floral-band b { color: #f47f68; }

.password-lock {
  position: relative;
  z-index: 4;
  display: flex;
  width: min(244px, 86%);
  min-height: 112px;
  margin: 23px auto 0;
  align-items: center;
  justify-content: center;
  border: 4px solid #174f8c;
  border-radius: 45px;
  background: #fffaf0;
  box-shadow:
    inset 0 0 0 5px #dfeef1,
    0 5px 0 rgb(16 69 123 / 24%);
}

.password-lock__cap {
  position: absolute;
  top: 36px;
  width: 25px;
  height: 40px;
  border: 4px solid #174f8c;
  background: repeating-linear-gradient(0deg, #e9f5f4 0 5px, #b8d4d8 5px 7px);
}

.password-lock__cap--left { left: -24px; border-radius: 12px 4px 4px 12px; }
.password-lock__cap--right { right: -24px; border-radius: 4px 12px 12px 4px; }

.password-wheels {
  position: relative;
  z-index: 1;
  display: grid;
  width: calc(100% - 34px);
  grid-template-columns: repeat(var(--wheel-count), minmax(0, 1fr));
  gap: 3px;
}

.password-wheels::after {
  position: absolute;
  z-index: 2;
  top: 50%;
  right: -8px;
  left: -8px;
  height: 2px;
  background: #164f8c;
  content: "";
  pointer-events: none;
}

.password-wheel {
  position: relative;
  display: grid;
  min-width: 0;
  justify-items: stretch;
}

.password-wheel__window {
  position: relative;
  z-index: 3;
  display: grid;
  height: 82px;
  overflow: hidden;
  border: 3px solid #174f8c;
  background: linear-gradient(#d9e7e7 0%, #fffdf4 26%, #fffdf4 74%, #c7dcde 100%);
  grid-template-rows: 1fr 1.35fr 1fr;
  outline: none;
  touch-action: none;
  user-select: none;
}

.password-wheel__window::after {
  position: absolute;
  top: 30%;
  right: 0;
  bottom: 30%;
  left: 0;
  border-top: 1px solid #6f97ad;
  border-bottom: 1px solid #6f97ad;
  background: rgb(255 255 255 / 36%);
  content: "";
  pointer-events: none;
}

.password-wheel__window:focus-visible {
  outline: 3px solid #ff876f;
  outline-offset: 2px;
}

.password-wheel__window span,
.password-wheel__window b {
  display: grid;
  place-items: center;
  color: #174f8c;
  font-family: Georgia, "Times New Roman", serif;
  line-height: 1;
}

.password-wheel__window span {
  font-size: 14px;
  opacity: 0.68;
}

.password-wheel__window b {
  position: relative;
  z-index: 1;
  font-size: 29px;
}

.password-wheel__step {
  position: absolute;
  z-index: 5;
  left: 50%;
  display: grid;
  width: 30px;
  height: 25px;
  padding: 0;
  place-items: center;
  border: 0;
  color: #174f8c;
  background: transparent;
  cursor: pointer;
  transform: translateX(-50%);
}

.password-wheel__step span { font-size: 22px; line-height: 1; }
.password-wheel__step--up { top: -25px; }
.password-wheel__step--down { bottom: -25px; }
.password-wheel__step:hover:not(:disabled) { color: #f16f5c; }
.password-wheel__step:active:not(:disabled) { transform: translateX(-50%) scale(0.88); }
.password-wheel__step:focus-visible { outline: 2px solid #f16f5c; outline-offset: -2px; }
.password-wheel__step:disabled { opacity: 0.38; }

.password-lockbox__body-corners {
  position: absolute;
  right: 20px;
  bottom: 12px;
  left: 20px;
  display: flex;
  justify-content: space-between;
  color: #1767a9;
  font-size: 21px;
  pointer-events: none;
}

.password-lockbox__body-corners span:last-child { transform: scaleX(-1); }

.password-lockbox-reveal {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  align-items: center;
}

.password-scene__intro--unlocked {
  padding-top: 16px;
}

.password-lockbox--open {
  transition: opacity 320ms ease, transform 360ms ease, filter 360ms ease;
  animation: lockbox-open-arrive 460ms cubic-bezier(0.2, 0.8, 0.2, 1);
}

.password-lockbox--open .password-lockbox__lid {
  height: 178px;
  border-radius: 27px 27px 10px 10px;
  background:
    radial-gradient(circle at 74% 70%, rgb(255 255 255 / 64%) 0 2px, transparent 3px),
    linear-gradient(152deg, #d9f3ff 0%, #a8dff5 46%, #c9effb 100%);
  transform: perspective(500px) rotateX(-5deg);
  animation: lockbox-lid-open 460ms cubic-bezier(0.2, 0.8, 0.2, 1);
}

.password-lockbox--open .password-lockbox__lid::after {
  inset: 18px;
  border-radius: 18px 18px 7px 7px;
}

.password-lockbox--open .password-lockbox__corner {
  top: 22px;
  font-size: 42px;
}

.password-lockbox--open .password-lockbox__lid-label {
  bottom: 28px;
}

.password-lockbox--open .password-lockbox__body {
  min-height: 214px;
  padding-top: 39px;
  background: linear-gradient(135deg, #9fd9ef 0%, #d5f2f8 52%, #8fcde9 100%);
}

.password-lockbox__lining {
  position: absolute;
  z-index: 1;
  inset: 41px 20px 19px;
  border: 3px solid #2b689c;
  border-radius: 14px;
  background:
    repeating-linear-gradient(90deg, rgb(42 99 145 / 6%) 0 2px, transparent 2px 5px),
    #fffdf4;
  box-shadow: inset 0 0 0 7px #dceff1;
}

.password-lockbox__envelope {
  position: relative;
  z-index: 5;
  display: block;
  width: min(226px, 78%);
  height: 134px;
  margin: 15px auto 0;
  overflow: hidden;
  border: 4px solid #174f8c;
  border-radius: 7px;
  color: #174f8c;
  background: linear-gradient(145deg, #ffad96 0%, #fa856f 100%);
  box-shadow: 0 7px 0 rgb(20 72 128 / 20%);
  cursor: pointer;
  transition: transform 180ms ease, box-shadow 180ms ease;
}

.password-lockbox__envelope::before,
.password-lockbox__envelope::after {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 78%;
  border-top: 3px solid #174f8c;
  content: "";
  pointer-events: none;
}

.password-lockbox__envelope::before {
  background: #ff9a82;
  clip-path: polygon(0 100%, 0 0, 52% 70%, 100% 0, 100% 100%);
}

.password-lockbox__envelope::after {
  background: linear-gradient(155deg, transparent 49%, #174f8c 50% 51%, transparent 52%);
  opacity: 0.55;
}

.password-lockbox__envelope-flap {
  position: absolute;
  z-index: 3;
  top: -2px;
  right: -2px;
  left: -2px;
  height: 86px;
  border-bottom: 4px solid #174f8c;
  border-radius: 0 0 45% 45%;
  background: #ffab93;
  clip-path: polygon(0 0, 100% 0, 55% 92%, 50% 96%, 45% 92%);
}

.password-lockbox__envelope-fold {
  position: absolute;
  z-index: 2;
  inset: 0;
  background:
    linear-gradient(35deg, transparent 49%, #174f8c 50% 51%, transparent 52%),
    linear-gradient(-35deg, transparent 49%, #174f8c 50% 51%, transparent 52%);
  opacity: 0.72;
}

.password-lockbox__envelope b {
  position: absolute;
  z-index: 6;
  top: 58px;
  left: calc(50% - 17px);
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 3px solid #174f8c;
  border-radius: 50%;
  color: #fff8dd;
  background: #f06e62;
  font-size: 16px;
}

.password-lockbox__envelope em {
  position: absolute;
  z-index: 7;
  right: 0;
  bottom: 9px;
  left: 0;
  color: #123f78;
  font-size: 11px;
  font-style: normal;
  font-weight: 800;
  letter-spacing: 0.1em;
}

.password-lockbox__envelope:hover:not(:disabled) {
  box-shadow: 0 10px 0 rgb(20 72 128 / 20%);
  transform: translateY(-3px) rotate(-0.6deg);
}

.password-lockbox__envelope:active:not(:disabled) {
  box-shadow: 0 3px 0 rgb(20 72 128 / 20%);
  transform: translateY(4px) scale(0.99);
}

.password-lockbox__envelope:focus-visible {
  outline: 4px solid #fff8dd;
  outline-offset: 4px;
}

.password-lockbox__envelope:disabled {
  cursor: wait;
}

.password-lockbox--leaving {
  opacity: 0;
  filter: blur(3px);
  transform: translateY(-22px) scale(1.07);
}

.password-lockbox--leaving .password-lockbox__envelope {
  transform: translateY(-70px) scale(1.34);
}

.password-lockbox-reveal__instruction {
  min-height: 20px;
  margin: 15px 0 0;
  color: #456a8e;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-align: center;
}

.password-unlock {
  display: flex;
  width: min(238px, 100%);
  min-height: 44px;
  margin: 14px auto 0;
  align-items: center;
  justify-content: center;
  border: 3px solid #174f8c;
  border-radius: 999px;
  color: #174f8c;
  background: #fff9e9;
  box-shadow: 0 4px 0 #174f8c;
  gap: 12px;
  font-family: ui-serif, "Songti SC", Georgia, serif;
  font-size: 14px;
  font-weight: 800;
  letter-spacing: 0.12em;
  cursor: pointer;
}

.password-unlock span { color: #f16f5c; }
.password-unlock:hover:not(:disabled) { background: #fff1cf; }
.password-unlock:active:not(:disabled) { box-shadow: 0 1px 0 #174f8c; transform: translateY(3px); }
.password-unlock:focus-visible { outline: 3px solid #f16f5c; outline-offset: 3px; }
.password-unlock:disabled { cursor: wait; opacity: 0.7; }

.password-lockbox--error .password-lock {
  box-shadow: inset 0 0 0 5px #ffe6df, 0 5px 0 rgb(16 69 123 / 24%);
}

.password-lockbox--error .password-wheel__window {
  border-color: #db5f55;
}

.password-scene__message {
  min-height: 18px;
  margin-top: 9px;
  color: #c94f47;
  text-align: center;
  font-size: 12px;
  font-weight: 700;
}

.password-scene__hint {
  width: min(290px, 100%);
  min-height: 20px;
  margin: 7px auto 0;
  padding: 8px 10px;
  border: 2px solid #174f8c;
  border-radius: 8px;
  color: #174f8c;
  background: #fff9e9;
  font-size: 12px;
  line-height: 1.5;
  text-align: center;
}

.envelope-pull-card:focus-visible {
  outline: 3px double #174f8c;
  outline-offset: 2px;
}

.envelope-scene {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  align-items: center;
  padding: 12px 0 0;
  animation: envelope-arrive 420ms ease-out;
}

.envelope-photos {
  display: grid;
  width: 100%;
  min-height: 112px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 7px;
}

.envelope-photos figure {
  min-width: 0;
  margin: 0;
  padding: 5px;
  border: 2px solid #174f8c;
  background: #fffaf0;
  box-shadow: 3px 3px 0 rgb(240 110 98 / 28%);
  transform: rotate(-1deg);
}

.envelope-photos figure:nth-child(even) {
  transform: rotate(2deg);
}

.envelope-photos img {
  display: block;
  width: 100%;
  height: 78px;
  border: 1px solid #174f8c;
  object-fit: cover;
}

.envelope-photos figcaption {
  padding-top: 3px;
  text-align: center;
  font-size: 9px;
  font-weight: 800;
}

.envelope-letter {
  z-index: 4;
  width: min(330px, 100%);
  max-height: 280px;
  margin: 0 auto -42px;
  padding: 22px 24px 58px;
  overflow: auto;
  border: 2px solid #174f8c;
  background:
    repeating-linear-gradient(#fffaf0 0 25px, #f6c9bd 26px 27px);
  box-shadow: 7px 7px 0 #174f8c;
  box-sizing: border-box;
  animation: letter-open 420ms ease-out;
}

.envelope-letter > span {
  display: block;
  margin-bottom: 13px;
  border-bottom: 2px solid #174f8c;
  color: #174f8c;
  font-size: 12px;
  font-weight: 900;
  letter-spacing: 0.12em;
}

.envelope-letter p {
  white-space: pre-wrap;
  font-family: ui-serif, Georgia, serif;
  font-size: 14px;
  line-height: 1.9;
}

.envelope-wrap {
  position: relative;
  width: min(350px, 100%);
  height: 300px;
  margin-top: clamp(42px, 10vh, 84px);
}

.envelope-wrap--letter-open {
  height: 164px;
}

.envelope-drawing {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 258px;
  pointer-events: none;
}

.envelope-drawing__open-flap,
.envelope-drawing__front {
  position: absolute;
  display: block;
  border: 3px solid #174f8c;
  overflow: hidden;
  fill: #ffae98;
  stroke: #174f8c;
  stroke-linecap: square;
  stroke-linejoin: miter;
  stroke-width: 3;
  vector-effect: non-scaling-stroke;
}

.envelope-drawing__open-flap {
  z-index: 0;
  right: 0;
  bottom: 137px;
  left: 0;
  width: 100%;
  height: 116px;
  border: 0;
  background: transparent;
}

.envelope-drawing__front {
  z-index: 3;
  right: 0;
  bottom: 0;
  left: 0;
  width: 100%;
  height: 150px;
  border: 0;
  background: transparent;
}

.envelope-drawing__front path {
  fill: none;
}

.envelope-drawing b {
  position: absolute;
  z-index: 4;
  right: 19px;
  bottom: 15px;
  padding: 2px 5px;
  border: 1px solid #174f8c;
  color: #174f8c;
  background: #fff8dd;
  font-size: 9px;
  letter-spacing: 0.14em;
}

.envelope-pull-card {
  position: absolute;
  z-index: 2;
  bottom: 40px;
  left: 23%;
  display: grid;
  width: 54%;
  height: 158px;
  padding: 7px;
  place-items: center;
  border: 3px solid #174f8c;
  border-radius: 0;
  color: #174f8c;
  background: #fffaf0;
  cursor: ns-resize;
  touch-action: none;
  transform-origin: 50% 100%;
  transition: transform 120ms ease-out, opacity 260ms ease;
}

.envelope-pull-card img {
  position: absolute;
  inset: 7px;
  width: calc(100% - 14px);
  height: calc(100% - 14px);
  border: 1px solid #174f8c;
  box-sizing: border-box;
  object-fit: cover;
  opacity: 0;
  pointer-events: none;
}

.envelope-pull-card img.envelope-pull-card__photo--ready {
  opacity: 1;
}

.envelope-pull-card:disabled {
  cursor: wait;
}

.envelope-photo-loading,
.envelope-photo-error {
  position: relative;
  z-index: 1;
  display: grid;
  justify-items: center;
  gap: 10px;
  padding: 12px;
  color: #174f8c;
  background: #fffaf0;
  font-family: inherit !important;
  font-size: 12px !important;
  letter-spacing: 0.04em !important;
  line-height: 1.45 !important;
  text-align: center;
}

.envelope-photo-loading i {
  width: 22px;
  height: 22px;
  border: 2px solid #a9bfd0;
  border-top-color: #174f8c;
  border-radius: 50%;
  animation: envelope-photo-loading 800ms linear infinite;
}

.envelope-pull-card span {
  font-family: ui-serif, Georgia, serif;
  font-size: 15px;
  font-weight: 900;
  letter-spacing: 0.1em;
  line-height: 1.8;
}

.envelope-pull-card--letter {
  left: 19%;
  width: 62%;
  background: repeating-linear-gradient(#fffaf0 0 24px, #f6c9bd 25px 26px);
}

.envelope-pull-card--flying {
  opacity: 0;
  transition: transform 380ms ease-in, opacity 380ms ease;
}

.envelope-instruction {
  display: grid;
  justify-items: center;
  gap: 2px;
  margin-top: 10px;
  text-align: center;
  font-size: 12px;
  color: #174f8c;
}

.envelope-instruction span {
  font-size: 20px;
  font-weight: 900;
  animation: envelope-swipe 900ms ease-in-out infinite alternate;
}

.envelope-status {
  min-height: 22px;
  padding-top: 4px;
  text-align: center;
  font-size: 11px;
  font-weight: 800;
  color: #456a8e;
}

@keyframes password-shake {
  25% { transform: translateX(-7px); }
  50% { transform: translateX(7px); }
  75% { transform: translateX(-4px); }
}

@keyframes lockbox-open-arrive {
  from { opacity: 0; transform: translateY(18px) scale(0.96); }
}

@keyframes lockbox-lid-open {
  from { height: 112px; transform: perspective(320px) rotateX(4deg); }
}

@keyframes envelope-arrive {
  from { opacity: 0; transform: translateY(42px) scale(0.82); }
}

@keyframes envelope-swipe {
  to { transform: translateY(-8px); }
}

@keyframes letter-open {
  from { opacity: 0; transform: translateY(120px) scale(0.9); }
}

@keyframes envelope-photo-loading {
  to { transform: rotate(360deg); }
}

@media (max-width: 430px) {
  .password-scene__intro {
    padding: 18px 4px 12px;
  }

  .password-scene__intro p {
    font-size: clamp(14px, 4.3vw, 17px);
    letter-spacing: 0.04em;
  }

  .password-scene__intro span {
    font-size: 11px;
    letter-spacing: 0.02em;
  }

  .password-lockbox__body {
    margin-right: 4px;
    margin-left: 4px;
    padding-right: 14px;
    padding-left: 14px;
  }

  .password-lock {
    width: min(244px, calc(100% - 34px));
  }

  .password-lockbox__floral-band {
    right: -4px;
    left: -4px;
  }

  .password-lockbox__floral-band i {
    width: min(48px, 13vw);
  }

  .password-lockbox--open .password-lockbox__body {
    padding-right: 14px;
    padding-left: 14px;
  }

  .password-lockbox__lining {
    right: 14px;
    left: 14px;
  }

  .password-lockbox__envelope {
    width: min(226px, calc(100% - 24px));
  }

  .envelope-photos {
    gap: 5px;
  }

  .envelope-photos figure {
    padding: 4px;
  }
}

@media (max-height: 700px) {
  .password-scene__intro { padding: 12px 10px 8px; }
  .password-lockbox__lid { height: 82px; }
  .password-lockbox__body { min-height: 162px; }
  .password-lockbox--open .password-lockbox__lid { height: 126px; }
  .password-lockbox--open .password-lockbox__body { min-height: 190px; }
  .password-lockbox__envelope { height: 118px; margin-top: 9px; }
  .password-lock { margin-top: 17px; }
  .password-unlock { margin-top: 9px; }
  .envelope-wrap { margin-top: 26px; }
  .envelope-photos img { height: 58px; }
  .envelope-letter { max-height: 220px; }
}

@media (prefers-reduced-motion: reduce) {
  .password-lockbox--error,
  .password-lockbox--open,
  .password-lockbox--open .password-lockbox__lid,
  .envelope-scene,
  .envelope-instruction span,
  .envelope-photo-loading i,
  .envelope-letter {
    animation: none;
  }

  .envelope-pull-card--flying {
    transition-duration: 1ms;
  }

  .password-lockbox--leaving {
    transition-duration: 1ms;
  }
}
</style>

<style scoped src="./PasswordStage.gift.css"></style>
