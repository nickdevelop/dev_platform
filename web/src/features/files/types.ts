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

// A right-clicked tree row: which node, and where on screen the cursor was, so
// the host can open the file actions menu under it.
export interface FileTreeMenuTarget {
  path: string
  isDir: boolean
  x: number
  y: number
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
  originalContent?: string
  modifiedContent?: string
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

export interface CodeCommentCard {
  id: string
  filePath: string
  fileName: string
  startLine: number
  endLine: number
  codeSnippet: string
  comment: string
  createdAt: number
}

