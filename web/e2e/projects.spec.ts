// End-to-end lifecycle: LAN-style login through the real UI against the real
// Go backend, then a full project lifecycle (create, register, search, edit the
// display name, unregister, logout) inside a temporary scratch directory.
// Filesystem effects are asserted directly from the Node side. The platform
// never deletes or renames project directories, so every removal here must
// leave them on disk.
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
  // Authorized workspace content: one existing directory to register.
  mkdirSync(join(workspaceRoot, 'alpha'))

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

  const row = (name: string) => page.locator('.project-row', { hasText: name })

  // Register an existing directory (no new directory is created).
  await page.getByRole('button', { name: '新建项目', exact: true }).click()
  const registerDialog = page.getByRole('dialog')
  await registerDialog.getByLabel('项目名称').fill('alpha')
  await registerDialog.getByRole('checkbox').uncheck()
  await registerDialog.getByLabel('项目路径').fill(join(workspaceRoot, 'alpha'))
  await registerDialog.getByRole('button', { name: '确定', exact: true }).click()
  const alphaItem = row('alpha')
  await expect(alphaItem).toBeVisible()

  // Create a brand-new project directory.
  await page.getByRole('button', { name: '新建项目', exact: true }).click()
  const createDialog = page.getByRole('dialog')
  await createDialog.getByLabel('项目名称').fill('beta')
  await createDialog.getByLabel('项目路径').fill(workspaceRoot)
  await createDialog.getByRole('button', { name: '确定', exact: true }).click()
  const betaItem = row('beta')
  await expect(betaItem).toBeVisible()
  expect(existsSync(join(workspaceRoot, 'beta'))).toBe(true)

  // Search matches project names: other projects are hidden, the open one stays.
  await page.getByLabel('搜索项目或会话').fill('alp')
  await expect(betaItem).toBeHidden()
  await expect(alphaItem).toBeVisible()
  await page.getByLabel('搜索项目或会话').fill('')

  // The project row pencil edits only the platform display name; the directory
  // on disk keeps its original name.
  await betaItem.hover()
  await betaItem.locator('[data-test="project-edit"]').click()
  const editDialog = page.getByRole('dialog')
  await expect(editDialog).toContainText('编辑项目')
  await expect(editDialog).toContainText(join(workspaceRoot, 'beta'))
  await editDialog.getByLabel('项目显示名称').fill('gamma')
  await editDialog.getByRole('button', { name: '保存', exact: true }).click()
  const gammaItem = row('gamma')
  await expect(gammaItem).toBeVisible()
  await expect(betaItem).toBeHidden()
  expect(existsSync(join(workspaceRoot, 'beta'))).toBe(true)
  expect(existsSync(join(workspaceRoot, 'gamma'))).toBe(false)

  // The project settings page is gone: unregistering now happens on the
  // management list, which keeps the same "directory is preserved" contract.
  await page.goto('/projects/management')

  const alphaManageRow = page.locator('[data-test="project-manage-row"]', { hasText: 'alpha' })
  await alphaManageRow.getByRole('button', { name: '解除', exact: true }).click()
  const unregisterDialog = page.getByRole('dialog')
  await expect(unregisterDialog).toContainText('会完全保留')
  await unregisterDialog.getByRole('button', { name: '解除登记', exact: true }).click()
  await expect(alphaManageRow).toHaveCount(0)
  await expect(row('alpha')).toHaveCount(0)
  expect(existsSync(join(workspaceRoot, 'alpha'))).toBe(true)

  // The edited project is unregistered the same way and its directory survives.
  const gammaManageRow = page.locator('[data-test="project-manage-row"]', { hasText: 'gamma' })
  await gammaManageRow.getByRole('button', { name: '解除', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '解除登记', exact: true }).click()
  await expect(gammaManageRow).toHaveCount(0)
  await expect(row('gamma')).toHaveCount(0)
  expect(existsSync(join(workspaceRoot, 'beta'))).toBe(true)

  // Logging out ends the session and returns to the login view.
  await page.getByTitle('退出登录').click()
  await expect(page).toHaveURL(/\/login$/)
})
