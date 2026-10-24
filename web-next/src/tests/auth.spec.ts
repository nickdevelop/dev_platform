import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'

interface Call {
  url: string
  method: string
  body: unknown
}

function stubSession(): Call[] {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      calls.push({ url, method, body })
      if (url.endsWith('/api/auth/login')) {
        return {
          status: 200,
          json: async () => ({ authenticated: true, csrfToken: 'csrf-login' }),
        } as Response
      }
      if (url.endsWith('/api/auth/logout')) {
        return {
          status: 200,
          json: async () => ({ authenticated: false, csrfToken: 'csrf-logout' }),
        } as Response
      }
      return {
        status: 200,
        json: async () => ({ authenticated: true, csrfToken: 'csrf-session' }),
      } as Response
    }),
  )
  return calls
}

describe('auth store', () => {
  beforeEach(() => {
    installTestCSRF()
    useAuthStore.setState({
      authenticated: false,
      expiresAt: null,
      csrfToken: '',
      sessionChecked: false,
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('checks the session once and keeps the CSRF token', async () => {
    const calls = stubSession()
    await useAuthStore.getState().ensureSession()
    await useAuthStore.getState().ensureSession()

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/auth/session')
    expect(useAuthStore.getState().authenticated).toBe(true)
    expect(useAuthStore.getState().csrfToken).toBe('csrf-session')
  })

  it('logs in through mutate and sends the password', async () => {
    const calls = stubSession()
    await useAuthStore.getState().login('secret')

    expect(calls[0].url).toBe('/api/auth/login')
    expect(calls[0].method).toBe('POST')
    expect(calls[0].body).toEqual({ password: 'secret' })
    expect(useAuthStore.getState().authenticated).toBe(true)
    expect(useAuthStore.getState().csrfToken).toBe('csrf-login')
  })

  it('clears state even when logout fails', async () => {
    stubSession()
    await useAuthStore.getState().login('secret')
    useAuthStore.setState({ authenticated: true })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network down')
      }),
    )

    await expect(useAuthStore.getState().logout()).rejects.toThrow('network down')
    expect(useAuthStore.getState().authenticated).toBe(false)
    expect(useAuthStore.getState().csrfToken).toBe('')
  })
})
