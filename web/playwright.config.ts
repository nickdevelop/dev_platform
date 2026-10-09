import { defineConfig } from '@playwright/test'

// Playwright runs against servers started by scripts/run-e2e.mjs: the Go
// backend on 127.0.0.1:8080 and the Vite dev server (same-origin /api proxy)
// on 127.0.0.1:5173. Run via `npm run test:e2e`.
export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:5173',
    headless: true,
    trace: 'retain-on-failure',
  },
})
