import { existsSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type APIRequestContext } from '@playwright/test'

const password = process.env.E2E_PASSWORD
const workspaceRoot = process.env.E2E_WORKSPACE_ROOT

if (!password || !workspaceRoot) {
  throw new Error('run this spec through web-next/scripts/run-e2e.mjs')
}

async function login(request: APIRequestContext): Promise<string> {
  const pre = await (await request.get('/api/auth/session')).json()
  const response = await request.post('/api/auth/login', {
    data: { password },
    headers: { 'X-CSRF-Token': pre.csrfToken },
  })
  expect(response.status(), await response.text()).toBe(200)
  const session = await (await request.get('/api/auth/session')).json()
  return session.csrfToken as string
}

async function registerProject(request: APIRequestContext, name: string): Promise<{ id: number; path: string; name: string }> {
  const csrf = await login(request)
  const dir = join(workspaceRoot!, `${name}-${Date.now()}-${process.pid}`)
  mkdirSync(dir, { recursive: true })
  const response = await request.post('/api/projects', {
    data: { path: dir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(response.status(), await response.text()).toBe(201)
  return { ...(await response.json()), path: dir }
}

async function uiLogin(page: import('@playwright/test').Page): Promise<void> {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password!)
  await page.getByRole('button', { name: /登\s*录/ }).click()
  await expect(page).toHaveURL(/\/$/)
}

test('logs in, registers a project, opens settings and switches theme', async ({ page, request }) => {
  const project = await registerProject(request, 'react-e2e')
  expect(existsSync(project.path)).toBe(true)

  await uiLogin(page)
  await expect(page.getByText(project.name, { exact: true })).toBeVisible()

  await page.getByRole('button', { name: `项目设置：${project.name}` }).click()
  await expect(page).toHaveURL(new RegExp(`/projects/${project.id}/settings$`))
  await expect(page.getByRole('tab', { name: '项目设置' })).toBeVisible()

  await page.goto('/settings/appearance')
  await page.getByRole('heading', { name: '外观' }).waitFor()
  await page.getByRole('button', { name: /深色/ }).click()
  await expect
    .poll(() => page.evaluate(() => document.documentElement.dataset.theme))
    .toBe('dark')
})

test('opens an agent session, sends a message and receives the fake pi reply', async ({ page, request }) => {
  const project = await registerProject(request, 'react-e2e-chat')
  const csrf = await login(request)
  const created = await request.post(`/api/projects/${project.id}/sessions`, {
    data: { backend: 'pi', title: 'React E2E session' },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(created.status(), await created.text()).toBe(201)
  const session = (await created.json()) as { session: { id: string } }

  await uiLogin(page)
  await page.goto(`/projects/${project.id}/sessions/${session.session.id}`)
  await expect(page.getByRole('heading', { name: 'React E2E session' })).toBeVisible()

  const input = page.locator('[data-test="message-input"]')
  await input.fill('hello from e2e')
  await page.getByRole('button', { name: /发\s*送/ }).click()
  await expect(page.getByText(/pi reply: hello from e2e/)).toBeVisible({ timeout: 30_000 })
})


test('loads every migrated surface through the production routes', async ({ page, request }) => {
  const project = await registerProject(request, 'react-e2e-surfaces')
  await uiLogin(page)

  await page.goto('/chat')
  await expect(page.getByRole('button', { name: /新建/ })).toBeVisible()

  await page.goto('/prompts')
  await expect(page.getByText('提示词', { exact: true }).first()).toBeVisible()

  await page.goto('/tasks')
  await expect(page.getByText('任务', { exact: true }).first()).toBeVisible()

  await page.goto(`/projects/${project.id}/sessions/new`)
  await expect(page.getByText('新建会话').first()).toBeVisible()

  await page.goto(`/projects/${project.id}/files`)
  await expect(page.locator('[data-test="files-page"]')).toBeVisible()

  await page.goto('/settings/appearance')
  await expect(page.getByRole('heading', { name: '外观' })).toBeVisible()
  await page.goto('/settings/models')
  await expect(page.getByRole('heading', { name: '模型' })).toBeVisible()
  await page.goto('/settings/agents')
  await expect(page.getByRole('heading', { name: 'Agent' })).toBeVisible()

  await page.goto('/terminal')
  await expect(page.getByRole('button', { name: /本机终端/ })).toBeVisible()
})
