// Regression: the settings center hides the project sidebar, so the category
// rail is the only way out. It used to render「项目」as an inert button, which
// trapped the user in /settings with nothing but 设置 and 退出 to click. This
// proves the real browser flow: platform → settings → back to the project area
// → open a project page.
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
): Promise<number> {
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

test('the project category escapes the settings center back to the project area', async ({
  page,
  request,
}) => {
  const csrf = await operatorLogin(request)
  await registerProject(request, csrf, 'rail')
  await uiLogin(page)

  await page.getByRole('link', { name: '设置' }).click()
  await expect(page).toHaveURL(/\/settings\/appearance$/)
  await expect(page.getByRole('navigation', { name: '设置分类' })).toBeVisible()

  await page.getByRole('link', { name: '项目' }).click()
  await expect(page).toHaveURL(/\/$/)

  const sidebar = page.getByRole('complementary', { name: '项目导航' })
  await expect(sidebar).toBeVisible()
  await sidebar.locator('.project-settings').first().click()
  await expect(page.getByRole('region', { name: '项目设置' })).toBeVisible()
})
