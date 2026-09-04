<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

import { getGenerationRun, type GenerationRun } from '@/api/generation'
import { getGamePreview, listAssets, type CreatorPreview, type GameAsset } from '@/api/games'
import GenerationProgressScreen from '@/components/GenerationProgressScreen.vue'

const props = defineProps<{ gameId: string; runId: string }>()
const run = ref<GenerationRun | null>(null)
const preview = ref<CreatorPreview | null>(null)
const sourceAssets = ref<GameAsset[]>([])
const loadError = ref('')
let pollTimer: ReturnType<typeof globalThis.setTimeout> | undefined
let loadedCompletionVersionId = ''

onMounted(refresh)
onUnmounted(() => {
  if (pollTimer) globalThis.clearTimeout(pollTimer)
})

function schedulePoll() {
  if (pollTimer) globalThis.clearTimeout(pollTimer)
  if (!run.value || !['queued', 'running'].includes(run.value.status)) return
  pollTimer = globalThis.setTimeout(refresh, 1200)
}

async function refresh() {
  try {
    run.value = await getGenerationRun(props.gameId, props.runId)
    loadError.value = ''
    if (run.value.status === 'succeeded') await loadCompletion(run.value.gameVersionId)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '暂时无法读取制作进度'
  } finally {
    schedulePoll()
  }
}

async function loadCompletion(versionId: string) {
  if (!versionId || loadedCompletionVersionId === versionId) return
  loadedCompletionVersionId = versionId
  const [previewResult, assetResult] = await Promise.allSettled([
    getGamePreview(props.gameId, versionId),
    listAssets(props.gameId, versionId),
  ])
  if (previewResult.status === 'fulfilled') preview.value = previewResult.value
  if (assetResult.status === 'fulfilled') sourceAssets.value = assetResult.value.items
}
</script>

<template>
  <GenerationProgressScreen
    :run="run"
    :load-error="loadError"
    :game-id="gameId"
    :preview="preview"
    :source-assets="sourceAssets"
  />
</template>
