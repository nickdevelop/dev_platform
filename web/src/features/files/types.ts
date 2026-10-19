// File and workspace tree types matching backend projects API.

export interface FileTreeNode {
  name: string
  path: string
  isDir: boolean
  size?: number
  children?: FileTreeNode[]
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
}

export interface FileTab {
  path: string
  name: string
}
