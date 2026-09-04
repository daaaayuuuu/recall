<script setup lang="ts">
import { computed } from 'vue'

import { avatarURL } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()
const currentAvatarURL = computed(() => {
  const assetID = authStore.user?.avatarAssetId
  return assetID ? avatarURL(assetID) : null
})
const avatarFallback = computed(() => (authStore.user?.nickname || authStore.user?.userId || 'U').slice(0, 1).toUpperCase())
</script>

<template>
  <div class="shell shell--creator">
    <header class="shell__header shell__header--recall-wordmark">
      <RouterLink
        class="brand brand--recall"
        to="/app/create"
        aria-label="留刻创建者端"
      >
        <span
          class="brand__mark"
          aria-hidden="true"
        ><i /><i /></span>
        <span class="brand__wordmark">
          <strong>留刻</strong>
          <small>RECALL</small>
        </span>
      </RouterLink>
      <nav
        class="nav"
        aria-label="制作方导航"
      >
        <RouterLink
          to="/app/create"
          aria-label="创建游戏"
        >
          <span class="nav-label nav-label--desktop">创建游戏</span>
          <span class="nav-label nav-label--mobile">创建</span>
        </RouterLink>
        <RouterLink
          to="/app/games"
          aria-label="我的游戏"
        >
          <span class="nav-label nav-label--desktop">我的游戏</span>
          <span class="nav-label nav-label--mobile">游戏</span>
        </RouterLink>
        <RouterLink
          to="/app/settings"
          aria-label="个人设置"
        >
          <span class="nav-profile">
            <span
              class="nav-avatar"
              aria-hidden="true"
            >
              <img
                v-if="currentAvatarURL"
                :key="currentAvatarURL"
                :src="currentAvatarURL"
                alt=""
              >
              <span v-else>{{ avatarFallback }}</span>
            </span>
            <span class="nav-label nav-label--desktop">个人设置</span>
            <span class="nav-label nav-label--mobile">设置</span>
          </span>
        </RouterLink>
      </nav>
    </header>
    <main class="shell__main">
      <RouterView />
    </main>
  </div>
</template>
