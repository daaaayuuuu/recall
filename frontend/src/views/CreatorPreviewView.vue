<script setup lang="ts">
import { onMounted, ref } from 'vue'

import { APIError } from '@/api/client'
import { getGamePreview, type CreatorPreview } from '@/api/games'
import GamePlayer from '@/components/GamePlayer.vue'

const props = defineProps<{ gameId: string; versionId: string }>()
const phase = ref<'loading' | 'playing' | 'completed' | 'error'>('loading')
const preview = ref<CreatorPreview | null>(null)
const previewSkipRequest = ref(0)
const errorMessage = ref('游戏暂时无法加载，请稍后再试')

onMounted(loadPreview)

async function loadPreview() {
  if (!props.versionId) {
    errorMessage.value = '缺少要试玩的游戏版本'
    phase.value = 'error'
    return
  }
  phase.value = 'loading'
  try {
    preview.value = await getGamePreview(props.gameId, props.versionId)
    phase.value = 'playing'
  } catch (error) {
    errorMessage.value = error instanceof APIError ? error.message : '游戏暂时无法加载，请稍后再试'
    phase.value = 'error'
  }
}

function completeGame() {
  phase.value = 'completed'
}

function skipCurrentScene() {
  previewSkipRequest.value += 1
}

function playAgain() {
  previewSkipRequest.value = 0
  phase.value = 'playing'
}
</script>

<template>
  <section class="creator-preview">
    <header class="preview-toolbar">
      <div>
        <RouterLink
          class="back-link"
          :to="{ name: 'games' }"
        >
          ← 返回我的游戏
        </RouterLink>
        <p class="eyebrow">
          私密试玩
        </p>
        <h1>{{ preview?.game.title ?? '游戏试玩' }}</h1>
      </div>
      <el-tag v-if="preview">
        版本 {{ preview.version.versionNumber }}
      </el-tag>
    </header>

    <article
      class="panel panel--narrow preview-stage"
      :class="{ 'preview-stage--game': phase === 'playing' }"
    >
      <template v-if="phase === 'loading'">
        <p class="eyebrow">
          正在加载
        </p>
        <h2>正在准备试玩版本…</h2>
      </template>

      <template v-else-if="phase === 'playing' && preview">
        <div
          class="preview-stage__controls"
          role="group"
          aria-label="试玩控制"
          data-screenshot-exclude="true"
        >
          <button
            class="preview-stage__control preview-stage__skip"
            type="button"
            title="跳过当前场景"
            aria-label="跳过当前场景"
            @click="skipCurrentScene"
          >
            跳过 <span aria-hidden="true">→</span>
          </button>
          <RouterLink
            class="preview-stage__control preview-stage__exit"
            :to="{ name: 'games' }"
            title="退出试玩"
            aria-label="退出试玩"
          >
            退出 <span aria-hidden="true">×</span>
          </RouterLink>
        </div>
        <GamePlayer
          :game-config="preview"
          mode="creator-preview"
          :preview-skip-request="previewSkipRequest"
          @complete="completeGame"
        />
      </template>

      <template v-else-if="phase === 'completed'">
        <p class="eyebrow">
          试玩完成
        </p>
        <h2>这个版本已经完整跑通</h2>
        <p>你可以再次试玩，或返回“我的游戏”从卡片创建分享链接。</p>
        <div class="preview-stage__actions">
          <el-button
            type="primary"
            @click="playAgain"
          >
            再玩一次
          </el-button>
          <RouterLink
            class="secondary-link"
            :to="{ name: 'games' }"
          >
            返回我的游戏
          </RouterLink>
        </div>
      </template>

      <template v-else>
        <p class="eyebrow">
          无法试玩
        </p>
        <h2>{{ errorMessage }}</h2>
        <p>只有属于你且已经创建完成的版本可以进入私密试玩。</p>
        <RouterLink
          class="secondary-link"
          :to="{ name: 'games' }"
        >
          返回我的游戏
        </RouterLink>
      </template>
    </article>
  </section>
</template>
