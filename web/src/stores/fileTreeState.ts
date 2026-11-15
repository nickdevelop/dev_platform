// Workspace file tree memory per open session. The right panel unmounts its
// file sidebar whenever the tree or the whole panel is collapsed, and the
// sidebar's expansions, search and tab choice used to vanish with it. The
// snapshot lives here instead, keyed by session id, so reopening the tree of a
// session finds it where the user left it. Memory only: the snapshot is dropped
// when the session's tab closes (AppShell retains only the open sessions), the
// same release rule the open tabs follow.
import { defineStore } from 'pinia'

export interface FileTreeSnapshot {
  activeTab: 'changes' | 'all'
  searchQuery: string
  selectedCommit: string
  expandedTreePaths: string[]
  expandedChangeDirs: string[]
  isStagedGroupExpanded: boolean
  isUnstagedGroupExpanded: boolean
}

export const useFileTreeStateStore = defineStore('fileTreeState', {
  state: () => ({
    snapshots: {} as Record<string, FileTreeSnapshot>,
  }),

  actions: {
    snapshotOf(sessionId: string): FileTreeSnapshot | undefined {
      return this.snapshots[sessionId]
    },

    save(sessionId: string, snapshot: FileTreeSnapshot) {
      this.snapshots[sessionId] = snapshot
    },

    // retainOnly drops the snapshot of every session not in the list, so a
    // closed tab releases its file tree memory.
    retainOnly(sessionIds: string[]) {
      const keep = new Set(sessionIds)
      for (const sessionId of Object.keys(this.snapshots)) {
        if (!keep.has(sessionId)) delete this.snapshots[sessionId]
      }
    },
  },
})
