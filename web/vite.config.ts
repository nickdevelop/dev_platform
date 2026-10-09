/// <reference types="vitest/config" />
import { configDefaults, defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      // Same-origin /api in development; in production the Go service and the
      // built frontend are served from one origin. E2E runs move the backend
      // to another port and point the proxy at it via E2E_API_PROXY.
      '/api': process.env.E2E_API_PROXY ?? 'http://127.0.0.1:8080',
    },
  },
  test: {
    environment: 'jsdom',
    // e2e/ holds Playwright specs, run separately via `npm run test:e2e`.
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
})
