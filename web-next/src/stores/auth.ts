import { create } from 'zustand'
import { get, mutate } from '../api/client'

// The server's SessionResponse contract. Kept local to the store, mirroring
// docs/api/openapi.yaml; API modules never leak transport types into components.
export interface SessionResponse {
  authenticated: boolean
  expiresAt?: string
  csrfToken: string
}

interface AuthState {
  authenticated: boolean
  expiresAt: string | null
  csrfToken: string
  // The session endpoint is consulted once per page load, not on every
  // navigation; afterwards this store is the source of truth.
  sessionChecked: boolean
  ensureSession: () => Promise<void>
  fetchSession: () => Promise<string>
  login: (password: string) => Promise<void>
  logout: () => Promise<void>
  reset: () => void
  apply: (session: SessionResponse) => void
}

export const useAuthStore = create<AuthState>((set, getState) => ({
  authenticated: false,
  expiresAt: null,
  csrfToken: '',
  sessionChecked: false,

  ensureSession: async () => {
    if (getState().sessionChecked) return
    await getState().fetchSession()
  },

  fetchSession: async () => {
    const session = await get<SessionResponse>('/auth/session')
    getState().apply(session)
    set({ sessionChecked: true })
    return session.csrfToken
  },

  login: async (password) => {
    // Login is CSRF protected; mutate() bootstraps a token through one session
    // request when the store has none, so no pre-fetch is needed here. A 401
    // means a wrong password, not an expired session.
    const session = await mutate<SessionResponse>(
      'POST',
      '/auth/login',
      { password },
      { ignoreUnauthorized: true },
    )
    getState().apply(session)
    set({ sessionChecked: true })
  },

  logout: async () => {
    try {
      await mutate<SessionResponse>('POST', '/auth/logout')
    } finally {
      // The session is gone from the user's perspective even if the request
      // itself failed (for example because it had already expired).
      getState().reset()
    }
  },

  reset: () => {
    set({ authenticated: false, expiresAt: null, csrfToken: '' })
  },

  apply: (session) => {
    set({
      authenticated: session.authenticated,
      expiresAt: session.expiresAt ?? null,
      csrfToken: session.csrfToken,
    })
  },
}))
