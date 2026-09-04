<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive, ref } from 'vue'

import { register } from '@/api/auth'
import { APIError } from '@/api/client'
import {
  validateInvitationCode,
  validateNickname,
  validatePassword,
  validatePasswordConfirmation,
} from '@/utils/registration'
import { validateUserId } from '@/utils/userId'

const loading = ref(false)
const completedUserId = ref('')
const form = reactive({ invitationCode: '', nickname: '', userId: '', password: '', passwordConfirmation: '' })
const fieldErrors = reactive({ invitationCode: '', nickname: '', userId: '', password: '', passwordConfirmation: '' })

type RegistrationField = keyof typeof fieldErrors

function clearFieldError(field: RegistrationField) {
  fieldErrors[field] = ''
}

function clearFieldErrors() {
  for (const field of Object.keys(fieldErrors) as RegistrationField[]) fieldErrors[field] = ''
}

function updateInvitationCode(value: string) {
  form.invitationCode = value.toUpperCase()
  clearFieldError('invitationCode')
}

function applyAPIFieldErrors(error: APIError) {
  for (const [field, message] of Object.entries(error.fields)) {
    if (Object.prototype.hasOwnProperty.call(fieldErrors, field)) {
      fieldErrors[field as RegistrationField] = message
    }
  }
  if (error.code === 'USER_ID_ALREADY_REGISTERED') fieldErrors.userId = error.message
}

function validateNicknameInput(value: string) {
  fieldErrors.nickname = validateNickname(value)
}

function validateUserIdInput(value: string) {
  fieldErrors.userId = validateUserId(value)
}

function validatePasswordInput(value: string) {
  fieldErrors.password = validatePassword(value)
  if (form.passwordConfirmation) {
    fieldErrors.passwordConfirmation = validatePasswordConfirmation(value, form.passwordConfirmation)
  }
}

function validatePasswordConfirmationInput(value: string) {
  fieldErrors.passwordConfirmation = validatePasswordConfirmation(form.password, value)
}

async function submit() {
  clearFieldErrors()
  fieldErrors.invitationCode = validateInvitationCode(form.invitationCode)
  fieldErrors.nickname = validateNickname(form.nickname)
  fieldErrors.userId = validateUserId(form.userId)
  fieldErrors.password = validatePassword(form.password)
  fieldErrors.passwordConfirmation = validatePasswordConfirmation(form.password, form.passwordConfirmation)
  if (Object.values(fieldErrors).some(Boolean)) return

  loading.value = true
  try {
    const result = await register({
      invitationCode: form.invitationCode,
      nickname: form.nickname,
      userId: form.userId,
      password: form.password,
    })
    completedUserId.value = result.user.userId
  } catch (error) {
    if (error instanceof APIError) applyAPIFieldErrors(error)
    ElMessage.error(error instanceof Error ? error.message : '注册失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="register-view">
    <header class="registration-header">
      <RouterLink
        class="registration-header__back"
        to="/auth/login"
        aria-label="返回登录"
      >
        <span aria-hidden="true">‹</span>
      </RouterLink>
      <strong>创建账号</strong>
      <span aria-hidden="true" />
    </header>

    <section class="registration-content">
      <div
        v-if="!completedUserId"
        class="registration-intro"
      >
        <p class="eyebrow">
          第一次来留刻
        </p>
        <h1>先给自己一个名字</h1>
        <p class="registration-intro__subtitle">
          账号只用于保存和管理你创建的礼物。
        </p>
      </div>

      <el-result
        v-if="completedUserId"
        icon="success"
        title="注册成功"
        :sub-title="`你的用户ID是 ${completedUserId}，请妥善保存并使用它登录。`"
      >
        <template #extra>
          <RouterLink
            class="registration-success-link"
            to="/auth/login"
          >
            前往登录
          </RouterLink>
        </template>
      </el-result>

      <el-form
        v-else
        class="auth-form registration-form"
        label-position="top"
        @submit.prevent="submit"
      >
        <el-form-item
          label="邀请码"
          :error="fieldErrors.invitationCode"
        >
          <el-input
            :model-value="form.invitationCode"
            autocomplete="one-time-code"
            placeholder="输入邀请码"
            :aria-invalid="Boolean(fieldErrors.invitationCode)"
            @update:model-value="updateInvitationCode"
          >
            <template #suffix>
              <span class="registration-required">必填</span>
            </template>
          </el-input>
        </el-form-item>
        <el-form-item
          label="名称"
          :error="fieldErrors.nickname"
        >
          <el-input
            v-model="form.nickname"
            maxlength="64"
            autocomplete="nickname"
            placeholder="对方会看到的名称"
            :aria-invalid="Boolean(fieldErrors.nickname)"
            @input="validateNicknameInput"
          />
        </el-form-item>
        <el-form-item
          label="用户 ID"
          :error="fieldErrors.userId"
        >
          <el-input
            v-model="form.userId"
            maxlength="32"
            autocomplete="username"
            autocapitalize="none"
            placeholder="设置唯一登录 ID"
            :aria-invalid="Boolean(fieldErrors.userId)"
            @input="validateUserIdInput"
          />
        </el-form-item>
        <el-form-item
          label="密码"
          :error="fieldErrors.password"
        >
          <el-input
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            show-password
            placeholder="设置登录密码"
            :aria-invalid="Boolean(fieldErrors.password)"
            @input="validatePasswordInput"
          />
        </el-form-item>
        <el-form-item
          label="确认密码"
          :error="fieldErrors.passwordConfirmation"
        >
          <el-input
            v-model="form.passwordConfirmation"
            type="password"
            autocomplete="new-password"
            show-password
            placeholder="再次输入密码"
            :aria-invalid="Boolean(fieldErrors.passwordConfirmation)"
            @input="validatePasswordConfirmationInput"
          />
        </el-form-item>
        <el-button
          native-type="submit"
          type="primary"
          size="large"
          :loading="loading"
        >
          创建账号
        </el-button>
      </el-form>

      <p
        v-if="!completedUserId"
        class="registration-footer-note"
      >
        创建成功后，将返回登录页重新登录
      </p>
    </section>
  </div>
</template>
