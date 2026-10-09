// Workspace-root API calls. Request/response DTOs are explicitly synchronized
// with docs/api/openapi.yaml (schemas WorkspaceRoot, WorkspaceRootList and
// AddWorkspaceRootRequest); they stay JSON-stable and independent of Go struct
// shapes. Every project register/create path is authorized against these
// roots, so a fresh deployment must add at least one before projects work.

import { apiRequest } from './client'

// Mirrors #/components/schemas/WorkspaceRoot.
export interface WorkspaceRoot {
  id: number
  path: string
  createdAt: string
}

// Mirrors #/components/schemas/WorkspaceRootList.
export interface WorkspaceRootList {
  roots: WorkspaceRoot[]
}

export function listWorkspaceRoots(): Promise<WorkspaceRootList> {
  return apiRequest<WorkspaceRootList>('GET', '/workspaces/roots')
}

// Mirrors #/components/schemas/AddWorkspaceRootRequest.
export function addWorkspaceRoot(path: string, csrfToken: string): Promise<WorkspaceRoot> {
  return apiRequest<WorkspaceRoot>('POST', '/workspaces/roots', { body: { path }, csrfToken })
}

export function removeWorkspaceRoot(id: number, csrfToken: string): Promise<void> {
  return apiRequest<void>('DELETE', `/workspaces/roots/${id}`, { csrfToken })
}
