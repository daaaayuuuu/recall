<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'

import { useAdminAuthStore } from '@/stores/auth'

const adminStore = useAdminAuthStore()
const router = useRouter()

async function handleLogout() {
  try {
    await adminStore.logout()
    await router.push({ name: 'admin-login' })
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '退出失败，请稍后重试')
  }
}
</script>

<template>
  <div class="shell shell--admin">
    <header class="shell__header">
      <RouterLink
        class="brand"
        to="/admin"
        aria-label="RECALL 管理后台首页"
      >
        <span class="admin-brand__product">RECALL</span>
        <span class="admin-brand__label">管理后台</span>
      </RouterLink>
      <nav
        class="admin-nav"
        aria-label="管理后台导航"
      >
        <RouterLink to="/admin">
          创建任务
        </RouterLink>
        <RouterLink to="/admin/behavior-events">
          行为记录
        </RouterLink>
        <RouterLink to="/admin/invitation-codes">
          邀请码
        </RouterLink>
        <RouterLink to="/admin/ai-settings">
          AI 配置
        </RouterLink>
      </nav>
      <button
        class="nav__button"
        type="button"
        aria-label="退出管理后台"
        @click="handleLogout"
      >
        <span class="nav-label--desktop">退出后台</span>
        <span class="nav-label--mobile">退出</span>
      </button>
    </header>
    <main class="shell__main">
      <RouterView />
    </main>
  </div>
</template>
