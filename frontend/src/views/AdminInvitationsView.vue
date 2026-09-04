<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, ref } from 'vue'

import {
  createAdminInvitation,
  listAdminInvitations,
  revokeAdminInvitation,
  type AdminInvitation,
  type InvitationStatus,
} from '@/api/invitations'
import { useAdminAuthStore } from '@/stores/auth'
import { confirmDestructiveAction } from '@/utils/confirm'

const adminStore = useAdminAuthStore()
const invitations = ref<AdminInvitation[]>([])
const generatedCode = ref('')
const loading = ref(true)
const generating = ref(false)
const revokingId = ref('')

const unusedCount = computed(() => invitations.value.filter((item) => item.status === 'unused').length)

const statusLabels: Record<InvitationStatus, string> = {
  unused: '未使用',
  used: '已使用',
  revoked: '已撤销',
}

onMounted(load)

async function load() {
  loading.value = true
  try {
    invitations.value = (await listAdminInvitations()).items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '邀请码加载失败')
  } finally {
    loading.value = false
  }
}

async function generate() {
  generating.value = true
  try {
    const csrfToken = await adminStore.ensureCSRF()
    const invitation = await createAdminInvitation(csrfToken)
    generatedCode.value = invitation.code
    invitations.value.unshift(invitation)
    ElMessage.success('邀请码已生成，请立即复制保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '邀请码生成失败')
  } finally {
    generating.value = false
  }
}

async function copyGeneratedCode() {
  if (!generatedCode.value) return
  try {
    await globalThis.navigator.clipboard.writeText(generatedCode.value)
    ElMessage.success('邀请码已复制')
  } catch {
    ElMessage.error('复制失败，请手动复制邀请码')
  }
}

async function revoke(invitation: AdminInvitation) {
  try {
    await confirmDestructiveAction(
      `确定撤销邀请码 ${invitation.codeHint} 吗？撤销后不能用于注册。`,
      '撤销邀请码',
      { confirmButtonText: '确认撤销', cancelButtonText: '取消' },
    )
  } catch {
    return
  }

  revokingId.value = invitation.id
  try {
    const csrfToken = await adminStore.ensureCSRF()
    const updated = await revokeAdminInvitation(invitation.id, csrfToken)
    invitations.value = invitations.value.map((item) => item.id === updated.id ? updated : item)
    ElMessage.success('邀请码已撤销')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '邀请码撤销失败')
  } finally {
    revokingId.value = ''
  }
}

function formatTime(value: string | null) {
  if (!value) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}
</script>

<template>
  <section
    class="admin-page invitation-page"
    :aria-busy="loading"
  >
    <header class="section-header admin-page__header">
      <div>
        <p class="eyebrow">
          注册权限
        </p>
        <h1>邀请码管理</h1>
        <p class="section-copy">
          每个邀请码只能成功注册一个账号。完整邀请码只在生成时显示一次。
        </p>
      </div>
      <el-button
        type="primary"
        size="large"
        :loading="generating"
        @click="generate"
      >
        生成邀请码
      </el-button>
    </header>

    <section
      v-if="generatedCode"
      class="panel invitation-created"
      aria-live="polite"
    >
      <div>
        <p class="eyebrow">
          本次生成
        </p>
        <strong data-testid="generated-invitation-code">{{ generatedCode }}</strong>
        <p>刷新或离开页面后将不再显示完整邀请码，请现在复制。</p>
      </div>
      <el-button @click="copyGeneratedCode">
        复制邀请码
      </el-button>
    </section>

    <div class="invitation-summary">
      <span>最近 {{ invitations.length }} 条</span>
      <span>可用 {{ unusedCount }} 条</span>
    </div>

    <div
      v-if="loading"
      class="panel loading-panel"
    >
      正在加载邀请码…
    </div>
    <el-empty
      v-else-if="invitations.length === 0"
      description="尚未生成邀请码"
    />
    <div
      v-else
      class="invitation-list"
      aria-label="邀请码列表"
    >
      <article
        v-for="invitation in invitations"
        :key="invitation.id"
        class="panel invitation-card"
      >
        <header>
          <code>{{ invitation.codeHint }}</code>
          <el-tag
            :type="invitation.status === 'used' ? 'success' : invitation.status === 'revoked' ? 'info' : 'warning'"
          >
            {{ statusLabels[invitation.status] }}
          </el-tag>
        </header>
        <dl>
          <div><dt>创建时间</dt><dd>{{ formatTime(invitation.createdAt) }}</dd></div>
          <div><dt>创建管理员</dt><dd>{{ invitation.createdByAdmin }}</dd></div>
          <div v-if="invitation.usedAt">
            <dt>使用时间</dt><dd>{{ formatTime(invitation.usedAt) }}</dd>
          </div>
          <div v-if="invitation.usedByLoginId">
            <dt>注册用户</dt><dd>{{ invitation.usedByLoginId }}</dd>
          </div>
          <div v-if="invitation.revokedAt">
            <dt>撤销时间</dt><dd>{{ formatTime(invitation.revokedAt) }}</dd>
          </div>
        </dl>
        <el-button
          v-if="invitation.status === 'unused'"
          type="danger"
          plain
          :loading="revokingId === invitation.id"
          @click="revoke(invitation)"
        >
          撤销
        </el-button>
      </article>
    </div>
  </section>
</template>

<style scoped>
.invitation-page {
  display: grid;
  gap: 20px;
}

.invitation-created {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 24px;
  border-color: #8ab59a;
  background: #f0f8f2;
}

.invitation-created p {
  margin: 6px 0 0;
  color: #5f685f;
}

.invitation-created strong {
  display: block;
  margin-top: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: clamp(1.6rem, 5vw, 2.2rem);
  letter-spacing: 0.12em;
}

.invitation-summary {
  display: flex;
  gap: 16px;
  color: #6c6860;
  font-size: 0.9rem;
}

.invitation-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr));
  gap: 16px;
}

.invitation-card {
  display: grid;
  gap: 16px;
  padding: 22px;
}

.invitation-card header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.invitation-card code {
  font-size: 1.15rem;
  font-weight: 750;
  letter-spacing: 0.08em;
}

.invitation-card dl {
  display: grid;
  gap: 8px;
  margin: 0;
}

.invitation-card dl div {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  gap: 10px;
}

.invitation-card dt {
  color: #777168;
}

.invitation-card dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}

@media (max-width: 700px) {
  .invitation-created {
    align-items: stretch;
    flex-direction: column;
  }

  .invitation-created .el-button {
    width: 100%;
  }
}
</style>
