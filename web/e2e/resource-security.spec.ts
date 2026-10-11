// End-to-end credential safety: a stored MCP value is write-only. It is never
// returned by the secret metadata route or by the resource routes, and opaque
// references are validated.
import { expect, test, type APIRequestContext } from '@playwright/test'

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

test('never returns a stored credential value', async ({ request }) => {
  const csrf = await operatorLogin(request)
  const value = 'e2e-top-secret-value'

  const created = await request.post('/api/secrets', {
    data: { value },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(created.status(), await created.text()).toBe(201)
  const secret = (await created.json()) as { id: string; present: boolean }
  expect(secret.present).toBe(true)
  expect(JSON.stringify(secret)).not.toContain(value)

  const metadata = await request.get(`/api/secrets/${secret.id}`)
  expect(metadata.status()).toBe(200)
  expect(await metadata.text()).not.toContain(value)

  // A malformed reference is rejected.
  const malformed = await request.get('/api/secrets/not%20a%20valid%20id')
  expect(malformed.status()).toBe(400)

  // A resource that references a credential never echoes the value.
  const resource = await request.post('/api/resources', {
    data: {
      kind: 'mcp',
      name: 'security-server',
      content: '{"command":"npx"}',
      enabled: true,
      secretValue: value,
    },
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(resource.status(), await resource.text()).toBe(201)
  const resourceBody = await resource.text()
  expect(resourceBody).not.toContain(value)
  const parsed = JSON.parse(resourceBody) as { id: number; secretRef?: string }
  expect(parsed.secretRef).toBeTruthy()

  const fetched = await request.get(`/api/resources/${parsed.id}`)
  expect(fetched.status()).toBe(200)
  expect(await fetched.text()).not.toContain(value)

  // Deleting the reference removes the value.
  const deleted = await request.delete(`/api/secrets/${secret.id}`, {
    headers: { 'X-CSRF-Token': csrf },
  })
  expect(deleted.status(), await deleted.text()).toBe(204)
  expect((await request.get(`/api/secrets/${secret.id}`)).status()).toBe(404)
})
