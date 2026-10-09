/// <reference types="vitest/config" />
import { configDefaults, defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// Same-origin /api in development; in production the Go service and the built
// frontend are served from one origin. E2E runs move the backend to another
// port and point the proxy at it via E2E_API_PROXY.
const apiTarget = process.env.E2E_API_PROXY ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        configure(proxy) {
          // The dev proxy buffers response headers until the first body byte,
          // which delays the browser's EventSource `open` event for long-lived
          // SSE streams (the Go handler already sets X-Accel-Buffering: no for
          // nginx). Flush the headers as soon as the upstream responds with
          // text/event-stream so the stream is observably open immediately.
          proxy.on('proxyRes', (proxyRes, _request, response) => {
            const contentType = proxyRes.headers['content-type']
            if (typeof contentType === 'string' && contentType.includes('text/event-stream')) {
              // http-proxy writes the response head after this event fires, so
              // force the buffered headers out on the next tick, once they are
              // actually set, to make the stream observably open immediately.
              setImmediate(() => response.flushHeaders())
            }
          })
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    // e2e/ holds Playwright specs, run separately via `npm run test:e2e`.
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
  build: {
    // The Go service serves this directory in production (PLATFORM_WEB_DIR
    // or ./web/dist next to the working directory); keep the layout stable:
    // index.html at the root, hashed bundles under assets/.
    outDir: 'dist',
    emptyOutDir: true,
  },
})
