<script setup lang="ts">
/* global HTMLElement, ResizeObserver */
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue'

import type { GameStageResult } from '@/game-runtime/types'

import {
  advanceAfterDisplay,
  createFirstMeetingState,
  firstMeetingRoundSequences,
  getCurrentSequence,
  getExpectedEmoji,
  selectEmoji,
  sendCurrentRound,
  type FirstMeetingEmoji,
} from './firstMeetingState'

interface EmojiChoice {
  id: FirstMeetingEmoji
  label: string
}

type GuideTarget = FirstMeetingEmoji | 'send'

const assetBase = '/assets/love-journey/first-meeting'
const emojiChoices = [
  { id: 'wink', label: '眨眼笑脸' },
  { id: 'heart', label: '爱心' },
  { id: 'laugh', label: '大笑脸' },
  { id: 'blush', label: '害羞笑脸' },
] as const satisfies readonly EmojiChoice[]
const emojiAssets: Record<FirstMeetingEmoji, string> = {
  wink: `${assetBase}/emoji-wink.png`,
  heart: `${assetBase}/emoji-heart.png`,
  laugh: `${assetBase}/emoji-laugh.png`,
  blush: `${assetBase}/emoji-blush.png`,
}
const emojiLabels = Object.fromEntries(
  emojiChoices.map((choice) => [choice.id, choice.label]),
) as Record<FirstMeetingEmoji, string>
const replyDisplayMilliseconds = 900

const props = defineProps<{ active: boolean }>()
const emit = defineEmits<{ complete: [result: GameStageResult] }>()

const state = ref(createFirstMeetingState())
const composer = ref<HTMLElement | null>(null)
const completionEmitted = ref(false)
const controlElements = new Map<GuideTarget, HTMLElement>()
const guidePosition = ref({ left: 0, top: 0, width: 0, ready: false })
let displayTimer: number | undefined
let composerResizeObserver: ResizeObserver | undefined

const currentSequence = computed(() => getCurrentSequence(state.value))
const expectedEmoji = computed(() => getExpectedEmoji(state.value))
const currentGuideTarget = computed<GuideTarget | null>(() => {
  if (!props.active) return null
  if (state.value.phase === 'selecting') return expectedEmoji.value
  if (state.value.phase === 'ready-to-send') return 'send'
  return null
})
const guideStyle = computed(() => ({
  left: `${guidePosition.value.left}px`,
  top: `${guidePosition.value.top}px`,
  width: `${guidePosition.value.width}px`,
}))
const partnerAriaLabel = computed(() => (
  `对方发来：${sequenceText(currentSequence.value)}`
))
const playerAriaLabel = computed(() => state.value.playerMessage
  ? `玩家发出：${sequenceText(state.value.playerMessage)}`
  : '')
const statusText = computed(() => {
  const round = state.value.roundIndex + 1
  if (state.value.phase === 'completed') return '三轮对话已完成'
  if (state.value.phase === 'sent') return `第 ${round} 轮已发送，双方消息顺序一致`
  if (state.value.phase === 'ready-to-send') return `第 ${round} 轮已选满，请点击发送`
  const expected = expectedEmoji.value
  return expected
    ? `第 ${round} 轮，请选择${emojiLabels[expected]}，已选择 ${state.value.selectedEmojis.length} 个`
    : `第 ${round} 轮正在选择`
})

function sequenceText(sequence: readonly FirstMeetingEmoji[]) {
  return sequence.map((emoji) => emojiLabels[emoji]).join('、')
}

function setControlElement(id: GuideTarget, element: unknown) {
  if (element instanceof HTMLElement) controlElements.set(id, element)
  else controlElements.delete(id)
}

function updateGuidePosition() {
  const host = composer.value
  const target = currentGuideTarget.value
    ? controlElements.get(currentGuideTarget.value)
    : undefined
  if (!host || !target) {
    guidePosition.value = { ...guidePosition.value, ready: false }
    return
  }

  const hostBounds = host.getBoundingClientRect()
  const targetBounds = target.getBoundingClientRect()
  guidePosition.value = {
    left: targetBounds.left - hostBounds.left + targetBounds.width / 2,
    top: targetBounds.top - hostBounds.top + targetBounds.height / 2,
    width: Math.max(targetBounds.width, targetBounds.height) * 1.12,
    ready: true,
  }
}

function chooseEmoji(emoji: FirstMeetingEmoji) {
  if (!props.active || state.value.phase !== 'selecting' || displayTimer !== undefined) return
  const nextState = selectEmoji(state.value, emoji)
  if (nextState === state.value) return

  state.value = nextState
  void nextTick(updateGuidePosition)
  globalThis.navigator.vibrate?.(8)
}

function submitRound() {
  if (!props.active || state.value.phase !== 'ready-to-send' || displayTimer !== undefined) return
  const nextState = sendCurrentRound(state.value)
  if (nextState === state.value) return

  state.value = nextState
  globalThis.navigator.vibrate?.(12)
  displayTimer = globalThis.window.setTimeout(() => {
    displayTimer = undefined
    state.value = advanceAfterDisplay(state.value)
    void nextTick(updateGuidePosition)
    if (state.value.phase === 'completed') finish()
  }, replyDisplayMilliseconds)
}

function finish() {
  if (completionEmitted.value || state.value.phase !== 'completed') return
  completionEmitted.value = true
  globalThis.navigator.vibrate?.([18, 30, 18])
  emit('complete', {
    stageId: 'first-meeting',
    completedAt: Date.now(),
    actionCount: firstMeetingRoundSequences.length,
    metadata: {
      roundSequences: firstMeetingRoundSequences.map((sequence) => [...sequence]),
    },
  })
}

watch(currentGuideTarget, () => {
  void nextTick(updateGuidePosition)
})

onMounted(() => {
  updateGuidePosition()
  if (composer.value && typeof globalThis.ResizeObserver !== 'undefined') {
    composerResizeObserver = new globalThis.ResizeObserver(updateGuidePosition)
    composerResizeObserver.observe(composer.value)
  }
  globalThis.window.addEventListener('resize', updateGuidePosition)
})

onBeforeUnmount(() => {
  if (displayTimer !== undefined) globalThis.window.clearTimeout(displayTimer)
  composerResizeObserver?.disconnect()
  globalThis.window.removeEventListener('resize', updateGuidePosition)
})
</script>

<template>
  <section
    class="journey-scene first-meeting-scene"
    aria-labelledby="first-meeting-title"
    :data-phase="state.phase"
    :data-round="state.roundIndex + 1"
  >
    <h1
      id="first-meeting-title"
      class="first-meeting-visually-hidden"
    >
      场景 1：初见
    </h1>

    <div class="first-meeting-artboard">
      <img
        class="first-meeting-artboard__background"
        :src="`${assetBase}/background.png`"
        alt=""
        aria-hidden="true"
      >
      <div
        class="first-meeting-character-layer"
        aria-hidden="true"
      >
        <img
          class="first-meeting-character first-meeting-character--left"
          :src="`${assetBase}/character-left.png`"
          alt=""
        >
        <img
          class="first-meeting-character first-meeting-character--right"
          :src="`${assetBase}/character-right.png`"
          alt=""
        >
      </div>

      <div
        class="first-meeting-bubble first-meeting-bubble--partner"
        data-testid="first-meeting-partner-bubble"
        role="img"
        :aria-label="partnerAriaLabel"
      >
        <img
          class="first-meeting-bubble__frame"
          :src="`${assetBase}/partner-bubble-empty.png`"
          alt=""
          aria-hidden="true"
        >
        <span class="first-meeting-bubble__emojis first-meeting-bubble__emojis--partner">
          <img
            v-for="emoji in currentSequence"
            :key="`partner-${state.roundIndex}-${emoji}`"
            :src="emojiAssets[emoji]"
            alt=""
            aria-hidden="true"
          >
        </span>
      </div>

      <div
        v-if="state.phase === 'sent' || state.phase === 'completed'"
        class="first-meeting-bubble first-meeting-bubble--player"
        data-testid="first-meeting-player-bubble"
        role="img"
        :aria-label="playerAriaLabel"
      >
        <img
          class="first-meeting-bubble__frame"
          :src="`${assetBase}/player-bubble-empty.png`"
          alt=""
          aria-hidden="true"
        >
        <span class="first-meeting-bubble__emojis first-meeting-bubble__emojis--player">
          <img
            v-for="emoji in state.playerMessage"
            :key="`player-${state.roundIndex}-${emoji}`"
            :src="emojiAssets[emoji]"
            alt=""
            aria-hidden="true"
          >
        </span>
      </div>

      <div
        ref="composer"
        class="first-meeting-composer"
        aria-label="Emoji 输入区"
      >
        <img
          class="first-meeting-guide"
          :class="{ 'first-meeting-guide--visible': guidePosition.ready && currentGuideTarget }"
          :style="guideStyle"
          :src="`${assetBase}/guide-ring.png`"
          alt=""
          aria-hidden="true"
          data-testid="first-meeting-guide"
          :data-target="currentGuideTarget"
        >

        <div
          class="first-meeting-control-row"
          data-testid="first-meeting-control-row"
        >
          <button
            v-for="choice in emojiChoices"
            :key="choice.id"
            :ref="(element) => setControlElement(choice.id, element)"
            class="first-meeting-control first-meeting-control--emoji"
            type="button"
            :data-testid="`first-meeting-emoji-${choice.id}`"
            :data-emoji="choice.id"
            :aria-label="`选择${choice.label}`"
            :aria-current="currentGuideTarget === choice.id ? 'step' : undefined"
            :disabled="!active || state.phase !== 'selecting'"
            @click="chooseEmoji(choice.id)"
          >
            <img
              :src="emojiAssets[choice.id]"
              alt=""
              aria-hidden="true"
            >
          </button>

          <button
            :ref="(element) => setControlElement('send', element)"
            class="first-meeting-control first-meeting-control--send"
            type="button"
            aria-label="发送"
            data-testid="first-meeting-send"
            :aria-current="currentGuideTarget === 'send' ? 'step' : undefined"
            :disabled="!active || state.phase !== 'ready-to-send'"
            @click="submitRound"
          >
            <img
              :src="`${assetBase}/send-button.png`"
              alt=""
              aria-hidden="true"
            >
          </button>
        </div>
      </div>

      <p
        class="first-meeting-visually-hidden"
        role="status"
        aria-live="polite"
        data-testid="first-meeting-status"
      >
        {{ statusText }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.first-meeting-scene {
  --first-meeting-blue: #123195;
  --first-meeting-focus: #ffd43b;

  position: relative;
  display: block;
  min-height: min(726px, calc(100dvh - 34px));
  padding: 0;
  overflow: hidden;
  color: var(--first-meeting-blue);
  background: #fdfcfb;
  container-type: size;
  isolation: isolate;
}

.first-meeting-artboard {
  position: absolute;
  z-index: 0;
  top: 50%;
  left: 50%;
  width: min(100cqw, calc(100cqh * 0.562201));
  height: auto;
  overflow: hidden;
  aspect-ratio: 940 / 1672;
  transform: translate(-50%, -50%);
}

.first-meeting-artboard__background,
.first-meeting-character-layer,
.first-meeting-character,
.first-meeting-bubble,
.first-meeting-bubble__frame,
.first-meeting-bubble__emojis,
.first-meeting-guide,
.first-meeting-control-row {
  position: absolute;
}

.first-meeting-artboard__background {
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: fill;
}

.first-meeting-artboard img {
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
}

.first-meeting-character-layer {
  z-index: 1;
  inset: 0;
  overflow: hidden;
  clip-path: inset(0 0 44.02% 0);
}

.first-meeting-character {
  display: block;
  height: auto;
}

.first-meeting-character--left {
  top: 22.07%;
  left: 6.61%;
  width: 40.7%;
}

.first-meeting-character--right {
  top: 14.43%;
  left: 48.44%;
  width: 52.29%;
}

.first-meeting-bubble {
  z-index: 3;
  height: auto;
  animation: first-meeting-message-pop 220ms cubic-bezier(0.2, 0.85, 0.3, 1.2);
}

.first-meeting-bubble--partner {
  top: 64.05%;
  left: 8.51%;
  width: 64.94%;
  aspect-ratio: 1811 / 459;
}

.first-meeting-bubble--player {
  top: 70%;
  left: 22.77%;
  width: 71.63%;
  aspect-ratio: 1285 / 404;
}

.first-meeting-bubble__frame {
  z-index: 0;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
}

.first-meeting-bubble__emojis {
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.first-meeting-bubble__emojis--partner {
  top: 29%;
  left: 32.9%;
  width: 59.9%;
  height: 48%;
}

.first-meeting-bubble__emojis--player {
  top: 44.5%;
  left: 7.77%;
  width: 52.51%;
  height: 35.1%;
}

.first-meeting-bubble__emojis img {
  display: block;
  width: auto;
  max-width: 20%;
  height: 100%;
  object-fit: contain;
}

.first-meeting-composer {
  position: absolute;
  z-index: 5;
  inset: 0;
  touch-action: manipulation;
}

.first-meeting-control-row {
  top: 91.4%;
  left: 12.5%;
  display: grid;
  width: 75%;
  grid-template-columns: repeat(5, 1fr);
  align-items: center;
  transform: translateY(-50%);
}

.first-meeting-control {
  position: relative;
  z-index: 2;
  display: grid;
  width: max(92.87%, 44px);
  min-width: 44px;
  min-height: 44px;
  aspect-ratio: 1;
  padding: 0;
  border: 0;
  border-radius: 50%;
  outline: 0;
  place-items: center;
  justify-self: center;
  background: transparent;
  cursor: pointer;
  transform: none;
  -webkit-tap-highlight-color: transparent;
}

.first-meeting-control img {
  display: block;
  width: 65%;
  height: 65%;
  object-fit: contain;
}

.first-meeting-control--send img {
  width: 80%;
  height: 80%;
}

.first-meeting-control:focus-visible {
  border-radius: 18%;
  outline: 4px solid var(--first-meeting-focus);
  outline-offset: -4px;
}

.first-meeting-control:disabled {
  cursor: default;
}

.first-meeting-guide {
  z-index: 3;
  display: block;
  height: auto;
  opacity: 0;
  transform: translate(-50%, -50%) scale(0.86);
  transition:
    left 180ms ease-out,
    top 180ms ease-out,
    width 180ms ease-out,
    opacity 120ms ease-out,
    transform 180ms cubic-bezier(0.2, 0.85, 0.3, 1.2);
}

.first-meeting-guide--visible {
  opacity: 1;
  transform: translate(-50%, -50%) scale(1);
}

@keyframes first-meeting-message-pop {
  from {
    opacity: 0;
    transform: translateY(6%) scale(0.94);
  }
}

.first-meeting-visually-hidden {
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
  .first-meeting-bubble {
    animation: none;
  }

  .first-meeting-guide {
    transition: none;
  }
}
</style>
