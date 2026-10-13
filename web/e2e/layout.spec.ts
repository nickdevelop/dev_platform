// End-to-end layout check for the conversation pane. The requirement is a single
// scroller: a long turn must scroll inside the message log, never the document
// and never the right pane (which would show as a second scrollbar beside the
// log's own). The unit test asserts the shell class; this proves it in a browser.
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

test('a long turn scrolls the message log, not the page or the outer pane', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'layout')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, 'pi', 'pi-layout')
  await sessionRow(page, 'pi-layout').click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')

  // A long prompt produces a reply taller than the message viewport, so every
  // overflow decision is exercised.
  const long = 'scroll-me '.repeat(400)
  await page.locator('#message-text').fill(long)
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(conversation).toContainText('pi reply: scroll-me', { timeout: 20_000 })

  const metrics = await page.evaluate(() => {
    const doc = document.documentElement
    const pane = document.querySelector('.pane') as HTMLElement
    const log = document.querySelector('[data-test="conversation-scroll"]') as HTMLElement
    return {
      docScrolls: doc.scrollHeight > doc.clientHeight + 1,
      bodyOverflow: getComputedStyle(document.body).overflowY,
      paneScrolls: pane.scrollHeight > pane.clientHeight + 1,
      paneOverflow: getComputedStyle(pane).overflowY,
      logScrolls: log.scrollHeight > log.clientHeight + 1,
      logOverflow: getComputedStyle(log).overflowY,
    }
  })

  // The page itself and the outer pane never scroll; the message log is the one
  // and only scroller.
  expect(metrics.docScrolls).toBe(false)
  expect(metrics.bodyOverflow).toBe('hidden')
  expect(metrics.paneScrolls).toBe(false)
  expect(metrics.paneOverflow).toBe('hidden')
  expect(metrics.logScrolls).toBe(true)
  expect(metrics.logOverflow).toBe('auto')
})

// Regression: a long session tree must scroll inside the sidebar, never stretch
// the shell grid. An unconstrained implicit grid row sized to the tree's
// content, which pushed the full-height conversation (and its composer) below
// the fold while body overflow:hidden hid every scrollbar.
test('a long session tree scrolls the sidebar and keeps the composer in view', async ({ page, request }) => {
  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'layout-tree')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, 'pi', 'pi-tree')
  await sessionRow(page, 'pi-tree').click()
  await expect(page.locator('[data-test="conversation"]')).toHaveAttribute('data-stream-status', 'open')

  // Fill the tree well past the viewport. The grid rule under test is purely
  // layout, so synthetic rows exercise it without paying for 300 real sessions.
  await page.evaluate(() => {
    const tree = document.querySelector('.tree') as HTMLElement
    tree.innerHTML += Array.from({ length: 300 }, (_, i) => `<div>synthetic session ${i}</div>`).join('')
  })

  const metrics = await page.evaluate(() => {
    const doc = document.documentElement
    const tree = document.querySelector('.tree') as HTMLElement
    const composer = document.querySelector('#message-text') as HTMLElement
    const box = composer.getBoundingClientRect()
    return {
      docScrolls: doc.scrollHeight > doc.clientHeight + 1,
      treeScrolls: tree.scrollHeight > tree.clientHeight + 1,
      treeOverflow: getComputedStyle(tree).overflowY,
      composerInViewport: box.bottom <= innerHeight && box.top >= 0 && box.height > 0,
    }
  })

  expect(metrics.docScrolls).toBe(false)
  expect(metrics.treeScrolls).toBe(true)
  expect(metrics.treeOverflow).toBe('auto')
  expect(metrics.composerInViewport).toBe(true)
})

// Regression: the step group that wraps reasoning/tool activity was sized
// width:100% plus its padding under the default content-box, so it stuck out
// past the log's content box and the conversation grew a horizontal
// scrollbar. A folded group must still sit inside the log.
test('a step group never adds a horizontal scroller to the conversation', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectId = await registerProject(request, csrf, 'layout-steps')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, 'opencode-v2', 'oc-steps')
  await sessionRow(page, 'oc-steps').click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')

  await page.locator('#message-text').fill('step group please')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(conversation).toContainText('opencode reply: step group please', { timeout: 20_000 })
  // The reply's reasoning forms a step group, which is the box under test.
  await expect(conversation.locator('[data-test="step-group"]')).toBeVisible()

  const metrics = await page.evaluate(() => {
    const log = document.querySelector('[data-test="conversation-scroll"]') as HTMLElement
    const group = document.querySelector('[data-test="step-group"]') as HTMLElement
    return {
      logScrollOverflow: log.scrollWidth - log.clientWidth,
      groupWidth: group.getBoundingClientRect().width,
      logWidth: log.getBoundingClientRect().width,
    }
  })
  expect(metrics.logScrollOverflow).toBeLessThanOrEqual(0)
  expect(metrics.groupWidth).toBeLessThanOrEqual(Math.ceil(metrics.logWidth) + 1)
})
