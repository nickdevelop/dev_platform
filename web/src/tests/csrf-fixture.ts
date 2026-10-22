// Shared test helper: registers a fake CSRF provider with the API transport.
// Specs that exercise mutating endpoints through the real client need one,
// because mutate() refuses to send a state-changing request without a token
// source (that refusal is the point of the design, see api/client.ts).
import { installCSRF } from '../api/client'

export const testCSRFToken = 'csrf-test'

export function installTestCSRF(token = testCSRFToken): void {
  installCSRF({ get: () => token, refresh: async () => token })
}
