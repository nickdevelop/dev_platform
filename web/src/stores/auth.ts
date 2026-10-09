// Pinia store for the shared-password session: whether the caller is
// authenticated, when the session expires, and the CSRF token that mutating
// API calls must echo in X-CSRF-Token.

import { defineStore } from 'pinia'
import { apiRequest } from '../api/client'

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

    async fetchSession() {
      const session = await apiRequest<SessionResponse>('GET', '/auth/session')
      this.apply(session)
      this.sessionChecked = true
    },

    async login(password: string) {
      if (!this.csrfToken) {
        // Login is CSRF-protected, so a token must be obtained first.
        await this.fetchSession()
      }
      const session = await apiRequest<SessionResponse>('POST', '/auth/login', {
        body: { password },
        csrfToken: this.csrfToken,
        ignoreUnauthorized: true,
      })
      this.apply(session)
    },

    async logout() {
      try {
        await apiRequest<SessionResponse>('POST', '/auth/logout', { csrfToken: this.csrfToken })
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
