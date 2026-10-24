import react from '@vitejs/plugin-react'
import { configDefaults, defineConfig } from 'vitest/config'

// P0 keeps the production server contract identical to web/vite.config.ts:
// same-origin /api, WebSocket proxying, SSE header flushing for EventSource,
// hashed assets under assets/, and fonts that must never be inlined.
const apiTarget = process.env.E2E_API_PROXY ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        ws: true,
        configure(proxy) {
          proxy.on('proxyRes', (proxyRes, _request, response) => {
            const contentType = proxyRes.headers['content-type']
            if (typeof contentType === 'string' && contentType.includes('text/event-stream')) {
              setImmediate(() => response.flushHeaders())
            }
          })
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: true,
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsInlineLimit: (filePath) => (filePath.endsWith('.woff2') ? false : undefined),
    chunkSizeWarningLimit: 4000,
  },
})
