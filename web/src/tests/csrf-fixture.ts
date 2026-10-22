// Shared test helper: registers a fake CSRF provider with the API transport.
// Specs that exercise mutating endpoints through the real client need one,
// because mutate() refuses to send a state-changing request without a token
// source (that refusal is the point of the design, see api/client.ts).
import { installCSRF } from '../api/client'
import { useAuthStore } from '../stores/auth'

export const testCSRFToken = 'csrf-test'

export function installTestCSRF(token = testCSRFToken): void {
  installCSRF({ get: () => token, refresh: async () => token })
}

// installStoreCSRF mirrors the production wiring in main.ts: the token comes
// from the auth store, so a spec that exercises login/logout observes the token
// the server actually issued. An active Pinia must exist when a request runs.
export function installStoreCSRF(): void {
  installCSRF({
    get: () => useAuthStore().csrfToken,
    refresh: () => useAuthStore().fetchSession(),
  })
}
