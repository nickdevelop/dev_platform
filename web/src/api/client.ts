// Same-origin API client for the platform's /api routes. All DTOs passed
// through this module are JSON-stable and mirror docs/api/openapi.yaml; Go
// struct shapes never leak into the frontend.

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string) {
    super(`API ${status}: ${code}`)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

// Thrown when an API call is rejected with 401, i.e. the session is missing
// or has expired. The global session-expiry handler reacts to it.
export class UnauthorizedError extends ApiError {
  constructor() {
    super(401, 'unauthorized')
    this.name = 'UnauthorizedError'
  }
}

// Invoked once per 401 so the router can reset auth state and return the
// user to the login view. Registered by createAppRouter; null in isolation.
type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

export function setOnUnauthorized(handler: UnauthorizedHandler | null) {
  unauthorizedHandler = handler
}

// ---------------------------------------------------------------------------
// CSRF transport
//
// The server enforces double-submit CSRF on every mutating request: the
// X-CSRF-Token header must equal the platform_csrf cookie. Making each call
// site pass the token is a contract that is silently broken the first time
// someone forgets it, so the token is injected here, at the single HTTP exit.
// get() and mutate() are the only entry points: a writer cannot forget the
// header, and the method type stops a mutating verb from taking the read path.
// ---------------------------------------------------------------------------

export type MutatingMethod = 'POST' | 'PUT' | 'PATCH' | 'DELETE'

const mutatingMethods: ReadonlySet<string> = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

function isMutating(method: string): boolean {
  return mutatingMethods.has(method.toUpperCase())
}

// CSRFProvider is the session store seen from the transport: a read of the
// current token plus a refresh that obtains a new one.
export interface CSRFProvider {
  get: () => string
  refresh: () => Promise<string>
}

let csrfProvider: CSRFProvider | null = null

// installCSRF wires the session store in as the token source. The composition
// root calls this so this module never imports the store (which would invert
// the layer dependency).
export function installCSRF(provider: CSRFProvider): void {
  csrfProvider = provider
}

function requireCSRFProvider(): CSRFProvider {
  if (!csrfProvider) {
    throw new Error('api client: installCSRF() must run before mutating requests')
  }
  return csrfProvider
}

// Only one refresh may be in flight: a burst of mutating calls holding a stale
// token must not stampede the session endpoint.
let refreshInFlight: Promise<string> | null = null

function refreshCSRFToken(): Promise<string> {
  const provider = requireCSRFProvider()
  if (!refreshInFlight) {
    refreshInFlight = provider.refresh().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

// ensureCSRFToken returns the token held by the store, bootstrapping it through
// one session request on first use (the token is unknown before the app has
// asked for the session).
async function ensureCSRFToken(): Promise<string> {
  const current = requireCSRFProvider().get()
  if (current) return current
  return refreshCSRFToken()
}

interface ExchangeResult {
  status: number
  payload: unknown
}

// exchange performs one request and reads the body exactly once, so a retried
// request never has to reuse a consumed response.
async function exchange(
  method: string,
  path: string,
  body: unknown,
  csrfToken: string,
): Promise<ExchangeResult> {
  const headers: Record<string, string> = {}
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  if (csrfToken !== '') {
    headers['X-CSRF-Token'] = csrfToken
  }

  const response = await fetch(`/api${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (response.status === 204) {
    return { status: 204, payload: undefined }
  }

  let payload: unknown = null
  try {
    payload = await response.json()
  } catch {
    // A non-JSON body carries no error code; report it as unknown.
  }
  return { status: response.status, payload }
}

function isErrorResponse(payload: unknown): payload is { error: string } {
  return (
    typeof payload === 'object' &&
    payload !== null &&
    typeof (payload as { error?: unknown }).error === 'string'
  )
}

const csrfRejectionCode = 'invalid_csrf_token'

function isCSRFRejection(payload: unknown): boolean {
  return isErrorResponse(payload) && payload.error === csrfRejectionCode
}

interface SendOptions {
  body?: unknown
  csrfToken?: string
  // Set for endpoints whose 401 means "wrong password" rather than "session
  // expired" (login), so the global expiry handling does not fire.
  ignoreUnauthorized?: boolean
  // managed marks the request as sent through get()/mutate(): the transport
  // owns the CSRF token (bootstrap + one recovery attempt).
  managed?: boolean
}

function unwrap<T>(result: ExchangeResult, options: SendOptions): T {
  const { status, payload } = result

  // A 401 on a normal call means the session expired: notify the global
  // handler and report it as UnauthorizedError. A 401 on a login call
  // (ignoreUnauthorized) instead means bad credentials, so fall through and
  // map the server's error body (e.g. invalid_credentials) rather than
  // reporting a session-expiry.
  if (status === 401 && !options.ignoreUnauthorized) {
    unauthorizedHandler?.()
    throw new UnauthorizedError()
  }

  if (status >= 200 && status < 300) {
    return payload as T
  }

  const code = isErrorResponse(payload)
    ? payload.error
    : status === 401
      ? 'unauthorized'
      : 'unknown_error'
  throw new ApiError(status, code)
}

async function send<T>(method: string, path: string, options: SendOptions = {}): Promise<T> {
  const mutating = isMutating(method)
  let token = options.csrfToken ?? ''
  if (mutating && options.managed && token === '') {
    token = await ensureCSRFToken()
  }

  let result = await exchange(method, path, options.body, token)

  // A stale token is a legitimate client state (cookie TTL, server restart, a
  // second tab). Recover silently once and only for a CSRF rejection: no other
  // 403 is ever retried.
  if (mutating && options.managed && result.status === 403 && isCSRFRejection(result.payload)) {
    token = await refreshCSRFToken()
    result = await exchange(method, path, options.body, token)
  }

  return unwrap<T>(result, options)
}

// get sends a safe (read-only) request. RequireCSRF lets these through, so no
// token is attached.
export function get<T>(path: string): Promise<T> {
  return send<T>('GET', path)
}

export interface MutateOptions {
  // Set for endpoints whose 401 means "wrong password" rather than "session
  // expired" (login).
  ignoreUnauthorized?: boolean
}

// mutate sends a state-changing request and attaches the CSRF token. The method
// type keeps read-only verbs off this path and mutating verbs off get().
export function mutate<T>(
  method: MutatingMethod,
  path: string,
  body?: unknown,
  options: MutateOptions = {},
): Promise<T> {
  return send<T>(method, path, {
    body,
    ignoreUnauthorized: options.ignoreUnauthorized,
    managed: true,
  })
}

const errorMessages: Record<string, string> = {
  invalid_request: '请求格式不正确。',
  invalid_credentials: '密码错误。',
  too_many_attempts: '尝试次数过多，请稍后再试。',
  unauthorized: '登录状态已失效，请重新登录。',
  invalid_csrf_token: '请求校验失败，请刷新页面后重试。',
  invalid_appearance: '外观配置不合法。',
  agent_disabled: '该 Agent 已在平台设置中停用，请先启用。',
  auto_detected: '已自动检测到该 Agent，无需手动填写路径。',
  invalid_path: '路径无效或不可执行。',
  invalid_service: '模型服务配置不合法。',
  invalid_model: '模型配置不合法。',
  service_not_found: '模型服务不存在。',
  model_not_found: '模型不存在。',
  model_conflict: '该服务下已存在相同的模型 ID。',
  upstream_unreachable: '无法连接到模型服务，请检查 API 地址。',
  upstream_error: '模型服务返回错误，请检查配置与 Key。',
  not_a_directory: '该路径不是目录。',
  path_conflict: '目标路径已存在。',
  project_not_found: '项目不存在。',
  // The opencode service is a third-party process the platform never starts on
  // its own, so every message about it points at the one place an operator can
  // act. These two strings are the product's wording, not a paraphrase.
  service_not_running: 'opencode service 未启动，请到平台设置 → Agent 检测并启动。',
  service_already_running: 'opencode service 已在运行。',
  service_unsupported: '该 Agent 后端不需要单独启动 service。',
  instance_start_failed: 'Agent 实例启动失败，请查看诊断信息。',
  unknown_backend: '未知的 Agent 后端。',
  directory_missing: '目录已不存在。',
  invalid_target: '目标路径不合法。',
  project_not_authorized: '项目路径已失效或不可访问，请刷新后重试。',
  invalid_name: '名称不合法。',
  confirmation_mismatch: '确认输入与项目名不一致。',
  instance_stop_failed: '项目的 Agent 实例未能全部停止，项目未被移除，请稍后重试。',
  inconsistent_state: '操作未能完成，状态可能不一致，请刷新后确认。',
  permission_expired: '该工具确认请求已过期（对方已处理或超时），列表已刷新。',
  form_expired: '该提问已过期（对方已处理或超时），列表已刷新。',
  server_error: '服务内部错误。',
  unknown_error: '发生未知错误。',
}

// Maps an API failure to a user-facing message. Never exposes tokens or
// internal details.
export function describeApiError(error: unknown, fallback = '操作失败，请重试。'): string {
  if (error instanceof ApiError) {
    return errorMessages[error.code] ?? fallback
  }
  return fallback
}
