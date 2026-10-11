// End-to-end workbench workflow. It runs the real Go backend with the mock
// opencode server and fake pi executable (installed on PATH by
// scripts/run-e2e.mjs) concurrently in one project, then drives the UI:
//
//   - two agent instances in one project (pi and opencode-v2),
//   - tasks are created by choosing only an agent backend,
//   - sessions on each instance stream their own replies,
//   - events from one instance never render in another instance's view,
//   - a running turn offers 停止, which interrupts only that turn and returns
//     the session to idle while its siblings stay untouched.
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

// openSession selects a task in the tree and waits until its SSE stream is open.
// The conversation section exposes data-stream-session/-status so the wait is
// on an observable signal instead of a fixed timeout.
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
  await page.locator('.new-task').click()
  await expect(page.getByRole('heading', { name: '新建任务' })).toBeVisible()
  await page.locator('#backend-select').selectOption(backendName)
  await page.locator('#session-title').fill(title)
  await page.locator('form.create-session button.create').click()
  await expect(taskRow(page, title)).toBeVisible()
}

test('isolates streamed events and interrupts one turn across agent instances', async ({ page, request }) => {
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

  await page.goto(`/projects/${project.id}`)
  await expect(page.getByRole('region', { name: '项目设置' })).toBeVisible()

  // Tasks are created by choosing only the agent backend; the platform starts
  // whatever backend service each one needs.
  await createTask(page, 'pi', 'pi-session')
  await createTask(page, 'opencode-v2', 'oc-session-1')
  await createTask(page, 'opencode-v2', 'oc-hold')

  // The pi session streams its reply into its own conversation.
  await openSession(page, 'pi-session')
  await page.locator('#message-text').fill('ping-pi')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('[data-test="conversation"]')).toContainText('pi reply: ping-pi', { timeout: 20_000 })

  // The opencode session streams its own reply, and the pi event never leaks
  // into this instance's view.
  await openSession(page, 'oc-session-1')
  await page.locator('#message-text').fill('ping-oc')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('[data-test="conversation"]')).toContainText('opencode reply: ping-oc', {
    timeout: 20_000,
  })
  await expect(page.locator('[data-test="conversation"]')).not.toContainText('pi reply: ping-pi')

  // Switching back to the pi session shows only its own history.
  await openSession(page, 'pi-session')
  await expect(page.locator('[data-test="conversation"]')).toContainText('pi reply: ping-pi')
  await expect(page.locator('[data-test="conversation"]')).not.toContainText('opencode reply: ping-oc')

  // A held turn keeps the session running; the composer offers 停止. Stopping
  // interrupts only this turn, so the tree stops showing 运行中 and the
  // composer returns to 发送.
  await openSession(page, 'oc-hold')
  await page.locator('#message-text').fill('__e2e_hold__ keep this turn open')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(taskRow(page, 'oc-hold')).toContainText('运行中', { timeout: 20_000 })
  await page.getByRole('button', { name: '停止', exact: true }).click()
  await expect(taskRow(page, 'oc-hold')).not.toContainText('运行中', { timeout: 20_000 })
  await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible()

  // The other sessions' turns are untouched.
  await expect(taskRow(page, 'oc-session-1')).not.toContainText('断开')
  await expect(taskRow(page, 'pi-session')).not.toContainText('断开')
})

test('deletes a task from the conversation header after confirmation', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const addRoot = await request.post('/api/workspaces/roots', {
    data: { path: workspaceRoot },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect([201, 409], await addRoot.text()).toContain(addRoot.status())

  const projectDir = join(workspaceRoot, `delete-task-${Date.now()}-${process.pid}`)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }

  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)
  await page.goto(`/projects/${project.id}`)

  await createTask(page, 'pi', 'pi-delete')
  await openSession(page, 'pi-delete')

  // Cancelling the confirmation leaves the task in place.
  await page.locator('[data-test="delete-task"]').click()
  const dialog = page.locator('[data-test="delete-task-dialog"]')
  await expect(dialog).toBeVisible()
  await dialog.locator('.confirm-cancel').click()
  await expect(dialog).toBeHidden()
  await expect(taskRow(page, 'pi-delete')).toBeVisible()

  // Confirming removes it and returns to the project settings.
  await page.locator('[data-test="delete-task"]').click()
  await dialog.locator('[data-test="confirm-delete"]').click()
  await expect(page.getByRole('region', { name: '项目设置' })).toBeVisible()
  await expect(taskRow(page, 'pi-delete')).toHaveCount(0)
})
