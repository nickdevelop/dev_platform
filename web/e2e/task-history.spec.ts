// End-to-end history replay and click-to-use. It runs the real Go backend with
// the mock opencode server and fake pi executable (installed on PATH by
// scripts/run-e2e.mjs), both of which persist a native transcript the adapters
// read back, and drives the UI:
//
//   - a reply produced in one page visit is still shown after a full page
//     reload, above the separator that marks where replayed history ends,
//   - a task whose instance was stopped becomes usable again by opening it:
//     the platform re-attaches the session and the composer accepts a message
//     without the user starting anything.
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

async function registerProject(request: APIRequestContext, csrf: string, name: string): Promise<number> {
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

  const projectDir = join(workspaceRoot, `${name}-${Date.now()}-${process.pid}`)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  return ((await registered.json()) as { id: number }).id
}

async function uiLogin(page: Page) {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)
}

function taskRow(page: Page, title: string) {
  return page.locator('.task', { hasText: title })
}

async function createTask(page: Page, backendName: string, title: string) {
  await page.locator('.new-task').click()
  await expect(page.getByRole('heading', { name: '新建任务' })).toBeVisible()
  await page.locator('#backend-select').selectOption(backendName)
  await page.locator('#session-title').fill(title)
  await page.locator('form.create-session button.create').click()
  await expect(taskRow(page, title)).toBeVisible()
}

async function openTask(page: Page, title: string) {
  await taskRow(page, title).click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')
  return conversation
}

async function send(page: Page, text: string) {
  await page.locator('#message-text').fill(text)
  await page.getByRole('button', { name: '发送', exact: true }).click()
}

test('a reply survives a page reload, shown above the history separator', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'task-history')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}`)

  await createTask(page, 'pi', 'pi-history')
  const conversation = await openTask(page, 'pi-history')
  await send(page, 'remember this turn')
  await expect(conversation).toContainText('pi reply: remember this turn', { timeout: 20_000 })

  await createTask(page, 'opencode-v2', 'oc-history')
  const ocConversation = await openTask(page, 'oc-history')
  await send(page, 'remember this opencode turn')
  await expect(ocConversation).toContainText('opencode reply: remember this opencode turn', {
    timeout: 20_000,
  })

  // A full reload throws away every in-browser buffer: what is shown next can
  // only have come from the agent's own transcript.
  await page.reload()
  const replayed = await openTask(page, 'pi-history')

  await expect(replayed).toContainText('remember this turn', { timeout: 20_000 })
  await expect(replayed).toContainText('pi reply: remember this turn', { timeout: 20_000 })
  await expect(replayed.locator('[data-test="history-separator"]')).toBeVisible()

  // Reasoning is replayed as its own collapsed block, above the reply.
  const piThinking = replayed.locator('[data-test="thinking"]')
  await expect(piThinking).toHaveCount(1)
  await expect(piThinking.locator('summary')).toContainText('已思考')
  await expect(piThinking).toContainText('thinking about: remember this turn')

  // And the task is usable again: the replayed turn is not the end of the log.
  await send(page, 'and this one')
  await expect(replayed).toContainText('pi reply: and this one', { timeout: 20_000 })

  // The other backend's history comes from its own native log, reasoning
  // included.
  const replayedOC = await openTask(page, 'oc-history')
  await expect(replayedOC).toContainText('remember this opencode turn', { timeout: 20_000 })
  await expect(replayedOC.locator('[data-test="history-separator"]')).toBeVisible()
  const ocThinking = replayedOC.locator('[data-test="thinking"]')
  await expect(ocThinking.locator('summary')).toContainText('已思考')
  await expect(ocThinking).toContainText('internal reasoning is not conversation')
})

test('opening a task whose instance was stopped re-attaches it without any manual start', async ({
  page,
  request,
}) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'task-resume')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}`)

  await createTask(page, 'pi', 'pi-resume')
  const conversation = await openTask(page, 'pi-resume')
  await send(page, 'first turn')
  await expect(conversation).toContainText('pi reply: first turn', { timeout: 20_000 })

  // Stop the project's running instances through the operator API, which is
  // what a platform restart or an operator action leaves behind.
  const instances = (await (await request.get(`/api/projects/${projectId}/instances`)).json()) as {
    instances: Array<{ id: string; status: string }>
  }
  const running = instances.instances.filter((instance) => instance.status === 'running')
  expect(running.length).toBeGreaterThan(0)
  for (const instance of running) {
    const stopped = await request.post(
      `/api/projects/${projectId}/instances/${encodeURIComponent(instance.id)}/stop`,
      { headers: { 'X-CSRF-Token': csrf } },
    )
    expect(stopped.status(), await stopped.text()).toBe(200)
  }

  // Reload so the page starts from the session list alone, then open the task:
  // it must repair itself and accept a message.
  await page.reload()
  const resumed = await openTask(page, 'pi-resume')
  await expect(resumed).toContainText('pi reply: first turn', { timeout: 20_000 })

  await send(page, 'after resume')
  await expect(resumed).toContainText('pi reply: after resume', { timeout: 20_000 })
})
