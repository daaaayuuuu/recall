<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'

import {
  listAdminBehaviorEvents,
  type AdminBehaviorEvent,
  type AdminBehaviorEventFilters,
  type BehaviorEventName,
  type BehaviorEventSource,
} from '@/api/analytics'

const EVENT_NAMES: readonly BehaviorEventName[] = [
  'creator.page_viewed',
  'creator.registered',
  'creator.logged_in',
  'game.created',
  'game.version_created',
  'asset.uploaded',
  'generation.submitted',
  'generation.succeeded',
  'generation.failed',
  'share.created',
  'share.opened',
  'play.started',
  'play.completed',
  'play.replayed',
]

type FilterForm = {
  eventName: BehaviorEventName | ''
  creatorId: string
  loginId: string
  gameId: string
  source: BehaviorEventSource | ''
  from: string
  to: string
}

const emptyFilters = (): FilterForm => ({
  eventName: '',
  creatorId: '',
  loginId: '',
  gameId: '',
  source: '',
  from: '',
  to: '',
})

const filters = reactive<FilterForm>(emptyFilters())
const events = ref<AdminBehaviorEvent[]>([])
const nextCursor = ref<string | null>(null)
const loading = ref(false)
const loadingMore = ref(false)
const errorMessage = ref('')
let requestVersion = 0

const isEmpty = computed(() => !loading.value && !errorMessage.value && events.value.length === 0)

onMounted(() => loadEvents(false))

function invalidateResults() {
  requestVersion += 1
  events.value = []
  nextCursor.value = null
  errorMessage.value = ''
  loading.value = false
  loadingMore.value = false
}

function toTimestamp(value: string) {
  if (!value) return undefined
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toISOString()
}

function queryFilters(cursor?: string): AdminBehaviorEventFilters {
  return {
    eventName: filters.eventName || undefined,
    creatorId: filters.creatorId.trim() || undefined,
    loginId: filters.loginId.trim() || undefined,
    gameId: filters.gameId.trim() || undefined,
    source: filters.source || undefined,
    from: toTimestamp(filters.from),
    to: toTimestamp(filters.to),
    cursor,
    limit: 50,
  }
}

async function loadEvents(append: boolean) {
  if (append && !nextCursor.value) return
  const version = ++requestVersion
  errorMessage.value = ''
  if (append) loadingMore.value = true
  else loading.value = true

  try {
    const page = await listAdminBehaviorEvents(queryFilters(append ? nextCursor.value ?? undefined : undefined))
    if (version !== requestVersion) return
    if (append) {
      const known = new Set(events.value.map((event) => event.id))
      events.value = [...events.value, ...page.items.filter((event) => !known.has(event.id))]
    } else {
      events.value = page.items
    }
    nextCursor.value = page.nextCursor
  } catch (error) {
    if (version !== requestVersion) return
    errorMessage.value = error instanceof Error ? error.message : '行为记录加载失败，请稍后重试'
  } finally {
    if (version === requestVersion) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

function applyFilters() {
  invalidateResults()
  void loadEvents(false)
}

function resetFilters() {
  Object.assign(filters, emptyFilters())
  invalidateResults()
  void loadEvents(false)
}

function formatDate(value: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN', { hour12: false })
}

type DisplayProperty = { label: string; value: string }

function pickProperty(
  properties: Record<string, unknown>,
  key: string,
  label: string,
): DisplayProperty | null {
  const value = properties[key]
  if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') return null
  return { label, value: String(value) }
}

function visibleProperties(event: AdminBehaviorEvent): DisplayProperty[] {
  const fields: Partial<Record<BehaviorEventName, readonly [string, string][]>> = {
    'creator.page_viewed': [['page', '页面']],
    'game.created': [['templateId', '模板 ID']],
    'game.version_created': [['versionNumber', '版本号'], ['templateId', '模板 ID']],
    'asset.uploaded': [['kind', '素材类型'], ['mimeType', 'MIME 类型'], ['sizeBytes', '大小（字节）']],
    'generation.submitted': [['attemptNumber', '尝试次数'], ['deduplicated', '幂等复用']],
    'generation.succeeded': [['executionCount', '执行次数']],
    'generation.failed': [['errorCode', '错误代码'], ['retryable', '可重试'], ['executionCount', '执行次数']],
    'share.created': [['lifetimeDays', '有效天数']],
    'play.completed': [['mode', '游玩模式']],
    'play.replayed': [['mode', '游玩模式']],
  }
  return (fields[event.eventName] ?? [])
    .map(([key, label]) => pickProperty(event.properties, key, label))
    .filter((property): property is DisplayProperty => property !== null)
}
</script>

<template>
  <section
    class="admin-page behavior-events-page"
    :aria-busy="loading || loadingMore"
  >
    <header class="section-header admin-page__header">
      <div>
        <p class="eyebrow">
          平台管理
        </p>
        <h1>用户行为记录</h1>
        <p class="section-copy">
          按服务端接收时间倒序查看脱敏事件；详情仅展示冻结契约中的白名单属性。
        </p>
      </div>
    </header>

    <form
      class="panel behavior-filters"
      aria-label="行为记录筛选"
      @submit.prevent="applyFilters"
    >
      <label>
        <span>事件类型</span>
        <select
          v-model="filters.eventName"
          data-testid="event-name-filter"
          @change="invalidateResults"
        >
          <option value="">全部事件</option>
          <option
            v-for="eventName in EVENT_NAMES"
            :key="eventName"
            :value="eventName"
          >{{ eventName }}</option>
        </select>
      </label>
      <label>
        <span>Creator ID</span>
        <input
          v-model="filters.creatorId"
          data-testid="creator-id-filter"
          autocomplete="off"
          placeholder="内部 ULID"
          @input="invalidateResults"
        >
      </label>
      <label>
        <span>登录 ID</span>
        <input
          v-model="filters.loginId"
          data-testid="login-id-filter"
          autocomplete="off"
          placeholder="精确匹配"
          @input="invalidateResults"
        >
      </label>
      <label>
        <span>游戏 ID</span>
        <input
          v-model="filters.gameId"
          data-testid="game-id-filter"
          autocomplete="off"
          placeholder="游戏 ULID"
          @input="invalidateResults"
        >
      </label>
      <label>
        <span>来源</span>
        <select
          v-model="filters.source"
          data-testid="source-filter"
          @change="invalidateResults"
        >
          <option value="">全部来源</option>
          <option value="frontend">frontend</option>
          <option value="api">api</option>
          <option value="worker">worker</option>
        </select>
      </label>
      <label>
        <span>开始时间（含）</span>
        <input
          v-model="filters.from"
          data-testid="from-filter"
          type="datetime-local"
          @input="invalidateResults"
        >
      </label>
      <label>
        <span>结束时间（不含）</span>
        <input
          v-model="filters.to"
          data-testid="to-filter"
          type="datetime-local"
          @input="invalidateResults"
        >
      </label>
      <div class="behavior-filters__actions">
        <button
          class="behavior-button behavior-button--primary"
          type="submit"
          data-testid="apply-filters"
        >
          查询
        </button>
        <button
          class="behavior-button"
          type="button"
          data-testid="reset-filters"
          @click="resetFilters"
        >
          重置筛选
        </button>
      </div>
    </form>

    <div
      v-if="loading"
      class="panel loading-panel behavior-state"
      role="status"
    >
      正在加载行为记录…
    </div>
    <div
      v-else-if="errorMessage"
      class="panel behavior-state behavior-state--error"
      role="alert"
    >
      <strong>行为记录加载失败</strong>
      <span>{{ errorMessage }}</span>
      <button
        class="behavior-button"
        type="button"
        data-testid="retry-events"
        @click="loadEvents(false)"
      >
        重试
      </button>
    </div>
    <div
      v-else-if="isEmpty"
      class="panel behavior-state"
      role="status"
    >
      当前没有符合条件的行为记录
    </div>
    <div
      v-else
      class="behavior-event-list"
      aria-label="行为记录列表"
    >
      <article
        v-for="event in events"
        :key="event.id"
        class="panel behavior-event-card"
      >
        <header class="behavior-event-card__header">
          <strong>{{ event.eventName }}</strong>
          <span class="behavior-source">{{ event.source }}</span>
        </header>
        <dl class="behavior-event-card__summary">
          <div><dt>发生时间</dt><dd><time>{{ formatDate(event.occurredAt || event.createdAt) }}</time></dd></div>
          <div><dt>行为主体</dt><dd><code>{{ event.loginId || event.creatorId || event.actorType }}</code></dd></div>
          <div><dt>Creator ID</dt><dd><code>{{ event.creatorId || '—' }}</code></dd></div>
          <div><dt>登录 ID</dt><dd><code>{{ event.loginId || '—' }}</code></dd></div>
          <div><dt>游戏</dt><dd><code>{{ event.gameId || '—' }}</code></dd></div>
          <div><dt>Request ID</dt><dd><code>{{ event.requestId || '—' }}</code></dd></div>
        </dl>
        <dl
          v-if="visibleProperties(event).length"
          class="behavior-event-card__properties"
          aria-label="白名单事件详情"
        >
          <div
            v-for="property in visibleProperties(event)"
            :key="property.label"
          >
            <dt>{{ property.label }}</dt>
            <dd>{{ property.value }}</dd>
          </div>
        </dl>
      </article>

      <button
        v-if="nextCursor"
        class="behavior-button behavior-load-more"
        type="button"
        data-testid="load-more-events"
        :disabled="loadingMore"
        @click="loadEvents(true)"
      >
        {{ loadingMore ? '正在加载…' : '加载更多' }}
      </button>
    </div>
  </section>
</template>
