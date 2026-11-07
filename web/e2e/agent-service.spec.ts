// The machine-level opencode service, end to end. The platform treats it as a
// third-party process: it adopts whatever is running, never starts or stops it
// from a business path, and the only controls are on Platform Settings → Agent.
// This spec drives exactly that:
//
//   - settings shows the service the harness started, as 运行中,
//   - stopping it asks for confirmation first, and then it really stops,
//   - a session created while it is down is refused with the one sentence the
//     product chose, pointing at the settings page,
//   - starting it from that page makes session creation work again,
//
// and it leaves the service running for the specs that follow.
//
// Started via `npm run test:e2e`, which starts the mock service before the
// platform and gives both the same isolated XDG_STATE_HOME.
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import { projectRow } from './support/project-workbench'

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

async function registerProject(request: APIRequestContext, csrf: string, name: string): Promise<{ id: number; name: string }> {
  const projectDir = join(workspaceRoot, `${name}-${Date.now()}-${process.pid}`)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  return (await registered.json()) as { id: number; name: string }
}

async function uiLogin(page: Page) {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)
}

async function openAgentSettings(page: Page) {
  await page.goto('/settings/agents')
  await expect(page).toHaveURL(/\/settings\/agents$/)
  // Service controls live on the opencode backend tab; the page opens on the
  // general tab, so activate it explicitly instead of relying on stored state.
  await page.locator('[data-test="agent-tab-opencode-v2"]').click()
  await expect(page.locator('[data-test="service-state-opencode-v2"]')).toBeVisible()
}

// createOpencodeSession opens the project's own tree row and submits the
// new-session form. The old selected-project settings tab is gone, so the row
// is addressed by name instead of relying on global selection.
async function createOpencodeSession(page: Page, projectName: string, title: string) {
  await page.goto('/')
  const row = projectRow(page, projectName)
  await expect(row).toBeVisible()
  await row.locator('.project-new-session').click()
  await expect(page.getByRole('heading', { name: '新建会话' })).toBeVisible()
  await page.locator('[data-test="agent-switch"] [data-backend="opencode-v2"]').click()
  await page.locator('#first-message').fill(title)
  await page.locator('form.create-session button.create').click()
}

// serviceState reads the label the settings page renders for the opencode
// service. It reflects the last detection, which is exactly what the operator
// is shown.
async function serviceState(page: Page): Promise<string> {
  return (await page.locator('[data-test="service-state-opencode-v2"]').textContent())?.trim() ?? ''
}

test('the service is observed, stopped, refused, and started again by an operator', async ({
  page,
  request,
}) => {
  const csrf = await operatorLogin(request)
  const project = await registerProject(request, csrf, 'service-toggle')
  await uiLogin(page)

  await test.step('settings shows the running service with its details', async () => {
    await openAgentSettings(page)
    await expect(page.locator('[data-test="service-state-opencode-v2"]')).toContainText('运行中')
    await expect(page.locator('[data-test="service-endpoint"]')).toContainText('127.0.0.1')
    await expect(page.locator('[data-test="service-pid"]')).toContainText('进程')
    await expect(page.locator('[data-test="service-checked-at"]')).toContainText('最后检测于')
    // A backend with no service of its own gets no controls at all.
    await expect(page.locator('[data-test="service-state-pi"]')).toHaveCount(0)
    // Starting a running service is not offered.
    await expect(page.locator('[data-test="service-start"]')).toBeDisabled()
  })

  await test.step('a session works while the service runs', async () => {
    await createOpencodeSession(page, project.name, 'oc-running')
    await expect(page.locator('[data-test="conversation"]')).toBeVisible({ timeout: 20_000 })
  })

  await test.step('stopping asks first, then stops', async () => {
    await openAgentSettings(page)
    // Re-detect so the state on screen is current.
    await page.locator('[data-test="service-detect"]').click()
    await expect(page.locator('[data-test="service-state-opencode-v2"]')).toContainText('运行中')

    await page.locator('[data-test="service-stop"]').click()
    const dialog = page.locator('[data-test="service-stop-dialog"]')
    await expect(dialog).toContainText('桌面端 opencode')
    await expect(dialog).toContainText('正在运行的会话会被中断')
    // The confirmation is not the stop: nothing has happened yet.
    await expect(page.locator('[data-test="service-state-opencode-v2"]')).toContainText('运行中')

    await page.locator('[data-test="service-stop-confirm"]').click()
    await expect(page.locator('[data-test="service-state-opencode-v2"]')).toContainText('未启动', {
      timeout: 15_000,
    })
    await expect(page.locator('[data-test="service-stop"]')).toBeDisabled()
  })

  await test.step('a session is refused with the sentence that names the fix', async () => {
    await createOpencodeSession(page, project.name, 'oc-refused')

    const error = page.locator('[data-test="new-session-error"]')
    await expect(error).toContainText('opencode service 未启动，请到平台设置 → Agent 检测并启动。')
    await expect(page.locator('[data-test="new-session-service-link"]')).toBeVisible()
    // A refused session leaves nothing behind to clean up.
    await expect(page.locator('[data-test="conversation"]')).toHaveCount(0)
  })

  await test.step('the operator starts it and sessions work again', async () => {
    await openAgentSettings(page)
    await page.locator('[data-test="service-detect"]').click()
    await expect(page.locator('[data-test="service-state-opencode-v2"]')).toContainText('未启动')

    await page.locator('[data-test="service-start"]').click()
    await expect(page.locator('[data-test="service-state-opencode-v2"]')).toContainText('运行中', {
      timeout: 20_000,
    })

    await createOpencodeSession(page, project.name, 'oc-restored')
    await expect(page.locator('[data-test="conversation"]')).toBeVisible({ timeout: 20_000 })
  })

  // The state the suite started from: a registered, running service.
  await openAgentSettings(page)
  await page.locator('[data-test="service-detect"]').click()
  expect(await serviceState(page)).toBe('运行中')
})
