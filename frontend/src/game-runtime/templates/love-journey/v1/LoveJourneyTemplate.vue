<script setup lang="ts">
/* global HTMLAudioElement, HTMLElement */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import {
  createGameScreenshotFilename,
  downloadGameScreenshot,
} from '@/game-runtime/screenshot/downloadGameScreenshot'
import type { GameStageResult, GameTemplateProps } from '@/game-runtime/types'

import DiningStage from './stages/DiningStage.vue'
import FirstMeetingStage from './stages/FirstMeetingStage.vue'
import MovieStage from './stages/MovieStage.vue'
import PasswordStage from './stages/PasswordStage.vue'
import PuzzleStage from './stages/PuzzleStage.vue'
import TravelStage from './stages/TravelStage.vue'
import { loveJourneyPuzzlePieceCount, nextLoveJourneyStage } from './journeyFlow'
import { loveJourneyManifest, type LoveJourneyStageId } from './manifest'

const props = defineProps<GameTemplateProps>()
const emit = defineEmits<{ complete: [] }>()

const BACKGROUND_MUSIC_VOLUME = 0.35
const BACKGROUND_MUSIC_FADE_IN_MS = 3_000
const BACKGROUND_MUSIC_FADE_STEP_MS = 50
const STAGE_AUTO_ADVANCE_DELAY_MS = 700

const currentStageId = ref<LoveJourneyStageId>(loveJourneyManifest.initialStageId)
const stageRevision = ref(0)
const results = ref<Record<string, GameStageResult>>({})
const puzzlePhase = ref(false)
const pendingStageResult = ref<GameStageResult | null>(null)
const templateRoot = ref<HTMLElement | null>(null)
const backgroundMusic = ref<HTMLAudioElement | null>(null)
const screenshotStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle')
let backgroundMusicFadeTimer: number | undefined
let stageAutoAdvanceTimer: number | undefined
let backgroundMusicStarted = false
let backgroundMusicPlayPending = false

const currentStage = computed(() =>
  loveJourneyManifest.stages.find((stage) => stage.id === currentStageId.value)!,
)
const currentStageComplete = computed(() => Boolean(results.value[currentStageId.value]))
const currentPuzzlePieceCount = computed(() => loveJourneyPuzzlePieceCount(currentStageId.value))
const progressLabel = computed(() => `${currentStage.value.sequence} / ${loveJourneyManifest.stages.length}`)
const finalStageId = loveJourneyManifest.stages.at(-1)?.id
const journeyTitle = computed(() => props.gameConfig.config.openingTitle || loveJourneyManifest.displayName)
const letterPassword = computed(() => {
  const configuredPassword = props.gameConfig.config.letterPassword
  if (/^\d{4}$/.test(configuredPassword ?? '')) return configuredPassword!
  return props.gameConfig.templateVersion === '1.0.0' ? '2580' : ''
})

function playBackgroundMusic() {
  const music = backgroundMusic.value
  if (!music || backgroundMusicStarted || backgroundMusicPlayPending || !music.paused) return

  music.volume = 0
  backgroundMusicPlayPending = true
  try {
    void music.play()
      .then(() => {
        backgroundMusicPlayPending = false
        backgroundMusicStarted = true
        fadeInBackgroundMusic(music)
      })
      .catch(() => {
        backgroundMusicPlayPending = false
      })
  } catch {
    backgroundMusicPlayPending = false
    // Some embedded browsers can reject playback synchronously until a user gesture.
  }
}

function fadeInBackgroundMusic(music: HTMLAudioElement) {
  if (backgroundMusicFadeTimer !== undefined) {
    globalThis.window.clearInterval(backgroundMusicFadeTimer)
  }

  const startedAt = Date.now()
  backgroundMusicFadeTimer = globalThis.window.setInterval(() => {
    const progress = Math.min(1, (Date.now() - startedAt) / BACKGROUND_MUSIC_FADE_IN_MS)
    music.volume = BACKGROUND_MUSIC_VOLUME * progress
    if (progress === 1 && backgroundMusicFadeTimer !== undefined) {
      globalThis.window.clearInterval(backgroundMusicFadeTimer)
      backgroundMusicFadeTimer = undefined
    }
  }, BACKGROUND_MUSIC_FADE_STEP_MS)
}

onMounted(playBackgroundMusic)

onBeforeUnmount(() => {
  if (backgroundMusicFadeTimer !== undefined) {
    globalThis.window.clearInterval(backgroundMusicFadeTimer)
  }
  clearStageAutoAdvanceTimer()
  backgroundMusic.value?.pause()
})

function fallbackStageResult(): GameStageResult {
  return {
    stageId: currentStageId.value,
    completedAt: Date.now(),
    actionCount: 1,
  }
}

function recordStageGameplayCompletion(result?: GameStageResult) {
  if (results.value[currentStageId.value]) return

  const stageResult = result ?? fallbackStageResult()
  if (currentPuzzlePieceCount.value !== undefined) {
    if (puzzlePhase.value) return
    pendingStageResult.value = stageResult
    puzzlePhase.value = true
    return
  }

  results.value = {
    ...results.value,
    [currentStageId.value]: stageResult,
  }
  scheduleStageAutoAdvance(currentStageId.value)
}

function recordPuzzleCompletion(puzzleResult: GameStageResult) {
  if (!puzzlePhase.value || results.value[currentStageId.value]) return
  const stageResult = pendingStageResult.value ?? fallbackStageResult()
  results.value = {
    ...results.value,
    [currentStageId.value]: {
      ...stageResult,
      stageId: currentStageId.value,
      completedAt: puzzleResult.completedAt,
      actionCount: stageResult.actionCount + puzzleResult.actionCount,
      metadata: {
        ...stageResult.metadata,
        closingPuzzle: {
          pieceCount: currentPuzzlePieceCount.value,
          actionCount: puzzleResult.actionCount,
        },
      },
    },
  }
  scheduleStageAutoAdvance(currentStageId.value)
}

function clearStageAutoAdvanceTimer() {
  if (stageAutoAdvanceTimer === undefined) return
  globalThis.window.clearTimeout(stageAutoAdvanceTimer)
  stageAutoAdvanceTimer = undefined
}

function scheduleStageAutoAdvance(completedStageId: LoveJourneyStageId) {
  if (completedStageId === finalStageId) return
  clearStageAutoAdvanceTimer()
  stageAutoAdvanceTimer = globalThis.window.setTimeout(() => {
    stageAutoAdvanceTimer = undefined
    if (currentStageId.value !== completedStageId || !currentStageComplete.value) return
    advance()
  }, STAGE_AUTO_ADVANCE_DELAY_MS)
}

function advance() {
  if (!currentStageComplete.value) return
  clearStageAutoAdvanceTimer()
  const next = nextLoveJourneyStage(currentStageId.value)
  if (!next) {
    emit('complete')
    return
  }
  puzzlePhase.value = false
  pendingStageResult.value = null
  currentStageId.value = next
  stageRevision.value += 1
}

function skipCurrentStage() {
  if (props.mode !== 'creator-preview') return

  clearStageAutoAdvanceTimer()
  const next = nextLoveJourneyStage(currentStageId.value)
  if (!next) {
    emit('complete')
    return
  }

  puzzlePhase.value = false
  pendingStageResult.value = null
  currentStageId.value = next
  stageRevision.value += 1
}

watch(
  () => props.previewSkipRequest,
  (request, previousRequest) => {
    if (!request || request === previousRequest) return
    skipCurrentStage()
  },
  { immediate: true },
)

async function saveScreenshot() {
  if (!templateRoot.value || screenshotStatus.value === 'saving') return
  screenshotStatus.value = 'saving'
  try {
    await downloadGameScreenshot(
      templateRoot.value,
      createGameScreenshotFilename(journeyTitle.value),
    )
    screenshotStatus.value = 'saved'
  } catch {
    screenshotStatus.value = 'error'
  }
}
</script>

<template>
  <div
    ref="templateRoot"
    class="love-journey-template"
    @pointerdown.capture="playBackgroundMusic"
    @keydown.capture="playBackgroundMusic"
  >
    <audio
      ref="backgroundMusic"
      src="/assets/love-journey/background-music.mp3"
      preload="auto"
      loop
      aria-hidden="true"
      data-screenshot-exclude="true"
    />

    <div
      class="journey-progress"
      aria-label="游戏进度"
    >
      <span :title="journeyTitle">{{ journeyTitle }}</span>
      <span>{{ progressLabel }}</span>
    </div>

    <main class="journey-stage">
      <PuzzleStage
        v-if="puzzlePhase && currentPuzzlePieceCount !== undefined"
        :key="`closing-puzzle-${currentStageId}-${stageRevision}`"
        :active="true"
        :piece-count="currentPuzzlePieceCount"
        :experience-label="currentStage.label"
        :experience-title="currentStage.title"
        @complete="recordPuzzleCompletion"
      />
      <FirstMeetingStage
        v-else-if="currentStageId === 'first-meeting'"
        :key="`first-meeting-${stageRevision}`"
        :active="true"
        @complete="recordStageGameplayCompletion"
      />
      <DiningStage
        v-else-if="currentStageId === 'dining'"
        :key="`dining-${stageRevision}`"
        :active="true"
        @complete="recordStageGameplayCompletion"
      />
      <MovieStage
        v-else-if="currentStageId === 'movie'"
        :key="`movie-${stageRevision}`"
        :active="true"
        @complete="recordStageGameplayCompletion"
      />
      <TravelStage
        v-else-if="currentStageId === 'travel'"
        :key="`travel-${stageRevision}`"
        :active="true"
        @complete="recordStageGameplayCompletion"
      />
      <PasswordStage
        v-else
        :key="`password-${stageRevision}`"
        :active="true"
        :password="letterPassword"
        :password-hint="gameConfig.config.passwordHint"
        :photos="gameConfig.assets"
        :love-letter="gameConfig.config.loveLetter ?? ''"
        @complete="recordStageGameplayCompletion"
      />
    </main>

    <div
      v-if="currentStageComplete && currentStageId === finalStageId"
      class="journey-flow-action"
    >
      <button
        class="journey-primary-button journey-screenshot-button"
        type="button"
        data-testid="journey-save-screenshot"
        data-screenshot-exclude="true"
        :disabled="screenshotStatus === 'saving'"
        @click="saveScreenshot"
      >
        {{ screenshotStatus === 'saving' ? '正在生成截图…' : '保存游戏截图 ↓' }}
      </button>
      <button
        class="journey-primary-button"
        type="button"
        data-testid="journey-complete"
        @click="advance"
      >
        完成这段旅程 →
      </button>
      <p
        v-if="screenshotStatus !== 'idle'"
        class="journey-screenshot-status"
        data-screenshot-exclude="true"
        role="status"
        aria-live="polite"
      >
        {{ screenshotStatus === 'saving'
          ? '正在生成 PNG 图片'
          : screenshotStatus === 'saved'
            ? '截图已开始下载'
            : '截图生成失败，请重试' }}
      </p>
    </div>
  </div>
</template>

<style>
.love-journey-template {
  --journey-black: #000;
  --journey-white: #fff;
  --journey-muted: #3d3d3d;

  display: flex;
  width: min(100%, 430px);
  max-width: 100vw;
  min-height: min(760px, 100dvh);
  margin: 0 auto;
  box-sizing: border-box;
  flex-direction: column;
  overflow: hidden;
  border: 2px solid var(--journey-black);
  color: var(--journey-black);
  background: var(--journey-white);
  font-family:
    Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI",
    sans-serif;
  text-align: left;
}

.journey-progress {
  display: flex;
  min-height: 34px;
  align-items: center;
  justify-content: space-between;
  padding: 0 18px;
  border-bottom: 1px dashed var(--journey-black);
  background: #f3f3f3;
  font-size: 11px;
  font-weight: 750;
  letter-spacing: 0.1em;
}

.journey-progress span:first-child {
  overflow: hidden;
  min-width: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.journey-stage {
  display: flex;
  width: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.journey-scene {
  display: flex;
  width: 100%;
  min-height: 0;
  min-width: 0;
  box-sizing: border-box;
  flex: 1;
  flex-direction: column;
  padding: clamp(18px, 5vw, 28px);
  color: var(--journey-black);
  background: var(--journey-white);
}

.journey-scene__header {
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  border-bottom: 2px solid var(--journey-black);
}

.journey-scene__header h1,
.journey-scene__header p {
  margin: 0;
}

.journey-scene__header h1 {
  max-width: 62%;
  padding-bottom: 8px;
  overflow-wrap: anywhere;
  font-size: clamp(26px, 8vw, 34px);
  line-height: 1;
}

.journey-scene__header p {
  color: var(--journey-muted);
  font-size: 13px;
  letter-spacing: 0.14em;
}

.journey-primary-button {
  width: 100%;
  min-height: 46px;
  padding: 0 14px;
  border: 2px solid var(--journey-black);
  border-radius: 0;
  color: var(--journey-black);
  background: var(--journey-white);
  font-weight: 800;
  cursor: pointer;
  touch-action: manipulation;
}

.journey-primary-button:active {
  color: var(--journey-white);
  background: var(--journey-black);
}

.journey-primary-button:focus-visible {
  outline: 3px double var(--journey-black);
  outline-offset: 2px;
}

.journey-flow-action {
  display: grid;
  gap: 8px;
  padding: 0 18px 16px;
}

.journey-screenshot-button {
  color: var(--journey-white);
  background: var(--journey-black);
}

.journey-screenshot-button:active:not(:disabled) {
  color: var(--journey-black);
  background: var(--journey-white);
}

.journey-screenshot-button:disabled {
  border-style: dashed;
  color: #555;
  background: #eee;
  cursor: wait;
}

.journey-screenshot-status {
  min-height: 18px;
  margin: 0;
  text-align: center;
  font-size: 12px;
}

@media (max-width: 430px) {
  .love-journey-template {
    width: 100%;
    min-height: 100dvh;
    border: 0;
  }

  .journey-scene {
    padding: 14px;
  }

  .journey-scene__header h1 {
    max-width: 58%;
    font-size: clamp(24px, 8vw, 30px);
  }
}

@media (max-height: 650px) {
  .love-journey-template {
    min-height: 100dvh;
  }

  .journey-scene {
    padding-top: 12px;
    padding-bottom: 10px;
  }
}
</style>
