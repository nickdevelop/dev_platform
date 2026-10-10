// Unified session API calls and the normalized event envelope. Request and
// response DTOs are explicitly synchronized with docs/api/openapi.yaml
// (schemas AgentSession/AgentSessionList, CreateAgentSessionRequest,
// SendAgentSessionMessageRequest) and stay JSON-stable. The SSE event payload
// mirrors the backend's agents.NormalizedEvent JSON envelope; only the fields
// the workbench renders are typed.

import { apiRequest } from './client'
import type { BackendId, Capabilities } from './agents'

export type { BackendId, Capabilities } from './agents'

// Mirrors the AgentSession status enum.
export type SessionStatus = 'active' | 'stopped' | 'disconnected'

// Mirrors the AgentSession turn enum: whether the session is processing one
// user turn. Derived from the instance lifecycle and never persisted.
export type TurnState = 'idle' | 'running'

// Mirrors #/components/schemas/AgentSession.
export interface AgentSession {
  id: string
  projectId: number
  backend: BackendId
  instanceId: string
  nativeSessionId: string
  title?: string
  status: SessionStatus
  turn: TurnState
  capabilities: Capabilities
  createdAt: string
  updatedAt: string
}

// Mirrors #/components/schemas/AgentSessionList.
export interface AgentSessionList {
  sessions: AgentSession[]
}

// Mirrors #/components/schemas/CreateAgentSessionRequest. The backend selects
// the instance: the server reuses a running one or starts one. instanceId is
// no longer accepted.
export interface CreateSessionBody {
  backend: BackendId
  title?: string
}

// Mirrors #/components/schemas/SendAgentSessionMessageRequest.
export interface SendMessageBody {
  text: string
  model?: string
}

// The normalized event union (internal/agents/events.go). Exactly one typed
// payload matches `type`; unknown carries only the bounded raw body.
export type NormalizedEventType =
  | 'lifecycle'
  | 'message'
  | 'tool_call'
  | 'tool_result'
  | 'error'
  | 'unknown'

export interface MessagePayload {
  role: string
  text: string
  final: boolean
}

export interface ToolCallPayload {
  callId: string
  name: string
  input?: unknown
}

export interface ToolResultPayload {
  callId: string
  output?: string
  isError: boolean
}

export interface ErrorPayload {
  code?: string
  message: string
}

export interface LifecyclePayload {
  state: string
  detail?: string
}

export interface NormalizedEvent {
  instanceId: string
  sessionId?: string
  backend: BackendId
  sequence: number
  timestamp: string
  type: NormalizedEventType
  // True for an event read from the agent's own transcript rather than
  // observed live. Realtime events omit it.
  replayed?: boolean
  lifecycle?: LifecyclePayload
  message?: MessagePayload
  toolCall?: ToolCallPayload
  toolResult?: ToolResultPayload
  error?: ErrorPayload
  raw?: unknown
}

// Mirrors #/components/schemas/AgentSessionHistory: the session's prior turns,
// oldest first, and whether older events were left out.
export interface SessionHistory {
  events: NormalizedEvent[]
  truncated: boolean
}

// GET /api/projects/{id}/sessions
export function listProjectSessions(projectId: number): Promise<AgentSessionList> {
  return apiRequest<AgentSessionList>('GET', `/projects/${projectId}/sessions`)
}

// POST /api/projects/{id}/sessions
export function createProjectSession(
  projectId: number,
  body: CreateSessionBody,
  csrfToken: string,
): Promise<AgentSession> {
  return apiRequest<AgentSession>('POST', `/projects/${projectId}/sessions`, {
    body,
    csrfToken,
  })
}

// GET /api/projects/{id}/instances/{instanceId}/sessions
export function discoverProjectInstanceSessions(
  projectId: number,
  instanceId: string,
): Promise<AgentSessionList> {
  return apiRequest<AgentSessionList>(
    'GET',
    `/projects/${projectId}/instances/${encodeURIComponent(instanceId)}/sessions`,
  )
}

// GET /api/projects/{id}/sessions/{sessionId}/history
export function getSessionHistory(
  projectId: number,
  sessionId: string,
): Promise<SessionHistory> {
  return apiRequest<SessionHistory>(
    'GET',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/history`,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/continue
export function continueProjectSession(
  projectId: number,
  sessionId: string,
  csrfToken: string,
): Promise<AgentSession> {
  return apiRequest<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/continue`,
    { csrfToken },
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/messages
export function sendProjectSessionMessage(
  projectId: number,
  sessionId: string,
  body: SendMessageBody,
  csrfToken: string,
): Promise<AgentSession> {
  return apiRequest<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/messages`,
    { body, csrfToken },
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/stop
export function stopProjectSession(
  projectId: number,
  sessionId: string,
  csrfToken: string,
): Promise<AgentSession> {
  return apiRequest<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/stop`,
    { csrfToken },
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/interrupt
export function interruptProjectSession(
  projectId: number,
  sessionId: string,
  csrfToken: string,
): Promise<AgentSession> {
  return apiRequest<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/interrupt`,
    { csrfToken },
  )
}

// The authenticated SSE endpoint for a session's normalized event stream.
// Returned as a same-origin path for EventSource.
export function sessionEventsPath(projectId: number, sessionId: string): string {
  return `/api/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/events`
}
