<script setup lang="ts">
import { ref } from 'vue'

import type { PlayableGameConfig } from '@/api/gameplay'
import GamePlayer from '@/components/GamePlayer.vue'

const publicMode = new globalThis.URLSearchParams(globalThis.location.search).get('mode') === 'public'
const previewSkipRequest = ref(0)
const previewConfig: PlayableGameConfig = {
  templateId: 'love-journey',
  templateVersion: '1.1.0',
  configVersion: 1,
  config: {
    openingTitle: '我们的爱的旅程',
    rounds: [],
    loveLetter: '谢谢你走进我的生活，也谢谢你陪我把平凡的日子变成值得珍藏的故事。未来的旅程，还想继续和你一起慢慢走。',
    letterPassword: '2580',
    passwordHint: '我们第一次见面的日期',
  },
  assets: [
    {
      key: 'render-1',
      type: 'image',
      url: '/dev-assets/couple-photo.svg',
      mimeType: 'image/svg+xml',
      expiresAt: '2099-12-31T23:59:59Z',
    },
    {
      key: 'render-2',
      type: 'image',
      url: '/dev-assets/travel-photo-1.svg',
      mimeType: 'image/svg+xml',
      expiresAt: '2099-12-31T23:59:59Z',
    },
    {
      key: 'render-3',
      type: 'image',
      url: '/dev-assets/travel-photo-2.svg',
      mimeType: 'image/svg+xml',
      expiresAt: '2099-12-31T23:59:59Z',
    },
  ],
}

function restartPreview() {
  globalThis.location.reload()
}

function skipCurrentScene() {
  previewSkipRequest.value += 1
}
</script>

<template>
  <main
    class="template-dev-preview"
    :class="publicMode ? 'play-card--game' : 'preview-stage--game'"
  >
    <div
      v-if="!publicMode"
      class="preview-stage__controls"
      role="group"
      aria-label="试玩控制"
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
      <button
        class="preview-stage__control preview-stage__exit"
        type="button"
        title="退出试玩"
        aria-label="退出试玩"
        @click="restartPreview"
      >
        退出 <span aria-hidden="true">×</span>
      </button>
    </div>
    <GamePlayer
      :game-config="previewConfig"
      :mode="publicMode ? 'public' : 'creator-preview'"
      :preview-skip-request="previewSkipRequest"
      @complete="restartPreview"
    />
  </main>
</template>

<style scoped>
.template-dev-preview {
  min-height: 100dvh;
  background: #fff;
}
</style>
