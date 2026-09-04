<script setup lang="ts">
import { markRaw, ref, shallowRef, watch } from 'vue'

import type { PlayableGameConfig } from '@/api/gameplay'
import { findTemplate } from '@/game-runtime/templates/registry'
import LegacyPlaceholderTemplate from '@/game-runtime/templates/LegacyPlaceholderTemplate.vue'
import type { GameRuntimeMode } from '@/game-runtime/types'

const props = defineProps<{
  gameConfig: PlayableGameConfig
  mode: GameRuntimeMode
  previewSkipRequest?: number
}>()

const emit = defineEmits<{ complete: [] }>()
const templateComponent = shallowRef(markRaw(LegacyPlaceholderTemplate))
const status = ref<'loading' | 'ready' | 'unsupported' | 'error'>('loading')
let loadRevision = 0

watch(
  () => [props.gameConfig.templateId, props.gameConfig.templateVersion] as const,
  async ([templateId, templateVersion]) => {
    const revision = ++loadRevision

    if (templateId === 'memory-game' && templateVersion === '1.0.0') {
      templateComponent.value = markRaw(LegacyPlaceholderTemplate)
      status.value = 'ready'
      return
    }

    const definition = findTemplate(templateId, templateVersion)
    if (!definition) {
      status.value = 'unsupported'
      return
    }

    status.value = 'loading'
    try {
      const loaded = await definition.load()
      if (revision !== loadRevision) return
      templateComponent.value = markRaw(loaded.default)
      status.value = 'ready'
    } catch {
      if (revision === loadRevision) status.value = 'error'
    }
  },
  { immediate: true },
)
</script>

<template>
  <div
    class="game-player"
    :class="{ 'game-player--template': gameConfig.templateId === 'love-journey' }"
  >
    <component
      :is="templateComponent"
      v-if="status === 'ready'"
      :game-config="gameConfig"
      :mode="mode"
      :preview-skip-request="previewSkipRequest"
      @complete="emit('complete')"
    />

    <div
      v-else
      class="game-player__notice"
      role="status"
    >
      <p class="eyebrow">
        {{ status === 'loading' ? '正在加载' : '无法加载' }}
      </p>
      <h1 v-if="status === 'loading'">
        正在准备游戏模板…
      </h1>
      <h1 v-else-if="status === 'unsupported'">
        不支持这个游戏模板版本
      </h1>
      <h1 v-else>
        游戏模板加载失败
      </h1>
      <p v-if="status !== 'loading'">
        请联系分享者重新生成游戏，或稍后再试。
      </p>
    </div>
  </div>
</template>

<style scoped>
.game-player__notice {
  display: grid;
  gap: 14px;
  padding: 32px 20px;
  text-align: center;
}

.game-player__notice h1,
.game-player__notice p {
  margin: 0;
}
</style>
