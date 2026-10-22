// Pinia store for the shared-password session: whether the caller is
// authenticated, when the session expires, and the CSRF token the transport
// echoes on mutating requests.
//
// The store is the token's single source of truth. It never hands the token to
// a call site: api/client.ts reads it through the provider installed by the
// composition root, so no request can be sent without it.

import { defineStore } from 'pinia'
import { get, mutate } from '../api/client'

// Mirrors docs/api/openapi.yaml #/components/schemas/SessionResponse.
export interface SessionResponse {
  authenticated: boolean
  expiresAt?: string
  csrfToken: string
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    authenticated: false,
    expiresAt: null as string | null,
    csrfToken: '',
    // The session endpoint is consulted once per page load, not on every
    // navigation; afterwards the store's state is the source of truth.
    sessionChecked: false,
  }),

  getters: {
    isAuthenticated: (state) => state.authenticated,
  },

  actions: {
    // Resolves the current session, fetching it at most once per page load.
    async ensureSession() {
      if (this.sessionChecked) return
      await this.fetchSession()
    },

    // fetchSession resolves the current session and returns the CSRF token so
    // the transport can use it as its refresh callback.
    async fetchSession(): Promise<string> {
      const session = await get<SessionResponse>('/auth/session')
      this.apply(session)
      this.sessionChecked = true
      return session.csrfToken
    },

    async login(password: string) {
      // Login is CSRF protected, and mutate() bootstraps a token through one
      // session request when the store has none, so no pre-fetch is needed
      // here. A 401 means a wrong password, not an expired session, hence
      // ignoreUnauthorized.
      const session = await mutate<SessionResponse>(
        'POST',
        '/auth/login',
        { password },
        { ignoreUnauthorized: true },
      )
      this.apply(session)
      this.sessionChecked = true
    },

    async logout() {
      try {
        await mutate<SessionResponse>('POST', '/auth/logout')
      } finally {
        // The session is gone from the user's perspective even if the
        // request itself failed (e.g. it had already expired).
        this.reset()
      }
    },

    reset() {
      this.authenticated = false
      this.expiresAt = null
      this.csrfToken = ''
    },

    apply(session: SessionResponse) {
      this.authenticated = session.authenticated
      this.expiresAt = session.expiresAt ?? null
      this.csrfToken = session.csrfToken
    },
  },
})
