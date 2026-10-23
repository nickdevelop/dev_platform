// Seeds the throwaway E2E database with the current model catalogs. The
// product deliberately serves persisted snapshots and refreshes them only on an
// operator action, so the suite performs that action once before the tests,
// exactly as a user would from Platform Settings → Agent.
import { request, type APIRequestContext } from '@playwright/test'

const VITE_PORT = process.env.E2E_VITE_PORT ?? '15173'
const baseURL = `http://127.0.0.1:${VITE_PORT}`

async function sessionToken(context: APIRequestContext): Promise<string> {
  const response = await context.get('/api/auth/session')
  if (!response.ok()) throw new Error(`e2e model seed: GET session = ${response.status()}`)
  const body = (await response.json()) as { csrfToken?: string }
  if (!body.csrfToken) throw new Error('e2e model seed: session returned no CSRF token')
  return body.csrfToken
}

export default async function globalSetup(): Promise<void> {
  const password = process.env.E2E_PASSWORD
  if (!password) throw new Error('e2e model seed: E2E_PASSWORD is missing')

  const context = await request.newContext({ baseURL })
  try {
    const loginToken = await sessionToken(context)
    const login = await context.post('/api/auth/login', {
      data: { password },
      headers: { 'X-CSRF-Token': loginToken },
    })
    if (!login.ok()) throw new Error(`e2e model seed: login = ${login.status()}`)

    const token = await sessionToken(context)
    const backendsResponse = await context.get('/api/agent-backends')
    if (!backendsResponse.ok()) {
      throw new Error(`e2e model seed: list backends = ${backendsResponse.status()}`)
    }
    const backends = (await backendsResponse.json()) as {
      backends?: Array<{ backend: string; capabilities?: { modelSelection?: boolean } }>
    }
    for (const entry of backends.backends ?? []) {
      if (!entry.capabilities?.modelSelection) continue
      const refresh = await context.post(
        `/api/agent-backends/${encodeURIComponent(entry.backend)}/models/refresh`,
        { headers: { 'X-CSRF-Token': token } },
      )
      if (!refresh.ok()) {
        // A backend whose fixture is not part of this run (for example a
        // provider that needs real credentials) stays unsynced; tests that use
        // it would fail on their own assertion, not here.
        console.warn(
          `e2e model seed: refresh ${entry.backend} = ${refresh.status()} ${await refresh.text()}`,
        )
      }
    }
  } finally {
    await context.dispose()
  }
}
