// API calls for workspace file tree, content reading, and git status.

import { get, mutate } from './client'
import type { FileContentResponse, FileTreeNode, GitCommitItem, GitFileDiffResponse, GitStatusItem } from '../features/files/types'

interface FileTreeResponse {
  tree: FileTreeNode[]
}

interface GitStatusResponse {
  changes: GitStatusItem[]
}

interface GitCommitsResponse {
  commits: GitCommitItem[]
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

export async function getGitStatus(projectId: number, commitRef?: string): Promise<GitStatusItem[]> {
  const query = commitRef ? `?commit=${encodeURIComponent(commitRef)}` : ''
  const resp = await get<GitStatusResponse>(`/projects/${projectId}/git/status${query}`)
  return resp.changes || []
}

export async function getGitCommits(projectId: number, limit?: number): Promise<GitCommitItem[]> {
  const query = limit ? `?limit=${encodeURIComponent(limit)}` : ''
  const resp = await get<GitCommitsResponse>(`/projects/${projectId}/git/commits${query}`)
  return resp.commits || []
}

export function getGitFileDiff(projectId: number, path: string, commitRef?: string): Promise<GitFileDiffResponse> {
  const params = new URLSearchParams({ path })
  if (commitRef) {
    params.set('commit', commitRef)
  }
  return get<GitFileDiffResponse>(`/projects/${projectId}/git/diff?${params.toString()}`)
}

export function stageGitFiles(projectId: number, paths?: string[]): Promise<{ ok: boolean }> {
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/stage`, { paths: paths || [] })
}

export function unstageGitFiles(projectId: number, paths?: string[]): Promise<{ ok: boolean }> {
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/unstage`, { paths: paths || [] })
}

export function discardGitFile(projectId: number, path: string): Promise<{ ok: boolean }> {
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/discard`, { path })
}

export function commitGitChanges(projectId: number, message: string): Promise<GitCommitItem> {
  return mutate<GitCommitItem>('POST', `/projects/${projectId}/git/commit`, { message })
}

export function getGitBranches(projectId: number): Promise<{ branches: string[]; current: string }> {
  return get<{ branches: string[]; current: string }>(`/projects/${projectId}/git/branches`)
}

export function checkoutGitBranch(projectId: number, branch: string, create?: boolean): Promise<{ ok: boolean }> {
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/branches`, { branch, create: Boolean(create) })
}
