// End-to-end security boundary: driven against the real stack (Go backend +
// Vite dev server with same-origin /api proxy), this spec pins the contract
// that LAN exposure relies on:
//
//   - every management API rejects unauthenticated requests,
//   - mutating requests enforce the double-submit CSRF scheme,
//   - path traversal attempts through API inputs are rejected and never
//     touch the filesystem outside the authorized workspace root.
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
  for (const path of ['/api/projects', '/api/workspaces/roots']) {
    const resp = await request.get(path)
    expect(resp.status(), `GET ${path}`).toBe(401)
    expect((await resp.json()).error).toBe('unauthorized')
  }

  // A mutation without the CSRF header is rejected even with a session.
  const csrf = await login(request)
  const noCSRF = await request.post('/api/workspaces/roots', { data: { path: workspaceRoot } })
  expect(noCSRF.status()).toBe(403)
  expect((await noCSRF.json()).error).toBe('invalid_csrf_token')

  // The workspace root registration works once CSRF is present, so the
  // rejections above are about the security scheme, not the payload. A
  // 409 means the sibling spec in the same run already authorized it.
  const withCSRF = await request.post('/api/workspaces/roots', {
    data: { path: workspaceRoot },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect([201, 409], await withCSRF.text()).toContain(withCSRF.status())
})

test('path traversal through API inputs is rejected', async ({ request }) => {
  const csrf = await login(request)
  const headers = { 'X-CSRF-Token': csrf }

  // Authorize only the temporary workspace root (409: the sibling spec in
  // the same run already authorized it).
  const addRoot = await request.post('/api/workspaces/roots', {
    data: { path: workspaceRoot },
    headers,
  })
  expect([201, 409], await addRoot.text()).toContain(addRoot.status())

  // A relative workspace-root input cannot smuggle a traversal.
  const badRoot = await request.post('/api/workspaces/roots', {
    data: { path: '../workspace-escape' },
    headers,
  })
  expect(badRoot.status(), await badRoot.text()).toBe(400)
  expect((await badRoot.json()).error).toBe('invalid_root')

  // Registering a project path that escapes the root via .. is rejected and
  // creates nothing outside the root.
  const escapeRegister = join(workspaceRoot, '..', 'traversal-register')
  const register = await request.post('/api/projects', {
    data: { path: escapeRegister },
    headers,
  })
  expect(register.status(), await register.text()).toBe(400)
  expect((await register.json()).error).toBe('path_not_authorized')
  expect(existsSync(escapeRegister), 'register traversal must not touch the filesystem').toBe(false)

  // A project name containing a path separator cannot traverse on create.
  const escapeCreate = join(workspaceRoot, '..', 'traversal-create')
  const create = await request.post('/api/projects/create', {
    data: { parent: workspaceRoot, name: '../traversal-create' },
    headers,
  })
  expect(create.status(), await create.text()).toBe(400)
  expect((await create.json()).error).toBe('invalid_name')
  expect(existsSync(escapeCreate), 'create traversal must not touch the filesystem').toBe(false)

  // Moving a registered project outside the authorized root is rejected and
  // leaves the directory at its original location.
  mkdirSync(join(workspaceRoot, 'movable'))
  const registered = await request.post('/api/projects', {
    data: { path: join(workspaceRoot, 'movable') },
    headers,
  })
  expect(registered.status(), await registered.text()).toBe(201)
  const project = (await registered.json()) as { id: number }

  const escapeMove = join(workspaceRoot, '..', 'traversal-move')
  const move = await request.post(`/api/projects/${project.id}/move`, {
    data: { path: escapeMove, confirmName: 'movable' },
    headers,
  })
  expect(move.status(), await move.text()).toBe(400)
  expect((await move.json()).error).toBe('path_not_authorized')
  expect(existsSync(escapeMove), 'move traversal must not touch the filesystem').toBe(false)
  expect(existsSync(join(workspaceRoot, 'movable')), 'the project stays in place').toBe(true)
})
