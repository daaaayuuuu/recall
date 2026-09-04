<script setup lang="ts">
import { ElMessage } from 'element-plus'
import QRCode from 'qrcode'
import { computed, onMounted, ref } from 'vue'

import { getGame, type Game } from '@/api/games'
import { createShareLink, type ShareLink } from '@/api/sharing'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ gameId: string }>()
const authStore = useAuthStore()
const game = ref<Game | null>(null)
const loading = ref(true)
const loadError = ref('')
const shareDays = ref(7)
const customShareExpiry = ref('')
const creatingShare = ref(false)
const createdShare = ref<ShareLink | null>(null)
const qrCodeURL = ref('')
const copied = ref(false)

const formattedExpiry = computed(() => {
  if (!createdShare.value) return ''
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(createdShare.value.expiresAt))
})

onMounted(loadGame)

async function loadGame() {
  loading.value = true
  loadError.value = ''
  try {
    game.value = await getGame(props.gameId)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '游戏加载失败'
  } finally {
    loading.value = false
  }
}

function selectedExpiry() {
  return shareDays.value === 0
    ? new Date(customShareExpiry.value)
    : new Date(Date.now() + shareDays.value * 24 * 60 * 60 * 1000)
}

async function createShare() {
  if (!game.value || game.value.status !== 'ready') return

  const expiry = selectedExpiry()
  if (Number.isNaN(expiry.getTime()) || expiry.getTime() <= Date.now()) {
    ElMessage.warning('请选择未来的分享截止时间')
    return
  }

  creatingShare.value = true
  copied.value = false
  try {
    const csrfToken = await authStore.ensureCSRF()
    const share = await createShareLink(props.gameId, expiry.toISOString(), csrfToken)
    createdShare.value = share

    try {
      qrCodeURL.value = await QRCode.toDataURL(share.url, {
        width: 640,
        margin: 2,
        errorCorrectionLevel: 'M',
        color: { dark: '#202796', light: '#fbfdf8' },
      })
    } catch {
      qrCodeURL.value = ''
      ElMessage.warning('链接已创建，但二维码生成失败')
    }

    try {
      await copyShareURL()
      ElMessage.success('分享链接已创建并复制')
    } catch {
      ElMessage.success('分享链接已创建，请手动复制')
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '创建分享链接失败')
  } finally {
    creatingShare.value = false
  }
}

async function copyShareURL() {
  if (!createdShare.value || !globalThis.navigator.clipboard) {
    throw new Error('clipboard unavailable')
  }
  await globalThis.navigator.clipboard.writeText(createdShare.value.url)
  copied.value = true
}

async function copyShare() {
  try {
    await copyShareURL()
    ElMessage.success('分享链接已复制')
  } catch {
    ElMessage.error('浏览器无法访问剪贴板，请手动复制链接')
  }
}

function saveQRCode() {
  if (!qrCodeURL.value) return
  const download = globalThis.document.createElement('a')
  const safeTitle = (game.value?.title || '游戏').replace(/[\\/:*?"<>|]/g, '-')
  download.href = qrCodeURL.value
  download.download = `${safeTitle}-分享二维码.png`
  globalThis.document.body.append(download)
  download.click()
  download.remove()
}
</script>

<template>
  <section class="quick-share-page">
    <RouterLink
      class="quick-share-back"
      :to="{ name: 'games' }"
    >
      ← 返回我的游戏
    </RouterLink>

    <article
      class="quick-share-card"
      :class="{ 'quick-share-card--result': createdShare }"
    >
      <div
        v-if="loading"
        class="quick-share-state"
        role="status"
      >
        正在准备分享页面…
      </div>

      <div
        v-else-if="loadError || !game"
        class="quick-share-state"
      >
        <p class="eyebrow">
          无法分享
        </p>
        <h1>{{ loadError || '没有找到这个游戏' }}</h1>
        <button
          class="quick-share-button quick-share-button--secondary"
          type="button"
          @click="loadGame"
        >
          重新加载
        </button>
      </div>

      <template v-else-if="!createdShare">
        <header class="quick-share-heading">
          <p class="eyebrow">
            分享游戏
          </p>
          <h1>把完成的游戏发给朋友</h1>
          <p>接收方无需账号。链接到期或主动停止后，将无法开始新一局游戏。</p>
        </header>

        <form
          class="quick-share-form"
          @submit.prevent="createShare"
        >
          <select
            v-model.number="shareDays"
            aria-label="分享有效期"
            :disabled="game.status !== 'ready' || creatingShare"
          >
            <option :value="1">
              1 天
            </option>
            <option :value="7">
              7 天
            </option>
            <option :value="30">
              30 天
            </option>
            <option :value="0">
              自定义截止时间
            </option>
          </select>

          <input
            v-if="shareDays === 0"
            v-model="customShareExpiry"
            type="datetime-local"
            aria-label="自定义分享截止时间"
            :disabled="creatingShare"
          >

          <button
            class="quick-share-button quick-share-button--primary quick-share-create-button"
            type="submit"
            :disabled="game.status !== 'ready' || creatingShare"
          >
            {{ creatingShare ? '正在生成链接…' : '创建并复制链接' }}
          </button>
        </form>

        <p
          v-if="game.status !== 'ready'"
          class="quick-share-warning"
          role="status"
        >
          游戏创建完成后即可生成分享链接
        </p>

        <div
          class="quick-share-empty"
          aria-hidden="true"
        >
          <span class="quick-share-empty__shadow" />
          <span class="quick-share-empty__box" />
          <span class="quick-share-empty__lid" />
        </div>
        <p class="quick-share-empty-label">
          还没有创建过分享链接
        </p>
      </template>

      <template v-else>
        <header class="quick-share-heading quick-share-heading--result">
          <p class="eyebrow">
            准备送出这份回忆
          </p>
          <h1>把故事交给 TA</h1>
          <p>链接和二维码指向同一份私密礼物。</p>
        </header>

        <div class="quick-share-qr-card">
          <i class="quick-share-tape quick-share-tape--left" />
          <i class="quick-share-tape quick-share-tape--right" />
          <img
            v-if="qrCodeURL"
            :src="qrCodeURL"
            :alt="`${game.title}的分享二维码`"
          >
          <p
            v-else
            class="quick-share-qr-error"
          >
            二维码生成失败，请使用下方链接分享
          </p>
          <strong>扫码打开「{{ game.title }}」</strong>
          <small>有效期至 {{ formattedExpiry }}</small>
        </div>

        <div class="quick-share-link-row">
          <code>{{ createdShare.url }}</code>
          <span>私密链接</span>
        </div>

        <button
          class="quick-share-button quick-share-button--primary"
          type="button"
          @click="copyShare"
        >
          {{ copied ? '已复制链接' : '复制链接' }}
        </button>
        <button
          class="quick-share-button quick-share-button--secondary"
          type="button"
          :disabled="!qrCodeURL"
          @click="saveQRCode"
        >
          <span aria-hidden="true">↓</span> 保存二维码
        </button>

        <aside class="quick-share-tip">
          <span aria-hidden="true">♡</span>
          <p>建议直接发送给对方，再附上一句你亲手写的邀请。</p>
        </aside>
      </template>
    </article>
  </section>
</template>

<style scoped>
.quick-share-page {
  width: min(560px, 100%);
  margin: 0 auto;
}

.quick-share-back {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  margin-bottom: 16px;
  color: var(--recall-ink);
  font-weight: 750;
}

.quick-share-card {
  min-height: min(820px, calc(100svh - 170px));
  padding: clamp(28px, 6vw, 42px) clamp(26px, 7vw, 36px);
  border: 4px solid var(--recall-ink);
  border-radius: 28px;
  background: var(--recall-paper);
  box-shadow: 7px 8px 0 rgb(37 44 155 / 12%);
}

.quick-share-card--result {
  min-height: 0;
}

.quick-share-heading .eyebrow {
  margin-bottom: 26px;
  color: var(--recall-coral);
  font-size: 1rem;
}

.quick-share-heading h1,
.quick-share-state h1 {
  margin: 0 0 20px;
  color: var(--recall-ink);
  font-family: "Kaiti SC", STKaiti, serif;
  font-size: clamp(2rem, 8vw, 2.65rem);
  line-height: 1.15;
  letter-spacing: 0.02em;
}

.quick-share-heading:not(.quick-share-heading--result) h1 {
  font-size: clamp(1.75rem, 7.2vw, 2.65rem);
}

.quick-share-heading > p:last-child {
  margin: 0;
  color: var(--recall-muted);
  font-size: clamp(0.98rem, 4vw, 1.25rem);
  line-height: 1.75;
}

.quick-share-heading--result {
  text-align: center;
}

.quick-share-heading--result .eyebrow {
  margin-bottom: 14px;
}

.quick-share-heading--result h1 {
  font-size: clamp(2.3rem, 10vw, 3.2rem);
}

.quick-share-form {
  display: grid;
  gap: 16px;
  margin-top: 30px;
}

.quick-share-form select,
.quick-share-form input {
  width: 100%;
  min-height: 68px;
  padding: 0 20px;
  border: 4px solid var(--recall-ink);
  border-radius: 22px;
  color: var(--recall-ink-dark);
  background: var(--recall-paper);
  font-size: 1.1rem;
  outline: 0;
}

.quick-share-form select:focus-visible,
.quick-share-form input:focus-visible {
  box-shadow: 0 0 0 4px rgb(32 39 150 / 18%);
}

.quick-share-button {
  width: 100%;
  min-height: 66px;
  padding: 0 20px;
  border: 4px solid var(--recall-ink);
  border-radius: 20px;
  color: var(--recall-ink);
  background: var(--recall-paper);
  box-shadow: 6px 8px 0 var(--recall-ink);
  font-size: 1.12rem;
  font-weight: 800;
  cursor: pointer;
  transition: box-shadow 140ms ease, transform 140ms ease;
}

.quick-share-button--primary {
  color: #fff;
  background: var(--recall-coral);
}

.quick-share-button + .quick-share-button {
  margin-top: 18px;
}

.quick-share-button:active:not(:disabled) {
  box-shadow: 2px 3px 0 var(--recall-ink);
  transform: translateY(4px);
}

.quick-share-button:focus-visible {
  outline: 3px double var(--recall-ink);
  outline-offset: 4px;
}

.quick-share-button:disabled,
.quick-share-form select:disabled,
.quick-share-form input:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.quick-share-warning {
  margin: 20px 0 0;
  color: var(--recall-coral-dark);
  font-weight: 700;
  text-align: center;
}

.quick-share-empty {
  position: relative;
  width: 220px;
  height: 235px;
  margin: 62px auto 10px;
}

.quick-share-empty__shadow {
  position: absolute;
  right: 0;
  bottom: 5px;
  left: 0;
  height: 24px;
  border-radius: 50%;
  background: #f6f7fb;
}

.quick-share-empty__box {
  position: absolute;
  bottom: 18px;
  left: 38px;
  width: 148px;
  height: 118px;
  background: #f0f1f5;
  clip-path: polygon(0 0, 68% 0, 68% 28%, 100% 21%, 100% 100%, 0 100%);
}

.quick-share-empty__lid {
  position: absolute;
  top: 28px;
  left: 42px;
  width: 150px;
  height: 90px;
  background: #f2f3f7;
  clip-path: polygon(19% 0, 100% 37%, 72% 100%, 0 55%);
  transform: rotate(7deg);
}

.quick-share-empty-label {
  margin: 0;
  color: #a3a6ae;
  font-size: 1rem;
  text-align: center;
}

.quick-share-qr-card {
  position: relative;
  display: grid;
  justify-items: center;
  margin: 36px auto 28px;
  padding: 36px 30px 24px;
  border: 4px solid var(--recall-ink);
  border-radius: 0 0 14px 14px;
  background: #fff;
  box-shadow: 8px 10px 0 rgb(37 44 155 / 16%);
}

.quick-share-qr-card img {
  width: min(280px, 78vw);
  aspect-ratio: 1;
  image-rendering: pixelated;
}

.quick-share-qr-card strong {
  margin-top: 12px;
  color: var(--recall-ink);
  font-size: 0.98rem;
  text-align: center;
}

.quick-share-qr-card small {
  margin-top: 8px;
  color: var(--recall-muted);
  text-align: center;
}

.quick-share-tape {
  position: absolute;
  z-index: 1;
  top: -22px;
  width: 84px;
  height: 32px;
  border: 3px solid var(--recall-ink);
  background: #ffd44d;
  opacity: 0.94;
}

.quick-share-tape--left {
  left: -26px;
  transform: rotate(-13deg);
}

.quick-share-tape--right {
  right: -26px;
  transform: rotate(13deg);
}

.quick-share-qr-error {
  display: grid;
  width: min(280px, 78vw);
  aspect-ratio: 1;
  place-items: center;
  margin: 0;
  padding: 24px;
  color: var(--recall-muted);
  background: #f3f4f7;
  text-align: center;
}

.quick-share-link-row {
  display: flex;
  min-height: 72px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 24px;
  padding: 12px 18px;
  border: 4px solid var(--recall-ink);
  border-radius: 20px;
  background: var(--recall-cyan);
}

.quick-share-link-row code {
  min-width: 0;
  overflow: hidden;
  color: var(--recall-ink-dark);
  font-family: inherit;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-share-link-row span {
  flex: 0 0 auto;
  padding: 5px 10px;
  border-radius: 999px;
  color: #fff;
  background: var(--recall-ink);
  font-size: 0.7rem;
}

.quick-share-tip {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 14px;
  align-items: center;
  margin-top: 30px;
  padding: 18px;
  border: 3px solid var(--recall-ink);
  border-radius: 18px;
  background: #fff0c3;
}

.quick-share-tip > span {
  color: var(--recall-coral);
  font-size: 2rem;
}

.quick-share-tip p {
  margin: 0;
  color: var(--recall-muted);
  font-size: 0.84rem;
  line-height: 1.6;
}

.quick-share-state {
  display: grid;
  min-height: 420px;
  place-content: center;
  gap: 20px;
  color: var(--recall-muted);
  text-align: center;
}

@media (max-width: 520px) {
  .quick-share-card {
    min-height: calc(100svh - 170px);
    padding: 28px 26px 34px;
    border-radius: 24px;
  }

  .quick-share-form select,
  .quick-share-form input,
  .quick-share-button {
    min-height: 58px;
  }

  .quick-share-empty {
    margin-top: 40px;
    transform: scale(0.88);
  }

  .quick-share-qr-card {
    padding-inline: 18px;
  }
}
</style>
