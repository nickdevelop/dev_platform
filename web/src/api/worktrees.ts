// Git worktree API calls. Worktrees are platform-registered sibling checkout
// directories of a project's repository; the main checkout is not listed here
// and is addressed by worktreeId 0.
import { get, mutate } from './client'

// Mirrors #/components/schemas/ProjectWorktree.
export interface ProjectWorktree {
  id: number
  projectId: number
  path: string
  branch: string
  baseBranch?: string
  available: boolean
  createdAt: string
  updatedAt: string
}

interface WorktreeListResponse {
  worktrees: ProjectWorktree[]
}

export interface CreateWorktreeBody {
  branch: string
  baseBranch?: string
  path?: string
}

export async function listProjectWorktrees(projectId: number): Promise<ProjectWorktree[]> {
  const resp = await get<WorktreeListResponse>(`/projects/${projectId}/worktrees`)
  return resp.worktrees || []
}

export function createProjectWorktree(
  projectId: number,
  body: CreateWorktreeBody,
): Promise<ProjectWorktree> {
  return mutate<ProjectWorktree>('POST', `/projects/${projectId}/worktrees`, body)
}

export function removeProjectWorktree(projectId: number, worktreeId: number): Promise<void> {
  return mutate<void>('DELETE', `/projects/${projectId}/worktrees/${worktreeId}`)
}
