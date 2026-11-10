// API calls for workspace file tree, content reading, and git status.
// Every call accepts an optional worktreeId: absent or 0 addresses the
// project's main checkout, a non-zero value addresses one git worktree.

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

function withWorktree(params: URLSearchParams, worktreeId?: number) {
  if (worktreeId) params.set('worktreeId', String(worktreeId))
  return params
}

export async function getFileTree(
  projectId: number,
  path?: string,
  worktreeId?: number,
): Promise<FileTreeNode[]> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  if (path) params.set('path', path)
  const query = params.toString() ? `?${params.toString()}` : ''
  const resp = await get<FileTreeResponse>(`/projects/${projectId}/files/tree${query}`)
  return resp.tree || []
}

export function getFileContent(
  projectId: number,
  path: string,
  worktreeId?: number,
): Promise<FileContentResponse> {
  const params = withWorktree(new URLSearchParams({ path }), worktreeId)
  return get<FileContentResponse>(`/projects/${projectId}/files/content?${params.toString()}`)
}

export function getFileRawUrl(projectId: number, path: string, worktreeId?: number): string {
  const params = withWorktree(new URLSearchParams({ path }), worktreeId)
  return `/api/projects/${projectId}/files/raw?${params.toString()}`
}

export async function createPreviewTicket(projectId: number, worktreeId?: number): Promise<string> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  const resp = await mutate<{ ticket: string }>(
    'POST',
    `/projects/${projectId}/files/preview-ticket${query}`,
  )
  return resp.ticket
}

export function getPreviewFileUrl(projectId: number, ticket: string, path: string): string {
  const cleanPath = path.replace(/^\/+/, '')
  return `/api/projects/${projectId}/files/preview/${ticket}/${cleanPath}`
}

export async function getGitStatus(
  projectId: number,
  commitRef?: string,
  worktreeId?: number,
): Promise<GitStatusItem[]> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  if (commitRef) params.set('commit', commitRef)
  const query = params.toString() ? `?${params.toString()}` : ''
  const resp = await get<GitStatusResponse>(`/projects/${projectId}/git/status${query}`)
  return resp.changes || []
}

export async function getGitCommits(
  projectId: number,
  limit?: number,
  worktreeId?: number,
): Promise<GitCommitItem[]> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  if (limit) params.set('limit', String(limit))
  const query = params.toString() ? `?${params.toString()}` : ''
  const resp = await get<GitCommitsResponse>(`/projects/${projectId}/git/commits${query}`)
  return resp.commits || []
}

export function getGitFileDiff(
  projectId: number,
  path: string,
  commitRef?: string,
  worktreeId?: number,
): Promise<GitFileDiffResponse> {
  const params = withWorktree(new URLSearchParams({ path }), worktreeId)
  if (commitRef) {
    params.set('commit', commitRef)
  }
  return get<GitFileDiffResponse>(`/projects/${projectId}/git/diff?${params.toString()}`)
}

export function stageGitFiles(
  projectId: number,
  paths?: string[],
  worktreeId?: number,
): Promise<{ ok: boolean }> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/stage${query}`, { paths: paths || [] })
}

export function unstageGitFiles(
  projectId: number,
  paths?: string[],
  worktreeId?: number,
): Promise<{ ok: boolean }> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/unstage${query}`, { paths: paths || [] })
}

export function discardGitFile(
  projectId: number,
  path: string,
  worktreeId?: number,
): Promise<{ ok: boolean }> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/discard${query}`, { path })
}

export function commitGitChanges(
  projectId: number,
  message: string,
  worktreeId?: number,
): Promise<GitCommitItem> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  return mutate<GitCommitItem>('POST', `/projects/${projectId}/git/commit${query}`, { message })
}

export function getGitBranches(
  projectId: number,
  worktreeId?: number,
): Promise<{ branches: string[]; current: string }> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  return get<{ branches: string[]; current: string }>(
    `/projects/${projectId}/git/branches${query}`,
  )
}

export function checkoutGitBranch(
  projectId: number,
  branch: string,
  create?: boolean,
  worktreeId?: number,
): Promise<{ ok: boolean }> {
  const params = withWorktree(new URLSearchParams(), worktreeId)
  const query = params.toString() ? `?${params.toString()}` : ''
  return mutate<{ ok: boolean }>('POST', `/projects/${projectId}/git/branches${query}`, {
    branch,
    create: Boolean(create),
  })
}
