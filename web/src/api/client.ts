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

export interface RequestOptions {
  body?: unknown
  csrfToken?: string
  // Set for endpoints whose 401 means "wrong password" rather than "session
  // expired" (login), so the global expiry handling does not fire.
  ignoreUnauthorized?: boolean
}

export async function apiRequest<T>(
  method: string,
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const headers: Record<string, string> = {}
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  if (options.csrfToken) {
    headers['X-CSRF-Token'] = options.csrfToken
  }

  const response = await fetch(`/api${path}`, {
    method,
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  })

  // A 401 on a normal call means the session expired: notify the global
  // handler and report it as UnauthorizedError. A 401 on a login call
  // (ignoreUnauthorized) instead means bad credentials, so fall through and
  // map the server's error body (e.g. invalid_credentials) rather than
  // reporting a session-expiry.
  if (response.status === 401 && !options.ignoreUnauthorized) {
    unauthorizedHandler?.()
    throw new UnauthorizedError()
  }

  if (response.status === 204) {
    return undefined as T
  }

  let payload: unknown = null
  try {
    payload = await response.json()
  } catch {
    // A non-JSON body carries no error code; report it as unknown.
  }
  const code = isErrorResponse(payload)
    ? payload.error
    : response.status === 401
      ? 'unauthorized'
      : 'unknown_error'

  if (response.status < 200 || response.status >= 300) {
    throw new ApiError(response.status, code)
  }
  return payload as T
}

function isErrorResponse(payload: unknown): payload is { error: string } {
  return (
    typeof payload === 'object' &&
    payload !== null &&
    typeof (payload as { error?: unknown }).error === 'string'
  )
}

const errorMessages: Record<string, string> = {
  invalid_request: '请求格式不正确。',
  invalid_credentials: '密码错误。',
  too_many_attempts: '尝试次数过多，请稍后再试。',
  unauthorized: '登录状态已失效，请重新登录。',
  invalid_csrf_token: '请求校验失败，请刷新页面后重试。',
  path_not_authorized: '路径不在已授权的工作区内。',
  root_already_registered: '该工作区根目录已授权。',
  invalid_root: '目录无效或不存在。',
  root_not_found: '工作区根目录不存在。',
  not_a_directory: '该路径不是目录。',
  path_conflict: '目标路径已存在。',
  project_not_found: '项目不存在。',
  backend_disabled: '该项目已禁用此 Agent 后端，请先在项目 Agent 设置中启用。',
  instance_start_failed: 'Agent 实例启动失败，请查看诊断信息。',
  unknown_backend: '未知的 Agent 后端。',
  directory_missing: '目录已不存在。',
  invalid_target: '目标路径不合法。',
  invalid_name: '名称不合法。',
  resource_conflict: '同类型下已存在同名资源。',
  invalid_secret_value: '凭据值不能为空。',
  confirmation_mismatch: '确认输入与项目名不一致。',
  trash_unavailable: '系统回收站不可用。',
  inconsistent_state: '操作未能完成，状态可能不一致，请刷新后确认。',
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
