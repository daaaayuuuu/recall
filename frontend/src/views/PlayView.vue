<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'

import { APIError } from '@/api/client'
import { trackPlayEvent } from '@/analytics/tracker'
import {
  createPlaySession,
  getGameConfig,
  resolvePublicShare,
  type GameConfig,
  type PublicShare,
} from '@/api/sharing'
import GamePlayer from '@/components/GamePlayer.vue'

const props = defineProps<{ publicId: string }>()
const phase = ref<'loading' | 'intro' | 'playing' | 'completed' | 'ended'>('loading')
const publicShare = ref<PublicShare | null>(null)
const gameConfig = ref<GameConfig | null>(null)
const starting = ref(false)
const endedMessage = ref('这份游戏分享已经结束')
let secret = ''

onMounted(initialize)

async function initialize() {
  const fragment = new globalThis.URLSearchParams(globalThis.location.hash.slice(1))
  secret = fragment.get('t') ?? ''
  if (secret) {
    globalThis.history.replaceState(null, '', `${globalThis.location.pathname}${globalThis.location.search}`)
    try {
      publicShare.value = await resolvePublicShare(props.publicId, secret)
      phase.value = 'intro'
      return
    } catch (error) {
      endWith(error)
      return
    }
  }
  try {
    gameConfig.value = await getGameConfig()
    phase.value = 'playing'
  } catch (error) {
    endWith(error)
  }
}

async function startGame() {
  if (!secret) return
  starting.value = true
  try {
    await createPlaySession(props.publicId, secret)
    secret = ''
    gameConfig.value = await getGameConfig()
    phase.value = 'playing'
  } catch (error) {
    endWith(error)
  } finally {
    starting.value = false
  }
}

function completeGame() {
  if (phase.value !== 'playing') return
  phase.value = 'completed'
  void trackPlayEvent('play.completed')
}

function playAgain() {
  if (phase.value !== 'completed') return
  phase.value = 'playing'
  void trackPlayEvent('play.replayed')
}

function endWith(error: unknown) {
  if (error instanceof APIError) {
    endedMessage.value = error.message
  } else {
    ElMessage.error('游戏暂时无法加载')
    endedMessage.value = '游戏暂时无法加载，请稍后再试'
  }
  phase.value = 'ended'
}
</script>

<template>
  <section
    class="panel panel--narrow play-card"
    :class="{ 'play-card--game': phase === 'playing' }"
  >
    <template v-if="phase === 'loading'">
      <p class="eyebrow">
        正在加载
      </p>
      <h1>正在打开朋友分享的游戏…</h1>
    </template>

    <template v-else-if="phase === 'intro' && publicShare">
      <p class="eyebrow">
        RECALL
      </p>
      <h1>这是「{{ publicShare.creator.displayName }}」分享给你的游戏</h1>
      <h2>{{ publicShare.game.title }}</h2>
      <p>本次分享有效期至 {{ new Date(publicShare.share.expiresAt).toLocaleString() }}</p>
      <el-button
        type="primary"
        :loading="starting"
        @click="startGame"
      >
        开始游戏
      </el-button>
    </template>

    <template v-else-if="phase === 'playing' && gameConfig">
      <GamePlayer
        :game-config="gameConfig"
        mode="public"
        @complete="completeGame"
      />
    </template>

    <template v-else-if="phase === 'completed'">
      <p class="eyebrow">
        游戏完成
      </p>
      <h1>这段回忆已经好好收藏</h1>
      <p>在本局 30 分钟会话到期前，你还可以再玩一次。</p>
      <el-button
        type="primary"
        @click="playAgain"
      >
        再玩一次
      </el-button>
    </template>

    <template v-else>
      <p class="eyebrow">
        分享已结束
      </p>
      <h1>{{ endedMessage }}</h1>
      <p>你可以联系分享者重新发送一个新链接。</p>
    </template>
  </section>
</template>
