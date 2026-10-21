// API calls for workspace file tree, content reading, and git status.

import { apiRequest } from './client'
import type { FileContentResponse, FileTreeNode, GitStatusItem } from '../features/files/types'

interface FileTreeResponse {
  tree: FileTreeNode[]
}

interface GitStatusResponse {
  changes: GitStatusItem[]
}

export async function getFileTree(projectId: number, path?: string): Promise<FileTreeNode[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const resp = await apiRequest<FileTreeResponse>('GET', `/projects/${projectId}/files/tree${query}`)
  return resp.tree || []
}

export function getFileContent(projectId: number, path: string): Promise<FileContentResponse> {
  const query = encodeURIComponent(path)
  return apiRequest<FileContentResponse>('GET', `/projects/${projectId}/files/content?path=${query}`)
}

export function getFileRawUrl(projectId: number, path: string): string {
  const query = encodeURIComponent(path)
  return `/api/projects/${projectId}/files/raw?path=${query}`
}

export async function getGitStatus(projectId: number): Promise<GitStatusItem[]> {
  const resp = await apiRequest<GitStatusResponse>('GET', `/projects/${projectId}/git/status`)
  return resp.changes || []
}
