import { createRouter, createWebHistory } from 'vue-router'

import AdminLayout from '@/layouts/AdminLayout.vue'
import AuthLayout from '@/layouts/AuthLayout.vue'
import CreatorLayout from '@/layouts/CreatorLayout.vue'
import PlayLayout from '@/layouts/PlayLayout.vue'
import { trackCreatorEvent } from '@/analytics/tracker'
import type { CreatorPageName } from '@/api/analytics'
import { useAdminAuthStore, useAuthStore } from '@/stores/auth'

const creatorPages = new Set<CreatorPageName>([
  'create',
  'games',
  'game-edit',
  'game-preview',
  'game-share',
  'generation-progress',
  'settings',
])

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/app/create' },
    ...(import.meta.env.DEV
      ? [{
          path: '/dev/template-preview',
          name: 'template-preview',
          component: () => import('@/game-runtime/devtools/TemplatePreviewView.vue'),
        }]
      : []),
    {
      path: '/auth',
      component: AuthLayout,
      children: [
        {
          path: 'login',
          name: 'login',
          component: () => import('@/views/LoginView.vue'),
          meta: { guestOnly: true },
        },
        {
          path: 'register',
          name: 'register',
          component: () => import('@/views/RegisterView.vue'),
          meta: { guestOnly: true },
        },
      ],
    },
    {
      path: '/app',
      component: CreatorLayout,
      meta: { requiresAuth: true },
      children: [
        { path: '', redirect: '/app/create' },
        { path: 'create', name: 'create', component: () => import('@/views/HomeView.vue') },
        { path: 'games', name: 'games', component: () => import('@/views/GamesView.vue') },
        {
          path: 'games/:gameId/generation/:runId',
          name: 'generation-progress',
          component: () => import('@/views/GenerationProgressView.vue'),
          props: true,
        },
        {
          path: 'games/:gameId/preview',
          name: 'game-preview',
          component: () => import('@/views/CreatorPreviewView.vue'),
          props: (route) => ({
            gameId: String(route.params.gameId),
            versionId: typeof route.query.versionId === 'string' ? route.query.versionId : '',
          }),
        },
        {
          path: 'games/:gameId/share',
          name: 'game-share',
          component: () => import('@/views/QuickShareView.vue'),
          props: true,
        },
        {
          path: 'games/:gameId/edit',
          name: 'game-edit',
          component: () => import('@/views/HomeView.vue'),
          props: true,
        },
        { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue') },
      ],
    },
    {
      path: '/play',
      component: PlayLayout,
      children: [
        { path: ':publicId', name: 'play', component: () => import('@/views/PlayView.vue'), props: true },
      ],
    },
    {
      path: '/admin/login',
      name: 'admin-login',
      component: () => import('@/views/AdminLoginView.vue'),
      meta: { adminGuestOnly: true },
    },
    {
      path: '/admin',
      component: AdminLayout,
      meta: { requiresAdmin: true },
      children: [
        { path: '', name: 'admin', component: () => import('@/views/AdminView.vue') },
        {
          path: 'behavior-events',
          name: 'admin-behavior-events',
          component: () => import('@/views/AdminBehaviorEventsView.vue'),
        },
        {
          path: 'invitation-codes',
          name: 'admin-invitation-codes',
          component: () => import('@/views/AdminInvitationsView.vue'),
        },
        {
          path: 'ai-settings',
          name: 'admin-ai-settings',
          component: () => import('@/views/AdminAISettingsView.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.requiresAdmin || to.meta.adminGuestOnly) {
    const adminStore = useAdminAuthStore()
    await adminStore.initialize()
    if (to.meta.requiresAdmin && !adminStore.isAuthenticated) {
      return { name: 'admin-login', query: { redirect: to.fullPath } }
    }
    if (to.meta.adminGuestOnly && adminStore.isAuthenticated) {
      return { name: 'admin' }
    }
    return true
  }

  if (to.meta.requiresAuth || to.meta.guestOnly) {
    const authStore = useAuthStore()
    await authStore.initialize()
    if (to.meta.requiresAuth && !authStore.isAuthenticated) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    if (to.meta.guestOnly && authStore.isAuthenticated) {
      return { name: 'create' }
    }
  }
  return true
})

router.afterEach((to, _from, failure) => {
  if (failure || typeof to.name !== 'string' || !creatorPages.has(to.name as CreatorPageName)) return
  if (!to.matched.some((record) => record.meta.requiresAuth)) return

  const authStore = useAuthStore()
  if (authStore.isAuthenticated) {
    void trackCreatorEvent(to.name as CreatorPageName)
  }
})

export default router
