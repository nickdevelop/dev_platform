// Resource API calls. Request/response DTOs are explicitly synchronized with
// docs/api/openapi.yaml (schemas ResourceDefinition, ResourceList,
// SessionPromptList, ResourceDefinitionRequest, SecretMetadata, SyncPreview
// and the sync request bodies) and stay JSON-stable.
//
// Secret handling: a credential value is write-only. It is sent once in a
// ResourceDefinitionRequest or SecretWriteRequest and never comes back; only
// the opaque secretRef is returned.

import { apiRequest } from './client'
import type { BackendId } from './agents'

// Mirrors the ResourceKind enum.
export type ResourceKind = 'skill' | 'mcp' | 'prompt'

// Mirrors the PromptScope enum.
export type PromptScope = 'project' | 'session'

// Mirrors the SyncState enum.
export type SyncState = 'pending' | 'applied' | 'conflict' | 'failed' | 'unsupported'

// Mirrors the ConflictKind enum.
export type ConflictKind = 'unmanaged-target' | 'external-drift' | 'target-invalid'

// Mirrors #/components/schemas/ResourceDefinition. secretRef is an opaque
// reference; no stored credential value is ever present.
export interface ResourceDefinition {
  id: number
  kind: ResourceKind
  name: string
  description?: string
  content: string
  enabled: boolean
  promptScope?: PromptScope
  secretRef?: string
  backends?: BackendId[]
  createdAt: string
  updatedAt: string
}

// Mirrors #/components/schemas/ResourceList.
export interface ResourceList {
  resources: ResourceDefinition[]
}

// Mirrors #/components/schemas/SessionPromptList.
export interface SessionPromptList {
  prompts: ResourceDefinition[]
}

// Mirrors #/components/schemas/ResourceDefinitionRequest. secretValue is
// write-only and omitted when the user did not change the credential.
export interface ResourceDefinitionBody {
  kind: ResourceKind
  name: string
  description?: string
  content: string
  enabled: boolean
  promptScope?: PromptScope
  secretRef?: string
  secretValue?: string
  backends?: BackendId[]
}

// Mirrors #/components/schemas/SecretMetadata.
export interface SecretMetadata {
  id: string
  present: boolean
}

// Mirrors #/components/schemas/SyncTarget.
export interface SyncTarget {
  projectId: number
  backendId?: BackendId
  path: string
}

// Mirrors #/components/schemas/SyncConflict.
export interface SyncConflict {
  kind: ConflictKind
  resourceId: number
  target: SyncTarget
  detail?: string
}

// Mirrors #/components/schemas/SyncPreview.
export interface SyncPreview {
  resourceId: number
  kind: ResourceKind
  target: SyncTarget
  state: SyncState
  changeSummary: string
  conflict?: SyncConflict
}

// Mirrors #/components/schemas/SyncPreviewList.
export interface SyncPreviewList {
  previews: SyncPreview[]
}

export function listResources(): Promise<ResourceList> {
  return apiRequest<ResourceList>('GET', '/resources')
}

export function createResource(
  body: ResourceDefinitionBody,
  csrfToken: string,
): Promise<ResourceDefinition> {
  return apiRequest<ResourceDefinition>('POST', '/resources', { body, csrfToken })
}

export function updateResource(
  id: number,
  body: ResourceDefinitionBody,
  csrfToken: string,
): Promise<ResourceDefinition> {
  return apiRequest<ResourceDefinition>('POST', `/resources/${id}/update`, { body, csrfToken })
}

export function deleteResource(id: number, csrfToken: string): Promise<void> {
  return apiRequest<void>('POST', `/resources/${id}/delete`, { csrfToken })
}

export function listProjectResources(projectId: number): Promise<ResourceList> {
  return apiRequest<ResourceList>('GET', `/projects/${projectId}/resources`)
}

export function associateProjectResource(
  projectId: number,
  resourceId: number,
  csrfToken: string,
): Promise<void> {
  return apiRequest<void>('POST', `/projects/${projectId}/resources`, {
    body: { resourceId },
    csrfToken,
  })
}

export function disassociateProjectResource(
  projectId: number,
  resourceId: number,
  csrfToken: string,
): Promise<void> {
  return apiRequest<void>('POST', `/projects/${projectId}/resources/remove`, {
    body: { resourceId },
    csrfToken,
  })
}

export function listSessionPrompts(
  projectId: number,
  sessionId: string,
): Promise<SessionPromptList> {
  return apiRequest<SessionPromptList>(
    'GET',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/prompts`,
  )
}

export function selectSessionPrompt(
  projectId: number,
  sessionId: string,
  resourceId: number,
  csrfToken: string,
): Promise<void> {
  return apiRequest<void>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/prompts`,
    { body: { resourceId }, csrfToken },
  )
}

export function removeSessionPrompt(
  projectId: number,
  sessionId: string,
  resourceId: number,
  csrfToken: string,
): Promise<void> {
  return apiRequest<void>(
    'POST',
    `/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/prompts/remove`,
    { body: { resourceId }, csrfToken },
  )
}

export function createSecret(value: string, csrfToken: string): Promise<SecretMetadata> {
  return apiRequest<SecretMetadata>('POST', '/secrets', { body: { value }, csrfToken })
}

export function deleteSecret(id: string, csrfToken: string): Promise<void> {
  return apiRequest<void>('DELETE', `/secrets/${encodeURIComponent(id)}`, { csrfToken })
}

export function previewProjectSync(projectId: number): Promise<SyncPreviewList> {
  return apiRequest<SyncPreviewList>('GET', `/projects/${projectId}/resources/sync`)
}

export function applyProjectSync(
  projectId: number,
  force: boolean,
  csrfToken: string,
): Promise<SyncPreviewList> {
  return apiRequest<SyncPreviewList>('POST', `/projects/${projectId}/resources/sync`, {
    body: { force },
    csrfToken,
  })
}

export function resolveProjectSync(
  projectId: number,
  resourceId: number,
  csrfToken: string,
): Promise<SyncPreviewList> {
  return apiRequest<SyncPreviewList>('POST', `/projects/${projectId}/resources/sync/resolve`, {
    body: { resourceId },
    csrfToken,
  })
}
