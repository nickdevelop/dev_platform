// opencode-specific interaction API calls under the /opencode prefix.
// Generic management (list/create/delete/discover/history, legacy
// messages/interrupt/events) stays in ./sessions; agent-specific dialogue
// operations live here so /pi can diverge independently. DTOs mirror
// docs/api/openapi.yaml (OpencodePermission/OpencodeForm) and stay JSON-stable.

import { apiRequest } from './client'

export type PermissionDecision = 'once' | 'always' | 'reject'

// Mirrors sessions.PermissionRequest: a pending tool approval.
export interface OpencodePermission {
  id: string
  sessionId: string
  action: string
  resources: string[]
  message?: string
}

// Mirrors sessions.FormInfo: a pending form. Fields stays opaque here; the
// workbench renders the Form.Field union generically.
export interface OpencodeForm {
  id: string
  sessionId: string
  title: string
  fields?: unknown
}

interface PermissionListResponse {
  permissions: OpencodePermission[]
}

interface FormListResponse {
  forms: OpencodeForm[]
}

// POST /api/projects/{id}/opencode/sessions/{sessionId}/prompt (new prefix;
// same body and 202 semantics as the legacy messages route).
export function opencodePrompt(
  projectId: number,
  sessionId: string,
  body: { text: string; model?: string },
  csrfToken: string,
): Promise<unknown> {
  return apiRequest<unknown>(
    'POST',
    `/projects/${projectId}/opencode/sessions/${encodeURIComponent(sessionId)}/prompt`,
    { body, csrfToken },
  )
}

// GET /api/projects/{id}/opencode/sessions/{sessionId}/permission
export function listOpencodePermissions(
  projectId: number,
  sessionId: string,
): Promise<PermissionListResponse> {
  return apiRequest<PermissionListResponse>(
    'GET',
    `/projects/${projectId}/opencode/sessions/${encodeURIComponent(sessionId)}/permission`,
  )
}

// POST /api/projects/{id}/opencode/sessions/{sessionId}/permission/{requestId}/reply
export function replyOpencodePermission(
  projectId: number,
  sessionId: string,
  requestId: string,
  body: { decision: PermissionDecision; message?: string },
  csrfToken: string,
): Promise<void> {
  return apiRequest<void>(
    'POST',
    `/projects/${projectId}/opencode/sessions/${encodeURIComponent(sessionId)}/permission/${encodeURIComponent(requestId)}/reply`,
    { body, csrfToken },
  )
}

// GET /api/projects/{id}/opencode/sessions/{sessionId}/form
export function listOpencodeForms(projectId: number, sessionId: string): Promise<FormListResponse> {
  return apiRequest<FormListResponse>(
    'GET',
    `/projects/${projectId}/opencode/sessions/${encodeURIComponent(sessionId)}/form`,
  )
}

// POST /api/projects/{id}/opencode/sessions/{sessionId}/form/{formId}/reply
export function replyOpencodeForm(
  projectId: number,
  sessionId: string,
  formId: string,
  body: { answer: Record<string, unknown> },
  csrfToken: string,
): Promise<void> {
  return apiRequest<void>(
    'POST',
    `/projects/${projectId}/opencode/sessions/${encodeURIComponent(sessionId)}/form/${encodeURIComponent(formId)}/reply`,
    { body, csrfToken },
  )
}

// The raw (un-normalized) SSE endpoint for a session. Served as a same-origin
// path for fetch+ReadableStream (EventSource cannot carry headers and the
// platform relies on cookies). Bytes are the sidecar's OpenCodeEvent JSON.
export function opencodeRawEventsPath(projectId: number, sessionId: string): string {
  return `/api/projects/${projectId}/opencode/sessions/${encodeURIComponent(sessionId)}/raw-events`
}
