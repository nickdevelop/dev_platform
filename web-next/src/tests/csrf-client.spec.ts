// Transport-level tests for the CSRF-aware API client. The contract these pin
// is the one the whole frontend relies on: mutating requests always carry
// X-CSRF-Token and recover once from an expired token, read requests never do,
// and a call site cannot opt out by forgetting to pass anything.
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  UnauthorizedError,
  describeApiError,
  get,
  installCSRF,
  mutate,
  setOnUnauthorized,
} from '../api/client'

interface RecordedCall {
  path: string
  method: string
  headers: Record<string, string>
  body: unknown
}

interface StubResponse {
  status: number
  body?: unknown
}

// stubFetch answers calls in order and records what was sent. The queue falling
// back to 200 lets a test describe only the responses it cares about.
function stubFetch(responses: StubResponse[] = []): RecordedCall[] {
  const queue = [...responses]
  const calls: RecordedCall[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({
        path: String(input),
        method: (init?.method ?? 'GET').toUpperCase(),
        headers: { ...(init?.headers as Record<string, string>) },
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      })
      const next = queue.shift() ?? { status: 200, body: {} }
      return { status: next.status, json: async () => next.body ?? {} } as Response
    }),
  )
  return calls
}

// installToken registers a stateful provider: refresh rotates the token so a
// test can tell the pre- and post-refresh requests apart.
function installToken(initial: string, next = 'refreshed-token') {
  let current = initial
  const get = vi.fn(() => current)
  const refresh = vi.fn(async () => {
    current = next
    return current
  })
  installCSRF({ get, refresh })
  return { get, refresh }
}

afterEach(() => {
  vi.unstubAllGlobals()
  setOnUnauthorized(null)
})

describe('get', () => {
  it('never sends a CSRF token', async () => {
    installToken('token-1')
    const calls = stubFetch([{ status: 200, body: { ok: true } }])

    await get('/model-services')

    expect(calls).toHaveLength(1)
    expect(calls[0].method).toBe('GET')
    expect(calls[0].headers['X-CSRF-Token']).toBeUndefined()
  })
})

describe('mutate', () => {
  it('attaches the token from the session store', async () => {
    installToken('token-1')
    const calls = stubFetch([{ status: 200, body: { id: 1 } }])

    await mutate('POST', '/model-services/fetch-models', { baseUrl: 'https://x/v1' })

    expect(calls).toHaveLength(1)
    expect(calls[0].headers['X-CSRF-Token']).toBe('token-1')
    expect(calls[0].headers['Content-Type']).toBe('application/json')
    expect(calls[0].body).toEqual({ baseUrl: 'https://x/v1' })
  })

  it('works for every mutating verb and sends no body when none is given', async () => {
    installToken('token-1')
    const calls = stubFetch()

    await mutate('DELETE', '/models/1')
    await mutate('PUT', '/models/1', { enabled: false })

    expect(calls.map((call) => call.method)).toEqual(['DELETE', 'PUT'])
    expect(calls[0].headers['X-CSRF-Token']).toBe('token-1')
    expect(calls[0].headers['Content-Type']).toBeUndefined()
    expect(calls[1].headers['X-CSRF-Token']).toBe('token-1')
  })

  it('bootstraps the token through one refresh when the store has none', async () => {
    const { get, refresh } = installToken('', 'bootstrapped')
    const calls = stubFetch([{ status: 200, body: {} }])

    await mutate('POST', '/projects', { path: '/tmp/p' })

    expect(get).toHaveBeenCalled()
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(calls[0].headers['X-CSRF-Token']).toBe('bootstrapped')
  })

  it('shares a single refresh across concurrent calls', async () => {
    let resolveRefresh: (token: string) => void = () => {}
    const refresh = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          resolveRefresh = resolve
        }),
    )
    installCSRF({ get: () => '', refresh })
    const calls = stubFetch([{ status: 200, body: {} }, { status: 200, body: {} }])

    const first = mutate('POST', '/projects', { path: '/tmp/a' })
    const second = mutate('POST', '/projects', { path: '/tmp/b' })
    await vi.waitFor(() => expect(refresh).toHaveBeenCalled())
    resolveRefresh('shared-token')
    await Promise.all([first, second])

    expect(refresh).toHaveBeenCalledTimes(1)
    expect(calls.map((call) => call.headers['X-CSRF-Token'])).toEqual([
      'shared-token',
      'shared-token',
    ])
  })

  it('recovers once from an expired token and retries with the fresh one', async () => {
    const { refresh } = installToken('stale-token', 'fresh-token')
    const calls = stubFetch([
      { status: 403, body: { error: 'invalid_csrf_token' } },
      { status: 200, body: { models: [] } },
    ])

    const result = await mutate('POST', '/model-services/fetch-models', { baseUrl: 'https://x/v1' })

    expect(result).toEqual({ models: [] })
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(calls).toHaveLength(2)
    expect(calls[0].headers['X-CSRF-Token']).toBe('stale-token')
    expect(calls[1].headers['X-CSRF-Token']).toBe('fresh-token')
    // The retry must replay the same request body.
    expect(calls[1].body).toEqual(calls[0].body)
  })

  it('surfaces the CSRF rejection after a single retry', async () => {
    const { refresh } = installToken('stale-token', 'fresh-token')
    const calls = stubFetch([
      { status: 403, body: { error: 'invalid_csrf_token' } },
      { status: 403, body: { error: 'invalid_csrf_token' } },
    ])

    await expect(mutate('POST', '/projects', { path: '/tmp/p' })).rejects.toMatchObject({
      status: 403,
      code: 'invalid_csrf_token',
    })
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(calls).toHaveLength(2)
  })

  it('never retries a 403 that is not a CSRF rejection', async () => {
    const { refresh } = installToken('token-1')
    const calls = stubFetch([{ status: 403, body: { error: 'permission_denied' } }])

    await expect(mutate('POST', '/projects', { path: '/tmp/p' })).rejects.toMatchObject({
      status: 403,
      code: 'permission_denied',
    })
    expect(refresh).not.toHaveBeenCalled()
    expect(calls).toHaveLength(1)
  })

  it('reports an expired session as UnauthorizedError', async () => {
    installToken('token-1')
    stubFetch([{ status: 401, body: { error: 'unauthorized' } }])
    const onUnauthorized = vi.fn()
    setOnUnauthorized(onUnauthorized)

    await expect(mutate('POST', '/projects', { path: '/tmp/p' })).rejects.toBeInstanceOf(
      UnauthorizedError,
    )
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
  })

  it('maps a bad-credential 401 to its server code when the caller opts in', async () => {
    installToken('token-1')
    stubFetch([{ status: 401, body: { error: 'invalid_credentials' } }])
    const onUnauthorized = vi.fn()
    setOnUnauthorized(onUnauthorized)

    await expect(
      mutate('POST', '/auth/login', { password: 'wrong' }, { ignoreUnauthorized: true }),
    ).rejects.toMatchObject({ status: 401, code: 'invalid_credentials' })
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('resolves undefined for a 204 response', async () => {
    installToken('token-1')
    stubFetch([{ status: 204 }])

    await expect(mutate('DELETE', '/models/1')).resolves.toBeUndefined()
  })
})

describe('describeApiError', () => {
  it('maps the CSRF rejection to an actionable message', () => {
    expect(describeApiError(new ApiError(403, 'invalid_csrf_token'))).toBe(
      '请求校验失败，请刷新页面后重试。',
    )
  })
})

// Kept last: vi.resetModules() gives a fresh module instance, so the binding
// imported at the top of this file no longer refers to the client under test.
describe('installCSRF', () => {
  it('fails fast when a mutating request runs without a provider', async () => {
    vi.resetModules()
    const isolated = await import('../api/client')

    await expect(isolated.mutate('POST', '/projects', {})).rejects.toThrow(/installCSRF/)
  })
})
