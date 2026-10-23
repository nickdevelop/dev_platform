// Quick assistant end-to-end: the platform toggle reveals a draggable floating
// button, clicking it opens the 480x600 popover, and the reserved "快捷助手"
// chat group is visible with its management controls locked down.
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const password = process.env.E2E_PASSWORD

if (!password) {
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

async function uiLogin(page: Page) {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password!)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)
}

async function setQuickAssistant(request: APIRequestContext, csrf: string, enabled: boolean) {
  const response = await request.put('/api/settings/quick-assistant', {
    data: { enabled },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(response.status(), await response.text()).toBe(200)
}

test.describe.serial('quick assistant', () => {
  let csrf = ''

  test.beforeEach(async ({ request }) => {
    csrf = await operatorLogin(request)
    await setQuickAssistant(request, csrf, false)
  })

  test.afterEach(async ({ request }) => {
    await setQuickAssistant(request, csrf, false)
  })

  test('enables the toggle, opens a 480x600 popover and drags the button', async ({ page }) => {
    await uiLogin(page)

    await page.getByRole('link', { name: '设置' }).click()
    await expect(page).toHaveURL(/\/settings\/appearance$/)
    const toggle = page.locator('[data-test="quick-assistant-toggle"]')
    await expect(toggle).not.toBeChecked()

    const saved = page.waitForResponse(
      (response) =>
        response.url().includes('/api/settings/quick-assistant') &&
        response.request().method() === 'PUT',
    )
    await toggle.check()
    await saved
    await expect(toggle).toBeChecked()

    // The setting survives a reload and reveals the floating button.
    await page.goto('/')
    const button = page.locator('[data-test="quick-assistant-button"]')
    await expect(button).toBeVisible()

    // Drag the button; the new position persists in localStorage.
    const before = await button.boundingBox()
    expect(before).not.toBeNull()
    await page.mouse.move(before!.x + before!.width / 2, before!.y + before!.height / 2)
    await page.mouse.down()
    await page.mouse.move(before!.x + before!.width / 2 - 220, before!.y + before!.height / 2 + 90, {
      steps: 8,
    })
    await page.mouse.up()
    const after = await button.boundingBox()
    expect(after).not.toBeNull()
    expect(Math.round(after!.x)).not.toBe(Math.round(before!.x))
    const stored = await page.evaluate(() =>
      JSON.parse(localStorage.getItem('dev-platform-quick-assistant-position') ?? 'null'),
    )
    expect(stored).toMatchObject({ left: expect.any(Number), top: expect.any(Number) })

    // Opening the popover keeps the platform page underneath and shows the
    // reserved-group welcome state.
    await button.click()
    const popover = page.locator('[data-test="quick-assistant-popover"]')
    await expect(popover).toBeVisible()
    const box = await popover.boundingBox()
    expect(box).not.toBeNull()
    expect(Math.round(box!.width)).toBe(480)
    expect(Math.round(box!.height)).toBe(600)
    await expect(page.getByRole('dialog', { name: '快捷助手' })).toBeVisible()
    await expect(page.getByText('你好，我是快捷助手')).toBeVisible()

    await page.locator('[data-test="quick-assistant-close"]').click()
    await expect(popover).toHaveCount(0)

    // Turning the setting off removes the launcher.
    await page.goto('/settings/appearance')
    const toggleAgain = page.locator('[data-test="quick-assistant-toggle"]')
    await expect(toggleAgain).toBeChecked()
    const disabled = page.waitForResponse(
      (response) =>
        response.url().includes('/api/settings/quick-assistant') &&
        response.request().method() === 'PUT',
    )
    await toggleAgain.uncheck()
    await disabled
    await page.goto('/')
    await expect(page.locator('[data-test="quick-assistant-button"]')).toHaveCount(0)
  })

  test('shows the reserved group in Chat with its controls locked', async ({ page }) => {
    await uiLogin(page)
    await page.getByRole('link', { name: 'Chat' }).click()
    await expect(page).toHaveURL(/\/chat$/)

    const sidebar = page.getByRole('complementary', { name: '对话列表' })
    await expect(sidebar.getByText('快捷助手')).toBeVisible()
    await expect(sidebar.getByText('固定')).toBeVisible()
    await expect(sidebar.locator('[aria-label="重命名分组「快捷助手」"]')).toHaveCount(0)
    await expect(sidebar.locator('[aria-label="删除分组「快捷助手」"]')).toHaveCount(0)
    await expect(sidebar.locator('[aria-label="在「快捷助手」中新建对话"]')).toHaveCount(0)
  })
})
