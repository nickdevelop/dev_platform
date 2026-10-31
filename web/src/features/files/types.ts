// File and workspace tree types matching backend projects API.

export interface FileTreeNode {
  name: string
  path: string
  isDir: boolean
  size?: number
  children?: FileTreeNode[]
  loading?: boolean
  loaded?: boolean
}

export interface FileContentResponse {
  path: string
  size: number
  lines: number
  mimeType: string
  isBinary: boolean
  content?: string
  truncated?: boolean
}

export type GitFileChangeStatus = 'modified' | 'added' | 'deleted' | 'untracked' | 'renamed'

export interface GitStatusItem {
  path: string
  status: GitFileChangeStatus
  staged?: boolean
}

export interface GitBranchesResponse {
  branches: string[]
  current: string
}

export interface GitCommitItem {
  hash: string
  shortHash: string
  message: string
  author: string
  date: string
  timestamp: number
}

export interface GitFileDiffResponse {
  path: string
  oldPath?: string
  status: GitFileChangeStatus | string
  patch: string
  additions: number
  deletions: number
  isBinary: boolean
}

export interface FileTab {
  path: string
  name: string
  mode?: 'view' | 'diff'
  commitRef?: string
}

export interface ChangeTreeNode {
  name: string
  path: string
  isDir: boolean
  status?: GitFileChangeStatus
  children?: ChangeTreeNode[]
  changeCount?: number
}
