# ADR 0001 — CSRF token lives in the HTTP transport, not in call sites

- Status: accepted
- Date: 2025-10 (S1–S6 of the CSRF hardening plan)
- Related: `web/src/api/client.ts`, `internal/app/auth/csrf.go`,
  `internal/transport/http/router.go`, `internal/transport/http/csrf_matrix_guard_test.go`,
  `web/src/tests/csrf-coverage.spec.ts`

## Context

The platform protects every mutating `/api` request with a double-submit CSRF
scheme: the server issues a `platform_csrf` cookie (not HttpOnly, `SameSite=Strict`)
and `RequireCSRF` rejects any non-safe method whose `X-CSRF-Token` header does not
equal that cookie. `RequireCSRF` is installed once, globally, in `NewRouter`.

The frontend contract was opt-in: `apiRequest(method, path, { csrfToken })`
attached the header **only if the caller passed the token**. Call sites received
the token as an extra parameter from the auth store, so every new mutating API
function was a chance to forget it.

That is what happened: `POST /api/model-services/fetch-models` and
`POST /api/models/test` omitted the token and answered `403 invalid_csrf_token`.
Both looked like reads (`fetchModels`, `testModel`) but are POSTs, which is
exactly the case a per-call contract fails to catch. Two of nineteen mutating
functions were broken, and no test failed: unit tests stubbed `fetch`, backend
tests bypassed the middleware, and the e2e suite only exercised one mutation.

## Decision

1. **The transport owns the token.** `web/src/api/client.ts` exposes two
   functions and nothing else reaches the network:
   - `get<T>(path)` — safe methods, never attaches a token.
   - `mutate<T>(method, path, body?)` — `POST|PUT|PATCH|DELETE` only (typed), always
     attaches `X-CSRF-Token`.
   The transitional `apiRequest` entry point was deleted, so there is no second
   way to build a request.
2. **The session store is the single source of truth.** `main.ts` registers a
   provider (`installCSRF({ get, refresh })`) that reads the auth store and
   refreshes it through `GET /api/auth/session`. `client.ts` never imports the
   store, so the layer dependency stays `api → (provider) ← composition root`.
3. **Bootstrap and recovery are the transport's job.** When the store holds no
   token, `mutate` fetches one (one session request, single-flight). When the
   server answers `403 invalid_csrf_token`, the transport refreshes the token and
   retries **once**; any other 403/401 is never retried.
4. **The token rotates at identity changes.** Login and logout issue a fresh
   token, so a value observed before that change is dead afterwards.
5. **Four guards keep it true for code not written yet:**
   - `csrf_matrix_guard_test.go` walks `router.Routes()` and asserts, for every
     non-safe route, `403 invalid_csrf_token` without the header and something
     other than a CSRF rejection with it.
   - `csrf-coverage.spec.ts` lints the source tree: no bare `fetch(`, no
     hand-written `X-CSRF-Token`, no `csrfToken` outside the store and the
     transport, no second entry point, no non-mutating verb through `mutate`.
   - `openapi_guard_test.go` asserts every documented mutating operation declares
     the `X-CSRF-Token` parameter.
   - `e2e/security.spec.ts` drives the two probe endpoints end to end.
6. **Rejections are auditable.** `RequireCSRF` sets a context flag and the access
   log appends `csrf=rejected`, so a client that systematically drops the header
   is visible in the logs.

## Consequences

- Adding an API function cannot omit the token: the parameter no longer exists,
  and the source-tree guard fails a build that reintroduces a shortcut.
- Both the browser and the retry path need a token source. A spec that drives a
  mutating endpoint through the real client must install one
  (`web/src/tests/csrf-fixture.ts`), which is also what keeps the tests honest
  about the production wiring.
- A stale token is no longer a user-visible error; only a second consecutive
  rejection surfaces, with the existing "请刷新页面后重试" message.
- Scripts (see the curl walkthrough in `docs/operations.md`) must re-read the
  cookie after login, because login rotates the token.
- The client must still send the header as well as the cookie: `SameSite=Strict`
  is defence in depth, not a replacement for the double submit.

## Rejected alternatives

- **Keep the explicit parameter and add tests.** Tests only cover call sites that
  already exist; the failure mode is the one nobody wrote yet.
- **Send the token from the CSRF cookie automatically in every request,
  including reads.** Safe methods do not need it, and extra headers widen the
  surface for no benefit.
- **Synchronizer token pattern (server-side per-session token).** Requires a
  store lookup on every mutation and loses the current stateless property.
- **Signed double-submit (HMAC).** Stronger against cookie injection and able to
  revoke tokens server-side, but needs a shared secret and invalidates every
  token on restart. Deferred: `SameSite=Strict` plus the double submit already
  covers the single-instance LAN threat model, and the transport's one-shot
  recovery makes a restart-induced rejection invisible to users. Revisit when the
  platform is exposed beyond a trusted LAN or gains multiple instances.
- **`__Host-` cookie prefix.** Requires `Secure`, i.e. TLS; the service targets
  plain-HTTP LAN deployments today.
