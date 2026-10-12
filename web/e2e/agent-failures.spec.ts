// End-to-end failure semantics. It runs the real Go backend with the mock
// opencode server and the fake pi executable (installed on PATH by
// scripts/run-e2e.mjs) and drives the UI:
//
//   - the diagnostics panel reports every backend with its detected and
//     tested version,
//   - a failing pi turn (the fake CLI exits non-zero on a marker prompt)
//     surfaces as a redacted error entry in that session's conversation,
//   - the opencode instance in the same project keeps streaming replies,
//     so one backend's failure never stops another,
//   - starting an unknown backend is rejected without touching the
//     project's existing instances.
//
// Started via `npm run test:e2e`, which builds the server and provides
// E2E_PASSWORD and E2E_WORKSPACE_ROOT for an isolated scratch directory.
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const password = process.env.E2E_PASSWORD
const workspaceRoot = process.env.E2E_WORKSPACE_ROOT

if (!password || !workspaceRoot) {
  throw new Error('run this spec through web/scripts/run-e2e.mjs, not plain playwright test')
}

// operatorLogin authenticates the request fixture and returns a CSRF token
// for the operator setup calls run outside the browser.
async function operatorLogin(request: APIRequestContext): Promise<string> {
  const preSession = await (await request.get('/api/auth/session')).json()
  const login = await request.post('/api/auth/login', {
    data: { password },
    headers: { 'X-CSRF-Token': preSession.csrfToken },
  })
  expect(login.status(), await login.text()).toBe(200)
  const session = await (await request.get('/api/auth/session')).json()
  return session.csrfToken as string
}

// registerProject registers a fresh project directory, returning its id.
async function registerProject(request: APIRequestContext, csrf: string): Promise<number> {
  const projectDir = join(workspaceRoot, `agent-failures-${Date.now()}-${process.pid}`)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }
  return project.id
}

// uiLogin drives the browser through the login form.
async function uiLogin(page: Page) {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)
}

// openSession selects a task in the tree and waits until its SSE stream is open.
async function openSession(page: Page, title: string) {
  await taskRow(page, title).click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')
}

function taskRow(page: Page, title: string) {
  return page.locator('.task', { hasText: title })
}

// createTask opens the project's new-task form from the tree, chooses the agent
// backend, and waits for the task to appear.
async function createTask(page: Page, backendName: string, title: string) {
  // Scoped to the selected project row: the tree renders one ＋ per
  // registered project, and earlier specs register their own.
  await page.locator('.project-row.selected .project-new-task').click()
  await expect(page.getByRole('heading', { name: '新建任务' })).toBeVisible()
  await page.locator('#backend-select').selectOption(backendName)
  await page.locator('#session-title').fill(title)
  await page.locator('form.create-session button.create').click()
  await expect(taskRow(page, title)).toBeVisible()
}

test('a failing pi turn never disturbs the opencode instance in the same project', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf)
  await uiLogin(page)

  // The diagnostics panel reports both backends with the versions their
  // fixtures mimic, inside the tested ranges.
  await page.goto(`/projects/${projectId}`)
  await expect(page.getByRole('region', { name: '项目设置' })).toBeVisible()
  const piReport = page.locator('[data-backend-diagnostics="pi"]')
  const ocReport = page.locator('[data-backend-diagnostics="opencode-v2"]')
  await expect(piReport.locator('[data-diagnostics-status]')).toHaveText('正常')
  await expect(ocReport.locator('[data-diagnostics-status]')).toHaveText('正常')
  await expect(piReport).toContainText('1.1.0')
  await expect(ocReport).toContainText('2.0.24')

  // One task per backend, created by choosing only the agent; the platform
  // starts whatever backend service each one needs.
  await createTask(page, 'pi', 'pi-failing')
  await createTask(page, 'opencode-v2', 'oc-healthy')

  // The pi turn fails: the fake CLI writes one stderr diagnostic and exits
  // non-zero. The error is surfaced in the failing session's conversation.
  await openSession(page, 'pi-failing')
  await page.locator('#message-text').fill('__e2e_fail__ please fail')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('[data-test="agent-error"]').first()).toContainText('provider unavailable', {
    timeout: 20_000,
  })

  // The opencode instance in the same project is untouched and streams its
  // own reply; the pi error never leaks into its conversation.
  await openSession(page, 'oc-healthy')
  await page.locator('#message-text').fill('ping-oc')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('[data-test="conversation"]')).toContainText('opencode reply: ping-oc', { timeout: 20_000 })
  await expect(page.locator('[data-test="conversation"]')).not.toContainText('provider unavailable')

  // Switching back to the pi session still shows its recorded failure.
  await openSession(page, 'pi-failing')
  await expect(page.locator('[data-test="agent-error"]').first()).toContainText('provider unavailable')

  // The diagnostics panel still reports both backends usable after the
  // failure: a failed turn is a session problem, not a broken backend.
  await page.goto(`/projects/${projectId}`)
  await expect(piReport.locator('[data-diagnostics-status]')).toHaveText('正常')
  await expect(ocReport.locator('[data-diagnostics-status]')).toHaveText('正常')
})

test('starting an unknown backend is rejected without touching existing instances', async ({ request }) => {
  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf)

  const started = await request.post(`/api/projects/${projectId}/instances`, {
    data: { backend: 'pi' },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(started.status(), await started.text()).toBe(201)
  const instance = (await started.json()) as { id: string; status: string }
  expect(instance.status).toBe('running')

  const rejected = await request.post(`/api/projects/${projectId}/instances`, {
    data: { backend: 'claude-code' },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(rejected.status()).toBe(400)
  expect(await rejected.json()).toEqual({ error: 'unknown_backend' })

  const listed = await request.get(`/api/projects/${projectId}/instances`)
  const { instances } = (await listed.json()) as { instances: Array<{ id: string; status: string }> }
  expect(instances).toHaveLength(1)
  expect(instances[0].id).toBe(instance.id)
  expect(instances[0].status).toBe('running')
})
