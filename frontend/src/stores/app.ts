import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', {
  state: () => ({
    apiStatus: 'unknown' as 'unknown' | 'ready' | 'unavailable',
  }),
  actions: {
    setAPIStatus(status: 'ready' | 'unavailable') {
      this.apiStatus = status
    },
  },
})

