// Workspace file tree memory per open session. The right panel unmounts its
// file sidebar whenever the tree or the whole panel is collapsed, and the
// sidebar's expansions, search and tab choice used to vanish with it. The
// snapshot lives here instead, keyed by session id, so reopening the tree of a
// session finds it where the user left it. The loaded tree data is kept too, so
// reopening does not fetch it again until the session's turn ends (the agent may
// have changed files by then). Memory only: everything is dropped when the
// session's tab closes (AppShell retains only the open sessions), the same
// release rule the open tabs follow.
import { defineStore } from 'pinia'
import type { FileTreeNode, GitCommitItem, GitStatusItem } from '../features/files/types'

export interface FileTreeSnapshot {
  activeTab: 'changes' | 'all'
  searchQuery: string
  selectedCommit: string
  expandedTreePaths: string[]
  expandedChangeDirs: string[]
  isStagedGroupExpanded: boolean
  isUnstagedGroupExpanded: boolean
}

// The loaded data belongs to one project and worktree; it is only reused for
// the same scope.
export interface FileTreeData {
  projectId: number
  worktreeId: number
  selectedCommit: string
  treeNodes: FileTreeNode[]
  gitChanges: GitStatusItem[]
  commits: GitCommitItem[]
}

export const useFileTreeStateStore = defineStore('fileTreeState', {
  state: () => ({
    snapshots: {} as Record<string, FileTreeSnapshot>,
    data: {} as Record<string, FileTreeData>,
  }),

  actions: {
    snapshotOf(sessionId: string): FileTreeSnapshot | undefined {
      return this.snapshots[sessionId]
    },

    save(sessionId: string, snapshot: FileTreeSnapshot) {
      this.snapshots[sessionId] = snapshot
    },

    dataOf(sessionId: string): FileTreeData | undefined {
      return this.data[sessionId]
    },

    saveData(sessionId: string, data: FileTreeData) {
      this.data[sessionId] = data
    },

    // invalidateData drops the loaded tree data so the next open loads it again.
    invalidateData(sessionId: string) {
      delete this.data[sessionId]
    },

    // retainOnly drops the memory of every session not in the list, so a
    // closed tab releases its file tree memory.
    retainOnly(sessionIds: string[]) {
      const keep = new Set(sessionIds)
      for (const sessionId of Object.keys(this.snapshots)) {
        if (!keep.has(sessionId)) delete this.snapshots[sessionId]
      }
      for (const sessionId of Object.keys(this.data)) {
        if (!keep.has(sessionId)) delete this.data[sessionId]
      }
    },
  },
})
