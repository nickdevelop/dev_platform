// Git state of a project's working directory. Mirrors
// #/components/schemas/GitInfo: isGit false means the directory is not a git
// work tree (or git is unavailable) and every other field is empty then.
import { get } from './client'

export interface GitInfo {
  isGit: boolean
  branch?: string
  dirty?: boolean
  root?: string
}

// GET /api/projects/{id}/git-info
export function getProjectGitInfo(projectId: number): Promise<GitInfo> {
  return get<GitInfo>(`/projects/${projectId}/git-info`)
}
