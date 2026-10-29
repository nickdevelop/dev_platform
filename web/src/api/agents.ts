// Agent backend and project instance API calls. Request/response DTOs are
// explicitly synchronized with docs/api/openapi.yaml (schemas AgentCapabilities,
// AgentBackendList, AgentInstance/AgentInstanceList, StartAgentInstanceRequest)
// and the instance routes; they stay JSON-stable and independent of Go struct
// shapes. Capabilities are the source of truth for which workbench controls a
// backend supports: an omitted or false capability means the control must not
// be shown.

import { get, mutate } from './client'

// Mirrors #/components/schemas/AgentCapabilities. Every field is an explicit
// boolean so a capability the backend lacks is visibly false.
export interface Capabilities {
  streaming: boolean
  sessionDiscovery: boolean
  sessionContinue: boolean
  stop: boolean
  interrupt: boolean
  modelSelection: boolean
  toolCallEvents: boolean
  toolConfirmation: boolean
  sessionHistory: boolean
  rawEvents: boolean
}

// A backend id as the platform catalog names it. The set is not enumerated in
// the UI: every label, order and service fact comes from the backend list.
export type BackendId = string

// Mirrors the AgentInstance status enum.
export type InstanceStatus = 'starting' | 'running' | 'stopped' | 'failed'

// Mirrors #/components/schemas/AgentBackend.
export interface AgentBackend {
  backend: BackendId
  capabilities: Capabilities
  // Platform-level enablement (the only switch): a disabled backend cannot be
  // chosen for new sessions or instances. Absent is treated as enabled (the
  // server always sends it).
  enabled?: boolean
  // Display name from the platform catalog; falls back to the raw id.
  label: string
  // Presentation order from the platform catalog; lower comes first.
  order: number
  // Whether the backend runs a machine-level service the operator can start
  // and stop.
  serviceSupported: boolean
  // Optional visual tone preference from the backend descriptor (e.g. "info", "warning", "primary").
  tone?: string
  // Optional exact brand colour (#RRGGBB) from the backend descriptor. Takes
  // precedence over tone.
  color?: string
  // Optional short logo identifier (e.g. "OC", "PI", "DS") from the descriptor.
  shortName?: string
}

// Mirrors #/components/schemas/AgentBackendList.
export interface AgentBackendList {
  backends: AgentBackend[]
}

// Mirrors #/components/schemas/AgentDiagnostic. The message is actionable and
// already redacted by the server.
export interface AgentDiagnostic {
  kind: 'executable' | 'version' | 'api' | 'event_stream' | 'adapter'
  status: 'ok' | 'warning' | 'error' | 'unknown'
  message: string
  detail?: string
}

// Mirrors #/components/schemas/AgentBackendDiagnostics.
export interface BackendDiagnostics {
  backend: BackendId
  status: 'ok' | 'warning' | 'error' | 'unknown'
  version?: string
  testedVersion?: string
  checks: AgentDiagnostic[]
  capabilities?: Capabilities
}

// Mirrors #/components/schemas/AgentDiagnosticsReport.
export interface AgentDiagnosticsReport {
  diagnostics: BackendDiagnostics[]
}

// Mirrors #/components/schemas/AgentServiceStatus.
export type ServiceState = 'running' | 'stopped' | 'unreachable'

export interface AgentServiceStatus {
  backend: BackendId
  // False when the backend has no machine-level service at all (pi), in which
  // case no start/stop control may be offered.
  supported: boolean
  state: ServiceState
  endpoint?: string
  pid?: number
  version?: string
  executable?: string
  // Actionable, already-redacted reason for an unusable service.
  detail?: string
  checkedAt: string
}

// Mirrors #/components/schemas/AgentServiceStatusResponse.
export interface AgentServiceStatusResponse {
  status: AgentServiceStatus
}

// Mirrors #/components/schemas/AgentInstance.
export interface AgentInstance {
  id: string
  projectId: number
  backend: BackendId
  directory: string
  status: InstanceStatus
  endpoint?: string
  error?: string
  createdAt: string
  updatedAt: string
}

// Mirrors #/components/schemas/AgentInstanceList.
export interface AgentInstanceList {
  instances: AgentInstance[]
}

// Mirrors #/components/schemas/StartAgentInstanceRequest.
export interface StartAgentInstanceBody {
  backend: BackendId
}

// Mirrors #/components/schemas/ModelInfo. The id uses the provider/model form
// accepted by SendMessageBody.model, with an optional #variant suffix.
// contextLimit is the model's context window in tokens when the backend
// reports one; absent means unknown and no percentage may be rendered.
export interface ModelInfo {
  id: string
  name: string
  provider: string
  variants?: string[]
  contextLimit?: number
  supportsImages?: boolean
  enabled?: boolean
}

// Mirrors #/components/schemas/AgentModelList: the platform-wide model snapshot
// persisted for one backend. updatedAt is null when the backend has never been
// refreshed; an empty models array with a non-null updatedAt means the backend
// reported no models on its last refresh.
export interface AgentModelList {
  models: ModelInfo[]
  updatedAt: string | null
}

// GET /api/agent-backends
export function listAgentBackends(): Promise<AgentBackendList> {
  return get<AgentBackendList>('/agent-backends')
}

// GET /api/agent-backends/diagnostics
export function fetchAgentDiagnostics(): Promise<AgentDiagnosticsReport> {
  return get<AgentDiagnosticsReport>('/agent-backends/diagnostics')
}

// GET /api/agent-services/{backend}
// Observes a backend's machine-level service. Only the platform settings page
// calls it: no business path starts or stops a service, and none polls it.
export function getServiceStatus(backend: BackendId): Promise<AgentServiceStatus> {
  return get<AgentServiceStatusResponse>(`/agent-services/${encodeURIComponent(backend)}`).then(
    (response) => response.status,
  )
}

// POST /api/agent-services/{backend}/start
export function startAgentService(backend: BackendId): Promise<AgentServiceStatus> {
  return mutate<AgentServiceStatusResponse>(
    'POST',
    `/agent-services/${encodeURIComponent(backend)}/start`,
  ).then((response) => response.status)
}

// POST /api/agent-services/{backend}/stop
export function stopAgentService(backend: BackendId): Promise<AgentServiceStatus> {
  return mutate<AgentServiceStatusResponse>(
    'POST',
    `/agent-services/${encodeURIComponent(backend)}/stop`,
  ).then((response) => response.status)
}

// GET /api/projects/{id}/instances
export function listProjectInstances(projectId: number): Promise<AgentInstanceList> {
  return get<AgentInstanceList>(`/projects/${projectId}/instances`)
}

// POST /api/projects/{id}/instances
export function startProjectInstance(projectId: number, backend: BackendId): Promise<AgentInstance> {
  return mutate<AgentInstance>('POST', `/projects/${projectId}/instances`, {
    backend,
  } satisfies StartAgentInstanceBody)
}

// POST /api/projects/{id}/instances/{instanceId}/stop
export function stopProjectInstance(projectId: number, instanceId: string): Promise<AgentInstance> {
  return mutate<AgentInstance>(
    'POST',
    `/projects/${projectId}/instances/${encodeURIComponent(instanceId)}/stop`,
  )
}

// GET /api/agent-backends/{backend}/models
//
// Reads the platform's persisted model snapshot for a backend. It never asks
// the backend for its catalog: a stopped or unreachable agent still serves its
// last successful refresh.
export function listAgentBackendModels(backend: BackendId): Promise<AgentModelList> {
  return get<AgentModelList>(`/agent-backends/${encodeURIComponent(backend)}/models`)
}

// POST /api/agent-backends/{backend}/models/refresh
//
// Asks the backend for its current catalog and replaces the persisted snapshot.
// This is the only path that contacts the agent; it is always triggered by the
// operator's refresh button.
export function refreshAgentBackendModels(backend: BackendId): Promise<AgentModelList> {
  return mutate<AgentModelList>(
    'POST',
    `/agent-backends/${encodeURIComponent(backend)}/models/refresh`,
  )
}

export interface AgentModelPatch {
  modelId?: string
  enabled?: boolean
  name?: string
  contextLimit?: number
  supportsImages?: boolean
}

// PATCH /api/agent-backends/{backend}/models
//
// Updates platform-level configuration of an agent model (enable/disable, custom name, etc.).
export function updateAgentBackendModel(
  backend: BackendId,
  modelId: string,
  patch: Omit<AgentModelPatch, 'modelId'>,
): Promise<ModelInfo> {
  return mutate<ModelInfo>(
    'PATCH',
    `/agent-backends/${encodeURIComponent(backend)}/models`,
    { modelId, ...patch },
  )
}
