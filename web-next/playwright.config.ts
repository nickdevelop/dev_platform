import { defineConfig } from '@playwright/test'

// Playwright runs against servers started by scripts/run-e2e.mjs: the Go
// backend (127.0.0.1:8080) and the Vite dev server (same-origin /api proxy).
// Run via `npm run test:e2e`; the dev server port follows E2E_VITE_PORT so an
// occupied 5173 does not wedge the whole run (that variable also drives the
// harness's port choice in scripts/run-e2e.mjs).
const VITE_PORT = process.env.E2E_VITE_PORT ?? '15178'

export default defineConfig({
  testDir: './e2e',
  // Every spec drives the same real backend, SQLite database and machine-wide
  // opencode service. Some specs stop or start that service, so running files
  // in parallel makes unrelated specs race against it. Keep the suite serial.
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list']],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? `http://127.0.0.1:${VITE_PORT}`,
    headless: true,
    trace: 'retain-on-failure',
  },
})
