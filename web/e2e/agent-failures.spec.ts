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

// registerProject authorizes the workspace root (only when it is not
// registered yet, so a second test in the same run does not provoke a
// constraint error) and registers a fresh project, returning its id.
async function registerProject(request: APIRequestContext, csrf: string): Promise<number> {
  const roots = (await (await request.get('/api/workspaces/roots')).json()) as {
    roots: Array<{ path: string }>
  }
  if (!roots.roots.some((root) => root.path === workspaceRoot)) {
    const addRoot = await request.post('/api/workspaces/roots', {
      data: { path: workspaceRoot },
      headers: { 'X-CSRF-Token': csrf },
    })
    expect([201, 409], await addRoot.text()).toContain(addRoot.status())
  }

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

// openSession selects a session row and waits until its SSE stream is open.
async function openSession(page: Page, title: string) {
  const row = page.locator('.session-item', { hasText: title })
  await row.click()
  const sessionId = await row.getAttribute('data-session-id')
  expect(sessionId).toBeTruthy()
  const conversation = page.locator('.conversation')
  await expect(conversation).toHaveAttribute('data-stream-session', sessionId as string)
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')
}

test('a failing pi turn never disturbs the opencode instance in the same project', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf)
  await uiLogin(page)

  // The diagnostics panel reports both backends with the versions their
  // fixtures mimic, inside the tested ranges.
  await page.goto(`/projects/${projectId}/agents`)
  await expect(page.getByRole('heading', { name: 'Agent 实例设置' })).toBeVisible()
  const piReport = page.locator('[data-backend-diagnostics="pi"]')
  const ocReport = page.locator('[data-backend-diagnostics="opencode-v2"]')
  await expect(piReport.locator('[data-diagnostics-status]')).toHaveText('正常')
  await expect(ocReport.locator('[data-diagnostics-status]')).toHaveText('正常')
  await expect(piReport).toContainText('1.1.0')
  await expect(ocReport).toContainText('2.0.24')

  // Start one instance per backend.
  await page.locator('#backend-select').selectOption('pi')
  await page.getByRole('button', { name: '启动实例' }).click()
  await expect(page.locator('.instance-item', { hasText: 'pi' })).toBeVisible({ timeout: 20_000 })
  await page.locator('#backend-select').selectOption('opencode-v2')
  await page.getByRole('button', { name: '启动实例' }).click()
  await expect(page.locator('.instance-item', { hasText: 'opencode-v2' })).toBeVisible({ timeout: 20_000 })

  // One session per instance in the workbench.
  await page.getByRole('button', { name: '打开会话工作台' }).click()
  await expect(page.getByRole('heading', { name: '会话工作台' })).toBeVisible()
  await page.locator('#instance-select').selectOption({ index: 0 })
  await page.locator('#session-title').fill('pi-failing')
  await page.getByRole('button', { name: '新建会话' }).click()
  await expect(page.locator('.session-item', { hasText: 'pi-failing' })).toBeVisible()
  await page.locator('#instance-select').selectOption({ index: 1 })
  await page.locator('#session-title').fill('oc-healthy')
  await page.getByRole('button', { name: '新建会话' }).click()
  await expect(page.locator('.session-item', { hasText: 'oc-healthy' })).toBeVisible()

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
  await expect(page.locator('.conversation')).toContainText('opencode reply: ping-oc', { timeout: 20_000 })
  await expect(page.locator('.conversation')).not.toContainText('provider unavailable')

  // Switching back to the pi session still shows its recorded failure.
  await openSession(page, 'pi-failing')
  await expect(page.locator('[data-test="agent-error"]').first()).toContainText('provider unavailable')

  // The diagnostics panel still reports both backends usable after the
  // failure: a failed turn is a session problem, not a broken backend.
  await page.goto(`/projects/${projectId}/agents`)
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
