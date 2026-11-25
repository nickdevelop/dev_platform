/// <reference types="vitest/config" />
import { configDefaults, defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// Same-origin /api in development; in production the Go service and the built
// frontend are served from one origin. E2E runs move the backend to another
// port and point the proxy at it via E2E_API_PROXY.
const apiTarget = process.env.E2E_API_PROXY ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [
    vue(),
    // Element Plus is resolved per component: a template that writes <el-button>
    // pulls in only that component's ES module, so the bundle carries the
    // widgets the platform actually uses instead of the whole kit. Styles come
    // separately and in one file (see main.ts), because the function-style APIs
    // (ElMessage, ElMessageBox, ElLoading) are not visible to a template
    // resolver and would otherwise arrive unstyled.
    // Local components keep their explicit imports (`dirs: []`), so nothing in
    // `shared/` or `features/` silently becomes globally registered.
    // The generated declaration file is committed under src/types so `vue-tsc`
    // type-checks `<el-*>` templates without a dev server run.
    Components({
      dirs: [],
      resolvers: [ElementPlusResolver({ noStyles: true })],
      dts: 'src/types/components.d.ts',
    }),
  ],
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        ws: true,
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
    setupFiles: ['./src/tests/setup/element-plus.ts'],
    // Element Plus resolves its widgets' stylesheets at import time. Left
    // externalized, Vitest hands those `.css` imports to Node, which cannot
    // parse them; inlining routes them through Vite, where CSS resolves to an
    // empty module (styles are not under test).
    server: {
      deps: {
        inline: ['element-plus'],
      },
    },
    // e2e/ holds Playwright specs, run separately via `npm run test:e2e`.
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
  build: {
    // The Go service serves this directory in production (PLATFORM_WEB_DIR
    // or ./web/dist next to the working directory); keep the layout stable:
    // index.html at the root, hashed bundles under assets/.
    outDir: 'dist',
    emptyOutDir: true,
    // Never inline a font slice into the stylesheet as base64, not even the
    // small ones: the unicode-range split exists so the browser fetches a slice
    // once and caches it, and a data URL would bury all 123 of them in the CSS
    // download instead.
    assetsInlineLimit: (filePath) => (filePath.endsWith('.woff2') ? false : undefined),
    // PlantUML core ships a bundled WebAssembly runtime (3.6MB + 1.4MB);
    // bound the chunk size warning limit so it does not trigger spurious build warnings.
    chunkSizeWarningLimit: 4000,
  },
})
