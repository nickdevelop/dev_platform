// End-to-end lifecycle: LAN-style login through the real UI against the real
// Go backend, then a full project lifecycle (register, create, search,
// rename, move, unregister, logout) inside a temporary scratch directory.
// Filesystem effects are asserted directly from the Node side. The platform
// never deletes project directories, so every removal here must leave them
// on disk.
//
// Started via `npm run test:e2e`, which provides E2E_PASSWORD and
// E2E_WORKSPACE_ROOT pointing at an isolated scratch directory; the password
// is a throwaway value created for this run only.
import { existsSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'

const password = process.env.E2E_PASSWORD
const workspaceRoot = process.env.E2E_WORKSPACE_ROOT

if (!password || !workspaceRoot) {
  throw new Error('run this spec through web/scripts/run-e2e.mjs, not plain playwright test')
}

test('login and drive a project through its full lifecycle', async ({ page, request }) => {
  // Authorized workspace content: one existing directory to register, and
  // one parent directory that a project can be moved into.
  mkdirSync(join(workspaceRoot, 'alpha'))
  mkdirSync(join(workspaceRoot, 'archive'))

  // Operator setup: the `request` fixture has its own cookie jar, so the
  // browser context below stays unauthenticated and the login flow is still
  // exercised through the UI.
  const preSession = await (await request.get('/api/auth/session')).json()
  const preLogin = await request.post('/api/auth/login', {
    data: { password },
    headers: { 'X-CSRF-Token': preSession.csrfToken },
  })
  expect(preLogin.status(), await preLogin.text()).toBe(200)

  // Unauthenticated users land on the login view.
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)

  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)

  const settings = page.getByRole('region', { name: '项目设置' })
  const row = (name: string) => page.locator('.project-row', { hasText: name })
  // A row click only expands/collapses the project; the gear opens settings.
  const openSettings = (name: string) => row(name).locator('.project-settings').click()

  // Register an existing directory.
  await page.getByRole('button', { name: '注册', exact: true }).click()
  const registerDialog = page.getByRole('dialog')
  await registerDialog.getByLabel('目录的绝对路径').fill(join(workspaceRoot, 'alpha'))
  await registerDialog.getByRole('button', { name: '注册', exact: true }).click()
  const alphaItem = row('alpha')
  await expect(alphaItem).toBeVisible()
  await openSettings('alpha')
  await expect(settings).toContainText(join(workspaceRoot, 'alpha'))

  // Create a brand-new project directory.
  await page.getByRole('button', { name: '新建', exact: true }).click()
  const createDialog = page.getByRole('dialog')
  await createDialog.getByLabel('父目录的绝对路径').fill(workspaceRoot)
  await createDialog.getByLabel('项目目录名').fill('beta')
  await createDialog.getByRole('button', { name: '创建', exact: true }).click()
  const betaItem = row('beta')
  await expect(betaItem).toBeVisible()
  expect(existsSync(join(workspaceRoot, 'beta'))).toBe(true)

  // Search matches project names: other projects are hidden, the open one stays.
  await page.getByLabel('搜索项目或会话').fill('alp')
  await expect(betaItem).toBeHidden()
  await expect(alphaItem).toBeVisible()
  await page.getByLabel('搜索项目或会话').fill('')

  // Rename requires typing the project's current name.
  await openSettings('beta')
  await settings.getByRole('button', { name: '重命名' }).click()
  const renameDialog = page.getByRole('dialog')
  await renameDialog.getByLabel('新目录名').fill('gamma')
  await renameDialog.getByLabel(/输入当前项目名/).fill('beta')
  await renameDialog.getByRole('button', { name: '重命名', exact: true }).click()
  const gammaItem = row('gamma')
  await expect(gammaItem).toBeVisible()
  expect(existsSync(join(workspaceRoot, 'gamma'))).toBe(true)
  expect(existsSync(join(workspaceRoot, 'beta'))).toBe(false)

  // Move requires the typed confirmation as well.
  await settings.getByRole('button', { name: '移动' }).click()
  const moveDialog = page.getByRole('dialog')
  await moveDialog.getByLabel('目标完整路径').fill(join(workspaceRoot, 'archive', 'gamma'))
  await moveDialog.getByLabel(/输入当前项目名/).fill('gamma')
  await moveDialog.getByRole('button', { name: '移动', exact: true }).click()
  await expect(settings).toContainText(join(workspaceRoot, 'archive', 'gamma'))
  expect(existsSync(join(workspaceRoot, 'archive', 'gamma'))).toBe(true)
  expect(existsSync(join(workspaceRoot, 'gamma'))).toBe(false)

  // Unregister removes only the platform record; the directory survives.
  await openSettings('alpha')
  await settings.getByRole('button', { name: '解除登记' }).click()
  const unregisterDialog = page.getByRole('dialog')
  await expect(unregisterDialog).toContainText('会被完整保留')
  await unregisterDialog.getByRole('button', { name: '解除登记', exact: true }).click()
  await expect(alphaItem).toBeHidden()
  expect(existsSync(join(workspaceRoot, 'alpha'))).toBe(true)

  // The project moved to archive is unregistered the same way and survives too.
  await openSettings('gamma')
  await settings.getByRole('button', { name: '解除登记' }).click()
  await page.getByRole('dialog').getByRole('button', { name: '解除登记', exact: true }).click()
  await expect(row('gamma')).toBeHidden()
  expect(existsSync(join(workspaceRoot, 'archive', 'gamma'))).toBe(true)

  // Logging out ends the session and returns to the login view.
  await page.getByTitle('退出登录').click()
  await expect(page).toHaveURL(/\/login$/)
})
