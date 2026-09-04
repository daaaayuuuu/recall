<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, ref } from 'vue'

import {
  getAdminAISettings,
  testAdminAIConnection,
  updateAdminAISettings,
  type AICapability,
  type AdminAISettings,
  type AISettingsSnapshot,
  type APIKeyMutations,
} from '@/api/aiSettings'
import { useAdminAuthStore } from '@/stores/auth'

const adminStore = useAdminAuthStore()
const loading = ref(true)
const saving = ref(false)
const testing = ref<AICapability | null>(null)
const view = ref<AdminAISettings | null>(null)
const settings = ref<AISettingsSnapshot | null>(null)
const apiKeys = ref<APIKeyMutations>(emptyKeyMutations())

const canEdit = computed(() => Boolean(view.value?.dynamicEnabled && settings.value))

onMounted(load)

function emptyKeyMutations(): APIKeyMutations {
  return {
    text: { value: '', clear: false },
    imageModeration: { value: '', clear: false },
    imageToImage: { value: '', clear: false },
  }
}

function copySettings(value: AISettingsSnapshot): AISettingsSnapshot {
  return JSON.parse(JSON.stringify(value)) as AISettingsSnapshot
}

async function load() {
  loading.value = true
  try {
    const result = await getAdminAISettings()
    applyView(result)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : 'AI 配置加载失败')
  } finally {
    loading.value = false
  }
}

function applyView(result: AdminAISettings) {
  view.value = result
  settings.value = copySettings(result.settings)
  apiKeys.value = emptyKeyMutations()
}

async function save() {
  if (!view.value || !settings.value || !canEdit.value) return
  saving.value = true
  try {
    const csrf = await adminStore.ensureCSRF()
    const result = await updateAdminAISettings(view.value.version, settings.value, apiKeys.value, csrf)
    applyView(result)
    ElMessage.success(`AI 配置版本 ${result.version} 已发布`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : 'AI 配置保存失败')
  } finally {
    saving.value = false
  }
}

async function testConnection(capability: AICapability) {
  if (!settings.value || !canEdit.value) return
  testing.value = capability
  try {
    const csrf = await adminStore.ensureCSRF()
    const result = await testAdminAIConnection(capability, settings.value, apiKeys.value, csrf)
    ElMessage.success(`连接测试成功，耗时 ${result.latencyMs} ms`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : 'AI 服务连接测试失败')
  } finally {
    testing.value = null
  }
}

function keyDescription(capability: keyof APIKeyMutations) {
  const status = view.value?.apiKeys[capability]
  if (!status?.configured) return '尚未配置'
  return `${status.hint || '已配置'} · 来源：${status.source === 'admin' ? '管理员配置' : '环境变量'}`
}

function acceptNewKey(capability: keyof APIKeyMutations) {
  apiKeys.value[capability].clear = false
}
</script>

<template>
  <section
    class="admin-page ai-settings-page"
    :aria-busy="loading"
  >
    <header class="section-header admin-page__header">
      <div>
        <p class="eyebrow">
          平台管理
        </p>
        <h1>AI 配置</h1>
        <p class="section-copy">
          发布后的设置会自动同步到 API 和 Worker；正在执行的任务继续使用开始时的配置版本。
        </p>
      </div>
      <div
        v-if="view"
        class="ai-settings-version"
      >
        <el-tag :type="view.source === 'admin' ? 'success' : 'info'">
          {{ view.source === 'admin' ? `版本 ${view.version}` : 'ENV 基线' }}
        </el-tag>
        <small v-if="view.updatedAt">
          {{ view.updatedBy }} · {{ new Date(view.updatedAt).toLocaleString() }}
        </small>
      </div>
    </header>

    <div
      v-if="loading"
      class="panel loading-panel"
    >
      正在加载 AI 配置…
    </div>

    <template v-else-if="view && settings">
      <el-alert
        v-if="!view.dynamicEnabled"
        type="warning"
        :closable="false"
        title="动态 AI 配置未启用"
        description="请先在部署环境中设置 DYNAMIC_AI_CONFIG_ENABLED=true 和独立的 AI_CONFIG_ENCRYPTION_KEY_V1。当前只展示 ENV 基线。"
      />

      <div class="ai-settings-grid">
        <article class="panel ai-settings-card">
          <header class="ai-settings-card__header">
            <div>
              <p class="eyebrow">
                Text
              </p>
              <h2>文本润色</h2>
            </div>
            <label class="ai-enabled"><input
              v-model="settings.text.enabled"
              type="checkbox"
              :disabled="!canEdit"
            > 启用</label>
          </header>
          <div class="ai-form-grid">
            <label>Provider<input
              v-model="settings.text.provider"
              :disabled="!canEdit"
              autocomplete="off"
            ></label>
            <label>模型<input
              v-model="settings.text.model"
              :disabled="!canEdit"
              autocomplete="off"
            ></label>
            <label class="span-2">Base URL<input
              v-model="settings.text.baseUrl"
              :disabled="!canEdit"
              type="url"
              autocomplete="off"
            ></label>
            <label>超时<input
              v-model="settings.text.timeout"
              :disabled="!canEdit"
              placeholder="30s"
            ></label>
            <label>最大输出 Token<input
              v-model.number="settings.text.maxOutputTokens"
              :disabled="!canEdit"
              type="number"
              min="1"
            ></label>
            <label class="span-2">API Key
              <input
                v-model="apiKeys.text.value"
                :disabled="!canEdit || apiKeys.text.clear"
                type="password"
                autocomplete="new-password"
                placeholder="留空表示保持不变"
                @input="acceptNewKey('text')"
              >
              <small>{{ keyDescription('text') }}</small>
            </label>
            <label class="clear-secret span-2"><input
              v-model="apiKeys.text.clear"
              :disabled="!canEdit"
              type="checkbox"
            > 清除已保存的 API Key</label>
          </div>
          <el-button
            :loading="testing === 'text'"
            :disabled="!canEdit"
            @click="testConnection('text')"
          >
            测试连接
          </el-button>
        </article>

        <article class="panel ai-settings-card">
          <header class="ai-settings-card__header">
            <div>
              <p class="eyebrow">
                Safety
              </p>
              <h2>图片审核</h2>
            </div>
            <label class="ai-enabled"><input
              v-model="settings.imageModeration.enabled"
              type="checkbox"
              :disabled="!canEdit"
            > 启用</label>
          </header>
          <div class="ai-form-grid">
            <label>Provider<input
              v-model="settings.imageModeration.provider"
              :disabled="!canEdit"
              autocomplete="off"
            ></label>
            <label>模型<input
              v-model="settings.imageModeration.model"
              :disabled="!canEdit"
              autocomplete="off"
            ></label>
            <label class="span-2">Base URL<input
              v-model="settings.imageModeration.baseUrl"
              :disabled="!canEdit"
              type="url"
              autocomplete="off"
            ></label>
            <label>超时<input
              v-model="settings.imageModeration.timeout"
              :disabled="!canEdit"
              placeholder="20s"
            ></label>
            <label>最大输出 Token<input
              v-model.number="settings.imageModeration.maxOutputTokens"
              :disabled="!canEdit"
              type="number"
              min="1"
            ></label>
            <label class="span-2">API Key
              <input
                v-model="apiKeys.imageModeration.value"
                :disabled="!canEdit || apiKeys.imageModeration.clear"
                type="password"
                autocomplete="new-password"
                placeholder="留空表示保持不变"
                @input="acceptNewKey('imageModeration')"
              >
              <small>{{ keyDescription('imageModeration') }}</small>
            </label>
            <label class="clear-secret span-2"><input
              v-model="apiKeys.imageModeration.clear"
              :disabled="!canEdit"
              type="checkbox"
            > 清除已保存的 API Key</label>
          </div>
          <el-button
            :loading="testing === 'image_moderation'"
            :disabled="!canEdit"
            @click="testConnection('image_moderation')"
          >
            测试连接
          </el-button>
        </article>

        <article class="panel ai-settings-card ai-settings-card--wide">
          <header class="ai-settings-card__header">
            <div>
              <p class="eyebrow">
                Generation
              </p>
              <h2>图生图</h2>
            </div>
            <label class="ai-enabled"><input
              v-model="settings.imageToImage.enabled"
              type="checkbox"
              :disabled="!canEdit"
            > 启用</label>
          </header>
          <div class="ai-form-grid ai-form-grid--wide">
            <label>Provider<input
              v-model="settings.imageToImage.provider"
              :disabled="!canEdit"
              autocomplete="off"
            ></label>
            <label>模型<input
              v-model="settings.imageToImage.model"
              :disabled="!canEdit"
              autocomplete="off"
            ></label>
            <label class="span-2">Base URL<input
              v-model="settings.imageToImage.baseUrl"
              :disabled="!canEdit"
              type="url"
              autocomplete="off"
            ></label>
            <label>质量
              <select
                v-model="settings.imageToImage.quality"
                :disabled="!canEdit"
              ><option value="auto">auto</option><option value="low">low</option><option value="medium">medium</option><option value="high">high</option></select>
            </label>
            <label>超时<input
              v-model="settings.imageToImage.timeout"
              :disabled="!canEdit"
              placeholder="3m"
            ></label>
            <label>最大输入字节<input
              v-model.number="settings.imageToImage.maxInputBytes"
              :disabled="!canEdit"
              type="number"
              min="1"
            ></label>
            <label>最大输出字节<input
              v-model.number="settings.imageToImage.maxOutputBytes"
              :disabled="!canEdit"
              type="number"
              min="1"
            ></label>
            <label class="span-2">API Key
              <input
                v-model="apiKeys.imageToImage.value"
                :disabled="!canEdit || apiKeys.imageToImage.clear"
                type="password"
                autocomplete="new-password"
                placeholder="留空表示保持不变"
                @input="acceptNewKey('imageToImage')"
              >
              <small>{{ keyDescription('imageToImage') }}</small>
            </label>
            <label class="clear-secret span-2"><input
              v-model="apiKeys.imageToImage.clear"
              :disabled="!canEdit"
              type="checkbox"
            > 清除已保存的 API Key</label>
          </div>
          <el-button
            :loading="testing === 'image_to_image'"
            :disabled="!canEdit"
            @click="testConnection('image_to_image')"
          >
            测试连接
          </el-button>
        </article>
      </div>

      <footer class="ai-settings-actions">
        <p>保存会发布一个新版本；API Key 不会在页面或查询接口中回显。</p>
        <el-button
          type="primary"
          size="large"
          :loading="saving"
          :disabled="!canEdit"
          @click="save"
        >
          发布 AI 配置
        </el-button>
      </footer>
    </template>
  </section>
</template>

<style scoped>
.ai-settings-version { display: flex; flex-direction: column; align-items: flex-end; gap: .4rem; }
.ai-settings-version small { color: #6c6860; }
.ai-settings-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1rem; margin-top: 1rem; }
.ai-settings-card { display: flex; flex-direction: column; gap: 1rem; padding: 24px; }
.ai-settings-card--wide { grid-column: 1 / -1; }
.ai-settings-card__header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
.ai-settings-card__header h2 { margin: .15rem 0 0; }
.ai-enabled, .clear-secret { display: inline-flex !important; flex-direction: row !important; align-items: center; gap: .5rem; }
.ai-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .8rem; }
.ai-form-grid label { display: flex; flex-direction: column; gap: .35rem; color: #6c6860; font-size: .86rem; }
.ai-form-grid input, .ai-form-grid select { width: 100%; min-height: 42px; box-sizing: border-box; border: 1px solid #d8d1c5; border-radius: 10px; padding: .65rem .75rem; background: #fff; color: #20201d; }
.ai-form-grid input[type="checkbox"], .ai-enabled input { width: auto; min-height: auto; }
.ai-form-grid small { color: #6c6860; }
.span-2 { grid-column: 1 / -1; }
.ai-settings-actions { position: sticky; bottom: 0; display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-top: 1.25rem; padding: 1rem; border: 1px solid #d8d1c5; border-radius: 14px; background: rgb(255 253 249 / 94%); backdrop-filter: blur(12px); }
.ai-settings-actions p { margin: 0; color: #6c6860; }
@media (max-width: 900px) { .ai-settings-grid { grid-template-columns: 1fr; } .ai-settings-card--wide { grid-column: auto; } }
@media (max-width: 620px) { .ai-form-grid { grid-template-columns: 1fr; } .span-2 { grid-column: auto; } .ai-settings-actions { align-items: stretch; flex-direction: column; } }
</style>
