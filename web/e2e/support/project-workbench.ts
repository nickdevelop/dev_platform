// Shared workbench navigation for the E2E specs.
//
// The old "/projects/:id/settings" page and its selected-project row are gone:
// the router redirects that URL to the projects management view, and the tree
// renders one "+" per project instead of relying on a globally selected row.
// Specs therefore target a project by name, which stays stable as earlier
// specs register their own projects in the same isolated workspace.
import { expect, type Page } from '@playwright/test'

export function projectRow(page: Page, projectName: string) {
  return page.locator('.project-row', { hasText: projectName })
}

export async function createSession(
  page: Page,
  projectName: string,
  backendName: string,
  title: string,
) {
  await page.goto('/')
  const row = projectRow(page, projectName)
  await expect(row).toBeVisible()
  await row.locator('.project-new-session').click()
  await expect(page.getByRole('heading', { name: '新建会话' })).toBeVisible()
  await page.locator(`[data-test="agent-switch"] [data-backend="${backendName}"]`).click()
  await page.locator('#first-message').fill(title)
  await page.locator('form.create-session button.create').click()
  await expect(page.locator('.session', { hasText: title })).toBeVisible()
}
