<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAdminAuthStore } from '@/stores/auth'

const store = useAdminAuthStore()
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const form = reactive({ username: '', password: '' })

async function submit() {
  loading.value = true
  try {
    await store.login(form.username, form.password)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/admin'
    await router.push(redirect)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '管理员登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="centered-page admin-login-page">
    <section class="panel panel--narrow admin-login-panel">
      <RouterLink
        class="brand"
        to="/"
      >
        RECALL 管理后台
      </RouterLink>
      <p class="eyebrow auth-heading">
        管理员认证
      </p>
      <h1>安全登录</h1>
      <p
        id="admin-login-hint"
        class="admin-login-hint"
      >
        仅供平台管理员使用，登录状态与制作方账号相互独立。
      </p>
      <el-form
        class="auth-form"
        label-position="top"
        aria-describedby="admin-login-hint"
        @submit.prevent="submit"
      >
        <el-form-item label="用户名">
          <el-input
            v-model="form.username"
            autocomplete="username"
          />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            type="password"
            autocomplete="current-password"
            show-password
          />
        </el-form-item>
        <el-button
          native-type="submit"
          type="primary"
          size="large"
          :loading="loading"
        >
          登录后台
        </el-button>
      </el-form>
    </section>
  </main>
</template>
