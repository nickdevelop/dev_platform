// Agent backend and project instance API calls. Request/response DTOs are
// explicitly synchronized with docs/api/openapi.yaml (schemas AgentCapabilities,
// AgentBackendList, AgentInstance/AgentInstanceList, StartAgentInstanceRequest)
// and the instance routes; they stay JSON-stable and independent of Go struct
// shapes. Capabilities are the source of truth for which workbench controls a
// backend supports: an omitted or false capability means the control must not
// be shown.

import { apiRequest } from './client'

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
}

// Mirrors #/components/schemas/BackendModelList: the models one backend offers
// to a project, and how current the answer is. updatedAt is when the backend was
// last asked; stale means that answer is a cached snapshot with a fresher read
// already running — show these models now, and ask again shortly.
export interface BackendModelList {
  models: ModelInfo[]
  updatedAt: string
  stale: boolean
}

// GET /api/agent-backends
export function listAgentBackends(): Promise<AgentBackendList> {
  return apiRequest<AgentBackendList>('GET', '/agent-backends')
}

// GET /api/agent-backends/diagnostics
export function fetchAgentDiagnostics(): Promise<AgentDiagnosticsReport> {
  return apiRequest<AgentDiagnosticsReport>('GET', '/agent-backends/diagnostics')
}

// GET /api/agent-services/{backend}
// Observes a backend's machine-level service. Only the platform settings page
// calls it: no business path starts or stops a service, and none polls it.
export function getServiceStatus(backend: BackendId): Promise<AgentServiceStatus> {
  return apiRequest<AgentServiceStatusResponse>(
    'GET',
    `/agent-services/${encodeURIComponent(backend)}`,
  ).then((response) => response.status)
}

// POST /api/agent-services/{backend}/start
export function startAgentService(
  backend: BackendId,
  csrfToken: string,
): Promise<AgentServiceStatus> {
  return apiRequest<AgentServiceStatusResponse>(
    'POST',
    `/agent-services/${encodeURIComponent(backend)}/start`,
    { csrfToken },
  ).then((response) => response.status)
}

// POST /api/agent-services/{backend}/stop
export function stopAgentService(
  backend: BackendId,
  csrfToken: string,
): Promise<AgentServiceStatus> {
  return apiRequest<AgentServiceStatusResponse>(
    'POST',
    `/agent-services/${encodeURIComponent(backend)}/stop`,
    { csrfToken },
  ).then((response) => response.status)
}

// GET /api/projects/{id}/instances
export function listProjectInstances(projectId: number): Promise<AgentInstanceList> {
  return apiRequest<AgentInstanceList>('GET', `/projects/${projectId}/instances`)
}

// POST /api/projects/{id}/instances
export function startProjectInstance(
  projectId: number,
  backend: BackendId,
  csrfToken: string,
): Promise<AgentInstance> {
  return apiRequest<AgentInstance>('POST', `/projects/${projectId}/instances`, {
    body: { backend } satisfies StartAgentInstanceBody,
    csrfToken,
  })
}

// POST /api/projects/{id}/instances/{instanceId}/stop
export function stopProjectInstance(
  projectId: number,
  instanceId: string,
  csrfToken: string,
): Promise<AgentInstance> {
  return apiRequest<AgentInstance>(
    'POST',
    `/projects/${projectId}/instances/${encodeURIComponent(instanceId)}/stop`,
    { csrfToken },
  )
}

// GET /api/projects/{id}/agent-backends/{backend}/models
//
// The model catalog belongs to the backend and the project's directory, not to a
// running instance, so a project that has no instance yet can still offer its
// models. refresh asks the backend now instead of accepting the cached snapshot.
export function listBackendModels(
  projectId: number,
  backend: BackendId,
  refresh = false,
): Promise<BackendModelList> {
  const query = refresh ? '?refresh=1' : ''
  return apiRequest<BackendModelList>(
    'GET',
    `/projects/${projectId}/agent-backends/${encodeURIComponent(backend)}/models${query}`,
  )
}
