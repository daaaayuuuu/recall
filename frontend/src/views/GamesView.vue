<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'

import { deleteGame, listGames, type Game } from '@/api/games'
import { useAuthStore } from '@/stores/auth'
import { confirmDestructiveAction } from '@/utils/confirm'

const authStore = useAuthStore()
const games = ref<Game[]>([])
const loading = ref(true)
const deletingGameIds = ref<Set<string>>(new Set())

onMounted(async () => {
  try {
    games.value = (await listGames()).items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '游戏列表加载失败')
  } finally {
    loading.value = false
  }
})

const statusLabels: Record<Game['status'], string> = {
  draft: '草稿',
  queued: '等待创建',
  generating: '创建中',
  ready: '已完成',
  failed: '创建失败',
}

function canModify(game: Game) {
  return Boolean(
    game.currentVersionId
    && game.status !== 'queued'
    && game.status !== 'generating',
  )
}

async function confirmDelete(game: Game) {
  try {
    await confirmDestructiveAction(
      `删除后，“${game.title}”的所有版本、素材和分享链接都将永久失效。`,
      '确认删除这个游戏？',
      {
        confirmButtonText: '确认删除',
        cancelButtonText: '取消',
      },
    )
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error instanceof Error ? error.message : '无法打开删除确认')
    return
  }

  deletingGameIds.value = new Set([...deletingGameIds.value, game.id])
  try {
    const csrfToken = await authStore.ensureCSRF()
    await deleteGame(game.id, csrfToken)
    games.value = games.value.filter((item) => item.id !== game.id)
    ElMessage.success('游戏已删除')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '删除游戏失败')
  } finally {
    const next = new Set(deletingGameIds.value)
    next.delete(game.id)
    deletingGameIds.value = next
  }
}
</script>

<template>
  <section>
    <div
      v-if="loading"
      class="panel loading-panel"
    >
      正在加载游戏…
    </div>
    <el-empty
      v-else-if="games.length === 0"
      description="还没有游戏草稿"
    >
      <RouterLink to="/app/create">
        创建第一个游戏
      </RouterLink>
    </el-empty>
    <div
      v-else
      class="game-grid"
    >
      <article
        v-for="game in games"
        :key="game.id"
        class="game-card"
      >
        <div class="game-card__content">
          <div class="game-card__body">
            <div class="game-card__meta">
              <el-tag size="small">
                {{ statusLabels[game.status] }}
              </el-tag>
              <span>{{ game.assetCount }} 张图片</span>
            </div>
            <h2>{{ game.title }}</h2>
            <p>{{ game.description || '还没有填写游戏描述' }}</p>
            <time :datetime="game.updatedAt">更新于 {{ new Date(game.updatedAt).toLocaleString() }}</time>
          </div>
        </div>
        <div class="game-card__actions">
          <RouterLink
            v-if="game.status === 'ready' && game.currentVersionId"
            class="game-card__action game-card__action--primary"
            :to="{ name: 'game-preview', params: { gameId: game.id }, query: { versionId: game.currentVersionId } }"
          >
            试玩
          </RouterLink>
          <button
            v-else
            class="game-card__action game-card__action--primary"
            type="button"
            disabled
          >
            试玩
          </button>

          <RouterLink
            v-if="canModify(game)"
            class="game-card__action"
            :to="{ name: 'game-edit', params: { gameId: game.id } }"
          >
            修改
          </RouterLink>
          <button
            v-else
            class="game-card__action"
            type="button"
            disabled
          >
            修改
          </button>

          <RouterLink
            v-if="game.status === 'ready'"
            class="game-card__action game-card__action--icon game-card__action--share"
            :to="{ name: 'game-share', params: { gameId: game.id } }"
            :aria-label="`分享${game.title}`"
            title="分享游戏"
          >
            <svg
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <circle
                cx="18"
                cy="5"
                r="2.5"
              />
              <circle
                cx="6"
                cy="12"
                r="2.5"
              />
              <circle
                cx="18"
                cy="19"
                r="2.5"
              />
              <path d="m8.2 10.8 7.6-4.5M8.2 13.2l7.6 4.5" />
            </svg>
          </RouterLink>
          <button
            v-else
            class="game-card__action game-card__action--icon game-card__action--share"
            type="button"
            disabled
            :aria-label="`分享${game.title}`"
            title="当前暂不可分享"
          >
            <svg
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <circle
                cx="18"
                cy="5"
                r="2.5"
              />
              <circle
                cx="6"
                cy="12"
                r="2.5"
              />
              <circle
                cx="18"
                cy="19"
                r="2.5"
              />
              <path d="m8.2 10.8 7.6-4.5M8.2 13.2l7.6 4.5" />
            </svg>
          </button>

          <button
            class="game-card__action game-card__action--icon game-card__action--delete"
            type="button"
            :disabled="deletingGameIds.has(game.id)"
            :aria-label="`删除${game.title}`"
            title="删除游戏"
            @click="confirmDelete(game)"
          >
            <svg
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path d="M4 7h16M9 7V4h6v3M7 7l1 13h8l1-13M10 11v5M14 11v5" />
            </svg>
          </button>
        </div>
      </article>
    </div>
  </section>
</template>
