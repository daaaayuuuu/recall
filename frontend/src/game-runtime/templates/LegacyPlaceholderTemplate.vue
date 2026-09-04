<script setup lang="ts">
import { watch } from 'vue'

import type { GameTemplateProps } from '@/game-runtime/types'

const props = defineProps<GameTemplateProps>()
const emit = defineEmits<{ complete: [] }>()

watch(
  () => props.previewSkipRequest,
  (request, previousRequest) => {
    if (props.mode !== 'creator-preview' || !request || request === previousRequest) return
    emit('complete')
  },
  { immediate: true },
)
</script>

<template>
  <div class="legacy-game-placeholder">
    <p class="eyebrow">
      {{ mode === 'creator-preview' ? '制作方私密试玩' : '回忆游戏' }}
    </p>
    <h1>{{ gameConfig.config.openingTitle }}</h1>
    <p>这个历史游戏仍使用早期占位模板。</p>
    <div
      v-if="gameConfig.assets.length > 0"
      class="play-assets"
    >
      <img
        v-for="asset in gameConfig.assets"
        :key="asset.key"
        :src="asset.url"
        :alt="asset.key"
      >
    </div>
    <el-alert
      v-else-if="mode === 'creator-preview'"
      title="当前版本没有游戏画面资源。"
      type="info"
      :closable="false"
      show-icon
    />
    <el-button
      type="primary"
      @click="$emit('complete')"
    >
      完成这一局
    </el-button>
  </div>
</template>

<style scoped>
.legacy-game-placeholder {
  display: grid;
  gap: 18px;
  text-align: center;
}

.legacy-game-placeholder h1,
.legacy-game-placeholder p {
  margin: 0;
}
</style>
