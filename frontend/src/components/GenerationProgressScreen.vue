<script setup lang="ts">
import { computed } from 'vue'

import type { GenerationRun } from '@/api/generation'
import type { CreatorPreview, GameAsset } from '@/api/games'

type StepState = 'completed' | 'active' | 'pending' | 'error'
const LOVE_JOURNEY_MEMORY_COUNT = 5

const props = withDefaults(defineProps<{
  run?: GenerationRun | null
  preparing?: boolean
  loadError?: string
  gameId?: string
  preview?: CreatorPreview | null
  sourceAssets?: GameAsset[]
}>(), {
  run: null,
  preparing: false,
  loadError: '',
  gameId: '',
  preview: null,
  sourceAssets: () => [],
})

const isComplete = computed(() => props.run?.status === 'succeeded')
const isStopped = computed(() => props.run?.status === 'failed' || props.run?.status === 'cancelled' || Boolean(props.loadError))
const activeStepIndex = computed(() => {
  if (isComplete.value) return 4
  if (!props.run || props.preparing || props.run.stage === 'queued') return 0
  if (props.run.stage === 'transforming_images') return 1
  if (props.run.stage === 'saving_results') return 2
  return 3
})
const displayProgress = computed(() => {
  if (isComplete.value) return 100
  const progress = Math.max(0, Math.min(100, props.run?.progress ?? 0))
  if (props.preparing || !props.run || props.run.stage === 'queued') return Math.max(progress, 8)
  if (props.run.stage === 'transforming_images') return Math.max(progress, 68)
  if (props.run.stage === 'saving_results') return Math.max(progress, 88)
  return progress
})

const stepDefinitions = [
  {
    title: '识别两个人物特征',
    active: '正在识别照片中的人物特征…',
    completed: '脸型、发型与显著标记已提取',
  },
  {
    title: '绘制回忆场景',
    active: '正在生成回忆场景…',
    completed: '回忆场景已经绘制完成',
  },
  {
    title: '整理照片与情书',
    active: '正在整理照片与情书…',
    completed: '照片与情书已经整理完成',
  },
  {
    title: '装订互动礼物',
    active: '正在装订最后的互动细节…',
    completed: '互动礼物已经装订完成',
  },
]

function stepState(index: number): StepState {
  if (isComplete.value || index < activeStepIndex.value) return 'completed'
  if (index === activeStepIndex.value) return isStopped.value ? 'error' : 'active'
  return 'pending'
}

function stepDescription(index: number) {
  const definition = stepDefinitions[index]
  const state = stepState(index)
  if (!definition) return ''
  if (state === 'completed') return definition.completed
  if (state === 'active') return definition.active
  if (state === 'error') return props.loadError || props.run?.errorMessage || '制作暂时中断，可以稍后继续处理'
  return '等待开始'
}

const heading = computed(() => {
  if (isComplete.value) return '你们的故事制作好了'
  if (isStopped.value) return '制作暂时停下来了'
  return '让回忆慢慢成形'
})
const subheading = computed(() => {
  if (isStopped.value) return '已经保存的资料不会丢失，可以返回修改页面重新处理。'
  return '制作期间请保持页面打开。'
})
const completionImages = computed(() => {
  const urls = [
    ...(props.preview?.assets.map((asset) => asset.url) ?? []),
    ...props.sourceAssets.map((asset) => asset.previewUrl),
  ]
  return [...new Set(urls.filter(Boolean))].slice(0, 2)
})
const completionStats = computed(() => [
  {
    value: LOVE_JOURNEY_MEMORY_COUNT,
    label: '段回忆',
  },
  {
    value: props.sourceAssets.filter((asset) => asset.slotKey !== 'cover').length,
    label: '张照片',
  },
  {
    value: props.preview?.config.loveLetter?.trim() ? 1 : 0,
    label: '封情书',
  },
])
const previewVersionId = computed(() => props.preview?.version.id ?? props.run?.gameVersionId ?? '')
</script>

<template>
  <section
    class="generation-progress-page"
    :data-status="run?.status ?? (preparing ? 'preparing' : 'loading')"
  >
    <template v-if="isComplete">
      <div class="generation-complete">
        <div
          class="generation-complete__visual"
          aria-label="故事照片已经装订完成"
        >
          <i class="generation-complete__oval" />
          <i class="generation-complete__spark generation-complete__spark--one">✦</i>
          <i class="generation-complete__spark generation-complete__spark--two">✦</i>
          <i class="generation-complete__heart">♡</i>
          <figure
            v-for="position in 2"
            :key="position"
            :class="`generation-complete__photo generation-complete__photo--${position}`"
          >
            <img
              v-if="completionImages[position - 1]"
              :src="completionImages[position - 1]"
              :alt="`生成后的回忆画面 ${position}`"
            >
            <span v-else>回忆画面</span>
          </figure>
          <strong class="generation-complete__check">✓</strong>
        </div>

        <header class="generation-complete__heading">
          <p>回忆已经装订完成</p>
          <h1>你们的故事，做好了</h1>
          <span>先以对方的视角体验一次，再决定是否分享。</span>
        </header>

        <dl class="generation-complete__stats">
          <div
            v-for="(stat, index) in completionStats"
            :key="stat.label"
          >
            <dt>{{ stat.value }}</dt>
            <dd>{{ stat.label }}</dd>
            <i v-if="index < completionStats.length - 1" />
          </div>
        </dl>

        <RouterLink
          v-if="gameId && previewVersionId"
          class="generation-complete__preview"
          :to="{ name: 'game-preview', params: { gameId }, query: { versionId: previewVersionId } }"
        >
          预览我们的故事 <span aria-hidden="true">→</span>
        </RouterLink>
        <RouterLink
          v-if="gameId"
          class="generation-complete__edit"
          :to="{ name: 'game-edit', params: { gameId } }"
        >
          返回修改内容
        </RouterLink>
      </div>
    </template>

    <template v-else>
      <div
        class="generation-progress-visual"
        aria-hidden="true"
      >
        <i class="generation-progress-visual__oval" />
        <i class="generation-progress-visual__dot generation-progress-visual__dot--one" />
        <i class="generation-progress-visual__dot generation-progress-visual__dot--two" />
        <i class="generation-progress-visual__dot generation-progress-visual__dot--three" />
        <div class="generation-progress-visual__sheet generation-progress-visual__sheet--coral" />
        <div class="generation-progress-visual__sheet generation-progress-visual__sheet--cyan" />
        <div class="generation-progress-visual__paper">
          <strong>{{ displayProgress }}%</strong>
          <span>♡</span>
        </div>
      </div>

      <header class="generation-progress-heading">
        <p>正在制作你们的故事</p>
        <h1>{{ heading }}</h1>
        <span>{{ subheading }}</span>
      </header>

      <ol
        class="generation-progress-steps"
        aria-label="游戏制作进度"
      >
        <li
          v-for="(step, index) in stepDefinitions"
          :key="step.title"
          :class="`generation-progress-step--${stepState(index)}`"
        >
          <span class="generation-progress-step__marker">
            {{ stepState(index) === 'completed' ? '✓' : stepState(index) === 'error' ? '!' : '' }}
          </span>
          <span class="generation-progress-step__copy">
            <strong>{{ step.title }}</strong>
            <small>{{ stepDescription(index) }}</small>
          </span>
          <b v-if="stepState(index) === 'active'">{{ displayProgress }}%</b>
        </li>
      </ol>

      <aside class="generation-progress-tip">
        <strong>小提示</strong>
        <span>{{ isStopped ? '素材和草稿已经保存，可以放心返回检查。' : '具体的小事，往往比盛大的告白更容易被记住。' }}</span>
      </aside>

      <RouterLink
        v-if="gameId && isStopped"
        class="generation-progress-action"
        :to="{ name: 'game-edit', params: { gameId } }"
      >
        返回修改内容
      </RouterLink>
    </template>
  </section>
</template>

<style scoped>
.generation-progress-page {
  width: 100%;
  margin-inline: auto;
  padding: 0 18px 22px;
  color: #252662;
  background: transparent;
}

.generation-progress-visual {
  position: relative;
  height: 220px;
  margin: 0 -18px;
  overflow: hidden;
}

.generation-progress-visual__oval {
  position: absolute;
  top: 12px;
  left: -66px;
  width: 205px;
  height: 110px;
  border: 4px solid #202796;
  border-radius: 50%;
  background: #d8eff5;
  transform: rotate(-4deg);
}

.generation-progress-visual__sheet,
.generation-progress-visual__paper {
  position: absolute;
  top: 22px;
  left: 50%;
  width: 150px;
  height: 185px;
  border: 4px solid #202796;
  border-radius: 12px;
}

.generation-progress-visual__sheet--coral {
  background: #ffc878;
  transform: translateX(-53%) rotate(-9deg);
}

.generation-progress-visual__sheet--cyan {
  background: #d8eff5;
  transform: translateX(-43%) rotate(7deg);
}

.generation-progress-visual__paper {
  display: grid;
  place-content: center;
  gap: 8px;
  background: #fbfdf8;
  box-shadow: 5px 7px 0 rgb(32 39 150 / 12%);
  text-align: center;
  transform: translateX(-50%);
}

.generation-progress-visual__paper strong {
  color: #202796;
  font-family: Georgia, "Times New Roman", serif;
  font-size: 2.55rem;
  font-weight: 500;
}

.generation-progress-visual__paper span {
  color: #e96b55;
  font-size: 2.45rem;
  line-height: 0.8;
}

.generation-progress-visual__dot {
  position: absolute;
  z-index: 4;
  width: 15px;
  height: 15px;
  border: 3px solid #202796;
  border-radius: 50%;
  background: #fbb303;
}

.generation-progress-visual__dot--one {
  top: 16px;
  right: 26%;
}

.generation-progress-visual__dot--two {
  right: 22%;
  bottom: 24px;
  background: #62c5d9;
}

.generation-progress-visual__dot--three {
  bottom: 6px;
  left: 24%;
  background: #e96b55;
}

.generation-progress-heading {
  text-align: center;
}

.generation-progress-heading p {
  margin: 0 0 6px;
  color: #e96b55;
  font-size: 0.78rem;
  font-weight: 800;
  letter-spacing: 0.12em;
}

.generation-progress-heading h1 {
  margin: 0 0 3px;
  color: #202796;
  font-family: "Kaiti SC", STKaiti, serif;
  font-size: clamp(1.65rem, 7vw, 2.05rem);
  font-weight: 500;
  letter-spacing: 0.08em;
}

.generation-progress-heading > span {
  color: #7b8199;
  font-size: 0.78rem;
}

.generation-progress-steps {
  display: grid;
  gap: 8px;
  margin: 18px 0 14px;
  padding: 0;
  list-style: none;
}

.generation-progress-steps li {
  display: flex;
  min-height: 62px;
  align-items: center;
  gap: 10px;
  padding: 9px 13px;
  border: 3px solid #202796;
  border-radius: 14px;
  background: #d8eff5;
  box-shadow: 4px 5px 0 rgb(32 39 150 / 10%);
}

.generation-progress-steps li.generation-progress-step--pending {
  border-color: #b8c6e3;
  color: #8d93a8;
  background: #edf7f7;
  box-shadow: none;
}

.generation-progress-steps li.generation-progress-step--error {
  border-color: #e96b55;
  background: #fff0e9;
}

.generation-progress-step__marker {
  display: grid;
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  place-items: center;
  border: 3px solid currentColor;
  border-radius: 50%;
  font-weight: 900;
}

.generation-progress-step--completed .generation-progress-step__marker {
  color: #fff;
  background: #202796;
}

.generation-progress-step--active .generation-progress-step__marker {
  border-style: dashed;
  animation: generation-progress-spin 2.4s linear infinite;
}

.generation-progress-step__copy {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 3px;
}

.generation-progress-step__copy strong {
  color: inherit;
  font-size: 0.88rem;
}

.generation-progress-step__copy small {
  color: inherit;
  font-size: 0.68rem;
}

.generation-progress-steps li > b {
  color: #202796;
  font-size: 0.9rem;
}

.generation-progress-tip {
  display: flex;
  gap: 12px;
  padding: 10px 13px;
  border: 3px solid #202796;
  border-radius: 15px;
  background: #fff1ba;
  font-size: 0.7rem;
  line-height: 1.55;
}

.generation-progress-tip strong {
  flex: 0 0 auto;
  color: #202796;
}

.generation-progress-tip span {
  color: #7b7180;
}

.generation-progress-action {
  display: flex;
  min-height: 58px;
  align-items: center;
  justify-content: center;
  margin-top: 18px;
  border: 3px solid #202796;
  border-radius: 16px;
  color: #fff;
  background: #e96b55;
  box-shadow: 5px 6px 0 #202796;
  font-weight: 800;
}

.generation-complete {
  padding-top: 2px;
  text-align: center;
}

.generation-complete__visual {
  position: relative;
  height: 320px;
  margin: 0 -18px -2px;
  overflow: hidden;
}

.generation-complete__oval {
  position: absolute;
  top: 8px;
  left: -72px;
  width: 230px;
  height: 118px;
  border: 4px solid #202796;
  border-radius: 50%;
  background: #d8eff5;
}

.generation-complete__photo {
  position: absolute;
  top: 34px;
  left: 50%;
  width: 198px;
  height: 254px;
  margin: 0;
  padding: 9px;
  border: 5px solid #202796;
  background: #fbfdf8;
  box-shadow: 7px 8px 0 rgb(32 39 150 / 10%);
}

.generation-complete__photo--1 {
  z-index: 2;
  transform: translateX(-104%) rotate(-7deg);
}

.generation-complete__photo--2 {
  z-index: 1;
  transform: translateX(2%) rotate(6deg);
}

.generation-complete__photo img,
.generation-complete__photo > span {
  display: grid;
  width: 100%;
  height: 100%;
  place-items: center;
  color: #202796;
  background: linear-gradient(145deg, #d8eff5 0 58%, #ffc878 58% 100%);
  object-fit: cover;
  font-family: "Kaiti SC", STKaiti, serif;
  font-size: 1rem;
}

.generation-complete__check {
  position: absolute;
  z-index: 5;
  bottom: 2px;
  left: 50%;
  display: grid;
  width: 70px;
  height: 70px;
  place-items: center;
  border: 5px solid #202796;
  border-radius: 50%;
  color: #fff;
  background: #e96b55;
  font-size: 2.25rem;
  transform: translateX(-50%);
}

.generation-complete__spark,
.generation-complete__heart {
  position: absolute;
  z-index: 4;
  color: #fbb303;
  font-style: normal;
  font-size: 2rem;
}

.generation-complete__spark--one { top: 10px; left: 7%; }
.generation-complete__spark--two { top: 84px; right: 2%; color: #32b5d0; }
.generation-complete__heart { bottom: 32px; left: 1%; color: #e96b55; font-size: 2.8rem; }

.generation-complete__heading p {
  margin: 0 0 8px;
  color: #e96b55;
  font-size: 0.82rem;
  font-weight: 800;
  letter-spacing: 0.12em;
}

.generation-complete__heading h1 {
  margin: 0 0 4px;
  color: #202796;
  font-family: "Kaiti SC", STKaiti, serif;
  font-size: clamp(1.85rem, 8vw, 2.35rem);
  font-weight: 500;
  letter-spacing: 0.06em;
}

.generation-complete__heading span {
  color: #7b8199;
  font-size: 0.78rem;
}

.generation-complete__stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  margin: 20px 0 16px;
  padding: 12px 7px;
  border-block: 4px solid #202796;
  background: #d8eff5;
}

.generation-complete__stats div {
  position: relative;
  display: flex;
  align-items: baseline;
  justify-content: center;
  gap: 4px;
}

.generation-complete__stats dt {
  color: #202796;
  font-size: 1.55rem;
  font-weight: 700;
}

.generation-complete__stats dd {
  margin: 0;
  color: #838aa2;
  font-size: 0.65rem;
}

.generation-complete__stats i {
  position: absolute;
  top: 50%;
  right: -4px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #e96b55;
  transform: translateY(-50%);
}

.generation-complete__preview {
  display: flex;
  min-height: 60px;
  align-items: center;
  justify-content: center;
  gap: 20px;
  border: 4px solid #202796;
  border-radius: 17px;
  color: #fff;
  background: #e96b55;
  box-shadow: 6px 7px 0 #202796;
  font-size: 1.05rem;
  font-weight: 800;
}

.generation-complete__preview span {
  font-size: 1.55rem;
  font-weight: 400;
}

.generation-complete__edit {
  display: inline-flex;
  margin-top: 14px;
  color: #202796;
  font-size: 0.82rem;
  font-weight: 800;
}

@keyframes generation-progress-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 420px) {
  .generation-progress-page {
    padding-inline: 8px;
  }

  .generation-progress-visual {
    margin-inline: -8px;
  }

  .generation-progress-steps li {
    padding-inline: 11px;
  }

  .generation-complete__visual {
    height: 278px;
    margin-inline: -8px;
  }

  .generation-complete__photo {
    width: 164px;
    height: 214px;
  }

  .generation-complete__photo--1 {
    transform: translateX(-101%) rotate(-7deg);
  }

  .generation-complete__photo--2 {
    transform: translateX(1%) rotate(6deg);
  }

  .generation-complete__check {
    width: 62px;
    height: 62px;
    font-size: 2rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .generation-progress-step--active .generation-progress-step__marker {
    animation: none;
  }
}
</style>
