// Interaction API calls for capability-gated interactions.
// Generic management (list/create/delete/history, legacy
// messages/interrupt/events) stays in ./sessions; dialogue interaction
// operations live here. DTOs mirror docs/api/openapi.yaml
// (PermissionRequest/FormInfo) and stay JSON-stable.

import { get, mutate } from './client'

export type PermissionDecision = 'once' | 'always' | 'reject'

// Mirrors sessions.PermissionRequest: a pending tool approval.
export interface PermissionRequest {
  id: string
  sessionId: string
  action: string
  resources: string[]
  message?: string
}

// Mirrors sessions.FormInfo: a pending form. Fields stays opaque here; the
// workbench renders the Form.Field union generically.
export interface FormInfo {
  id: string
  sessionId: string
  title: string
  fields?: unknown
}

interface PermissionListResponse {
  permissions: PermissionRequest[]
}

interface FormListResponse {
  forms: FormInfo[]
}

// GET /api/projects/{id}/sessions/{sessionId}/permission
export function listPermissionRequests(
  projectId: number,
  sessionId: string,
): Promise<PermissionListResponse> {
  return get<PermissionListResponse>(
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/permission`,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/permission/{requestId}/reply
export function replyPermissionRequest(
  projectId: number,
  sessionId: string,
  requestId: string,
  body: { decision: PermissionDecision; message?: string },
): Promise<void> {
  return mutate<void>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/permission/${encodeURIComponent(requestId)}/reply`,
    body,
  )
}

// GET /api/projects/{id}/sessions/{sessionId}/form
export function listFormInfos(projectId: number, sessionId: string): Promise<FormListResponse> {
  return get<FormListResponse>(
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/form`,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/form/{formId}/reply
export function replyFormInfo(
  projectId: number,
  sessionId: string,
  formId: string,
  body: { answer: Record<string, unknown> },
): Promise<void> {
  return mutate<void>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/form/${encodeURIComponent(formId)}/reply`,
    body,
  )
}

// The raw (un-normalized) SSE endpoint for a session. Served as a same-origin
// path for fetch+ReadableStream (EventSource cannot carry headers and the
// platform relies on cookies). Bytes are the sidecar's OpenCodeEvent JSON.
export function rawEventsPath(projectId: number, sessionId: string): string {
  return `/api/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/raw-events`
}
