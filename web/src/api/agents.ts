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
  modelSelection: boolean
  toolCallEvents: boolean
  toolConfirmation: boolean
}

// Mirrors the AgentInstance/AgentBackend backend enum.
export type BackendId = 'opencode-v2' | 'pi'

// Mirrors the AgentInstance status enum.
export type InstanceStatus = 'starting' | 'running' | 'stopped' | 'failed'

// Mirrors #/components/schemas/AgentBackend.
export interface AgentBackend {
  backend: BackendId
  capabilities: Capabilities
}

// Mirrors #/components/schemas/AgentBackendList.
export interface AgentBackendList {
  backends: AgentBackend[]
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

// GET /api/agent-backends
export function listAgentBackends(): Promise<AgentBackendList> {
  return apiRequest<AgentBackendList>('GET', '/agent-backends')
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
