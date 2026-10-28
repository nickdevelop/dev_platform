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

async function registerProject(
  request: APIRequestContext,
  csrf: string,
  name: string,
): Promise<{ id: number; name: string }> {
  const projectName = `${name}-${Date.now()}-${process.pid}`
  const projectDir = join(workspaceRoot, projectName)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  return { id: ((await registered.json()) as { id: number }).id, name: projectName }
}

function projectRow(page: Page, projectName: string) {
  return page.locator('.project-row', {
    has: page.locator('.project-name', { hasText: projectName }),
  })
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

async function createSession(
  page: Page,
  projectName: string,
  backendName: string,
  title: string,
) {
  // Scope the ＋ to the project this test just registered: earlier specs
  // register their own projects in the same isolated workspace.
  await projectRow(page, projectName).locator('.project-new-session').click()
  await expect(page.getByRole('heading', { name: '新建会话' })).toBeVisible()
  await page.locator(`.backend-switch [data-backend="${backendName}"]`).click()
  await page.locator('#first-message').fill(title)
  await page.locator('form.create-session button.create').click()
  await expect(sessionRow(page, title)).toBeVisible()
}

test('a long turn scrolls the message log, not the page or the outer pane', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const { id: projectId, name: projectName } = await registerProject(request, csrf, 'layout')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, projectName, 'pi', 'pi-layout')
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
  const { id: projectId, name: projectName } = await registerProject(request, csrf, 'layout-tree')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, projectName, 'pi', 'pi-tree')
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

test('both rails stay in view when the file tree and preview are open together', async ({
  page,
  request,
}) => {
  const csrf = await operatorLogin(request)
  const { id: projectId, name: projectName } = await registerProject(request, csrf, 'layout-rails')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, projectName, 'pi', 'pi-rails')
  await sessionRow(page, 'pi-rails').click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')

  // The default file-tree drawer is docked; open the preview from the right rail.
  const previewBtn = conversation.locator('[data-test="toggle-preview-btn"]')
  await expect(previewBtn).toHaveAttribute('aria-pressed', 'false')
  await previewBtn.click()
  await expect(previewBtn).toHaveAttribute('aria-pressed', 'true')
  await expect(conversation.locator('[data-test="activity-pane"]')).toBeVisible()

  const metrics = await page.evaluate(() => {
    const split = document.querySelector('[data-test="conversation-split"]') as HTMLElement
    const composer = document.querySelector('.message-column form.send') as HTMLElement
    const rightRail = document.querySelector('[data-test="chat-side-rail-right"]') as HTMLElement
    const preview = document.querySelector('[data-test="activity-pane"]') as HTMLElement
    return {
      splitLeft: split.getBoundingClientRect().left,
      splitScrolls: split.scrollWidth > split.clientWidth + 1,
      previewRight: preview.getBoundingClientRect().right,
      rightRailLeft: rightRail.getBoundingClientRect().left,
      rightRailRight: rightRail.getBoundingClientRect().right,
      composerRight: composer.getBoundingClientRect().right,
      composerBottom: composer.getBoundingClientRect().bottom,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
    }
  })

  // The rail is never pushed past the viewport, and the composer stays inside
  // the visible conversation area instead of being clipped by the panels.
  expect(metrics.splitScrolls).toBe(false)
  expect(metrics.rightRailLeft).toBeGreaterThanOrEqual(metrics.splitLeft)
  expect(metrics.rightRailRight).toBeLessThanOrEqual(metrics.viewportWidth + 1)
  expect(metrics.previewRight).toBeLessThanOrEqual(metrics.rightRailLeft + 1)
  expect(metrics.composerRight).toBeLessThanOrEqual(metrics.previewRight + 1)
  expect(metrics.composerBottom).toBeLessThanOrEqual(metrics.viewportHeight + 1)

  // The same right-rail button collapses the preview again.
  await previewBtn.click()
  await expect(previewBtn).toHaveAttribute('aria-pressed', 'false')
  await expect(conversation.locator('[data-test="activity-pane"]')).toHaveCount(0)
})

// Regression: the activity card that wraps reasoning/tool output was sized
// width:100% plus its padding under the default content-box, so it stuck out
// past its scroller's content box and the pane grew a horizontal scrollbar. A
// flat activity card must still sit inside the activity pane.
test('an activity card never adds a horizontal scroller to the activity pane', async ({
  page,
  request,
}) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const { id: projectId, name: projectName } = await registerProject(request, csrf, 'layout-steps')
  await uiLogin(page)
  await page.goto(`/projects/${projectId}/settings`)

  await createSession(page, projectName, 'opencode-v2', 'oc-steps')
  await sessionRow(page, 'oc-steps').click()
  const conversation = page.locator('[data-test="conversation"]')
  await expect(conversation).toHaveAttribute('data-stream-status', 'open')
  await expect(conversation.locator('[data-test="chat-side-rail-left"]')).toBeVisible()
  await expect(conversation.locator('[data-test="chat-side-rail-right"]')).toBeVisible()

  await page.locator('#message-text').fill('step group please')
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(conversation).toContainText('opencode reply: step group please', { timeout: 20_000 })
  // If preview pane was collapsed by default, open it so activity-scroll is mounted.
  const expandBtn = conversation.locator('[data-test="toggle-preview-btn"]')
  await expect(expandBtn).toBeVisible()
  if ((await expandBtn.getAttribute('aria-pressed')) === 'false') {
    await expandBtn.click()
  }
  await expect(expandBtn).toHaveAttribute('aria-pressed', 'true')
  // The reply's reasoning lands in the flat activity list, which is the box
  // under test. Each turn contributes its own reasoning block, so the card under
  // test is the first one — the same element the geometry check below reads.
  await expect(conversation.locator('[data-test="activity-scroll"] [data-test="thinking"]').first()).toBeVisible()

  const metrics = await page.evaluate(() => {
    const log = document.querySelector('[data-test="activity-scroll"]') as HTMLElement
    const group = document.querySelector('[data-test="activity-scroll"] [data-test="thinking"]') as HTMLElement
    return {
      logScrollOverflow: log.scrollWidth - log.clientWidth,
      groupWidth: group.getBoundingClientRect().width,
      logWidth: log.getBoundingClientRect().width,
    }
  })
  expect(metrics.logScrollOverflow).toBeLessThanOrEqual(0)
  expect(metrics.groupWidth).toBeLessThanOrEqual(Math.ceil(metrics.logWidth) + 1)
})
