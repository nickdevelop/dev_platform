// End-to-end security boundary: driven against the real stack (Go backend +
// Vite dev server with same-origin /api proxy), this spec pins the contract
// that LAN exposure relies on:
//
//   - every management API rejects unauthenticated requests,
//   - mutating requests enforce the double-submit CSRF scheme,
//   - malformed or relative path inputs are rejected and never touch the
//     filesystem. (Arbitrary absolute directories are now allowed by design;
//     the platform no longer confines projects to authorized roots.)
//
// Started via `npm run test:e2e`, which provides E2E_PASSWORD and
// E2E_WORKSPACE_ROOT pointing at an isolated scratch directory.
import { existsSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'

const password = process.env.E2E_PASSWORD
const workspaceRoot = process.env.E2E_WORKSPACE_ROOT

if (!password || !workspaceRoot) {
  throw new Error('run this spec through web/scripts/run-e2e.mjs, not plain playwright test')
}

// login performs the session + CSRF handshake with the request fixture.
async function login(request: import('@playwright/test').APIRequestContext) {
  const preSession = await (await request.get('/api/auth/session')).json()
  const login = await request.post('/api/auth/login', {
    data: { password },
    headers: { 'X-CSRF-Token': preSession.csrfToken },
  })
  expect(login.status(), await login.text()).toBe(200)
  const session = await (await request.get('/api/auth/session')).json()
  expect(session.authenticated).toBe(true)
  return session.csrfToken as string
}

test('management APIs reject unauthenticated access and enforce CSRF', async ({ request }) => {
  // The public health route stays open.
  const health = await request.get('/api/health')
  expect(health.status()).toBe(200)

  // Every management API requires a session.
  const resp = await request.get('/api/projects')
  expect(resp.status(), 'GET /api/projects').toBe(401)
  expect((await resp.json()).error).toBe('unauthorized')

  // A mutation without the CSRF header is rejected even with a session.
  const csrf = await login(request)
  const appearance = { theme: 'light', fontSize: 14, density: 'standard' }
  const noCSRF = await request.put('/api/settings/appearance', { data: appearance })
  expect(noCSRF.status()).toBe(403)
  expect((await noCSRF.json()).error).toBe('invalid_csrf_token')

  // With CSRF present the same request succeeds, so the rejection above is
  // about the security scheme, not the payload.
  const withCSRF = await request.put('/api/settings/appearance', {
    data: appearance,
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(withCSRF.status(), await withCSRF.text()).toBe(200)
})

test('malformed path inputs are rejected without touching the filesystem', async ({ request }) => {
  const csrf = await login(request)
  const headers = { 'X-CSRF-Token': csrf }

  // A relative project path is rejected.
  const relative = await request.post('/api/projects', {
    data: { path: '../escape' },
    headers,
  })
  expect(relative.status(), await relative.text()).toBe(400)
  expect((await relative.json()).error).toBe('invalid_target')

  // A project name containing a path separator cannot traverse on create.
  const escapeCreate = join(workspaceRoot, '..', 'traversal-create')
  const create = await request.post('/api/projects/create', {
    data: { parent: workspaceRoot, name: '../traversal-create' },
    headers,
  })
  expect(create.status(), await create.text()).toBe(400)
  expect((await create.json()).error).toBe('invalid_name')
  expect(existsSync(escapeCreate), 'create traversal must not touch the filesystem').toBe(false)

  // A relative move destination is rejected and leaves the directory at its
  // original location.
  mkdirSync(join(workspaceRoot, 'movable'), { recursive: true })
  const registered = await request.post('/api/projects', {
    data: { path: join(workspaceRoot, 'movable') },
    headers,
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }

  const move = await request.post(`/api/projects/${project.id}/move`, {
    data: { path: 'relative-destination', confirmName: 'movable' },
    headers,
  })
  expect(move.status(), await move.text()).toBe(400)
  expect(existsSync(join(workspaceRoot, 'movable')), 'the project stays in place').toBe(true)
})
