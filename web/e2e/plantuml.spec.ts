// End-to-end proof that a PlantUML fence in an agent reply becomes a drawing.
// This is the only place the real engine runs: the unit tests inject a fake, so
// here we check the thing a person actually gets — a rendered SVG in the chat,
// the source one click away, and a readable failure for a diagram the offline
// engine cannot draw.
//
// The fake `pi` executable echoes the prompt back as its reply, so the diagram
// source reaches the browser through the real adapter, the real SSE stream and
// the real renderer, and the reply's two text deltas also exercise the
// streaming case: the fence is half-written before it closes.
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

function taskRow(page: Page, title: string) {
  return page.locator('.task', { hasText: title })
}

// The most recent agent turn, so an assertion cannot be satisfied by a diagram
// an earlier turn already drew.
function lastTurn(page: Page) {
  return page.locator('.chat-entry.kind-message').last()
}

// ask sends one prompt and waits for the reply to land in the conversation.
async function ask(page: Page, prompt: string) {
  await page.locator('#message-text').fill(prompt)
  await page.getByRole('button', { name: '发送', exact: true }).click()
  await expect(page.locator('[data-test="conversation"]')).toContainText('pi reply:', { timeout: 20_000 })
}

test('draws a plantuml fence in the chat and toggles to its source', async ({ page, request }) => {
  test.slow()

  const csrf = await operatorLogin(request)
  const projectDir = join(workspaceRoot, `plantuml-${Date.now()}-${process.pid}`)
  mkdirSync(projectDir, { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: projectDir },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }

  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('平台密码').fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/$/)

  await page.goto(`/projects/${project.id}/settings`)
  await page.locator('.project-row.selected .project-new-task').click()
  await page.locator('#backend-select').selectOption('pi')
  await page.locator('#session-title').fill('puml-e2e')
  await page.locator('form.create-session button.create').click()
  await expect(taskRow(page, 'puml-e2e')).toBeVisible()
  await taskRow(page, 'puml-e2e').click()
  await expect(page.locator('[data-test="conversation"]')).toHaveAttribute('data-stream-status', 'open')

  // 1. A closed fence renders as a diagram: the engine really ran, so there is
  // an <svg> inside the block's canvas.
  await ask(
    page,
    '画个登录时序\n```plantuml\n@startuml\nactor 用户\n用户 -> 服务: 登录\n服务 --> 用户: 成功\n@enduml\n```',
  )
  const block = page.locator('[data-test="plantuml-block"]').first()
  await expect(block).toBeVisible()
  await expect(block.locator('[data-test="plantuml-canvas"] svg')).toBeVisible()
  // The diagram is not also shown as a code block. (Scoped to the block: the
  // user's own turn and the reasoning line both quote the prompt.)
  await expect(block.locator('[data-test="plantuml-source"]')).toHaveCount(0)

  // 2. The head row toggles this block to its source and back.
  await block.locator('.plantuml-head').click()
  await expect(block.locator('[data-test="plantuml-source"]')).toContainText('@startuml')
  await expect(block.locator('[data-test="plantuml-canvas"]')).toHaveCount(0)
  await block.locator('.plantuml-head').click()
  await expect(block.locator('[data-test="plantuml-canvas"] svg')).toBeVisible()

  // 3. A diagram the offline engine cannot draw says so and shows its source,
  // instead of painting the engine's own error board as if it succeeded.
  await ask(
    page,
    '换成 C4\n```plantuml\n@startuml\n!include <C4/C4_Container>\nA -> B: x\n@enduml\n```',
  )
  const failed = lastTurn(page)
  await expect(failed.locator('[data-test="plantuml-error"]')).toContainText('无法渲染')
  await expect(failed.locator('[data-test="plantuml-source"]')).toContainText('!include <C4/C4_Container>')

  // 4. A reply carrying two diagrams draws both, one after the other — the
  // engine hangs if it is asked for two at once, so this is the queue at work.
  await ask(
    page,
    '两张图\n```plantuml\n@startuml\nA -> B: 一\n@enduml\n```\n中间\n```puml\n@startuml\nC -> D: 二\n@enduml\n```',
  )
  const pair = lastTurn(page)
  await expect(pair.locator('[data-test="plantuml-canvas"] svg')).toHaveCount(2)
  await expect(pair.getByText('中间')).toBeVisible()

  // 5. A dark palette asks for a dark drawing and puts it on a dark canvas.
  await page.goto('/settings/appearance')
  await page.locator('[data-theme-option="dark"]').click()
  await expect(page.locator('[data-theme-option="dark"].active')).toBeVisible()
  await page.goto(`/projects/${project.id}/settings`)
  await taskRow(page, 'puml-e2e').click()
  await expect(page.locator('[data-test="plantuml-block"]').first().locator('.plantuml-canvas.is-dark svg')).toBeVisible({
    timeout: 30_000,
  })

  // 6. A reply that stopped inside the fence cannot ever draw, so it shows the
  // source it was cut off in instead of waiting forever.
  await ask(page, '没写完\n```plantuml\n@startuml\nA -> B\n（回复在这里断了）')
  const cutOff = lastTurn(page)
  await expect(cutOff.locator('[data-test="plantuml-source"]')).toBeVisible()
  await expect(cutOff.locator('[data-test="plantuml-waiting"]')).toContainText('没画完')

  // 7. A drawing is shown whole: the canvas itself never scrolls, because a
  // diagram you have to scroll inside to read is not a drawing. A tall one
  // grows the message (the message log is the one scroller), and the whole
  // picture sits inside the canvas.
  const tall = Array.from({ length: 40 }, (_unused, i) => `步骤${i} -> 服务 : 调用 ${i}`).join('\n')
  await ask(page, `长图\n\`\`\`plantuml\n@startuml\n${tall}\n@enduml\n\`\`\``)
  const tallCanvas = lastTurn(page).locator('[data-test="plantuml-canvas"]')
  await expect(tallCanvas.locator('svg')).toBeVisible()
  await expect
    .poll(async () => tallCanvas.evaluate((el) => el.scrollHeight - el.clientHeight), {
      message: 'the tall drawing must not need a scrollbar',
    })
    .toBeLessThanOrEqual(0)
  const tallBox = await tallCanvas.evaluate((el) => {
    const canvas = el.getBoundingClientRect()
    const drawing = el.querySelector('svg')!.getBoundingClientRect()
    return {
      bottom: canvas.bottom,
      drawingBottom: drawing.bottom,
      drawingHeight: drawing.height,
    }
  })
  // The whole drawing is on screen, and it really is tall: a canvas that
  // collapsed to nothing would satisfy the overflow check above.
  expect(tallBox.drawingBottom).toBeLessThanOrEqual(Math.ceil(tallBox.bottom))
  expect(tallBox.drawingHeight).toBeGreaterThan(400)

  // 8. A drawing wider than the pane is scaled down as a picture, not clipped:
  // the rendered aspect ratio is the one the engine asked for.
  const wide = Array.from({ length: 22 }, (_unused, i) => `节点${i} --> 节点${i + 1} : 说明${i}`).join('\n')
  await ask(page, `宽图\n\`\`\`plantuml\n@startuml\n${wide}\n@enduml\n\`\`\``)
  const wideCanvas = lastTurn(page).locator('[data-test="plantuml-canvas"]')
  await expect(wideCanvas.locator('svg')).toBeVisible()
  const wideBox = await wideCanvas.evaluate((el) => {
    const canvas = el.getBoundingClientRect()
    const svg = el.querySelector('svg')!
    const drawing = svg.getBoundingClientRect()
    const box = svg.getAttribute('viewBox')
    const parts = box?.split(/[\s,]+/).map(Number)
    const askedWidth = parts ? parts[2] : parseFloat(svg.getAttribute('width') ?? '0')
    const askedHeight = parts ? parts[3] : parseFloat(svg.getAttribute('height') ?? '0')
    return {
      askedWidth,
      canvasWidth: canvas.width,
      drawingWidth: drawing.width,
      askedRatio: askedWidth / askedHeight,
      ratio: drawing.width / drawing.height,
      scrollOverflow: el.scrollWidth - el.clientWidth,
      viewBox: box,
    }
  })
  expect(wideBox.viewBox, 'the engine must give a viewBox so the drawing can scale').toBeTruthy()
  // The picture really is wider than the pane, so the checks below are about
  // scaling and not about a drawing that never needed it.
  expect(wideBox.askedWidth).toBeGreaterThan(wideBox.canvasWidth)
  expect(wideBox.scrollOverflow).toBeLessThanOrEqual(0)
  expect(wideBox.drawingWidth).toBeLessThanOrEqual(wideBox.canvasWidth)
  expect(wideBox.ratio).toBeCloseTo(wideBox.askedRatio, 1)
})
