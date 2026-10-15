// Git state of an instance's working directory. Mirrors
// #/components/schemas/GitInfo: isGit false means the directory is not a git
// work tree (or git is unavailable) and every other field is empty then.
import { apiRequest } from './client'

export interface GitInfo {
  isGit: boolean
  branch?: string
  dirty?: boolean
  root?: string
}

// GET /api/projects/{id}/instances/{instanceId}/git-info
export function getInstanceGitInfo(projectId: number, instanceId: string): Promise<GitInfo> {
  return apiRequest<GitInfo>(
    'GET',
    `/projects/${projectId}/instances/${encodeURIComponent(instanceId)}/git-info`,
  )
}
