// Project API calls. Request/response DTOs are explicitly synchronized with
// docs/api/openapi.yaml (schemas Project, ProjectList and the project request
// bodies); they stay JSON-stable and independent of Go struct shapes.

import { get, mutate } from './client'

// Mirrors #/components/schemas/Project.
export interface Project {
  id: number
  name: string
  path: string
}

// Mirrors #/components/schemas/ProjectList.
export interface ProjectList {
  projects: Project[]
}

export function listProjects(): Promise<ProjectList> {
  return get<ProjectList>('/projects')
}

// Mirrors #/components/schemas/RegisterProjectRequest.
export function registerProject(path: string): Promise<Project> {
  return mutate<Project>('POST', '/projects', { path })
}

// Mirrors #/components/schemas/CreateProjectRequest.
export function createProject(parent: string, name: string): Promise<Project> {
  return mutate<Project>('POST', '/projects/create', { parent, name })
}

// Mirrors #/components/schemas/MoveProjectRequest.
export function moveProject(id: number, path: string, confirmName: string): Promise<Project> {
  return mutate<Project>('POST', `/projects/${id}/move`, { path, confirmName })
}

// Mirrors #/components/schemas/RenameProjectRequest.
export function renameProject(id: number, name: string, confirmName: string): Promise<Project> {
  return mutate<Project>('POST', `/projects/${id}/rename`, { name, confirmName })
}

// Updates only the project's display name without touching the directory on disk.
export function updateProjectName(id: number, name: string): Promise<Project> {
  return mutate<Project>('POST', `/projects/${id}/name`, { name })
}

// Removes the project from the platform, stopping its live instances first; the
// directory on disk is never touched.
export function unregisterProject(id: number): Promise<void> {
  return mutate<void>('POST', `/projects/${id}/unregister`)
}

export interface FileItem {
  path: string
  isDir: boolean
}

export interface ProjectFilesResponse {
  files: FileItem[]
}

export function searchProjectFiles(
  projectId: number,
  query = '',
  limit = 50,
): Promise<ProjectFilesResponse> {
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  if (limit) params.set('limit', String(limit))
  const qs = params.toString() ? `?${params.toString()}` : ''
  return get<ProjectFilesResponse>(`/projects/${projectId}/files${qs}`)
}
