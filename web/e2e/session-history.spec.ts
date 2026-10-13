// End-to-end history replay and click-to-use. It runs the real Go backend with
// the mock opencode server and fake pi executable (installed on PATH by
// scripts/run-e2e.mjs), both of which persist a native transcript the adapters
// read back, and drives the UI:
//
//   - a reply produced in one page visit is still shown after a full page
//     reload, replayed from the agent's own transcript and continuing in the
//     same conversation as the resumed turn,
//   - a session whose instance was stopped becomes usable again by opening it:
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

function sessionRow(page: Page, title: string) {
  return page.locator('.session', { hasText: title })
}

async function createSession(page: Page, backendName: string, title: string) {
  // Scoped to the selected project row: the tree renders one ＋ per
  // registered project, and earlier specs register their own.
  await page.locator('.project-row.selected .project-new-session').click()
  await expect(page.getByRole('heading', { name: '新建会话' })).toBeVisible()
  await page.locator(`.backend-switch [data-backend="${backendName}"]`).click()
  await page.locator('#first-message').fill(title)
  await page.locator('form.create-session button.create').click()
  await expect(sessionRow(page, title)).toBeVisible()
}

async function openSessionTab(page: Page, title: string) {
  await sessionRow(page, title).click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible()
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')
  return conversation
}

async function send(page: Page, text: string) {
  await page.locator('#message-text').fill(text)
  await page.getByRole('button', { name: '发送', exact: true }).click()
}

test('a reply survives a page reload and continues in the same conversation', async ({
  page,
  request,
}) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'session-history')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, 'pi', 'pi-history')
  const conversation = await openSessionTab(page, 'pi-history')
  await send(page, 'remember this turn')
  await expect(conversation).toContainText('pi reply: remember this turn', { timeout: 20_000 })

  await createSession(page, 'opencode-v2', 'oc-history')
  const ocConversation = await openSessionTab(page, 'oc-history')
  await send(page, 'remember this opencode turn')
  await expect(ocConversation).toContainText('opencode reply: remember this opencode turn', {
    timeout: 20_000,
  })

  // A full reload throws away every in-browser buffer: what is shown next can
  // only have come from the agent's own transcript.
  await page.reload()
  const replayed = await openSessionTab(page, 'pi-history')

  await expect(replayed).toContainText('remember this turn', { timeout: 20_000 })
  await expect(replayed).toContainText('pi reply: remember this turn', { timeout: 20_000 })

  // Reasoning is replayed as its own collapsed block, above the reply.
  const piThinking = replayed.locator('[data-test="thinking"]')
  await expect(piThinking).toHaveCount(1)
  await expect(piThinking.locator('summary')).toContainText('Thinking')
  await expect(piThinking).toContainText('thinking about: remember this turn')

  // And the session is usable again: the replayed turn is not the end of the log.
  await send(page, 'and this one')
  await expect(replayed).toContainText('pi reply: and this one', { timeout: 20_000 })

  // The other backend's history comes from its own native log, reasoning
  // included.
  const replayedOC = await openSessionTab(page, 'oc-history')
  await expect(replayedOC).toContainText('remember this opencode turn', { timeout: 20_000 })
  const ocThinking = replayedOC.locator('[data-test="thinking"]')
  await expect(ocThinking.locator('summary')).toContainText('Thinking')
  await expect(ocThinking).toContainText('internal reasoning is not conversation')
})

test('opening a session whose instance was stopped re-attaches it without any manual start', async ({
  page,
  request,
}) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'session-resume')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, 'pi', 'pi-resume')
  const conversation = await openSessionTab(page, 'pi-resume')
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

  // Reload so the page starts from the session list alone, then open the session:
  // it must repair itself and accept a message.
  await page.reload()
  const resumed = await openSessionTab(page, 'pi-resume')
  await expect(resumed).toContainText('pi reply: first turn', { timeout: 20_000 })

  await send(page, 'after resume')
  await expect(resumed).toContainText('pi reply: after resume', { timeout: 20_000 })
})
