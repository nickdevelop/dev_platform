// End-to-end workbench workflow. It runs the real Go backend with the mock
// opencode server and fake pi executable (installed on PATH by
// scripts/run-e2e.mjs) concurrently in one project, then drives the UI:
//
//   - two agent instances in one project (pi and opencode-v2),
//   - sessions on each instance stream their own replies,
//   - events from one instance never render in another instance's view,
//   - stopping a session cascades to its same-instance siblings, while a
//     different instance's session stays active.
//
// Started via `npm run test:e2e`, which builds the server and provides
// E2E_PASSWORD and E2E_WORKSPACE_ROOT for an isolated scratch directory.
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type APIRequestContext } from '@playwright/test'

const password = process.env.E2E_PASSWORD
const workspaceRoot = process.env.E2E_WORKSPACE_ROOT

if (!password || !workspaceRoot) {
  throw new Error('run this spec through web/scripts/run-e2e.mjs, not plain playwright test')
}

// operatorLogin authenticates the request fixture and returns a CSRF token for
// the operator setup calls run outside the browser.
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

test('isolates streamed events and stop across agent instances in one project', async ({ page, request }) => {
  test.slow()

  // Operator setup: authorize the workspace root and register a fresh project
  // directory under it.
  const csrf = await operatorLogin(request)
  const addRoot = await request.post('/api/workspaces/roots', {
    data: { path: workspaceRoot },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect([201, 409], await addRoot.text()).toContain(addRoot.status())

  const projectDir = join(workspaceRoot, 'agent-workbench')
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }

  // Log in through the UI, then open the project's Agent settings.
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)

  await page.goto(`/projects/${project.id}/agents`)
  await expect(page.getByRole('heading', { name: 'Agent 实例设置' })).toBeVisible()

  // Start one pi instance and one opencode instance in the same project.
  await page.locator('#backend-select').selectOption('pi')
  await page.getByRole('button', { name: '启动实例' }).click()
  await expect(page.locator('.instance-item', { hasText: 'pi' })).toBeVisible({ timeout: 20_000 })

  await page.locator('#backend-select').selectOption('opencode-v2')
  await page.getByRole('button', { name: '启动实例' }).click()
  await expect(page.locator('.instance-item', { hasText: 'opencode-v2' })).toBeVisible({ timeout: 20_000 })

  // Open the unified session workbench.
  await page.getByRole('button', { name: '打开会话工作台' }).click()
  await expect(page).toHaveURL(new RegExp(`/projects/${project.id}/workbench$`))
  await expect(page.getByRole('heading', { name: '会话工作台' })).toBeVisible()

  // A pi session and two opencode sessions. Instances are listed in creation
  // order: index 0 is pi, index 1 is opencode-v2.
  await page.locator('#instance-select').selectOption({ index: 0 })
  await page.locator('#session-title').fill('pi-session')
  await page.getByRole('button', { name: '新建会话' }).click()
  await expect(page.locator('.session-item', { hasText: 'pi-session' })).toBeVisible()

  for (const title of ['oc-session-1', 'oc-session-2']) {
    await page.locator('#instance-select').selectOption({ index: 1 })
    await page.locator('#session-title').fill(title)
    await page.getByRole('button', { name: '新建会话' }).click()
    await expect(page.locator('.session-item', { hasText: title })).toBeVisible()
  }

  // The pi session streams its reply into its own conversation.
  await page.locator('.session-item', { hasText: 'pi-session' }).click()
  await page.waitForTimeout(400)
  await page.locator('#message-text').fill('ping-pi')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('.conversation')).toContainText('pi reply: ping-pi', { timeout: 20_000 })

  // The opencode session streams its own reply, and the pi event never leaks
  // into this instance's view.
  await page.locator('.session-item', { hasText: 'oc-session-1' }).click()
  await page.waitForTimeout(400)
  await page.locator('#message-text').fill('ping-oc')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('.conversation')).toContainText('opencode reply: ping-oc', {
    timeout: 20_000,
  })
  await expect(page.locator('.conversation')).not.toContainText('pi reply: ping-pi')

  // Switching back to the pi session shows only its own history.
  await page.locator('.session-item', { hasText: 'pi-session' }).click()
  await expect(page.locator('.conversation')).toContainText('pi reply: ping-pi')
  await expect(page.locator('.conversation')).not.toContainText('opencode reply: ping-oc')

  // Stop the opencode session. The backend stops at instance level: the same
  // instance's sibling is disconnected, while the pi instance is untouched.
  await page.locator('.session-item', { hasText: 'oc-session-1' }).click()
  await page.getByRole('button', { name: '停止运行' }).click()
  await expect(page.locator('.session-item', { hasText: 'oc-session-1' })).toContainText('已停止', {
    timeout: 20_000,
  })
  await expect(page.locator('.session-item', { hasText: 'oc-session-2' })).toContainText('已断开')
  await expect(page.locator('.session-item', { hasText: 'pi-session' })).toContainText('进行中')
})
