<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { avatarURL } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { confirmDestructiveAction } from '@/utils/confirm'

const authStore = useAuthStore()
const router = useRouter()
const profileLoading = ref(false)
const passwordLoading = ref(false)
const avatarLoading = ref(false)
const logoutLoading = ref(false)
const avatarDialogVisible = ref(false)
const nicknameDialogVisible = ref(false)
const passwordDialogVisible = ref(false)
const avatarInput = ref<globalThis.HTMLInputElement | null>(null)
const profile = reactive({ nickname: authStore.user?.nickname ?? '' })
const password = reactive({ current: '', next: '', confirmation: '' })
const currentAvatarURL = computed(() => {
  const assetID = authStore.user?.avatarAssetId
  return assetID ? avatarURL(assetID) : null
})
const avatarFallback = computed(() => (authStore.user?.nickname || authStore.user?.userId || 'U').slice(0, 1).toUpperCase())
const nicknameLabel = computed(() => authStore.user?.nickname?.trim() || '未设置')

function openAvatarDialog() {
  avatarDialogVisible.value = true
}

function openNicknameDialog() {
  profile.nickname = authStore.user?.nickname ?? ''
  nicknameDialogVisible.value = true
}

function openPasswordDialog() {
  password.current = ''
  password.next = ''
  password.confirmation = ''
  passwordDialogVisible.value = true
}

async function saveProfile() {
  profileLoading.value = true
  try {
    await authStore.updateNickname(profile.nickname)
    ElMessage.success('个人资料已保存')
    nicknameDialogVisible.value = false
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    profileLoading.value = false
  }
}

function chooseAvatar() {
  avatarInput.value?.click()
}

async function handleAvatarSelected(event: globalThis.Event) {
  const input = event.target as globalThis.HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  avatarLoading.value = true
  try {
    await authStore.uploadAvatar(file)
    ElMessage.success('头像已更新')
    avatarDialogVisible.value = false
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '头像上传失败')
  } finally {
    avatarLoading.value = false
    input.value = ''
  }
}

async function removeAvatar() {
  try {
    await confirmDestructiveAction('删除后将恢复为默认头像。', '删除头像', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  avatarLoading.value = true
  try {
    await authStore.deleteAvatar()
    ElMessage.success('头像已删除')
    avatarDialogVisible.value = false
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '头像删除失败')
  } finally {
    avatarLoading.value = false
  }
}

async function savePassword() {
  if (password.next !== password.confirmation) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  passwordLoading.value = true
  try {
    const result = await authStore.changePassword(password.current, password.next)
    ElMessage.success(result.message)
    await router.push({ name: 'login' })
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '密码修改失败')
  } finally {
    passwordLoading.value = false
  }
}

async function handleLogout() {
  logoutLoading.value = true
  try {
    await authStore.logout()
    await router.push({ name: 'login' })
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '退出失败，请稍后重试')
  } finally {
    logoutLoading.value = false
  }
}
</script>

<template>
  <section class="settings-grid">
    <article class="panel settings-card settings-summary-card">
      <header class="settings-card__header">
        <p class="eyebrow">
          个人设置
        </p>
        <h1>账号资料</h1>
      </header>

      <div
        class="settings-list"
        aria-label="账号资料"
      >
        <div class="settings-list__item settings-list__item--avatar">
          <div
            class="avatar-preview settings-list__avatar"
            aria-hidden="true"
          >
            <img
              v-if="currentAvatarURL"
              :key="currentAvatarURL"
              :src="currentAvatarURL"
              alt=""
            >
            <span v-else>{{ avatarFallback }}</span>
          </div>
          <div class="settings-list__value">
            <span>头像</span>
            <strong>{{ currentAvatarURL ? '已设置' : '默认头像' }}</strong>
          </div>
          <el-button
            class="settings-list__action"
            aria-label="修改头像"
            @click="openAvatarDialog"
          >
            修改
          </el-button>
        </div>

        <div class="settings-list__item">
          <div class="settings-list__value">
            <span>昵称</span>
            <strong>{{ nicknameLabel }}</strong>
          </div>
          <el-button
            class="settings-list__action"
            aria-label="修改昵称"
            @click="openNicknameDialog"
          >
            修改
          </el-button>
        </div>

        <div class="settings-list__item settings-list__item--static">
          <div class="settings-list__value">
            <span>用户 ID</span>
            <strong>{{ authStore.user?.userId }}</strong>
          </div>
        </div>
      </div>
    </article>

    <article class="panel settings-card settings-summary-card">
      <header class="settings-card__header">
        <p class="eyebrow">
          账号安全
        </p>
        <h2>登录与安全</h2>
      </header>

      <div class="settings-list settings-list--security">
        <div class="settings-list__item">
          <div class="settings-list__value">
            <span>登录密码</span>
            <strong aria-label="密码已设置">••••••••</strong>
          </div>
          <el-button
            class="settings-list__action"
            aria-label="修改密码"
            @click="openPasswordDialog"
          >
            修改
          </el-button>
        </div>
      </div>
    </article>

    <div class="settings-logout">
      <el-button
        class="settings-logout__button"
        type="danger"
        :loading="logoutLoading"
        @click="handleLogout"
      >
        退出登录
      </el-button>
    </div>

    <el-dialog
      v-model="avatarDialogVisible"
      class="recall-dialog settings-edit-dialog"
      modal-class="recall-dialog-overlay"
      title="修改头像"
      width="460px"
      append-to-body
      align-center
      destroy-on-close
      :close-on-click-modal="false"
    >
      <div class="settings-avatar-editor">
        <div
          class="avatar-preview settings-avatar-editor__preview"
          aria-hidden="true"
        >
          <img
            v-if="currentAvatarURL"
            :key="currentAvatarURL"
            :src="currentAvatarURL"
            alt=""
          >
          <span v-else>{{ avatarFallback }}</span>
        </div>
        <p>支持 JPEG、PNG 和 WebP，上传后自动居中裁剪。</p>
        <input
          ref="avatarInput"
          class="visually-hidden"
          type="file"
          accept="image/jpeg,image/png,image/webp"
          @change="handleAvatarSelected"
        >
        <div class="settings-dialog__actions settings-dialog__actions--stacked">
          <el-button
            type="primary"
            :loading="avatarLoading"
            @click="chooseAvatar"
          >
            {{ currentAvatarURL ? '选择新头像' : '上传头像' }}
          </el-button>
          <el-button
            v-if="currentAvatarURL"
            type="danger"
            plain
            :disabled="avatarLoading"
            @click="removeAvatar"
          >
            删除当前头像
          </el-button>
        </div>
      </div>
    </el-dialog>

    <el-dialog
      v-model="nicknameDialogVisible"
      class="recall-dialog settings-edit-dialog"
      modal-class="recall-dialog-overlay"
      title="修改昵称"
      width="460px"
      append-to-body
      align-center
      destroy-on-close
      :close-on-click-modal="false"
    >
      <el-form
        class="settings-dialog__form"
        label-position="top"
        @submit.prevent="saveProfile"
      >
        <el-form-item label="昵称">
          <el-input
            v-model="profile.nickname"
            maxlength="64"
            show-word-limit
            placeholder="分享页将展示该昵称"
            autocomplete="nickname"
          />
        </el-form-item>
        <div class="settings-dialog__actions">
          <el-button @click="nicknameDialogVisible = false">
            取消
          </el-button>
          <el-button
            native-type="submit"
            type="primary"
            :loading="profileLoading"
          >
            保存昵称
          </el-button>
        </div>
      </el-form>
    </el-dialog>

    <el-dialog
      v-model="passwordDialogVisible"
      class="recall-dialog settings-edit-dialog"
      modal-class="recall-dialog-overlay"
      title="修改密码"
      width="460px"
      append-to-body
      align-center
      destroy-on-close
      :close-on-click-modal="false"
    >
      <p class="settings-dialog__hint">
        修改成功后，所有设备上的会话都会退出。
      </p>
      <el-form
        class="settings-dialog__form"
        label-position="top"
        @submit.prevent="savePassword"
      >
        <el-form-item label="当前密码">
          <el-input
            v-model="password.current"
            type="password"
            autocomplete="current-password"
            show-password
          />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input
            v-model="password.next"
            type="password"
            autocomplete="new-password"
            show-password
          />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input
            v-model="password.confirmation"
            type="password"
            autocomplete="new-password"
            show-password
          />
        </el-form-item>
        <div class="settings-dialog__actions">
          <el-button @click="passwordDialogVisible = false">
            取消
          </el-button>
          <el-button
            native-type="submit"
            type="primary"
            :loading="passwordLoading"
          >
            确认修改
          </el-button>
        </div>
      </el-form>
    </el-dialog>
  </section>
</template>
