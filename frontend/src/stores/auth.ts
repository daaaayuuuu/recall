import { defineStore } from 'pinia'

import * as authAPI from '@/api/auth'
import { APIError } from '@/api/client'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as authAPI.CreatorUser | null,
    initialized: false,
    csrfToken: null as string | null,
  }),
  getters: {
    isAuthenticated: (state) => state.user !== null,
  },
  actions: {
    async initialize() {
      if (this.initialized) return
      try {
        const { user } = await authAPI.getSession()
        this.user = user
      } catch (error) {
        if (!(error instanceof APIError) || error.status !== 401) throw error
        this.user = null
      } finally {
        this.initialized = true
      }
    },
    async login(userId: string, password: string) {
      const result = await authAPI.login({ userId, password })
      this.user = result.user
      this.csrfToken = null
      this.initialized = true
    },
    async ensureCSRF() {
      if (this.csrfToken) return this.csrfToken
      const result = await authAPI.getCSRFToken()
      this.csrfToken = result.csrfToken
      return this.csrfToken
    },
    async logout() {
      const csrfToken = await this.ensureCSRF()
      await authAPI.logout(csrfToken)
      this.user = null
      this.csrfToken = null
      this.initialized = true
    },
    async updateNickname(nickname: string) {
      const csrfToken = await this.ensureCSRF()
      this.user = await authAPI.updateMe(nickname, csrfToken)
    },
    async uploadAvatar(file: File) {
      const csrfToken = await this.ensureCSRF()
      this.user = await authAPI.uploadAvatar(file, csrfToken)
    },
    async deleteAvatar() {
      const csrfToken = await this.ensureCSRF()
      this.user = await authAPI.deleteAvatar(csrfToken)
    },
    async changePassword(currentPassword: string, newPassword: string) {
      const csrfToken = await this.ensureCSRF()
      const result = await authAPI.changePassword(currentPassword, newPassword, csrfToken)
      this.user = null
      this.csrfToken = null
      return result
    },
  },
})

export const useAdminAuthStore = defineStore('admin-auth', {
  state: () => ({
    admin: null as authAPI.AdminIdentity | null,
    initialized: false,
    csrfToken: null as string | null,
  }),
  getters: {
    isAuthenticated: (state) => state.admin !== null,
  },
  actions: {
    async initialize() {
      if (this.initialized) return
      try {
        const { admin } = await authAPI.getAdminSession()
        this.admin = admin
      } catch (error) {
        if (!(error instanceof APIError) || error.status !== 401) throw error
        this.admin = null
      } finally {
        this.initialized = true
      }
    },
    async login(username: string, password: string) {
      const result = await authAPI.adminLogin({ username, password })
      this.admin = result.admin
      this.csrfToken = null
      this.initialized = true
    },
    async ensureCSRF() {
      if (this.csrfToken) return this.csrfToken
      const result = await authAPI.getAdminCSRFToken()
      this.csrfToken = result.csrfToken
      return this.csrfToken
    },
    async logout() {
      const csrfToken = await this.ensureCSRF()
      await authAPI.adminLogout(csrfToken)
      this.admin = null
      this.csrfToken = null
      this.initialized = true
    },
  },
})
