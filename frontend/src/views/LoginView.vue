<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const form = reactive({ userId: '', password: '' })

async function submit() {
  loading.value = true
  try {
    await authStore.login(form.userId, form.password)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/app/create'
    await router.push(redirect)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '登录失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-view">
    <p class="eyebrow login-view__welcome">
      欢迎回来
    </p>
    <h1>继续制作你们的故事</h1>
    <p class="login-view__subtitle">
      每一段共同经历，都值得被好好打开。
    </p>
    <el-form
      class="auth-form"
      label-position="top"
      @submit.prevent="submit"
    >
      <el-form-item label="用户 ID">
        <el-input
          v-model="form.userId"
          autocomplete="username"
          autocapitalize="none"
          placeholder="输入你的用户 ID"
        />
      </el-form-item>
      <el-form-item label="密码">
        <el-input
          v-model="form.password"
          type="password"
          autocomplete="current-password"
          placeholder="输入密码"
          show-password
        />
      </el-form-item>
      <el-button
        native-type="submit"
        type="primary"
        size="large"
        :loading="loading"
      >
        登录
      </el-button>
    </el-form>
    <div class="auth-links">
      <span>还没有账号？</span>
      <RouterLink to="/auth/register">
        创建账号
      </RouterLink>
    </div>
  </div>
</template>
