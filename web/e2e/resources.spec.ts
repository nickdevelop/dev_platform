// End-to-end resource synchronization. It runs the real Go backend and drives
// the UI to:
//
//   - create an MCP resource with a write-only credential and associate it,
//   - preview and apply the managed files,
//   - edit a managed file outside the platform and see the sync pause on the
//     external-drift conflict,
//   - resolve the conflict by explicitly overwriting the target.
//
// The credential value must never appear in the UI.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const password = process.env.E2E_PASSWORD
const workspaceRoot = process.env.E2E_WORKSPACE_ROOT

if (!password || !workspaceRoot) {
  throw new Error('run this spec through web/scripts/run-e2e.mjs, not plain playwright test')
}

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

async function login(page: Page) {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)
}

test('synchronizes a resource and pauses on external drift', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const addRoot = await request.post('/api/workspaces/roots', {
    data: { path: workspaceRoot },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect([201, 409], await addRoot.text()).toContain(addRoot.status())

  const projectDir = join(workspaceRoot, `resource-sync-${Date.now()}-${process.pid}`)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }

  // Create an MCP resource with a write-only credential. The response carries
  // only the opaque reference.
  const secretValue = 'e2e-secret-value'
  const created = await request.post('/api/resources', {
    data: {
      kind: 'mcp',
      name: 'e2e-server',
      content: '{"command":"npx","args":["-y","e2e-server"]}',
      enabled: true,
      secretValue,
    },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(created.status(), await created.text()).toBe(201)
  const resource = (await created.json()) as { id: number; secretRef: string }
  expect(resource.secretRef).toBeTruthy()
  expect(await created.text()).not.toContain(secretValue)

  const associated = await request.post(`/api/projects/${project.id}/resources`, {
    data: { resourceId: resource.id },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(associated.status(), await associated.text()).toBe(204)

  await login(page)
  await page.goto(`/projects/${project.id}`)
  await expect(page.getByRole('region', { name: '资源关联' })).toBeVisible()
  await expect(page.getByText('e2e-server')).toBeVisible()
  // The credential value never renders in the UI.
  await expect(page.locator('body')).not.toContainText(secretValue)

  // Preview and apply: the target files are created and owned.
  await page.locator('[data-test="preview-sync"]').click()
  await expect(page.locator('[data-test="sync-preview"] .preview-item[data-state="pending"]').first()).toBeVisible()
  await page.locator('[data-test="apply-sync"]').click()
  await expect(page.locator('[data-test="sync-preview"] .preview-item[data-state="applied"]').first()).toBeVisible()

  const piTarget = join(projectDir, '.pi', 'mcp', 'e2e-server.json')
  await expect
    .poll(() => readFileSync(piTarget, 'utf8'), { timeout: 10_000 })
    .toContain(secretValue)

  // Edit the managed file outside the platform; the sync must pause.
  writeFileSync(piTarget, '{"tampered":true}\n')
  await page.locator('[data-test="preview-sync"]').click()
  await expect(page.locator('[data-test="sync-preview"] .preview-item[data-state="conflict"]').first()).toBeVisible()

  // Resolve the conflict by explicitly overwriting the target.
  await page.locator(`[data-test="resolve-${resource.id}"]`).first().click()
  await expect(page.locator('[data-test="conflict-dialog"]')).toBeVisible()
  await page.locator('[data-test="conflict-confirm"]').click()
  await expect(page.locator('[data-test="sync-preview"] .preview-item[data-state="applied"]').first()).toBeVisible()
  await expect.poll(() => readFileSync(piTarget, 'utf8'), { timeout: 10_000 }).toContain(secretValue)
})
