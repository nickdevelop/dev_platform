// Unified session API calls and the normalized event envelope. Request and
// response DTOs are explicitly synchronized with docs/api/openapi.yaml
// (schemas AgentSession/AgentSessionList, CreateAgentSessionRequest,
// SendAgentSessionMessageRequest) and stay JSON-stable. The SSE event payload
// mirrors the backend's agents.NormalizedEvent JSON envelope; only the fields
// the workbench renders are typed.

import { get, mutate } from './client'
import type { BackendId, Capabilities } from './agents'
import type { FormInfo, PermissionRequest } from './interactions'

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
  model?: string
  status: SessionStatus
  turn: TurnState
  capabilities: Capabilities
  createdAt: string
  updatedAt: string
}

// Mirrors #/components/schemas/AgentSessionList.
export interface AgentSessionList {
  sessions: AgentSession[]
  total?: number
}

export interface ListSessionsOptions {
  limit?: number
  offset?: number
  turn?: string
}

// Mirrors #/components/schemas/CreateAgentSessionRequest. The backend selects
// the instance: the server reuses a running one or starts one. instanceId is
// no longer accepted. message and model are optional: when present, the
// session is created on the model and the message becomes its first turn in
// the same request.
export interface CreateSessionBody {
  backend: BackendId
  title?: string
  message?: string
  model?: string
  images?: ImageAttachment[]
}

// Mirrors #/components/schemas/CreateAgentSessionResult. messageError carries
// the code of a refused optional first message; the session is always live.
export interface CreateSessionResult {
  session: AgentSession
  messageError?: string
}

export type ImageMediaType = 'image/jpeg' | 'image/png' | 'image/webp' | 'image/gif'

export interface ImageAttachment {
  id: string
  name: string
  mediaType: ImageMediaType
  data: string
  size: number
}

// Mirrors #/components/schemas/SendAgentSessionMessageRequest.
export interface SendMessageBody {
  text: string
  model?: string
  images?: ImageAttachment[]
}

// The normalized event union (internal/agents/events.go). Exactly one typed
// payload matches `type`; unknown carries only the bounded raw body.
export type NormalizedEventType =
  | 'lifecycle'
  | 'message'
  | 'thinking'
  | 'tool_call'
  | 'tool_result'
  | 'error'
  | 'turn'
  | 'interaction'
  | 'unknown'

// Mirrors agents.MessageModel: the model that produced one assistant message.
// id is the provider-scoped "provider/model" form; variant is set when the
// model ran a named variant.
export interface MessageModel {
  id: string
  provider: string
  variant?: string
}

export interface MessagePayload {
  role: string
  text: string
  images?: ImageAttachment[]
  final: boolean
  model?: MessageModel
  // Exact per-message token usage from the backend, present on final
  // assistant messages when the backend reports it. Never estimated.
  usage?: MessageUsage
}

// Mirrors agents.MessageUsage: exact backend-reported token usage.
export interface MessageUsage {
  input: number
  output: number
  reasoning?: number
  cacheRead?: number
  cacheWrite?: number
}

// Mirrors agents.ThinkingPayload: the agent's reasoning, or one streaming chunk
// of it. Reasoning is its own event kind and its own rendered entry.
export interface ThinkingPayload {
  text: string
  final: boolean
}

export interface ToolCallPayload {
  callId: string
  name: string
  input?: unknown
}

export interface FileDiffInfo {
  file: string
  patch?: string
  additions: number
  deletions: number
  status?: 'added' | 'deleted' | 'modified'
}

export interface ToolResultPayload {
  callId: string
  output?: string
  isError: boolean
  metadata?: { files?: FileDiffInfo[] }
}

export interface ErrorPayload {
  code?: string
  message: string
}

export interface LifecyclePayload {
  state: string
  detail?: string
  // Exact turn token usage from the backend (opencode step finish), used as
  // the freshest context anchor. Absent on every other lifecycle event.
  usage?: MessageUsage
  // Dynamic context window in tokens reported by the backend (e.g. from session
  // projections or context events).
  contextLimit?: number
}

// Mirrors agents.TurnPayload: a turn boundary. The started and ended events of
// one turn share id. Only these events change whether a session is busy.
export interface TurnPayload {
  id: string
  phase: 'started' | 'ended'
  outcome?: 'completed' | 'cancelled' | 'failed'
}

// Mirrors agents.InteractionPayload: a tool confirmation or question form the
// agent waits on. A requested interaction carries its permission or form; a
// resolved one carries only the id.
export interface InteractionPayload {
  id: string
  kind: 'permission' | 'form'
  phase: 'requested' | 'resolved'
  permission?: PermissionRequest
  form?: FormInfo
}

// Payload of the "interactions" SSE frame a stream opens with: the session's
// pending set at the join point, which replaces the client's set.
export interface InteractionSnapshot {
  permissions: PermissionRequest[]
  forms: FormInfo[]
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
  thinking?: ThinkingPayload
  toolCall?: ToolCallPayload
  toolResult?: ToolResultPayload
  error?: ErrorPayload
  turn?: TurnPayload
  interaction?: InteractionPayload
  raw?: unknown
}

// Mirrors #/components/schemas/AgentSessionHistory: the session's prior turns,
// oldest first, and whether older events were left out.
export interface SessionHistory {
  events: NormalizedEvent[]
  truncated: boolean
}

// GET /api/projects/{id}/sessions
export function listProjectSessions(
  projectId: number,
  options?: ListSessionsOptions,
): Promise<AgentSessionList> {
  const params = new URLSearchParams()
  if (options?.limit !== undefined && options.limit > 0) {
    params.set('limit', String(options.limit))
  }
  if (options?.offset !== undefined && options.offset > 0) {
    params.set('offset', String(options.offset))
  }
  if (options?.turn !== undefined && options.turn !== '') {
    params.set('turn', options.turn)
  }
  const qs = params.toString()
  const query = qs ? `?${qs}` : ''
  return get<AgentSessionList>(`/projects/${projectId}/sessions${query}`)
}

// POST /api/projects/{id}/sessions
export function createProjectSession(
  projectId: number,
  body: CreateSessionBody,
): Promise<CreateSessionResult> {
  return mutate<CreateSessionResult>('POST', `/projects/${projectId}/sessions`, body)
}

// GET /api/projects/{id}/sessions/{sessionId}/history
export function getSessionHistory(
  projectId: number,
  sessionId: string,
): Promise<SessionHistory> {
  return get<SessionHistory>(
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/history`,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/continue
export function continueProjectSession(
  projectId: number,
  sessionId: string,
): Promise<AgentSession> {
  return mutate<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/continue`,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/messages
export function sendProjectSessionMessage(
  projectId: number,
  sessionId: string,
  body: SendMessageBody,
): Promise<AgentSession> {
  return mutate<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/messages`,
    body,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/stop
export function stopProjectSession(
  projectId: number,
  sessionId: string,
): Promise<AgentSession> {
  return mutate<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/stop`,
  )
}

// POST /api/projects/{id}/sessions/{sessionId}/interrupt
export function interruptProjectSession(
  projectId: number,
  sessionId: string,
): Promise<AgentSession> {
  return mutate<AgentSession>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/interrupt`,
  )
}

// DELETE /api/projects/{id}/sessions/{sessionId}. Removes the platform's index
// entry only; the agent-native session is not touched.
export function deleteProjectSession(projectId: number, sessionId: string): Promise<void> {
  return mutate<void>(
    'DELETE',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}`,
  )
}

// The authenticated SSE endpoint for a session's normalized event stream.
// Returned as a same-origin path for EventSource. A resuming dial passes the
// id of the last frame it applied so the server replays what was missed;
// EventSource carries that id itself as the Last-Event-ID header when the
// browser reconnects, but a freshly constructed EventSource cannot, so the
// workbench's manual re-dial passes it as a query parameter instead.
export function sessionEventsPath(
  projectId: number,
  sessionId: string,
  lastEventId?: string,
): string {
  const base = `/api/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/events`
  return lastEventId ? `${base}?lastEventId=${encodeURIComponent(lastEventId)}` : base
}

export type CommandKind = 'command' | 'skill' | 'prompt'

export interface AgentCommand {
  name: string
  description: string
  kind: CommandKind
  argumentHint?: string
}

export interface SessionCommandsResponse {
  commands: AgentCommand[]
}

// GET /api/projects/{id}/sessions/{sessionId}/commands
export function listSessionCommands(
  projectId: number,
  sessionId: string,
): Promise<SessionCommandsResponse> {
  return get<SessionCommandsResponse>(
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/commands`,
  )
}
