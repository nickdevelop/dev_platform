// API calls for workspace file tree, content reading, and git status.

import { get, mutate } from './client'
import type { FileContentResponse, FileTreeNode, GitStatusItem } from '../features/files/types'

interface FileTreeResponse {
  tree: FileTreeNode[]
}

interface GitStatusResponse {
  changes: GitStatusItem[]
}

export async function getFileTree(projectId: number, path?: string): Promise<FileTreeNode[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const resp = await get<FileTreeResponse>(`/projects/${projectId}/files/tree${query}`)
  return resp.tree || []
}

export function getFileContent(projectId: number, path: string): Promise<FileContentResponse> {
  const query = encodeURIComponent(path)
  return get<FileContentResponse>(`/projects/${projectId}/files/content?path=${query}`)
}

export function getFileRawUrl(projectId: number, path: string): string {
  const query = encodeURIComponent(path)
  return `/api/projects/${projectId}/files/raw?path=${query}`
}

export async function createPreviewTicket(projectId: number): Promise<string> {
  const resp = await mutate<{ ticket: string }>('POST', `/projects/${projectId}/files/preview-ticket`)
  return resp.ticket
}

export function getPreviewFileUrl(projectId: number, ticket: string, path: string): string {
  const cleanPath = path.replace(/^\/+/, '')
  return `/api/projects/${projectId}/files/preview/${ticket}/${cleanPath}`
}

export async function getGitStatus(projectId: number): Promise<GitStatusItem[]> {
  const resp = await get<GitStatusResponse>(`/projects/${projectId}/git/status`)
  return resp.changes || []
}
