import { fileURLToPath, URL } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

function loopbackPort(name: string): number {
  const value = process.env[name] ?? ''
  if (!/^[0-9]{4,5}$/.test(value)) throw new Error(`${name} must be a 4-5 digit port`)
  const port = Number(value)
  if (!Number.isInteger(port) || port < 1024 || port > 65535) throw new Error(`${name} is outside the safe test range`)
  return port
}

const frontendPort = loopbackPort('ANALYTICS_E2E_FRONTEND_PORT')
const apiPort = loopbackPort('ANALYTICS_E2E_API_PORT')
const apiTarget = `http://127.0.0.1:${apiPort}`

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '127.0.0.1',
    port: frontendPort,
    strictPort: true,
    proxy: {
      '/api/v1': { target: apiTarget, changeOrigin: false },
      '/api': {
        target: apiTarget,
        changeOrigin: false,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
