<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'

import {
  listAdminGenerationRuns,
  type AdminGenerationRun,
  type GenerationStatus,
} from '@/api/generation'

const runs = ref<AdminGenerationRun[]>([])
const loading = ref(true)
const status = ref<GenerationStatus | ''>('failed')

onMounted(loadRuns)

async function loadRuns() {
  loading.value = true
  try {
    runs.value = (await listAdminGenerationRuns(status.value)).items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '创建记录加载失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section
    class="admin-page"
    :aria-busy="loading"
  >
    <header class="section-header admin-page__header">
      <div>
        <p class="eyebrow">
          平台管理
        </p>
        <h1>创建任务诊断</h1>
        <p class="section-copy">
          这里只显示错误代码和白名单诊断字段，不展示用户回忆文本、图片或对象地址。
        </p>
      </div>
      <div class="admin-filter">
        <span
          id="admin-status-filter-label"
          class="admin-filter__label"
        >任务状态</span>
        <el-select
          v-model="status"
          aria-labelledby="admin-status-filter-label"
          @change="loadRuns"
        >
          <el-option
            label="失败任务"
            value="failed"
          />
          <el-option
            label="全部任务"
            value=""
          />
          <el-option
            label="排队中"
            value="queued"
          />
          <el-option
            label="运行中"
            value="running"
          />
          <el-option
            label="已成功"
            value="succeeded"
          />
          <el-option
            label="已取消"
            value="cancelled"
          />
        </el-select>
      </div>
    </header>

    <div
      v-if="loading"
      class="panel loading-panel"
    >
      正在加载创建记录…
    </div>
    <el-empty
      v-else-if="runs.length === 0"
      description="当前没有符合条件的创建记录"
    />
    <div
      v-else
      class="admin-run-list"
      aria-label="创建任务列表"
    >
      <article
        v-for="run in runs"
        :key="run.id"
        class="panel admin-run-card"
      >
        <header class="admin-run-card__header">
          <div class="admin-run-card__heading">
            <strong class="admin-run-card__title">{{ run.errorCode || run.status }}</strong>
            <span class="admin-run-card__id">任务 <code>{{ run.id }}</code></span>
          </div>
          <el-tag :type="run.status === 'failed' ? 'danger' : run.status === 'succeeded' ? 'success' : undefined">
            {{ run.status }}
          </el-tag>
        </header>
        <dl class="admin-run-card__details">
          <div><dt>游戏</dt><dd><code>{{ run.gameId }}</code></dd></div>
          <div><dt>版本</dt><dd><code>{{ run.gameVersionId }}</code></dd></div>
          <div><dt>执行次数</dt><dd>{{ run.executionCount }}</dd></div>
          <div><dt>Trace ID</dt><dd><code>{{ run.traceId }}</code></dd></div>
          <div><dt>说明</dt><dd>{{ run.adminMessage || '—' }}</dd></div>
        </dl>
        <pre
          v-if="run.sanitizedDetails"
          tabindex="0"
          :aria-label="`任务 ${run.id} 的诊断详情`"
        >{{ JSON.stringify(run.sanitizedDetails, null, 2) }}</pre>
      </article>
    </div>
  </section>
</template>
